package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/proposals"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// MCP path (06-mcp › orchestrator.input.get / submit): an external model becomes the analysis engine. The
// Orchestrator still computes candidates and numbers in Go; the client receives them with the dealer context,
// masked, and returns proposals that are validated (schema + provenance) before they count.

// MCPInput is what orchestrator.input.get returns.
type MCPInput struct {
	CycleID      string             `json:"cycle_id"`
	Agent        string             `json:"agent"`
	Kinds        []string           `json:"kinds"`
	Today        string             `json:"today"`
	Instructions string             `json:"instructions"`
	Policies     map[string]any     `json:"policies"`
	Dealers      []mcpDealer        `json:"dealers"`
	Stock        []domain.StockItem `json:"stock,omitempty"`
	Messages     []mcpMessage       `json:"messages,omitempty"`
	NewNumbers   []mcpNewNumber     `json:"new_numbers,omitempty"`
	Candidates   []domain.Proposal  `json:"candidates"`
}

type mcpDealer struct {
	ID       string              `json:"id"` // slug
	UUID     uuid.UUID           `json:"uuid"`
	Name     string              `json:"name"`
	Tier     string              `json:"tier"`
	Branch   string              `json:"branch"`
	Sales    string              `json:"sales"`
	Status   string              `json:"status"`
	Segment  string              `json:"segment"`
	Rhythm   *int                `json:"rhythm_days"`
	Last     *int                `json:"last_order_days"`
	DueIn    *int                `json:"due_in"`
	Credit   domain.Credit       `json:"credit"`
	Limit    int64               `json:"credit_limit"`
	SOW      int                 `json:"share_of_wallet"`
	OmzetBln int64               `json:"omzet_bln"`
	Memo     string              `json:"memo"`
	Contacts []string            `json:"contacts"`
	Invoices []views.OpenInvoice `json:"open_invoices"`
	Signals  []mcpSignal         `json:"signals"`
}

type mcpSignal struct {
	ID   uuid.UUID `json:"id"`
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

type mcpMessage struct {
	SignalID uuid.UUID `json:"signal_id"`
	Dealer   uuid.UUID `json:"dealer_uuid"`
	Contact  string    `json:"contact"`
	At       time.Time `json:"at"`
	Text     string    `json:"text"`
}

type mcpNewNumber struct {
	SignalID uuid.UUID `json:"signal_id"`
	Name     string    `json:"name"`
	Org      string    `json:"org"`
	Score    int       `json:"score"`
	Text     string    `json:"text"`
}

const mcpInstructions = "Kembalikan proposal untuk agen ini lewat orchestrator_submit. Setiap proposal wajib: kind dari 'kinds', " +
	"dealer_id = uuid dealer di Input (kecuali dealer baru), signal_ids hanya dari Input, confidence 0–1, why, dan preview (draft WA ≤ 3 kalimat) bila mengirim. " +
	"Angka (harga, limit, jumlah) ambil dari 'candidates' — jangan menghitung ulang. Placeholder <PIC_n>/<NO_n> biarkan apa adanya."

// BuildMCPInput prepares one agent's Input for an MCP client: Go candidates (templates, no LLM), dealer
// context, masked. It returns the masked JSON, the signal ids a submission may cite and the unmask mapping.
func BuildMCPInput(ctx context.Context, in *agents.Input, a agents.Agent, cycleID uuid.UUID) (json.RawMessage, []uuid.UUID, map[string]string, []domain.Proposal, error) {
	cands, err := a.Analyze(ctx, in, nil)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	x := MCPInput{CycleID: cycleID.String(), Agent: a.Name(), Kinds: a.Kinds(), Today: in.Today.Format("2006-01-02"), Instructions: mcpInstructions, Candidates: cands,
		Policies: map[string]any{"margin_floor_pct": in.Policies.Margin.Pct, "credit_room_min": in.Policies.Credit.RoomMin, "followup_gap_days": in.Policies.Followup.GapDays,
			"aging_days": in.Policies.Stock.AgingDays, "bundle_max_discount_pct": in.Policies.Stock.BundleMaxDiscountPct}}
	var names []string
	ids := map[uuid.UUID]bool{}
	for _, d := range in.Dealers {
		m := d.Metrics
		md := mcpDealer{ID: d.ID, UUID: d.UUID, Name: d.Name, Tier: d.Tier, Branch: d.Branch, Sales: d.Owner.Name, Status: m.Status, Segment: m.Segment,
			Rhythm: m.Rhythm, Last: m.Last, DueIn: m.DueIn, Credit: m.Credit, Limit: d.CreditLimit, SOW: m.SOW, OmzetBln: m.OmzetBln, Memo: d.Memo, Invoices: d.OpenInvoices}
		names = append(names, d.Owner.Name)
		for _, c := range d.Contacts {
			md.Contacts = append(md.Contacts, c.Name)
			names = append(names, c.Name)
		}
		for _, s := range d.Signals {
			md.Signals = append(md.Signals, mcpSignal{ID: s.ID, Kind: s.Kind, At: s.At, Text: s.Text})
			ids[s.ID] = true
		}
		x.Dealers = append(x.Dealers, md)
	}
	for _, m := range in.WA {
		x.Messages = append(x.Messages, mcpMessage{SignalID: m.SignalID, Dealer: m.DealerID, Contact: m.Contact, At: m.At, Text: m.Text})
		ids[m.SignalID] = true
		names = append(names, m.Contact)
	}
	if a.Name() == "AI Stok" {
		x.Stock = in.Stock
	}
	for _, id := range in.StockSignals {
		ids[id] = true
	}
	for _, n := range in.NewNumbers {
		x.NewNumbers = append(x.NewNumbers, mcpNewNumber{SignalID: n.SignalID, Name: n.Name, Org: n.Org, Score: n.Score, Text: n.Text})
		ids[n.SignalID] = true
		names = append(names, n.Sales)
	}
	for _, p := range cands {
		for _, id := range p.SignalIDs {
			ids[id] = true
		}
	}
	raw, err := json.Marshal(x)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var tree any
	_ = json.Unmarshal(raw, &tree)
	m := llm.NewMasker()
	masked, _ := json.Marshal(maskStrings(tree, m, names))
	out := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return masked, out, m.Mapping(), cands, nil
}

// maskStrings masks string values only (amounts stay numbers even when they have 10+ digits).
func maskStrings(v any, m *llm.Masker, names []string) any {
	switch x := v.(type) {
	case string:
		return m.Mask(x, names)
	case []any:
		for i := range x {
			x[i] = maskStrings(x[i], m, names)
		}
	case map[string]any:
		for k, e := range x {
			x[k] = maskStrings(e, m, names)
		}
	}
	return v
}

func unmaskStrings(v any, mapping map[string]string) any {
	switch x := v.(type) {
	case string:
		return llm.UnmaskWith(x, mapping)
	case []any:
		for i := range x {
			x[i] = unmaskStrings(x[i], mapping)
		}
	case map[string]any:
		for k, e := range x {
			x[k] = unmaskStrings(e, mapping)
		}
	}
	return v
}

// Submission errors.
var (
	ErrNoInput   = errors.New("no input for this cycle and agent: call orchestrator.input.get first")
	ErrBadSubmit = errors.New("submission rejected")
)

// ValidateSubmission unmasks and checks proposals from an MCP client: the agent's kinds, provenance only from
// the Input, a known dealer, confidence 0–1. MCP proposals never run on their own (autonomy approve).
func ValidateSubmission(a agents.Agent, raw json.RawMessage, allowed []uuid.UUID, mapping map[string]string, in *agents.Input, day string) ([]domain.Proposal, []string) {
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, []string{"proposals: " + err.Error()}
	}
	clean, _ := json.Marshal(unmaskStrings(tree, mapping)) // per string: JSON escapes "<" so text replacement would miss
	var ps []domain.Proposal
	if err := json.Unmarshal(clean, &ps); err != nil {
		return nil, []string{"proposals: " + err.Error()}
	}
	var ok []domain.Proposal
	var errs []string
	for i, p := range ps {
		p.Agent, p.Autonomy = a.Name(), "approve"
		bad := ""
		for _, id := range p.SignalIDs {
			if !slices.Contains(allowed, id) {
				bad = fmt.Sprintf("signal %s tidak ada di Input", id)
				break
			}
		}
		if bad == "" && p.DealerID != nil && in.Dealer(*p.DealerID) == nil {
			bad = "dealer_id tidak ada di Input"
		}
		if bad == "" {
			if err := p.Validate(a.Kinds()); err != nil {
				bad = err.Error()
			}
		}
		if bad != "" {
			errs = append(errs, fmt.Sprintf("#%d %q: %s", i+1, p.Title, bad))
			continue
		}
		if p.DedupeKey == "" {
			d := "-"
			if p.DealerID != nil {
				d = p.DealerID.String()
			}
			p.DedupeKey = fmt.Sprintf("mcp:%s:%s:%s:%s", strings.ReplaceAll(a.Name(), " ", ""), p.Kind, d, day)
		}
		if p.Payload == nil {
			p.Payload = map[string]any{}
		}
		p.Payload["source"] = "mcp"
		ok = append(ok, p)
	}
	return ok, errs
}

// agentByName finds one of the six agents.
func (o *Orchestrator) agentByName(name string) agents.Agent {
	for _, a := range o.agentList(domain.Scope{Kind: "all"}) {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

// InputFor returns (building and storing when needed) the Input of an agent for a cycle (orchestrator.input.get).
func (o *Orchestrator) InputFor(ctx context.Context, cycleID uuid.UUID, agent string) (json.RawMessage, error) {
	if row, err := o.St.Q.GetCycleInput(ctx, gen.GetCycleInputParams{CycleID: cycleID, Agent: agent}); err == nil {
		return row.Input, nil
	}
	a := o.agentByName(agent)
	if a == nil {
		return nil, fmt.Errorf("unknown agent %q", agent)
	}
	cyc, err := o.St.Q.GetCycle(ctx, cycleID)
	if err != nil {
		return nil, err
	}
	only := ""
	if sc, err := domain.ParseScope(cyc.Scope); err == nil && sc.Kind == "dealer" {
		only = sc.ID
	}
	cid := cycleID.String()
	in, _, err := proposals.BuildInput(ctx, o.St, o.Clock, only, &cid)
	if err != nil {
		return nil, err
	}
	raw, ids, mapping, _, err := BuildMCPInput(ctx, in, a, cycleID)
	if err != nil {
		return nil, err
	}
	mb, _ := json.Marshal(mapping)
	if err := o.St.Q.UpsertCycleInput(ctx, gen.UpsertCycleInputParams{CycleID: cycleID, Agent: agent, Input: raw, SignalIds: ids, Mapping: mb}); err != nil {
		return nil, err
	}
	return raw, nil
}

// SubmitResult is what orchestrator.submit answers.
type SubmitResult struct {
	Accepted int      `json:"accepted"`
	Rejected []string `json:"rejected"`
	Mode     string   `json:"mode"` // waiting (the running cycle takes them) | stored (added to a finished cycle)
}

// Submit validates proposals from an MCP client for (cycle, agent). While the cycle's Analisis waits for MCP
// (routing = mcp) the submission becomes its input; otherwise the proposals are stored as `proposed` in that cycle.
func (o *Orchestrator) Submit(ctx context.Context, cycleID uuid.UUID, agent string, raw json.RawMessage, client *uuid.UUID) (SubmitResult, error) {
	row, err := o.St.Q.GetCycleInput(ctx, gen.GetCycleInputParams{CycleID: cycleID, Agent: agent})
	if err != nil {
		return SubmitResult{}, ErrNoInput
	}
	a := o.agentByName(agent)
	if a == nil {
		return SubmitResult{}, fmt.Errorf("unknown agent %q", agent)
	}
	cyc, err := o.St.Q.GetCycle(ctx, cycleID)
	if err != nil {
		return SubmitResult{}, err
	}
	var mapping map[string]string
	_ = json.Unmarshal(row.Mapping, &mapping)
	only := ""
	if sc, err := domain.ParseScope(cyc.Scope); err == nil && sc.Kind == "dealer" {
		only = sc.ID
	}
	in, _, err := proposals.BuildInput(ctx, o.St, o.Clock, only, nil)
	if err != nil {
		return SubmitResult{}, err
	}
	ok, rejected := ValidateSubmission(a, raw, row.SignalIds, mapping, in, o.Clock.Now().Format("2006-01-02"))
	res := SubmitResult{Accepted: len(ok), Rejected: rejected}
	if len(ok) == 0 {
		return res, fmt.Errorf("%w: %s", ErrBadSubmit, strings.Join(rejected, "; "))
	}
	now := o.Clock.Now()
	if cyc.Status == "running" && row.SubmittedAt == nil {
		b, _ := json.Marshal(ok)
		res.Mode = "waiting"
		return res, o.St.Q.SubmitCycleInput(ctx, gen.SubmitCycleInputParams{CycleID: cycleID, Agent: agent, Submitted: b, SubmittedBy: client, SubmittedAt: &now})
	}
	res.Mode = "stored"
	cid := cycleID.String()
	stored := 0
	for _, p := range ok {
		if used, _ := o.St.Q.ProposalKeyUsed(ctx, &p.DedupeKey); used {
			continue
		}
		if id, err := proposals.Insert(ctx, o.St.Q, p, "proposed", &cid, now); err != nil {
			return res, err
		} else if id != uuid.Nil {
			stored++
		}
	}
	res.Accepted = stored
	_ = events.Notify(ctx, o.St.Pool, "proposal_changed", map[string]any{"cycle_id": cycleID, "source": "mcp", "created": stored})
	return res, nil
}

// analyzeViaMCP is Analisis when policy llm.routing.mode = mcp: the Input of each agent is published for MCP
// clients; the stage waits for their submissions (MCPWait, default 10 minutes) and falls back to the Go
// templates for agents nobody answered (stage partial).
func (o *Orchestrator) analyzeViaMCP(ctx context.Context, r *stageRun, list []agents.Agent) (map[string]any, error) {
	fallback := map[string][]domain.Proposal{}
	cid := r.cyc.ID
	var waiting []string
	for _, a := range list {
		raw, ids, mapping, cands, err := BuildMCPInput(ctx, r.in, a, cid)
		if err != nil {
			r.errs[a.Name()] = err.Error()
			continue
		}
		mb, _ := json.Marshal(mapping)
		if err := o.St.Q.UpsertCycleInput(ctx, gen.UpsertCycleInputParams{CycleID: cid, Agent: a.Name(), Input: raw, SignalIds: ids, Mapping: mb}); err != nil {
			return nil, err
		}
		fallback[a.Name()] = cands
		waiting = append(waiting, a.Name())
	}
	_ = events.Notify(ctx, o.St.Pool, "cycle_stage", map[string]any{"cycle_id": cid, "number": r.cyc.Number, "stage": "analyze", "status": "waiting",
		"detail": map[string]any{"waiting_for": waiting, "via": "mcp"}, "scope": r.scope.String()})
	wait := o.MCPWait
	if wait == 0 {
		wait = 10 * time.Minute
	}
	deadline := time.Now().Add(wait)
	submitted := map[string]json.RawMessage{}
	for time.Now().Before(deadline) && len(submitted) < len(waiting) {
		rows, err := o.St.Q.ListCycleInputs(ctx, cid)
		if err != nil {
			return nil, err
		}
		for _, x := range rows {
			if x.SubmittedAt != nil {
				submitted[x.Agent] = x.Submitted
			}
		}
		if len(submitted) == len(waiting) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	r.byAgent = map[string][]domain.Proposal{}
	fromMCP := 0
	for _, a := range list {
		name := a.Name()
		status := "done"
		if raw, ok := submitted[name]; ok {
			var ps []domain.Proposal
			_ = json.Unmarshal(raw, &ps) // validated and unmasked by Submit
			r.byAgent[name] = ps
			fromMCP++
		} else {
			r.byAgent[name] = fallback[name]
			status = "template"
		}
		_ = o.St.Q.InsertAgentRun(ctx, gen.InsertAgentRunParams{CycleID: &cid, Agent: name, Status: &status, ProposalsCount: i32(len(r.byAgent[name]))})
	}
	n := 0
	for _, a := range list {
		for _, p := range r.byAgent[a.Name()] {
			c := &Cand{P: p}
			if p.DealerID != nil {
				c.Dealer = r.in.Dealer(*p.DealerID)
			}
			r.cands = append(r.cands, c)
			n++
		}
	}
	partial := fromMCP < len(waiting)
	return map[string]any{"agents": len(list), "proposals": n, "via": "mcp", "mcp_agents": fromMCP, "partial": partial,
		"text": fmt.Sprintf("%d/%d agen lewat MCP", fromMCP, len(waiting))}, nil
}
