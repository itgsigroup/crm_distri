package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

// fakeWAProfile is WhatsApp's side of a profile: photos (6281900300102 has one, the rest none), the Info line and
// the groups of the GSI number.
type fakeWAProfile struct{ photos atomic.Int32 }

func (f *fakeWAProfile) Media(context.Context, string, wa.Media) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeWAProfile) Photo(_ context.Context, _, jid string, full bool) ([]byte, string, error) {
	f.photos.Add(1)
	if !strings.HasPrefix(jid, "6281900300102") {
		return nil, "", wa.ErrNoPhoto
	}
	if full {
		return []byte("BIGJPEG"), "image/jpeg", nil
	}
	return []byte("JPEG"), "image/jpeg", nil
}

func (f *fakeWAProfile) Profile(context.Context, string, string) (wa.Profile, error) {
	return wa.Profile{Name: "Toko Sinar Elektronik", About: "Ready stock CCTV", Business: true}, nil
}

func (f *fakeWAProfile) GroupMembers(context.Context, string, string) ([]wa.RosterMember, error) {
	return nil, nil
}

func (f *fakeWAProfile) Groups(context.Context, string) ([]wa.Group, error) {
	return []wa.Group{
		{JID: "120363000000001@g.us", Name: "Proyek Sinar x GSI", Members: []wa.RosterMember{{Number: "6281900300102", Admin: true}, {Number: "6281234504471"}, {Number: "6281300000999"}}},
		{JID: "120363000000002@g.us", Name: "Grup lain", Members: []wa.RosterMember{{Number: "6281300000999"}}},
	}, nil
}

// ADR 0071: the chat list shows the contact's profile photo, cached so WhatsApp is not asked on every load (a
// contact without one is remembered too); the profile shows WhatsApp's name and Info and the groups the contact
// shares with GSI's numbers, linked to the group's chat.
func TestChatPhotoAndProfile(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	f := &fakeWAProfile{}
	srv := httptest.NewServer(api.New(config.Config{Env: "dev"}, st, clock.Fixed(chatNow), slog.New(slog.NewTextHandler(io.Discard, nil))).WithMedia(f).WithRoster(f).Handler())
	t.Cleanup(srv.Close)
	const ceo = "sam@gsi.co.id"
	sinar := sinarThread(t, st)
	var account string
	if err := st.Pool.QueryRow(ctx, "select account from chat_threads where id = $1", sinar).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into wa_numbers (wa_number, session_id, label, transport, state) values ($1, $1, 'Andi - Sales', 'fake', 'connected')
		on conflict (wa_number) do update set state = 'connected', label = 'Andi - Sales'`, account); err != nil {
		t.Fatal(err)
	}
	var group, other string
	if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('group', '120363000000001@g.us', 'Proyek Sinar x GSI', $1, now()) returning id`, account).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000777@s.whatsapp.net', 'Tanpa foto', $1, now()) returning id`, account).Scan(&other); err != nil {
		t.Fatal(err)
	}
	photo := func(id, q string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/chat/threads/"+id+"/photo"+q, nil)
		req.Header.Set("X-Dev-User", ceo)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	for range 2 {
		if code, body := photo(sinar.String(), ""); code != http.StatusOK || body != "JPEG" {
			t.Fatalf("photo: %d %q", code, body)
		}
	}
	if code, body := photo(sinar.String(), "?full=1"); code != http.StatusOK || body != "BIGJPEG" {
		t.Fatalf("full photo: %d %q", code, body)
	}
	for range 2 {
		if code, _ := photo(other, ""); code != http.StatusNotFound {
			t.Fatalf("no photo: %d", code)
		}
	}
	if n := f.photos.Load(); n != 3 { // small + full of Sinar, and "none" of the other — each asked once
		t.Fatalf("WhatsApp asked %d times", n)
	}

	_, p := get(t, srv, "/api/chat/threads/"+sinar.String()+"/profile", ceo)
	if p["number"] != "6281900300102" || p["about"] != "Ready stock CCTV" || p["name"] != "Toko Sinar Elektronik" || p["business"] != true {
		t.Fatalf("profile: %v", p)
	}
	gs, _ := p["groups"].([]any)
	if len(gs) != 1 {
		t.Fatalf("groups: %v", p["groups"])
	}
	g := gs[0].(map[string]any)
	if g["name"] != "Proyek Sinar x GSI" || g["thread_id"] != group || g["members"] != float64(3) || g["admin"] != true || g["via"] != "Andi - Sales" {
		t.Fatalf("group: %v", g)
	}
}
