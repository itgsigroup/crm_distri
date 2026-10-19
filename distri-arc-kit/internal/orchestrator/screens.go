package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// Analisa Stok and Analisa Kas (ADR 0030): the AI reading of the Push stok and Kredit · kas pages, the same way as
// Analisa WhatsApp. GSI Orbit computes every number (aging, push candidates, cash-in forecast); Claude — through MCP
// (orchestrator_input_get / orchestrator_submit), or the server's model — writes an overview and one note per item.
// Only what the AI wrote is stored (screen_analyses); without an AI answer the page says it has not been read.
const (
	AgentStockRead = "Analisa Stok"
	AgentCashRead  = "Analisa Kas"
	// AgentOrbitRead: Analisa Orbit (ADR 0052) — the dealers in orbit read with their WhatsApp chats
	AgentOrbitRead = "Analisa Orbit"
	// AgentSegmenRead: Analisa Segmen (ADR 0053) — the dealers moving between segments read with their WhatsApp chats
	AgentSegmenRead = "Analisa Segmen"
	// AgentRelasiRead: Analisa Relasi (ADR 0054) — how close each sales is to a dealer, read from what they write
	AgentRelasiRead = "Analisa Relasi"
)

// maxScreenItems caps the items one reading sends (largest value first).
const maxScreenItems = 20

// screenOf maps a reading agent to its page.
var screenOf = map[string]string{AgentStockRead: "stock", AgentCashRead: "credit", AgentOrbitRead: "orbit", AgentSegmenRead: "segmen", AgentRelasiRead: "relasi"}

// IsScreenReading reports whether an agent name is one of the page readings.
func IsScreenReading(agent string) bool { _, ok := screenOf[agent]; return ok }

// ScreenReadings are the page readings a cycle of these agents includes: AI Follow-up's pages (Orbit, Segmen, Peta
// relasi), AI Stok's page, and AI Kredit's or AI Penagihan's.
func ScreenReadings(agentNames []string) []string { return ScreenReadingsIn("all", agentNames) }

// ScreenReadingsIn narrows AI Follow-up's pages to the one asked for: Analisis ulang on Orbit reads Orbit only (fast),
// a full cycle reads them all (ADR 0054).
func ScreenReadingsIn(scope string, agentNames []string) []string {
	var out []string
	if slices.Contains(agentNames, "AI Follow-up") {
		switch scope {
		case "screen:orbit":
			out = append(out, AgentOrbitRead)
		case "screen:segmen":
			out = append(out, AgentSegmenRead)
		case "screen:relasi":
			out = append(out, AgentRelasiRead)
		default:
			out = append(out, AgentOrbitRead, AgentSegmenRead, AgentRelasiRead)
		}
	}
	if slices.Contains(agentNames, "AI Stok") {
		out = append(out, AgentStockRead)
	}
	if slices.Contains(agentNames, "AI Kredit") || slices.Contains(agentNames, "AI Penagihan") {
		out = append(out, AgentCashRead)
	}
	return out
}

// screenReading is what the AI reads: facts (computed totals) and items, each with a key its note answers.
type screenReading struct {
	Facts map[string]any   `json:"facts"`
	Items []map[string]any `json:"items"`
	names []string
}

// screenReadingFor builds a page's reading from the current data (the same for the server's model and for MCP).
func (o *Orchestrator) screenReadingFor(ctx context.Context, agent string) (*screenReading, error) {
	b, err := views.NewBuilder(o.St, o.Clock).Board(ctx)
	if err != nil {
		return nil, err
	}
	switch agent {
	case AgentStockRead:
		rows, err := o.St.Q.ListStockItems(ctx)
		if err != nil {
			return nil, err
		}
		return stockReading(b, views.StockItems(rows)), nil
	case AgentCashRead:
		return cashReading(b), nil
	case AgentOrbitRead, AgentSegmenRead, AgentRelasiRead:
		now := o.Clock.Now()
		rows, err := o.St.Q.WAWindowMessages(ctx, gen.WAWindowMessagesParams{Since: now.AddDate(0, 0, -orbitChatDays), Until: now})
		if err != nil {
			return nil, err
		}
		chats := map[uuid.UUID][]orbitMsg{}
		for _, m := range rows {
			if m.DealerID == nil || strings.TrimSpace(deref(m.Body)) == "" {
				continue
			}
			who := "pelanggan"
			if deref(m.Direction) == "out" || m.Internal {
				who = "sales"
			}
			chats[*m.DealerID] = append(chats[*m.DealerID], orbitMsg{At: m.SentAt, Who: who, Text: deref(m.Body), Sales: deref(m.SalesName)})
		}
		switch agent {
		case AgentSegmenRead:
			return segmenReading(b, chats, now), nil
		case AgentRelasiRead:
			return relasiReading(b, chats, now), nil
		}
		return orbitReading(b, chats, now), nil
	}
	return nil, fmt.Errorf("unknown reading %q", agent)
}

// stockReading: the aging stock with the most value (and its matching dealers), and the critical SKUs.
func stockReading(b *views.Board, stock []domain.StockItem) *screenReading {
	aging := b.StockAging(stock, "")
	slices.SortStableFunc(aging, func(x, y views.AgingItem) int { return int(y.Value - x.Value) })
	rd := &screenReading{Items: []map[string]any{}}
	var total int64
	old := 0
	for _, a := range aging {
		total += a.Value
		if a.AgeDays > b.Policies.Stock.AgingDays {
			old++
		}
	}
	for _, a := range aging[:min(len(aging), maxScreenItems)] {
		var cands []map[string]any
		for _, c := range a.Candidates {
			cands = append(cands, map[string]any{"dealer": c.Name, "reason": c.Reason, "order_due_in_days": c.DueIn, "drifting": c.Drifting, "omzet_bln": c.OmzetBln})
		}
		rd.Items = append(rd.Items, map[string]any{"key": a.ID.String(), "product": a.Name, "sku": a.SKU, "branch": a.Branch, "category": a.Category,
			"qty": a.Qty, "value": a.Value, "age_days": a.AgeDays, "weekly_velocity": a.WeeklyVelocity,
			"matching_dealers": a.CandidateCount, "due_this_week": a.DueThisWeek, "top_dealers": cands})
	}
	var crit []map[string]any
	critical := b.StockCritical(stock)
	for _, c := range critical[:min(len(critical), 10)] {
		crit = append(crit, map[string]any{"product": c.Name, "branch": c.Branch, "qty": c.Qty, "days_left": c.DaysLeft, "dependent_dealers": c.Dependents, "other_branches": c.OtherBranches})
	}
	rd.Facts = map[string]any{"aging_days_policy": b.Policies.Stock.AgingDays, "aging_items": len(aging), "aging_value": total, "older_than_policy": old,
		"bundle_max_discount_pct": b.Policies.Stock.BundleMaxDiscountPct, "critical": crit}
	return rd
}

// cashReading: the largest open receivables with their payment pattern and forecast, and the overview.
func cashReading(b *views.Board) *screenReading {
	ov, ar, _, fc, _ := b.Credit(30)
	slices.SortStableFunc(fc, func(x, y views.ForecastRow) int { return int(y.Open - x.Open) })
	bySlug := map[string]views.ARRow{}
	for _, r := range ar {
		bySlug[r.DealerID] = r
	}
	rd := &screenReading{Items: []map[string]any{}, Facts: map[string]any{"overview": ov}}
	for _, f := range fc[:min(len(fc), maxScreenItems)] {
		it, ok := b.Get(f.DealerID)
		if !ok || f.Open <= 0 {
			continue
		}
		a := bySlug[f.DealerID]
		item := map[string]any{"key": it.UUID.String(), "dealer": f.Name, "tier": it.Tier, "sales": it.Owner.Name, "open": f.Open, "invoices": f.Invoices,
			"overdue": a.Overdue, "late_days": a.LateDays, "pay_days": f.PayDays, "on_time_pct": a.OnTime, "late_count": f.LateCount,
			"asked_more_time": f.AskedTempo, "credit_state": a.Credit, "credit_limit": it.CreditLimit, "exposure": it.Metrics.Credit.Exposure,
			"probability_30d": f.Probability, "expected_30d": f.Expected}
		rd.Items = append(rd.Items, item)
		if it.Owner.Name != "" {
			rd.names = append(rd.names, it.Owner.Name)
		}
	}
	return rd
}

// orbitChatDays is how far back Analisa Orbit reads a dealer's WhatsApp.
const orbitChatDays = 30

// orbitMsg is one WhatsApp message of a dealer for Analisa Orbit.
type orbitMsg struct {
	At    time.Time
	Who   string // pelanggan | sales
	Text  string
	Sales string // the sales holding the number of the conversation
}

// orbitStatusRank: the dealers Analisa Orbit reads first — the ones slipping out of orbit, then the ones due soon.
var orbitStatusRank = map[string]int{"Churn": 0, "At risk": 1, "Aktif": 2, "Baru": 3, "Key account": 4}

// orbitReading: the dealers in orbit that need a word — at risk / churning, due within a week, or chatting — with
// their position (computed from the order cycle, glossary) and their last WhatsApp messages; the AI reads why they
// slip or come back from the chats (ADR 0052).
func orbitReading(b *views.Board, chats map[uuid.UUID][]orbitMsg, now time.Time) *screenReading {
	type cand struct {
		it   views.BoardItem
		rank int
	}
	var cs []cand
	count := map[string]int{}
	noChat := 0
	for _, it := range b.Items {
		m := it.Metrics
		if m.Status == "" || m.Status == "Prospek" {
			continue
		}
		count[m.Status]++
		r, ok := orbitStatusRank[m.Status]
		if !ok {
			continue
		}
		due := m.DueIn != nil && *m.DueIn <= 7
		talking := len(chats[it.UUID]) > 0
		if r > 1 && !due && !talking {
			continue
		}
		if r <= 1 && !talking {
			noChat++
		}
		if talking { // a dealer that writes is read before a silent one of the same rank
			r -= 10
		}
		cs = append(cs, cand{it, r})
	}
	slices.SortStableFunc(cs, func(x, y cand) int {
		if x.rank != y.rank {
			return x.rank - y.rank
		}
		return int(y.it.Metrics.OmzetBln - x.it.Metrics.OmzetBln)
	})
	rd := &screenReading{Items: []map[string]any{}}
	for _, c := range cs[:min(len(cs), maxScreenItems)] {
		it, m := c.it, c.it.Metrics
		msgs := chats[it.UUID]
		var chat []map[string]any
		for _, x := range msgs[max(0, len(msgs)-8):] { // the last 8 messages
			text := []rune(strings.TrimSpace(x.Text))
			if len(text) > 200 {
				text = append(text[:200], '…')
			}
			chat = append(chat, map[string]any{"at": x.At.Format("2006-01-02 15:04"), "dari": x.Who, "pesan": string(text)})
		}
		item := map[string]any{"key": it.UUID.String(), "dealer": it.Name, "tier": it.Tier, "sales": it.Owner.Name, "status": m.Status,
			"cyc": m.Cyc, "siklus_order_hari": m.Rhythm, "hari_sejak_order": m.Last, "jadwal_order_dalam_hari": m.DueIn, "aktivitas": m.Activity,
			"omzet_bln": m.OmzetBln, "sisa_limit": m.Credit.State, "chat_30_hari": len(msgs), "chat_terakhir": chat}
		if len(msgs) > 0 {
			item["hari_sejak_chat"] = int(now.Sub(msgs[len(msgs)-1].At).Hours() / 24)
		}
		rd.Items = append(rd.Items, item)
		if it.Owner.Name != "" {
			rd.names = append(rd.names, it.Owner.Name)
		}
	}
	rd.Facts = map[string]any{"dealer_per_status": count, "at_risk_churn_tanpa_chat_30_hari": noChat, "chat_dibaca_hari": orbitChatDays,
		"catatan": "posisi orbit dihitung dari siklus order (glossary); chat WhatsApp menjelaskan kenapa dealer bergeser"}
	return rd
}

// segmenReading: the dealers whose segment moved in three months, then the ones slowing down, then the ones writing
// on WhatsApp, then the largest — with their segment now and then (computed from orders, glossary) and their last
// WhatsApp messages; the AI reads from the chats why a dealer moved and how to serve it (ADR 0053).
func segmenReading(b *views.Board, chats map[uuid.UUID][]orbitMsg, now time.Time) *screenReading {
	type cand struct {
		it   views.BoardItem
		rank int
	}
	var cs []cand
	count, omzet := map[string]int{}, map[string]int64{}
	moved := 0
	for _, it := range b.Items {
		m := it.Metrics
		if m.Segment == "" || m.Status == "Prospek" {
			continue
		}
		count[m.Segment]++
		omzet[m.Segment] += m.OmzetBln
		rank := 4
		switch {
		case it.Prev != nil && it.Prev.Segment != "" && it.Prev.Segment != m.Segment:
			rank = 0
			moved++
		case it.Prev != nil && it.Prev.RhythmDays != nil && m.Rhythm != nil && *m.Rhythm > *it.Prev.RhythmDays:
			rank = 1 // slowing: orders further apart than three months ago
		case len(chats[it.UUID]) > 0:
			rank = 2
		}
		cs = append(cs, cand{it, rank})
	}
	slices.SortStableFunc(cs, func(x, y cand) int {
		if x.rank != y.rank {
			return x.rank - y.rank
		}
		return int(y.it.Metrics.OmzetBln - x.it.Metrics.OmzetBln)
	})
	rd := &screenReading{Items: []map[string]any{}}
	for _, c := range cs[:min(len(cs), maxScreenItems)] {
		it, m := c.it, c.it.Metrics
		msgs := chats[it.UUID]
		var chat []map[string]any
		for _, x := range msgs[max(0, len(msgs)-8):] {
			text := []rune(strings.TrimSpace(x.Text))
			if len(text) > 200 {
				text = append(text[:200], '…')
			}
			chat = append(chat, map[string]any{"at": x.At.Format("2006-01-02 15:04"), "dari": x.Who, "pesan": string(text)})
		}
		item := map[string]any{"key": it.UUID.String(), "dealer": it.Name, "tier": it.Tier, "sales": it.Owner.Name, "segmen": m.Segment,
			"order_per_bulan": m.Freq, "rata_rata_per_order": m.AvgOrder, "omzet_bln": m.OmzetBln, "porsi_belanja_pct": m.SOW, "product_mix": m.Mix,
			"sisa_limit": m.Credit.State, "chat_30_hari": len(msgs), "chat_terakhir": chat}
		if it.Prev != nil {
			item["segmen_3_bulan_lalu"] = it.Prev.Segment
			item["order_per_bulan_3_bulan_lalu"] = it.Prev.Freq
			item["rata_rata_per_order_3_bulan_lalu"] = it.Prev.AvgOrder
		}
		rd.Items = append(rd.Items, item)
		if it.Owner.Name != "" {
			rd.names = append(rd.names, it.Owner.Name)
		}
	}
	rd.Facts = map[string]any{"dealer_per_segmen": count, "omzet_bln_per_segmen": omzet, "pindah_segmen_3_bulan": moved,
		"ambang_order_per_bulan": b.Policies.Segment.FreqPerMonth, "ambang_rata_rata_per_order": b.Policies.Segment.SizeIDR,
		"segmen": "A = sering & besar, B = sering & kecil, C = jarang & besar, D = jarang & kecil (dihitung dari order, glossary)"}
	return rd
}

// relasiReading: how close the sales are to their dealers, read from what they write (MD: interaksi WA + order) —
// dealers chatted by two sales, dealers whose chats are thinning (last 15 days against the 15 before), big-order dealers
// that hardly chat, then the busiest — with who writes to them and their last messages (ADR 0054).
func relasiReading(b *views.Board, chats map[uuid.UUID][]orbitMsg, now time.Time) *screenReading {
	type cand struct {
		it   views.BoardItem
		rank int
	}
	half := now.AddDate(0, 0, -orbitChatDays/2)
	var cs []cand
	shared, thin, quietBig, talking := 0, 0, 0, 0
	for _, it := range b.Items {
		m := it.Metrics
		if m.Status == "" || m.Status == "Prospek" {
			continue
		}
		msgs := chats[it.UUID]
		bySales := map[string]int{}
		recent, before := 0, 0
		for _, x := range msgs {
			if x.Sales != "" {
				bySales[x.Sales]++
			}
			if x.At.After(half) {
				recent++
			} else {
				before++
			}
		}
		rank := 9
		switch {
		case len(bySales) >= 2:
			rank = 0
			shared++
		case before >= 4 && recent*2 < before:
			rank = 1
			thin++
		case m.AvgOrder >= 60_000_000 && len(msgs) < 3:
			rank = 2
			quietBig++
		case len(msgs) > 0:
			rank = 3
		}
		if len(msgs) > 0 {
			talking++
		}
		if rank == 9 {
			continue
		}
		cs = append(cs, cand{it, rank})
	}
	slices.SortStableFunc(cs, func(x, y cand) int {
		if x.rank != y.rank {
			return x.rank - y.rank
		}
		return len(chats[y.it.UUID]) - len(chats[x.it.UUID])
	})
	rd := &screenReading{Items: []map[string]any{}}
	for _, c := range cs[:min(len(cs), maxScreenItems)] {
		it, m := c.it, c.it.Metrics
		msgs := chats[it.UUID]
		bySales := map[string]int{}
		recent := 0
		for _, x := range msgs {
			if x.Sales != "" {
				bySales[x.Sales]++
			}
			if x.At.After(half) {
				recent++
			}
		}
		var chat []map[string]any
		for _, x := range msgs[max(0, len(msgs)-8):] {
			text := []rune(strings.TrimSpace(x.Text))
			if len(text) > 200 {
				text = append(text[:200], '…')
			}
			who := x.Who
			if who == "sales" && x.Sales != "" {
				who = "sales " + x.Sales
			}
			chat = append(chat, map[string]any{"at": x.At.Format("2006-01-02 15:04"), "dari": who, "pesan": string(text)})
		}
		rd.Items = append(rd.Items, map[string]any{"key": it.UUID.String(), "dealer": it.Name, "pemilik_sales": it.Owner.Name, "status": m.Status,
			"omzet_bln": m.OmzetBln, "rata_rata_per_order": m.AvgOrder, "pic_aktif": m.PICActive, "chat_30_hari": len(msgs),
			"chat_15_hari_terakhir": recent, "chat_15_hari_sebelumnya": len(msgs) - recent, "chat_per_sales": bySales, "chat_terakhir": chat})
		if it.Owner.Name != "" {
			rd.names = append(rd.names, it.Owner.Name)
		}
		for n := range bySales {
			rd.names = append(rd.names, n)
		}
	}
	rd.Facts = map[string]any{"dealer_yang_chat_30_hari": talking, "dealer_dipegang_dua_sales": shared, "chat_menipis": thin,
		"order_besar_chat_tipis": quietBig, "chat_dibaca_hari": orbitChatDays,
		"catatan": "garis di Peta relasi = interaksi WA + order (MD); analisa ini membaca isi chat WhatsApp"}
	return rd
}

// screenLine is the AI note on one item; screenAnswer the whole reading's answer.
type screenLine struct {
	Key        string  `json:"key"`
	Note       string  `json:"note"`
	Confidence float64 `json:"confidence"`
}

type screenAnswer struct {
	Overview string       `json:"overview"`
	Lines    []screenLine `json:"lines"`
}

func checkScreen(a screenAnswer, want map[string]bool) error {
	if strings.TrimSpace(a.Overview) == "" || utf8.RuneCountInString(a.Overview) > 800 {
		return errors.New("overview kosong atau lebih dari 800 karakter")
	}
	for _, l := range a.Lines {
		if !want[l.Key] {
			return fmt.Errorf("item %q tidak ada di Input", l.Key)
		}
		if strings.TrimSpace(l.Note) == "" || utf8.RuneCountInString(l.Note) > 300 {
			return fmt.Errorf("note %s kosong atau lebih dari 300 karakter", l.Key)
		}
		if l.Confidence < 0 || l.Confidence > 1 {
			return fmt.Errorf("confidence %s harus 0–1", l.Key)
		}
	}
	return nil
}

func (rd *screenReading) want() map[string]bool {
	w := map[string]bool{}
	for _, it := range rd.Items {
		if k, ok := it["key"].(string); ok {
			w[k] = true
		}
	}
	return w
}

// storeScreen saves an AI answer for a cycle's page reading.
func (o *Orchestrator) storeScreen(ctx context.Context, cycleID uuid.UUID, agent string, a screenAnswer) error {
	type line struct {
		Note       string `json:"note"`
		Confidence int    `json:"confidence"`
	}
	lines := map[string]line{}
	for _, l := range a.Lines {
		lines[l.Key] = line{Note: strings.TrimSpace(l.Note), Confidence: int(max(0, min(100, l.Confidence*100+0.5)))}
	}
	b, _ := json.Marshal(lines)
	return o.St.Q.SaveScreenAnalysis(ctx, gen.SaveScreenAnalysisParams{CycleID: cycleID, Screen: screenOf[agent], Overview: strings.TrimSpace(a.Overview),
		Lines: b, Source: "ai", CreatedAt: o.Clock.Now()})
}

var screenPrompt = map[string]string{AgentStockRead: "stock_read", AgentCashRead: "cash_read", AgentOrbitRead: "orbit_read", AgentSegmenRead: "segmen_read", AgentRelasiRead: "relasi_read"}

const screenMCPSubmit = `Kirim lewat orchestrator_submit {"cycle_id": "<cycle_id>", "agent": "%s", "overview": "...", "lines": [{"key": "<key item>", "note": "...", "confidence": 0.8}]}. Placeholder <PIC_n>/<NO_n> biarkan apa adanya.`

// ScreenInputFor is orchestrator_input_get for "Analisa Stok" / "Analisa Kas": the page's reading at the time it is
// first asked, masked and kept with the cycle (the answer is checked against it).
func (o *Orchestrator) ScreenInputFor(ctx context.Context, cycleID uuid.UUID, agent string) (json.RawMessage, error) {
	if row, err := o.St.Q.GetCycleInput(ctx, gen.GetCycleInputParams{CycleID: cycleID, Agent: agent}); err == nil {
		return row.Input, nil
	}
	if _, err := o.St.Q.GetCycle(ctx, cycleID); err != nil {
		return nil, err
	}
	rd, err := o.screenReadingFor(ctx, agent)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(map[string]any{"cycle_id": cycleID, "agent": agent, "instructions": llm.Prompt(screenPrompt[agent]) + " " + fmt.Sprintf(screenMCPSubmit, agent),
		"facts": rd.Facts, "items": rd.Items})
	var tree any
	_ = json.Unmarshal(raw, &tree)
	m := llm.NewMasker()
	masked, _ := json.Marshal(maskStrings(tree, m, rd.names))
	mb, _ := json.Marshal(m.Mapping())
	if err := o.St.Q.UpsertCycleInput(ctx, gen.UpsertCycleInputParams{CycleID: cycleID, Agent: agent, Input: masked, SignalIds: []uuid.UUID{}, Mapping: mb}); err != nil {
		return nil, err
	}
	return masked, nil
}

// ScreenSubmit is orchestrator_submit for a page reading: checked against the Input Claude was given, unmasked,
// stored as AI.
func (o *Orchestrator) ScreenSubmit(ctx context.Context, cycleID uuid.UUID, agent string, a screenAnswer, client *uuid.UUID) (SubmitResult, error) {
	row, err := o.St.Q.GetCycleInput(ctx, gen.GetCycleInputParams{CycleID: cycleID, Agent: agent})
	if err != nil {
		return SubmitResult{}, ErrNoInput
	}
	var given screenReading
	_ = json.Unmarshal(row.Input, &given)
	var mapping map[string]string
	_ = json.Unmarshal(row.Mapping, &mapping)
	a.Overview = llm.UnmaskWith(a.Overview, mapping)
	for i := range a.Lines {
		a.Lines[i].Note = llm.UnmaskWith(a.Lines[i].Note, mapping)
	}
	if err := checkScreen(a, given.want()); err != nil {
		return SubmitResult{Rejected: []string{err.Error()}}, fmt.Errorf("%w: %s", ErrBadSubmit, err.Error())
	}
	if err := o.storeScreen(ctx, cycleID, agent, a); err != nil {
		return SubmitResult{}, err
	}
	now := o.Clock.Now()
	b, _ := json.Marshal(a)
	_ = o.St.Q.SubmitCycleInput(ctx, gen.SubmitCycleInputParams{CycleID: cycleID, Agent: agent, Submitted: b, SubmittedBy: client, SubmittedAt: &now})
	_ = events.Notify(ctx, o.St.Pool, "cycle_done", map[string]any{"id": cycleID, "source": "mcp", "agent": agent})
	return SubmitResult{Accepted: len(a.Lines), Rejected: []string{}, Mode: "stored"}, nil
}

// ParseScreenAnswer reads an MCP submission (overview + lines).
func ParseScreenAnswer(overview string, lines []map[string]any) screenAnswer {
	a := screenAnswer{Overview: overview, Lines: []screenLine{}}
	for _, m := range lines {
		l := screenLine{}
		l.Key, _ = m["key"].(string)
		if l.Key == "" {
			l.Key, _ = m["id"].(string)
		}
		l.Note, _ = m["note"].(string)
		if l.Note == "" {
			l.Note, _ = m["summary"].(string)
		}
		l.Confidence, _ = m["confidence"].(float64)
		a.Lines = append(a.Lines, l)
	}
	return a
}

// writeScreenReadings asks the server's model to read the pages in the cycle's scope. Only a model's answer is
// stored (ADR 0025); a cycle sent to Claude through MCP is read there instead (orchestrator_submit).
func (o *Orchestrator) writeScreenReadings(ctx context.Context, r *stageRun) error {
	if o.Router == nil || !o.Router.HasModel() || r.system() || (r.cyc.Via != nil && *r.cyc.Via == "mcp") || (r.in != nil && r.in.Policies.LLM.MCPOnly()) {
		return nil
	}
	var names []string
	for _, a := range o.agentList(r.scope) {
		names = append(names, a.Name())
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"overview", "lines"}, "properties": map[string]any{
		"overview": map[string]any{"type": "string"},
		"lines": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"key", "note", "confidence"},
			"properties": map[string]any{"key": map[string]any{"type": "string"}, "note": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number"}}}}}}
	cid := r.cyc.ID.String()
	for _, agent := range ScreenReadingsIn(r.scope.String(), names) {
		rd, err := o.screenReadingFor(ctx, agent)
		if err != nil || len(rd.Items) == 0 {
			continue
		}
		want := rd.want()
		check := func(raw json.RawMessage) error {
			var a screenAnswer
			if err := json.Unmarshal(raw, &a); err != nil {
				return err
			}
			return checkScreen(a, want)
		}
		input, _ := json.Marshal(rd)
		fallback, _ := json.Marshal(screenAnswer{Lines: []screenLine{}})
		actx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		res := o.Router.Complete(actx, llm.Request{Purpose: "screen." + screenOf[agent], Agent: agent, CycleID: &cid, System: llm.Prompt("system") + "\n\n" + llm.Prompt(screenPrompt[agent]),
			Input: input, Schema: schema, MaxTokens: 3000, Names: rd.names, Fallback: fallback}, check)
		cancel()
		if !llm.FromModel(res) || check(res.JSON) != nil {
			continue // no AI line is better than a template one (ADR 0025)
		}
		var a screenAnswer
		if json.Unmarshal(res.JSON, &a) == nil {
			if err := o.storeScreen(ctx, r.cyc.ID, agent, a); err != nil {
				return err
			}
		}
	}
	return nil
}
