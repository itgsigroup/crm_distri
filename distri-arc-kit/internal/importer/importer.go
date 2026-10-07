package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Importer stages and applies real data.
type Importer struct {
	St    *store.Store
	Clock clock.Clock
	Log   *slog.Logger
}

// StageReport counts one staged batch.
type StageReport struct {
	Entity  Entity   `json:"entity"`
	Rows    int      `json:"rows"`
	Staged  int      `json:"staged"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors"`
}

func key(e Entity, r Row) string {
	switch e {
	case Sales, Customers:
		return strings.ToLower(r.Get("code"))
	case Invoices:
		return r.Get("number")
	case Stock:
		return r.Get("sku") + "|" + strings.ToLower(r.Get("warehouse"))
	}
	return ""
}

// Stage stores rows as delivered. Invoice lines replace the lines of their invoices; a stock batch is a snapshot
// (rows of the same source missing from it are removed).
func (im Importer) Stage(ctx context.Context, e Entity, rows []Row, source, by string) (StageReport, error) {
	rep := StageReport{Entity: e, Rows: len(rows), Errors: []string{}}
	if _, ok := Contract[e]; !ok {
		return rep, fmt.Errorf("entitas %q tidak dikenal", e)
	}
	run, err := im.St.Q.StartImportRun(ctx, gen.StartImportRunParams{Source: source, Entity: string(e), StartedBy: &by})
	if err != nil {
		return rep, err
	}
	err = im.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		lines := map[string]int{} // invoice → next line number
		keys := make([]string, 0, len(rows))
		for i, r := range rows {
			if miss := Missing(e, r); len(miss) > 0 {
				rep.Skipped++
				if len(rep.Errors) < 20 {
					rep.Errors = append(rep.Errors, fmt.Sprintf("baris %d: kolom wajib kosong: %s", i+2, strings.Join(miss, ", ")))
				}
				continue
			}
			k := key(e, r)
			if e == InvoiceLines {
				inv := r.Get("invoice_number")
				lines[inv]++
				k = fmt.Sprintf("%s#%04d", inv, lines[inv])
			}
			data, _ := json.Marshal(r)
			if err := q.UpsertImportRow(ctx, gen.UpsertImportRowParams{Entity: string(e), Key: k, Data: data, Source: source}); err != nil {
				return err
			}
			keys = append(keys, k)
			rep.Staged++
		}
		switch {
		case e == InvoiceLines && len(lines) > 0: // delivered invoices replace their lines
			invs := make([]string, 0, len(lines))
			for inv := range lines {
				invs = append(invs, inv)
			}
			if _, err := q.DeleteInvoiceLinesNotIn(ctx, gen.DeleteInvoiceLinesNotInParams{Invoices: invs, Keys: keys}); err != nil {
				return err
			}
		case e == Stock && rep.Staged > 0: // a stock batch is a snapshot
			if _, err := q.DeleteImportRowsNotIn(ctx, gen.DeleteImportRowsNotInParams{Entity: string(e), Source: source, Keys: keys}); err != nil {
				return err
			}
		}
		return nil
	})
	status, msg := "done", (*string)(nil)
	if err != nil {
		status = "failed"
		s := err.Error()
		msg = &s
	}
	notes, _ := json.Marshal(map[string]any{"errors": rep.Errors})
	_ = im.St.Q.FinishImportRun(ctx, gen.FinishImportRunParams{ID: run, Status: status, Rows: int32(rep.Rows), Upserted: int32(rep.Staged), Skipped: int32(rep.Skipped), Error: msg, Notes: notes})
	return rep, err
}

// ApplyReport counts one transform.
type ApplyReport struct {
	Full      bool           `json:"full"`
	Sales     int            `json:"sales"`
	Dealers   int            `json:"dealers"`
	Invoices  int            `json:"invoices"`
	Stock     int            `json:"stock"`
	Products  int            `json:"products"`
	Unmapped  map[string]int `json:"unmapped"` // kind → distinct values without a target
	Skipped   int            `json:"skipped"`
	Errors    []string       `json:"errors"`
	DurationS float64        `json:"duration_s"`
}

// mapper resolves source values through data_mappings and counts what it saw.
type mapper struct {
	target map[string]map[string]string
	seen   map[string]map[string]int
}

func (m *mapper) get(kind, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if m.seen[kind] == nil {
		m.seen[kind] = map[string]int{}
	}
	m.seen[kind][raw]++
	t := m.target[kind][raw]
	return t, t != ""
}

func (m *mapper) set(kind, raw, target string) {
	if m.target[kind] == nil {
		m.target[kind] = map[string]string{}
	}
	m.target[kind][raw] = target
}

func digitsOrNil(s string) *string {
	d := wa.Digits(s)
	if len(d) < 9 {
		return nil
	}
	return &d
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func rp(v int64) string {
	if v >= 1_000_000_000 {
		return strings.Replace(fmt.Sprintf("Rp %.2f M", float64(v)/1e9), ".", ",", 1)
	}
	return fmt.Sprintf("Rp %s jt", strings.Replace(strconv.FormatFloat(math.Round(float64(v)/1e5)/10, 'f', -1, 64), ".", ",", 1))
}

// Apply transforms staged rows into Distri ARC's tables. Full re-applies everything (after a mapping change);
// otherwise only invoices staged since the last apply are rebuilt (sales, customers and stock always).
func (im Importer) Apply(ctx context.Context, full bool, by string) (ApplyReport, error) {
	t0 := time.Now()
	rep := ApplyReport{Full: full, Unmapped: map[string]int{}, Errors: []string{}}
	run, err := im.St.Q.StartImportRun(ctx, gen.StartImportRunParams{Source: "transform", Entity: "apply", StartedBy: &by})
	if err != nil {
		return rep, err
	}
	since := time.Unix(0, 0)
	if !full {
		if t, err := im.St.Q.LastApplyStart(ctx); err == nil {
			since = t
		}
	}
	err = im.apply(ctx, since, &rep)
	rep.DurationS = math.Round(time.Since(t0).Seconds()*10) / 10
	status, msg := "done", (*string)(nil)
	if err != nil {
		status = "failed"
		s := err.Error()
		msg = &s
	}
	notes, _ := json.Marshal(rep)
	unmapped := 0
	for _, n := range rep.Unmapped {
		unmapped += n
	}
	_ = im.St.Q.FinishImportRun(ctx, gen.FinishImportRunParams{ID: run, Status: status, Rows: int32(rep.Dealers + rep.Invoices + rep.Stock), Upserted: int32(rep.Dealers + rep.Invoices + rep.Stock + rep.Sales),
		Skipped: int32(rep.Skipped), Unmapped: int32(unmapped), Error: msg, Notes: notes})
	if err != nil {
		return rep, err
	}
	if _, err := dealersvc.New(im.St, im.Clock).Recompute(ctx); err != nil {
		return rep, fmt.Errorf("recompute: %w", err)
	}
	return rep, nil
}

func (im Importer) rows(ctx context.Context, e Entity) ([]Row, error) {
	raw, err := im.St.Q.ListImportRows(ctx, string(e))
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(raw))
	for _, x := range raw {
		var r Row
		if json.Unmarshal(x.Data, &r) == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func (im Importer) apply(ctx context.Context, since time.Time, rep *ApplyReport) error {
	m := &mapper{target: map[string]map[string]string{}, seen: map[string]map[string]int{}}
	maps, err := im.St.Q.ListMappings(ctx, "")
	if err != nil {
		return err
	}
	for _, x := range maps {
		if x.Target != nil {
			m.set(x.Kind, x.SourceValue, *x.Target)
		}
	}
	profiles, err := im.St.Q.SalesProfilesForMatch(ctx)
	if err != nil {
		return err
	}
	byName := map[string]string{}
	for _, p := range profiles {
		byName[strings.ToLower(p.Name)] = p.ID.String()
		if p.ExternalName != "" {
			byName[strings.ToLower(p.ExternalName)] = p.ID.String()
		}
	}
	autoMapped := map[[2]string]string{}
	salesID := func(raw string) *uuid.UUID {
		if t, ok := m.get("sales", raw); ok {
			if id, err := uuid.Parse(t); err == nil {
				return &id
			}
		}
		if t, ok := byName[strings.ToLower(strings.TrimSpace(raw))]; ok && raw != "" { // same name as a profile: map it
			m.set("sales", strings.TrimSpace(raw), t)
			autoMapped[[2]string{"sales", strings.TrimSpace(raw)}] = t
			id, _ := uuid.Parse(t)
			return &id
		}
		return nil
	}
	branchOf := func(raw, fallback string) string {
		if t, ok := m.get("branch", raw); ok {
			return t
		}
		if strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
		return fallback
	}
	kat := func(raw string) string {
		if t, ok := m.get("category", raw); ok && domain.CategoryIndex(t) >= 0 {
			return t
		}
		return ""
	}

	// 1. sales profiles
	sales, err := im.rows(ctx, Sales)
	if err != nil {
		return err
	}
	salesBranch := map[string]string{}
	err = im.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		for _, r := range sales {
			code := r.Get("code")
			branch := branchOf(r.Get("branch"), "Semua cabang")
			id, err := q.UpsertImportedSalesUser(ctx, gen.UpsertImportedSalesUserParams{Name: r.Get("name"), Branch: branch, WaNumber: digitsOrNil(r.Get("wa_number")),
				Email: strp(r.Get("email")), ExternalName: &code, SourceID: &code})
			if err != nil {
				return fmt.Errorf("sales %s: %w", code, err)
			}
			for _, raw := range []string{code, r.Get("name")} {
				if _, ok := m.target["sales"][raw]; !ok && raw != "" {
					m.set("sales", raw, id.String())
					autoMapped[[2]string{"sales", raw}] = id.String()
				}
			}
			salesBranch[id.String()] = branch
			rep.Sales++
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 2. customers → dealers (+ PIC contact)
	customers, err := im.rows(ctx, Customers)
	if err != nil {
		return err
	}
	existing, err := im.St.Q.ImportedDealerIDs(ctx)
	if err != nil {
		return err
	}
	dealerID := map[string]uuid.UUID{}
	for _, x := range existing {
		if x.SourceID != nil {
			dealerID[*x.SourceID] = x.ID
		}
	}
	terms := map[string]int{}
	custBranch := map[string]string{}
	today := clock.Today(im.Clock.Now())
	usedSlug := map[string]bool{}
	var slugs []string
	if err := im.St.Pool.QueryRow(ctx, "select coalesce(array_agg(slug), '{}') from dealers where slug is not null and (source_system is distinct from 'import')").Scan(&slugs); err == nil {
		for _, s := range slugs {
			usedSlug[s] = true
		}
	}
	err = im.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		for _, r := range customers {
			code := strings.ToLower(r.Get("code"))
			ctype := CustomerType(r.Get("customer_type"))
			if ctype == "" {
				if t, ok := m.get("ctype", r.Get("customer_type")); ok && (t == "reseller" || t == "si") {
					ctype = t
				} else {
					ctype = "reseller"
				}
			}
			owner := salesID(r.Get("sales"))
			fallback := "Tanpa cabang"
			if owner != nil && salesBranch[owner.String()] != "" && salesBranch[owner.String()] != "Semua cabang" {
				fallback = salesBranch[owner.String()]
			}
			branch := branchOf(r.Get("branch"), fallback)
			custBranch[code] = branch
			var tier *string
			if t := strings.ToUpper(r.Get("tier")); t == "A" || t == "B" || t == "C" {
				tier = &t
			}
			days := int32(30)
			if v := int32(Number(r.Get("payment_terms_days"))); v > 0 {
				days = v
			}
			terms[code] = int(days)
			slug := Slug(r.Get("name"))
			if _, known := dealerID[code]; !known {
				for usedSlug[slug] {
					slug = Slug(r.Get("name")) + "-" + Slug(code)
					if usedSlug[slug] {
						slug += "-" + strconv.Itoa(len(usedSlug))
					}
				}
			}
			usedSlug[slug] = true
			id, err := q.ImportDealer(ctx, gen.ImportDealerParams{Slug: &slug, Name: r.Get("name"), City: strp(r.Get("city")), Branch: branch, Tier: tier, SegmentDesc: strp(r.Get("segment")),
				OwnerID: owner, CreditLimit: Rupiah(r.Get("credit_limit")), PaymentTermsDays: days, CustomerType: ctype, Phone: digitsOrNil(r.Get("phone")), SourceID: &code})
			if err != nil {
				return fmt.Errorf("pelanggan %s: %w", code, err)
			}
			dealerID[code] = id
			if wa := digitsOrNil(r.Get("phone")); wa != nil {
				name := r.Get("pic_name")
				if name == "" {
					name = r.Get("name")
				}
				src := "cust:" + code
				if err := q.ImportContact(ctx, gen.ImportContactParams{DealerID: &id, Name: &name, WaNumber: wa, SourceID: &src}); err != nil {
					return fmt.Errorf("kontak %s: %w", code, err)
				}
			}
			rep.Dealers++
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 3. invoices (+ order with lines, margin, payment, signals)
	lineRows, err := im.rows(ctx, InvoiceLines)
	if err != nil {
		return err
	}
	stockRows, err := im.rows(ctx, Stock)
	if err != nil {
		return err
	}
	stockCost := map[string]int64{} // HPP per SKU from the stock snapshot (margin when a line has no cost)
	for _, r := range stockRows {
		if c := Rupiah(r.Get("unit_cost")); c > 0 {
			stockCost[r.Get("sku")] = c
		}
	}
	sold90 := map[[2]string]float64{} // units sold in the last 90 days per SKU × branch (velocity when the stock has no sold_90d)
	lines := map[string][]Row{}
	for _, r := range lineRows {
		inv := r.Get("invoice_number")
		lines[inv] = append(lines[inv], r)
	}
	changed := map[string]bool{}
	if since.Unix() > 0 {
		recent, err := im.St.Q.ListImportRowsSince(ctx, gen.ListImportRowsSinceParams{Entity: string(InvoiceLines), UpdatedAt: since})
		if err != nil {
			return err
		}
		for _, x := range recent {
			changed[strings.SplitN(x.Key, "#", 2)[0]] = true
		}
	}
	invoices, err := im.rows(ctx, Invoices)
	if err != nil {
		return err
	}
	var recentInv map[string]bool
	if since.Unix() > 0 {
		recentInv = map[string]bool{}
		rs, err := im.St.Q.ListImportRowsSince(ctx, gen.ListImportRowsSinceParams{Entity: string(Invoices), UpdatedAt: since})
		if err != nil {
			return err
		}
		for _, x := range rs {
			recentInv[x.Key] = true
		}
	}
	type product struct {
		name, kat string
		price     int64
		at        time.Time
	}
	products := map[string]product{}
	now := im.Clock.Now()
	for start := 0; start < len(invoices); start += 500 {
		end := min(start+500, len(invoices))
		err = im.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
			for _, r := range invoices[start:end] {
				num := r.Get("number")
				cust := strings.ToLower(r.Get("customer_code"))
				did, ok := dealerID[cust]
				date := Date(r.Get("date"))
				if !ok || date == nil {
					rep.Skipped++
					if len(rep.Errors) < 20 {
						rep.Errors = append(rep.Errors, fmt.Sprintf("faktur %s: pelanggan %q tidak ada atau tanggal tidak terbaca", num, r.Get("customer_code")))
					}
					continue
				}
				var ls []domain.OrderLine
				var costed, costSum int64 // revenue and HPP of the lines whose HPP is known
				recent := metrics.DaysBetween(*date, today) <= 90
				for _, l := range lines[num] {
					qty := int64(math.Round(Number(l.Get("qty"))))
					price := Rupiah(l.Get("price"))
					amount := Rupiah(l.Get("amount"))
					if amount == 0 {
						amount = qty * price
					}
					k := kat(l.Get("category"))
					sku := l.Get("sku")
					if sku == "" {
						sku = l.Get("product")
					}
					cost := Rupiah(l.Get("cost"))
					if cost == 0 {
						cost = stockCost[sku]
					}
					ls = append(ls, domain.OrderLine{Product: l.Get("product"), Category: k, Qty: qty, Price: price, Subtotal: amount, Cost: cost})
					if cost > 0 && amount > 0 {
						costed += amount
						costSum += cost * qty
					}
					if recent && qty > 0 {
						// same key as the stock snapshot: mapped warehouse, else the raw warehouse, else the sale's branch
						br, ok := m.get("warehouse", l.Get("warehouse"))
						if !ok {
							br = strings.TrimSpace(l.Get("warehouse"))
						}
						if br == "" {
							br = branchOf(r.Get("branch"), custBranch[cust])
						}
						sold90[[2]string{sku, br}] += float64(qty)
					}
					if p, seen := products[sku]; !seen || date.After(p.at) {
						products[sku] = product{name: l.Get("product"), kat: k, price: price, at: *date}
					}
				}
				if recentInv != nil && !recentInv[num] && !changed[num] {
					continue // unchanged since the last apply
				}
				total := Rupiah(r.Get("total"))
				residual := Rupiah(r.Get("residual"))
				if strings.TrimSpace(r.Get("residual")) == "" && Date(r.Get("paid_date")) == nil {
					residual = total
				}
				paid := max(total-residual, 0)
				due := Date(r.Get("due_date"))
				if due == nil {
					d := date.AddDate(0, 0, max(terms[cust], 0))
					due = &d
				}
				var paidAt *time.Time
				if paid > 0 {
					paidAt = Date(r.Get("paid_date"))
				}
				state, istate := "invoice", "posted"
				var orderPaid *time.Time
				if residual <= 0 && total > 0 {
					state, istate, orderPaid = "bayar", "paid", paidAt
				}
				lj, _ := json.Marshal(ls)
				osrc, isrc := "order:"+num, "inv:"+num
				var margin *float64
				if costed > 0 {
					v := math.Round((1-float64(costSum)/float64(costed))*10000) / 100
					margin = &v
				}
				oid, err := q.UpsertOrder(ctx, gen.UpsertOrderParams{DealerID: &did, Number: &num, State: state, OrderedAt: date, ConfirmedAt: date, InvoicedAt: date, PaidAt: orderPaid,
					Total: total, MarginPct: margin, Lines: lj, CreatedBy: "import", SourceSystem: strp("import"), SourceID: &osrc, SourceWriteDate: &now})
				if err != nil {
					return fmt.Errorf("order %s: %w", num, err)
				}
				iid, err := q.UpsertInvoice(ctx, gen.UpsertInvoiceParams{DealerID: &did, OrderID: &oid, Number: &num, IssuedAt: date, DueAt: due, Total: total, Paid: paid, PaidAt: paidAt,
					State: istate, SourceSystem: strp("import"), SourceID: &isrc, SourceWriteDate: &now})
				if err != nil {
					return fmt.Errorf("faktur %s: %w", num, err)
				}
				if err := signal(ctx, q, "so", "import:order:"+num, did, *date, fmt.Sprintf("SO %s %s · %d barang", num, rp(total), len(ls))); err != nil {
					return err
				}
				if err := signal(ctx, q, "invoice", "import:invoice:"+num, did, *date, fmt.Sprintf("Invoice %s %s · jatuh tempo %s", num, rp(total), clock.DayMonth(*due))); err != nil {
					return err
				}
				if paid > 0 && paidAt != nil {
					psrc := "pay:" + num
					if err := q.UpsertPayment(ctx, gen.UpsertPaymentParams{DealerID: &did, InvoiceID: &iid, PaidAt: paidAt, Amount: paid, SourceSystem: strp("import"), SourceID: &psrc}); err != nil {
						return err
					}
					if err := signal(ctx, q, "payment", "import:payment:"+num, did, *paidAt, fmt.Sprintf("Pembayaran %s untuk %s", rp(paid), num)); err != nil {
						return err
					}
				}
				rep.Invoices++
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// 4. stock snapshot: warehouses → branches, aggregated per SKU and branch
	stock := stockRows
	type agg struct {
		name, kat  string
		qty, value int64
		cost       int64
		age        int
		sold90     float64
		hasSold    bool
	}
	byKey := map[[2]string]*agg{}
	for _, r := range stock {
		branch, ok := m.get("warehouse", r.Get("warehouse"))
		if !ok {
			branch = r.Get("warehouse")
		}
		k := [2]string{r.Get("sku"), branch}
		a := byKey[k]
		if a == nil {
			a = &agg{name: r.Get("product"), kat: kat(r.Get("category"))}
			byKey[k] = a
		}
		qty := int64(math.Round(Number(r.Get("qty"))))
		cost := Rupiah(r.Get("unit_cost"))
		value := Rupiah(r.Get("value"))
		if value == 0 {
			value = qty * cost
		}
		a.qty += qty
		a.value += value
		if cost > 0 {
			a.cost = cost
		}
		age := int(Number(r.Get("age_days")))
		if strings.TrimSpace(r.Get("age_days")) == "" {
			if d := Date(r.Get("received_date")); d != nil {
				age = max(metrics.DaysBetween(*d, today), 0)
			}
		}
		a.age = max(a.age, age)
		if strings.TrimSpace(r.Get("sold_90d")) != "" {
			a.sold90 += Number(r.Get("sold_90d"))
			a.hasSold = true
		}
		sku := r.Get("sku")
		if p, seen := products[sku]; !seen {
			products[sku] = product{name: r.Get("product"), kat: a.kat}
		} else if p.kat == "" {
			p.kat = a.kat
			products[sku] = p
		}
	}
	keys := make([][2]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
	err = im.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		if len(keys) == 0 {
			return nil
		}
		if err := q.DeleteImportedStock(ctx); err != nil {
			return err
		}
		for _, k := range keys {
			a := byKey[k]
			if !a.hasSold { // no sold_90d from the source: units sold on invoices in the last 90 days
				a.sold90 = sold90[k]
			}
			if a.qty <= 0 && a.sold90 == 0 {
				continue
			}
			unit := a.cost
			if unit == 0 && a.qty > 0 {
				unit = a.value / a.qty
			}
			src := k[0]
			if err := q.UpsertStockItem(ctx, gen.UpsertStockItemParams{Branch: k[1], Sku: k[0], Name: a.name, Category: a.kat, Qty: int32(a.qty), UnitCost: unit, Value: a.value,
				AgeDays: int32(a.age), WeeklyVelocity: math.Round(a.sold90/13*100) / 100, SourceSystem: strp("import"), SourceID: &src, SourceWriteDate: &now}); err != nil {
				return fmt.Errorf("stok %s: %w", k[0], err)
			}
			if err := signal(ctx, q, "stock", "import:stock:"+k[0]+":"+k[1], uuid.Nil, now, fmt.Sprintf("Stok %s %s: %d", a.name, k[1], a.qty)); err != nil {
				return err
			}
			rep.Stock++
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 5. product catalogue (AI Order matches WhatsApp requests against it)
	costBySKU := map[string]int64{}
	for k, a := range byKey {
		if a.cost > 0 {
			costBySKU[k[0]] = a.cost
		}
	}
	err = im.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		for sku, p := range products {
			if sku == "" || p.name == "" {
				continue
			}
			var cat *string
			if p.kat != "" {
				cat = &p.kat
			}
			if err := q.ImportProduct(ctx, gen.ImportProductParams{Sku: &sku, Name: p.name, Category: cat, ListPrice: p.price, Cost: costBySKU[sku]}); err != nil {
				return err
			}
			rep.Products++
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 6. mappings: what was seen (unmapped values appear in Pengaturan) and what was matched by name
	return im.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		for kind, vals := range m.seen {
			for raw, n := range vals {
				if err := q.SeenMapping(ctx, gen.SeenMappingParams{Kind: kind, SourceValue: raw, Seen: int32(n)}); err != nil {
					return err
				}
				if m.target[kind][raw] == "" {
					rep.Unmapped[kind]++
				}
			}
		}
		for k, t := range autoMapped {
			if err := q.SetMapping(ctx, gen.SetMappingParams{Kind: k[0], SourceValue: k[1], Target: &t, UpdatedBy: strp("impor (nama sama)")}); err != nil {
				return err
			}
		}
		return nil
	})
}

// signal records an imported fact as a signal (one per source key; the agents cite it as provenance).
func signal(ctx context.Context, q *gen.Queries, kind, key string, dealer uuid.UUID, at time.Time, summary string) error {
	payload, _ := json.Marshal(map[string]any{"source": "import", "text": summary})
	p := gen.UpsertSignalParams{Kind: kind, OccurredAt: at, DedupeKey: key, Summary: &summary, Payload: payload}
	if dealer != uuid.Nil {
		p.DealerID = &dealer
	}
	_, err := q.UpsertSignal(ctx, p)
	return err
}

// ErrNoData means nothing was staged yet.
var ErrNoData = errors.New("belum ada data yang diimpor")
