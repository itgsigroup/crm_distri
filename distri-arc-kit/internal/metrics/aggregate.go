package metrics

import (
	"math"
	"sort"
	"time"

	"distri-arc/internal/domain"
)

// ProductShare is one row of "Komposisi product mix": a product's share of 6-month order value.
type ProductShare struct {
	Product  string `json:"product"`
	Category string `json:"category"`
	Pct      int    `json:"pct"`
	Value    int64  `json:"value"`
}

// Composition returns the top products by value over the last 6 months (shares ≥ 2%, at most limit rows).
func Composition(orders []domain.Order, today time.Time, limit int) []ProductShare {
	by := map[string]*ProductShare{}
	var total int64
	for _, o := range orders {
		t := orderTime(o)
		if o.State == "cancel" || t == nil || DaysBetween(*t, today) > window6m {
			continue
		}
		for _, l := range o.Lines {
			ps, ok := by[l.Product]
			if !ok {
				ps = &ProductShare{Product: l.Product, Category: l.Category}
				by[l.Product] = ps
			}
			ps.Value += l.Value()
			total += l.Value()
		}
	}
	out := make([]ProductShare, 0, len(by))
	for _, ps := range by {
		if total > 0 {
			ps.Pct = int(math.Round(100 * float64(ps.Value) / float64(total)))
		}
		if ps.Pct >= 2 {
			out = append(out, *ps)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Product < out[j].Product
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// MonthTotal is the confirmed order value of one calendar month.
type MonthTotal struct {
	Month string `json:"month"` // "2026-10"
	Label string `json:"label"` // "Okt"
	Total int64  `json:"total"`
}

var idMonths = [12]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// MonthLabel returns the Indonesian short month name.
func MonthLabel(m time.Month) string { return idMonths[m-1] }

// MonthlyTotals returns order value per month for the last n calendar months (oldest first, current last).
func MonthlyTotals(orders []domain.Order, today time.Time, n int) []MonthTotal {
	t := Day(today)
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, -(n - 1), 0)
	out := make([]MonthTotal, n)
	for i := range out {
		m := start.AddDate(0, i, 0)
		out[i] = MonthTotal{Month: m.Format("2006-01"), Label: MonthLabel(m.Month())}
	}
	for _, o := range orders {
		ot := orderTime(o)
		if o.State == "cancel" || ot == nil {
			continue
		}
		k := Day(*ot).Format("2006-01")
		for i := range out {
			if out[i].Month == k {
				out[i].Total += o.Total
			}
		}
	}
	return out
}

// OnSchedulePct is "order tepat jadwal": % of active dealers (with a rhythm, not Churn) whose cyc ≤ drift.
func OnSchedulePct(ms []domain.DealerMetrics, p domain.PolicySet) int {
	den, ok := 0, 0
	for _, m := range ms {
		if m.Rhythm == nil || m.Status == domain.StatusChurn {
			continue
		}
		den++
		if m.Cyc <= p.Orbit.Drift {
			ok++
		}
	}
	if den == 0 {
		return 0
	}
	return int(math.Round(100 * float64(ok) / float64(den)))
}

// DSO is the mean days Order → Bayar for orders paid within the last 90 days. Cash sales (paid the day they are
// ordered) never become receivables and are left out.
func DSO(orders []domain.Order, today time.Time) int {
	sum, n := 0, 0
	for _, o := range orders {
		t := orderTime(o)
		if o.PaidAt == nil || t == nil || DaysBetween(*o.PaidAt, today) > 90 {
			continue
		}
		d := DaysBetween(*t, *o.PaidAt)
		if d <= 0 {
			continue
		}
		sum += d
		n++
	}
	if n == 0 {
		return 0
	}
	return int(math.Round(float64(sum) / float64(n)))
}

// COGSDaily is the average daily cost of goods sold over the last 90 days (order total × (1 − margin)).
func COGSDaily(orders []domain.Order, today time.Time) float64 {
	var cogs float64
	for _, o := range orders {
		t := orderTime(o)
		if o.State == "cancel" || t == nil || DaysBetween(*t, today) > 90 {
			continue
		}
		cogs += float64(o.Total) * (1 - o.MarginPct/100)
	}
	return cogs / 90
}

// StockTurnDays is perputaran stok: stock value ÷ daily COGS.
func StockTurnDays(stockValue int64, cogsDaily float64) int {
	if cogsDaily <= 0 {
		return 0
	}
	return int(math.Round(float64(stockValue) / cogsDaily))
}

// DealerView is the slice of a dealer that stock/recommendation rules read.
type DealerView struct {
	ID          string
	Name        string
	Metrics     domain.DealerMetrics
	Composition []ProductShare
}

// PushCandidate is a dealer that fits an aging stock item.
type PushCandidate struct {
	DealerID string `json:"dealer_id"`
	Name     string `json:"name"`
	Reason   string `json:"reason"` // "product mix cocok" | "lebar baru"
	DueIn    *int   `json:"due_in"`
	Drifting bool   `json:"drifting"`
	OmzetBln int64  `json:"omzet_bln"`
}

// IsAging reports whether a stock item is older than policy.aging_days.
func IsAging(s domain.StockItem, p domain.PolicySet) bool { return s.AgeDays > p.Stock.AgingDays }

// IsCritical reports whether a stock item runs out in fewer than policy.critical_days at its weekly velocity.
func IsCritical(s domain.StockItem, p domain.PolicySet) bool {
	d := s.DaysLeft()
	return d != nil && *d < p.Stock.CriticalDays
}

// PushCandidates applies the glossary rule for aging stock:
// mix fits (category bought, or an empty category for a Key account in Segmen A — "lebar baru") ∧
// (due_in ≤ 7 ∨ lewat jadwal) ∧ sisa limit ∉ {over limit, overdue}. Sorted by jadwal order, then omzet.
func PushCandidates(item domain.StockItem, dealers []DealerView, p domain.PolicySet) []PushCandidate {
	ci := domain.CategoryIndex(item.Category)
	var out []PushCandidate
	for _, d := range dealers {
		m := d.Metrics
		if ci < 0 || m.Rhythm == nil {
			continue
		}
		reason := ""
		switch {
		case m.MixCats[ci]:
			reason = "product mix cocok"
		case m.Status == domain.StatusKeyAccount && m.Segment == domain.SegmentA:
			reason = "lebar baru"
		default:
			continue
		}
		drifting := m.Cyc > p.Orbit.Drift
		dueSoon := m.DueIn != nil && *m.DueIn >= 0 && *m.DueIn <= 7
		if !dueSoon && !drifting {
			continue
		}
		if m.Credit.State == domain.CreditOverLimit || m.Credit.State == domain.CreditOverdue {
			continue
		}
		out = append(out, PushCandidate{DealerID: d.ID, Name: d.Name, Reason: reason, DueIn: m.DueIn, Drifting: drifting, OmzetBln: m.OmzetBln})
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := dueKey(out[i]), dueKey(out[j])
		if di != dj {
			return di < dj
		}
		return out[i].OmzetBln > out[j].OmzetBln
	})
	return out
}

func dueKey(c PushCandidate) int {
	if c.DueIn == nil {
		return 1 << 20
	}
	return *c.DueIn
}

// Recommendation is the suggested SO content for a follow-up.
type Recommendation struct {
	Products  []string `json:"products"`  // top 3 of the dealer's 6-month composition
	Sweetener string   `json:"sweetener"` // one aging SKU that fits the dealer's mix
	Critical  []string `json:"critical"`  // critical SKUs in the dealer's branch among the products
}

// OrderRecommendation builds the rekomendasi order: 3 top products + 1 aging sweetener + critical warnings.
func OrderRecommendation(d DealerView, branch string, stock []domain.StockItem, p domain.PolicySet) Recommendation {
	var r Recommendation
	for i, c := range d.Composition {
		if i == 3 {
			break
		}
		r.Products = append(r.Products, c.Product)
	}
	for _, s := range stock {
		ci := domain.CategoryIndex(s.Category)
		if r.Sweetener == "" && IsAging(s, p) && ci >= 0 && d.Metrics.MixCats[ci] {
			r.Sweetener = s.Name
		}
		if s.Branch == branch && IsCritical(s, p) {
			for _, prod := range r.Products {
				if s.Category == categoryOf(d.Composition, prod) {
					r.Critical = append(r.Critical, s.Name)
					break
				}
			}
		}
	}
	return r
}

func categoryOf(comp []ProductShare, product string) string {
	for _, c := range comp {
		if c.Product == product {
			return c.Category
		}
	}
	return ""
}

// PayProbability is the chance that an open invoice is paid within horizon days, from the dealer's pattern:
// pola bayar vs the days the invoice still has to reach that pattern, discounted when already overdue.
func PayProbability(inv domain.Invoice, payDays, onTime int, today time.Time, horizon int) float64 {
	age := DaysBetween(inv.IssuedAt, today)
	expected := payDays - age // days until the dealer usually pays
	base := float64(onTime) / 100
	var p float64
	switch {
	case expected <= horizon && expected >= 0:
		p = 0.55 + 0.45*base
	case expected < 0: // already beyond the usual pattern
		late := -expected
		p = math.Max(0.2, (0.35+0.45*base)*(1-float64(late)/60))
	default:
		p = math.Max(0.05, 0.5*float64(horizon)/float64(expected))
	}
	return math.Min(0.98, p)
}
