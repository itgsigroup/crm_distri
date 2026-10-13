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
)

// maxScreenItems caps the items one reading sends (largest value first).
const maxScreenItems = 20

// screenOf maps a reading agent to its page.
var screenOf = map[string]string{AgentStockRead: "stock", AgentCashRead: "credit"}

// IsScreenReading reports whether an agent name is one of the page readings.
func IsScreenReading(agent string) bool { _, ok := screenOf[agent]; return ok }

// ScreenReadings are the page readings a cycle of these agents includes: AI Stok's page, and AI Kredit's or AI
// Penagihan's.
func ScreenReadings(agentNames []string) []string {
	var out []string
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

var screenPrompt = map[string]string{AgentStockRead: "stock_read", AgentCashRead: "cash_read"}

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
	if o.Router == nil || !o.Router.HasModel() || (r.cyc.Via != nil && *r.cyc.Via == "mcp") || (r.in != nil && r.in.Policies.LLM.Mode == "mcp") {
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
	for _, agent := range ScreenReadings(names) {
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
