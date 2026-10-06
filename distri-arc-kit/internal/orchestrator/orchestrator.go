// Package orchestrator is the only way into analysis (04-orchestrator.md). A cycle runs six stages — Ingest,
// Analisis, Sintesis & konflik, Keputusan, Eksekusi, Belajar — each recorded in cycle_stages and announced over
// NOTIFY (SSE cycle_stage / cycle_done). One cycle runs at a time: a second request gets ErrRunning (409).
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/proposals"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// ErrRunning is returned while another cycle is queued or running.
var ErrRunning = errors.New("cycle_running")

// lockKey is the advisory lock held for the whole cycle; the second key is the schema so isolated test schemas
// (and a second tenant database schema) do not block each other.
const lockKey = 42

// Orchestrator directs the six agents.
type Orchestrator struct {
	St        *store.Store
	Clock     clock.Clock
	Router    *llm.Router
	Log       *slog.Logger
	Agents    []agents.Agent // default proposals.All()
	OdooWrite bool
	Jobs      *river.Client[pgx.Tx] // inserts outbox.send for auto sends (nil in tests)
	MCPWait   time.Duration         // routing = mcp: how long Analisis waits for submissions (default 10 min)
}

// Report is what a finished cycle returns.
type Report struct {
	Cycle     gen.Cycle         `json:"cycle"`
	Stored    int               `json:"stored"`
	Conflicts []domain.Conflict `json:"conflicts"`
	Plan      []domain.PlanItem `json:"plan"`
	Errors    map[string]string `json:"errors"`
}

func (o *Orchestrator) log() *slog.Logger {
	if o.Log == nil {
		return slog.Default()
	}
	return o.Log
}

// Queue records a cycle request; it fails with ErrRunning when a cycle is already queued or running.
func (o *Orchestrator) Queue(ctx context.Context, q *gen.Queries, sc domain.Scope, tr domain.Trigger) (gen.Cycle, error) {
	if q == nil {
		q = o.St.Q
	}
	via := tr.Via
	if via == "" {
		via = "api"
	}
	source := tr.Source
	if source == "" {
		source = "manual"
	}
	c, err := q.QueueCycle(ctx, gen.QueueCycleParams{Trigger: source, Scope: sc.String(), Via: &via, RequestedBy: strp(tr.By), StartedAt: o.Clock.Now()})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return c, ErrRunning
	}
	return c, err
}

// Run queues and executes a cycle in one call (CLI, scheduler, tests).
func (o *Orchestrator) Run(ctx context.Context, sc domain.Scope, tr domain.Trigger) (*Report, error) {
	c, err := o.Queue(ctx, nil, sc, tr)
	if err != nil {
		return nil, err
	}
	return o.Execute(ctx, c.ID)
}

// stageRun is the shared state of one cycle.
type stageRun struct {
	cyc       gen.Cycle
	scope     domain.Scope
	in        *agents.Input
	signals   []uuid.UUID
	recent    int
	byAgent   map[string][]domain.Proposal
	errs      map[string]string
	cands     []*Cand
	conflicts []domain.Conflict
	keyIDs    map[string]uuid.UUID
	stored    int
	plan      []domain.PlanItem
	auto      int
	decisions int
	outbox    int
	learned   int
	partial   bool
}

// Execute runs a queued cycle through the six stages.
func (o *Orchestrator) Execute(ctx context.Context, id uuid.UUID) (*Report, error) {
	conn, err := o.St.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, "select pg_try_advisory_lock($1, hashtext(current_schema()))", lockKey).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrRunning
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "select pg_advisory_unlock($1, hashtext(current_schema()))", lockKey)
	}()

	cyc, err := o.St.Q.GetCycle(ctx, id)
	if err != nil {
		return nil, err
	}
	sc, err := domain.ParseScope(cyc.Scope)
	if err != nil {
		return nil, o.fail(ctx, cyc, err)
	}
	start := o.Clock.Now()
	if err := o.St.Q.StartCycle(ctx, gen.StartCycleParams{ID: id, StartedAt: start}); err != nil {
		return nil, err
	}
	wall := time.Now()
	r := &stageRun{cyc: cyc, scope: sc, keyIDs: map[string]uuid.UUID{}, errs: map[string]string{}}
	steps := []struct {
		name string
		fn   func(context.Context, *stageRun) (map[string]any, error)
	}{
		{"ingest", o.ingest}, {"analyze", o.analyze}, {"synthesize", o.synthesize},
		{"decide", o.decide}, {"execute", o.execute}, {"learn", o.learn},
	}
	for _, s := range steps {
		if err := o.stage(ctx, r, s.name, s.fn); err != nil {
			return nil, o.fail(ctx, cyc, fmt.Errorf("%s: %w", s.name, err))
		}
	}
	status := "done"
	if r.partial {
		status = "partial"
	}
	note := o.note(ctx, r)
	dur := int32(time.Since(wall).Milliseconds())
	fin := o.Clock.Now()
	if err := o.St.Q.FinishCycle(ctx, gen.FinishCycleParams{ID: id, Status: status, FinishedAt: &fin, DurationMs: &dur,
		SignalsCount: i32(r.recent), AutoCount: i32(r.auto), DecisionCount: i32(r.decisions), ConflictCount: i32(visible(r.conflicts)), Note: &note}); err != nil {
		return nil, err
	}
	cyc, _ = o.St.Q.GetCycle(ctx, id)
	_ = events.Notify(ctx, o.St.Pool, "cycle_done", map[string]any{"id": id, "number": cyc.Number, "scope": sc.String(), "label": sc.Label(),
		"via": deref(cyc.Via), "status": status, "note": note, "updated": refreshed(r.cands), "stored": r.stored, "duration_ms": dur})
	_ = events.Notify(ctx, o.St.Pool, "proposal_changed", map[string]any{"cycle_id": id})
	o.log().Info("cycle done", "number", deref(cyc.Number), "scope", sc.String(), "stored", r.stored, "conflicts", visible(r.conflicts), "ms", dur)
	return &Report{Cycle: cyc, Stored: r.stored, Conflicts: r.conflicts, Plan: r.plan, Errors: r.errs}, nil
}

func (o *Orchestrator) fail(ctx context.Context, cyc gen.Cycle, cause error) error {
	ctx = context.WithoutCancel(ctx)
	now := o.Clock.Now()
	note := "gagal: " + cause.Error()
	_ = o.St.Q.FinishCycle(ctx, gen.FinishCycleParams{ID: cyc.ID, Status: "failed", FinishedAt: &now, Note: &note})
	_ = events.Notify(ctx, o.St.Pool, "cycle_done", map[string]any{"id": cyc.ID, "number": cyc.Number, "status": "failed", "note": note})
	o.log().Error("cycle failed", "id", cyc.ID, "err", cause)
	return cause
}

// stage records a stage transition (running → done/partial) and announces it.
func (o *Orchestrator) stage(ctx context.Context, r *stageRun, name string, fn func(context.Context, *stageRun) (map[string]any, error)) error {
	started := o.Clock.Now()
	_ = o.St.Q.SetCycleStage(ctx, gen.SetCycleStageParams{ID: r.cyc.ID, Stage: &name})
	_ = o.St.Q.UpsertCycleStage(ctx, gen.UpsertCycleStageParams{CycleID: r.cyc.ID, Stage: name, Status: "running", StartedAt: &started})
	_ = events.Notify(ctx, o.St.Pool, "cycle_stage", map[string]any{"cycle_id": r.cyc.ID, "number": r.cyc.Number, "stage": name, "status": "running", "scope": r.scope.String()})
	detail, err := fn(ctx, r)
	if err != nil {
		return err
	}
	status := "done"
	if detail["partial"] == true {
		status, r.partial = "partial", true
	}
	b, _ := json.Marshal(detail)
	fin := o.Clock.Now()
	if err := o.St.Q.UpsertCycleStage(ctx, gen.UpsertCycleStageParams{CycleID: r.cyc.ID, Stage: name, Status: status, StartedAt: &started, FinishedAt: &fin, Detail: b}); err != nil {
		return err
	}
	return events.Notify(ctx, o.St.Pool, "cycle_stage", map[string]any{"cycle_id": r.cyc.ID, "number": r.cyc.Number, "stage": name, "status": status, "detail": detail, "scope": r.scope.String()})
}

// ---------- 1. Ingest ----------

func (o *Orchestrator) ingest(ctx context.Context, r *stageRun) (map[string]any, error) {
	now := o.Clock.Now()
	today := clock.Today(now)
	var dealerFilter *uuid.UUID
	only := ""
	if r.scope.Kind == "dealer" {
		row, err := o.St.Q.GetDealer(ctx, &r.scope.ID)
		if err != nil {
			return nil, fmt.Errorf("dealer %q: %w", r.scope.ID, err)
		}
		dealerFilter, only = &row.ID, deref(row.Slug)
	}
	if r.scope.Kind == "all" {
		if err := o.St.Q.ExpireOpenProposals(ctx, today); err != nil {
			return nil, err
		}
	}
	sigs, err := o.St.Q.UnprocessedSignals(ctx, dealerFilter)
	if err != nil {
		return nil, err
	}
	// signals of the window since the last cycle (24 h for the first) count as new; an older backlog is marked
	// processed without counting (first run on a seeded or freshly synced database)
	since := now.Add(-24 * time.Hour)
	if last, err := o.St.Q.LatestDoneCycle(ctx); err == nil && last.StartedAt.After(since) {
		since = last.StartedAt
	}
	counts := map[string]int{}
	touched := map[uuid.UUID]bool{}
	for _, s := range sigs {
		r.signals = append(r.signals, s.ID)
		if s.OccurredAt.Before(since) {
			continue
		}
		r.recent++
		counts[s.Kind]++
		if s.DealerID != nil {
			touched[*s.DealerID] = true
		}
	}
	// a threshold that feeds the metrics changed since the last cycle: every dealer is recomputed under it
	policyChanged := false
	if last, err := o.St.Q.LatestDoneCycle(ctx); err == nil {
		policyChanged, _ = o.St.Q.PoliciesChangedSince(ctx, last.StartedAt)
	}
	if policyChanged {
		if _, err := dealersvc.New(o.St, o.Clock).Recompute(ctx); err != nil {
			return nil, err
		}
	} else if len(touched) > 0 {
		ids := make([]uuid.UUID, 0, len(touched))
		for id := range touched {
			ids = append(ids, id)
		}
		if _, err := dealersvc.New(o.St, o.Clock).Recompute(ctx, ids...); err != nil {
			return nil, err
		}
	}
	cid := r.cyc.ID.String()
	in, _, err := proposals.BuildInput(ctx, o.St, o.Clock, only, &cid)
	if err != nil {
		return nil, err
	}
	r.in = in
	branches := map[string]bool{}
	for _, s := range in.Stock {
		branches[s.Branch] = true
	}
	wa := counts["wa"] + counts["wa_group"]
	text := fmt.Sprintf("%d WA · %d SO · %d bayar · stok %d cabang", wa, counts["so"], counts["payment"], len(branches))
	return map[string]any{"signals": r.recent, "wa": wa, "so": counts["so"], "payments": counts["payment"], "branches": len(branches),
		"dealers": len(in.Dealers), "recomputed": len(touched), "policy_changed": policyChanged, "text": text}, nil
}

// ---------- 2. Analisis ----------

func (o *Orchestrator) agentList(sc domain.Scope) []agents.Agent {
	list := o.Agents
	if len(list) == 0 {
		list = proposals.All()
	}
	names := sc.Agents()
	if names == nil {
		return list
	}
	var out []agents.Agent
	for _, a := range list {
		for _, n := range names {
			if a.Name() == n {
				out = append(out, a)
			}
		}
	}
	return out
}

func (o *Orchestrator) analyze(ctx context.Context, r *stageRun) (map[string]any, error) {
	list := o.agentList(r.scope)
	if r.in.Policies.LLM.Mode == "mcp" {
		return o.analyzeViaMCP(ctx, r, list)
	}
	r.byAgent = map[string][]domain.Proposal{}
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	cid := r.cyc.ID
	for _, a := range list {
		g.Go(func() error {
			t0 := time.Now()
			actx, cancel := context.WithTimeout(gctx, 2*time.Minute)
			defer cancel()
			ps, err := a.Analyze(actx, r.in, o.Router)
			var valid []domain.Proposal
			for _, p := range ps {
				if verr := p.Validate(a.Kinds()); verr != nil {
					o.log().Warn("proposal rejected by domain", "agent", a.Name(), "title", p.Title, "err", verr)
					continue
				}
				valid = append(valid, p)
			}
			status, msg := "done", (*string)(nil)
			if err != nil {
				status = "failed"
				e := err.Error()
				msg = &e
			}
			hash := llm.Hash(a.Name(), []byte(r.cyc.ID.String()))
			_ = o.St.Q.InsertAgentRun(context.WithoutCancel(ctx), gen.InsertAgentRunParams{CycleID: &cid, Agent: a.Name(), Status: &status,
				DurationMs: i32(int(time.Since(t0).Milliseconds())), InputHash: &hash, ProposalsCount: i32(len(valid)), Error: msg})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				r.errs[a.Name()] = err.Error()
				return nil // a failing agent makes the stage partial; the others continue
			}
			r.byAgent[a.Name()] = valid
			return nil
		})
	}
	_ = g.Wait()
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
	text := fmt.Sprintf("%d agen paralel", len(list)) //nolint:misspell // Indonesian UI text
	if len(list) == 1 {
		text = list[0].Name()
	}
	return map[string]any{"agents": len(list), "proposals": n, "failed": len(r.errs), "partial": len(r.errs) > 0, "text": text}, nil
}

// ---------- 3. Sintesis & konflik ----------

func (o *Orchestrator) synthesize(ctx context.Context, r *stageRun) (map[string]any, error) {
	today := clock.Today(o.Clock.Now())
	ri := RuleInput{Policies: r.in.Policies, Today: o.Clock.Now(), LastFollowup: map[uuid.UUID]time.Time{}, Interactions: map[uuid.UUID][]SalesCount{}}
	supp, err := o.St.Q.ActiveSuppressions(ctx, &today)
	if err != nil {
		return nil, err
	}
	for _, s := range supp {
		if s.Agent != nil && s.Kind != nil && s.DealerID != nil && s.SuppressUntil != nil {
			ri.Suppressions = append(ri.Suppressions, Suppression{Agent: *s.Agent, Kind: *s.Kind, DealerID: *s.DealerID, Until: *s.SuppressUntil})
		}
	}
	if ls, err := o.St.Q.ActiveLessons(ctx, &today); err == nil {
		for _, l := range ls {
			ri.Lessons = append(ri.Lessons, LessonFromRow(l))
		}
	} else {
		return nil, err
	}
	since := o.Clock.Now().AddDate(0, 0, -30)
	for _, d := range r.in.Dealers {
		if d.LastFollowupAt != nil {
			ri.LastFollowup[d.UUID] = *d.LastFollowupAt
		}
		id := d.UUID
		rows, err := o.St.Q.SalesInteractions(ctx, gen.SalesInteractionsParams{DealerID: &id, Since: since})
		if err != nil {
			return nil, err
		}
		for _, x := range rows {
			ri.Interactions[d.UUID] = append(ri.Interactions[d.UUID], SalesCount{Sales: x.Sales, N: x.N})
		}
	}
	ri.Dealer = r.in.Dealer
	ri.MakeCollect = func(d *agents.Dealer) *domain.Proposal {
		one := *r.in
		one.Dealers = []*agents.Dealer{d}
		ps, err := agents.Collect{}.Analyze(ctx, &one, o.Router)
		if err != nil || len(ps) == 0 {
			return nil
		}
		return &ps[0]
	}
	// a scoped run sees today's proposals of the agents it does not run, so the rules (one offer per dealer,
	// collect before follow-up) hold across runs; they are already stored and only anchor the rules
	if anchors, err := o.anchors(ctx, r); err == nil {
		r.cands = append(r.cands, anchors...)
	} else {
		return nil, err
	}
	r.cands, r.conflicts = Synthesize(r.cands, ri)
	for _, c := range r.cands {
		if c.WaitFor != "" {
			c.P.Payload["wait_for"] = c.WaitFor
		}
	}
	n := visible(r.conflicts)
	return map[string]any{"conflicts": n, "merged": count(r.cands, func(c *Cand) bool { return c.Dropped }),
		"suppressed": count(r.cands, func(c *Cand) bool { return c.Suppressed != "" }), "text": fmt.Sprintf("%d konflik diselesaikan", n)}, nil
}

func (o *Orchestrator) anchors(ctx context.Context, r *stageRun) ([]*Cand, error) {
	if r.scope.Kind == "all" {
		return nil, nil
	}
	ran := map[string]bool{}
	for _, a := range o.agentList(r.scope) {
		ran[a.Name()] = true
	}
	if r.scope.Kind == "dealer" {
		return nil, nil // every agent runs for the dealer
	}
	rows, err := o.St.Q.CycleProposals(ctx, clock.Today(o.Clock.Now()))
	if err != nil {
		return nil, err
	}
	var out []*Cand
	for _, p := range rows {
		if ran[primaryAgent(p.Agent)] || (p.Status != "proposed" && p.Status != "approved" && p.Status != "edited") {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(p.Payload, &payload)
		if payload == nil {
			payload = map[string]any{}
		}
		normalizePayload(payload)
		c := &Cand{P: domain.Proposal{Agent: p.Agent, DealerID: p.DealerID, DealerIDs: p.DealerIds, Kind: p.Kind, Title: p.Title, Preview: deref(p.Preview),
			Confidence: p.Confidence, SignalIDs: p.SignalIds, Autonomy: p.Autonomy, Payload: payload, DedupeKey: deref(p.DedupeKey)}, Anchor: true}
		if p.DealerID != nil {
			c.Dealer = r.in.Dealer(*p.DealerID)
		}
		out = append(out, c)
	}
	return out, nil
}

// normalizePayload restores the in-memory shapes the rules read after a JSON round trip.
func normalizePayload(p map[string]any) {
	if xs, ok := p["invoices"].([]any); ok {
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		p["invoices"] = out
	}
	if xs, ok := p["dealers"].([]any); ok {
		out := make([]map[string]any, 0, len(xs))
		for _, x := range xs {
			if m, ok := x.(map[string]any); ok {
				if id, ok := m["id"].(string); ok {
					if u, err := uuid.Parse(id); err == nil {
						m["id"] = u
					}
				}
				out = append(out, m)
			}
		}
		p["dealers"] = out
	}
}

// ---------- 4. Keputusan ----------

func (o *Orchestrator) decide(ctx context.Context, r *stageRun) (map[string]any, error) {
	now := o.Clock.Now()
	cid := r.cyc.ID.String()
	for _, c := range r.cands {
		if c.Dropped || c.Anchor {
			continue
		}
		key := c.P.DedupeKey
		if existing, err := o.St.Q.OpenProposalByKey(ctx, &key); err == nil {
			r.keyIDs[key] = existing.ID // already stored today (idempotent re-run)
			continue
		}
		used, err := o.St.Q.ProposalKeyUsed(ctx, &key)
		if err != nil {
			return nil, err
		}
		if used {
			continue // decided earlier (rejected/expired with the same key)
		}
		status := "proposed"
		v := Evaluate(c, r.in.Policies)
		if c.Suppressed != "" {
			status = "suppressed"
		} else if v.Auto {
			c.P.Autonomy = "auto"
		} else {
			c.P.Autonomy = "approve"
		}
		id, err := proposals.Insert(ctx, o.St.Q, c.P, status, &cid, now)
		if err != nil {
			return nil, fmt.Errorf("store %q: %w", c.P.Title, err)
		}
		if id == uuid.Nil {
			continue
		}
		r.keyIDs[key] = id
		r.stored++
		// auto steps that do not reach a dealer (SO draft in Odoo, credit hold) are approved by the system now;
		// auto steps that message a dealer wait in the plan (ADR 0008)
		if status == "proposed" && c.P.Autonomy == "auto" && !MessagesDealer(c.P.Kind) {
			reason := "otonom · dalam batas kebijakan"
			if err := o.St.Q.ApproveAuto(ctx, gen.ApproveAutoParams{ID: id, DecidedAt: &now, DecisionReason: &reason}); err != nil {
				return nil, err
			}
		}
	}
	for _, cf := range r.conflicts {
		var ids []uuid.UUID
		for _, k := range cf.Keys {
			if id, ok := r.keyIDs[k]; ok {
				ids = append(ids, id)
			}
		}
		var dealer *uuid.UUID
		if cf.DealerID != nil {
			if d := r.in.DealerBySlug(*cf.DealerID); d != nil {
				dealer = &d.UUID
			}
		}
		cyc := r.cyc.ID
		if err := o.St.Q.InsertConflict(ctx, gen.InsertConflictParams{CycleID: &cyc, DealerID: dealer, AgentA: &cf.AgentA, AgentB: &cf.AgentB, Title: &cf.Title,
			Resolution: &cf.Resolution, Rule: &cf.Rule, ProposalIds: ids, Tone: &cf.Tone, Visible: cf.Visible}); err != nil {
			return nil, err
		}
	}
	// the day's counters and Rencana hari ini come from everything proposed today, so re-runs are idempotent
	today := clock.Today(now)
	rows, err := o.St.Q.CycleProposals(ctx, today)
	if err != nil {
		return nil, err
	}
	var pcs []PlanCand
	for _, p := range rows {
		if p.Status == "suppressed" {
			continue
		}
		if p.Autonomy == "auto" {
			r.auto++
		} else if p.Queue && p.Status == "proposed" {
			r.decisions++
		}
		var payload map[string]any
		_ = json.Unmarshal(p.Payload, &payload)
		pc := PlanCand{ID: p.ID, Key: deref(p.DedupeKey), Agent: p.Agent, Kind: p.Kind, Autonomy: p.Autonomy, Status: p.Status, Title: p.Title, Payload: payload, Order: 1 << 20}
		if p.DealerID != nil {
			pc.Dealer = r.in.Dealer(*p.DealerID)
			for i, d := range r.in.Dealers {
				if d.UUID == *p.DealerID {
					pc.Order = i
				}
			}
			if pc.Dealer == nil && r.scope.Kind != "all" {
				continue
			}
		}
		pcs = append(pcs, pc)
	}
	if r.scope.Kind == "all" {
		r.plan = Build(pcs, now)
		if err := o.writePlan(ctx, r, pcs, today); err != nil {
			return nil, err
		}
	}
	return map[string]any{"auto": r.auto, "decisions": r.decisions, "stored": r.stored, "plan": len(r.plan),
		"text": fmt.Sprintf("%d otonom · %d ke Anda", r.auto, r.decisions)}, nil
}

func (o *Orchestrator) writePlan(ctx context.Context, r *stageRun, pcs []PlanCand, today time.Time) error {
	ids := map[string]uuid.UUID{}
	for _, p := range pcs {
		ids[p.Key] = p.ID
	}
	return o.St.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		if err := q.DeletePlan(ctx, today); err != nil {
			return err
		}
		cyc := r.cyc.ID
		for _, it := range r.plan {
			var pids []uuid.UUID
			for _, k := range it.Keys {
				if id, ok := ids[k]; ok {
					pids = append(pids, id)
				}
			}
			var first *uuid.UUID
			if len(pids) > 0 {
				first = &pids[0]
			}
			agent, auto, text := it.Agent, it.Autonomy, it.Text
			if err := q.InsertPlanItem(ctx, gen.InsertPlanItemParams{PlanDate: today, CycleID: &cyc, Seq: int32(it.Seq), TimeLabel: &it.Time, Agent: &agent,
				Autonomy: &auto, TextHtml: &text, ProposalID: first, ProposalIds: pids, Status: it.Status, Link: strp(it.Link), WaitFor: strp(it.WaitFor)}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------- 5. Eksekusi ----------

func (o *Orchestrator) execute(ctx context.Context, r *stageRun) (map[string]any, error) {
	now := o.Clock.Now()
	today := clock.Today(now)
	pending, err := o.St.Q.ApprovedWithoutOutbox(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pending {
		switch {
		case p.Kind == domain.KindSODraft && p.Autonomy == "auto" && o.OdooWrite:
			payload, _ := json.Marshal(map[string]any{"proposal": p.Payload, "approved_by": "Orchestrator · otonom"})
			if _, err := o.St.Q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: p.ID, Channel: "odoo_so_draft", Payload: payload}); err != nil {
				return nil, err
			}
			r.outbox++
		case p.Autonomy == "auto" && !MessagesDealer(p.Kind):
			// nothing leaves the system before stage 12 (SO draft prepared in Distri ARC, credit hold recorded)
			if err := o.St.Q.SetProposalStatus(ctx, gen.SetProposalStatusParams{ID: p.ID, Status: "executed"}); err != nil {
				return nil, err
			}
		}
	}
	// ADR 0008: auto steps that message dealers are sent by the Orchestrator only when the owner enabled it
	if r.in.Policies.Guard.AutoSendsMessages() {
		n, err := o.autoSend(ctx, today, now)
		if err != nil {
			return nil, err
		}
		r.outbox += n
	}
	// waiting steps are released once the payment they wait for arrived
	plan, err := o.St.Q.ListPlan(ctx, today)
	if err != nil {
		return nil, err
	}
	released := 0
	for _, it := range plan {
		if it.Status != domain.PlanWaiting || it.WaitFor == nil {
			continue
		}
		if inv, ok := strings.CutPrefix(*it.WaitFor, "payment:"); ok {
			if paid, err := o.St.Q.PaymentForInvoice(ctx, &inv); err == nil && paid {
				if err := o.St.Q.ReleaseWaiting(ctx, it.ID); err != nil {
					return nil, err
				}
				released++
			}
		}
	}
	if err := o.St.Q.SyncPlanStatus(ctx, today); err != nil {
		return nil, err
	}
	return map[string]any{"outbox": r.outbox, "released": released, "auto_send": r.in.Policies.Guard.AutoSendsMessages(),
		"text": fmt.Sprintf("%d ke outbox · antrean per sales", r.outbox)}, nil
}

// autoSend approves, as the system, the auto plan steps whose slot has come (only with dealer_messages = auto).
func (o *Orchestrator) autoSend(ctx context.Context, today, now time.Time) (int, error) {
	plan, err := o.St.Q.ListPlan(ctx, today)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range plan {
		if deref(it.Autonomy) != "auto" || it.Status != domain.PlanScheduled || !slotPassed(deref(it.TimeLabel), now) {
			continue
		}
		for _, id := range it.ProposalIds {
			out, err := proposals.ApproveBySystem(ctx, o.St, o.Jobs, o.Clock, id)
			if err != nil {
				o.log().Warn("auto send failed", "proposal", id, "err", err)
				continue
			}
			if out.OutboxID != nil {
				n++
			}
		}
	}
	return n, nil
}

func slotPassed(label string, now time.Time) bool {
	t, err := time.ParseInLocation("15.04", label, now.Location())
	if err != nil {
		return false
	}
	slot := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	return !now.Before(slot)
}

// ---------- 6. Belajar ----------

func (o *Orchestrator) learn(ctx context.Context, r *stageRun) (map[string]any, error) {
	now := o.Clock.Now()
	since := clock.Today(now)
	if last, err := o.St.Q.LatestDoneCycle(ctx); err == nil {
		since = last.StartedAt
	}
	rej, err := o.St.Q.RejectedSince(ctx, since)
	if err != nil {
		return nil, err
	}
	policyReview := 0
	for _, x := range rej {
		if strings.HasPrefix(deref(x.Reason), "Tidak sesuai kebijakan") {
			policyReview++
		}
	}
	r.learned = len(rej)
	edited, err := o.St.Q.EditedSince(ctx, since)
	if err != nil {
		return nil, err
	}
	examples := map[string][]map[string]any{}
	for _, e := range edited {
		var ep map[string]string
		_ = json.Unmarshal(e.EditedPayload, &ep)
		if len(examples[e.Agent]) < 20 {
			examples[e.Agent] = append(examples[e.Agent], map[string]any{"kind": e.Kind, "draft": deref(e.Preview), "edited": ep["preview"]})
		}
	}
	stats, err := o.St.Q.AgentDecisionStats(ctx, now.AddDate(0, 0, -30))
	if err != nil {
		return nil, err
	}
	byAgent := map[string]gen.AgentDecisionStatsRow{}
	for _, s := range stats {
		byAgent[primaryAgent(s.Agent)] = gen.AgentDecisionStatsRow{Agent: s.Agent, Accepted: byAgent[primaryAgent(s.Agent)].Accepted + s.Accepted, Rejected: byAgent[primaryAgent(s.Agent)].Rejected + s.Rejected}
	}
	for _, a := range o.agentList(r.scope) {
		st := byAgent[a.Name()]
		var conf *int16
		if n := st.Accepted + st.Rejected; n > 0 {
			v := int16((st.Accepted*100 + n/2) / n)
			conf = &v
		}
		var params []byte
		if ex := examples[a.Name()]; len(ex) > 0 {
			params, _ = json.Marshal(map[string]any{"examples": ex})
		}
		out := summarize(a.Name(), r.byAgent[a.Name()])
		if err := o.St.Q.UpsertAgentState(ctx, gen.UpsertAgentStateParams{Agent: a.Name(), Confidence: conf, LastRunAt: &now, LastOutput: &out, Params: params}); err != nil {
			return nil, err
		}
	}
	memos, err := o.refreshMemos(ctx, r)
	if err != nil {
		return nil, err
	}
	lessons, err := o.learnLessons(ctx)
	if err != nil {
		return nil, err
	}
	if r.scope.Kind == "all" {
		if err := o.writeBrief(ctx, r); err != nil {
			return nil, err
		}
	}
	if len(r.signals) > 0 {
		if err := o.St.Q.MarkSignalsProcessed(ctx, gen.MarkSignalsProcessedParams{At: now, Ids: r.signals}); err != nil {
			return nil, err
		}
	}
	return map[string]any{"calibrations": r.learned, "policy_review": policyReview, "examples": len(edited), "memos": memos, "lessons": lessons,
		"text": fmt.Sprintf("%d kalibrasi · %d memo diperbarui", r.learned, memos)}, nil
}

// summarize is the agent card's "hasil siklus terakhir" line.
func summarize(agent string, ps []domain.Proposal) string {
	if len(ps) == 0 {
		return "Tidak ada saran baru"
	}
	kinds := map[string]int{}
	var order []string
	for _, p := range ps {
		if kinds[p.Kind] == 0 {
			order = append(order, p.Kind)
		}
		kinds[p.Kind]++
	}
	label := map[string]string{domain.KindFollowup: "follow-up", domain.KindCollect: "pengingat", domain.KindInstallment: "skema cicilan",
		domain.KindCreditRelease: "rilis kredit", domain.KindCreditLimit: "usul limit", domain.KindCreditHold: "tahan SO", domain.KindPriceCounter: "nego harga",
		domain.KindPushStock: "bundle stok", domain.KindSODraft: "SO draft", domain.KindReturn: "retur", domain.KindNewDealer: "nomor baru", domain.KindPriceList: "daftar harga"}
	var parts []string
	for _, k := range order {
		parts = append(parts, fmt.Sprintf("%d %s", kinds[k], label[k]))
	}
	_ = agent
	return strings.Join(parts, " · ")
}

// note is the one-line summary of the cycle (template; the LLM may rephrase it later).
func (o *Orchestrator) note(ctx context.Context, r *stageRun) string {
	_ = ctx
	via := strings.ToUpper(deref(r.cyc.Via))
	if r.scope.Kind == "all" {
		n := fmt.Sprintf("Rencana hari ini disusun · %d saran baru", r.stored)
		if r.learned > 0 {
			n += fmt.Sprintf(" · %d kalibrasi dari penolakan", r.learned)
		}
		if r.cyc.Trigger == "manual" {
			n = fmt.Sprintf("Analisis ulang semua (%s) · %d saran baru", via, r.stored)
		}
		return n
	}
	if r.stored == 0 {
		return fmt.Sprintf("Analisis ulang %s (%s) · saran diperbarui, tidak ada keputusan baru", r.scope.Label(), via)
	}
	return fmt.Sprintf("Analisis ulang %s (%s) · %d saran baru", r.scope.Label(), via, r.stored)
}

// refreshed counts the proposals this run re-checked with the newest signals ("N saran diperbarui").
func refreshed(cs []*Cand) int {
	return count(cs, func(c *Cand) bool { return !c.Anchor && !c.Dropped })
}

func visible(cs []domain.Conflict) int {
	n := 0
	for _, c := range cs {
		if c.Visible {
			n++
		}
	}
	return n
}

func count(cs []*Cand, f func(*Cand) bool) int {
	n := 0
	for _, c := range cs {
		if f(c) {
			n++
		}
	}
	return n
}

func i32(n int) *int32 { v := int32(n); return &v }

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
