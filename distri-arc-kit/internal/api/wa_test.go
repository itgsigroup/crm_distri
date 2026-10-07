package api_test

import (
	"net/http"
	"testing"
)

// Many numbers, one per user: a number is added for a user of the user master (its number comes from there);
// sales cannot add for others; a user holds one number; a sales user reads only their own number's chats.
func TestWANumbersAndChatScope(t *testing.T) {
	srv, _ := chatServer(t)
	code, cs := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "cs@gsi.co.id", "name": "CS Kantor", "role_key": "admin", "password": "rahasia-panjang-1", "wa_number": "0812 9999 0000"})
	if code != http.StatusCreated {
		t.Fatalf("create user: %d %v", code, cs)
	}
	csID := cs["id"].(string)
	if code, _ := post(t, srv.URL+"/api/wa/numbers", "andi@gsi.co.id", map[string]string{"user_id": csID}); code != http.StatusForbidden {
		t.Fatalf("sales adds another user's number: %d", code)
	}
	if code, out := post(t, srv.URL+"/api/wa/numbers", "sam@gsi.co.id", map[string]string{"user_id": csID}); code != http.StatusCreated || out["wa_number"] != "6281299990000" || out["label"] != "CS Kantor" {
		t.Fatalf("ceo adds the user's number: %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "cs2@gsi.co.id", "name": "CS 2", "role_key": "admin", "password": "rahasia-panjang-1", "wa_number": "+62 812-9999-0000"}); code != http.StatusConflict {
		t.Fatalf("second user with the same number: %d", code)
	}
	code, u2 := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "cs3@gsi.co.id", "name": "CS 3", "role_key": "admin", "password": "rahasia-panjang-1"})
	if code != http.StatusCreated {
		t.Fatalf("user without number: %d", code)
	}
	if code, _ := post(t, srv.URL+"/api/wa/numbers", "sam@gsi.co.id", map[string]string{"user_id": u2["id"].(string), "wa_number": "12345"}); code != http.StatusBadRequest {
		t.Fatalf("bad number accepted: %d", code)
	}
	if code, out := put(t, srv.URL+"/api/users/"+csID, "sam@gsi.co.id", map[string]any{"wa_number": "0812 7777 0000"}); code != http.StatusConflict {
		t.Fatalf("number changed while linked in Chat: %d %v", code, out)
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
