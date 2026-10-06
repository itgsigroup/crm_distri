package api_test

import (
	"net/http"
	"testing"
)

// Many numbers: CEO/admin add a team number; sales cannot; a sales user reads only their own numbers' chats.
func TestWANumbersAndChatScope(t *testing.T) {
	srv, _ := chatServer(t)
	if code, _ := post(t, srv.URL+"/api/wa/numbers", "andi@gsi.co.id", map[string]string{"wa_number": "0812 9999 0000", "label": "CS Kantor"}); code != http.StatusForbidden {
		t.Fatalf("sales adds number: %d", code)
	}
	if code, out := post(t, srv.URL+"/api/wa/numbers", "sam@gsi.co.id", map[string]string{"wa_number": "0812 9999 0000", "label": "CS Kantor"}); code != http.StatusCreated || out["wa_number"] != "6281299990000" {
		t.Fatalf("ceo adds number: %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/wa/numbers", "sam@gsi.co.id", map[string]string{"wa_number": "12345", "label": "x"}); code != http.StatusBadRequest {
		t.Fatalf("bad number accepted: %d", code)
	}
	_, all := get(t, srv, "/api/wa/status", "sam@gsi.co.id")
	_, own := get(t, srv, "/api/wa/status", "andi@gsi.co.id")
	if n, m := len(all["items"].([]any)), len(own["items"].([]any)); n != 5 || m != 1 {
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
