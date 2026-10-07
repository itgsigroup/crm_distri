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

// Role master: roles are fully custom (pages, data scope, decisions, policy rights); the page guards its API; only a
// policy holder changes roles; there is always one active policy holder.
func TestRoleMaster(t *testing.T) {
	srv, _ := chatServer(t)
	if code, _ := get(t, srv, "/api/roles", "andi@gsi.co.id"); code != http.StatusForbidden {
		t.Fatalf("sales reads roles: %d", code)
	}
	code, list := get(t, srv, "/api/roles", "admin@gsi.co.id")
	if code != 200 || len(list["items"].([]any)) != 5 || len(list["screens"].([]any)) != 14 {
		t.Fatalf("roles %d %v", code, list)
	}
	role := map[string]any{"name": "Sales Telemarketing", "scope": "own", "screens": []string{"today", "chat", "dealer", "ar", "users"}, "decide": []string{"followup", "reply", "credit_limit", "credit_release"}, "wa_allowed": true}
	if code, _ := post(t, srv.URL+"/api/roles", "admin@gsi.co.id", role); code != http.StatusForbidden {
		t.Fatalf("admin creates role: %d", code)
	}
	code, out := post(t, srv.URL+"/api/roles", "sam@gsi.co.id", role)
	if code != 200 || out["key"] != "sales-telemarketing" || out["base"] != "sales" {
		t.Fatalf("create role: %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "tele@gsi.co.id", "name": "Tele", "role_key": "sales-telemarketing", "password": "rahasia-panjang-1", "branch": "Gudang Bulan"}); code != http.StatusBadRequest {
		t.Fatalf("branch outside the master accepted: %d", code)
	}
	if code, _ := post(t, srv.URL+"/api/users", "sam@gsi.co.id", map[string]any{"email": "tele@gsi.co.id", "name": "Tele", "role_key": "sales-telemarketing", "password": "rahasia-panjang-1", "branch": "Semarang"}); code != http.StatusCreated {
		t.Fatalf("create user: %d", code)
	}
	_, me := get(t, srv, "/api/me", "tele@gsi.co.id")
	if s := strs(me["screens"]); !slices.Equal(s, []string{"today", "chat", "dealer"}) { // own data: no Kredit, no Pengguna
		t.Fatalf("screens %v", s)
	}
	if d := strs(me["decide"]); !slices.Equal(d, []string{"followup", "credit_limit", "reply"}) { // credit release needs policy rights
		t.Fatalf("decide %v", d)
	}
	if me["role"] != "sales" || me["role_name"] != "Sales Telemarketing" || me["scope"] != "own" {
		t.Fatalf("me %v", me)
	}
	for _, p := range []string{"/api/relasi?period=90", "/api/stock/aging", "/api/users"} {
		if code, _ := get(t, srv, p, "tele@gsi.co.id"); code != http.StatusForbidden {
			t.Fatalf("%s without its page: %d", p, code)
		}
	}
	if code, _ := get(t, srv, "/api/chat/threads", "tele@gsi.co.id"); code != 200 {
		t.Fatalf("chat with its page: %d", code)
	}
	// a finance-like custom role: all data, Kredit page, no management → base finance
	code, out = post(t, srv.URL+"/api/roles", "sam@gsi.co.id", map[string]any{"name": "Kasir", "scope": "all", "screens": []string{"today", "ar"}, "decide": []string{"collect"}})
	if code != 200 || out["base"] != "finance" {
		t.Fatalf("kasir %d %v", code, out)
	}
	if code, _ := del(t, srv.URL+"/api/roles/sales-telemarketing", "sam@gsi.co.id"); code != http.StatusConflict {
		t.Fatalf("role in use deleted: %d", code)
	}
	if code, _ := del(t, srv.URL+"/api/roles/kasir", "sam@gsi.co.id"); code != http.StatusNoContent {
		t.Fatalf("unused role kept: %d", code)
	}
	// the only policy holder cannot lose policy rights
	if code, _ := put(t, srv.URL+"/api/roles/ceo", "sam@gsi.co.id", map[string]any{"name": "CEO", "scope": "all", "screens": []string{"today", "conn"}, "decide": []string{}, "policies": false}); code != http.StatusBadRequest {
		t.Fatalf("last policy role dropped: %d", code)
	}
	var samID string
	_, users := get(t, srv, "/api/users", "sam@gsi.co.id")
	for _, x := range users["items"].([]any) {
		if m := x.(map[string]any); m["email"] == "sam@gsi.co.id" {
			samID = m["id"].(string)
		}
	}
	if code, _ := put(t, srv.URL+"/api/users/"+samID, "sam@gsi.co.id", map[string]any{"role_key": "admin"}); code != http.StatusBadRequest {
		t.Fatalf("last policy holder demoted: %d", code)
	}
	// branch master: the Pengguna-less sales cannot add; the CEO can
	if code, _ := post(t, srv.URL+"/api/branches", "andi@gsi.co.id", map[string]any{"name": "Medan"}); code != http.StatusForbidden {
		t.Fatalf("sales adds branch: %d", code)
	}
	if code, _ := post(t, srv.URL+"/api/branches", "sam@gsi.co.id", map[string]any{"name": "Medan", "city": "Medan"}); code != http.StatusCreated {
		t.Fatalf("add branch: %d", code)
	}
	if code, b := get(t, srv, "/api/branches", "andi@gsi.co.id"); code != 200 || len(b["items"].([]any)) < 5 {
		t.Fatalf("branches %d %v", code, b)
	}
}

func del(t *testing.T, url, user string) (int, string) {
	t.Helper()
	return send(t, http.MethodDelete, url, user, "", nil)
}
