package stockanalysis

import (
	"strings"
	"testing"

	"distri-arc/internal/domain"
)

var pol = domain.StockPolicy{AgingDays: 90, CriticalDays: 10}

func item(branch, sku, cat string, qty int, cost int64, age int, perWeek float64) domain.StockItem {
	return domain.StockItem{Branch: branch, SKU: sku, Name: "Barang " + sku, Category: cat, Qty: qty, UnitCost: cost, Value: int64(qty) * cost, AgeDays: age, WeeklyVelocity: perWeek}
}

func noPrice(string, string) int64 { return 0 }

// FSN: the fastest 20% of the moving products are fast; slow when the stock lasts beyond 90 days; dead without
// sales; critical when it runs out before 10 days, with a reorder covering 30 days; aging past 90 days.
func TestClasses(t *testing.T) {
	items := []domain.StockItem{
		item("Semarang", "F1", "Kamera & NVR", 10, 100, 10, 14), // 5 days cover: fast + critical
		item("Semarang", "N1", "Kamera & NVR", 20, 100, 20, 4),  // 35 days: normal
		item("Semarang", "N2", "Kabel & PoE", 30, 100, 20, 3),   // 70 days: normal
		item("Semarang", "N3", "Kabel & PoE", 20, 100, 20, 2),   // 70 days: normal
		item("Semarang", "S1", "Modul LED", 100, 100, 120, 1),   // 700 days: slow + aging
		item("Semarang", "D1", "Modul LED", 50, 200, 200, 0),    // dead + aging
		item("Semarang", "Z0", "Aksesoris", 0, 100, 0, 0),       // nothing: left out
	}
	a := Compute(items, "", noPrice, pol)
	got := map[string]Product{}
	for _, l := range [][]Product{a.FastList, a.SlowList, a.DeadList, a.Reorder} {
		for _, p := range l {
			got[p.SKU] = p
		}
	}
	if len(a.Products) != 6 || a.Products[0].SKU != "D1" || a.Products[1].SKU != "S1" || a.Products[5].SKU != "F1" {
		t.Fatalf("products by value: %+v", a.Products)
	}
	if a.SKUs != 6 || a.Value != 10*100+20*100+30*100+20*100+100*100+50*200 {
		t.Fatalf("skus %d value %d", a.SKUs, a.Value)
	}
	if f := got["F1"]; f.Class != Fast || !f.Critical || f.ReorderQty != 60-10 || *f.CoverDays != 5 {
		t.Fatalf("fast: %+v", f)
	}
	if s := got["S1"]; s.Class != Slow || !s.Aging {
		t.Fatalf("slow: %+v", s)
	}
	if d := got["D1"]; d.Class != Dead || !d.Aging || d.CoverDays != nil {
		t.Fatalf("dead: %+v", d)
	}
	if len(a.Reorder) != 1 || a.CriticalSKUs != 1 || a.AgingSKUs != 2 || a.AgingValue != 100*100+50*200 {
		t.Fatalf("reorder %d critical %d aging %d %d", len(a.Reorder), a.CriticalSKUs, a.AgingSKUs, a.AgingValue)
	}
	classes := map[string]Share{}
	for _, c := range a.Classes {
		classes[c.Key] = c
	}
	if classes["dead"].Value != 10000 || classes["normal"].SKUs != 3 || classes["fast"].SKUs != 1 {
		t.Fatalf("classes %+v", a.Classes)
	}
	// turnover: value / daily COGS = 28000 / ((14+4+3+2+1)*100/7)
	if a.Turnover == nil || *a.Turnover != 82 {
		t.Fatalf("turnover %v", a.Turnover)
	}
	if a.AgeBuckets[0].SKUs != 4 || a.AgeBuckets[3].SKUs != 1 || a.AgeBuckets[4].SKUs != 1 {
		t.Fatalf("buckets %+v", a.AgeBuckets)
	}
	text := ""
	for _, i := range a.Insights {
		text += i.Tone + ": " + i.Text + "\n"
	}
	for _, w := range []string{"Perputaran stok 82 hari, di atas target 40", "Rp 10.000 (36% nilai stok) tertahan di 1 barang dead stock", "1 barang akan habis dalam kurang dari 10 hari", "Kategori Modul LED paling lambat"} {
		if !strings.Contains(text, w) {
			t.Errorf("insight %q missing:\n%s", w, text)
		}
	}
}

// ABC on the sales value: A up to 80%, B to 95%, C the rest; a list price wins over the cost.
func TestABC(t *testing.T) {
	items := []domain.StockItem{
		item("Semarang", "A1", "x", 10, 100, 1, 80),
		item("Semarang", "B1", "x", 10, 100, 1, 15),
		item("Semarang", "C1", "x", 10, 100, 1, 5),
	}
	a := Compute(items, "", func(sku, _ string) int64 { return map[string]int64{"A1": 1000}[sku] }, pol)
	abc := map[string]string{}
	for _, l := range [][]Product{a.FastList} {
		for _, p := range l {
			abc[p.SKU] = p.ABC
		}
	}
	if a.ABC[0].SKUs != 1 || a.ABC[1].SKUs+a.ABC[2].SKUs != 2 {
		t.Fatalf("abc %+v", a.ABC)
	}
	if abc["A1"] != "A" {
		t.Fatalf("A1 %v", abc)
	}
}

// Mutasi: held without sales in Jakarta, sold and short in Semarang — move what covers 30 days there; one branch read
// alone has no transfers.
func TestTransfers(t *testing.T) {
	items := []domain.StockItem{
		item("Jakarta", "T1", "x", 40, 500, 100, 0),
		item("Semarang", "T1", "x", 2, 500, 5, 7), // 2 days: needs 30 - 2 = 28
	}
	a := Compute(items, "", noPrice, pol)
	if len(a.Transfers) != 1 || a.Transfers[0].From != "Jakarta" || a.Transfers[0].To != "Semarang" || a.Transfers[0].Qty != 28 || a.Transfers[0].Value != 28*500 {
		t.Fatalf("transfers %+v", a.Transfers)
	}
	if b := Compute(items, "Jakarta", noPrice, pol); len(b.Transfers) != 0 || b.SKUs != 1 || b.DeadList[0].SKU != "T1" {
		t.Fatalf("one branch: %+v", b)
	}
}

func TestRp(t *testing.T) {
	for in, want := range map[int64]string{12500: "Rp 12.500", 350_000_000: "Rp 350 jt", 1_250_000_000: "Rp 1,2 M"} {
		if got := Rp(in); got != want {
			t.Errorf("Rp(%d) = %q, want %q", in, got, want)
		}
	}
}
