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

// The chat list's row menu (migration 0040): a person pins up to three chats to the top of their own list; "Tandai
// belum dibaca" brings a count back; "Tandai belum dibalas" makes a chat wait until a reply goes out and "Tandai
// sudah dibalas" ends a wait.
func TestChatListRowMenu(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	h := api.New(config.Config{Env: "dev", ChatWindowDays: 14}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	ceo, other := "sam@gsi.co.id", "admin@gsi.co.id"
	var ids []string
	for i := range 4 {
		var id string
		if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', $1, $1, '6281234504471', now()) returning id",
			fmt.Sprintf("62813000004%02d@s.whatsapp.net", i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err := st.Pool.Exec(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, 'RM1', 'in', 'harga?', now() - interval '1 hour', 'received')", ids[0]); err != nil {
		t.Fatal(err)
	}
	row := func(user, id string) map[string]any {
		_, list := get(t, srv, "/api/chat/threads", user)
		for _, x := range list["items"].([]any) {
			if m := x.(map[string]any); m["id"] == id {
				return m
			}
		}
		t.Fatalf("thread %s not listed for %s", id, user)
		return nil
	}
	base := srv.URL + "/api/chat/threads/"
	for _, id := range ids[:3] {
		if code, _ := sendJSON(t, "PUT", base+id+"/pin-chat", ceo, map[string]any{"pinned": true}); code != 200 {
			t.Fatalf("pin %d", code)
		}
	}
	if code, _ := sendJSON(t, "PUT", base+ids[3]+"/pin-chat", ceo, map[string]any{"pinned": true}); code != http.StatusConflict {
		t.Fatalf("a fourth pin %d", code)
	}
	if row(ceo, ids[0])["pinned_at"] == nil || row(other, ids[0])["pinned_at"] != nil {
		t.Fatal("a pin belongs to the person who pinned")
	}
	if code, _ := sendJSON(t, "PUT", base+ids[0]+"/pin-chat", ceo, map[string]any{"pinned": false}); code != 200 || row(ceo, ids[0])["pinned_at"] != nil {
		t.Fatalf("unpin %d", code)
	}

	if code, _ := sendJSON(t, "POST", base+ids[1]+"/unread", ceo, nil); code != http.StatusNoContent || row(ceo, ids[1])["unread"] != float64(1) {
		t.Fatalf("mark unread %d", code)
	}

	// the customer's question waits; "sudah dibalas" ends it; "belum dibalas" on a quiet chat starts one
	if row(ceo, ids[0])["waiting_since"] == nil {
		t.Fatal("the customer's question is not waiting")
	}
	if code, _ := sendJSON(t, "PUT", base+ids[0]+"/reply-state", ceo, map[string]any{"state": "replied"}); code != http.StatusNoContent || row(ceo, ids[0])["waiting_since"] != nil {
		t.Fatalf("marked replied %d, still waiting", code)
	}
	if code, _ := sendJSON(t, "PUT", base+ids[2]+"/reply-state", ceo, map[string]any{"state": "unreplied"}); code != http.StatusNoContent || row(ceo, ids[2])["waiting_since"] == nil {
		t.Fatalf("marked unreplied %d, not waiting", code)
	}
	// a reply going out ends the hand-made wait
	if _, err := st.Pool.Exec(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, 'RM2', 'out', 'siap', now() + interval '1 second', 'sent')", ids[2]); err != nil {
		t.Fatal(err)
	}
	if row(ceo, ids[2])["waiting_since"] != nil {
		t.Fatal("still waiting after the reply")
	}
}
