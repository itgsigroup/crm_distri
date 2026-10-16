package api_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// Pinned messages and labels (migration 0039): at most three pins per conversation, the oldest replaced; a group's
// pins and labels are the same under every GSI number in it; anyone in Chat adds a label, only admins rename or
// delete one; labels show on the chat row and on the bubble.
func TestChatPinsAndLabels(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	h := api.New(config.Config{Env: "dev", ChatWindowDays: 14}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	ceo, sales := "sam@gsi.co.id", "andi@gsi.co.id"
	jid := "120363000000777@g.us"
	thread := func(account string) string {
		var id string
		if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('group', $1, 'Proyek X', $2, now()) returning id", jid, account).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := thread("6281234504471"), thread("6281534509032")
	var msgs []string
	for i := range 5 {
		var id string
		if err := st.Pool.QueryRow(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, from_name, body, sent_at, status) values ($1, $2, 'in', 'Budi', $3, now() - $4::interval, 'received') returning id",
			a, fmt.Sprintf("PIN%d", i), fmt.Sprintf("pesan %d", i), fmt.Sprintf("%d hours", 5-i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, id)
	}
	base := srv.URL + "/api/chat/threads/"
	for i, m := range msgs {
		if code, out := sendJSON(t, "PUT", base+a+"/messages/"+m+"/pin", ceo, map[string]any{"pinned": true}); code != 200 || len(out["pins"].([]any)) != i+1 {
			t.Fatalf("pin %d %v", code, out)
		}
	}
	// no limit (Sam 16 Oct): the other GSI number's view of the group holds all five pins, newest first
	_, page := get(t, srv, "/api/chat/threads/"+b, ceo)
	pins := page["pins"].([]any)
	if len(pins) != 5 || pins[0].(map[string]any)["message_id"] != msgs[4] || pins[4].(map[string]any)["message_id"] != msgs[0] {
		t.Fatalf("pins under the other number %v", pins)
	}
	pinned := 0
	for _, m := range page["messages"].([]any) {
		if m.(map[string]any)["pinned"] == true {
			pinned++
		}
	}
	if pinned != 5 {
		t.Fatalf("%d bubbles marked pinned", pinned)
	}
	if code, out := sendJSON(t, "PUT", base+b+"/messages/"+msgs[3]+"/pin", ceo, map[string]any{"pinned": false}); code != 200 || len(out["pins"].([]any)) != 4 {
		t.Fatalf("unpin %d %v", code, out)
	}

	// labels: a sales adds one; only an admin renames or deletes it
	code, lbl := sendJSON(t, "POST", srv.URL+"/api/chat/labels", sales, map[string]any{"name": "  Proyek   RS ", "color": "#12ab34"})
	if code != http.StatusCreated || lbl["name"] != "Proyek RS" || lbl["color"] != "#12AB34" {
		t.Fatalf("create %d %v", code, lbl)
	}
	lid := lbl["id"].(string)
	if code, _ := sendJSON(t, "POST", srv.URL+"/api/chat/labels", sales, map[string]any{"name": "proyek rs", "color": "#000000"}); code != http.StatusConflict {
		t.Fatalf("duplicate name %d", code)
	}
	if code, _ := sendJSON(t, "PUT", srv.URL+"/api/chat/labels/"+lid, sales, map[string]any{"name": "X", "color": "#000000"}); code != http.StatusForbidden {
		t.Fatalf("sales renamed a label %d", code)
	}
	if code, _ := sendJSON(t, "PUT", base+a+"/labels", ceo, map[string]any{"label_ids": []string{lid}}); code != 200 {
		t.Fatalf("chat label %d", code)
	}
	if code, _ := sendJSON(t, "PUT", base+a+"/messages/"+msgs[0]+"/labels", ceo, map[string]any{"label_ids": []string{lid}}); code != 200 {
		t.Fatalf("message label %d", code)
	}
	_, list := get(t, srv, "/api/chat/threads", ceo)
	rows := 0
	for _, x := range list["items"].([]any) {
		m := x.(map[string]any)
		if (m["id"] == a || m["id"] == b) && len(m["labels"].([]any)) == 1 {
			rows++
		}
	}
	if rows != 2 {
		t.Fatalf("label on %d group rows, want both numbers", rows)
	}
	_, page = get(t, srv, "/api/chat/threads/"+b, ceo)
	if l := page["thread"].(map[string]any)["labels"].([]any); len(l) != 1 {
		t.Fatalf("thread labels %v", l)
	}
	if m := page["messages"].([]any)[0].(map[string]any); m["id"] != msgs[0] || len(m["labels"].([]any)) != 1 {
		t.Fatalf("bubble labels %v", m)
	}
	_, ls := get(t, srv, "/api/chat/labels", ceo)
	if ls["can_manage"] != true || len(ls["items"].([]any)) != 6 {
		t.Fatalf("labels %v", ls)
	}
	if code, _ := sendJSON(t, "DELETE", srv.URL+"/api/chat/labels/"+lid, ceo, nil); code != http.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
	if _, page = get(t, srv, "/api/chat/threads/"+a, ceo); page["thread"].(map[string]any)["labels"] != nil && len(page["thread"].(map[string]any)["labels"].([]any)) != 0 {
		t.Fatalf("deleted label still on the chat")
	}
}
