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
	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/waanalysis"
)

// Analisa WhatsApp (ADR 0026). A full cycle reads two sources: the BigQuery data (SO, payments, stock — the signals
// the agents work on) and the WhatsApp conversations of the last WindowDays. Ingest reads the conversations, Keputusan
// links them to the proposals the agents made and puts the conversations waiting too long for a reply into Rencana
// hari ini, Belajar asks the model for the overview and one line per conversation.

// readWA loads and analyses the conversations of the window (full cycles only).
func (o *Orchestrator) readWA(ctx context.Context, r *stageRun) error {
	if r.scope.Kind != "all" {
		return nil
	}
	now := o.Clock.Now()
	rows, err := o.St.Q.WAWindowMessages(ctx, gen.WAWindowMessagesParams{Since: now.AddDate(0, 0, -waanalysis.WindowDays), Until: now})
	if err != nil {
		return err
	}
	var threads []waanalysis.Thread
	at := map[uuid.UUID]int{}
	for _, m := range rows {
		if m.ThreadID == nil {
			continue
		}
		i, ok := at[*m.ThreadID]
		if !ok {
			i = len(threads)
			at[*m.ThreadID] = i
			threads = append(threads, waanalysis.Thread{ID: *m.ThreadID, Kind: deref(m.ThreadKind), Title: deref(m.ThreadTitle), DealerID: m.DealerID,
				DealerSlug: deref(m.DealerSlug), DealerName: deref(m.DealerName), SalesID: m.SalesID, SalesName: deref(m.SalesName)})
		}
		threads[i].Messages = append(threads[i].Messages, waanalysis.Message{ID: m.ID, SignalID: m.SignalID, In: deref(m.Direction) == "in", Text: deref(m.Body), At: m.SentAt})
	}
	r.waThreads = map[uuid.UUID]waanalysis.Thread{}
	for _, t := range threads {
		r.waThreads[t.ID] = t
	}
	r.wa = waanalysis.Analyze(threads, r.in.Products, now)
	r.waRead = true
	return o.saveWA(ctx, r)
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

// writeWASummary asks the model to read the conversations. Only a model's answer is stored (ADR 0025): without one
// the tab shows the numbers and says the conversations were not read by AI.
func (o *Orchestrator) writeWASummary(ctx context.Context, r *stageRun) error {
	if !r.waRead || len(r.wa) == 0 || o.Router == nil || !o.Router.HasModel() {
		return nil
	}
	tot, _ := waanalysis.Summarize(r.wa)
	type msg struct {
		Dir  string `json:"dir"`
		Text string `json:"text"`
	}
	type th struct {
		ID      string            `json:"id"`
		Title   string            `json:"title"`
		Dealer  string            `json:"dealer,omitempty"`
		Sales   string            `json:"sales,omitempty"`
		Intent  string            `json:"intent"`
		Items   []waanalysis.Item `json:"items,omitempty"`
		Waiting string            `json:"waiting,omitempty"`
		Msgs    []msg             `json:"messages"`
	}
	var threads []th
	var names []string
	want := map[string]bool{}
	for _, in := range r.wa {
		if len(threads) >= maxWASummaries {
			break
		}
		if in.Inbound == 0 {
			continue
		}
		t := th{ID: in.ThreadID.String(), Title: in.Title, Dealer: in.DealerName, Sales: in.SalesName, Intent: waanalysis.IntentLabel(in.Intent), Items: in.Items}
		if in.WaitingSince != nil {
			t.Waiting = waanalysis.WaitLabel(in.WaitMin)
		}
		ms := r.waThreads[in.ThreadID].Messages
		if len(ms) > 8 {
			ms = ms[len(ms)-8:]
		}
		for _, m := range ms {
			d := "out"
			if m.In {
				d = "in"
			}
			t.Msgs = append(t.Msgs, msg{d, clipText(m.Text, 300)})
		}
		// the person in the title ("Mbak Rina · Toko Sinar") and the sales are masked before leaving the server
		if p, _, ok := strings.Cut(in.Title, " · "); ok || in.Kind == "new" {
			names = append(names, strings.TrimSpace(p))
		}
		if in.SalesName != "" {
			names = append(names, in.SalesName)
		}
		threads = append(threads, t)
		want[t.ID] = true
	}
	if len(threads) == 0 {
		return nil
	}
	input, _ := json.Marshal(map[string]any{"totals": tot, "threads": threads})
	fallback, _ := json.Marshal(map[string]any{"overview": "", "threads": []waLine{}})
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"overview", "threads"}, "properties": map[string]any{
		"overview": map[string]any{"type": "string"},
		"threads": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "summary", "confidence"},
			"properties": map[string]any{"id": map[string]any{"type": "string"}, "summary": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number"}}}}}}
	check := func(raw json.RawMessage) error {
		var out struct {
			Overview string
			Threads  []waLine
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		if strings.TrimSpace(out.Overview) == "" || utf8.RuneCountInString(out.Overview) > 800 {
			return errors.New("overview empty or too long")
		}
		for _, l := range out.Threads {
			if !want[l.ID] {
				return fmt.Errorf("unknown conversation %q", l.ID)
			}
			if strings.TrimSpace(l.Summary) == "" || utf8.RuneCountInString(l.Summary) > 300 {
				return errors.New("summary empty or too long")
			}
		}
		return nil
	}
	cid := r.cyc.ID.String()
	actx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res := o.Router.Complete(actx, llm.Request{Purpose: "wa.summary", Agent: AgentWA, CycleID: &cid, System: llm.Prompt("system") + "\n\n" + llm.Prompt("wa"),
		Input: input, Schema: schema, MaxTokens: 3000, Names: names, Fallback: fallback}, check)
	if !llm.FromModel(res) || check(res.JSON) != nil {
		return nil // no AI line is better than a template one (ADR 0025)
	}
	var out struct {
		Overview string
		Threads  []waLine
	}
	if err := json.Unmarshal(res.JSON, &out); err != nil {
		return nil
	}
	return o.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		ov := strings.TrimSpace(out.Overview)
		if err := q.SetWAOverview(ctx, gen.SetWAOverviewParams{CycleID: r.cyc.ID, Overview: &ov, Source: "ai"}); err != nil {
			return err
		}
		for _, l := range out.Threads {
			id, err := uuid.Parse(l.ID)
			if err != nil {
				continue
			}
			s := strings.TrimSpace(l.Summary)
			c := int16(max(0, min(100, l.Confidence*100+0.5)))
			if err := q.SetWAInsightSummary(ctx, gen.SetWAInsightSummaryParams{CycleID: r.cyc.ID, ThreadID: id, Summary: &s, Confidence: &c}); err != nil {
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
