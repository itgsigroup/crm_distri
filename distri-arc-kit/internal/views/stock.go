package views

import (
	"math"
	"sort"

	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
)

// AgingItem is a stock item older than policy.aging_days with the dealers it fits.
type AgingItem struct {
	domain.StockItem
	Candidates  []metrics.PushCandidate `json:"candidates"`
	DueThisWeek int                     `json:"due_this_week"`
}

// StockAging returns aging stock (oldest value first) with push candidates; includes items close to the
// threshold (≥ 75% of aging_days) that the mockup lists as "menua".
func (b *Board) StockAging(stock []domain.StockItem, branch string) []AgingItem {
	views := DealerViews(b.Items)
	var out []AgingItem
	for _, s := range stock {
		if branch != "" && s.Branch != branch {
			continue
		}
		if float64(s.AgeDays) < 0.75*float64(b.Policies.Stock.AgingDays) {
			continue
		}
		ai := AgingItem{StockItem: s, Candidates: metrics.PushCandidates(s, views, b.Policies)}
		for _, c := range ai.Candidates {
			if c.DueIn != nil && *c.DueIn >= 0 && *c.DueIn <= 7 {
				ai.DueThisWeek++
			}
		}
		out = append(out, ai)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}

// CriticalItem is stock that runs out before the next cycle.
type CriticalItem struct {
	domain.StockItem
	DaysLeft      float64     `json:"days_left"`
	Dependents    int         `json:"dependents"`
	OtherBranches []BranchQty `json:"other_branches"`
}

// BranchQty is the same SKU in another branch (transfer source).
type BranchQty struct {
	Branch string `json:"branch"`
	Qty    int    `json:"qty"`
}

// StockCritical returns SKUs that last fewer than policy.critical_days, soonest first.
func (b *Board) StockCritical(stock []domain.StockItem) []CriticalItem {
	var out []CriticalItem
	for _, s := range stock {
		if !metrics.IsCritical(s, b.Policies) {
			continue
		}
		ci := CriticalItem{StockItem: s, DaysLeft: math.Round(*s.DaysLeft()*10) / 10}
		cat := domain.CategoryIndex(s.Category)
		for _, it := range b.Items {
			if it.Branch == s.Branch && cat >= 0 && it.Metrics.MixCats[cat] && it.Metrics.Status != domain.StatusChurn {
				ci.Dependents++
			}
		}
		for _, o := range stock {
			if o.SKU == s.SKU && o.Branch != s.Branch {
				ci.OtherBranches = append(ci.OtherBranches, BranchQty{Branch: o.Branch, Qty: o.Qty})
			}
		}
		out = append(out, ci)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DaysLeft < out[j].DaysLeft })
	return out
}

// ProductSales is "Penjualan per produk".
type ProductSales struct {
	Product   string  `json:"product"`
	Category  string  `json:"category"`
	Value     int64   `json:"value"`
	MarginPct float64 `json:"margin_pct"`
}

// SalesByProduct sums line values of confirmed orders within days, top first; margin per product from the lines'
// HPP (imported data), else the order's margin.
func (b *Board) SalesByProduct(days, limit int) []ProductSales {
	type acc struct {
		v      int64
		cat    string
		margin float64
	}
	by := map[string]*acc{}
	for _, it := range b.Items {
		for _, o := range b.Data.Histories[it.UUID].Orders {
			if o.State == "cancel" || o.ConfirmedAt == nil || metrics.DaysBetween(*o.ConfirmedAt, b.Today) > days {
				continue
			}
			for _, l := range o.Lines {
				a, ok := by[l.Product]
				if !ok {
					a = &acc{cat: l.Category}
					by[l.Product] = a
				}
				a.v += l.Value()
				m := o.MarginPct // the order's margin, unless the line carries its own HPP
				if l.Cost > 0 && l.Value() > 0 {
					m = (1 - float64(l.Cost*l.Qty)/float64(l.Value())) * 100
				}
				a.margin += float64(l.Value()) * m
			}
		}
	}
	out := make([]ProductSales, 0, len(by))
	for p, a := range by {
		m := 0.0
		if a.v > 0 {
			m = math.Round(a.margin/float64(a.v)*10) / 10
		}
		out = append(out, ProductSales{Product: p, Category: a.cat, Value: a.v, MarginPct: m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ---------- Kredit · kas ----------

// CreditOverview is the KPI row of Kredit · kas.
type CreditOverview struct {
	DSODays        int   `json:"dso_days"`
	TermsAvg       int   `json:"terms_avg"`
	Receivable     int64 `json:"receivable"`
	OpenInvoices   int   `json:"open_invoices"`
	OpenDealers    int   `json:"open_dealers"`
	Overdue        int64 `json:"overdue"`
	OverdueDealers int   `json:"overdue_dealers"`
	Overdue30      int   `json:"overdue_over_30"`
	Forecast30     int64 `json:"forecast_30"`
}

// ARRow is "Sisa limit tiap dealer": dealers with open receivables.
type ARRow struct {
	DealerID string `json:"dealer_id"`
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	PayDays  int    `json:"pay_days"`
	OnTime   int    `json:"on_time"`
	Open     int64  `json:"open"`
	Overdue  int64  `json:"overdue"`
	LateDays int    `json:"late_days"`
	Credit   string `json:"credit_state"`
}

// ExposureRow is "Exposure vs limit".
type ExposureRow struct {
	DealerID  string `json:"dealer_id"`
	ShortName string `json:"short_name"`
	Exposure  int64  `json:"exposure"`
	Limit     int64  `json:"limit"`
	Pct       int    `json:"pct"`
}

// ForecastRow is one dealer's expected cash-in.
type ForecastRow struct {
	DealerID    string  `json:"dealer_id"`
	Name        string  `json:"name"`
	Invoices    int     `json:"invoices"`
	Open        int64   `json:"open"`
	PayDays     int     `json:"pay_days"`
	LateCount   int     `json:"late_count"`
	Probability float64 `json:"probability"`
	Expected    int64   `json:"expected"`
	AskedTempo  bool    `json:"asked_tempo"` // dealer asked for more time (akar: proyek belum cair)
}

// Credit computes the Kredit · kas page.
func (b *Board) Credit(days int) (CreditOverview, []ARRow, []ExposureRow, []ForecastRow, int64) {
	var ov CreditOverview
	var ar []ARRow
	var ex []ExposureRow
	var fc []ForecastRow
	var orders []domain.Order
	termsSum, termsN := 0, 0
	for _, it := range b.Items {
		h := b.Data.Histories[it.UUID]
		orders = append(orders, h.Orders...)
		if it.CreditLimit > 0 {
			termsSum += h.TermsDays
			termsN++
			ex = append(ex, ExposureRow{DealerID: it.ID, ShortName: it.ShortName, Exposure: it.Metrics.Credit.Exposure, Limit: it.CreditLimit,
				Pct: int(math.Round(100 * float64(it.Metrics.Credit.Exposure) / float64(it.CreditLimit)))})
		}
		row := ARRow{DealerID: it.ID, Name: it.Name, Owner: it.Owner.Name, PayDays: it.Metrics.Credit.PayDays, OnTime: it.Metrics.Credit.OnTime, Credit: it.Metrics.Credit.State}
		f := ForecastRow{DealerID: it.ID, Name: it.Name, PayDays: it.Metrics.Credit.PayDays, AskedTempo: it.RootCause == domain.RootProjectUnpaid}
		var expected float64
		for _, inv := range OpenInvoices(h, b.Today) {
			ov.Receivable += inv.Residual
			ov.OpenInvoices++
			row.Open += inv.Residual
			f.Invoices++
			f.Open += inv.Residual
			if inv.LateDays > 0 {
				row.Overdue += inv.Residual
				f.LateCount++
				if inv.LateDays > row.LateDays {
					row.LateDays = inv.LateDays
				}
			}
			for _, di := range h.Invoices {
				if di.Number == inv.Number {
					expected += float64(inv.Residual) * metrics.PayProbability(di, it.Metrics.Credit.PayDays, it.Metrics.Credit.OnTime, b.Today, days, it.RootCause == domain.RootProjectUnpaid)
				}
			}
		}
		if row.Open > 0 {
			ov.OpenDealers++
			ar = append(ar, row)
			f.Expected = int64(math.Round(expected))
			if f.Open > 0 {
				f.Probability = math.Round(float64(f.Expected)/float64(f.Open)*100) / 100
			}
			fc = append(fc, f)
		}
		if row.Overdue > 0 {
			ov.Overdue += row.Overdue
			ov.OverdueDealers++
			if row.LateDays > 30 {
				ov.Overdue30++
			}
		}
	}
	if termsN > 0 {
		ov.TermsAvg = int(math.Round(float64(termsSum) / float64(termsN)))
	}
	ov.DSODays = metrics.DSO(orders, b.Today)
	var total int64
	for _, f := range fc {
		total += f.Expected
	}
	ov.Forecast30 = total
	sort.SliceStable(ar, func(i, j int) bool {
		if ar[i].LateDays != ar[j].LateDays {
			return ar[i].LateDays > ar[j].LateDays
		}
		return ar[i].Open > ar[j].Open
	})
	sort.SliceStable(ex, func(i, j int) bool {
		return float64(ex[i].Exposure)/float64(ex[i].Limit) > float64(ex[j].Exposure)/float64(ex[j].Limit)
	})
	sort.SliceStable(fc, func(i, j int) bool { return fc[i].Expected > fc[j].Expected })
	return ov, ar, ex, fc, total
}
