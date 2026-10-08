package views

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Peta relasi (mockup screen-net): sales numbers ↔ dealers weighted by WhatsApp + order interactions per month.

// RelasiPeriods maps the period buttons (days) to months summed.
var RelasiPeriods = map[int]int{30: 1, 60: 2, 90: 3, 180: 6}

// MonthCount is one (sales, dealer, month) aggregate.
type MonthCount struct {
	SalesID  uuid.UUID
	DealerID uuid.UUID
	Month    time.Time
	N        int64
}

// RelasiSales is a sales number shown as a node.
type RelasiSales struct {
	ID       uuid.UUID
	Key      string
	Name     string
	Branch   string
	WANumber string
}

// RelasiNode is a node of the graph.
type RelasiNode struct {
	ID     string `json:"id"` // s-<sales key> | dealer slug
	Type   string `json:"type"`
	Name   string `json:"name"`
	Sub    string `json:"sub"`
	Tone   string `json:"tone"` // accent (sales) | good | warn | bad | neutral
	Score  *int   `json:"score,omitempty"`
	Total  int64  `json:"total"`
	Number string `json:"number,omitempty"`
}

// RelasiEdge is a sales ↔ dealer pair.
type RelasiEdge struct {
	Sales   string  `json:"sales"`
	Dealer  string  `json:"dealer"`
	W       int64   `json:"w"`       // interactions in the period
	Monthly []int64 `json:"monthly"` // last 6 months, oldest first (tooltip)
}

// RelasiPair is a row of "Pasangan terkuat".
type RelasiPair struct {
	Sales  string `json:"sales"`
	Dealer string `json:"dealer"`
	W      int64  `json:"w"`
}

// Relasi is the graph for one period and sales filter.
type Relasi struct {
	PeriodDays   int          `json:"period_days"`
	Months       int          `json:"months"`
	MonthLabels  []string     `json:"month_labels"`
	Nodes        []RelasiNode `json:"nodes"`
	Edges        []RelasiEdge `json:"edges"`
	Pairs        []RelasiPair `json:"pairs"`
	Connections  int          `json:"connections"`
	Interactions int64        `json:"interactions"`
	// DealersActive dealers had an interaction in 6 months; the map draws the DealersShown most active (≤ RelasiMaxDealers)
	DealersActive int `json:"dealers_active"`
	DealersShown  int `json:"dealers_shown"`
}

// RelasiMaxDealers bounds the 3D map: a force layout of thousands of nodes never settles in a browser, and a sales
// team reads the strongest relations first.
const RelasiMaxDealers = 250

// RelasiSince is the first day of the oldest month needed for the 6-month series ending in today's month.
func RelasiSince(today time.Time) time.Time {
	t := today.In(time.FixedZone("WIB", 7*3600))
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, -5, 0)
}

func toneOf(score int) string {
	switch {
	case score >= 70:
		return "good"
	case score >= 50:
		return "warn"
	}
	return "bad"
}

// BuildRelasi builds the graph: the sales numbers and dealers with an interaction in the last 6 months (the most
// active RelasiMaxDealers), edges with the period's sum. The node set is the same for every period and sales filter,
// so the layout does not jump; sales filters edges (and pairs) to one sales key.
func BuildRelasi(b *Board, sales []RelasiSales, rows []MonthCount, periodDays int, salesKey string) Relasi {
	k, ok := RelasiPeriods[periodDays]
	if !ok {
		periodDays, k = 30, 1
	}
	since := RelasiSince(b.Today)
	months := make([]time.Time, 6)
	for i := range months {
		months[i] = since.AddDate(0, i, 0)
	}
	r := Relasi{PeriodDays: periodDays, Months: k, Nodes: []RelasiNode{}, Edges: []RelasiEdge{}, Pairs: []RelasiPair{}}
	for _, m := range months {
		r.MonthLabels = append(r.MonthLabels, idMonth(m.Month()))
	}
	salesByID := map[uuid.UUID]RelasiSales{}
	for _, s := range sales {
		salesByID[s.ID] = s
	}
	dealerByID := map[uuid.UUID]BoardItem{}
	for _, it := range b.Items {
		dealerByID[it.UUID] = it
	}
	type key struct{ s, d uuid.UUID }
	series := map[key][]int64{}
	var order []key
	for _, x := range rows {
		kk := key{x.SalesID, x.DealerID}
		if _, ok := series[kk]; !ok {
			series[kk] = make([]int64, 6)
			order = append(order, kk)
		}
		for i, m := range months {
			if m.Year() == x.Month.Year() && m.Month() == x.Month.Month() {
				series[kk][i] += x.N
			}
		}
	}
	// the dealers drawn: most interactions over the 6 months, the same set for every period
	six := map[uuid.UUID]int64{}
	for _, kk := range order {
		for _, v := range series[kk] {
			six[kk.d] += v
		}
	}
	var active []uuid.UUID
	for d, n := range six {
		if _, ok := dealerByID[d]; ok && n > 0 {
			active = append(active, d)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if six[active[i]] != six[active[j]] {
			return six[active[i]] > six[active[j]]
		}
		return dealerByID[active[i]].Name < dealerByID[active[j]].Name
	})
	r.DealersActive = len(active)
	if len(active) > RelasiMaxDealers {
		active = active[:RelasiMaxDealers]
	}
	r.DealersShown = len(active)
	shown := map[uuid.UUID]bool{}
	for _, d := range active {
		shown[d] = true
	}
	salesShown := map[string]bool{}
	totals := map[string]int64{}
	for _, kk := range order {
		s, okS := salesByID[kk.s]
		d, okD := dealerByID[kk.d]
		if !okS || !okD {
			continue
		}
		m := series[kk]
		var w int64
		for _, v := range m[6-k:] {
			w += v
		}
		e := RelasiEdge{Sales: "s-" + s.Key, Dealer: d.ID, W: w, Monthly: m}
		if shown[kk.d] {
			r.Edges = append(r.Edges, e)
			salesShown[e.Sales] = true
		}
		if salesKey == "" || salesKey == s.Key {
			totals[e.Sales] += w
			totals[e.Dealer] += w
			if w > 0 {
				r.Connections++
				r.Interactions += w
				r.Pairs = append(r.Pairs, RelasiPair{Sales: s.Name, Dealer: d.Name, W: w})
			}
		}
	}
	for _, s := range sales {
		if !salesShown["s-"+s.Key] && s.Key != salesKey {
			continue
		}
		r.Nodes = append(r.Nodes, RelasiNode{ID: "s-" + s.Key, Type: "sales", Name: s.Name, Sub: s.Branch, Tone: "accent", Total: totals["s-"+s.Key], Number: s.WANumber})
	}
	for _, d := range active {
		it := dealerByID[d]
		score := it.Metrics.Score
		r.Nodes = append(r.Nodes, RelasiNode{ID: it.ID, Type: "dealer", Name: it.ShortName, Sub: fmt.Sprintf("%s · tier %s", it.City, it.Tier), Tone: toneOf(score), Score: &score, Total: totals[it.ID]})
	}
	sort.SliceStable(r.Pairs, func(i, j int) bool { return r.Pairs[i].W > r.Pairs[j].W })
	if len(r.Pairs) > 6 {
		r.Pairs = r.Pairs[:6]
	}
	return r
}

var idMonths3 = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func idMonth(m time.Month) string { return idMonths3[m-1] }

// RelasiInsight is one "Pola relasi" line.
type RelasiInsight struct {
	Tone   string `json:"tone"` // bad | accent | warn
	Icon   string `json:"icon"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	Dealer string `json:"dealer"`
}

// RelasiInsights applies the three relation rules (docs/stages/08):
//   - a dealer past its cycle whose WhatsApp intensity declines (last month below the earlier average);
//   - two sales numbers touching one dealer in the period;
//   - a large order size (≥ Rp 60 jt) with thin WhatsApp (< 50 interactions a month).
func RelasiInsights(b *Board, r Relasi, salesKey string) []RelasiInsight {
	bySlug := map[string]BoardItem{}
	for _, it := range b.Items {
		bySlug[it.ID] = it
	}
	inScope := func(dealer string) bool {
		if salesKey == "" {
			return true
		}
		for _, e := range r.Edges {
			if e.Dealer == dealer && e.Sales == "s-"+salesKey && e.W > 0 {
				return true
			}
		}
		return false
	}
	perDealer := map[string][]int64{}
	period := map[string]int64{}
	touch := map[string][]RelasiEdge{}
	var dealers []string
	for _, e := range r.Edges {
		if _, ok := perDealer[e.Dealer]; !ok {
			perDealer[e.Dealer] = make([]int64, len(e.Monthly))
			dealers = append(dealers, e.Dealer)
		}
		for i, v := range e.Monthly {
			perDealer[e.Dealer][i] += v
		}
		period[e.Dealer] += e.W
		if e.W > 0 {
			touch[e.Dealer] = append(touch[e.Dealer], e)
		}
	}
	salesName := map[string]string{}
	for _, n := range r.Nodes {
		if n.Type == "sales" {
			salesName[n.ID] = n.Name
		}
	}
	var out []RelasiInsight
	for _, it := range b.Items {
		m := it.Metrics
		ser := perDealer[it.ID]
		if m.Rhythm == nil || m.Last == nil || m.Cyc <= b.Policies.Orbit.Drift || len(ser) == 0 || !inScope(it.ID) {
			continue
		}
		var prev int64
		for _, v := range ser[:len(ser)-1] {
			prev += v
		}
		avg := float64(prev) / float64(len(ser)-1)
		if float64(ser[len(ser)-1]) >= avg {
			continue
		}
		fav := "produk favoritnya"
		if len(it.Composition) > 0 {
			fav = it.Composition[0].Product
		}
		out = append(out, RelasiInsight{Tone: "bad", Icon: "refresh", Dealer: it.ID,
			Title: fmt.Sprintf("%s: %d hari tanpa order (siklus %d) — intensitas WA turun", it.ShortName, *m.Last, *m.Rhythm),
			Text:  fmt.Sprintf("Hubungan menipis sebelum ordernya hilang (%s). Follow-up dengan %s.", seriesText(ser, r.MonthLabels), fav)})
	}
	for _, d := range dealers {
		es := touch[d]
		if len(es) < 2 || !inScope(d) {
			continue
		}
		sort.SliceStable(es, func(i, j int) bool { return es[i].W > es[j].W })
		var parts []string
		for _, e := range es {
			parts = append(parts, fmt.Sprintf("%s (%d)", salesName[e.Sales], e.W))
		}
		out = append(out, RelasiInsight{Tone: "accent", Icon: "net", Dealer: d, Title: fmt.Sprintf("%s disentuh %s", bySlug[d].ShortName, strings.Join(parts, " dan ")),
			Text: "Dua sales pada satu dealer — pastikan satu pemilik harga, satu pemilik piutang."})
	}
	for _, it := range b.Items {
		if it.Metrics.AvgOrder < 60_000_000 || !inScope(it.ID) {
			continue
		}
		if float64(period[it.ID])/float64(r.Months) >= 50 {
			continue
		}
		out = append(out, RelasiInsight{Tone: "warn", Icon: "chat", Dealer: it.ID, Title: fmt.Sprintf("%s: order besar, WA tipis", it.ShortName),
			Text: "Hubungan bertumpu transaksi. Risiko pindah distributor saat harga disentuh kompetitor."})
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func seriesText(s []int64, labels []string) string {
	if len(s) < 2 || len(labels) != len(s) {
		return ""
	}
	return fmt.Sprintf("%s %d → %s %d interaksi", labels[0], s[0], labels[len(s)-1], s[len(s)-1])
}
