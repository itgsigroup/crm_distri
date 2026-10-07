package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"distri-arc/internal/wa"
)

// Many numbers, one per user: a new number is linked first (scan in Chat) and reported by WhatsApp, then given to
// a user of the user master; one user holds one number; a sales user reads only their own number's chats.
func TestWANumbersAndChatScope(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	code, cs := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "cs@gsi.co.id", "name": "CS Kantor", "role_key": "admin", "password": "rahasia-panjang-1"})
	if code != http.StatusCreated {
		t.Fatalf("create user: %d %v", code, cs)
	}
	csID := cs["id"].(string)
	// the phone scans: the link is pending, then WhatsApp reports its number
	if _, err := st.Pool.Exec(ctx, "insert into wa_links (session_id, created_by) values ('link-t1', 'sam@gsi.co.id')"); err != nil {
		t.Fatal(err)
	}
	in := wa.NewIngestor(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := in.ProcessStatus(ctx, wa.Status{Session: "link-t1", State: "pairing", QR: "2@qr"}); err != nil {
		t.Fatal(err)
	}
	if code, l := get(t, srv, "/api/wa/links/link-t1", "sam@gsi.co.id"); code != 200 || l["state"] != "pairing" || l["qr_png"] == nil {
		t.Fatalf("pending link %d %v", code, l)
	}
	if err := in.ProcessStatus(ctx, wa.Status{Session: "link-t1", Account: "6281299990000", State: "connected"}); err != nil {
		t.Fatal(err)
	}
	if code, l := get(t, srv, "/api/wa/links/link-t1", "sam@gsi.co.id"); code != 200 || l["state"] != "connected" || l["wa_number"] != "6281299990000" {
		t.Fatalf("linked %d %v", code, l)
	}
	if code, _ := put(t, srv.URL+"/api/wa/numbers/6281299990000/user", "andi@gsi.co.id", map[string]any{"user_id": csID}); code != http.StatusForbidden {
		t.Fatalf("sales gives a number to another user: %d", code)
	}
	if code, out := put(t, srv.URL+"/api/wa/numbers/6281299990000/user", "sam@gsi.co.id", map[string]any{"user_id": csID}); code != 200 || out["label"] != "CS Kantor" {
		t.Fatalf("assign: %d %v", code, out)
	}
	// Andi already holds a number: a second one is refused
	var andiID string
	_ = st.Pool.QueryRow(ctx, "select id from users where email = 'andi@gsi.co.id'").Scan(&andiID)
	if _, err := st.Pool.Exec(ctx, "insert into wa_links (session_id) values ('link-t2')"); err != nil {
		t.Fatal(err)
	}
	if err := in.ProcessStatus(ctx, wa.Status{Session: "link-t2", Account: "6281277770000", State: "connected"}); err != nil {
		t.Fatal(err)
	}
	if code, _ := put(t, srv.URL+"/api/wa/numbers/6281277770000/user", "sam@gsi.co.id", map[string]any{"user_id": andiID}); code != http.StatusConflict {
		t.Fatalf("second number for one user: %d", code)
	}
	_, users := get(t, srv, "/api/users", "sam@gsi.co.id")
	for _, x := range users["items"].([]any) {
		if m := x.(map[string]any); m["email"] == "cs@gsi.co.id" && (m["wa_number"] != "6281299990000" || m["wa_state"] != "connected") {
			t.Fatalf("user master shows the number: %v", m)
		}
	}
	var session string
	_ = st.Pool.QueryRow(ctx, "select session_id from wa_numbers where wa_number = '6281299990000'").Scan(&session)
	if session != "link-t1" {
		t.Fatalf("number keeps its link session: %q", session)
	}
	_, all := get(t, srv, "/api/wa/status", "sam@gsi.co.id")
	_, own := get(t, srv, "/api/wa/status", "andi@gsi.co.id")
	if n, m := len(all["items"].([]any)), len(own["items"].([]any)); n != 6 || m != 1 {
		t.Fatalf("numbers: ceo %d, andi %d", n, m)
	}
	_, ceo := get(t, srv, "/api/chat/threads", "sam@gsi.co.id")
	_, andi := get(t, srv, "/api/chat/threads", "andi@gsi.co.id")
	ct, at := ceo["items"].([]any), andi["items"].([]any)
	if len(at) == 0 || len(at) >= len(ct) {
		t.Fatalf("threads: ceo %d, andi %d", len(ct), len(at))
	}
	for _, x := range at {
		if x.(map[string]any)["account"] != "6281234504471" {
			t.Fatalf("andi sees another number's chat: %v", x)
		}
	}
	var other string
	for _, x := range ct {
		if m := x.(map[string]any); m["account"] != "6281234504471" {
			other = m["id"].(string)
			break
		}
	}
	if code, _ := get(t, srv, "/api/chat/threads/"+other, "andi@gsi.co.id"); code != http.StatusNotFound {
		t.Fatalf("andi opens another number's chat: %d", code)
	}
	_, f := get(t, srv, "/api/chat/threads?account=6281534509032", "sam@gsi.co.id")
	for _, x := range f["items"].([]any) {
		if x.(map[string]any)["account"] != "6281534509032" {
			t.Fatal("account filter")
		}
	}
}
