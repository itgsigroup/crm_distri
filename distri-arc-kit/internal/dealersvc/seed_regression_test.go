package dealersvc_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// expected holds what the approved mockup's own formulas show for each sample dealer
// (tools/seedgen/mockup.json, produced by extract_mockup.mjs from reference/distri-arc-orbit-v2-mockup.html).
type expected struct {
	ID      string `json:"id"`
	DueIn   *int   `json:"due_in"`
	Status  string `json:"status"`
	Segment string `json:"segment"`
	Credit  struct {
		S string `json:"s"`
	} `json:"credit"`
	Score struct {
		G     int      `json:"g"`
		Parts [][2]any `json:"parts"`
	} `json:"score"`
}

var segmentKey = map[string]string{"jangkar": "A", "nadi": "B", "gelombang": "C", "riak": "D", "baru": "Baru"}

// TestSeedReproducesMockup is the seed regression test (docs/design/10-testing-ops.md): after seed + recompute,
// the metrics of all 18 dealers equal the mockup with zero tolerance.
func TestSeedReproducesMockup(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	svc := dealersvc.New(st, clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)))
	ms, err := svc.Recompute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../tools/seedgen/mockup.json")
	if err != nil {
		t.Fatal(err)
	}
	var mk struct {
		Expected []expected `json:"expected"`
	}
	if err := json.Unmarshal(raw, &mk); err != nil {
		t.Fatal(err)
	}
	ds, err := svc.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]domain.DealerMetrics{}
	for _, d := range ds.Dealers {
		bySlug[*d.Slug] = ms[d.ID]
	}
	counts := map[string]int{}
	for _, e := range mk.Expected {
		m, ok := bySlug[e.ID]
		if !ok {
			t.Fatalf("dealer %s missing", e.ID)
		}
		status := m.Status
		if status == domain.StatusBaru {
			status = domain.StatusAktif // the mockup draws Baru on the Aktif ring
		}
		counts[status]++
		if status != e.Status {
			t.Errorf("%s status %s want %s", e.ID, m.Status, e.Status)
		}
		if m.Segment != segmentKey[e.Segment] {
			t.Errorf("%s segment %s want %s", e.ID, m.Segment, segmentKey[e.Segment])
		}
		want := e.Credit.S
		if m.Credit.State != want {
			t.Errorf("%s credit %s want %s", e.ID, m.Credit.State, want)
		}
		if m.Score != e.Score.G {
			t.Errorf("%s score %d want %d", e.ID, m.Score, e.Score.G)
		}
		parts := []int{m.ScoreParts.Rhythm, m.ScoreParts.SOW, m.ScoreParts.Mix, m.ScoreParts.Credit, m.ScoreParts.Contact}
		for i, p := range e.Score.Parts {
			if int(p[1].(float64)) != parts[i] {
				t.Errorf("%s score part %v = %d want %v", e.ID, p[0], parts[i], p[1])
			}
		}
		if (e.DueIn == nil) != (m.DueIn == nil) || (e.DueIn != nil && *e.DueIn != *m.DueIn) {
			t.Errorf("%s due_in %v want %v", e.ID, m.DueIn, e.DueIn)
		}
	}
	// docs/stages/01: 8 Key account · 6 Aktif · 3 At risk · 1 Churn
	if counts[domain.StatusKeyAccount] != 8 || counts[domain.StatusAktif] != 6 || counts[domain.StatusAtRisk] != 3 || counts[domain.StatusChurn] != 1 {
		t.Errorf("status counts %v", counts)
	}
	mitra := bySlug["mitra"]
	if mitra.Status != domain.StatusAtRisk || mitra.Segment != domain.SegmentC || mitra.Credit.State != domain.CreditOverLimit || mitra.Score != 44 {
		t.Errorf("mitra %+v", mitra)
	}
}
