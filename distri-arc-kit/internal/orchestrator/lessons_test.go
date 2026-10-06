package orchestrator

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
)

func TestLessonsAggregate(t *testing.T) {
	rej := func(dealer, tier, reason string) Rejection {
		return Rejection{Agent: "AI Stok", Kind: domain.KindPushStock, Reason: reason, Dealer: dealer, Tier: tier, Product: "HDD 4TB surveillance", At: today}
	}
	rs := []Rejection{
		rej("Toko A", "C", "Tidak sesuai kebijakan — mereka beli di marketplace"),
		rej("Toko B", "C", "Tidak sesuai kebijakan — mereka beli di marketplace"),
		rej("Toko C", "C", "Tidak sesuai kebijakan"),
		rej("Toko D", "C", "Tidak tepat waktu"), // another reason: no lesson
	}
	ls := Lessons(rs[:2], today)
	if len(ls) != 0 {
		t.Fatalf("lesson from 2 rejections: %+v", ls)
	}
	ls = Lessons(rs, today)
	if len(ls) != 1 {
		t.Fatalf("%d lessons", len(ls))
	}
	l := ls[0]
	if l.Text != "AI Stok tidak lagi menawarkan HDD ke dealer tier C (ditolak 3×: “mereka beli di marketplace”)." || l.Tier != "C" {
		t.Fatalf("lesson %q tier %q", l.Text, l.Tier)
	}
	if !l.Until.Equal(today.AddDate(0, 0, 14)) {
		t.Fatalf("until %v", l.Until)
	}
	one := Lessons([]Rejection{rej("Toko A", "C", "Salah dealer"), rej("Toko A", "C", "Salah dealer"), rej("Toko A", "C", "Salah dealer")}, today)
	if one[0].Scope != "Toko A" || !strings.Contains(one[0].Text, "ke Toko A") {
		t.Fatalf("single-dealer scope: %+v", one[0])
	}
}

// A lesson takes tier C dealers out of a bundle (and suppresses it when nobody is left).
func TestLessonSuppressesBundle(t *testing.T) {
	c := dealer("lampu", domain.StatusAktif, domain.CreditCash, 2)
	c.Tier = "C"
	a := dealer("borneo", domain.StatusKeyAccount, domain.CreditAman, 1)
	push := &Cand{P: domain.Proposal{Agent: "AI Stok", Kind: domain.KindPushStock, Title: "Bundle HDD", Confidence: 0.8, SignalIDs: []uuid.UUID{uuid.New()},
		DealerIDs: []uuid.UUID{c.UUID, a.UUID}, Payload: map[string]any{"name": "HDD 4TB surveillance", "dealers": []map[string]any{{"id": c.UUID}, {"id": a.UUID}}}, DedupeKey: "push:hdd"}}
	in := ruleInput(c, a)
	in.Lessons = []Lesson{{Agent: "AI Stok", Kind: domain.KindPushStock, Product: "HDD", Tier: "C", Text: "AI Stok tidak lagi menawarkan HDD ke dealer tier C"}}
	_, cs := Synthesize([]*Cand{push}, in)
	if len(push.P.DealerIDs) != 1 || push.P.DealerIDs[0] != a.UUID || rules(cs, RuleSuppression) != 1 {
		t.Fatalf("dealers %v conflicts %d", push.P.DealerIDs, rules(cs, RuleSuppression))
	}
	led := &Cand{P: domain.Proposal{Agent: "AI Stok", Kind: domain.KindPushStock, Title: "Bundle LED", Confidence: 0.8, SignalIDs: []uuid.UUID{uuid.New()},
		DealerIDs: []uuid.UUID{c.UUID}, Payload: map[string]any{"name": "Modul LED P5 outdoor"}, DedupeKey: "push:led"}}
	Synthesize([]*Cand{led}, in)
	if len(led.P.DealerIDs) != 1 || led.Suppressed != "" {
		t.Fatal("lesson about HDD touched an LED bundle")
	}
}
