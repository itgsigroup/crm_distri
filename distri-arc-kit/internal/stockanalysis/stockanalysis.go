// Package stockanalysis is Analisa stok on Cek stok (Sam 22 Oct, "as the owner"): which products move fast, which
// sit, where the money is held — FSN classes (fast / normal / slow / dead), ABC by sales value, aging, stock cover,
// stock turnover (glossary: nilai stok / COGS harian, target ≤ 40 hari), what to reorder and what to move between
// branches — with insight lines written from the numbers. Numbers only from formulas (no AI).
package stockanalysis

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"distri-arc/internal/domain"
)

const (
	// SlowCoverDays: stock that lasts longer than this at its velocity is slow moving.
	SlowCoverDays = 90
	// FastShare: the fastest share of moving products is fast moving.
	FastShare = 0.2
	// ReorderCoverDays: a reorder suggestion covers this many days of sales.
	ReorderCoverDays = 30
	// TurnoverTargetDays is the glossary target of Perputaran stok.
	TurnoverTargetDays = 40
	listTop            = 10
	weeksPerMonth      = 52.0 / 12
)

// Class is FSN: fast | normal | slow | dead.
type Class string

const (
	Fast   Class = "fast"
	Normal Class = "normal"
	Slow   Class = "slow"
	Dead   Class = "dead"
)

// Product is one product of the analysis (summed over the branches read).
type Product struct {
	SKU        string   `json:"sku"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Qty        int      `json:"qty"`
	Value      int64    `json:"value"`
	PerWeek    float64  `json:"per_week"`    // units sold per week (90-day average)
	CoverDays  *float64 `json:"cover_days"`  // days the stock lasts at that pace; nil when it does not move
	AgeDays    int      `json:"age_days"`    // oldest stock
	MonthSales int64    `json:"month_sales"` // estimated sales value a month (list price, else cost)
	Class      Class    `json:"class"`
	ABC        string   `json:"abc"`         // A | B | C, "" without sales
	Critical   bool     `json:"critical"`    // runs out before policy.critical_days
	Aging      bool     `json:"aging"`       // older than policy.aging_days
	ReorderQty int      `json:"reorder_qty"` // to cover ReorderCoverDays, 0 when not needed
	Branches   []Branch `json:"branches"`
}

// Branch is a product in one branch.
type Branch struct {
	Branch  string  `json:"branch"`
	Qty     int     `json:"qty"`
	PerWeek float64 `json:"per_week"`
}

// Share is a slice of the stock: products, units, value.
type Share struct {
	Key      string `json:"key"`
	SKUs     int    `json:"skus"`
	Units    int    `json:"units"`
	Value    int64  `json:"value"`
	Pct      int    `json:"pct"` // of the stock value
	Turnover *int   `json:"turnover_days,omitempty"`
	Dead     int64  `json:"dead_value,omitempty"`
	Sales    int64  `json:"month_sales,omitempty"`
}

// Transfer moves a product from a branch where it does not sell to one where it runs short.
type Transfer struct {
	SKU   string `json:"sku"`
	Name  string `json:"name"`
	From  string `json:"from"`
	To    string `json:"to"`
	Qty   int    `json:"qty"`
	Value int64  `json:"value"`
}

// Insight is one line for the owner; Tone good | warn | bad | info.
type Insight struct {
	Tone string `json:"tone"`
	Text string `json:"text"`
}

// Analysis is GET /stock/analysis.
type Analysis struct {
	Branch       string     `json:"branch"`
	SKUs         int        `json:"skus"`
	Units        int        `json:"units"`
	Value        int64      `json:"value"`
	MonthSales   int64      `json:"month_sales"`
	Turnover     *int       `json:"turnover_days"`
	TurnoverGoal int        `json:"turnover_target"`
	AgingDays    int        `json:"aging_days"`
	CriticalDays float64    `json:"critical_days"`
	Classes      []Share    `json:"classes"`    // fast, normal, slow, dead
	ABC          []Share    `json:"abc"`        // A, B, C
	Categories   []Share    `json:"categories"` // by value
	Branches     []Share    `json:"branches"`   // by value
	AgeBuckets   []Share    `json:"age_buckets"`
	AgingValue   int64      `json:"aging_value"`
	AgingSKUs    int        `json:"aging_skus"`
	CriticalSKUs int        `json:"critical_skus"`
	FastList     []Product  `json:"fast"`
	Reorder      []Product  `json:"reorder"`
	DeadList     []Product  `json:"dead"`
	SlowList     []Product  `json:"slow"`
	Transfers    []Transfer `json:"transfers"`
	Insights     []Insight  `json:"insights"`
}

// Compute reads the stock items (one row per product and branch). branch "" reads every branch. price gives a
// product's list price (0 unknown: its cost is used for the sales value).
func Compute(items []domain.StockItem, branch string, price func(sku, name string) int64, p domain.StockPolicy) Analysis {
	a := Analysis{Branch: branch, TurnoverGoal: TurnoverTargetDays, AgingDays: p.AgingDays, CriticalDays: p.CriticalDays,
		Classes: []Share{}, ABC: []Share{}, Categories: []Share{}, Branches: []Share{}, AgeBuckets: []Share{},
		FastList: []Product{}, Reorder: []Product{}, DeadList: []Product{}, SlowList: []Product{}, Transfers: []Transfer{}, Insights: []Insight{}}
	bySKU := map[string]*Product{}
	cost := map[string]int64{}
	var cogsWeek float64
	branchShare := map[string]*Share{}
	branchCogs := map[string]float64{}
	for _, s := range items {
		if branch != "" && s.Branch != branch {
			continue
		}
		x := bySKU[s.SKU]
		if x == nil {
			x = &Product{SKU: s.SKU, Name: s.Name, Category: s.Category}
			bySKU[s.SKU] = x
		}
		x.Qty += max(0, s.Qty)
		x.Value += max(0, s.Value)
		x.PerWeek += s.WeeklyVelocity
		x.AgeDays = max(x.AgeDays, s.AgeDays)
		x.Branches = append(x.Branches, Branch{Branch: s.Branch, Qty: s.Qty, PerWeek: s.WeeklyVelocity})
		if s.UnitCost > 0 {
			cost[s.SKU] = s.UnitCost
		}
		cogsWeek += s.WeeklyVelocity * float64(s.UnitCost)
		b := branchShare[s.Branch]
		if b == nil {
			b = &Share{Key: s.Branch}
			branchShare[s.Branch] = b
		}
		b.Value += max(0, s.Value)
		if s.Qty > 0 {
			b.SKUs++
		}
		branchCogs[s.Branch] += s.WeeklyVelocity * float64(s.UnitCost)
		if s.WeeklyVelocity <= 0 && s.Qty > 0 {
			b.Dead += max(0, s.Value)
		}
	}
	prods := make([]*Product, 0, len(bySKU))
	for _, x := range bySKU {
		x.PerWeek = math.Round(x.PerWeek*100) / 100
		unit := price(x.SKU, x.Name)
		if unit <= 0 {
			unit = cost[x.SKU]
		}
		x.MonthSales = int64(x.PerWeek * weeksPerMonth * float64(unit))
		if x.PerWeek > 0 {
			c := math.Round(float64(x.Qty)/x.PerWeek*7*10) / 10
			x.CoverDays = &c
		}
		x.Aging = x.Qty > 0 && x.AgeDays > p.AgingDays
		x.Critical = x.CoverDays != nil && *x.CoverDays < p.CriticalDays
		if x.PerWeek > 0 {
			if need := int(math.Ceil(x.PerWeek/7*ReorderCoverDays)) - x.Qty; x.Critical && need > 0 {
				x.ReorderQty = need
			}
		}
		slices.SortFunc(x.Branches, func(a, b Branch) int { return strings.Compare(a.Branch, b.Branch) })
		prods = append(prods, x)
	}
	slices.SortFunc(prods, func(a, b *Product) int { return strings.Compare(a.SKU, b.SKU) })

	// FSN: the fastest FastShare of the moving products; slow when the stock lasts beyond SlowCoverDays; dead when
	// nothing sold in 90 days while stock is held
	var moving []float64
	for _, x := range prods {
		if x.PerWeek > 0 {
			moving = append(moving, x.PerWeek)
		}
	}
	sort.Float64s(moving)
	fastAt := math.Inf(1)
	if n := len(moving); n > 0 {
		fastAt = moving[min(n-1, int(math.Floor(float64(n)*(1-FastShare))))]
	}
	for _, x := range prods {
		switch {
		case x.PerWeek <= 0:
			x.Class = Dead
		case x.PerWeek >= fastAt:
			x.Class = Fast
		case x.CoverDays != nil && *x.CoverDays > SlowCoverDays:
			x.Class = Slow
		default:
			x.Class = Normal
		}
	}
	// a product with neither stock nor sales says nothing
	held := prods[:0]
	for _, x := range prods {
		if x.Qty > 0 || x.PerWeek > 0 {
			held = append(held, x)
		}
	}
	prods = held

	// ABC on the monthly sales value
	bySales := slices.Clone(prods)
	slices.SortStableFunc(bySales, func(a, b *Product) int { return cmpDesc(a.MonthSales, b.MonthSales) })
	var totalSales int64
	for _, x := range prods {
		totalSales += x.MonthSales
	}
	var run int64
	for _, x := range bySales {
		if x.MonthSales <= 0 {
			continue
		}
		share := float64(run) / float64(totalSales)
		run += x.MonthSales
		switch {
		case share < 0.8:
			x.ABC = "A"
		case share < 0.95:
			x.ABC = "B"
		default:
			x.ABC = "C"
		}
	}
	a.MonthSales = totalSales

	// totals and slices
	classes := map[Class]*Share{Fast: {Key: string(Fast)}, Normal: {Key: string(Normal)}, Slow: {Key: string(Slow)}, Dead: {Key: string(Dead)}}
	abc := map[string]*Share{"A": {Key: "A"}, "B": {Key: "B"}, "C": {Key: "C"}}
	cats := map[string]*Share{}
	catCogs := map[string]float64{}
	buckets := []*Share{{Key: "0–30"}, {Key: "31–60"}, {Key: "61–90"}, {Key: "91–180"}, {Key: "> 180"}}
	for _, x := range prods {
		a.SKUs++
		a.Units += x.Qty
		a.Value += x.Value
		c := classes[x.Class]
		c.SKUs++
		c.Units += x.Qty
		c.Value += x.Value
		c.Sales += x.MonthSales
		if s := abc[x.ABC]; s != nil {
			s.SKUs++
			s.Value += x.Value
			s.Sales += x.MonthSales
		}
		k := x.Category
		if k == "" {
			k = "Lainnya"
		}
		if cats[k] == nil {
			cats[k] = &Share{Key: k}
		}
		cats[k].SKUs++
		cats[k].Value += x.Value
		cats[k].Sales += x.MonthSales
		if x.Class == Dead {
			cats[k].Dead += x.Value
		}
		catCogs[k] += x.PerWeek * float64(cost[x.SKU])
		if x.Qty > 0 {
			b := buckets[bucket(x.AgeDays)]
			b.SKUs++
			b.Value += x.Value
		}
		if x.Aging {
			a.AgingValue += x.Value
			a.AgingSKUs++
		}
		if x.Critical {
			a.CriticalSKUs++
		}
	}
	a.Turnover = turnover(a.Value, cogsWeek)
	pct := func(v int64) int {
		if a.Value <= 0 {
			return 0
		}
		return int(math.Round(float64(v) * 100 / float64(a.Value)))
	}
	for _, k := range []Class{Fast, Normal, Slow, Dead} {
		s := classes[k]
		s.Pct = pct(s.Value)
		a.Classes = append(a.Classes, *s)
	}
	for _, k := range []string{"A", "B", "C"} {
		s := abc[k]
		s.Pct = pct(s.Value)
		a.ABC = append(a.ABC, *s)
	}
	for k, s := range cats {
		s.Pct = pct(s.Value)
		s.Turnover = turnover(s.Value, catCogs[k])
		a.Categories = append(a.Categories, *s)
	}
	slices.SortFunc(a.Categories, func(x, y Share) int { return cmpDesc(x.Value, y.Value) })
	for k, s := range branchShare {
		s.Pct = pct(s.Value)
		s.Turnover = turnover(s.Value, branchCogs[k])
		a.Branches = append(a.Branches, *s)
	}
	slices.SortFunc(a.Branches, func(x, y Share) int { return cmpDesc(x.Value, y.Value) })
	for _, b := range buckets {
		b.Pct = pct(b.Value)
		a.AgeBuckets = append(a.AgeBuckets, *b)
	}

	// lists
	pick := func(keep func(*Product) bool, less func(a, b *Product) int) []Product {
		var xs []*Product
		for _, x := range prods {
			if keep(x) {
				xs = append(xs, x)
			}
		}
		slices.SortStableFunc(xs, less)
		out := []Product{}
		for _, x := range xs[:min(len(xs), listTop)] {
			out = append(out, *x)
		}
		return out
	}
	a.FastList = pick(func(x *Product) bool { return x.Class == Fast }, func(x, y *Product) int { return cmpDescF(x.PerWeek, y.PerWeek) })
	a.Reorder = pick(func(x *Product) bool { return x.Critical }, func(x, y *Product) int { return cmpF(*x.CoverDays, *y.CoverDays) })
	a.DeadList = pick(func(x *Product) bool { return x.Class == Dead }, func(x, y *Product) int { return cmpDesc(x.Value, y.Value) })
	a.SlowList = pick(func(x *Product) bool { return x.Class == Slow }, func(x, y *Product) int { return cmpDesc(x.Value, y.Value) })
	if branch == "" {
		a.Transfers = transfers(prods, cost)
	}
	a.Insights = insights(a, classes, abc)
	return a
}

// transfers: a product held without sales in one branch while another branch sells it and runs short.
func transfers(prods []*Product, cost map[string]int64) []Transfer {
	out := []Transfer{}
	for _, x := range prods {
		for _, from := range x.Branches {
			if from.Qty <= 0 || from.PerWeek > 0 {
				continue
			}
			for _, to := range x.Branches {
				if to.PerWeek <= 0 || to.Branch == from.Branch {
					continue
				}
				if cover := float64(max(0, to.Qty)) / to.PerWeek * 7; cover >= ReorderCoverDays {
					continue
				}
				need := int(math.Ceil(to.PerWeek/7*ReorderCoverDays)) - max(0, to.Qty)
				q := min(from.Qty, need)
				if q > 0 {
					out = append(out, Transfer{SKU: x.SKU, Name: x.Name, From: from.Branch, To: to.Branch, Qty: q, Value: int64(q) * cost[x.SKU]})
				}
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Transfer) int { return cmpDesc(a.Value, b.Value) })
	return out[:min(len(out), listTop)]
}

func insights(a Analysis, classes map[Class]*Share, abc map[string]*Share) []Insight {
	out := []Insight{}
	add := func(tone, f string, v ...any) { out = append(out, Insight{Tone: tone, Text: fmt.Sprintf(f, v...)}) }
	if a.SKUs == 0 {
		return out
	}
	if t := a.Turnover; t != nil {
		if *t <= a.TurnoverGoal {
			add("good", "Perputaran stok %d hari — sesuai target ≤ %d hari.", *t, a.TurnoverGoal)
		} else {
			add("warn", "Perputaran stok %d hari, di atas target %d hari: modal tertahan lebih lama dari seharusnya.", *t, a.TurnoverGoal)
		}
	}
	if d := classes[Dead]; d.SKUs > 0 {
		tone := "warn"
		if d.Pct >= 20 {
			tone = "bad"
		}
		if d.Value > 0 {
			add(tone, "%s (%d%% nilai stok) tertahan di %d barang dead stock — tidak terjual 90 hari. Prioritaskan push, bundle, atau retur ke pemasok.", Rp(d.Value), d.Pct, d.SKUs)
		} else { // no cost read for them: the units say it
			add(tone, "%d barang dead stock (%d unit) tidak terjual 90 hari — harga pokoknya belum terbaca, nilainya belum dihitung. Prioritaskan push, bundle, atau retur.", d.SKUs, d.Units)
		}
	}
	if s := classes[Slow]; s.SKUs > 0 {
		add("warn", "%d barang slow moving senilai %s (%d%%): stoknya cukup untuk lebih dari %d hari — tahan pembelian ulang.", s.SKUs, Rp(s.Value), s.Pct, SlowCoverDays)
	}
	if f := classes[Fast]; f.SKUs > 0 {
		sp := 0
		if a.MonthSales > 0 {
			sp = int(math.Round(float64(f.Sales) * 100 / float64(a.MonthSales)))
		}
		add("good", "%d barang fast moving menghasilkan %d%% penjualan dengan hanya %d%% nilai stok — jaga stoknya jangan sampai kosong.", f.SKUs, sp, f.Pct)
	}
	if a.CriticalSKUs > 0 {
		add("bad", "%d barang akan habis dalam kurang dari %.0f hari pada kecepatan jual sekarang — lihat daftar Reorder segera.", a.CriticalSKUs, a.CriticalDays)
	}
	if a.AgingSKUs > 0 {
		add("warn", "Stok menua (> %d hari): %d barang senilai %s.", a.AgingDays, a.AgingSKUs, Rp(a.AgingValue))
	}
	if x := abc["A"]; x.SKUs > 0 && a.SKUs > 0 {
		add("info", "Analisa ABC: %d barang kelas A (%d%% dari barang) menghasilkan ±80%% penjualan — fokuskan ketersediaan dan negosiasi harga beli di sini.", x.SKUs, x.SKUs*100/a.SKUs)
	}
	// the slowest category holding at least 10% of the value: one without sales first, else the longest turnover
	var worst *Share
	for i := range a.Categories {
		c := &a.Categories[i]
		if c.Pct < 10 {
			continue
		}
		if worst == nil || slower(c, worst) {
			worst = c
		}
	}
	if worst != nil {
		if worst.Turnover == nil {
			add("warn", "Kategori %s menyimpan %d%% nilai stok tanpa penjualan 90 hari.", worst.Key, worst.Pct)
		} else if *worst.Turnover > a.TurnoverGoal {
			add("warn", "Kategori %s paling lambat berputar (%d hari) dan memegang %d%% nilai stok.", worst.Key, *worst.Turnover, worst.Pct)
		}
	}
	if len(a.Transfers) > 0 {
		var v int64
		for _, t := range a.Transfers {
			v += t.Value
		}
		add("info", "%d peluang mutasi antar cabang (±%s): barang yang diam di satu cabang dibutuhkan cabang lain — tanpa pembelian baru.", len(a.Transfers), Rp(v))
	}
	return out
}

// slower: no sales is slowest; then the longer turnover.
func slower(a, b *Share) bool {
	if a.Turnover == nil || b.Turnover == nil {
		return a.Turnover == nil && b.Turnover != nil
	}
	return *a.Turnover > *b.Turnover
}

func bucket(age int) int {
	switch {
	case age <= 30:
		return 0
	case age <= 60:
		return 1
	case age <= 90:
		return 2
	case age <= 180:
		return 3
	default:
		return 4
	}
}

// turnover is Perputaran stok in days: stock value / daily COGS (nil without sales).
func turnover(value int64, cogsWeek float64) *int {
	if cogsWeek <= 0 {
		return nil
	}
	d := int(math.Round(float64(value) / (cogsWeek / 7)))
	return &d
}

// Rp is "Rp 1,2 M", "Rp 350 jt", "Rp 12.500".
func Rp(v int64) string {
	f := float64(v)
	switch {
	case f >= 1e9:
		return strings.Replace(fmt.Sprintf("Rp %.1f M", f/1e9), ".", ",", 1)
	case f >= 1e6:
		return fmt.Sprintf("Rp %.0f jt", f/1e6)
	default:
		s := fmt.Sprintf("%d", v)
		var b strings.Builder
		for i, r := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				b.WriteByte('.')
			}
			b.WriteRune(r)
		}
		return "Rp " + b.String()
	}
}

func cmpDesc(a, b int64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}

func cmpDescF(a, b float64) int { return -cmpF(a, b) }

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
