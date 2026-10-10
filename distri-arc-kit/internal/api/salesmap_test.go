package api_test

import (
	"context"
	"net/http"
	"testing"
)

// Mapping sales: one Pengguna ↔ many BigQuery sales names. Linking two spellings to a user moves their dealers to
// the user's main sales profile (made on first link), points the import mapping there and hides the spellings from
// the sales list; another user's main profile is never taken.
func TestSalesMapLinksManyNamesToOneUser(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var a, b, dealer, user, andiProfile string
	_ = st.Pool.QueryRow(ctx, "insert into sales_users (name, branch, role, source_system, source_id) values ('Granike Monica', 'Semua cabang', 'sales', 'import', 'GM1') returning id").Scan(&a)
	_ = st.Pool.QueryRow(ctx, "insert into sales_users (name, branch, role, source_system, source_id) values ('Granike Monica M.', 'Semarang', 'sales', 'import', 'GM2') returning id").Scan(&b)
	_ = st.Pool.QueryRow(ctx, "update dealers set owner_id = $1 where id = (select id from dealers order by name limit 1) returning id", b).Scan(&dealer)
	_ = st.Pool.QueryRow(ctx, "insert into users (email, name, role, role_key, active) values ('granike@gsi.co.id', 'Granike Monika', 'sales', 'sales', true) returning id").Scan(&user)
	_ = st.Pool.QueryRow(ctx, "select sales_user_id::text from users where email = 'andi@gsi.co.id'").Scan(&andiProfile)

	if code, _ := post(t, srv.URL+"/api/sales-map/merge", "andi@gsi.co.id", map[string]any{"sources": []string{a, b}, "user_id": user}); code != http.StatusForbidden {
		t.Fatalf("a sales links names: %d", code)
	}
	code, out := post(t, srv.URL+"/api/sales-map/merge", "sam@gsi.co.id", map[string]any{"sources": []string{a, b}, "user_id": user})
	if code != 200 || out["merged"] != float64(2) {
		t.Fatalf("link: %d %v", code, out)
	}
	var main, owner, mapped string
	_ = st.Pool.QueryRow(ctx, "select coalesce(sales_user_id::text, '') from users where id = $1", user).Scan(&main)
	if main == "" || main != out["target_id"] {
		t.Fatalf("user main profile: %q vs %v", main, out["target_id"])
	}
	_ = st.Pool.QueryRow(ctx, "select owner_id::text from dealers where id = $1", dealer).Scan(&owner)
	if owner != main {
		t.Fatalf("dealer owned by %s, want the user's profile %s", owner, main)
	}
	_ = st.Pool.QueryRow(ctx, "select coalesce(target, '') from data_mappings where kind = 'sales' and source_value = 'GM2'").Scan(&mapped)
	if mapped != main {
		t.Fatalf("import mapping: %q", mapped)
	}
	_, sales := get(t, srv, "/api/sales", "sam@gsi.co.id")
	for _, x := range sales["items"].([]any) {
		if n := x.(map[string]any)["name"]; n == "Granike Monica" || n == "Granike Monica M." {
			t.Fatalf("linked spelling still listed: %v", n)
		}
	}
	_, m := get(t, srv, "/api/sales-map", "sam@gsi.co.id")
	for _, x := range m["items"].([]any) {
		if it := x.(map[string]any); it["id"] == a && it["user_id"] != user {
			t.Fatalf("GM1 not shown under the user: %v", it)
		}
	}
	// Andi's own profile is not taken by another user
	if code, _ := post(t, srv.URL+"/api/sales-map/merge", "sam@gsi.co.id", map[string]any{"sources": []string{andiProfile}, "user_id": user}); code != http.StatusBadRequest {
		t.Fatalf("took another user's main profile: %d", code)
	}
}
