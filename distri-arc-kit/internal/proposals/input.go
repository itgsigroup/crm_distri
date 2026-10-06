// Package proposals stores what the agents propose and records human decisions. It builds the agents' Input from
// the database, enforces provenance, calibration suppression and idempotency, and turns approvals into outbox
// rows — the only way anything reaches a dealer.
package proposals

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/metrics"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// BuildInput prepares the agents' view of every dealer (or one dealer when only is set).
func BuildInput(ctx context.Context, st *store.Store, c clock.Clock, only string, cycleID *string) (*agents.Input, *views.Board, error) {
	b, err := views.NewBuilder(st, c).Board(ctx)
	if err != nil {
		return nil, nil, err
	}
	in := &agents.Input{CycleID: cycleID, Today: b.Today, Policies: b.Policies}
	stock, err := st.Q.ListStockItems(ctx)
	if err != nil {
		return nil, nil, err
	}
	in.Stock = views.StockItems(stock)

	sold := map[string]bool{}
	for _, it := range b.Items {
		for _, o := range b.Data.Histories[it.UUID].Orders {
			for _, l := range o.Lines {
				sold[l.Product] = true
			}
		}
	}
	prods, err := st.Q.ListProducts(ctx)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	in.Catalog = map[string]agents.Product{}
	for _, p := range prods {
		if _, ok := in.Catalog[p.Name]; !ok {
			var prices map[string]int64
			_ = json.Unmarshal(p.Prices, &prices)
			in.Catalog[p.Name] = agents.Product{ID: p.ID, Name: p.Name, Category: deref(p.Category), Prices: prices, Cost: p.Cost, SKU: deref(p.Sku)}
		}
		if !sold[p.Name] || seen[p.Name] {
			continue // stock SKUs are matched separately; requests are matched to the sales catalog
		}
		var prices map[string]int64
		_ = json.Unmarshal(p.Prices, &prices)
		ap := agents.Product{ID: p.ID, Name: p.Name, Category: deref(p.Category), Prices: prices, Cost: p.Cost, SKU: deref(p.Sku)}
		if p.SourceID != nil {
			if _, after, ok := strings.Cut(*p.SourceID, ":"); ok {
				ap.OdooID, _ = strconv.Atoi(after)
			}
		}
		seen[p.Name] = true
		in.Products = append(in.Products, ap)
	}

	sales, err := st.Q.ListSalesUsers(ctx)
	if err != nil {
		return nil, nil, err
	}
	salesWA := map[string]string{}
	for _, s := range sales {
		salesWA[s.Name] = deref(s.WaNumber)
	}
	for _, it := range b.Items {
		if only != "" && it.ID != only && it.UUID.String() != only {
			continue
		}
		row, err := st.Q.GetDealer(ctx, &it.ID)
		if err != nil {
			return nil, nil, err
		}
		h := b.Data.Histories[it.UUID]
		d := &agents.Dealer{BoardItem: it, Contacts: h.Contacts, Memo: deref(row.Memo), OpenInvoices: views.OpenInvoices(h, b.Today), SalesWA: salesWA[it.Owner.Name]}
		if cs, err := st.Q.OpenCommitments(ctx, &it.UUID); err == nil {
			for _, c := range cs {
				d.Commitments = append(d.Commitments, agents.Commitment{ID: c.ID, Side: c.Side, Title: c.Title, Detail: deref(c.Detail), Status: c.Status,
					DueAt: c.DueAt, Invoice: deref(c.InvoiceNumber), SignalIDs: c.SignalIds})
			}
		} else {
			return nil, nil, err
		}
		if at, err := st.Q.LastSentFollowup(ctx, &it.UUID); err == nil && !at.IsZero() {
			d.LastFollowupAt = &at
		}
		for _, mt := range metrics.MonthlyTotals(h.Orders, b.Today, 6) {
			d.MonthlyOrders = append(d.MonthlyOrders, mt.Total)
		}
		id := it.UUID
		sigs, err := st.Q.DealerSignals(ctx, gen.DealerSignalsParams{DealerID: &id, Limit: 20})
		if err != nil {
			return nil, nil, err
		}
		for _, s := range sigs {
			var p struct {
				Conclusion string `json:"conclusion"`
			}
			_ = json.Unmarshal(s.Payload, &p)
			d.Signals = append(d.Signals, agents.Signal{ID: s.ID, Kind: s.Kind, At: s.OccurredAt, Text: deref(s.Summary), Conclusion: p.Conclusion})
		}
		in.Dealers = append(in.Dealers, d)
	}

	since := clock.Today(b.Today).AddDate(0, 0, -7)
	wa, err := st.Q.RecentInboundWA(ctx, since)
	if err != nil {
		return nil, nil, err
	}
	for _, m := range wa {
		if m.DealerID == nil || in.Dealer(*m.DealerID) == nil {
			continue
		}
		in.WA = append(in.WA, agents.WAMessage{SignalID: m.ID, DealerID: *m.DealerID, Contact: deref(m.ContactName), At: m.OccurredAt, Text: deref(m.Summary)})
	}

	in.StockSignals = map[string]uuid.UUID{}
	ss, err := st.Q.LatestStockSignals(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, s := range ss {
		head := strings.TrimPrefix(strings.SplitN(deref(s.Summary), ":", 2)[0], "Stok ")
		for _, it := range in.Stock {
			if head == it.Name+" "+it.Branch {
				in.StockSignals[it.Name+"|"+it.Branch] = s.ID
			}
		}
	}

	if only == "" {
		nn, err := st.Q.NewNumberThreads(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, n := range nn {
			var ident struct {
				Sources   []agents.IdentSource `json:"sources"`
				Potential string               `json:"potential"`
			}
			_ = json.Unmarshal(n.Sources, &ident)
			x := agents.NewNumber{ThreadID: n.ThreadID, WANumber: n.WaNumber, Name: deref(n.BestName), Org: deref(n.BestOrg), Score: int(deref(n.Score)),
				Sources: ident.Sources, Potential: ident.Potential, Sales: deref(n.Sales), Text: deref(n.LastText), At: n.LastAt}
			if n.SignalID != nil {
				x.SignalID = *n.SignalID
			}
			in.NewNumbers = append(in.NewNumbers, x)
		}
	}
	return in, b, nil
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
