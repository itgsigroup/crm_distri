package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/views"
)

// Imported data (Accurate) has no tier prices: the bundle uses the list price; an item without any price is skipped
// (never a NaN margin); one cycle proposes at most MaxPushPerCycle bundles, largest aging value first.
func TestStockBundlesFromImportedData(t *testing.T) {
	pol := domain.DefaultPolicies()
	due := 2
	room := 0.6
	m := domain.DealerMetrics{Rhythm: ptrInt(14), Cyc: 0.9, DueIn: &due, Status: domain.StatusAktif, Segment: domain.SegmentB, Credit: domain.Credit{State: domain.CreditAman, Room: &room}}
	m.MixCats[0] = true // Kamera & NVR
	d := &Dealer{BoardItem: views.BoardItem{ID: "toko", UUID: uuid.New(), Name: "Toko Satu", Metrics: m},
		Contacts: []domain.Contact{{Name: "Budi", IsPrimary: true}}, Signals: []Signal{{ID: uuid.New()}}}
	in := &Input{Today: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), Policies: pol, Dealers: []*Dealer{d}, Catalog: map[string]Product{}}
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("Kamera %02d", i)
		in.Stock = append(in.Stock, domain.StockItem{Name: name, Branch: "Semarang", Category: "Kamera & NVR", AgeDays: 120, Qty: 5, UnitCost: 100_000, Value: int64(1_000_000 + i)})
		in.Catalog[name] = Product{Name: name, List: 150_000}
	}
	in.Stock = append(in.Stock, domain.StockItem{Name: "Tanpa harga", Branch: "Semarang", Category: "Kamera & NVR", AgeDays: 200, Qty: 5, UnitCost: 100_000, Value: 9_000_000})
	in.Catalog["Tanpa harga"] = Product{Name: "Tanpa harga"}
	out, err := Stock{}.Analyze(context.Background(), in, nil)
	if err != nil {
		t.Fatal(err)
	}
	var push []domain.Proposal
	for _, p := range out {
		if p.Kind == domain.KindPushStock {
			push = append(push, p)
		}
	}
	if len(push) != MaxPushPerCycle {
		t.Fatalf("push proposals %d, want %d", len(push), MaxPushPerCycle)
	}
	if n := len(push[0].Payload["dealers"].([]map[string]any)); n != 1 {
		t.Fatalf("dealers %d", n)
	}
	if push[0].Payload["name"] != "Kamera 24" { // largest value first; the item without a price is skipped
		t.Fatalf("first push %v", push[0].Payload["name"])
	}
	for _, p := range push {
		if _, err := json.Marshal(p.Payload); err != nil || p.Payload["bundle_price"].(int64) <= 0 {
			t.Fatalf("payload %v: %v", p.Payload, err)
		}
	}
}

func ptrInt(v int) *int { return &v }

// A bundle offers the first MaxPushDealers matching dealers; the title says how many matched in total.
func TestStockBundleDealerCap(t *testing.T) {
	pol := domain.DefaultPolicies()
	room := 0.6
	in := &Input{Today: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), Policies: pol, Catalog: map[string]Product{"Kamera": {Name: "Kamera", List: 150_000}}}
	for i := 0; i < 35; i++ {
		due := i % 7
		m := domain.DealerMetrics{Rhythm: ptrInt(14), Cyc: 0.9, DueIn: &due, Status: domain.StatusAktif, Segment: domain.SegmentB, Credit: domain.Credit{State: domain.CreditAman, Room: &room}}
		m.MixCats[0] = true
		in.Dealers = append(in.Dealers, &Dealer{BoardItem: views.BoardItem{ID: fmt.Sprintf("d%02d", i), UUID: uuid.New(), Name: fmt.Sprintf("Toko %02d", i), Metrics: m},
			Contacts: []domain.Contact{{Name: "Budi", IsPrimary: true}}, Signals: []Signal{{ID: uuid.New()}}})
	}
	in.Stock = []domain.StockItem{{Name: "Kamera", Branch: "Semarang", Category: "Kamera & NVR", AgeDays: 120, Qty: 5, UnitCost: 100_000, Value: 500_000}}
	out, err := Stock{}.Analyze(context.Background(), in, nil)
	if err != nil || len(out) == 0 {
		t.Fatalf("%v %d", err, len(out))
	}
	if n := len(out[0].DealerIDs); n != MaxPushDealers || !strings.Contains(out[0].Title, "30 teratas dari 35") {
		t.Fatalf("dealers %d title %q", n, out[0].Title)
	}
}
