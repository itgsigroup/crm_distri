package api_test

import (
	"net/http"
	"slices"
	"testing"
)

func strs(v any) []string {
	out := []string{}
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

// Role master: admin reads, only the CEO changes; a role narrows its base (menu + API + decisions) and never widens it.
func TestRoleMaster(t *testing.T) {
	srv, _ := chatServer(t)
	if code, _ := get(t, srv, "/api/roles", "andi@gsi.co.id"); code != http.StatusForbidden {
		t.Fatalf("sales reads roles: %d", code)
	}
	code, list := get(t, srv, "/api/roles", "admin@gsi.co.id")
	if code != 200 || len(list["items"].([]any)) != 5 || len(list["bases"].([]any)) != 5 {
		t.Fatalf("roles %d %v", code, list)
	}
	role := map[string]any{"name": "Sales Telemarketing", "base": "sales", "screens": []string{"today", "chat", "dealer", "conn"}, "decide": []string{"followup", "reply", "credit_limit"}, "wa_allowed": true}
	if code, _ := post(t, srv.URL+"/api/roles", "admin@gsi.co.id", role); code != http.StatusForbidden {
		t.Fatalf("admin creates role: %d", code)
	}
	if code, _ := post(t, srv.URL+"/api/roles", "sam@gsi.co.id", map[string]any{"name": "Wakil CEO", "base": "ceo"}); code != http.StatusBadRequest {
		t.Fatalf("custom CEO base accepted: %d", code)
	}
	code, out := post(t, srv.URL+"/api/roles", "sam@gsi.co.id", role)
	if code != 200 || out["key"] != "sales-telemarketing" {
		t.Fatalf("create role: %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "tele@gsi.co.id", "name": "Tele", "role_key": "sales-telemarketing", "password": "rahasia-panjang-1", "branch": "Semarang"}); code != http.StatusCreated {
		t.Fatalf("create user: %d", code)
	}
	_, me := get(t, srv, "/api/me", "tele@gsi.co.id")
	if s := strs(me["screens"]); !slices.Equal(s, []string{"today", "chat", "dealer"}) { // "conn" is not a sales screen
		t.Fatalf("screens %v", s)
	}
	if d := strs(me["decide"]); !slices.Equal(d, []string{"followup", "reply"}) { // credit_limit is not a sales decision
		t.Fatalf("decide %v", d)
	}
	if me["role"] != "sales" || me["role_name"] != "Sales Telemarketing" {
		t.Fatalf("me %v", me)
	}
	for _, p := range []string{"/api/relasi?period=90", "/api/stock/aging"} {
		if code, _ := get(t, srv, p, "tele@gsi.co.id"); code != http.StatusForbidden {
			t.Fatalf("%s without its screen: %d", p, code)
		}
	}
	if code, _ := get(t, srv, "/api/chat/threads", "tele@gsi.co.id"); code != 200 {
		t.Fatalf("chat with its screen: %d", code)
	}
	if code, _ := del(t, srv.URL+"/api/roles/sales-telemarketing", "sam@gsi.co.id"); code != http.StatusConflict {
		t.Fatalf("role in use deleted: %d", code)
	}
	if code, _ := del(t, srv.URL+"/api/roles/sales", "sam@gsi.co.id"); code != http.StatusConflict {
		t.Fatalf("system role deleted: %d", code)
	}
	// the only CEO cannot be moved to another role
	var samID string
	_, users := get(t, srv, "/api/users", "sam@gsi.co.id")
	for _, x := range users["items"].([]any) {
		if m := x.(map[string]any); m["email"] == "sam@gsi.co.id" {
			samID = m["id"].(string)
		}
	}
	if code, _ := put(t, srv.URL+"/api/users/"+samID, "sam@gsi.co.id", map[string]any{"role_key": "admin"}); code != http.StatusBadRequest {
		t.Fatalf("last CEO demoted: %d", code)
	}
}

func del(t *testing.T, url, user string) (int, string) {
	t.Helper()
	return send(t, http.MethodDelete, url, user, "", nil)
}
