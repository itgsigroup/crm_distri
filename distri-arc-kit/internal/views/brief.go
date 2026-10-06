package views

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
)

// BriefDealer is a dealer quoted by a brief point.
type BriefDealer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	DueIn       *int   `json:"due_in,omitempty"`
	Last        *int   `json:"last_order_days,omitempty"`
	Rhythm      *int   `json:"rhythm_days,omitempty"`
	Status      string `json:"status,omitempty"`
	CreditState string `json:"credit_state,omitempty"`
	ExposurePct int    `json:"exposure_pct,omitempty"`
	LateDays    int    `json:"late_days,omitempty"`
	LateInvoice string `json:"late_invoice,omitempty"`
	RoomPct     int    `json:"room_pct,omitempty"`
	NextInvoice string `json:"next_invoice,omitempty"`
	RootCause   string `json:"root_cause,omitempty"`
	Release     string `json:"release,omitempty"` // open release request ("Rp 35 jt"), from the dealer's next proposal
}

// BriefPoint is one of the four points of "Ringkasan Orchestrator".
type BriefPoint struct {
	Kind         string        `json:"kind"` // on_schedule | drift | credit | push
	Tone         string        `json:"tone"`
	Dealers      []BriefDealer `json:"dealers"`
	Count        int           `json:"count"`
	Amount       int64         `json:"amount,omitempty"`
	Orders       int           `json:"orders,omitempty"`
	OrderDealers int           `json:"order_dealers,omitempty"`
	Item         *AgingItem    `json:"item,omitempty"`
	SignalIDs    []string      `json:"signal_ids"`
	Title        string        `json:"title"`
	Text         string        `json:"text"` // with [[dealer:<slug>|Name]] links
}

// Brief is the morning summary. Until the Orchestrator writes it (stage 06/10) it is a template from metrics.
type Brief struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Source      string       `json:"source"`
	Points      []BriefPoint `json:"points"`
	Counts      BriefCounts  `json:"counts"`
	Confidence  float64      `json:"confidence"`
	Cycle       *int64       `json:"cycle"` // number of the last full Orchestrator cycle (nil before the first)
}

// BriefCounts are the signals the brief was written from.
type BriefCounts struct {
	WA       int64 `json:"wa"`
	SO       int64 `json:"so"`
	Payments int64 `json:"payments"`
	Branches int   `json:"branches"`
}

func bd(it BoardItem) BriefDealer {
	m := it.Metrics
	d := BriefDealer{ID: it.ID, Name: it.Name, ShortName: it.ShortName, DueIn: m.DueIn, Last: m.Last, Rhythm: m.Rhythm, Status: m.Status, CreditState: m.Credit.State, RootCause: it.RootCause}
	if it.CreditLimit > 0 {
		d.ExposurePct = int(math.Round(100 * float64(m.Credit.Exposure) / float64(it.CreditLimit)))
		if m.Credit.Room != nil {
			d.RoomPct = int(math.Round(100 * *m.Credit.Room))
		}
	}
	return d
}

// TemplateBrief writes the four points from the board: jadwal order, lewat jadwal, sisa limit, push stok.
func (b *Board) TemplateBrief(stock []domain.StockItem, counts BriefCounts) Brief {
	br := Brief{GeneratedAt: b.Today, Source: "template", Counts: counts, Confidence: 0.92}

	due := b.Due(7)
	p1 := BriefPoint{Kind: "on_schedule", Tone: "good", Count: len(due), SignalIDs: []string{}}
	for _, it := range due {
		if *it.Metrics.DueIn <= 1 {
			p1.Dealers = append(p1.Dealers, bd(it))
		}
	}
	dealers := map[string]bool{}
	for _, it := range b.Items {
		for _, o := range b.Data.Histories[it.UUID].Orders {
			if o.ConfirmedAt != nil && metrics.DaysBetween(*o.ConfirmedAt, b.Today) <= 7 && o.State != "cancel" {
				p1.Amount += o.Total
				p1.Orders++
				dealers[it.ID] = true
			}
		}
	}
	p1.OrderDealers = len(dealers)

	drift := b.Drift()
	sort.SliceStable(drift, func(i, j int) bool { return drift[i].Metrics.Cyc < drift[j].Metrics.Cyc })
	p2 := BriefPoint{Kind: "drift", Tone: "warn", Count: len(drift), SignalIDs: []string{}}
	for _, it := range drift {
		p2.Dealers = append(p2.Dealers, bd(it))
		p2.Amount += it.Metrics.OmzetBln
	}

	p3 := BriefPoint{Kind: "credit", Tone: "bad", SignalIDs: []string{}}
	for _, it := range b.CreditTight() {
		h := b.Data.Histories[it.UUID]
		d := bd(it)
		bad := metrics.CreditTone(it.Metrics.Credit.State) == "bad"
		soon := it.Metrics.DueIn != nil && *it.Metrics.DueIn >= 0 && *it.Metrics.DueIn <= 7
		if !bad && !soon {
			continue
		}
		for _, inv := range OpenInvoices(h, b.Today) {
			if inv.LateDays > d.LateDays {
				d.LateDays, d.LateInvoice = inv.LateDays, inv.Number
			}
			if inv.LateDays == 0 && d.NextInvoice == "" && metrics.DaysBetween(b.Today, inv.DueAt) <= 7 {
				d.NextInvoice = inv.Number
			}
		}
		if it.Next != nil && it.Next.Kind == domain.KindCreditRelease && it.Next.Status == "proposed" {
			if m := reRelease.FindStringSubmatch(it.Next.Title); m != nil {
				d.Release = m[1]
			}
		}
		p3.Dealers = append(p3.Dealers, d)
		p3.Count++
	}

	p4 := BriefPoint{Kind: "push", Tone: "accent", SignalIDs: []string{}}
	for _, a := range b.StockAging(stock, "") {
		if a.AgeDays > b.Policies.Stock.AgingDays {
			item := a
			p4.Item = &item
			p4.Count = len(a.Candidates)
			break
		}
	}
	br.Points = []BriefPoint{p1, p2, p3, p4}
	for i := range br.Points {
		br.Points[i].Title, br.Points[i].Text = briefText(br.Points[i])
	}
	return br
}

var reRelease = regexp.MustCompile(`Rilis (Rp [\d,]+ (?:jt|M))`)

func link(d BriefDealer, short bool) string {
	n := d.Name
	if short {
		n = d.ShortName
	}
	return "[[dealer:" + d.ID + "|" + n + "]]"
}

func joinID(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " dan " + xs[len(xs)-1]
}

func rpt(v int64) string {
	if v >= 1_000_000_000 {
		return strings.Replace(fmt.Sprintf("Rp %.2f M", float64(v)/1e9), ".", ",", 1)
	}
	return fmt.Sprintf("Rp %d jt", int64(math.Round(float64(v)/1e6)))
}

var rootShort = map[string]string{domain.RootProjectUnpaid: "proyek belum cair", domain.RootMarketplaceModule: "harga modul vs marketplace", domain.RootMarketplace: "harga vs marketplace",
	domain.RootWholesaler: "beralih ke grosir lokal"}

// briefText writes a point the way the mockup reads ("Order tepat jadwal: …").
func briefText(p BriefPoint) (string, string) {
	switch p.Kind {
	case "on_schedule":
		var ds []string
		for _, d := range p.Dealers {
			ds = append(ds, link(d, true))
		}
		t := fmt.Sprintf("%d dealer jadwal order minggu ini", p.Count)
		if len(ds) > 0 {
			t += " — " + joinID(ds) + " besok"
		}
		t += fmt.Sprintf("; rekomendasi order disiapkan AI Follow-up. Order 7 hari terakhir %s dari %d dealer.", rpt(p.Amount), p.OrderDealers)
		return "Order tepat jadwal", t
	case "drift":
		var moving, churn, back []string
		for i, d := range p.Dealers {
			if d.Status == domain.StatusChurn {
				churn = append(churn, link(d, true))
				continue
			}
			if d.Last != nil && d.Rhythm != nil {
				unit := " / "
				if i == 0 {
					unit = " hari / siklus order "
				}
				moving = append(moving, fmt.Sprintf("%s (%d%s%d)", link(d, false), *d.Last, unit, *d.Rhythm))
			}
			if r, ok := rootShort[d.RootCause]; ok {
				back = append(back, fmt.Sprintf("%s (akar: %s)", d.ShortName, r))
			}
		}
		t := joinID(moving) + " mulai menjauh"
		if len(churn) > 0 {
			t += "; " + joinID(churn) + " sudah churn"
		}
		t += fmt.Sprintf(". Potensi %s/bulan.", rpt(p.Amount))
		if len(back) > 0 {
			t += " Yang bisa ditarik kembali: " + joinID(back) + "."
		}
		return "Lewat jadwal", t
	case "credit":
		var parts []string
		rank := func(d BriefDealer) int {
			switch {
			case d.Release != "":
				return 0
			case d.CreditState == domain.CreditOverLimit || d.CreditState == domain.CreditOverdue:
				return 1
			}
			return 2
		}
		ds := append([]BriefDealer{}, p.Dealers...)
		sort.SliceStable(ds, func(i, j int) bool { return rank(ds[i]) < rank(ds[j]) })
		for _, d := range ds {
			switch {
			case d.Release != "":
				s := fmt.Sprintf("%s minta rilis %s saat exposure sudah %d%% limit", link(d, true), d.Release, d.ExposurePct)
				if d.LateDays > 0 {
					s += fmt.Sprintf(" dan %s lewat %d hari", d.LateInvoice, d.LateDays)
				}
				parts = append(parts, s+" — usul DP 50%.")
			case d.CreditState == domain.CreditOverLimit || d.CreditState == domain.CreditOverdue:
				s := fmt.Sprintf("%s exposure %d%% limit", link(d, true), d.ExposurePct)
				if d.LateDays > 0 {
					s += fmt.Sprintf(" dan %s lewat %d hari", d.LateInvoice, d.LateDays)
				}
				parts = append(parts, s+" — order berikutnya tertahan sampai pembayaran masuk.")
			case d.DueIn != nil:
				inv := d.NextInvoice
				if inv == "" {
					inv = "invoice terbuka"
				}
				parts = append(parts, fmt.Sprintf("%s jadwal order %d hari lagi tapi sisa limit %d%% — tagih %s dulu agar ordernya tidak tertahan.", link(d, true), *d.DueIn, d.RoomPct, inv))
			}
		}
		if len(parts) == 0 {
			return "Over limit / overdue", "tidak ada dealer yang ordernya tertahan limit."
		}
		return "Over limit / overdue", strings.Join(parts, " ")
	case "push":
		if p.Item == nil {
			return "Push stok", "tidak ada stok di atas 90 hari."
		}
		it := p.Item
		return "Push stok", fmt.Sprintf("%s (%d pcs, %d hari, %s) cocok untuk %d dealer yang product mix-nya %s — %d di antaranya jadwal order minggu ini. Harga bundle tidak pernah di bawah floor margin.",
			it.Name, it.Qty, it.AgeDays, rpt(it.Value), p.Count, it.Category, it.DueThisWeek)
	}
	return "", ""
}
