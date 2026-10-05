// Package metrics implements every Orbit number of docs/design/01-glossary.md as pure functions.
// Thresholds come from domain.PolicySet; nothing here touches the database or an LLM.
package metrics

import (
	"math"
	"sort"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
)

const (
	window6m  = 182 // "6 bulan"
	window12m = 365 // "12 bulan"
	picDays   = 90  // PIC aktif: replied or ordered within 90 days
)

// Day returns the calendar date (midnight WIB) of t.
func Day(t time.Time) time.Time { return clock.Today(t) }

// DaysBetween counts calendar days (WIB) from a to b.
func DaysBetween(a, b time.Time) int {
	return int(math.Round(Day(b).Sub(Day(a)).Hours() / 24))
}

// confirmedDates returns confirmation dates of non-cancelled orders, oldest first.
func confirmedDates(orders []domain.Order) []time.Time {
	var out []time.Time
	for _, o := range orders {
		if o.State == "cancel" {
			continue
		}
		if t := orderTime(o); t != nil {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func orderTime(o domain.Order) *time.Time {
	if o.ConfirmedAt != nil {
		return o.ConfirmedAt
	}
	return o.OrderedAt
}

// Rhythm is siklus order: the median gap in days between confirmed orders of the last 6 months.
// It needs ≥ 2 orders; a dealer with a single order has no rhythm (status Baru). When the 6-month window holds
// fewer than 2 orders but older ones exist (a long-silent dealer), the last 3 orders of 12 months are used so
// the dealer still reads as Churn instead of Baru.
func Rhythm(dates []time.Time, today time.Time) *int {
	var in []time.Time
	for _, d := range dates {
		if age := DaysBetween(d, today); age >= 0 && age <= window6m {
			in = append(in, d)
		}
	}
	if len(in) < 2 {
		in = nil
		for i := len(dates) - 1; i >= 0 && len(in) < 3; i-- {
			if DaysBetween(dates[i], today) <= window12m {
				in = append([]time.Time{dates[i]}, in...)
			}
		}
	}
	if len(in) < 2 {
		return nil
	}
	gaps := make([]float64, 0, len(in)-1)
	for i := 1; i < len(in); i++ {
		gaps = append(gaps, float64(DaysBetween(in[i-1], in[i])))
	}
	r := int(math.Round(median(gaps)))
	if r <= 0 {
		r = 1
	}
	return &r
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// Cyc is the position in the cycle: days since last order ÷ rhythm (0 when there is no rhythm).
func Cyc(rhythm *int, last int) float64 {
	if rhythm == nil || *rhythm == 0 {
		return 0
	}
	return float64(last) / float64(*rhythm)
}

// Status evaluates the orbit ring in glossary order: Baru → Churn → At risk → Key account → Aktif.
func Status(rhythm *int, cyc float64, sow, onTime int, p domain.PolicySet) string {
	switch {
	case rhythm == nil:
		return domain.StatusBaru
	case cyc > p.Orbit.Churn:
		return domain.StatusChurn
	case cyc > p.Orbit.Drift:
		return domain.StatusAtRisk
	case sow >= p.Orbit.KeyAccount.SOWMin && onTime >= p.Orbit.KeyAccount.OnTimeMin:
		return domain.StatusKeyAccount
	default:
		return domain.StatusAktif
	}
}

// Activity: normal (cyc ≤ 1.05), menurun (≤ 2), berhenti (> 2), baru (no rhythm).
func Activity(rhythm *int, cyc float64) string {
	switch {
	case rhythm == nil:
		return domain.ActivityBaru
	case cyc <= 1.05:
		return domain.ActivityNormal
	case cyc <= 2:
		return domain.ActivityMenurun
	default:
		return domain.ActivityBerhenti
	}
}

// Freq is seringnya: orders per month = 30 ÷ rhythm.
func Freq(rhythm *int) *float64 {
	if rhythm == nil || *rhythm == 0 {
		return nil
	}
	f := 30 / float64(*rhythm)
	return &f
}

// Segment: A sering & besar, B sering & kecil, C jarang & besar, D jarang & kecil, Baru without rhythm.
func Segment(freq *float64, avg int64, p domain.PolicySet) string {
	if freq == nil {
		return domain.SegmentBaru
	}
	sering := *freq >= p.Segment.FreqPerMonth
	besar := avg >= p.Segment.SizeIDR
	switch {
	case sering && besar:
		return domain.SegmentA
	case sering:
		return domain.SegmentB
	case besar:
		return domain.SegmentC
	default:
		return domain.SegmentD
	}
}

// Omzet is omzet/bln = besarnya × seringnya (dealer baru: the first order's value).
func Omzet(avg int64, freq *float64) int64 {
	if freq == nil {
		return avg
	}
	return int64(math.Round(float64(avg) * *freq))
}

// SOW returns share of wallet and its source: the latest sales confirmation, else the competitor-based
// estimate from WA extraction, else 50 marked "default".
func SOW(estimates []domain.SOWEstimate, competitor *int) (int, string) {
	if len(estimates) > 0 {
		latest := estimates[0]
		for _, e := range estimates[1:] {
			if e.ConfirmedAt.After(latest.ConfirmedAt) {
				latest = e
			}
		}
		return clampInt(latest.SOW, 0, 100), "confirmed"
	}
	if competitor != nil {
		return clampInt(*competitor, 0, 100), "estimated"
	}
	return 50, "default"
}

// Mix returns which of the 6 categories were bought (≥ 1 line) in confirmed orders of the last 6 months.
func Mix(orders []domain.Order, today time.Time) (int, [6]bool) {
	var cats [6]bool
	for _, o := range orders {
		t := orderTime(o)
		if o.State == "cancel" || t == nil || DaysBetween(*t, today) > window6m {
			continue
		}
		for _, l := range o.Lines {
			if i := domain.CategoryIndex(l.Category); i >= 0 {
				cats[i] = true
			}
		}
	}
	n := 0
	for _, c := range cats {
		if c {
			n++
		}
	}
	return n, cats
}

// CreditInput is what CreditOf needs.
type CreditInput struct {
	Limit    int64
	Invoices []domain.Invoice
}

// CreditOf computes sisa limit: room, late, pola bayar (6 months), tepat waktu (12 months) and the state.
//
//	room  = 1 − exposure ÷ limit (limit 0 → cash)
//	state = over limit (room < 0) · overdue (an open invoice past due) · tipis (room < room_min or
//	        pola bayar > pay_max) · aman
//
// Tepat waktu counts invoices issued within 12 months that are paid or past due; on time = paid ≤ due date.
func CreditOf(in CreditInput, today time.Time, p domain.PolicySet) domain.Credit {
	c := domain.Credit{Limit: in.Limit, OnTime: 100}
	var paySum, payN, den, onTime int
	for _, inv := range in.Invoices {
		if inv.State == "cancel" {
			continue
		}
		age := DaysBetween(inv.IssuedAt, today)
		open := inv.Residual() > 0
		if open {
			c.Exposure += inv.Residual()
			if Day(inv.DueAt).Before(Day(today)) {
				c.Late = true
				if d := DaysBetween(inv.DueAt, today); d > c.LateDays {
					c.LateDays = d
				}
			}
		}
		if inv.PaidAt != nil && !open && age <= window6m {
			paySum += DaysBetween(inv.IssuedAt, *inv.PaidAt)
			payN++
		}
		if age <= window12m {
			switch {
			case inv.PaidAt != nil && !open:
				den++
				if !Day(*inv.PaidAt).After(Day(inv.DueAt)) {
					onTime++
				}
			case Day(inv.DueAt).Before(Day(today)):
				den++
			}
		}
	}
	if payN > 0 {
		c.PayDays = int(math.Round(float64(paySum) / float64(payN)))
	}
	if den > 0 {
		c.OnTime = int(math.Round(100 * float64(onTime) / float64(den)))
	}
	if in.Limit <= 0 {
		c.State = domain.CreditCash
		return c
	}
	room := 1 - float64(c.Exposure)/float64(in.Limit)
	c.Room = &room
	switch {
	case room < 0:
		c.State = domain.CreditOverLimit
	case c.Late:
		c.State = domain.CreditOverdue
	case room < p.Credit.RoomMin || c.PayDays > p.Credit.PayMaxDays:
		c.State = domain.CreditTipis
	default:
		c.State = domain.CreditAman
	}
	return c
}

// CreditTone maps a credit state to the UI color key.
func CreditTone(state string) string {
	switch state {
	case domain.CreditAman:
		return "good"
	case domain.CreditTipis:
		return "warn"
	case domain.CreditOverLimit, domain.CreditOverdue:
		return "bad"
	default:
		return "neutral"
	}
}

// PICActive counts contacts who replied on WA or ordered within 90 days.
func PICActive(contacts []domain.Contact, today time.Time) int {
	n := 0
	for _, c := range contacts {
		if c.LastInteractionAt != nil && DaysBetween(*c.LastInteractionAt, today) <= picDays {
			n++
		}
	}
	return n
}

// ScoreInput holds the five drivers of skor dealer.
type ScoreInput struct {
	Rhythm   *int
	Cyc      float64
	SOW      int
	Mix      int // categories bought, 0–6
	Limit    int64
	Room     *float64
	OnTime   int
	PICCount int
}

// Score computes skor dealer and its five components (glossary):
//
//	r = rhythm null ? 50 : max(0, 100 − max(0, cyc − 1) × 120)
//	p = sow · k = mix × 100 · n = limit 0 ? 80 : clamp(room × 150 + (on_time − 50), 0, 100)
//	i = min(100, pic × 40) · skor = round(0.30r + 0.25p + 0.15k + 0.15n + 0.15i)
func Score(in ScoreInput) (int, domain.ScoreParts) {
	r := 50.0
	if in.Rhythm != nil {
		r = math.Max(0, 100-math.Max(0, in.Cyc-1)*120)
	}
	p := float64(in.SOW)
	k := float64(in.Mix) / 6 * 100
	n := 80.0
	if in.Limit > 0 {
		room := 0.0
		if in.Room != nil {
			room = *in.Room
		}
		n = clamp(room*150+float64(in.OnTime-50), 0, 100)
	}
	i := math.Min(100, float64(in.PICCount*40))
	// Explicit float64 conversions round each product and stop the compiler from fusing them into FMA
	// instructions (arm64), so x.5 totals round exactly like the reference formula.
	s := int(math.Round(float64(0.30*r) + float64(0.25*p) + float64(0.15*k) + float64(0.15*n) + float64(0.15*i)))
	return s, domain.ScoreParts{Rhythm: roundInt(r), SOW: roundInt(p), Mix: roundInt(k), Credit: roundInt(n), Contact: roundInt(i)}
}

// Compute derives every metric of a dealer from its history.
func Compute(h domain.DealerHistory, p domain.PolicySet, today time.Time) domain.DealerMetrics {
	m := domain.DealerMetrics{AsOf: today}
	dates := confirmedDates(h.Orders)
	if len(dates) > 0 {
		last := DaysBetween(dates[len(dates)-1], today)
		m.Last = &last
	}
	m.Rhythm = Rhythm(dates, today)
	if m.Last != nil {
		m.Cyc = Cyc(m.Rhythm, *m.Last)
		if m.Rhythm != nil {
			due := *m.Rhythm - *m.Last
			m.DueIn = &due
		}
	}

	var sum int64
	var n int
	var cycleSum, cycleN int
	for _, o := range h.Orders {
		t := orderTime(o)
		if o.State == "cancel" || t == nil || DaysBetween(*t, today) > window6m {
			continue
		}
		sum += o.Total
		n++
		if o.PaidAt != nil {
			cycleSum += DaysBetween(*t, *o.PaidAt)
			cycleN++
		}
	}
	m.Orders6m = n
	if n > 0 {
		m.AvgOrder = int64(math.Round(float64(sum) / float64(n)))
	} else if len(h.Orders) > 0 {
		m.AvgOrder = latestOrder(h.Orders).Total
	}
	if cycleN > 0 {
		m.CycleDays = int(math.Round(float64(cycleSum) / float64(cycleN)))
	}
	m.Freq = Freq(m.Rhythm)
	m.OmzetBln = Omzet(m.AvgOrder, m.Freq)
	m.Segment = Segment(m.Freq, m.AvgOrder, p)
	m.SOW, m.SOWSource = SOW(h.SOWEstimates, h.CompetitorSOW)
	m.Mix, m.MixCats = Mix(h.Orders, today)
	m.Credit = CreditOf(CreditInput{Limit: h.CreditLimit, Invoices: h.Invoices}, today, p)
	m.PICActive = PICActive(h.Contacts, today)
	m.Status = Status(m.Rhythm, m.Cyc, m.SOW, m.Credit.OnTime, p)
	m.Activity = Activity(m.Rhythm, m.Cyc)
	m.Score, m.ScoreParts = Score(ScoreInput{Rhythm: m.Rhythm, Cyc: m.Cyc, SOW: m.SOW, Mix: m.Mix, Limit: h.CreditLimit, Room: m.Credit.Room, OnTime: m.Credit.OnTime, PICCount: m.PICActive})
	return m
}

func latestOrder(orders []domain.Order) domain.Order {
	var best domain.Order
	var bt time.Time
	for _, o := range orders {
		if t := orderTime(o); t != nil && t.After(bt) {
			best, bt = o, *t
		}
	}
	return best
}

// Phase returns the order-to-cash phase index (0 Order … 4 Bayar) of an order.
func Phase(o domain.Order) int {
	switch {
	case o.PaidAt != nil || o.State == "bayar":
		return 4
	case o.InvoicedAt != nil || o.State == "invoice":
		return 3
	case o.ShippedAt != nil || o.State == "kirim":
		return 2
	case o.State == "siap":
		return 1
	default:
		return 0
	}
}

// LastOrder returns the most recent non-cancelled order.
func LastOrder(orders []domain.Order) (domain.Order, bool) {
	var live []domain.Order
	for _, o := range orders {
		if o.State != "cancel" {
			live = append(live, o)
		}
	}
	if len(live) == 0 {
		return domain.Order{}, false
	}
	return latestOrder(live), true
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func roundInt(v float64) int { return int(math.Round(v)) }
