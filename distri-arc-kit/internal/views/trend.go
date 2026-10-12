package views

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
)

// Dashboard trend: every dashboard number recomputed at past dates from the same history and the same metrics
// formulas (01-glossary), so a chart point is what the card would have shown on that day.

const (
	TrendMonths = "months" // the last 12 months, one point per month (as of its last day, or today)
	TrendDays   = "days"   // one month, one point per day 1…30/31 (days after today stay empty)
)

// TrendDueDays is the window of "Jadwal order · 14 hari" (web DUE_DAYS).
const TrendDueDays = 14

// Trend keys that have no history in the data: stock is a current position only (no snapshots).
var trendUnavailable = []string{"stock_push", "stock_turn"}

// Trend is the series of the dashboard cards; a nil point is a day still to come.
type Trend struct {
	Mode        string              `json:"mode"`
	Month       string              `json:"month,omitempty"` // 2026-10 (days mode)
	Labels      []string            `json:"labels"`
	Dates       []string            `json:"dates"` // as-of date of each point (YYYY-MM-DD)
	Series      map[string][]*int64 `json:"series"`
	Unavailable []string            `json:"unavailable"`
	Months      []string            `json:"months"` // months that can be opened in days mode, newest first
}

type bucket struct {
	label      string
	start, end time.Time // start of the first day, start of the as-of day (inclusive)
	future     bool
}

func buckets(mode string, month, today time.Time) []bucket {
	today = clock.Today(today)
	var out []bucket
	if mode == TrendDays {
		first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, clock.WIB)
		for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
			out = append(out, bucket{label: fmt.Sprint(d.Day()), start: d, end: d, future: d.After(today)})
		}
		return out
	}
	cur := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, clock.WIB)
	for i := 11; i >= 0; i-- {
		first := cur.AddDate(0, -i, 0)
		end := first.AddDate(0, 1, -1)
		if end.After(today) {
			end = today
		}
		out = append(out, bucket{label: fmt.Sprintf("%s %02d", metrics.MonthLabel(first.Month()), first.Year()%100), start: first, end: end})
	}
	return out
}

func orderAt(o domain.Order) *time.Time {
	if o.ConfirmedAt != nil {
		return o.ConfirmedAt
	}
	return o.OrderedAt
}

// historyAsOf is the dealer's history as it stood at the end of `day`: later orders and invoices are dropped,
// payments made after it are undone.
func historyAsOf(h domain.DealerHistory, day time.Time) domain.DealerHistory {
	end := day.AddDate(0, 0, 1)
	out := h
	out.Orders = nil
	for _, o := range h.Orders {
		t := orderAt(o)
		if t == nil || !t.Before(end) {
			continue
		}
		if o.PaidAt != nil && !o.PaidAt.Before(end) {
			o.PaidAt = nil
			if o.State == "bayar" {
				o.State = "invoice"
			}
		}
		out.Orders = append(out.Orders, o)
	}
	out.Invoices = nil
	for _, inv := range h.Invoices {
		if !clock.Today(inv.IssuedAt).Before(end) {
			continue
		}
		if inv.PaidAt != nil && !clock.Today(*inv.PaidAt).Before(end) {
			inv.Paid, inv.PaidAt = 0, nil
		}
		out.Invoices = append(out.Invoices, inv)
	}
	return out
}

// ComputeTrend computes the dashboard series for the given dealers.
func ComputeTrend(ds *dealersvc.Dataset, ids []uuid.UUID, mode string, month time.Time) Trend {
	if mode != TrendDays {
		mode = TrendMonths
	}
	p := ds.Policies
	bs := buckets(mode, month, ds.Today)
	keys := []string{"dealers", "omzet", "on_schedule", "at_risk", "due", "dso", "tight", "receivable", "overdue", "cash_in"}
	t := Trend{Mode: mode, Series: map[string][]*int64{}, Unavailable: trendUnavailable}
	if mode == TrendDays {
		t.Month = month.Format("2006-01")
	}
	for _, k := range keys {
		t.Series[k] = make([]*int64, len(bs))
	}
	for i, b := range bs {
		t.Labels = append(t.Labels, b.label)
		t.Dates = append(t.Dates, b.end.Format("2006-01-02"))
		if b.future {
			continue
		}
		asOf := b.end.Add(12 * time.Hour) // midday of the as-of day: whole-day arithmetic in metrics
		next := b.end.AddDate(0, 0, 1)
		var ms []domain.DealerMetrics
		var orders []domain.Order
		var dealers, atRisk, due, tight, omzet, receivable, overdue, cash int64
		for _, id := range ids {
			full := ds.Histories[id]
			// sales and cash of the bucket itself, from the full history
			for _, o := range full.Orders {
				if at := orderAt(o); o.State != "cancel" && at != nil && !at.Before(b.start) && at.Before(next) {
					omzet += o.Total
				}
			}
			for _, inv := range full.Invoices {
				if inv.PaidAt != nil && !clock.Today(*inv.PaidAt).Before(b.start) && clock.Today(*inv.PaidAt).Before(next) {
					cash += inv.Paid
				}
			}
			h := historyAsOf(full, b.end)
			m := metrics.Compute(h, p, asOf)
			ms = append(ms, m)
			orders = append(orders, h.Orders...)
			if m.Segment != domain.SegmentProspek {
				dealers++
				if m.Status == domain.StatusAtRisk || m.Status == domain.StatusChurn {
					atRisk++
				}
				if m.Rhythm != nil && m.DueIn != nil && *m.DueIn >= 0 && *m.DueIn <= TrendDueDays && m.Status != domain.StatusChurn {
					due++
				}
			}
			if h.CreditLimit > 0 && m.Credit.State != domain.CreditAman {
				tight++
			}
			for _, inv := range h.Invoices {
				if r := inv.Residual(); r > 0 {
					receivable += r
					if clock.Today(inv.DueAt).Before(b.end) {
						overdue += r
					}
				}
			}
		}
		set := func(k string, v int64) { t.Series[k][i] = &v }
		set("dealers", dealers)
		set("omzet", omzet)
		set("on_schedule", int64(metrics.OnSchedulePct(ms, p)))
		set("at_risk", atRisk)
		set("due", due)
		set("dso", int64(metrics.DSO(orders, asOf)))
		set("tight", tight)
		set("receivable", receivable)
		set("overdue", overdue)
		set("cash_in", cash)
	}
	today := clock.Today(ds.Today)
	cur := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, clock.WIB)
	for i := 0; i < 12; i++ {
		t.Months = append(t.Months, cur.AddDate(0, -i, 0).Format("2006-01"))
	}
	return t
}

// trendTTL: the trend moves with imports, like the board.
const trendTTL = 5 * time.Minute

type trendEntry struct {
	at time.Time
	t  Trend
}

// Trend returns the dashboard series for the dealers `ids` (cached per scope, mode and month).
func (b *Builder) Trend(ctx context.Context, ids []uuid.UUID, mode string, month time.Time) (Trend, error) {
	sorted := make([]string, len(ids))
	for i, id := range ids {
		sorted[i] = id.String()
	}
	sort.Strings(sorted)
	h := sha256.New()
	_, _ = fmt.Fprint(h, mode, month.Format("2006-01"), sorted)
	key := hex.EncodeToString(h.Sum(nil))

	b.mu.Lock()
	if e, ok := b.trends[key]; ok && time.Since(e.at) < trendTTL {
		b.mu.Unlock()
		return e.t, nil
	}
	ds := b.trendData
	if ds == nil || time.Since(b.trendAt) > trendTTL || !clock.Today(ds.Today).Equal(clock.Today(b.clock.Now())) {
		var err error
		if ds, err = b.svc.LoadYears(ctx, 2); err != nil {
			b.mu.Unlock()
			return Trend{}, err
		}
		b.trendData, b.trendAt, b.trends = ds, time.Now(), map[string]trendEntry{}
	}
	b.mu.Unlock()

	t := ComputeTrend(ds, ids, mode, month)
	b.mu.Lock()
	if b.trends == nil {
		b.trends = map[string]trendEntry{}
	}
	b.trends[key] = trendEntry{at: time.Now(), t: t}
	b.mu.Unlock()
	return t, nil
}
