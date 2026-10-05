package odoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

const sys = "odoo"

// Syncer maps Odoo records into Distri ARC (read-only; 02-architecture › Ingest).
type Syncer struct {
	st    *store.Store
	src   Source
	clock clock.Clock
	log   *slog.Logger
	m     *ctxMaps // state of the running sync (one run at a time per Syncer)
}

// NewSyncer builds a syncer.
func NewSyncer(st *store.Store, src Source, c clock.Clock, log *slog.Logger) *Syncer {
	return &Syncer{st: st, src: src, clock: c, log: log}
}

// Report summarises one run.
type Report struct {
	Full     bool           `json:"full"`
	Records  map[string]int `json:"records"`
	Signals  int            `json:"signals"`
	Dealers  []uuid.UUID    `json:"dealers"`
	Duration time.Duration  `json:"duration_ms"`
}

type ctxMaps struct {
	branch   map[int]string    // res.company id → branch
	sales    map[int]uuid.UUID // res.users id → sales_users id
	kat      map[int]string    // product.category id → KAT
	product  map[int]Record    // product.product id → record
	markup   map[int]float64   // pricelist id → markup %
	dealer   map[int]uuid.UUID // partner id → dealer id
	touched  map[uuid.UUID]bool
	signals  int
	maxWrite map[string]time.Time
	records  map[string]int
}

var digitsRe = regexp.MustCompile(`\d+`)

// Run syncs every model changed since its cursor (full: everything).
func (s *Syncer) Run(ctx context.Context, full bool) (rep Report, err error) {
	defer recoverRead(&err)
	start := time.Now()
	if full {
		if err := s.st.Q.ResetSyncState(ctx); err != nil {
			return Report{}, err
		}
	}
	m := &ctxMaps{branch: map[int]string{}, sales: map[int]uuid.UUID{}, kat: map[int]string{}, product: map[int]Record{}, markup: map[int]float64{},
		dealer: map[int]uuid.UUID{}, touched: map[uuid.UUID]bool{}, maxWrite: map[string]time.Time{}, records: map[string]int{}}

	// reference data is always read in full (small)
	s.m = m
	companies, err := s.src.SearchRead(ctx, "res.company", nil, []string{"id", "name"})
	if err != nil {
		return Report{}, fmt.Errorf("res.company: %w", err)
	}
	for _, c := range companies {
		m.branch[c.Int("id")] = strings.TrimPrefix(c.Str("name"), "GSI ")
	}
	users, err := s.src.SearchRead(ctx, "res.users", nil, []string{"id", "name"})
	if err != nil {
		return Report{}, err
	}
	for _, u := range users {
		uid := int32(u.Int("id"))
		if su, err := s.st.Q.GetSalesByOdooUser(ctx, &uid); err == nil {
			m.sales[u.Int("id")] = su.ID
		}
	}
	cmap, err := s.st.Q.ListCategoryMap(ctx)
	if err != nil {
		return Report{}, err
	}
	for _, c := range cmap {
		m.kat[int(c.OdooCategoryID)] = c.Kat
	}
	pls, err := s.src.SearchRead(ctx, "product.pricelist", nil, []string{"id", "name", "x_markup_pct"})
	if err != nil {
		return Report{}, err
	}
	for _, p := range pls {
		m.markup[p.Int("id")] = p.Float("x_markup_pct")
	}

	if err := s.syncProducts(ctx, m); err != nil {
		return Report{}, fmt.Errorf("products: %w", err)
	}
	if err := s.syncPartners(ctx, m); err != nil {
		return Report{}, fmt.Errorf("partners: %w", err)
	}
	if err := s.syncOrders(ctx, m); err != nil {
		return Report{}, fmt.Errorf("orders: %w", err)
	}
	if err := s.syncStock(ctx, m); err != nil {
		return Report{}, fmt.Errorf("stock: %w", err)
	}
	now := s.clock.Now()
	for model, n := range m.records {
		var lw *time.Time
		if t, ok := m.maxWrite[model]; ok {
			lw = &t
		}
		if err := s.st.Q.SetSyncState(ctx, gen.SetSyncStateParams{Model: model, LastWriteDate: lw, LastRunAt: &now, Records: int32(n)}); err != nil {
			return Report{}, err
		}
	}
	rep = Report{Full: full, Records: m.records, Signals: m.signals, Duration: time.Since(start) / time.Millisecond}
	for id := range m.touched {
		rep.Dealers = append(rep.Dealers, id)
	}
	return rep, nil
}

// since returns the incremental domain for a model.
func (s *Syncer) since(ctx context.Context, model string) []any {
	st, err := s.st.Q.GetSyncState(ctx, model)
	if err != nil || st.LastWriteDate == nil {
		return nil
	}
	return []any{[]any{"write_date", ">", FormatTime(*st.LastWriteDate)}}
}

func (m *ctxMaps) seen(model string, r Record) {
	m.records[model]++
	if t := r.Time("write_date"); t != nil && t.After(m.maxWrite[model]) {
		m.maxWrite[model] = *t
	}
}

func (s *Syncer) syncProducts(ctx context.Context, m *ctxMaps) error {
	all, err := s.src.SearchRead(ctx, "product.product", nil, []string{"id", "name", "default_code", "categ_id", "list_price", "standard_price", "write_date"})
	if err != nil {
		return err
	}
	changed := map[int]bool{}
	for _, r := range mustRead(s.src.SearchRead(ctx, "product.product", s.since(ctx, "product.product"), []string{"id"})) {
		changed[r.Int("id")] = true
	}
	for _, p := range all {
		m.product[p.Int("id")] = p
		if !changed[p.Int("id")] {
			continue
		}
		cid, _ := p.M2O("categ_id")
		kat := m.kat[cid]
		list := int64(math.Round(p.Float("list_price")))
		prices := map[string]int64{"A": list, "B": int64(math.Round(float64(list) * (1 + m.markup[2]/100))), "C": int64(math.Round(float64(list) * (1 + m.markup[3]/100)))}
		pj, _ := json.Marshal(prices)
		sku, src := p.Str("default_code"), SourceID("product.product", p.Int("id"))
		cid32 := int32(cid)
		if err := s.st.Q.UpsertProduct(ctx, gen.UpsertProductParams{Sku: &sku, Name: p.Str("name"), Category: &kat, OdooCategoryID: &cid32, ListPrice: list,
			Cost: int64(math.Round(p.Float("standard_price"))), Prices: pj, SourceSystem: ptr(sys), SourceID: &src, SourceWriteDate: p.Time("write_date")}); err != nil {
			return err
		}
		m.seen("product.product", p)
	}
	return nil
}

func slugify(name string) string {
	n := strings.ToLower(name)
	for _, p := range []string{"pt ", "cv ", "ud ", "toko "} {
		n = strings.TrimPrefix(n, p)
	}
	var b strings.Builder
	for _, r := range n {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *Syncer) syncPartners(ctx context.Context, m *ctxMaps) error {
	// dealers first (all known dealers are needed to attach orders), then contacts
	all, err := s.src.SearchRead(ctx, "res.partner", []any{[]any{"customer_rank", ">", 0}}, nil)
	if err != nil {
		return err
	}
	changed := map[int]bool{}
	for _, r := range mustRead(s.src.SearchRead(ctx, "res.partner", s.since(ctx, "res.partner"), []string{"id"})) {
		changed[r.Int("id")] = true
	}
	for _, p := range all {
		pid := p.Int("id")
		src := SourceID("res.partner", pid)
		if !changed[pid] {
			if row, err := s.st.Q.DealerIDBySource(ctx, gen.DealerIDBySourceParams{SourceSystem: ptr(sys), SourceID: &src}); err == nil {
				m.dealer[pid] = row.ID
			}
			continue
		}
		cid, _ := p.M2O("company_id")
		branch := m.branch[cid]
		uid, _ := p.M2O("user_id")
		var owner *uuid.UUID
		if id, ok := m.sales[uid]; ok {
			owner = &id
		}
		_, pl := p.M2O("property_product_pricelist")
		tier := strings.TrimSpace(strings.TrimPrefix(pl, "Tier"))
		if len(tier) != 1 {
			tier = "C"
		}
		_, term := p.M2O("property_payment_term_id")
		days := 0
		if d := digitsRe.FindString(term); d != "" {
			days, _ = strconv.Atoi(d)
		}
		_, seg := p.M2O("industry_id")
		slug, city := slugify(p.Str("name")), p.Str("city")
		id, err := s.st.Q.UpsertDealer(ctx, gen.UpsertDealerParams{Slug: &slug, Name: p.Str("name"), City: &city, Branch: branch, Tier: &tier, SegmentDesc: &seg,
			OwnerID: owner, CreditLimit: int64(p.Float("credit_limit")), PaymentTermsDays: int32(days), SourceSystem: ptr(sys), SourceID: &src, SourceWriteDate: p.Time("write_date")})
		if err != nil {
			return fmt.Errorf("dealer %s: %w", p.Str("name"), err)
		}
		m.dealer[pid] = id
		m.touched[id] = true
		m.seen("res.partner", p)
	}
	contacts, err := s.src.SearchRead(ctx, "res.partner", append([]any{[]any{"is_company", "=", false}}, s.since(ctx, "res.partner")...), nil)
	if err != nil {
		return err
	}
	for _, c := range contacts {
		parent, _ := c.M2O("parent_id")
		did, ok := m.dealer[parent]
		if !ok {
			continue
		}
		no := Digits(c.Str("mobile"))
		if no == "" {
			no = Digits(c.Str("phone"))
		}
		name, role, src := c.Str("name"), c.Str("function"), SourceID("res.partner", c.Int("id"))
		var wa *string
		if no != "" {
			wa = &no
		}
		if _, err := s.st.Q.UpsertContactFromOdoo(ctx, gen.UpsertContactFromOdooParams{DealerID: &did, Name: &name, Role: &role, WaNumber: wa, SourceSystem: ptr(sys), SourceID: &src}); err != nil {
			return fmt.Errorf("contact %s: %w", name, err)
		}
		m.touched[did] = true
		m.seen("res.partner", c)
	}
	return nil
}

// OrderState maps Odoo states to the order-to-cash phase of the glossary.
func OrderState(odooState string, pickedDone, invoiced, paid bool) string {
	switch {
	case odooState == "cancel":
		return "cancel"
	case odooState == "draft" || odooState == "sent":
		return "order"
	case paid:
		return "bayar"
	case invoiced:
		return "invoice"
	case pickedDone:
		return "kirim"
	default:
		return "siap"
	}
}

func ids(xs []int) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func (s *Syncer) syncOrders(ctx context.Context, m *ctxMaps) error {
	// orders affected by any change in SO, picking, invoice or payment are rebuilt as a whole
	affected := map[int]bool{}
	for _, r := range mustRead(s.src.SearchRead(ctx, "sale.order", s.since(ctx, "sale.order"), []string{"id"})) {
		affected[r.Int("id")] = true
	}
	for _, r := range mustRead(s.src.SearchRead(ctx, "stock.picking", s.since(ctx, "stock.picking"), []string{"sale_id"})) {
		if id, _ := r.M2O("sale_id"); id != 0 {
			affected[id] = true
		}
		m.seen("stock.picking", r)
	}
	changedMoves := mustRead(s.src.SearchRead(ctx, "account.move", append([]any{[]any{"move_type", "=", "out_invoice"}}, s.since(ctx, "account.move")...), nil))
	changedPays := mustRead(s.src.SearchRead(ctx, "account.payment", s.since(ctx, "account.payment"), nil))
	var payInv []int
	for _, p := range changedPays {
		payInv = append(payInv, p.Ints("reconciled_invoice_ids")...)
	}
	origins := map[string]bool{}
	for _, mv := range changedMoves {
		origins[mv.Str("invoice_origin")] = true
	}
	if len(payInv) > 0 {
		for _, mv := range mustRead(s.src.SearchRead(ctx, "account.move", []any{[]any{"id", "in", ids(payInv)}}, nil)) {
			origins[mv.Str("invoice_origin")] = true
		}
	}
	if len(origins) > 0 {
		var names []any
		for o := range origins {
			names = append(names, o)
		}
		for _, r := range mustRead(s.src.SearchRead(ctx, "sale.order", []any{[]any{"name", "in", names}}, []string{"id"})) {
			affected[r.Int("id")] = true
		}
	}
	if len(affected) == 0 {
		return nil
	}
	var soIDs []int
	for id := range affected {
		soIDs = append(soIDs, id)
	}
	orders := mustRead(s.src.SearchRead(ctx, "sale.order", []any{[]any{"id", "in", ids(soIDs)}}, nil))
	lines := mustRead(s.src.SearchRead(ctx, "sale.order.line", []any{[]any{"order_id", "in", ids(soIDs)}}, nil))
	picks := mustRead(s.src.SearchRead(ctx, "stock.picking", []any{[]any{"sale_id", "in", ids(soIDs)}}, nil))
	var soNames []any
	for _, o := range orders {
		soNames = append(soNames, o.Str("name"))
	}
	moves := mustRead(s.src.SearchRead(ctx, "account.move", []any{[]any{"move_type", "=", "out_invoice"}, []any{"invoice_origin", "in", soNames}}, nil))
	var moveIDs []int
	for _, mv := range moves {
		moveIDs = append(moveIDs, mv.Int("id"))
	}
	pays := mustRead(s.src.SearchRead(ctx, "account.payment", []any{[]any{"reconciled_invoice_ids", "in", ids(moveIDs)}}, nil))

	linesBy := map[int][]Record{}
	for _, l := range lines {
		oid, _ := l.M2O("order_id")
		linesBy[oid] = append(linesBy[oid], l)
	}
	pickBy := map[int][]Record{}
	for _, p := range picks {
		oid, _ := p.M2O("sale_id")
		pickBy[oid] = append(pickBy[oid], p)
	}
	moveBy := map[string][]Record{}
	for _, mv := range moves {
		moveBy[mv.Str("invoice_origin")] = append(moveBy[mv.Str("invoice_origin")], mv)
	}
	payBy := map[int][]Record{}
	for _, p := range pays {
		for _, inv := range p.Ints("reconciled_invoice_ids") {
			payBy[inv] = append(payBy[inv], p)
		}
	}

	return s.st.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		for _, o := range orders {
			pid, _ := o.M2O("partner_id")
			did, ok := m.dealer[pid]
			if !ok {
				continue
			}
			var shipped *time.Time
			for _, p := range pickBy[o.Int("id")] {
				if p.Str("state") == "done" {
					shipped = p.Time("date_done")
				}
			}
			var invoicedAt, paidAt *time.Time
			invoiced, paid := false, len(moveBy[o.Str("name")]) > 0
			for _, mv := range moveBy[o.Str("name")] {
				if mv.Str("state") != "posted" {
					paid = false
					continue
				}
				invoiced = true
				invoicedAt = mv.Time("create_date")
				if ps := mv.Str("payment_state"); ps != "paid" && ps != "in_payment" {
					paid = false
				}
				for _, p := range payBy[mv.Int("id")] {
					if t := p.Time("create_date"); t != nil && (paidAt == nil || t.After(*paidAt)) {
						paidAt = t
					}
				}
			}
			if !paid {
				paidAt = nil
			}
			var dl []domain.OrderLine
			for _, l := range linesBy[o.Int("id")] {
				prid, pname := l.M2O("product_id")
				cid, _ := m.product[prid].M2O("categ_id")
				dl = append(dl, domain.OrderLine{Product: pname, Category: m.kat[cid], Qty: int64(l.Float("product_uom_qty")), Price: int64(math.Round(l.Float("price_unit"))), Subtotal: int64(math.Round(l.Float("price_subtotal")))})
			}
			lj, _ := json.Marshal(dl)
			state := OrderState(o.Str("state"), shipped != nil, invoiced, paid)
			margin := math.Round(o.Float("margin_percent")*10000) / 100
			num, src := o.Str("name"), SourceID("sale.order", o.Int("id"))
			oid, err := q.UpsertOrder(ctx, gen.UpsertOrderParams{DealerID: &did, Number: &num, State: state, OrderedAt: o.Time("create_date"), ConfirmedAt: o.Time("date_order"),
				ShippedAt: shipped, InvoicedAt: invoicedAt, PaidAt: paidAt, Total: int64(math.Round(o.Float("amount_total"))), MarginPct: &margin, Lines: lj,
				CreatedBy: "odoo", SourceSystem: ptr(sys), SourceID: &src, SourceWriteDate: o.Time("write_date")})
			if err != nil {
				return fmt.Errorf("order %s: %w", num, err)
			}
			m.touched[did] = true
			if affected[o.Int("id")] {
				m.seen("sale.order", o)
			}
			if err := s.signal(ctx, q, "so", "sale.order", o, did, o.Time("date_order"), fmt.Sprintf("SO %s %s · %s", num, rp(o.Float("amount_total")), o.Str("state"))); err != nil {
				return err
			}
			for _, mv := range moveBy[o.Str("name")] {
				msrc := SourceID("account.move", mv.Int("id"))
				total := int64(math.Round(mv.Float("amount_total")))
				residual := int64(math.Round(mv.Float("amount_residual")))
				var invPaidAt *time.Time
				for _, p := range payBy[mv.Int("id")] {
					if d := p.Date("date"); d != nil && (invPaidAt == nil || d.After(*invPaidAt)) {
						invPaidAt = d
					}
				}
				if residual > 0 {
					invPaidAt = nil
				}
				name := mv.Str("name")
				state := mv.Str("state")
				iid, err := q.UpsertInvoice(ctx, gen.UpsertInvoiceParams{DealerID: &did, OrderID: &oid, Number: &name, IssuedAt: mv.Date("invoice_date"), DueAt: mv.Date("invoice_date_due"),
					Total: total, Paid: total - residual, PaidAt: invPaidAt, State: state, SourceSystem: ptr(sys), SourceID: &msrc, SourceWriteDate: mv.Time("write_date")})
				if err != nil {
					return fmt.Errorf("invoice %s: %w", name, err)
				}
				m.seen("account.move", mv)
				if err := s.signal(ctx, q, "invoice", "account.move", mv, did, mv.Date("invoice_date"), fmt.Sprintf("Invoice %s %s · jatuh tempo %s", name, rp(mv.Float("amount_total")), mv.Str("invoice_date_due"))); err != nil {
					return err
				}
				for _, p := range payBy[mv.Int("id")] {
					psrc := SourceID("account.payment", p.Int("id"))
					if err := q.UpsertPayment(ctx, gen.UpsertPaymentParams{DealerID: &did, InvoiceID: &iid, PaidAt: p.Date("date"), Amount: int64(math.Round(p.Float("amount"))), SourceSystem: ptr(sys), SourceID: &psrc}); err != nil {
						return err
					}
					m.seen("account.payment", p)
					if err := s.signal(ctx, q, "payment", "account.payment", p, did, p.Date("date"), fmt.Sprintf("Pembayaran %s untuk %s", rp(p.Float("amount")), name)); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

func (s *Syncer) syncStock(ctx context.Context, m *ctxMaps) error {
	quants, err := s.src.SearchRead(ctx, "stock.quant", s.since(ctx, "stock.quant"), nil)
	if err != nil {
		return err
	}
	today := clock.Today(s.clock.Now())
	return s.st.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		for _, r := range quants {
			prid, pname := r.M2O("product_id")
			p := m.product[prid]
			cid, _ := p.M2O("categ_id")
			comp, _ := r.M2O("company_id")
			branch := m.branch[comp]
			qty := int32(r.Float("quantity"))
			value := int64(math.Round(r.Float("value")))
			unit := int64(0)
			if qty > 0 {
				unit = value / int64(qty)
			}
			age := int32(0)
			if in := r.Time("in_date"); in != nil {
				age = int32(math.Round(today.Sub(clock.Today(*in)).Hours() / 24))
			}
			src := SourceID("stock.quant", r.Int("id"))
			if err := q.UpsertStockItem(ctx, gen.UpsertStockItemParams{Branch: branch, Sku: p.Str("default_code"), Name: pname, Category: m.kat[cid], Qty: qty, UnitCost: unit, Value: value,
				AgeDays: age, WeeklyVelocity: r.Float("x_weekly_velocity"), SourceSystem: ptr(sys), SourceID: &src, SourceWriteDate: r.Time("write_date")}); err != nil {
				return err
			}
			m.seen("stock.quant", r)
			if err := s.signal(ctx, q, "stock", "stock.quant", r, uuid.Nil, r.Time("write_date"), fmt.Sprintf("Stok %s %s: %d", pname, branch, qty)); err != nil {
				return err
			}
		}
		return nil
	})
}

// signal records one Odoo change as a signal (dedupe model:id:write_date).
func (s *Syncer) signal(ctx context.Context, q *gen.Queries, kind, model string, r Record, dealer uuid.UUID, at *time.Time, summary string) error {
	wd := r.Str("write_date")
	key := fmt.Sprintf("%s:%d:%s", model, r.Int("id"), wd)
	if ok, err := q.SignalExists(ctx, key); err != nil || ok {
		return err
	}
	when := s.clock.Now()
	if at != nil {
		when = *at
	}
	payload, _ := json.Marshal(map[string]any{"model": model, "id": r.Int("id"), "name": r.Str("name"), "write_date": wd, "source": "odoo"})
	p := gen.UpsertSignalParams{Kind: kind, OccurredAt: when, DedupeKey: key, Summary: &summary, Payload: payload}
	if dealer != uuid.Nil {
		p.DealerID = &dealer
	}
	if _, err := q.UpsertSignal(ctx, p); err != nil {
		return err
	}
	s.m.signals++
	return nil
}

func rp(v float64) string {
	if v >= 1e9 {
		return strings.Replace(fmt.Sprintf("Rp %.2f M", v/1e9), ".", ",", 1)
	}
	return fmt.Sprintf("Rp %.0f jt", v/1e6)
}

func ptr[T any](v T) *T { return &v }

func mustRead(r []Record, err error) []Record {
	if err != nil {
		panic(fmt.Errorf("odoo read: %w", err))
	}
	return r
}

// recoverRead converts a mustRead panic into the run's error.
func recoverRead(err *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*err = e
			return
		}
		*err = errors.New(fmt.Sprint(r))
	}
}
