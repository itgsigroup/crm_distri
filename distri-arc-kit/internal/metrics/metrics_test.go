package metrics

import (
	"math"
	"testing"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
)

var today = time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB)

func ip(v int) *int { return &v }

func ago(days int) time.Time { return today.AddDate(0, 0, -days) }

// orders builds a confirmed order every rhythm days, the latest `last` days ago, each worth avg.
func orders(rhythm, last int, avg int64, cats ...string) []domain.Order {
	if len(cats) == 0 {
		cats = []string{"Kamera & NVR"}
	}
	var out []domain.Order
	for x := last; x <= 182; x += rhythm {
		t := ago(x)
		var lines []domain.OrderLine
		for _, c := range cats {
			lines = append(lines, domain.OrderLine{Product: c, Category: c, Qty: 1, Subtotal: avg / int64(len(cats))})
		}
		out = append(out, domain.Order{State: "bayar", ConfirmedAt: &t, Total: avg, Lines: lines})
		if rhythm == 0 {
			break
		}
	}
	return out
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.006 }

// Glossary table (docs/design/01-glossary.md › Tabel kasus uji): every row is a case here.
func TestGlossaryOrbitCases(t *testing.T) {
	p := domain.DefaultPolicies()
	cases := []struct {
		name               string
		rhythm, last       int
		sow, onTime        int
		wantCyc            float64
		wantStatus, wantAc string
		wantDue            *int
	}{
		{"Key account", 14, 6, 72, 93, 0.43, domain.StatusKeyAccount, domain.ActivityNormal, ip(8)},
		{"At risk", 21, 30, 40, 58, 1.43, domain.StatusAtRisk, domain.ActivityMenurun, ip(-9)},
		{"Churn", 28, 70, 15, 100, 2.5, domain.StatusChurn, domain.ActivityBerhenti, ip(-42)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rh := Rhythm(confirmedDates(orders(c.rhythm, c.last, 10e6)), today)
			if rh == nil || *rh != c.rhythm {
				t.Fatalf("rhythm = %v, want %d", rh, c.rhythm)
			}
			cyc := Cyc(rh, c.last)
			if !approx(cyc, c.wantCyc) {
				t.Errorf("cyc = %.3f, want %.2f", cyc, c.wantCyc)
			}
			if s := Status(rh, cyc, c.sow, c.onTime, p); s != c.wantStatus {
				t.Errorf("status = %s, want %s", s, c.wantStatus)
			}
			if a := Activity(rh, cyc); a != c.wantAc {
				t.Errorf("activity = %s, want %s", a, c.wantAc)
			}
			if due := *rh - c.last; due != *c.wantDue {
				t.Errorf("due_in = %d, want %d", due, *c.wantDue)
			}
		})
	}
	// Lewat 9 hari for the At-risk row.
	if lewat := 30 - 21; lewat != 9 {
		t.Fatal("lewat jadwal arithmetic")
	}
}

func TestGlossaryBaru(t *testing.T) {
	p := domain.DefaultPolicies()
	h := domain.DealerHistory{Orders: orders(0, 9, 18e6)}
	m := Compute(h, p, today)
	if m.Rhythm != nil || m.Status != domain.StatusBaru || m.Activity != domain.ActivityBaru || m.Segment != domain.SegmentBaru {
		t.Fatalf("baru: rhythm=%v status=%s activity=%s segment=%s", m.Rhythm, m.Status, m.Activity, m.Segment)
	}
	if m.OmzetBln != 18e6 {
		t.Errorf("omzet dealer baru = nilai order pertama, got %d", m.OmzetBln)
	}
}

func TestGlossarySegments(t *testing.T) {
	p := domain.DefaultPolicies()
	cases := []struct {
		rhythm   int
		avg      int64
		wantFreq float64
		want     string
	}{
		{14, 62e6, 2.14, domain.SegmentA},
		{12, 7e6, 2.5, domain.SegmentB},
		{21, 38e6, 1.43, domain.SegmentC},
		{35, 12e6, 0.86, domain.SegmentD},
	}
	for _, c := range cases {
		f := Freq(ip(c.rhythm))
		if !approx(*f, c.wantFreq) {
			t.Errorf("rhythm %d: freq %.3f want %.2f", c.rhythm, *f, c.wantFreq)
		}
		if s := Segment(f, c.avg, p); s != c.want {
			t.Errorf("rhythm %d avg %d: segment %s want %s", c.rhythm, c.avg, s, c.want)
		}
	}
}

func inv(issuedAgo, terms int, total, paid int64, paidAgo *int) domain.Invoice {
	i := domain.Invoice{IssuedAt: ago(issuedAgo), DueAt: ago(issuedAgo - terms), Total: total, Paid: paid, State: "posted"}
	if paidAgo != nil {
		t := ago(*paidAgo)
		i.PaidAt = &t
	}
	return i
}

// paidHistory returns invoices paid `pay` days after issue (one every 20 days, 120–180 days ago).
func paidHistory(pay int) []domain.Invoice {
	var out []domain.Invoice
	for x := 120; x <= 180; x += 20 {
		out = append(out, inv(x, 30, 10e6, 10e6, ip(x-pay)))
	}
	return out
}

func TestGlossaryCredit(t *testing.T) {
	p := domain.DefaultPolicies()
	cases := []struct {
		name     string
		limit    int64
		invoices []domain.Invoice
		room     float64
		want     string
	}{
		{"tipis", 250e6, append(paidHistory(28), inv(10, 30, 176e6, 0, nil)), 0.296, domain.CreditTipis},
		{"aman", 150e6, append(paidHistory(26), inv(10, 30, 40e6, 0, nil)), 0.733, domain.CreditAman},
		{"over limit", 150e6, append(paidHistory(26), inv(10, 30, 162e6, 0, nil)), -0.08, domain.CreditOverLimit},
		{"overdue", 250e6, append(paidHistory(26), inv(10, 30, 180e6, 0, nil), inv(45, 30, 40e6, 0, nil)), 0.12, domain.CreditOverdue},
		{"cash", 0, nil, 0, domain.CreditCash},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cr := CreditOf(CreditInput{Limit: c.limit, Invoices: c.invoices}, today, p)
			if cr.State != c.want {
				t.Fatalf("state %s want %s (credit %+v)", cr.State, c.want, cr)
			}
			if c.limit > 0 && !approx(*cr.Room, c.room) {
				t.Errorf("room %.3f want %.3f", *cr.Room, c.room)
			}
			if c.limit == 0 && cr.Room != nil {
				t.Error("cash dealer must have no room")
			}
		})
	}
}

func TestCreditPayAndOnTime(t *testing.T) {
	p := domain.DefaultPolicies()
	invs := []domain.Invoice{
		inv(100, 30, 1e6, 1e6, ip(72)), // 28 days: on time
		inv(80, 30, 1e6, 1e6, ip(40)),  // 40 days: late
		inv(60, 30, 1e6, 1e6, ip(34)),  // 26 days
		inv(40, 30, 1e6, 0, nil),       // open, overdue → counts late
		inv(5, 30, 1e6, 0, nil),        // open, not due → excluded
	}
	cr := CreditOf(CreditInput{Limit: 100e6, Invoices: invs}, today, p)
	if cr.PayDays != 31 { // (28+40+26)/3
		t.Errorf("pay days %d want 31", cr.PayDays)
	}
	if cr.OnTime != 50 { // 2 on time of 4
		t.Errorf("on time %d want 50", cr.OnTime)
	}
	if !cr.Late || cr.LateDays != 10 || cr.State != domain.CreditOverdue {
		t.Errorf("late %v %d state %s", cr.Late, cr.LateDays, cr.State)
	}
	if cr.PayDays <= p.Credit.PayMaxDays {
		// a dealer with room but slow pay is tipis
		slow := CreditOf(CreditInput{Limit: 100e6, Invoices: paidHistory(40)}, today, p)
		if slow.State != domain.CreditTipis {
			t.Errorf("pola bayar 40 > 35 should be tipis, got %s", slow.State)
		}
	}
}

func TestGlossaryScore(t *testing.T) {
	room := 1 - 162.0/150
	s, parts := Score(ScoreInput{Rhythm: ip(21), Cyc: 30.0 / 21, SOW: 40, Mix: 3, Limit: 150e6, Room: &room, OnTime: 58, PICCount: 2})
	want := domain.ScoreParts{Rhythm: 49, SOW: 40, Mix: 50, Credit: 0, Contact: 80}
	if parts != want || s != 44 {
		t.Fatalf("score %d parts %+v, want 44 %+v", s, parts, want)
	}
}

func TestGlossaryPush(t *testing.T) {
	p := domain.DefaultPolicies()
	led := domain.StockItem{Name: "Modul LED P5", Category: "Modul LED", AgeDays: 148}
	if !IsAging(led, p) {
		t.Fatal("148 hari should be aging")
	}
	due1 := 1
	room := 0.6
	fit := domain.DealerMetrics{Rhythm: ip(14), Cyc: 13.0 / 14, DueIn: &due1, Status: domain.StatusAktif, Segment: domain.SegmentB, Credit: domain.Credit{State: domain.CreditAman, Room: &room}}
	fit.MixCats[3] = true
	over := fit
	over.Credit.State = domain.CreditOverLimit
	churn := fit
	churn.Status, churn.Cyc, churn.DueIn = domain.StatusChurn, 2.5, nil
	got := PushCandidates(led, []DealerView{{ID: "fit", Metrics: fit}, {ID: "over", Metrics: over}, {ID: "churn", Metrics: churn}}, p)
	if len(got) != 1 || got[0].DealerID != "fit" {
		t.Fatalf("push candidates %+v", got)
	}
}

// Properties: score always 0–100; status moves outward monotonically with cyc; segment is stable at the threshold.
func TestProperties(t *testing.T) {
	p := domain.DefaultPolicies()
	rank := map[string]int{domain.StatusKeyAccount: 0, domain.StatusAktif: 0, domain.StatusAtRisk: 1, domain.StatusChurn: 2}
	for rhythm := 5; rhythm <= 60; rhythm += 5 {
		prev := -1
		for last := 0; last <= 200; last++ {
			cyc := Cyc(ip(rhythm), last)
			st := Status(ip(rhythm), cyc, 70, 95, p)
			if rank[st] < prev {
				t.Fatalf("status not monotone at rhythm %d last %d", rhythm, last)
			}
			prev = rank[st]
			for _, sow := range []int{0, 50, 100} {
				for _, room := range []float64{-1, 0, 0.5, 1} {
					r := room
					s, _ := Score(ScoreInput{Rhythm: ip(rhythm), Cyc: cyc, SOW: sow, Mix: last % 7, Limit: 1e6, Room: &r, OnTime: last % 101, PICCount: last % 5})
					if s < 0 || s > 100 {
						t.Fatalf("score %d out of range", s)
					}
				}
			}
		}
	}
	f := 1.5
	if Segment(&f, 20_000_000, p) != domain.SegmentA || Segment(&f, 19_999_999, p) != domain.SegmentB {
		t.Error("segment threshold must be inclusive (≥)")
	}
	f = 1.4999
	if Segment(&f, 20_000_000, p) != domain.SegmentC {
		t.Error("below frequency threshold is jarang")
	}
}

func TestRhythmMedianIgnoresAnomalies(t *testing.T) {
	// gaps 20, 24, 20, 20, 20 → median 20 (one irregular delivery does not move the cycle)
	var ds []time.Time
	for _, x := range []int{3, 23, 47, 67, 87, 107} {
		ds = append([]time.Time{ago(x)}, ds...)
	}
	if r := Rhythm(ds, today); r == nil || *r != 20 {
		t.Fatalf("rhythm %v want 20", r)
	}
}

func TestMonthlyAndComposition(t *testing.T) {
	os := orders(14, 6, 62e6, "Kamera & NVR", "HDD & storage")
	months := MonthlyTotals(os, today, 6)
	if len(months) != 6 || months[5].Label != "Okt" || months[0].Label != "Mei" {
		t.Fatalf("months %+v", months)
	}
	comp := Composition(os, today, 4)
	if len(comp) != 2 || comp[0].Pct != 50 {
		t.Fatalf("composition %+v", comp)
	}
}
