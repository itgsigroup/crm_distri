package importer

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
)

// Cek stok (27 Oct, "stok di cek masih tidak sesuai dengan database"): GSI Orbit's stock is the source's, row for
// row — two rows of one SKU × warehouse add up, a product without a name keeps its stock, an older upload of another
// source no longer adds to it, the table and the product's check show the same totals, and a failed Apply still
// rebuilds the stock.
func TestStockMatchesSource(t *testing.T) {
	st := testdb.New(t)
	im := Importer{St: st, Clock: clock.Fixed(time.Date(2026, 10, 27, 9, 0, 0, 0, clock.WIB)), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := context.Background()
	stage := func(source, csv string) {
		t.Helper()
		rows, err := ParseCSV(strings.NewReader(csv))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := im.Stage(ctx, Stock, rows, source, "test"); err != nil {
			t.Fatal(err)
		}
	}
	semarang := "Semarang"
	if err := st.Q.SetMapping(ctx, gen.SetMappingParams{Kind: "warehouse", SourceValue: "Gudang Lamper", Target: &semarang}); err != nil {
		t.Fatal(err)
	}
	stage("csv", "sku,product,warehouse,qty\nX1,Barang lama,Semarang,99\n")
	stage("bigquery", "sku,product,warehouse,qty,unit_cost,sold_90d\n"+
		"A1,Kamera A,Gudang Lamper,10,100000,4\n"+
		"A1,Kamera A,Gudang Lamper,5.00,120000,2\n"+
		"A1,,Jakarta,3,100000,\n"+
		"B1,Kabel,Semarang,7,5000,13\n")
	if _, err := im.Apply(ctx, true, "test"); err != nil {
		t.Fatal(err)
	}
	qty := func(sku, branch string) int {
		var n int
		_ = st.Pool.QueryRow(ctx, "select coalesce(sum(qty), 0) from stock_items where sku = $1 and branch = $2", sku, branch).Scan(&n)
		return n
	}
	if a, j, b, x := qty("A1", "Semarang"), qty("A1", "Jakarta"), qty("B1", "Semarang"), qty("X1", "Semarang"); a != 15 || j != 3 || b != 7 || x != 0 {
		t.Fatalf("stock A1 Semarang %d, A1 Jakarta %d, B1 %d, X1 (old csv) %d", a, j, b, x)
	}
	src, err := im.StockSource(ctx, "a1")
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, x := range src {
		total += x.Qty
	}
	if total != 18 {
		t.Fatalf("source rows of A1 add up to %d", total)
	}

	// the table: one row per SKU (no case), branches summed as /stock/check sums them
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty, source_system, source_id)
		values ('Semarang', 'a1', 'Kamera A', 'CCTV', 2, 'manual', 'a1')`); err != nil {
		t.Fatal(err)
	}
	rows, err := st.Q.ListStockProducts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if !strings.EqualFold(r.Sku, "A1") {
			continue
		}
		if found {
			t.Fatal("A1 listed twice")
		}
		found = true
		by := map[string]int64{}
		_ = json.Unmarshal(r.ByBranch, &by)
		if r.Qty != 20 || by["Semarang"] != 17 || by["Jakarta"] != 3 {
			t.Fatalf("table A1 qty %d by branch %v", r.Qty, by)
		}
	}
	if !found {
		t.Fatal("A1 not listed")
	}
	if _, err := st.Pool.Exec(ctx, "delete from stock_items where source_system = 'manual'"); err != nil {
		t.Fatal(err)
	}

	// the stock rebuilt alone (an Apply that failed before its stock step): new quantities, velocity kept
	stage("bigquery", "sku,product,warehouse,qty\nA1,Kamera A,Gudang Lamper,9\nB1,Kabel,Semarang,1\n")
	var rep ApplyReport
	if err := im.applyStockAlone(ctx, &rep); err != nil {
		t.Fatal(err)
	}
	if a, j, b := qty("A1", "Semarang"), qty("A1", "Jakarta"), qty("B1", "Semarang"); a != 9 || j != 0 || b != 1 || !rep.stockDone {
		t.Fatalf("after stock alone: A1 Semarang %d, A1 Jakarta %d, B1 %d", a, j, b)
	}
	var vel float64
	_ = st.Pool.QueryRow(ctx, "select weekly_velocity from stock_items where sku = 'B1'").Scan(&vel)
	if vel != 1 {
		t.Fatalf("B1 velocity %v, want the 1 per week it had", vel)
	}
}
