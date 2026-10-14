package views

import (
	"math"
	"sort"
	"strings"

	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
)

// MarginRow is one product's margin with every input behind it, worst first — to reconcile with finance (14 Oct
// review, task 13: "margin per SKU cocok dengan hitungan keuangan pada 10 SKU uji").
type MarginRow struct {
	Product  string `json:"product"`
	Brand    string `json:"brand,omitempty"`
	Qty      int64  `json:"qty"`
	Revenue  int64  `json:"revenue"`   // Σ line values (Rp)
	AvgPrice int64  `json:"avg_price"` // revenue ÷ qty
	AvgCost  int64  `json:"avg_cost"`  // HPP per unit used for the margin, before rebate
	// FromStockPct: share of the quantity whose HPP came from today's stock (the invoice line had none)
	FromStockPct int      `json:"from_stock_pct"`
	NoCostPct    int      `json:"no_cost_pct"` // share of the quantity without any HPP (left out of the margin)
	RebatePct    float64  `json:"rebate_pct,omitempty"`
	MarginPct    *float64 `json:"margin_pct"` // after the brand's rebate
	StockHPP     int64    `json:"stock_hpp,omitempty"`
	Note         string   `json:"note,omitempty"`
}

// MarginCheck lists the products sold in the last days whose name or brand contains query (all when empty).
func (b *Board) MarginCheck(days int, query string, stock []domain.StockItem) []MarginRow {
	q := strings.ToLower(strings.TrimSpace(query))
	type acc struct {
		row                       MarginRow
		costed, costSum           float64
		fromStock, noCost, posQty int64
	}
	by := map[string]*acc{}
	for _, it := range b.Items {
		for _, o := range b.Data.Histories[it.UUID].Orders {
			if o.State == "cancel" || o.ConfirmedAt == nil || metrics.DaysBetween(*o.ConfirmedAt, b.Today) > days {
				continue
			}
			for _, l := range o.Lines {
				if q != "" && !strings.Contains(strings.ToLower(l.Product+" "+l.Brand), q) {
					continue
				}
				a := by[l.Product]
				if a == nil {
					a = &acc{row: MarginRow{Product: l.Product, Brand: l.Brand}}
					by[l.Product] = a
				}
				v := l.Value()
				a.row.Qty += l.Qty
				a.row.Revenue += v
				if l.Qty <= 0 || v <= 0 {
					continue // returns and bonus lines carry no margin
				}
				a.posQty += l.Qty
				switch {
				case l.Cost <= 0:
					a.noCost += l.Qty
					continue
				case l.CostSource == "stock":
					a.fromStock += l.Qty
				}
				a.costed += float64(v)
				a.costSum += float64(l.Cost * l.Qty)
			}
		}
	}
	hpp := map[string][2]float64{}
	for _, s := range stock {
		k := strings.ToLower(s.Name)
		w := math.Max(float64(s.Qty), 1)
		v := hpp[k]
		hpp[k] = [2]float64{v[0] + float64(s.UnitCost)*w, v[1] + w}
	}
	out := make([]MarginRow, 0, len(by))
	for _, a := range by {
		r := a.row
		if r.Qty > 0 {
			r.AvgPrice = r.Revenue / r.Qty
		}
		if a.posQty > 0 {
			r.FromStockPct = int(math.Round(float64(a.fromStock) / float64(a.posQty) * 100))
			r.NoCostPct = int(math.Round(float64(a.noCost) / float64(a.posQty) * 100))
		}
		if v, ok := hpp[strings.ToLower(r.Product)]; ok && v[1] > 0 {
			r.StockHPP = int64(math.Round(v[0] / v[1]))
		}
		r.RebatePct = b.Policies.Margin.Rebates[strings.ToLower(r.Brand)]
		if a.costed > 0 {
			qtyCosted := a.posQty - a.noCost
			if qtyCosted > 0 {
				r.AvgCost = int64(math.Round(a.costSum / float64(qtyCosted)))
			}
			m := math.Round((1-a.costSum*(1-r.RebatePct/100)/a.costed)*1000) / 10
			r.MarginPct = &m
		}
		switch {
		case r.MarginPct == nil:
			r.Note = "tanpa HPP: margin tidak dihitung"
		case *r.MarginPct < 0 && r.FromStockPct >= 50:
			r.Note = "margin minus dari HPP stok hari ini — minta HPP saat jual dari Accurate"
		case *r.MarginPct < 0 && r.StockHPP > 0 && r.StockHPP < r.AvgPrice:
			r.Note = "HPP faktur di atas harga jual, padahal HPP stok di bawahnya — cek kolom cost (PPN? satuan? total baris?)"
		case *r.MarginPct < 0:
			r.Note = "margin minus — cek HPP, diskon, dan rebate"
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		mi, mj := math.Inf(1), math.Inf(1)
		if out[i].MarginPct != nil {
			mi = *out[i].MarginPct
		}
		if out[j].MarginPct != nil {
			mj = *out[j].MarginPct
		}
		if mi != mj {
			return mi < mj
		}
		return out[i].Revenue > out[j].Revenue
	})
	return out
}
