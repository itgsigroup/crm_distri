package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// exportOdoo writes the same sample data in Odoo 17 model shapes (db/seed/odoo/<model>.json). internal/odoo.Fake
// serves these files as search_read results, so `arc ctl odoo sync --full` with ODOO_MODE=fake rebuilds exactly
// the seeded dealers, orders, invoices, payments and stock — with identical source ids, hence idempotent.
func exportOdoo(dir string, sales []SalesUser, dealers []*Dealer, stock []Stock) {
	must(os.MkdirAll(dir, 0o755))
	wd := odooTime(anchor.Add(6 * time.Hour))
	m2o := func(id int, name string) []any { return []any{id, name} }
	branches := []string{"Semarang", "Yogyakarta", "Surabaya", "Jakarta"}
	company := map[string]int{}
	var companies []map[string]any
	for i, b := range branches {
		company[b] = i + 1
		companies = append(companies, map[string]any{"id": i + 1, "name": "GSI " + b, "write_date": wd})
	}
	comp := func(b string) []any { return m2o(company[b], "GSI "+b) }

	userID := map[string]int{}
	var users []map[string]any
	for _, s := range sales {
		userID[s.Key] = s.OdooUser
		users = append(users, map[string]any{"id": s.OdooUser, "name": s.Name, "login": s.Email, "mobile": s.WANumber, "write_date": wd})
	}

	cats := []struct {
		id        int
		name, kat string
	}{{11, "CCTV / Kamera & NVR", "Kamera & NVR"}, {12, "Storage / HDD", "HDD & storage"}, {13, "Jaringan / Kabel & PoE", "Kabel & PoE"}, {14, "LED / Modul", "Modul LED"}, {15, "Fire Alarm", "Fire alarm"}, {16, "Aksesoris", "Aksesoris"}}
	catID := map[string]int{}
	var categories []map[string]any
	for _, c := range cats {
		catID[c.kat] = c.id
		categories = append(categories, map[string]any{"id": c.id, "name": c.name, "complete_name": "All / " + c.name, "write_date": wd})
	}
	pricelists := []map[string]any{{"id": 1, "name": "Tier A", "x_markup_pct": 0.0}, {"id": 2, "name": "Tier B", "x_markup_pct": 3.0}, {"id": 3, "name": "Tier C", "x_markup_pct": 6.0}}
	plID := map[string]int{"A": 1, "B": 2, "C": 3}
	terms := []map[string]any{{"id": 1, "name": "30 Hari"}, {"id": 2, "name": "Tunai"}}

	// products: the sales catalog plus stock SKUs
	productID := map[string]int{}
	var products []map[string]any
	names := make([]string, 0, len(catalog))
	for n := range catalog {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		p := catalog[n]
		id := 7001 + i
		productID[n] = id
		products = append(products, map[string]any{"id": id, "name": n, "default_code": strings.ToUpper(strings.ReplaceAll(n, " ", "-")), "categ_id": m2o(catID[p.cat], cats[catID[p.cat]-11].name),
			"list_price": p.price, "standard_price": math.Round(float64(p.price) * (1 - p.margin/100)), "write_date": wd})
	}
	for _, s := range stock {
		if _, ok := productID["sku:"+s.SKU]; ok {
			continue
		}
		id := 7101 + len(productID)
		productID["sku:"+s.SKU] = id
		products = append(products, map[string]any{"id": id, "name": s.Name, "default_code": s.SKU, "categ_id": m2o(catID[s.Category], cats[catID[s.Category]-11].name),
			"list_price": math.Round(float64(s.UnitCost) * 1.12), "standard_price": s.UnitCost, "write_date": wd})
	}

	var partners, orders, lines, pickings, moves, payments []map[string]any
	lineSeq, pickSeq := 90001, 60001
	for _, d := range dealers {
		pid := odooID(d.OdooID)
		term := any(m2o(1, "30 Hari"))
		if d.TermsDays == 0 {
			term = m2o(2, "Tunai")
		}
		partners = append(partners, map[string]any{"id": pid, "name": d.Name, "is_company": true, "customer_rank": 1, "city": d.City,
			"company_id": comp(d.Branch), "user_id": m2o(userID[d.Owner], title(d.Owner)), "property_product_pricelist": m2o(plID[d.Tier], "Tier "+d.Tier),
			"property_payment_term_id": term, "credit_limit": d.CreditLimit, "industry_id": m2o(pid, d.SegmentDesc), "parent_id": false, "write_date": wd})
		for _, c := range d.Contacts {
			partners = append(partners, map[string]any{"id": odooID(c.OdooID), "name": c.Name, "is_company": false, "customer_rank": 0, "parent_id": m2o(pid, d.Name),
				"function": c.Role, "mobile": "+" + c.WANumber, "company_id": comp(d.Branch), "write_date": wd})
		}
		invByOrder := map[string]*Invoice{}
		for _, inv := range d.Invoices {
			invByOrder[inv.Order] = inv
		}
		for _, o := range d.Orders {
			oid := odooID(o.OdooID)
			var lineIDs []int
			for _, l := range o.Lines {
				lineIDs = append(lineIDs, lineSeq)
				lines = append(lines, map[string]any{"id": lineSeq, "order_id": m2o(oid, o.Number), "product_id": m2o(productID[l.Product], l.Product),
					"product_uom_qty": l.Qty, "price_unit": l.Price, "price_subtotal": l.Subtotal, "write_date": wd})
				lineSeq++
			}
			inv := invByOrder[o.Number]
			so := map[string]any{"id": oid, "name": o.Number, "partner_id": m2o(pid, d.Name), "user_id": m2o(userID[d.Owner], title(d.Owner)), "company_id": comp(d.Branch),
				"state": "sale", "create_date": odooTime(o.OrderedAt), "date_order": odooTime(o.ConfirmedAt), "amount_total": o.Total, "margin_percent": o.MarginPct / 100,
				"order_line": lineIDs, "picking_ids": []int{pickSeq}, "invoice_ids": []int{}, "write_date": wd}
			pickings = append(pickings, map[string]any{"id": pickSeq, "name": fmt.Sprintf("WH/OUT/%05d", pickSeq-60000), "origin": o.Number, "sale_id": m2o(oid, o.Number),
				"state": "done", "date_done": odooTime(o.ShippedAt), "write_date": wd})
			pickSeq++
			if inv != nil {
				mid := odooID(inv.OdooID)
				so["invoice_ids"] = []int{mid}
				state := "not_paid"
				if inv.Paid >= inv.Total {
					state = "paid"
				} else if inv.Paid > 0 {
					state = "partial"
				}
				moves = append(moves, map[string]any{"id": mid, "name": inv.Number, "partner_id": m2o(pid, d.Name), "move_type": "out_invoice", "state": "posted",
					"create_date": odooTime(o.InvoicedAt), "invoice_date": inv.IssuedAt, "invoice_date_due": inv.DueAt, "amount_total": inv.Total,
					"amount_residual": inv.Total - inv.Paid, "payment_state": state, "invoice_origin": o.Number, "company_id": comp(d.Branch), "write_date": wd})
			}
			orders = append(orders, so)
		}
		for _, p := range d.Payments {
			var paidAt time.Time
			for _, o := range d.Orders {
				if inv := invByOrder[o.Number]; inv != nil && inv.Number == p.Invoice && o.PaidAt != nil {
					paidAt = *o.PaidAt
				}
			}
			payments = append(payments, map[string]any{"id": odooID(p.OdooID), "partner_id": m2o(pid, d.Name), "date": p.PaidAt, "create_date": odooTime(paidAt),
				"amount": p.Amount, "state": "paid", "reconciled_invoice_ids": []int{invNum(p.Invoice)}, "write_date": wd})
		}
	}
	var quants []map[string]any
	for _, s := range stock {
		in := anchor.AddDate(0, 0, -s.AgeDays)
		quants = append(quants, map[string]any{"id": odooID(s.OdooID), "product_id": m2o(productID["sku:"+s.SKU], s.Name), "location_id": m2o(company[s.Branch]*10, strings.ToUpper(s.Branch[:3])+"/Stock"),
			"company_id": comp(s.Branch), "quantity": s.Qty, "value": s.Qty * s.UnitCost, "in_date": odooTime(in), "x_weekly_velocity": s.WeeklyVelocity, "write_date": wd})
	}
	for model, rows := range map[string]any{
		"res.company": companies, "res.users": users, "product.category": categories, "product.pricelist": pricelists, "account.payment.term": terms,
		"product.product": products, "res.partner": partners, "sale.order": orders, "sale.order.line": lines, "stock.picking": pickings,
		"account.move": moves, "account.payment": payments, "stock.quant": quants,
	} {
		write(filepath.Join(dir, model+".json"), rows)
	}
}

// odooTime renders a timestamp the way Odoo's JSON-RPC does: UTC "2006-01-02 15:04:05".
func odooTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

func odooID(src string) int {
	var n int
	_, after, _ := strings.Cut(src, ":")
	if _, err := fmt.Sscanf(after, "%d", &n); err != nil {
		panic(src)
	}
	return n
}

func title(key string) string { return strings.ToUpper(key[:1]) + key[1:] }
