package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Cek stok (21 Oct): one product as GSI Orbit reads it — per branch, the source rows per warehouse mapped to their
// branch, the "Tanya stok" answer for a conversation branch (a sales covering every branch counts every branch) — and
// real counts stored against what GSI Orbit read at that moment.
func TestStockCheck(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	for _, q := range []string{
		`insert into stock_items (branch, sku, name, category, qty) values
		   ('Surabaya', 'X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 'Kamera & NVR', 43), ('Jakarta', 'X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 'Kamera & NVR', 24)`,
		`insert into import_rows (entity, key, data, source) values
		   ('stock', 'X5S_8W|gudang sby', '{"sku":"X5S_8W","product":"Ezviz NVR 8Ch X5S 8W","warehouse":"Gudang SBY","qty":"43"}', 'bigquery'),
		   ('stock', 'X5S_8W|soetta', '{"sku":"X5S_8W","product":"Ezviz NVR 8Ch X5S 8W","warehouse":"Soetta","qty":"24"}', 'bigquery')`,
		`insert into data_mappings (kind, source_value, target) values ('warehouse', 'Gudang SBY', 'Surabaya')`,
		`insert into products (sku, name, list_price, prices) values ('X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 865000, '{"A":865000,"B":900000}')`,
	} {
		if _, err := st.Pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	code, out := get(t, srv, "/api/stock/check?sku=x5s_8w&branch=Semua+cabang", "sam@gsi.co.id")
	if code != http.StatusOK || out["product"] != "Ezviz NVR 8Ch X5S 8W" {
		t.Fatalf("check %d %v", code, out)
	}
	if pr := out["price"].(map[string]any); pr["list"] != float64(865000) || pr["tiers"].(map[string]any)["B"] != float64(900000) {
		t.Fatalf("price: %v", out["price"])
	}
	a := out["answer"].(map[string]any)
	if a["ready"] != true || a["qty"] != float64(67) || a["headline"] != "Ready · 67 di semua cabang" {
		t.Fatalf("answer for a sales covering every branch: %v", a)
	}
	rows := out["source_rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("source rows: %v", rows)
	}
	for _, x := range rows {
		r := x.(map[string]any)
		if r["warehouse"] == "Gudang SBY" && (r["branch"] != "Surabaya" || r["mapped"] != true) ||
			r["warehouse"] == "Soetta" && (r["branch"] != "Soetta" || r["mapped"] != false) {
			t.Fatalf("mapping: %v", r)
		}
	}
	got := map[string][2]float64{}
	for _, x := range out["branches"].([]any) {
		b := x.(map[string]any)
		got[b["branch"].(string)] = [2]float64{b["arc_qty"].(float64), b["source_qty"].(float64)}
	}
	// a warehouse not mapped to a branch shows as its own row, so the difference to Jakarta is visible
	if got["Surabaya"] != [2]float64{43, 43} || got["Jakarta"] != [2]float64{24, 0} || got["Soetta"] != [2]float64{0, 24} {
		t.Fatalf("branches: %v", got)
	}
	if code, out := get(t, srv, "/api/stock/check?sku=Jakarta", "sam@gsi.co.id"); code != http.StatusNotFound {
		t.Fatalf("unknown sku %d %v", code, out)
	}

	code, out = post(t, srv.URL+"/api/stock/checks", "sam@gsi.co.id", map[string]any{"sku": "X5S_8W", "note": "Accurate 21 Okt",
		"counts": []map[string]any{{"branch": "Surabaya", "real_qty": 43}, {"branch": "Jakarta", "real_qty": 0}, {"branch": "Soetta", "real_qty": nil}}})
	if code != http.StatusOK || out["saved"] != float64(2) {
		t.Fatalf("save %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/stock/checks", "sam@gsi.co.id", map[string]any{"sku": "X5S_8W", "counts": []map[string]any{}}); code != http.StatusBadRequest {
		t.Fatalf("nothing counted: %d", code)
	}
	_, out = get(t, srv, "/api/stock/check?sku=X5S_8W", "sam@gsi.co.id")
	for _, x := range out["branches"].([]any) {
		b := x.(map[string]any)
		if b["branch"] == "Jakarta" {
			c := b["last_check"].(map[string]any)
			if c["arc_qty"] != float64(24) || c["real_qty"] != float64(0) || c["note"] != "Accurate 21 Okt" || c["checked_by_name"] == "" {
				t.Fatalf("last check: %v", c)
			}
		}
	}
	var list []map[string]any
	if code := getJSON(t, srv, "/api/stock/products", "sam@gsi.co.id", &list); code != http.StatusOK {
		t.Fatalf("products %d", code)
	}
	found := false
	for _, p := range list {
		if p["sku"] == "X5S_8W" {
			found = true
			by, last := p["by_branch"].(map[string]any), p["last_check"].(map[string]any)
			if p["qty"] != float64(67) || p["price"] != float64(865000) || by["Jakarta"] != float64(24) || last["branches"] != float64(2) || last["off"] != float64(1) {
				t.Fatalf("product row: %v", p)
			}
		}
	}
	if !found {
		t.Fatal("X5S_8W not listed")
	}
}

// getJSON decodes any JSON body (a list) into out.
func getJSON(t *testing.T, srv *httptest.Server, path, user string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req.Header.Set("X-Dev-User", user)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	_ = json.NewDecoder(res.Body).Decode(out)
	return res.StatusCode
}
