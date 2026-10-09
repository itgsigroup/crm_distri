package api_test

import (
	"context"
	"net/http"
	"testing"
)

// Mapping sales: two spellings of one person from the source data merge into one GSI Orbit sales — the dealers
// move to it and the spellings disappear from the sales list.
func TestSalesMergeMovesDealersAndHidesSpellings(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var a, b, dealer string
	_ = st.Pool.QueryRow(ctx, "insert into sales_users (name, branch, role, source_system, source_id) values ('Granike Monica', 'Semua cabang', 'sales', 'import', 'GM1') returning id").Scan(&a)
	_ = st.Pool.QueryRow(ctx, "insert into sales_users (name, branch, role, source_system, source_id) values ('Granike Monica M.', 'Semarang', 'sales', 'import', 'GM2') returning id").Scan(&b)
	_ = st.Pool.QueryRow(ctx, "update dealers set owner_id = $1 where id = (select id from dealers order by name limit 1) returning id", b).Scan(&dealer)
	code, out := post(t, srv.URL+"/api/sales-map/profiles", "sam@gsi.co.id", map[string]any{"name": "Granike Monika", "branch": "Semarang"})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	target := out["id"].(string)
	if code, _ := post(t, srv.URL+"/api/sales-map/merge", "andi@gsi.co.id", map[string]any{"sources": []string{a, b}, "target_id": target}); code != http.StatusForbidden {
		t.Fatalf("a sales merges: %d", code)
	}
	if code, out := post(t, srv.URL+"/api/sales-map/merge", "sam@gsi.co.id", map[string]any{"sources": []string{a, b}, "target_id": target}); code != 200 || out["merged"] != float64(2) {
		t.Fatalf("merge: %d %v", code, out)
	}
	var owner string
	_ = st.Pool.QueryRow(ctx, "select owner_id::text from dealers where id = $1", dealer).Scan(&owner)
	if owner != target {
		t.Fatalf("dealer still owned by %s", owner)
	}
	var mapped string
	_ = st.Pool.QueryRow(ctx, "select coalesce(target, '') from data_mappings where kind = 'sales' and source_value = 'GM2'").Scan(&mapped)
	if mapped != target {
		t.Fatalf("import mapping: %q", mapped)
	}
	_, sales := get(t, srv, "/api/sales", "sam@gsi.co.id")
	for _, x := range sales["items"].([]any) {
		if n := x.(map[string]any)["name"]; n == "Granike Monica" || n == "Granike Monica M." {
			t.Fatalf("merged spelling still listed: %v", n)
		}
	}
}
