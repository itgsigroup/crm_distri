package api_test

import (
	"context"
	"testing"
)

// Analisa stok reads every branch's stock (or one), classes the products and writes the owner's insights.
func TestStockAnalysis(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `delete from stock_items`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty, unit_cost, value, age_days, weekly_velocity) values
		('Semarang', 'AN-FAST', 'Kamera Laris', 'Kamera & NVR', 10, 1000000, 10000000, 10, 14),
		('Semarang', 'AN-DEAD', 'LED Lama', 'Modul LED', 20, 500000, 10000000, 200, 0),
		('Jakarta', 'AN-FAST', 'Kamera Laris', 'Kamera & NVR', 30, 1000000, 30000000, 40, 0)`); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if code := getJSON(t, srv, "/api/stock/analysis", "sam@gsi.co.id", &out); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if out["skus"] != float64(2) || out["value"] != float64(50000000) || len(out["insights"].([]any)) == 0 {
		t.Fatalf("totals %v %v", out["skus"], out["value"])
	}
	if dead := out["dead"].([]any); len(dead) != 1 || dead[0].(map[string]any)["sku"] != "AN-DEAD" {
		t.Fatalf("dead %v", dead)
	}
	if tr := out["transfers"].([]any); len(tr) != 1 || tr[0].(map[string]any)["from"] != "Jakarta" {
		t.Fatalf("transfers %v", tr)
	}
	if out["data"] == nil {
		t.Fatal("no provenance")
	}
	if code := getJSON(t, srv, "/api/stock/analysis?branch=Jakarta", "sam@gsi.co.id", &out); code != 200 || out["skus"] != float64(1) {
		t.Fatalf("one branch: %d %v", code, out["skus"])
	}
}
