package ask_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/ask"
	"distri-arc/internal/clock"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// A stock question is answered as asked (Sam 22 Oct): a product → its stock per branch; fast / dead / slow moving,
// reorder, transfers, age, a branch → that answer — none names dealers; only push / bundle lists candidate dealers.
func TestAskStockAsAsked(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	s := &ask.Service{St: st, Clock: clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB))}
	askq := func(q string) ask.Answer {
		a, err := s.Ask(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s → [%s] %s", q, a.Intent, a.Text)
		return a
	}
	cases := map[string]string{
		"stok kamera ip 4mp dome berapa?": "CAM-IP4-DOME",
		"barang apa yang fast moving":     "fast moving",
		"dead stock apa saja":             "ead stock",
		"barang yang perlu reorder":       "reorder",
		"stok slow moving":                "low moving",
		"umur stok":                       "Umur stok",
		"stok di jakarta":                 "1.053 unit",
		"stok":                            "Perputaran",
		"stok xyzabc ada?":                "tidak ditemukan",
	}
	for q, want := range cases {
		a := askq(q)
		if a.Intent != ask.IntentStock || !strings.Contains(a.Text, want) {
			t.Errorf("%q: intent %s, want %q in %q", q, a.Intent, want, a.Text)
		}
		if strings.Contains(a.Text, "[[dealer:") || a.Dealer != "" {
			t.Errorf("%q names dealers: %q", q, a.Text)
		}
	}
	if a := askq("stok apa yang harus didorong"); !strings.Contains(a.Text, "[[dealer:") {
		t.Errorf("push lists the candidate dealers: %q", a.Text)
	}
}
