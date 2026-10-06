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
	"distri-arc/internal/views"
)

func board() []views.BoardItem {
	return []views.BoardItem{{ID: "mitra", Name: "CV Mitra Jaya Teknik"}, {ID: "indo", Name: "Indo Vision Security"}}
}

// Router: five kinds of questions.
func TestClassify(t *testing.T) {
	cases := map[string]string{
		"dealer mana yang berisiko":    ask.IntentRisk,
		"stok apa yang harus didorong": ask.IntentStock,
		"berapa prediksi kas masuk":    ask.IntentCash,
		"siapa yang order minggu ini":  ask.IntentSchedule,
		"Mitra Jaya":                   ask.IntentDealer,
		"siapa yang lewat jadwal":      ask.IntentDrift,
		"apa kabar hari ini":           ask.IntentGeneral,
	}
	for q, want := range cases {
		got, d := ask.Classify(q, board())
		if got != want {
			t.Errorf("%q → %s, want %s", q, got, want)
		}
		if want == ask.IntentDealer && (d == nil || d.ID != "mitra") {
			t.Errorf("%q did not resolve the dealer", q)
		}
	}
}

// docs/stages/10 acceptance: "dealer mana yang berisiko" → three dealers with sources.
func TestAskRiskOnSeed(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	s := &ask.Service{St: st, Clock: clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB))}
	a, err := s.Ask(ctx, "dealer mana yang berisiko")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.Text, "Tiga dealer:") || strings.Count(a.Text, "[[dealer:") != 3 {
		t.Fatalf("answer %q", a.Text)
	}
	for _, slug := range []string{"mitra", "graha", "lampu"} {
		if !strings.Contains(a.Text, "[[dealer:"+slug+"|") {
			t.Errorf("%s missing: %s", slug, a.Text)
		}
	}
	if len(a.Sources) != 3 {
		t.Fatalf("%d sources", len(a.Sources))
	}
	if !ask.KeepsFacts(a.Text, a.Text) || ask.KeepsFacts(a.Text, "Tiga dealer berisiko.") {
		t.Fatal("KeepsFacts")
	}
	t.Log(a.Text)
}
