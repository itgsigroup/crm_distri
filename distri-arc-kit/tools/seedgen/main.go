// Command seedgen turns the sample data of the approved mockup (tools/seedgen/mockup.json, extracted by
// extract_mockup.mjs) into concrete, Odoo-shaped seed records in db/seed/*.json: 18 dealers with 12 months of
// sale orders, invoices and payments, contacts, share-of-wallet confirmations, commitments, signals and stock.
//
// The records are synthesised so that internal/metrics (docs/design/01-glossary.md) computes exactly the
// mockup's numbers: siklus order is the median gap, besarnya the 6-month mean, sisa limit the open
// receivable, pola bayar / tepat waktu from invoice→payment days. Output is deterministic.
//
// Usage: go run ./tools/seedgen   (then: arc ctl seed)
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var wib = time.FixedZone("WIB", 7*3600)

// Anchor is "today" of the sample data: Senin, 5 Oktober 2026.
var anchor = time.Date(2026, 10, 5, 0, 0, 0, 0, wib)

const (
	window6m  = 182 // days counted as "6 bulan"
	window12m = 365
	jt        = int64(1_000_000)
)

// ---------- mockup.json ----------

type mPerson struct {
	N    string `json:"n"`
	Role string `json:"role"`
	S    int    `json:"s"`
}
type mCommit struct {
	T  string `json:"t"`
	S  string `json:"s"`
	St string `json:"st"`
	D  string `json:"d"`
}
type mTimeline struct {
	D   string `json:"d"`
	Via string `json:"via"`
	Who string `json:"who"`
	T   string `json:"t"`
	X   string `json:"x"`
}
type mDealer struct {
	ID       string  `json:"id"`
	Account  string  `json:"account"`
	City     string  `json:"city"`
	Branch   string  `json:"branch"`
	Tier     string  `json:"tier"`
	Seg      string  `json:"seg"`
	Owner    string  `json:"owner"`
	Limit    float64 `json:"limit"`
	Exposure float64 `json:"exposure"`
	Pay      int     `json:"pay"`
	OnTime   int     `json:"onTime"`
	Rhythm   int     `json:"rhythm"`
	Last     int     `json:"last"`
	Avg      float64 `json:"avg"`
	Porsi    int     `json:"porsi"`
	Prev     *struct {
		Rhythm int
		Avg    float64
	} `json:"prev"`
	Kr      []int     `json:"kr"`
	Fav     [][2]any  `json:"fav"`
	Months  []float64 `json:"months"`
	People  []mPerson `json:"people"`
	Memo    string    `json:"memo"`
	Commits struct {
		Kami   []mCommit `json:"kami"`
		Mereka []mCommit `json:"mereka"`
	} `json:"commits"`
	Timeline []mTimeline `json:"timeline"`
}
type mMsg struct {
	D   string `json:"d"`
	F   string `json:"f"`
	Who string `json:"who"`
	Int bool   `json:"int"`
	T   string `json:"t"`
	Tm  string `json:"tm"`
	Ann *struct {
		K   string `json:"k"`
		T   string `json:"t"`
		Act string `json:"act"`
	} `json:"ann"`
}
type mChat struct {
	ID     string                `json:"id"`
	Type   string                `json:"type"`
	Name   string                `json:"name"`
	Sub    string                `json:"sub"`
	Via    string                `json:"via"`
	Acc    string                `json:"acc"`
	Unread int                   `json:"unread"`
	Tag    struct{ K, T string } `json:"tag"`
	Sug    []string              `json:"sug"`
	Msgs   []mMsg                `json:"msgs"`
}
type mSales struct {
	ID     string `json:"id"`
	N      string `json:"n"`
	Branch string `json:"branch"`
	No     string `json:"no"`
}
type mockup struct {
	SALES   []mSales  `json:"SALES"`
	KAT     []string  `json:"KAT"`
	DEALERS []mDealer `json:"DEALERS"`
	CHATS   []mChat   `json:"CHATS"`
}

// ---------- output records ----------

type Line struct {
	Product  string `json:"product"`
	Category string `json:"category"`
	Qty      int64  `json:"qty"`
	Price    int64  `json:"price"`    // unit price (rounded)
	Subtotal int64  `json:"subtotal"` // authoritative line value, as Odoo price_subtotal
}
type Order struct {
	OdooID      string     `json:"odoo_id"`
	Number      string     `json:"number"`
	State       string     `json:"state"`
	OrderedAt   time.Time  `json:"ordered_at"`
	ConfirmedAt time.Time  `json:"confirmed_at"`
	ShippedAt   time.Time  `json:"shipped_at"`
	InvoicedAt  time.Time  `json:"invoiced_at"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	Total       int64      `json:"total"`
	MarginPct   float64    `json:"margin_pct"`
	Lines       []Line     `json:"lines"`
	daysAgo     int
	open        bool
	invNo       string
}
type Invoice struct {
	OdooID   string  `json:"odoo_id"`
	Number   string  `json:"number"`
	Order    string  `json:"order"`
	IssuedAt string  `json:"issued_at"`
	DueAt    string  `json:"due_at"`
	Total    int64   `json:"total"`
	Paid     int64   `json:"paid"`
	PaidAt   *string `json:"paid_at,omitempty"`
}
type Payment struct {
	OdooID  string `json:"odoo_id"`
	Invoice string `json:"invoice"`
	PaidAt  string `json:"paid_at"`
	Amount  int64  `json:"amount"`
}
type Contact struct {
	Key               string     `json:"key"`
	Name              string     `json:"name"`
	Role              string     `json:"role"`
	WANumber          string     `json:"wa_number"`
	IsPrimary         bool       `json:"is_primary"`
	Interactions90d   int        `json:"interactions_90d"`
	LastInteractionAt *time.Time `json:"last_interaction_at"`
}
type Commitment struct {
	Key     string  `json:"key"`
	Side    string  `json:"side"`
	Title   string  `json:"title"`
	Detail  string  `json:"detail"`
	Status  string  `json:"status"`
	DueAt   *string `json:"due_at,omitempty"`
	Invoice string  `json:"invoice,omitempty"`
}
type Dealer struct {
	Slug        string       `json:"slug"`
	OdooID      string       `json:"odoo_id"`
	Name        string       `json:"name"`
	City        string       `json:"city"`
	Branch      string       `json:"branch"`
	Tier        string       `json:"tier"`
	SegmentDesc string       `json:"segment_desc"`
	Owner       string       `json:"owner"`
	CreditLimit int64        `json:"credit_limit"`
	TermsDays   int          `json:"payment_terms_days"`
	Memo        string       `json:"memo"`
	Contacts    []Contact    `json:"contacts"`
	SOW         SOW          `json:"sow"`
	Orders      []*Order     `json:"orders"`
	Invoices    []*Invoice   `json:"invoices"`
	Payments    []Payment    `json:"payments"`
	Commitments []Commitment `json:"commitments"`
}
type SOW struct {
	Quarter string `json:"quarter"`
	SOW     int    `json:"sow"`
	Note    string `json:"note"`
}
type Signal struct {
	DedupeKey  string         `json:"dedupe_key"`
	Kind       string         `json:"kind"`
	Dealer     string         `json:"dealer,omitempty"`
	Contact    string         `json:"contact,omitempty"`
	Sales      string         `json:"sales,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
	Summary    string         `json:"summary"`
	Payload    map[string]any `json:"payload"`
}
type Stock struct {
	OdooID         string  `json:"odoo_id"`
	Branch         string  `json:"branch"`
	SKU            string  `json:"sku"`
	Name           string  `json:"name"`
	Category       string  `json:"category"`
	Qty            int64   `json:"qty"`
	UnitCost       int64   `json:"unit_cost"`
	AgeDays        int     `json:"age_days"`
	WeeklyVelocity float64 `json:"weekly_velocity"`
}
type SalesUser struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Branch   string `json:"branch"`
	WANumber string `json:"wa_number"`
	Role     string `json:"role"`
	OdooUser int    `json:"odoo_user_id"`
	Email    string `json:"email"`
}

// ---------- catalog ----------

type product struct {
	cat    string
	price  int64
	margin float64
}

var catalog = map[string]product{
	"Kamera IP 4MP":        {"Kamera & NVR", 1_150_000, 12},
	"Kamera IP 2MP":        {"Kamera & NVR", 650_000, 12},
	"Kamera analog":        {"Kamera & NVR", 300_000, 14},
	"Aksesoris kamera":     {"Kamera & NVR", 120_000, 18},
	"NVR 8ch":              {"Kamera & NVR", 1_800_000, 11},
	"NVR 16ch":             {"Kamera & NVR", 3_700_000, 11},
	"NVR 32ch":             {"Kamera & NVR", 5_100_000, 11},
	"NVR 16/32ch":          {"Kamera & NVR", 4_200_000, 11},
	"DVR 4ch":              {"Kamera & NVR", 900_000, 13},
	"DVR":                  {"Kamera & NVR", 1_000_000, 13},
	"HDD 4TB":              {"HDD & storage", 1_650_000, 7},
	"HDD":                  {"HDD & storage", 1_300_000, 7},
	"HDD 1TB":              {"HDD & storage", 650_000, 7},
	"Kabel & aksesoris":    {"Kabel & PoE", 950_000, 13},
	"Kabel":                {"Kabel & PoE", 900_000, 13},
	"Kabel UTP (roll)":     {"Kabel & PoE", 900_000, 13},
	"Kabel FRC":            {"Kabel & PoE", 1_100_000, 14},
	"Switch PoE":           {"Kabel & PoE", 1_250_000, 12},
	"Modul LED P5":         {"Modul LED", 520_000, 16},
	"Modul LED P4":         {"Modul LED", 600_000, 16},
	"Controller":           {"Modul LED", 450_000, 16},
	"Detektor addressable": {"Fire alarm", 750_000, 19},
	"Detektor":             {"Fire alarm", 350_000, 19},
	"Panel":                {"Fire alarm", 9_500_000, 17},
	"Power supply":         {"Aksesoris", 250_000, 18},
	"Aksesoris":            {"Aksesoris", 150_000, 20},
	"Konektor RJ45 (box)":  {"Aksesoris", 250_000, 20},
}

// filler bought once in 6 months so product mix matches the mockup's kr flags.
var filler = map[string]string{
	"Kamera & NVR": "Kamera IP 2MP", "HDD & storage": "HDD 1TB", "Kabel & PoE": "Kabel UTP (roll)",
	"Modul LED": "Modul LED P4", "Fire alarm": "Detektor", "Aksesoris": "Konektor RJ45 (box)",
}

// favRename keeps a dealer's product composition label while fixing its category (Sumber Rejeki only buys
// camera goods; its "Aksesoris" share are camera accessories, so product mix stays 1/6 as in the mockup).
var favRename = map[string]map[string]string{"rejeki": {"Aksesoris": "Aksesoris kamera"}}

// ---------- per-dealer receivable anchors ----------

type openInv struct {
	daysAgo int
	totalJt float64
	number  string
}

// open lists the unpaid invoices (order age in days, amount) that make up each dealer's exposure, chosen to
// match the mockup texts (INV/0844 lewat 17 hari, INV/0889 lewat 11 hari, INV/0901 jatuh tempo 5 Okt, ...).
var open = map[string][]openInv{
	"sinar":    {{6, 118, ""}, {26, 58, "INV/0912"}},
	"graha":    {{3, 85, ""}, {23, 40, ""}, {47, 95, "INV/0844"}},
	"mitra":    {{30, 116, ""}, {41, 46, "INV/0889"}},
	"indo":     {{2, 92, ""}, {12, 46, ""}, {22, 46, ""}},
	"bina":     {{25, 40, ""}},
	"anugerah": {{8, 35, ""}},
	"sarana":   {{4, 98, ""}, {16, 22, ""}},
	"prima":    {{17, 22, ""}},
	"jaya":     {{13, 34, ""}, {27, 26, ""}},
	"nusa":     {{20, 57, ""}, {30, 41, "INV/0901"}},
	"borneo":   {{15, 40, ""}},
	"global":   {{2, 90, ""}},
}

var odooPartner = map[string]int{}

// reservedInv are the invoice numbers quoted by the mockup; sequential numbering skips them.
var reservedInv = func() map[string]bool {
	m := map[string]bool{}
	for _, list := range open {
		for _, o := range list {
			if o.number != "" {
				m[o.number] = true
			}
		}
	}
	return m
}()

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	raw, err := os.ReadFile(filepath.Join(root, "tools/seedgen/mockup.json"))
	must(err)
	var m mockup
	must(json.Unmarshal(raw, &m))

	sales := buildSales(m)
	salesByName := map[string]SalesUser{}
	for _, s := range sales {
		salesByName[s.Name] = s
	}

	var dealers []*Dealer
	invSeq, soSeq := 700, 4100
	for i, md := range m.DEALERS {
		odooPartner[md.ID] = 3001 + i
		d := buildDealer(md, salesByName, &soSeq, &invSeq)
		dealers = append(dealers, d)
	}
	signals := buildSignals(m, dealers, salesByName)
	stock := buildStock(dealers)
	prev := map[string]any{}
	for _, md := range m.DEALERS {
		if md.Prev != nil {
			prev[md.ID] = map[string]any{"rhythm_days": md.Prev.Rhythm, "avg_order": int64(md.Prev.Avg), "as_of": anchor.AddDate(0, -3, 0).Format("2006-01-02")}
		}
	}

	out := filepath.Join(root, "db/seed")
	must(os.MkdirAll(out, 0o755))
	write(filepath.Join(out, "sales.json"), sales)
	write(filepath.Join(out, "dealers.json"), dealers)
	write(filepath.Join(out, "signals.json"), signals)
	write(filepath.Join(out, "stock.json"), stock)
	write(filepath.Join(out, "metrics_prev.json"), prev)
	write(filepath.Join(out, "chats.json"), buildChats(m))
	nOrders, nWA := 0, 0
	for _, d := range dealers {
		nOrders += len(d.Orders)
	}
	for _, s := range signals {
		if s.Kind == "wa" || s.Kind == "wa_group" {
			nWA++
		}
	}
	fmt.Printf("seed: %d sales, %d dealers, %d orders, %d signals (%d WA), %d stock items\n", len(sales), len(dealers), nOrders, len(signals), nWA, len(stock))
}

func buildSales(m mockup) []SalesUser {
	nums := map[string]string{"Andi": "6281234504471", "Dewi": "6281534509032", "Rizky": "6281134502210", "Fajar": "6281334507785"}
	out := []SalesUser{}
	for i, s := range m.SALES {
		out = append(out, SalesUser{Key: strings.TrimPrefix(s.ID, "s-"), Name: s.N, Branch: s.Branch, WANumber: nums[s.N], Role: "sales", OdooUser: 11 + i, Email: strings.ToLower(s.N) + "@gsi.co.id"})
	}
	out = append(out,
		SalesUser{Key: "sam", Name: "Sam Setiadi", Branch: "Semua cabang", WANumber: "6281100000001", Role: "ceo", OdooUser: 2, Email: "sam@gsi.co.id"},
		SalesUser{Key: "admin", Name: "Admin Distri", Branch: "Semarang", WANumber: "6281100000002", Role: "admin", OdooUser: 3, Email: "admin@gsi.co.id"},
		SalesUser{Key: "finance", Name: "Rina Keuangan", Branch: "Semarang", WANumber: "6281100000003", Role: "finance", OdooUser: 4, Email: "finance@gsi.co.id"},
	)
	return out
}

func day(daysAgo int, hour int) time.Time {
	return anchor.AddDate(0, 0, -daysAgo).Add(time.Duration(hour) * time.Hour)
}
func ymd(t time.Time) string { return t.Format("2006-01-02") }

func buildDealer(md mDealer, sales map[string]SalesUser, soSeq, invSeq *int) *Dealer {
	cash := md.Limit == 0
	terms := 30
	if cash || md.Rhythm == 0 {
		terms = 0
	}
	d := &Dealer{
		Slug: md.ID, OdooID: fmt.Sprintf("res.partner:%d", odooPartner[md.ID]), Name: md.Account, City: md.City, Branch: md.Branch,
		Tier: md.Tier, SegmentDesc: md.Seg, Owner: sales[md.Owner].Key, CreditLimit: int64(md.Limit), TermsDays: terms, Memo: md.Memo,
		SOW: SOW{Quarter: "2026-Q3", SOW: md.Porsi, Note: "Dikonfirmasi sales " + md.Owner},
	}

	// --- order schedule (days ago) ---
	opens := open[md.ID]
	explicit := []int{md.Last}
	totals := map[int]int64{}
	invNos := map[int]string{}
	for _, o := range opens {
		if o.daysAgo != md.Last {
			explicit = append(explicit, o.daysAgo)
		}
		totals[o.daysAgo] = int64(o.totalJt * float64(jt))
		invNos[o.daysAgo] = o.number
	}
	sort.Ints(explicit)
	var sched []int
	if md.Rhythm == 0 {
		sched = []int{md.Last}
	} else {
		sched = append(sched, explicit...)
		for x := explicit[len(explicit)-1] + md.Rhythm; x <= window6m; x += md.Rhythm {
			sched = append(sched, x)
		}
	}
	// older history (7–12 months) only shapes "tepat waktu"; its gap is searched so the integer % matches.
	older := olderHistory(md, sched, terms, opens)
	sched = append(sched, older...)

	// --- order values: 6-month mean is exactly besarnya ---
	var in6 []int
	for _, x := range sched {
		if x <= window6m {
			in6 = append(in6, x)
		}
	}
	avg := int64(md.Avg)
	target := avg * int64(len(in6))
	var fixedSum int64
	for _, x := range in6 {
		fixedSum += totals[x]
	}
	months := []string{"05", "06", "07", "08", "09", "10"}
	meanNZ := 0.0
	nz := 0
	for _, v := range md.Months {
		if v > 0 {
			meanNZ += v
			nz++
		}
	}
	if nz > 0 {
		meanNZ /= float64(nz)
	} else {
		meanNZ = 1
	}
	weight := func(x int) float64 {
		t := day(x, 0)
		mm := t.Format("01")
		for i, k := range months {
			if k == mm && md.Months[i] > 0 {
				return md.Months[i]
			}
		}
		return meanNZ
	}
	var free []int
	var wsum float64
	for _, x := range in6 {
		if _, ok := totals[x]; !ok {
			free = append(free, x)
			wsum += weight(x)
		}
	}
	remaining := target - fixedSum
	if len(free) > 0 && remaining <= 0 {
		log.Fatalf("%s: open invoices exceed 6-month volume", md.ID)
	}
	var assigned int64
	for i, x := range free {
		v := int64(math.Round(float64(remaining)*weight(x)/wsum/100_000)) * 100_000
		if i == len(free)-1 {
			v = remaining - assigned
		}
		totals[x] = v
		assigned += v
	}
	for _, x := range sched {
		if x > window6m {
			totals[x] = int64(math.Round(md.Avg*0.95/100_000)) * 100_000
		}
	}

	// --- lines ---
	fav := md.Fav
	type share struct {
		name string
		pct  float64
	}
	var shares []share
	cats := map[string]bool{}
	for _, f := range fav {
		name := f[0].(string)
		if r, ok := favRename[md.ID][name]; ok {
			name = r
		}
		p, ok := catalog[name]
		if !ok {
			log.Fatalf("%s: product %q not in catalog", md.ID, name)
		}
		cats[p.cat] = true
		shares = append(shares, share{name, f[1].(float64)})
	}
	var missing []string
	kat := []string{"Kamera & NVR", "HDD & storage", "Kabel & PoE", "Modul LED", "Fire alarm", "Aksesoris"}
	for i, k := range kat {
		if md.Kr[i] == 1 && !cats[k] {
			missing = append(missing, k)
		}
		if md.Kr[i] == 0 && cats[k] {
			log.Fatalf("%s: favourite product in category %s which the mockup marks as never bought", md.ID, k)
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(sched))) // oldest first for numbering
	byAge := map[int]*Order{}
	fillerPlaced := false
	for _, x := range sched {
		*soSeq++
		o := &Order{
			OdooID: fmt.Sprintf("sale.order:%d", *soSeq), Number: fmt.Sprintf("S%05d", *soSeq), State: "bayar",
			OrderedAt: day(x, 9), ConfirmedAt: day(x, 9).Add(20 * time.Minute), ShippedAt: day(x, 14), InvoicedAt: day(x, 15),
			Total: totals[x], daysAgo: x, invNo: invNos[x],
		}
		rest := o.Total
		var lines []Line
		// filler goes into the most recent free 6-month order so it stays inside the window
		if !fillerPlaced && len(missing) > 0 && x <= window6m && x == firstFree(in6, totals, invNos, opens) {
			for _, k := range missing {
				fp := catalog[filler[k]]
				lines = append(lines, Line{Product: filler[k], Category: k, Qty: 1, Price: fp.price, Subtotal: fp.price})
				rest -= fp.price
			}
			fillerPlaced = true
		}
		var used int64
		for i, s := range shares {
			v := int64(math.Round(float64(rest) * s.pct / 100))
			if i == len(shares)-1 {
				v = rest - used
			}
			used += v
			p := catalog[s.name]
			q := int64(math.Max(1, math.Round(float64(v)/float64(p.price))))
			lines = append(lines, Line{Product: s.name, Category: p.cat, Qty: q, Price: v / q, Subtotal: v})
		}
		var cost float64
		for _, l := range lines {
			cost += float64(l.Subtotal) * (1 - catalog[l.Product].margin/100)
		}
		o.Lines = lines
		o.MarginPct = math.Round((1-cost/float64(o.Total))*10000) / 100
		d.Orders = append(d.Orders, o)
		byAge[x] = o
	}
	if len(missing) > 0 && !fillerPlaced {
		log.Fatalf("%s: could not place filler lines", md.ID)
	}

	// --- invoices & payments ---
	payDays := paymentDays(md, sched, terms, opens)
	for _, x := range sched {
		o := byAge[x]
		no := o.invNo
		if no == "" {
			for {
				*invSeq++
				no = fmt.Sprintf("INV/%04d", *invSeq)
				if !reservedInv[no] {
					break
				}
			}
		}
		issued := day(x, 0)
		inv := &Invoice{OdooID: "account.move:" + strings.TrimPrefix(no, "INV/"), Number: no, Order: o.Number, IssuedAt: ymd(issued), DueAt: ymd(issued.AddDate(0, 0, terms)), Total: o.Total}
		if pd, ok := payDays[x]; ok {
			p := issued.AddDate(0, 0, pd)
			ps := ymd(p)
			inv.Paid, inv.PaidAt = o.Total, &ps
			pt := p.Add(11 * time.Hour)
			o.PaidAt = &pt
			d.Payments = append(d.Payments, Payment{OdooID: "account.payment:" + strings.TrimPrefix(no, "INV/"), Invoice: no, PaidAt: ps, Amount: o.Total})
		} else {
			o.State = "invoice"
			o.open = true
		}
		d.Invoices = append(d.Invoices, inv)
	}

	// --- contacts ---
	numBase := 6281900000000 + int64(odooPartner[md.ID])*100
	maxS := -1
	for _, p := range md.People {
		if p.S > maxS {
			maxS = p.S
		}
	}
	primaryDone := false
	for i, p := range md.People {
		c := Contact{Key: md.ID + ":" + slugify(p.N), Name: p.N, Role: p.Role, WANumber: fmt.Sprintf("%d", numBase+int64(i+1))}
		switch {
		case p.S >= 3:
			c.Interactions90d = 24 + 3*i
			c.LastInteractionAt = ptr(day(1+i, 10))
		case p.S == 2:
			c.Interactions90d = 7 + i
			c.LastInteractionAt = ptr(day(4+2*i, 11))
		case p.S == 1:
			c.Interactions90d = 0
			c.LastInteractionAt = ptr(day(112+7*i, 10))
		}
		if p.S == maxS && p.S >= 2 && !primaryDone {
			c.IsPrimary, primaryDone = true, true
		}
		d.Contacts = append(d.Contacts, c)
	}

	// --- commitments ---
	for i, c := range md.Commits.Kami {
		d.Commitments = append(d.Commitments, Commitment{Key: fmt.Sprintf("%s:kami:%d", md.ID, i), Side: "kami", Title: c.T, Detail: c.S, Status: c.St})
	}
	for i, c := range md.Commits.Mereka {
		cm := Commitment{Key: fmt.Sprintf("%s:mereka:%d", md.ID, i), Side: "mereka", Title: c.T, Detail: c.S, Status: c.St}
		for _, inv := range d.Invoices {
			if strings.Contains(c.T, inv.Number) {
				cm.Invoice = inv.Number
				due := inv.DueAt
				cm.DueAt = &due
				t, _ := time.ParseInLocation("2006-01-02", due, wib)
				cm.Detail = "Jatuh tempo " + idDate(t)
				if c.St != "late" {
					cm.Detail += fmt.Sprintf(" · pola %d hari", md.Pay)
				}
			}
		}
		d.Commitments = append(d.Commitments, cm)
	}
	return d
}

func firstFree(in6 []int, totals map[int]int64, invNos map[int]string, opens []openInv) int {
	isOpen := map[int]bool{}
	for _, o := range opens {
		isOpen[o.daysAgo] = true
	}
	best := -1
	for _, x := range in6 {
		if !isOpen[x] && (best == -1 || x < best) {
			best = x
		}
	}
	if best == -1 { // every 6-month order is open: use the most recent one
		for _, x := range in6 {
			if best == -1 || x < best {
				best = x
			}
		}
	}
	return best
}

// olderHistory picks the 7–12 month order gap so that round(on-time %) can equal the mockup value exactly.
func olderHistory(md mDealer, sched []int, terms int, opens []openInv) []int {
	if md.Rhythm == 0 {
		return nil
	}
	last := sched[len(sched)-1]
	try := func(gap int) []int {
		var out []int
		for x := last + gap; x <= window12m; x += gap {
			out = append(out, x)
		}
		return out
	}
	if terms == 0 { // cash: everything on time
		return try(md.Rhythm)
	}
	for delta := 0; delta <= 20; delta++ {
		for _, g := range []int{md.Rhythm + delta, md.Rhythm - delta} {
			if g < 7 {
				continue
			}
			o := try(g)
			if _, ok := lateCount(md, append(append([]int{}, sched...), o...), terms, opens); ok {
				return o
			}
		}
	}
	log.Fatalf("%s: no history length gives on-time %d%%", md.ID, md.OnTime)
	return nil
}

// lateCount returns how many paid invoices must be late so that on-time % rounds to the mockup value.
// Denominator: invoices issued within 12 months that are paid or already past due.
func lateCount(md mDealer, sched []int, terms int, opens []openInv) (int, bool) {
	isOpen := map[int]bool{}
	for _, o := range opens {
		isOpen[o.daysAgo] = true
	}
	den, openLate, paid := 0, 0, 0
	for _, x := range sched {
		if x > window12m {
			continue
		}
		if isOpen[x] {
			if x > terms { // past due and unpaid
				den++
				openLate++
			}
			continue
		}
		den++
		paid++
	}
	for L := 0; L <= paid; L++ {
		if int(math.Round(100*float64(den-L-openLate)/float64(den))) == md.OnTime {
			return L, true
		}
	}
	return 0, false
}

// paymentDays assigns invoice→payment days to every paid invoice: mean over 6 months equals pola bayar,
// and exactly lateCount invoices exceed the terms.
func paymentDays(md mDealer, sched []int, terms int, opens []openInv) map[int]int {
	isOpen := map[int]bool{}
	for _, o := range opens {
		isOpen[o.daysAgo] = true
	}
	res := map[int]int{}
	var paid []int
	for _, x := range sched {
		if !isOpen[x] {
			paid = append(paid, x)
		}
	}
	sort.Ints(paid)
	if terms == 0 {
		for _, x := range paid {
			res[x] = 0
		}
		return res
	}
	L, ok := lateCount(md, sched, terms, opens)
	if !ok {
		log.Fatalf("%s: on-time not reachable", md.ID)
	}
	var in6, out6 []int
	for _, x := range paid {
		if x <= window6m {
			in6 = append(in6, x)
		} else {
			out6 = append(out6, x)
		}
	}
	S := md.Pay * len(in6)
	var late map[int]bool
	// Candidates for late payment must be old enough to have been paid after the terms.
	cands := func(xs []int) []int {
		var c []int
		for _, x := range xs {
			if x > terms+1 {
				c = append(c, x)
			}
		}
		return c
	}
	lateCap := func(x int) int { return int(math.Min(float64(x-1), float64(terms+60))) }
	onCap := func(x int) int { return int(math.Min(float64(terms), float64(x-1))) }
	// decide how many late ones go inside the 6-month window: the more the mean exceeds terms, the more.
	c6, cOld := cands(in6), cands(out6)
	for k := 0; k <= L; k++ {
		if k > len(c6) || L-k > len(cOld) {
			continue
		}
		minS, maxS := 0, 0
		late = map[int]bool{}
		// spread late ones over the window
		for i := 0; i < k; i++ {
			late[c6[len(c6)-1-i*len(c6)/max(k, 1)%len(c6)]] = true
		}
		if len(late) != k {
			late = map[int]bool{}
			for i := 0; i < k; i++ {
				late[c6[len(c6)-1-i]] = true
			}
		}
		for _, x := range in6 {
			if late[x] {
				minS += terms + 1
				maxS += lateCap(x)
			} else {
				minS += min(3, x-1)
				maxS += onCap(x)
			}
		}
		if S < minS || S > maxS {
			continue
		}
		// assign: start from lower bounds and distribute the surplus evenly
		cur := map[int]int{}
		sum := 0
		for _, x := range in6 {
			if late[x] {
				cur[x] = terms + 1
			} else {
				cur[x] = min(3, x-1)
			}
			sum += cur[x]
		}
		for sum < S {
			progressed := false
			for _, x := range in6 {
				if sum >= S {
					break
				}
				hi := onCap(x)
				if late[x] {
					hi = lateCap(x)
				}
				if cur[x] < hi {
					cur[x]++
					sum++
					progressed = true
				}
			}
			if !progressed {
				break
			}
		}
		for x, v := range cur {
			res[x] = v
		}
		// older ones: L-k late, rest on time
		n := 0
		for i := len(cOld) - 1; i >= 0 && n < L-k; i-- {
			res[cOld[i]] = min(terms+18, cOld[i]-1)
			n++
		}
		for _, x := range out6 {
			if _, ok := res[x]; !ok {
				res[x] = terms - 2
			}
		}
		return res
	}
	log.Fatalf("%s: cannot reach pola bayar %d with on-time %d", md.ID, md.Pay, md.OnTime)
	return nil
}

var idMonths = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func idDate(t time.Time) string { return fmt.Sprintf("%d %s", t.Day(), idMonths[t.Month()-1]) }

func parseIDDate(s string) time.Time {
	var dd int
	var mon string
	if _, err := fmt.Sscanf(s, "%d %s", &dd, &mon); err != nil {
		log.Fatalf("bad date %q: %v", s, err)
	}
	for i, m := range idMonths {
		if m == mon {
			return time.Date(2026, time.Month(i+1), dd, 10, 0, 0, 0, wib)
		}
	}
	log.Fatalf("bad date %q", s)
	return time.Time{}
}

func buildSignals(m mockup, dealers []*Dealer, sales map[string]SalesUser) []Signal {
	var out []Signal
	byName := map[string]*Dealer{}
	for _, d := range dealers {
		byName[d.Slug] = d
	}
	contactKey := func(d *Dealer, who string) string {
		for _, c := range d.Contacts {
			if c.Name == who {
				return c.Key
			}
		}
		return ""
	}
	// 1) dealer timeline (interactions + agent conclusion) from the mockup
	for _, md := range m.DEALERS {
		d := byName[md.ID]
		for i, t := range md.Timeline {
			kind := map[string]string{"chat": "wa", "box": "so", "doc": "manual"}[t.Via]
			if t.Via == "box" && t.Who == "Pembayaran" {
				kind = "payment"
			}
			at := parseIDDate(t.D)
			if kind == "so" && i == len(md.Timeline)-1 && strings.Contains(t.T, "terakhir") {
				// "Order terakhir" must agree with the generated last order
				for _, o := range d.Orders {
					if o.daysAgo == md.Last {
						at = o.ConfirmedAt
						t.T = fmt.Sprintf("Order terakhir %s.", fmtRp(o.Total))
					}
				}
			}
			out = append(out, Signal{
				DedupeKey: fmt.Sprintf("seed:timeline:%s:%d", md.ID, i), Kind: kind, Dealer: md.ID, Contact: contactKey(d, t.Who), Sales: d.Owner,
				OccurredAt: at, Summary: t.T,
				Payload: map[string]any{"via": t.Via, "who": t.Who, "text": t.T, "conclusion": t.X, "source": "seed"},
			})
		}
	}
	// 2) WhatsApp messages of the mockup chat threads
	for _, c := range m.CHATS {
		dayLabel := "Hari ini"
		for i, msg := range c.Msgs {
			if msg.D != "" {
				dayLabel = msg.D
				continue
			}
			at := chatTime(dayLabel, msg.Tm)
			kind := "wa"
			if c.Type == "gint" {
				kind = "wa_group"
			}
			p := map[string]any{"thread": c.ID, "direction": msg.F, "text": msg.T, "via": "chat", "source": "seed", "sales": strings.ToLower(c.Via)}
			if msg.Who != "" {
				p["from_name"] = msg.Who
				p["internal"] = msg.Int
			}
			if msg.Ann != nil {
				p["annotation"] = map[string]any{"k": msg.Ann.K, "t": msg.Ann.T, "act": msg.Ann.Act}
			}
			s := Signal{DedupeKey: fmt.Sprintf("seed:wa:%s:%d", c.ID, i), Kind: kind, Dealer: c.Acc, Sales: strings.ToLower(c.Via), OccurredAt: at, Summary: msg.T, Payload: p}
			if d, ok := byName[c.Acc]; ok {
				who := strings.TrimSpace(strings.Split(c.Name, "·")[0])
				s.Contact = contactKey(d, who)
			}
			out = append(out, s)
		}
	}
	// 3) synthetic WhatsApp history (order requests and confirmations) for every active contact
	templates := [][2]string{
		{"%s, stok %s masih ada? Mau ambil untuk minggu ini.", "Ada %s, siap kirim. Saya buatkan SO-nya ya."},
		{"Tolong kirim seperti biasa ya, %s.", "Siap, dikirim besok pagi %s. Terima kasih."},
		{"Invoice kemarin sudah kami transfer ya.", "Sudah masuk, terima kasih banyak."},
	}
	n := 0
	for _, d := range dealers {
		var fav string
		for _, o := range d.Orders {
			if len(o.Lines) > 0 {
				fav = o.Lines[len(o.Lines)-1].Product
			}
		}
		owner := sales[strings.ToUpper(d.Owner[:1])+d.Owner[1:]]
		for ci, c := range d.Contacts {
			if c.LastInteractionAt == nil || c.Interactions90d == 0 {
				continue
			}
			tp := templates[(n+ci)%len(templates)]
			at := c.LastInteractionAt.Add(-time.Duration(24*(2+ci)) * time.Hour)
			in := tp[0]
			if strings.Count(in, "%s") == 2 {
				in = fmt.Sprintf(in, salutation(owner.Name), fav)
			} else if strings.Count(in, "%s") == 1 {
				in = fmt.Sprintf(in, salutation(owner.Name))
			}
			outText := tp[1]
			if strings.Count(outText, "%s") == 1 {
				outText = fmt.Sprintf(outText, honor(c.Name))
			}
			for j, txt := range []string{in, outText} {
				dir := "in"
				if j == 1 {
					dir = "out"
				}
				out = append(out, Signal{
					DedupeKey: fmt.Sprintf("seed:wa:hist:%s:%d", c.Key, j), Kind: "wa", Dealer: d.Slug, Contact: c.Key, Sales: d.Owner,
					OccurredAt: at.Add(time.Duration(j*17) * time.Minute), Summary: txt,
					Payload: map[string]any{"thread": "c-" + d.Slug, "direction": dir, "text": txt, "via": "chat", "source": "seed", "history": true},
				})
			}
			n++
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].OccurredAt.Before(out[j].OccurredAt) })
	return out
}

func salutation(sales string) string {
	if sales == "Dewi" {
		return "Bu " + sales
	}
	return "Mas " + sales
}

func honor(name string) string {
	for _, p := range []string{"Pak ", "Bu ", "Mbak ", "Mas "} {
		if strings.HasPrefix(name, p) {
			return name
		}
	}
	return "Pak " + name
}

func chatTime(dayLabel, hm string) time.Time {
	var h, mi int
	if _, err := fmt.Sscanf(strings.Replace(hm, ".", ":", 1), "%d:%d", &h, &mi); err != nil {
		log.Fatalf("bad time %q: %v", hm, err)
	}
	base := anchor
	if dayLabel == "Kemarin" {
		base = anchor.AddDate(0, 0, -1)
	}
	return base.Add(time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute)
}

// buildChats keeps the presentation hints of the mockup threads (suggestions, tags, unread) for the
// WhatsApp fake transport (stage 03); the messages themselves are the wa signals above.
func buildChats(m mockup) []map[string]any {
	var out []map[string]any
	for _, c := range m.CHATS {
		out = append(out, map[string]any{"id": c.ID, "type": c.Type, "name": c.Name, "sub": c.Sub, "via": strings.ToLower(c.Via), "dealer": c.Acc, "unread": c.Unread, "tag": map[string]string{"k": c.Tag.K, "t": c.Tag.T}, "suggestions": c.Sug})
	}
	return out
}

func buildStock(dealers []*Dealer) []Stock {
	items := []Stock{
		// aging (> 90 hari) — the push-stock candidates of the mockup
		{Branch: "Semarang", SKU: "LED-P5-OUT", Name: "Modul LED P5 outdoor", Category: "Modul LED", Qty: 420, UnitCost: 500_000, AgeDays: 148, WeeklyVelocity: 6},
		{Branch: "Surabaya", SKU: "NVR-32CH", Name: "NVR 32 channel", Category: "Kamera & NVR", Qty: 14, UnitCost: 6_000_000, AgeDays: 97, WeeklyVelocity: 1},
		{Branch: "Jakarta", SKU: "FA-DET-KONV", Name: "Detektor asap konvensional", Category: "Fire alarm", Qty: 260, UnitCost: 207_692, AgeDays: 210, WeeklyVelocity: 2},
		{Branch: "Yogyakarta", SKU: "CAM-AN-2MP", Name: "Kamera analog 2MP", Category: "Kamera & NVR", Qty: 88, UnitCost: 250_000, AgeDays: 76, WeeklyVelocity: 5},
		// critical (habis < 10 hari pada siklus order sekarang)
		{Branch: "Semarang", SKU: "CAM-IP4-DOME", Name: "Kamera IP 4MP dome", Category: "Kamera & NVR", Qty: 18, UnitCost: 1_010_000, AgeDays: 12, WeeklyVelocity: 24},
		{Branch: "Yogyakarta", SKU: "HDD-4TB-SV", Name: "HDD 4TB surveillance", Category: "HDD & storage", Qty: 6, UnitCost: 1_530_000, AgeDays: 9, WeeklyVelocity: 9},
		{Branch: "Surabaya", SKU: "PSU-12V10A", Name: "Power supply 12V 10A", Category: "Aksesoris", Qty: 30, UnitCost: 205_000, AgeDays: 15, WeeklyVelocity: 35},
		{Branch: "Yogyakarta", SKU: "NVR-8CH", Name: "NVR 8ch", Category: "Kamera & NVR", Qty: 5, UnitCost: 1_600_000, AgeDays: 20, WeeklyVelocity: 4},
		{Branch: "Jakarta", SKU: "SW-POE-8", Name: "Switch PoE 8 port", Category: "Kabel & PoE", Qty: 9, UnitCost: 1_100_000, AgeDays: 18, WeeklyVelocity: 7},
		{Branch: "Semarang", SKU: "UTP-CAT6", Name: "Kabel UTP Cat6 (roll)", Category: "Kabel & PoE", Qty: 12, UnitCost: 780_000, AgeDays: 22, WeeklyVelocity: 10},
		{Branch: "Surabaya", SKU: "CAM-IP2-BUL", Name: "Kamera IP 2MP bullet", Category: "Kamera & NVR", Qty: 20, UnitCost: 570_000, AgeDays: 14, WeeklyVelocity: 16},
		// healthy stock
		{Branch: "Jakarta", SKU: "CAM-IP4-DOME", Name: "Kamera IP 4MP dome", Category: "Kamera & NVR", Qty: 210, UnitCost: 1_010_000, AgeDays: 24, WeeklyVelocity: 30},
	}
	// Fill the rest so that perputaran stok (stock value ÷ daily COGS of the last 90 days) is 46 hari.
	var cogs90 float64
	for _, d := range dealers {
		for _, o := range d.Orders {
			if anchor.Sub(o.ConfirmedAt).Hours()/24 <= 90 {
				cogs90 += float64(o.Total) * (1 - o.MarginPct/100)
			}
		}
	}
	targetValue := int64(math.Round(46 * cogs90 / 90))
	var have int64
	for _, s := range items {
		have += s.Qty * s.UnitCost
	}
	fill := []Stock{
		{Branch: "Semarang", SKU: "NVR-16CH", Name: "NVR 16ch", Category: "Kamera & NVR", UnitCost: 3_290_000, AgeDays: 31, WeeklyVelocity: 6},
		{Branch: "Yogyakarta", SKU: "CAM-IP4-BUL", Name: "Kamera IP 4MP bullet", Category: "Kamera & NVR", UnitCost: 1_010_000, AgeDays: 28, WeeklyVelocity: 18},
		{Branch: "Yogyakarta", SKU: "FA-DET-ADR", Name: "Detektor addressable", Category: "Fire alarm", UnitCost: 610_000, AgeDays: 35, WeeklyVelocity: 12},
		{Branch: "Surabaya", SKU: "HDD-4TB-SV", Name: "HDD 4TB surveillance", Category: "HDD & storage", UnitCost: 1_530_000, AgeDays: 26, WeeklyVelocity: 14},
		{Branch: "Jakarta", SKU: "LED-P4-IN", Name: "Modul LED P4 indoor", Category: "Modul LED", UnitCost: 505_000, AgeDays: 40, WeeklyVelocity: 20},
		{Branch: "Jakarta", SKU: "NVR-32CH", Name: "NVR 32 channel", Category: "Kamera & NVR", UnitCost: 4_540_000, AgeDays: 33, WeeklyVelocity: 3},
	}
	gap := targetValue - have
	if gap <= 0 {
		log.Fatalf("stock calibration: base items already exceed target %d", targetValue)
	}
	per := gap / int64(len(fill))
	var added int64
	for i := range fill {
		q := per / fill[i].UnitCost
		if i == len(fill)-1 {
			q = (gap - added) / fill[i].UnitCost
		}
		fill[i].Qty = q
		added += q * fill[i].UnitCost
	}
	items = append(items, fill...)
	for i := range items {
		items[i].OdooID = fmt.Sprintf("stock.quant:%d", 5001+i)
	}
	return items
}

func fmtRp(v int64) string {
	if v >= 1_000_000_000 {
		s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(v)/1e9), "0"), ".")
		return "Rp " + strings.Replace(s, ".", ",", 1) + " M"
	}
	return fmt.Sprintf("Rp %d jt", int64(math.Round(float64(v)/1e6)))
}

func slugify(s string) string {
	s = strings.ToLower(s)
	for _, p := range []string{"pak ", "bu ", "mbak ", "mas "} {
		s = strings.TrimPrefix(s, p)
	}
	return strings.ReplaceAll(s, " ", "-")
}

func ptr[T any](v T) *T { return &v }

func write(path string, v any) {
	b, err := json.MarshalIndent(v, "", " ")
	must(err)
	must(os.WriteFile(path, append(b, '\n'), 0o644))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
