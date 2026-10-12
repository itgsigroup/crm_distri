package views_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
	"distri-arc/internal/views"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 10, 0, 0, 0, clock.WIB) }
func ptr[T any](v T) *T                        { return &v }

// One dealer: an order in August paid in September, an order in October still open. Each point is the card as it
// stood on that day: sales and cash of the bucket, and a receivable that a later payment does not erase.
func TestComputeTrend(t *testing.T) {
	id := uuid.New()
	aug := day(2026, 8, 10)
	octo := day(2026, 10, 2)
	ds := &dealersvc.Dataset{
		Today:    day(2026, 10, 5),
		Policies: domain.DefaultPolicies(),
		Histories: map[uuid.UUID]domain.DealerHistory{id: {
			CreditLimit: 100_000_000, TermsDays: 30,
			Orders: []domain.Order{
				{State: "bayar", ConfirmedAt: &aug, PaidAt: ptr(day(2026, 9, 12)), Total: 10_000_000},
				{State: "invoice", ConfirmedAt: &octo, Total: 4_000_000},
				{State: "cancel", ConfirmedAt: &octo, Total: 99_000_000},
			},
			Invoices: []domain.Invoice{
				{Number: "INV/1", IssuedAt: aug, DueAt: day(2026, 9, 9), Total: 10_000_000, Paid: 10_000_000, PaidAt: ptr(day(2026, 9, 12))},
				{Number: "INV/2", IssuedAt: octo, DueAt: day(2026, 11, 1), Total: 4_000_000},
			},
		}},
	}

	m := views.ComputeTrend(ds, []uuid.UUID{id}, views.TrendMonths, ds.Today)
	if len(m.Labels) != 12 || m.Labels[11] != "Okt 26" || m.Labels[9] != "Agu 26" {
		t.Fatalf("labels %v", m.Labels)
	}
	v := func(k string, i int) int64 {
		if m.Series[k][i] == nil {
			t.Fatalf("%s[%d] nil", k, i)
		}
		return *m.Series[k][i]
	}
	if v("omzet", 9) != 10_000_000 || v("omzet", 10) != 0 || v("omzet", 11) != 4_000_000 {
		t.Fatalf("omzet %d %d %d (cancelled orders do not count)", v("omzet", 9), v("omzet", 10), v("omzet", 11))
	}
	if v("cash_in", 10) != 10_000_000 || v("cash_in", 9) != 0 {
		t.Fatalf("cash_in Sep %d Aug %d", v("cash_in", 10), v("cash_in", 9))
	}
	// end of August: INV/1 not yet paid (paid 12 Sep), not yet due either
	if v("receivable", 9) != 10_000_000 || v("overdue", 9) != 0 {
		t.Fatalf("Aug receivable %d overdue %d", v("receivable", 9), v("overdue", 9))
	}
	if v("receivable", 10) != 0 || v("receivable", 11) != 4_000_000 {
		t.Fatalf("receivable Sep %d Oct %d", v("receivable", 10), v("receivable", 11))
	}
	if v("dealers", 7) != 0 || v("dealers", 11) != 1 {
		t.Fatalf("dealers Jun %d Oct %d (no order yet = prospect)", v("dealers", 7), v("dealers", 11))
	}

	d := views.ComputeTrend(ds, []uuid.UUID{id}, views.TrendDays, day(2026, 10, 1))
	if d.Month != "2026-10" || len(d.Labels) != 31 {
		t.Fatalf("days %s %d", d.Month, len(d.Labels))
	}
	if d.Series["omzet"][1] == nil || *d.Series["omzet"][1] != 4_000_000 || *d.Series["omzet"][0] != 0 {
		t.Fatal("omzet on 2 Oct")
	}
	if d.Series["omzet"][4] == nil || d.Series["omzet"][5] != nil {
		t.Fatal("days after today are empty, today is not")
	}
	if len(d.Unavailable) == 0 || len(d.Months) != 12 || d.Months[0] != "2026-10" {
		t.Fatalf("unavailable %v months %v", d.Unavailable, d.Months)
	}
}

// The builder serves the seed: twelve points per series, the last one equal to today's card.
func TestTrendSeed(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	b := views.NewBuilder(st, clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB)))
	board, err := b.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	var active, tight int64
	for _, it := range board.Items {
		ids = append(ids, it.UUID)
		if it.Metrics.Segment != domain.SegmentProspek {
			active++
		}
		if it.CreditLimit > 0 && it.Metrics.Credit.State != domain.CreditAman {
			tight++
		}
	}
	tr, err := b.Trend(ctx, ids, views.TrendMonths, board.Today)
	if err != nil {
		t.Fatal(err)
	}
	last := len(tr.Labels) - 1
	if got := *tr.Series["dealers"][last]; got != active {
		t.Fatalf("dealers today %d, card %d", got, active)
	}
	if got := *tr.Series["tight"][last]; got != tight {
		t.Fatalf("tight today %d, card %d", got, tight)
	}
	if got := *tr.Series["on_schedule"][last]; got != int64(board.KPI("", nil).OnSchedulePct) {
		t.Fatalf("on schedule today %d, card %d", got, board.KPI("", nil).OnSchedulePct)
	}
}
