package api_test

import (
	"context"
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
	"distri-arc/internal/wa"
)

type fakeRoster []wa.RosterMember

func (f fakeRoster) GroupMembers(context.Context, string, string) ([]wa.RosterMember, error) {
	return f, nil
}

// A group's members (Anggota): WhatsApp's list through the group thread's number — admins first, the number itself
// left out, hidden (LID-only) members counted — plus who wrote in it. A click on a member opens the one-to-one chat of
// that GSI number with them, made once and found again after; a number outside the group opens nothing.
func TestGroupMembersOpenTheirChat(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	const andi, budi, rina = "6281234504471", "6285700001234", "6285700005678"
	roster := fakeRoster{{Number: andi}, {Number: budi, Name: "Budi WA", Admin: true}, {Number: ""}}
	h := api.New(config.Config{Env: "dev", ChatWindowDays: 14}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).WithRoster(roster).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ceo := "sam@gsi.co.id"
	var g string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('group', '120363000000888@g.us', 'Proyek Y', $1, now()) returning id", andi).Scan(&g); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, from_number, from_name, body, sent_at, status) values ($1, 'GM1', 'in', $2, 'Rina', 'halo', now(), 'received')", g, rina); err != nil {
		t.Fatal(err)
	}
	code, out := get(t, srv, "/api/chat/threads/"+g+"/members", ceo)
	items, _ := out["items"].([]any)
	if code != 200 || len(items) != 2 || out["hidden"] != float64(1) {
		t.Fatalf("members %d %v", code, out)
	}
	first := items[0].(map[string]any)
	if first["number"] != budi || first["admin"] != true || first["name"] != "Budi WA" || items[1].(map[string]any)["number"] != rina {
		t.Fatalf("members order or names %v", items)
	}
	open := srv.URL + "/api/chat/threads/" + g + "/members/" + budi + "/open"
	code, o1 := sendJSON(t, "POST", open, ceo, map[string]any{})
	if code != http.StatusCreated || o1["thread_id"] == nil {
		t.Fatalf("open %d %v", code, o1)
	}
	code, o2 := sendJSON(t, "POST", open, ceo, map[string]any{})
	if code != 200 || o2["thread_id"] != o1["thread_id"] {
		t.Fatalf("open again %d %v (first %v)", code, o2, o1["thread_id"])
	}
	var kind, title, account string
	if err := st.Pool.QueryRow(ctx, "select kind, title, account from chat_threads where id = $1", o1["thread_id"]).Scan(&kind, &title, &account); err != nil {
		t.Fatal(err)
	}
	if kind != "new" || title != "Budi WA" || account != andi {
		t.Fatalf("opened chat %s %q %s", kind, title, account)
	}
	if code, page := get(t, srv, "/api/chat/threads/"+o1["thread_id"].(string), ceo); code != 200 || len(page["messages"].([]any)) != 0 {
		t.Fatalf("open chat page %d %v", code, page)
	}
	if code, _ := sendJSON(t, "POST", srv.URL+"/api/chat/threads/"+g+"/members/6289999999999/open", ceo, map[string]any{}); code != http.StatusNotFound {
		t.Fatalf("non-member opened: %d", code)
	}
}
