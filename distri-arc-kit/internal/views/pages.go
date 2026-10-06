package views

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/metrics"
	"distri-arc/internal/store/gen"
)

// ---------- Pusat kendali lists ----------

// Due returns dealers whose jadwal order falls within days (0 = today), nearest first.
func (b *Board) Due(days int) []BoardItem {
	var out []BoardItem
	for _, it := range b.Items {
		if d := it.Metrics.DueIn; it.Metrics.Rhythm != nil && d != nil && *d >= 0 && *d <= days {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return *out[i].Metrics.DueIn < *out[j].Metrics.DueIn })
	return out
}

// Drift returns dealers past drift × their cycle (lewat jadwal), biggest order first.
func (b *Board) Drift() []BoardItem {
	var out []BoardItem
	for _, it := range b.Items {
		if it.Metrics.Rhythm != nil && it.Metrics.Cyc > b.Policies.Orbit.Drift {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Metrics.AvgOrder > out[j].Metrics.AvgOrder })
	return out
}

// CreditTight returns credit dealers whose sisa limit is not aman, least room first.
func (b *Board) CreditTight() []BoardItem {
	var out []BoardItem
	for _, it := range b.Items {
		if it.CreditLimit > 0 && it.Metrics.Credit.State != domain.CreditAman {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreditLimit-out[i].Metrics.Credit.Exposure < out[j].CreditLimit-out[j].Metrics.Credit.Exposure
	})
	return out
}

// KPI is "KPI utama".
type KPI struct {
	OnSchedulePct int               `json:"on_schedule_pct"`
	DSODays       int               `json:"dso_days"`
	StockTurnDays int               `json:"stock_turn_days"`
	Targets       domain.KPITargets `json:"targets"`
	DriftCount    int               `json:"drift_count"`
	OverdueAmount int64             `json:"overdue_amount"`
	TightCount    int               `json:"tight_count"`
	StockValue    int64             `json:"stock_value"`
}

// KPI computes order tepat jadwal, DSO and perputaran stok, optionally for one branch.
func (b *Board) KPI(branch string, stock []domain.StockItem) KPI {
	var ms []domain.DealerMetrics
	var orders []domain.Order
	k := KPI{Targets: b.Policies.KPI}
	for _, it := range b.Items {
		if branch != "" && it.Branch != branch {
			continue
		}
		ms = append(ms, it.Metrics)
		orders = append(orders, b.Data.Histories[it.UUID].Orders...)
		if it.Metrics.Rhythm != nil && it.Metrics.Cyc > b.Policies.Orbit.Drift {
			k.DriftCount++
		}
		if it.CreditLimit > 0 && it.Metrics.Credit.State != domain.CreditAman {
			k.TightCount++
		}
		for _, inv := range b.Data.Histories[it.UUID].Invoices {
			if inv.Residual() > 0 && metrics.Day(inv.DueAt).Before(metrics.Day(b.Today)) {
				k.OverdueAmount += inv.Residual()
			}
		}
	}
	for _, s := range stock {
		if branch == "" || s.Branch == branch {
			k.StockValue += s.Value
		}
	}
	k.OnSchedulePct = metrics.OnSchedulePct(ms, b.Policies)
	k.DSODays = metrics.DSO(orders, b.Today)
	k.StockTurnDays = metrics.StockTurnDays(k.StockValue, metrics.COGSDaily(orders, b.Today))
	return k
}

// AgendaItem is one line of a sales agenda.
type AgendaItem struct {
	Kind      string `json:"kind"` // due | drift | collect
	DealerID  string `json:"dealer_id"`
	ShortName string `json:"short_name"`
	DueIn     *int   `json:"due_in,omitempty"`
	LateDays  int    `json:"late_days,omitempty"`
}

// AgendaRow is "Agenda sales" for one sales person.
type AgendaRow struct {
	Sales   Owner        `json:"sales"`
	Dealers int          `json:"dealers"`
	Count   int          `json:"count"`
	Items   []AgendaItem `json:"items"`
}

// Agenda splits the day per sales: 2 nearest jadwal order, 1 lewat jadwal, 1 "tagih dulu".
func (b *Board) Agenda(sales []gen.SalesUser) []AgendaRow {
	var out []AgendaRow
	for _, s := range sales {
		if s.Role != "sales" {
			continue
		}
		mine := b.Filter(s.Name)
		row := AgendaRow{Sales: Owner{Key: lower(s.Name), Name: s.Name, Initials: Initials(s.Name), Branch: s.Branch}, Dealers: len(mine)}
		var due, drift, tight []BoardItem
		for _, it := range mine {
			m := it.Metrics
			if m.Rhythm != nil && m.DueIn != nil && *m.DueIn >= 0 && *m.DueIn <= 7 {
				due = append(due, it)
			}
			isDrift := m.Rhythm != nil && m.Cyc > b.Policies.Orbit.Drift
			if isDrift {
				drift = append(drift, it)
			}
			if it.CreditLimit > 0 && metrics.CreditTone(m.Credit.State) == "bad" && !isDrift {
				tight = append(tight, it)
			}
		}
		sort.SliceStable(due, func(i, j int) bool { return *due[i].Metrics.DueIn < *due[j].Metrics.DueIn })
		row.Count = len(due) + len(drift) + len(tight)
		for i, it := range due {
			if i == 2 {
				break
			}
			row.Items = append(row.Items, AgendaItem{Kind: "due", DealerID: it.ID, ShortName: it.ShortName, DueIn: it.Metrics.DueIn})
		}
		if len(drift) > 0 {
			it := drift[0]
			row.Items = append(row.Items, AgendaItem{Kind: "drift", DealerID: it.ID, ShortName: it.ShortName, LateDays: *it.Metrics.Last - *it.Metrics.Rhythm})
		}
		if len(tight) > 0 {
			it := tight[0]
			row.Items = append(row.Items, AgendaItem{Kind: "collect", DealerID: it.ID, ShortName: it.ShortName})
		}
		out = append(out, row)
	}
	return out
}

// ---------- Orbit ----------

// StatusSummary is one row of "Isi orbit".
type StatusSummary struct {
	Status   string `json:"status"`
	Count    int    `json:"count"`
	OmzetBln int64  `json:"omzet_bln"`
}

// OrbitSummary groups dealers per ring (Baru is drawn on the Aktif ring).
func OrbitSummary(items []BoardItem) []StatusSummary {
	order := []string{domain.StatusKeyAccount, domain.StatusAktif, domain.StatusAtRisk, domain.StatusChurn}
	out := make([]StatusSummary, len(order))
	for i, s := range order {
		out[i].Status = s
	}
	for _, it := range items {
		st := RingOf(it.Metrics.Status)
		for i := range out {
			if out[i].Status == st {
				out[i].Count++
				out[i].OmzetBln += it.Metrics.OmzetBln
			}
		}
	}
	return out
}

// RingOf maps a status to its orbit ring.
func RingOf(status string) string {
	if status == domain.StatusBaru {
		return domain.StatusAktif
	}
	return status
}

// Mover is a structured "Yang bergerak" / "Berpindah segmen" insight; the web client writes the sentence.
type Mover struct {
	Kind       string `json:"kind"`
	DealerID   string `json:"dealer_id"`
	Name       string `json:"name"`
	Last       *int   `json:"last_order_days,omitempty"`
	Rhythm     *int   `json:"rhythm_days,omitempty"`
	DueIn      *int   `json:"due_in,omitempty"`
	Credit     string `json:"credit_state,omitempty"`
	Mix        int    `json:"mix,omitempty"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Up         bool   `json:"up,omitempty"`
	PrevRhythm *int   `json:"prev_rhythm_days,omitempty"`
	PrevAvg    int64  `json:"prev_avg_order,omitempty"`
	Avg        int64  `json:"avg_order,omitempty"`
	SOW        int    `json:"sow,omitempty"`
	Root       string `json:"root_cause,omitempty"`
}

// OrbitMovers lists dealers moving out, dealers close to their jadwal order and Key accounts with a thin mix.
func OrbitMovers(items []BoardItem) []Mover {
	var out []Mover
	for _, it := range items {
		if it.Metrics.Status == domain.StatusAtRisk {
			out = append(out, Mover{Kind: "moving_out", DealerID: it.ID, Name: it.Name, Last: it.Metrics.Last, Rhythm: it.Metrics.Rhythm, Credit: it.Metrics.Credit.State, Root: it.RootCause})
		}
	}
	for _, it := range items {
		if d := it.Metrics.DueIn; it.Metrics.Rhythm != nil && d != nil && *d >= 0 && *d <= 2 {
			out = append(out, Mover{Kind: "approaching", DealerID: it.ID, Name: it.Name, DueIn: d, Credit: it.Metrics.Credit.State})
		}
	}
	for _, it := range items {
		if it.Metrics.Status == domain.StatusKeyAccount && it.Metrics.Mix <= 4 {
			out = append(out, Mover{Kind: "thin_mix", DealerID: it.ID, Name: it.Name, Mix: it.Metrics.Mix})
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// ---------- Segmen ----------

// SegmentSummary is one row of "Isi segmen".
type SegmentSummary struct {
	Segment  string `json:"segment"`
	Count    int    `json:"count"`
	OmzetBln int64  `json:"omzet_bln"`
	Pct      int    `json:"pct"`
}

// SegmenSummary groups dealers per segment with their share of monthly omzet.
func SegmenSummary(items []BoardItem) ([]SegmentSummary, int64) {
	order := []string{domain.SegmentA, domain.SegmentB, domain.SegmentC, domain.SegmentD, domain.SegmentBaru}
	out := make([]SegmentSummary, len(order))
	var total int64
	for i, s := range order {
		out[i].Segment = s
	}
	for _, it := range items {
		total += it.Metrics.OmzetBln
		for i := range out {
			if out[i].Segment == it.Metrics.Segment {
				out[i].Count++
				out[i].OmzetBln += it.Metrics.OmzetBln
			}
		}
	}
	for i := range out {
		if total > 0 {
			out[i].Pct = int(math.Round(100 * float64(out[i].OmzetBln) / float64(total)))
		}
	}
	return out, total
}

var segRank = map[string]int{domain.SegmentD: 0, domain.SegmentBaru: 0, domain.SegmentB: 1, domain.SegmentC: 1, domain.SegmentA: 2}

// SegmenMovers lists segment changes against three months ago, then dealers that slow down or strengthen.
func SegmenMovers(items []BoardItem) []Mover {
	var out []Mover
	for _, it := range items {
		if it.Prev == nil || it.Prev.Segment == it.Metrics.Segment {
			continue
		}
		out = append(out, Mover{Kind: "moved", DealerID: it.ID, Name: it.Name, From: it.Prev.Segment, To: it.Metrics.Segment,
			Up: segRank[it.Metrics.Segment] > segRank[it.Prev.Segment], PrevRhythm: it.Prev.RhythmDays, Rhythm: it.Metrics.Rhythm,
			PrevAvg: it.Prev.AvgOrder, Avg: it.Metrics.AvgOrder, Root: it.RootCause})
	}
	for _, it := range items {
		p := it.Prev
		if p == nil || p.Segment != it.Metrics.Segment || p.RhythmDays == nil || it.Metrics.Rhythm == nil || *p.RhythmDays >= *it.Metrics.Rhythm {
			continue
		}
		out = append(out, Mover{Kind: "slowing", DealerID: it.ID, Name: it.Name, To: it.Metrics.Segment, PrevRhythm: p.RhythmDays, Rhythm: it.Metrics.Rhythm})
	}
	for _, it := range items {
		p := it.Prev
		if p == nil || p.Segment != it.Metrics.Segment || p.AvgOrder >= it.Metrics.AvgOrder || p.RhythmDays == nil || it.Metrics.Rhythm == nil || *p.RhythmDays < *it.Metrics.Rhythm {
			continue
		}
		out = append(out, Mover{Kind: "stronger", DealerID: it.ID, Name: it.Name, To: it.Metrics.Segment, PrevAvg: p.AvgOrder, Avg: it.Metrics.AvgOrder, SOW: it.Metrics.SOW})
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// ---------- Dealer page ----------

// ContactView is a PIC with its activity level.
type ContactView struct {
	Name            string     `json:"name"`
	Role            string     `json:"role"`
	Level           string     `json:"level"` // utama | aktif | jarang | belum
	Active          bool       `json:"active"`
	IsPrimary       bool       `json:"is_primary"`
	Interactions90d int        `json:"interactions_90d"`
	LastInteraction *time.Time `json:"last_interaction_at"`
}

// Contacts returns the PIC list: "kontak utama" for strong contacts (≥ 20 interactions in 90 days), "aktif"
// for any reply/order within 90 days, "jarang" when older, "belum kontak" when never.
func Contacts(cs []domain.Contact, today time.Time) []ContactView {
	out := make([]ContactView, 0, len(cs))
	for _, c := range cs {
		v := ContactView{Name: c.Name, Role: c.Role, IsPrimary: c.IsPrimary, Interactions90d: c.Interactions90d, LastInteraction: c.LastInteractionAt}
		switch {
		case c.LastInteractionAt == nil:
			v.Level = "belum"
		case metrics.DaysBetween(*c.LastInteractionAt, today) > 90:
			v.Level = "jarang"
		case c.Interactions90d >= 20:
			v.Level, v.Active = "utama", true
		default:
			v.Level, v.Active = "aktif", true
		}
		out = append(out, v)
	}
	return out
}

// OrderView is one SO with its order-to-cash phase.
type OrderView struct {
	Number      string             `json:"number"`
	ConfirmedAt *time.Time         `json:"confirmed_at"`
	Total       int64              `json:"total"`
	Phase       int                `json:"phase"`
	PhaseLabel  string             `json:"phase_label"`
	PaidAt      *time.Time         `json:"paid_at"`
	Lines       []domain.OrderLine `json:"lines"`
}

// Orders returns the 6-month orders (newest first), monthly totals and the last order's phase.
type Orders struct {
	Orders    []OrderView          `json:"orders"`
	Months    []metrics.MonthTotal `json:"months"`
	Last      *OrderView           `json:"last"`
	CycleDays int                  `json:"cycle_days"`
}

// DealerOrders builds the order-to-cash block.
func DealerOrders(h domain.DealerHistory, m domain.DealerMetrics, today time.Time, months int) Orders {
	out := Orders{Months: metrics.MonthlyTotals(h.Orders, today, months), CycleDays: m.CycleDays}
	os := append([]domain.Order(nil), h.Orders...)
	sort.SliceStable(os, func(i, j int) bool { return t(os[i]).After(t(os[j])) })
	cut := clock.Today(today).AddDate(0, -months, 0)
	for _, o := range os {
		if o.State == "cancel" || t(o).Before(cut) {
			continue
		}
		ph := metrics.Phase(o)
		out.Orders = append(out.Orders, OrderView{Number: o.Number, ConfirmedAt: o.ConfirmedAt, Total: o.Total, Phase: ph, PhaseLabel: domain.Phases[ph], PaidAt: o.PaidAt, Lines: o.Lines})
	}
	if len(out.Orders) > 0 {
		l := out.Orders[0]
		out.Last = &l
	}
	return out
}

func t(o domain.Order) time.Time {
	if o.ConfirmedAt != nil {
		return *o.ConfirmedAt
	}
	if o.OrderedAt != nil {
		return *o.OrderedAt
	}
	return time.Time{}
}

// OpenInvoice is an unpaid invoice with how late it is.
type OpenInvoice struct {
	Number   string    `json:"number"`
	IssuedAt time.Time `json:"issued_at"`
	DueAt    time.Time `json:"due_at"`
	Total    int64     `json:"total"`
	Residual int64     `json:"residual"`
	LateDays int       `json:"late_days"`
}

// OpenInvoices lists unpaid invoices, oldest first.
func OpenInvoices(h domain.DealerHistory, today time.Time) []OpenInvoice {
	var out []OpenInvoice
	for _, i := range h.Invoices {
		if i.Residual() <= 0 || i.State == "cancel" {
			continue
		}
		late := metrics.DaysBetween(i.DueAt, today)
		if late < 0 {
			late = 0
		}
		out = append(out, OpenInvoice{Number: i.Number, IssuedAt: i.IssuedAt, DueAt: i.DueAt, Total: i.Total, Residual: i.Residual(), LateDays: late})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].IssuedAt.Before(out[b].IssuedAt) })
	return out
}

// Commitment is a two-way commitment row.
type Commitment struct {
	Title    string     `json:"title"`
	Detail   string     `json:"detail"`
	Status   string     `json:"status"`
	DueAt    *time.Time `json:"due_at"`
	LateDays int        `json:"late_days"`
	Invoice  string     `json:"invoice,omitempty"`
}

// Commitments groups commitments into Kami / Mereka; late ones carry their days past due.
func Commitments(rows []gen.ListDealerCommitmentsRow, today time.Time) map[string][]Commitment {
	out := map[string][]Commitment{"kami": {}, "mereka": {}}
	for _, r := range rows {
		c := Commitment{Title: r.Title, Detail: deref(r.Detail), Status: r.Status, DueAt: r.DueAt, Invoice: deref(r.InvoiceNumber)}
		if r.DueAt != nil && r.Status != "done" {
			if d := metrics.DaysBetween(*r.DueAt, today); d > 0 {
				c.LateDays = d
				c.Status = "late"
			}
		}
		out[r.Side] = append(out[r.Side], c)
	}
	return out
}

// TimelineEntry is one interaction with its agent conclusion.
type TimelineEntry struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Via        string    `json:"via"`
	Who        string    `json:"who"`
	Text       string    `json:"text"`
	Conclusion string    `json:"conclusion"`
	SignalID   string    `json:"signal_id"`
}

// Timeline converts signals with a conclusion.
func Timeline(rows []gen.Signal) []TimelineEntry {
	out := make([]TimelineEntry, 0, len(rows))
	for _, s := range rows {
		var p struct {
			Via, Who, Text, Conclusion string
		}
		_ = json.Unmarshal(s.Payload, &p)
		if p.Text == "" {
			p.Text = deref(s.Summary)
		}
		out = append(out, TimelineEntry{At: s.OccurredAt, Kind: s.Kind, Via: p.Via, Who: p.Who, Text: p.Text, Conclusion: p.Conclusion, SignalID: s.ID.String()})
	}
	return out
}

// Detail is the dealer page (everything Stage 02's Dealer screen shows).
type Detail struct {
	BoardItem
	Memo          string                  `json:"memo"`
	MemoUpdatedAt *time.Time              `json:"memo_updated_at"`
	MemoSignals   []TimelineEntry         `json:"memo_signals"`
	MemoSentences []MemoSentence          `json:"memo_sentences"` // each sentence with its sources (hover)
	Contacts      []ContactView           `json:"contacts"`
	Commitments   map[string][]Commitment `json:"commitments"`
	Orders        Orders                  `json:"orders"`
	OpenInvoices  []OpenInvoice           `json:"open_invoices"`
	Timeline      []TimelineEntry         `json:"timeline"`
	Flags         []string                `json:"flags"`
}

// MemoSentence is one claim of the memo and the signals it rests on.
type MemoSentence struct {
	Text      string   `json:"text"`
	SignalIDs []string `json:"signal_ids"`
}

// DealerDetail assembles the dealer page.
func (bld *Builder) DealerDetail(ctx context.Context, b *Board, it BoardItem) (Detail, error) {
	id := it.ID
	row, err := bld.st.Q.GetDealer(ctx, &id)
	if err != nil {
		return Detail{}, err
	}
	h := b.Data.Histories[it.UUID]
	d := Detail{BoardItem: it, Memo: deref(row.Memo), MemoUpdatedAt: row.MemoUpdatedAt, Contacts: Contacts(h.Contacts, b.Today),
		Orders: DealerOrders(h, it.Metrics, b.Today, 6), OpenInvoices: OpenInvoices(h, b.Today)}
	if len(row.MemoSignalIds) > 0 {
		sigs, err := bld.st.Q.ListSignalsByIDs(ctx, row.MemoSignalIds)
		if err != nil {
			return Detail{}, err
		}
		d.MemoSignals = Timeline(sigs)
	}
	_ = json.Unmarshal(row.MemoSentences, &d.MemoSentences)
	cm, err := bld.st.Q.ListDealerCommitments(ctx, &it.UUID)
	if err != nil {
		return Detail{}, err
	}
	d.Commitments = Commitments(cm, b.Today)
	tl, err := bld.st.Q.ListDealerTimeline(ctx, gen.ListDealerTimelineParams{DealerID: &it.UUID, Limit: 10})
	if err != nil {
		return Detail{}, err
	}
	d.Timeline = Timeline(tl)
	m := it.Metrics
	if it.CreditLimit > 0 && metrics.CreditTone(m.Credit.State) == "bad" {
		d.Flags = append(d.Flags, "credit_blocked")
	}
	if it.CreditLimit > 0 && m.Credit.OnTime >= b.Policies.Credit.LimitUp.OnTimeMin && m.Credit.Room != nil && *m.Credit.Room < b.Policies.Credit.RoomMin {
		d.Flags = append(d.Flags, "limit_up_candidate")
	}
	if m.PICActive <= 1 {
		d.Flags = append(d.Flags, "single_pic")
	}
	if d.Flags == nil {
		d.Flags = []string{}
	}
	return d, nil
}

// StockItems converts stored stock.
func StockItems(rows []gen.StockItem) []domain.StockItem {
	out := make([]domain.StockItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.StockItem{ID: r.ID, Branch: r.Branch, SKU: r.Sku, Name: r.Name, Category: r.Category, Qty: int(r.Qty), UnitCost: r.UnitCost, Value: r.Value, AgeDays: int(r.AgeDays), WeeklyVelocity: r.WeeklyVelocity})
	}
	return out
}

// DealerViews adapts board items for the stock rules.
func DealerViews(items []BoardItem) []metrics.DealerView {
	out := make([]metrics.DealerView, 0, len(items))
	for _, it := range items {
		out = append(out, metrics.DealerView{ID: it.ID, Name: it.Name, Metrics: it.Metrics, Composition: it.Composition})
	}
	return out
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
