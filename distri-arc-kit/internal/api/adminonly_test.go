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

// adminOnlyGroups: the number is in one group where only admins write (and it is none) and one open group.
type adminOnlyGroups struct{}

func (adminOnlyGroups) GroupMembers(context.Context, string, string) ([]wa.RosterMember, error) {
	return nil, nil
}

func (adminOnlyGroups) Groups(context.Context, string) ([]wa.Group, error) {
	return []wa.Group{{JID: "120363000000071@g.us", Name: "Info Promo GSI", AdminOnly: true}, {JID: "120363000000072@g.us", Name: "Proyek"}}, nil
}

// A group where only admins may send, this number being none: the chat says so (the composer closes) and a send is
// refused before it reaches the outbox; an open group sends as usual.
func TestAdminOnlyGroup(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.New(config.Config{Env: "dev"}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).WithRoster(adminOnlyGroups{}).Handler())
	t.Cleanup(srv.Close)
	const ceo, account = "sam@gsi.co.id", "628990000002"
	var locked, open string
	for jid, id := range map[string]*string{"120363000000071@g.us": &locked, "120363000000072@g.us": &open} {
		if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('group', $1, 'Grup', $2, now()) returning id`, jid, account).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	adminOnly := func(id string) bool {
		_, out := get(t, srv, "/api/chat/threads/"+id, ceo)
		v, _ := out["thread"].(map[string]any)["admin_only"].(bool)
		return v
	}
	for i := 0; !adminOnly(locked); i++ { // the group list is read in the background the first time
		if i > 100 {
			t.Fatal("admin_only never set")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if adminOnly(open) {
		t.Fatal("an open group is not admin-only")
	}
	if code, out := post(t, srv.URL+"/api/chat/threads/"+locked+"/messages", ceo, map[string]any{"body": "Halo semua"}); code != http.StatusConflict {
		t.Fatalf("send to an admin-only group: %d %v", code, out)
	}
	var n int
	_ = st.Pool.QueryRow(ctx, "select count(*) from chat_messages where thread_id = $1", locked).Scan(&n)
	if n != 0 {
		t.Fatalf("%d messages stored for a refused send", n)
	}
	if code, out := post(t, srv.URL+"/api/chat/threads/"+open+"/messages", ceo, map[string]any{"body": "Halo semua"}); code != http.StatusAccepted {
		t.Fatalf("send to an open group: %d %v", code, out)
	}
}
