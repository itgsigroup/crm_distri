package views

import (
	"math"
	"sort"
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
	return br
}
