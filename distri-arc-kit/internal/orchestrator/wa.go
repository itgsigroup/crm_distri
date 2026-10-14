package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/waanalysis"
)

// Analisa WhatsApp (ADR 0026). A full cycle reads two sources: the BigQuery data (SO, payments, stock — the signals
// the agents work on) and the WhatsApp conversations of the last WindowDays. Ingest reads the conversations, Keputusan
// links them to the proposals the agents made and puts the conversations waiting too long for a reply into Rencana
// hari ini, Belajar asks the model for the overview and one line per conversation.

// readWA loads and analyses the conversations of the window. Every analysis reads each bubble of the conversations
// in its scope (one dealer's, or all); only a full cycle writes the conversation analysis, the totals and the plan.
func (o *Orchestrator) readWA(ctx context.Context, r *stageRun) error {
	now := o.Clock.Now()
	rows, err := o.St.Q.WAWindowMessages(ctx, gen.WAWindowMessagesParams{Since: now.AddDate(0, 0, -waanalysis.WindowDays), Until: now})
	if err != nil {
		return err
	}
	var dealer, thread *uuid.UUID
	if r.scope.Kind == "thread" {
		id, err := uuid.Parse(r.scope.ID)
		if err != nil {
			return err
		}
		// a customer group is read under one of its threads, whichever GSI number's thread was opened
		if c, err := o.St.Q.ConversationOf(ctx, id); err == nil {
			id = c
		}
		thread = &id
	}
	if r.scope.Kind == "dealer" {
		d, err := o.St.Q.GetDealer(ctx, &r.scope.ID)
		if err != nil {
			return err
		}
		dealer = &d.ID
	}
	var threads []waanalysis.Thread
	at := map[uuid.UUID]int{}
	for _, m := range rows {
		if (dealer != nil && (m.DealerID == nil || *m.DealerID != *dealer)) || (thread != nil && m.ThreadID != *thread) {
			continue
		}
		i, ok := at[m.ThreadID]
		if !ok {
			i = len(threads)
			at[m.ThreadID] = i
			threads = append(threads, waanalysis.Thread{ID: m.ThreadID, Kind: deref(m.ThreadKind), Title: deref(m.ThreadTitle), DealerID: m.DealerID,
				DealerSlug: deref(m.DealerSlug), DealerName: deref(m.DealerName), SalesID: m.SalesID, SalesName: deref(m.SalesName)})
		}
		// in a customer group a GSI employee's message is our side: it answers the customers like a reply would
		threads[i].Messages = append(threads[i].Messages, waanalysis.Message{ID: m.ID, SignalID: m.SignalID, In: deref(m.Direction) == "in" && !m.Internal,
			Text: deref(m.Body), At: m.SentAt})
	}
	if err := o.saveWAMessages(ctx, r, threads, now); err != nil {
		return err
	}
	if r.scope.Kind != "all" {
		return nil
	}
	r.waThreads = map[uuid.UUID]waanalysis.Thread{}
	for _, t := range threads {
		r.waThreads[t.ID] = t
	}
	r.wa = waanalysis.Analyze(threads, r.in.Products, now)
	r.waRead = true
	// link to the proposals already open and put the waiting chats into the plan now: a cycle that fails at
	// Analisis (no AI answer) still reads WhatsApp and still asks the sales to reply
	if err := o.linkWA(ctx, r); err != nil {
		return err
	}
	if err := o.saveWA(ctx, r); err != nil {
		return err
	}
	return o.writeChatSteps(ctx, r)
}

// saveWAMessages stores the analysis of every bubble of the conversations read (the latest analysis per message).
func (o *Orchestrator) saveWAMessages(ctx context.Context, r *stageRun, threads []waanalysis.Thread, now time.Time) error {
	type row struct {
		MessageID uuid.UUID         `json:"message_id"`
		ThreadID  uuid.UUID         `json:"thread_id"`
		SentAt    time.Time         `json:"sent_at"`
		Direction string            `json:"direction"`
		State     string            `json:"state"`
		Intent    string            `json:"intent"`
		Items     []waanalysis.Item `json:"items"`
		WaitMin   *int              `json:"wait_min"`
	}
	var rows []row
	for _, t := range threads {
		for _, m := range waanalysis.AnalyzeMessages(t, r.in.Products, now) {
			dir := "out"
			if m.In {
				dir = "in"
			}
			rows = append(rows, row{m.MessageID, m.ThreadID, m.SentAt, dir, m.State, m.Intent, m.Items, m.WaitMin})
		}
	}
	r.waMsgs = len(rows)
	if len(rows) == 0 {
		return nil
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	cyc := r.cyc.ID
	return o.St.Q.UpsertWAMessageInsights(ctx, gen.UpsertWAMessageInsightsParams{CycleID: &cyc, AnalyzedAt: now, Rows: b})
}

// linkWA adds the proposals made from each conversation's messages (this cycle's included).
func (o *Orchestrator) linkWA(ctx context.Context, r *stageRun) error {
	var ids []uuid.UUID
	for _, in := range r.wa {
		ids = append(ids, in.SignalIDs...)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := o.St.Q.ProposalsForSignals(ctx, ids)
	if err != nil {
		return err
	}
	bySignal := map[uuid.UUID][]gen.ProposalsForSignalsRow{}
	for _, p := range rows {
		if p.Status == "suppressed" {
			continue
		}
		for _, s := range p.SignalIds {
			bySignal[s] = append(bySignal[s], p)
		}
	}
	r.waOpen = map[uuid.UUID]bool{}
	for i := range r.wa {
		in := &r.wa[i]
		seen := map[uuid.UUID]bool{}
		in.ProposalIDs = in.ProposalIDs[:0]
		for _, s := range in.SignalIDs {
			for _, p := range bySignal[s] {
				if seen[p.ID] {
					continue
				}
				seen[p.ID] = true
				in.ProposalIDs = append(in.ProposalIDs, p.ID)
				if p.Status == "proposed" || p.Status == "approved" || p.Status == "edited" {
					r.waOpen[in.ThreadID] = true
				}
			}
		}
	}
	return nil
}

// saveWA replaces the cycle's Analisa WhatsApp rows (idempotent: one row per cycle and conversation).
func (o *Orchestrator) saveWA(ctx context.Context, r *stageRun) error {
	if !r.waRead {
		return nil
	}
	tot, _ := waanalysis.Summarize(r.wa)
	cyc := r.cyc.ID
	return o.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		if err := q.SaveWAAnalysis(ctx, gen.SaveWAAnalysisParams{CycleID: cyc, WindowDays: waanalysis.WindowDays, Threads: int32(tot.Threads), Inbound: int32(tot.Inbound),
			Outbound: int32(tot.Outbound), Waiting: int32(tot.Waiting), ReplyMedianMin: i32p(tot.ReplyMedianMin), Source: "rules", CreatedAt: o.Clock.Now()}); err != nil {
			return err
		}
		if err := q.DeleteWAInsights(ctx, cyc); err != nil {
			return err
		}
		for _, in := range r.wa {
			items, _ := json.Marshal(in.Items)
			if err := q.InsertWAInsight(ctx, gen.InsertWAInsightParams{CycleID: cyc, ThreadID: in.ThreadID, Kind: in.Kind, DealerID: in.DealerID, SalesID: in.SalesID,
				Title: in.Title, Intent: in.Intent, Items: items, LastText: strp(in.LastText), Inbound: int32(in.Inbound), Outbound: int32(in.Outbound),
				LastInAt: in.LastInAt, LastOutAt: in.LastOutAt, WaitingSince: in.WaitingSince, ReplyMedianMin: i32p(in.ReplyMedianMin), NeedsReply: in.NeedsReply,
				Priority: int32(in.Priority), SignalIds: in.SignalIDs, ProposalIds: in.ProposalIDs}); err != nil {
				return err
			}
		}
		return nil
	})
}

func i32p(v *int) *int32 {
	if v == nil {
		return nil
	}
	x := int32(*v)
	return &x
}

// waText is Ingest's WhatsApp half: "WhatsApp 12 chat · 3 menunggu balasan".
func waText(r *stageRun) string {
	tot, _ := waanalysis.Summarize(r.wa)
	s := fmt.Sprintf("WhatsApp %d chat", tot.Threads)
	if tot.NeedsReply > 0 {
		s += fmt.Sprintf(" · %d menunggu", tot.NeedsReply)
	}
	return s
}

// chatCands are the conversations Rencana hari ini asks a sales to answer: waiting longer than ReplyDue, with no open
// proposal of an agent already answering them (those wait in Keputusan).
func chatCands(r *stageRun) []ChatCand {
	var out []ChatCand
	for _, in := range r.wa {
		if !in.NeedsReply || r.waOpen[in.ThreadID] {
			continue
		}
		c := ChatCand{ThreadID: in.ThreadID, Title: in.Title, Intent: in.Intent, Items: in.Items, WaitMin: in.WaitMin, LastText: in.LastText}
		if in.DealerID != nil && r.in != nil {
			c.Dealer = r.in.Dealer(*in.DealerID)
		}
		out = append(out, c)
	}
	return out
}

// ChatCand is a conversation waiting for a reply, as the plan sees it.
type ChatCand struct {
	ThreadID uuid.UUID
	Title    string
	Dealer   *agents.Dealer
	Intent   string
	Items    []waanalysis.Item
	WaitMin  int
	LastText string
}

// chatStep is "Balas chat <dealer> — menunggu 5 jam: minta order 10 Kamera IP 4MP".
func chatStep(c ChatCand) domain.PlanItem {
	who := "nomor baru " + c.Title
	if c.Dealer != nil {
		who = planLink(c.Dealer)
	}
	what := waanalysis.IntentLabel(c.Intent)
	if items := waanalysis.ItemsText(c.Items); items != "" {
		what += " " + items
	} else if c.LastText != "" && (c.Intent == agents.IntentOther || c.Intent == waanalysis.IntentNone) {
		what = "“" + clipText(c.LastText, 80) + "”" // no request recognised: the customer's own words
	}
	text := fmt.Sprintf("Balas chat %s — menunggu %s: %s", who, waanalysis.WaitLabel(c.WaitMin), what)
	return domain.PlanItem{Agent: AgentWA, Autonomy: "approve", Text: text, Status: domain.PlanScheduled, Link: "chat:" + c.ThreadID.String()}
}

// AgentWA labels the steps and the lines Analisa WhatsApp contributes.
const AgentWA = "Analisa WhatsApp"

// ---------- AI: overview and one line per conversation ----------

// maxWASummaries caps the conversations sent to the model per cycle (most urgent first).
const maxWASummaries = 20

type waLine struct {
	ID         string  `json:"id"`
	Summary    string  `json:"summary"`
	Confidence float64 `json:"confidence"`
}

// waMsg / waThreadIn are one conversation as the reader (the model, or Claude through MCP) sees it.
type waMsg struct {
	ID   string `json:"id,omitempty"` // customer messages of the window: the bubble a note may be written for
	Dir  string `json:"dir"`
	From string `json:"from,omitempty"` // who wrote it, in a customer group
	Text string `json:"text"`
}

// waNote is the AI note on one customer bubble.
type waNote struct {
	ID         string  `json:"id"`
	Note       string  `json:"note"`
	Confidence float64 `json:"confidence"`
}

// addMsg adds one bubble to a conversation of the reading. In a customer group a GSI employee's message is our side
// (out) and a customer's names who wrote it — a name masked like the others before leaving the server.
func (rd *waReading) addMsg(t *waThreadIn, m gen.ChatMessage, window time.Time) {
	if m.Status == "failed" || strings.TrimSpace(deref(m.Body)) == "" {
		return
	}
	wm := waMsg{Dir: deref(m.Direction), Text: clipText(deref(m.Body), 300)}
	if m.Internal {
		wm.Dir = "out"
	}
	if t.Group && wm.Dir == "in" {
		if wm.From = strings.TrimSpace(deref(m.FromName)); wm.From != "" {
			rd.names = append(rd.names, wm.From)
		}
	}
	if wm.Dir == "in" && !m.SentAt.Before(window) {
		wm.ID = m.ID.String()
		rd.msgWant[wm.ID] = true
	}
	t.Msgs = append(t.Msgs, wm)
}

type waThreadIn struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Dealer  string            `json:"dealer,omitempty"`
	Sales   string            `json:"sales,omitempty"`
	Intent  string            `json:"intent"`
	Items   []waanalysis.Item `json:"items,omitempty"`
	Waiting string            `json:"waiting,omitempty"`
	Group   bool              `json:"group,omitempty"` // a customer group: several customers and GSI employees write
	Msgs    []waMsg           `json:"messages"`
}

// waReading is what is read for a cycle's Analisa WhatsApp: the totals and the most urgent conversations with
// customer messages (≤ maxWASummaries), their last 8 messages; names are the people to mask.
type waReading struct {
	Totals  waanalysis.Totals `json:"totals"`
	Threads []waThreadIn      `json:"threads"`
	names   []string
	want    map[string]bool
	msgWant map[string]bool
}

// waReadingFor builds the reading of a cycle from what Ingest stored (the same for the API model and for MCP).
func (o *Orchestrator) waReadingFor(ctx context.Context, cycleID uuid.UUID) (*waReading, error) {
	rows, err := o.St.Q.ListWAInsights(ctx, cycleID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return o.bubbleReadingFor(ctx, cycleID)
	}
	now := o.Clock.Now()
	var ins []waanalysis.Insight
	for _, x := range rows {
		ins = append(ins, waanalysis.Insight{Kind: x.Kind, SalesName: deref(x.SalesName), Intent: x.Intent, Inbound: int(x.Inbound), Outbound: int(x.Outbound),
			WaitingSince: x.WaitingSince, NeedsReply: x.NeedsReply, ReplyMedianMin: intp(x.ReplyMedianMin)})
	}
	tot, _ := waanalysis.Summarize(ins)
	rd := &waReading{Totals: tot, Threads: []waThreadIn{}, want: map[string]bool{}, msgWant: map[string]bool{}}
	window := now.AddDate(0, 0, -waanalysis.WindowDays)
	for _, x := range rows { // ListWAInsights: most urgent first
		if len(rd.Threads) >= maxWASummaries {
			break
		}
		if x.Inbound == 0 {
			continue
		}
		var items []waanalysis.Item
		_ = json.Unmarshal(x.Items, &items)
		t := waThreadIn{ID: x.ThreadID.String(), Title: x.Title, Dealer: deref(x.DealerName), Sales: deref(x.SalesName), Intent: waanalysis.IntentLabel(x.Intent), Items: items, Group: x.Kind == "group"}
		if x.WaitingSince != nil {
			t.Waiting = waanalysis.WaitLabel(int(now.Sub(*x.WaitingSince).Minutes()))
		}
		tid := x.ThreadID
		msgs, err := o.St.Q.ListThreadMessages(ctx, gen.ListThreadMessagesParams{ThreadID: &tid, SentAt: now.Add(time.Minute), Lim: 8})
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			rd.addMsg(&t, m, window)
		}
		// the person in the title ("Mbak Rina · Toko Sinar") and the sales are masked before leaving the server
		if p, _, ok := strings.Cut(x.Title, " · "); ok || x.Kind == "new" {
			rd.names = append(rd.names, strings.TrimSpace(p))
		}
		if n := deref(x.SalesName); n != "" {
			rd.names = append(rd.names, n)
		}
		rd.Threads = append(rd.Threads, t)
		rd.want[t.ID] = true
	}
	return rd, nil
}

// bubbleReadingFor is the reading of a cycle that did not analyse the whole of WhatsApp (one conversation, one
// dealer, the chat screen): the conversations whose bubbles it read, most waiting first, so Claude can write the note
// per customer message (and a line per conversation). No totals or overview are stored for such a cycle.
func (o *Orchestrator) bubbleReadingFor(ctx context.Context, cycleID uuid.UUID) (*waReading, error) {
	threads, err := o.St.Q.WACycleThreads(ctx, &cycleID)
	if err != nil {
		return nil, err
	}
	if len(threads) == 0 {
		return nil, ErrNoWA
	}
	now := o.Clock.Now()
	window := now.AddDate(0, 0, -waanalysis.WindowDays)
	rd := &waReading{Threads: []waThreadIn{}, want: map[string]bool{}, msgWant: map[string]bool{}}
	for _, x := range threads {
		if len(rd.Threads) >= maxWASummaries {
			break
		}
		t := waThreadIn{ID: x.ThreadID.String(), Title: deref(x.Title), Dealer: deref(x.DealerName), Sales: deref(x.SalesName), Intent: waanalysis.IntentLabel(x.Intent), Group: deref(x.Kind) == "group"}
		if since, ok := x.WaitingSince.(time.Time); ok {
			t.Waiting = waanalysis.WaitLabel(int(now.Sub(since).Minutes()))
		}
		tid := x.ThreadID
		msgs, err := o.St.Q.ListThreadMessages(ctx, gen.ListThreadMessagesParams{ThreadID: &tid, SentAt: now.Add(time.Minute), Lim: 12})
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			rd.addMsg(&t, m, window)
		}
		if p, _, ok := strings.Cut(deref(x.Title), " · "); ok || deref(x.Kind) == "new" {
			rd.names = append(rd.names, strings.TrimSpace(p))
		}
		if n := deref(x.SalesName); n != "" {
			rd.names = append(rd.names, n)
		}
		rd.Totals.Threads++
		rd.Threads = append(rd.Threads, t)
		rd.want[t.ID] = true
	}
	return rd, nil
}

func intp(v *int32) *int {
	if v == nil {
		return nil
	}
	x := int(*v)
	return &x
}

// ErrNoWA: the cycle has no Analisa WhatsApp (not a full cycle, or no conversation in the window).
var ErrNoWA = errors.New("siklus ini tidak punya Analisa WhatsApp")

// waAnswer is the reading's answer: an overview and one line per conversation.
type waAnswer struct {
	Overview string   `json:"overview"`
	Threads  []waLine `json:"threads"`
	Messages []waNote `json:"messages,omitempty"` // per bubble (optional)
}

func (rd *waReading) check(a waAnswer) error {
	if strings.TrimSpace(a.Overview) == "" || utf8.RuneCountInString(a.Overview) > 800 {
		return errors.New("overview kosong atau lebih dari 800 karakter")
	}
	for _, l := range a.Threads {
		if !rd.want[l.ID] {
			return fmt.Errorf("percakapan %q tidak ada di Input", l.ID)
		}
		if strings.TrimSpace(l.Summary) == "" || utf8.RuneCountInString(l.Summary) > 300 {
			return fmt.Errorf("summary %s kosong atau lebih dari 300 karakter", l.ID)
		}
		if l.Confidence < 0 || l.Confidence > 1 {
			return fmt.Errorf("confidence %s harus 0–1", l.ID)
		}
	}
	for _, n := range a.Messages {
		if !rd.msgWant[n.ID] {
			return fmt.Errorf("pesan %q tidak ada di Input (hanya pesan pelanggan yang punya id)", n.ID)
		}
		if strings.TrimSpace(n.Note) == "" || utf8.RuneCountInString(n.Note) > 200 {
			return fmt.Errorf("note pesan %s kosong atau lebih dari 200 karakter", n.ID)
		}
		if n.Confidence < 0 || n.Confidence > 1 {
			return fmt.Errorf("confidence pesan %s harus 0–1", n.ID)
		}
	}
	return nil
}

// storeWA saves an AI answer for a cycle (source ai).
func (o *Orchestrator) storeWA(ctx context.Context, cycleID uuid.UUID, a waAnswer) error {
	return o.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		ov := strings.TrimSpace(a.Overview)
		if err := q.SetWAOverview(ctx, gen.SetWAOverviewParams{CycleID: cycleID, Overview: &ov, Source: "ai"}); err != nil {
			return err
		}
		for _, l := range a.Threads {
			id, err := uuid.Parse(l.ID)
			if err != nil {
				continue
			}
			s := strings.TrimSpace(l.Summary)
			c := int16(max(0, min(100, l.Confidence*100+0.5)))
			if err := q.SetWAInsightSummary(ctx, gen.SetWAInsightSummaryParams{CycleID: cycleID, ThreadID: id, Summary: &s, Confidence: &c}); err != nil {
				return err
			}
		}
		for _, n := range a.Messages {
			id, err := uuid.Parse(n.ID)
			if err != nil {
				continue
			}
			note := strings.TrimSpace(n.Note)
			c := int16(max(0, min(100, n.Confidence*100+0.5)))
			if err := q.SetWAMessageNote(ctx, gen.SetWAMessageNoteParams{Note: &note, Confidence: &c, CycleID: &cycleID, MessageID: id}); err != nil {
				return err
			}
		}
		return nil
	})
}

// writeWASummary asks the server's model to read the conversations. Only a model's answer is stored (ADR 0025).
// Without a model the reading waits for Claude through MCP (orchestrator_input_get / orchestrator_submit with agent
// "Analisa WhatsApp").
func (o *Orchestrator) writeWASummary(ctx context.Context, r *stageRun) error {
	if o.Router == nil || !o.Router.HasModel() || (r.waRead && len(r.wa) == 0) || (!r.waRead && r.waMsgs == 0) {
		return nil
	}
	if (r.cyc.Via != nil && *r.cyc.Via == "mcp") || (r.in != nil && r.in.Policies.LLM.MCPOnly()) {
		return nil // read by Claude through MCP (orchestrator_submit "Analisa WhatsApp"), ADR 0032
	}
	rd, err := o.waReadingFor(ctx, r.cyc.ID)
	if err != nil || len(rd.Threads) == 0 {
		return nil
	}
	input, _ := json.Marshal(rd)
	fallback, _ := json.Marshal(waAnswer{Threads: []waLine{}})
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"overview", "threads"}, "properties": map[string]any{
		"overview": map[string]any{"type": "string"},
		"threads": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "summary", "confidence"},
			"properties": map[string]any{"id": map[string]any{"type": "string"}, "summary": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number"}}}},
		"messages": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "note", "confidence"},
			"properties": map[string]any{"id": map[string]any{"type": "string"}, "note": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number"}}}}}}
	check := func(raw json.RawMessage) error {
		var a waAnswer
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		return rd.check(a)
	}
	cid := r.cyc.ID.String()
	actx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res := o.Router.Complete(actx, llm.Request{Purpose: "wa.summary", Agent: AgentWA, CycleID: &cid, System: llm.Prompt("system") + "\n\n" + llm.Prompt("wa"),
		Input: input, Schema: schema, MaxTokens: 3000, Names: rd.names, Fallback: fallback}, check)
	if !llm.FromModel(res) || check(res.JSON) != nil {
		return nil // no AI line is better than a template one (ADR 0025)
	}
	var a waAnswer
	if err := json.Unmarshal(res.JSON, &a); err != nil {
		return nil
	}
	return o.storeWA(ctx, r.cyc.ID, a)
}

// ---------- MCP: Claude reads the conversations (no API key on the server) ----------

// WAInputFor is orchestrator_input_get for agent "Analisa WhatsApp": the cycle's reading, masked; the mapping to
// unmask the answer is kept in cycle_inputs (never sent to the client).
func (o *Orchestrator) WAInputFor(ctx context.Context, cycleID uuid.UUID) (json.RawMessage, error) {
	if row, err := o.St.Q.GetCycleInput(ctx, gen.GetCycleInputParams{CycleID: cycleID, Agent: AgentWA}); err == nil {
		return row.Input, nil
	}
	rd, err := o.waReadingFor(ctx, cycleID)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(map[string]any{"cycle_id": cycleID, "agent": AgentWA, "instructions": llm.Prompt("wa") + " " + waMCPSubmit,
		"totals": rd.Totals, "threads": rd.Threads})
	var tree any
	_ = json.Unmarshal(raw, &tree)
	m := llm.NewMasker()
	masked, _ := json.Marshal(maskStrings(tree, m, rd.names))
	mb, _ := json.Marshal(m.Mapping())
	if err := o.St.Q.UpsertCycleInput(ctx, gen.UpsertCycleInputParams{CycleID: cycleID, Agent: AgentWA, Input: masked, SignalIds: []uuid.UUID{}, Mapping: mb}); err != nil {
		return nil, err
	}
	return masked, nil
}

const waMCPSubmit = `Kirim lewat orchestrator_submit {"cycle_id": "<cycle_id>", "agent": "Analisa WhatsApp", "overview": "...", "threads": [{"id": "<id percakapan>", "summary": "...", "confidence": 0.8}], "messages": [{"id": "<id pesan pelanggan>", "note": "...", "confidence": 0.8}]}. Placeholder <PIC_n>/<NO_n> biarkan apa adanya.`

// WASubmit is orchestrator_submit for agent "Analisa WhatsApp": Claude's overview and lines, validated against the
// Input it was given, unmasked, stored as AI (any time after Ingest — also when the cycle itself failed).
func (o *Orchestrator) WASubmit(ctx context.Context, cycleID uuid.UUID, a waAnswer, client *uuid.UUID) (SubmitResult, error) {
	row, err := o.St.Q.GetCycleInput(ctx, gen.GetCycleInputParams{CycleID: cycleID, Agent: AgentWA})
	if err != nil {
		return SubmitResult{}, ErrNoInput
	}
	rd, err := o.waReadingFor(ctx, cycleID)
	if err != nil {
		return SubmitResult{}, err
	}
	var mapping map[string]string
	_ = json.Unmarshal(row.Mapping, &mapping)
	a.Overview = llm.UnmaskWith(a.Overview, mapping)
	for i := range a.Threads {
		a.Threads[i].Summary = llm.UnmaskWith(a.Threads[i].Summary, mapping)
	}
	for i := range a.Messages {
		a.Messages[i].Note = llm.UnmaskWith(a.Messages[i].Note, mapping)
	}
	if err := rd.check(a); err != nil {
		return SubmitResult{Rejected: []string{err.Error()}}, fmt.Errorf("%w: %s", ErrBadSubmit, err.Error())
	}
	if err := o.storeWA(ctx, cycleID, a); err != nil {
		return SubmitResult{}, err
	}
	now := o.Clock.Now()
	b, _ := json.Marshal(a)
	_ = o.St.Q.SubmitCycleInput(ctx, gen.SubmitCycleInputParams{CycleID: cycleID, Agent: AgentWA, Submitted: b, SubmittedBy: client, SubmittedAt: &now})
	_ = events.Notify(ctx, o.St.Pool, "cycle_done", map[string]any{"id": cycleID, "source": "mcp", "agent": AgentWA})
	return SubmitResult{Accepted: len(a.Threads) + len(a.Messages), Rejected: []string{}, Mode: "stored"}, nil
}

// ParseWAAnswer reads an MCP submission (overview + threads + optional notes per customer message).
func ParseWAAnswer(overview string, threads []map[string]any, messages ...map[string]any) waAnswer {
	a := waAnswer{Overview: overview, Threads: []waLine{}}
	for _, m := range messages {
		n := waNote{}
		n.ID, _ = m["id"].(string)
		n.Note, _ = m["note"].(string)
		n.Confidence, _ = m["confidence"].(float64)
		a.Messages = append(a.Messages, n)
	}
	for _, t := range threads {
		l := waLine{}
		l.ID, _ = t["id"].(string)
		l.Summary, _ = t["summary"].(string)
		l.Confidence, _ = t["confidence"].(float64)
		a.Threads = append(a.Threads, l)
	}
	return a
}

// ---------- Rencana: "Balas chat" from Ingest ----------

// writeChatSteps puts the conversations waiting for a reply into today's plan already in Ingest, so they reach
// Rencana hari ini even when the cycle fails later (no AI answer). Decide rewrites the whole plan with them.
func (o *Orchestrator) writeChatSteps(ctx context.Context, r *stageRun) error {
	now := o.Clock.Now()
	today := clock.Today(now)
	cands := chatCands(r)
	cyc := r.cyc.ID
	return o.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		if err := q.DeleteChatPlanItems(ctx, today); err != nil {
			return err
		}
		seq, err := q.MaxPlanSeq(ctx, today)
		if err != nil {
			return err
		}
		slot := firstSlot(now).Format("15.04")
		for i, c := range cands {
			if i >= waanalysis.MaxPlanSteps {
				break
			}
			it := chatStep(c)
			seq++
			agent, auto := it.Agent, it.Autonomy
			if err := q.InsertPlanItem(ctx, gen.InsertPlanItemParams{PlanDate: today, CycleID: &cyc, Seq: seq, TimeLabel: &slot, Agent: &agent,
				Autonomy: &auto, TextHtml: &it.Text, Status: it.Status, Link: strp(it.Link)}); err != nil {
				return err
			}
		}
		return nil
	})
}

func clipText(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
