// Package dealersvc loads dealer histories from PostgreSQL, runs metrics.Compute and stores the result in
// dealers.metrics_current (cache read by the UI) and dealer_metrics_daily (daily snapshot for trends).
package dealersvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Service recomputes dealer metrics.
type Service struct {
	st    *store.Store
	clock clock.Clock
}

// New builds the service.
func New(st *store.Store, c clock.Clock) *Service { return &Service{st: st, clock: c} }

// Dataset is the in-memory picture of all dealers used by recompute and the read models.
type Dataset struct {
	Today     time.Time
	Policies  domain.PolicySet
	Dealers   []gen.ListDealersFullRow
	Histories map[uuid.UUID]domain.DealerHistory
}

// Load reads all dealers with 12 months of orders and invoices (plus every open invoice).
func (s *Service) Load(ctx context.Context) (*Dataset, error) {
	now := s.clock.Now()
	pol, err := policy.Load(ctx, s.st.Q)
	if err != nil {
		return nil, err
	}
	since := now.AddDate(-1, 0, -7)
	dealers, err := s.st.Q.ListDealersFull(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.st.Q.ListOrders(ctx, &since)
	if err != nil {
		return nil, err
	}
	invoices, err := s.st.Q.ListInvoices(ctx, &since)
	if err != nil {
		return nil, err
	}
	contacts, err := s.st.Q.ListContacts(ctx)
	if err != nil {
		return nil, err
	}
	sows, err := s.st.Q.ListSowEstimates(ctx)
	if err != nil {
		return nil, err
	}
	h := map[uuid.UUID]domain.DealerHistory{}
	for _, d := range dealers {
		h[d.ID] = domain.DealerHistory{CreditLimit: d.CreditLimit, TermsDays: int(d.PaymentTermsDays)}
	}
	for _, o := range orders {
		if o.DealerID == nil {
			continue
		}
		x := h[*o.DealerID]
		x.Orders = append(x.Orders, OrderFromRow(o))
		h[*o.DealerID] = x
	}
	for _, i := range invoices {
		if i.DealerID == nil {
			continue
		}
		x := h[*i.DealerID]
		x.Invoices = append(x.Invoices, InvoiceFromRow(i))
		h[*i.DealerID] = x
	}
	for _, c := range contacts {
		if c.DealerID == nil {
			continue
		}
		x := h[*c.DealerID]
		x.Contacts = append(x.Contacts, ContactFromRow(c))
		h[*c.DealerID] = x
	}
	for _, e := range sows {
		if e.DealerID == nil {
			continue
		}
		x := h[*e.DealerID]
		x.SOWEstimates = append(x.SOWEstimates, domain.SOWEstimate{Quarter: e.Quarter, SOW: int(e.Sow), ConfirmedAt: e.ConfirmedAt})
		h[*e.DealerID] = x
	}
	return &Dataset{Today: now, Policies: pol, Dealers: dealers, Histories: h}, nil
}

// Recompute computes and stores metrics for the given dealers (all when ids is empty).
func (s *Service) Recompute(ctx context.Context, ids ...uuid.UUID) (map[uuid.UUID]domain.DealerMetrics, error) {
	// PIC aktif first: interactions_90d / last_interaction_at from the messages of each contact's number
	if err := s.st.Q.RecountContacts(ctx, s.clock.Now().AddDate(0, 0, -90)); err != nil {
		return nil, err
	}
	ds, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}
	want := map[uuid.UUID]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[uuid.UUID]domain.DealerMetrics{}
	for _, d := range ds.Dealers {
		if len(want) > 0 && !want[d.ID] {
			continue
		}
		m := metrics.Compute(ds.Histories[d.ID], ds.Policies, ds.Today)
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		if err := s.st.Q.SetDealerMetrics(ctx, gen.SetDealerMetricsParams{ID: d.ID, MetricsCurrent: b}); err != nil {
			return nil, fmt.Errorf("store metrics %s: %w", d.Name, err)
		}
		out[d.ID] = m
	}
	return out, nil
}

// Snapshot writes today's metrics of every dealer into dealer_metrics_daily (idempotent per day).
func (s *Service) Snapshot(ctx context.Context) (int, error) {
	ms, err := s.Recompute(ctx)
	if err != nil {
		return 0, err
	}
	asOf := clock.Today(s.clock.Now())
	for id, m := range ms {
		parts, _ := json.Marshal(m.ScoreParts)
		cyc := m.Cyc
		sow, mix := int16(m.SOW), int16(m.Mix)
		pay, ontime, pic, score := int32(m.Credit.PayDays), int16(m.Credit.OnTime), int16(m.PICActive), int16(m.Score)
		avg := m.AvgOrder
		st, ac, seg, cs := m.Status, m.Activity, m.Segment, m.Credit.State
		p := gen.UpsertMetricsSnapshotParams{DealerID: id, AsOf: asOf, Cyc: &cyc, Status: &st, Activity: &ac, Freq: m.Freq, AvgOrder: &avg,
			Segment: &seg, Sow: &sow, Mix: &mix, CreditRoom: m.Credit.Room, CreditState: &cs, PayDays: &pay, OnTime: &ontime, PicActive: &pic, Score: &score, ScoreParts: parts}
		if m.Rhythm != nil {
			r := int32(*m.Rhythm)
			p.RhythmDays = &r
		}
		if m.Last != nil {
			l := int32(*m.Last)
			p.LastOrderDays = &l
		}
		if err := s.st.Q.UpsertMetricsSnapshot(ctx, p); err != nil {
			return 0, err
		}
	}
	return len(ms), nil
}

// OrderFromRow converts a stored order.
func OrderFromRow(o gen.Order) domain.Order {
	d := domain.Order{ID: o.ID, State: o.State, OrderedAt: o.OrderedAt, ConfirmedAt: o.ConfirmedAt, ShippedAt: o.ShippedAt, InvoicedAt: o.InvoicedAt, PaidAt: o.PaidAt, Total: o.Total, CreatedBy: o.CreatedBy}
	if o.Number != nil {
		d.Number = *o.Number
	}
	if o.MarginPct != nil {
		d.MarginPct = *o.MarginPct
	}
	_ = json.Unmarshal(o.Lines, &d.Lines)
	return d
}

// InvoiceFromRow converts a stored invoice.
func InvoiceFromRow(i gen.Invoice) domain.Invoice {
	d := domain.Invoice{ID: i.ID, OrderID: i.OrderID, Total: i.Total, Paid: i.Paid, PaidAt: i.PaidAt, State: i.State}
	if i.Number != nil {
		d.Number = *i.Number
	}
	if i.IssuedAt != nil {
		d.IssuedAt = *i.IssuedAt
	}
	if i.DueAt != nil {
		d.DueAt = *i.DueAt
	}
	return d
}

// ContactFromRow converts a stored contact.
func ContactFromRow(c gen.Contact) domain.Contact {
	d := domain.Contact{ID: c.ID, IsPrimary: c.IsPrimary, LastInteractionAt: c.LastInteractionAt, Interactions90d: int(c.Interactions90d)}
	if c.Name != nil {
		d.Name = *c.Name
	}
	if c.Role != nil {
		d.Role = *c.Role
	}
	if c.WaNumber != nil {
		d.WANumber = *c.WaNumber
	}
	return d
}

// Metrics decodes dealers.metrics_current.
func Metrics(raw json.RawMessage) (domain.DealerMetrics, bool) {
	var m domain.DealerMetrics
	if len(raw) == 0 {
		return m, false
	}
	return m, json.Unmarshal(raw, &m) == nil
}

// ShortName drops the legal prefix (PT, CV, UD, Toko) the way the mockup labels dealers on boards.
func ShortName(name string) string {
	for _, p := range []string{"PT ", "CV ", "UD ", "Toko "} {
		if strings.HasPrefix(name, p) {
			return strings.TrimPrefix(name, p)
		}
	}
	return name
}
