package proposals

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// All returns the stage-05 agents (Orchestrator adds the rest in stage 09).
func All() []agents.Agent { return []agents.Agent{agents.Order{}, agents.Followup{}, agents.Credit{}} }

// Runner runs agents and stores their proposals (agents.RunAll in docs/stages/05; wrapped by the Orchestrator).
type Runner struct {
	St     *store.Store
	Clock  clock.Clock
	Router *llm.Router
	Log    *slog.Logger
	Agents []agents.Agent
}

// RunOptions narrows a run.
type RunOptions struct {
	Agent   string  // only this agent ("AI Follow-up"); empty = all
	Dealer  string  // only this dealer (slug or id)
	CycleID *string // Orchestrator cycle
}

// Stored is one proposal as stored.
type Stored struct {
	ID       uuid.UUID       `json:"id"`
	Proposal domain.Proposal `json:"proposal"`
	Status   string          `json:"status"`
}

// Result summarises a run.
type Result struct {
	Stored     []Stored          `json:"stored"`
	Suppressed int               `json:"suppressed"`
	Duplicates int               `json:"duplicates"`
	Invalid    int               `json:"invalid"`
	ByAgent    map[string]int    `json:"by_agent"`
	Errors     map[string]string `json:"errors"`
}

// Analyze runs the agents and returns their (validated) proposals without storing them.
func (r *Runner) Analyze(ctx context.Context, o RunOptions) (*agents.Input, map[string][]domain.Proposal, map[string]string, error) {
	in, _, err := BuildInput(ctx, r.St, r.Clock, o.Dealer, o.CycleID)
	if err != nil {
		return nil, nil, nil, err
	}
	list := r.Agents
	if len(list) == 0 {
		list = All()
	}
	out := map[string][]domain.Proposal{}
	errs := map[string]string{}
	for _, a := range list {
		if o.Agent != "" && a.Name() != o.Agent {
			continue
		}
		ps, err := a.Analyze(ctx, in, r.Router)
		if err != nil {
			errs[a.Name()] = err.Error()
			continue
		}
		for _, p := range ps {
			if err := p.Validate(a.Kinds()); err != nil {
				if r.Log != nil {
					r.Log.Warn("proposal rejected by domain", "agent", a.Name(), "title", p.Title, "err", err)
				}
				continue
			}
			out[a.Name()] = append(out[a.Name()], p)
		}
	}
	return in, out, errs, nil
}

// Run analyses and stores. Stage-05 autonomy: only a complete SO draft (Odoo draft, nothing to the dealer) and
// a credit hold run without a human; every message to a dealer waits for a decision.
func (r *Runner) Run(ctx context.Context, o RunOptions) (Result, error) {
	res := Result{ByAgent: map[string]int{}, Errors: map[string]string{}}
	today := clock.Today(r.Clock.Now())
	if err := r.St.Q.ExpireOpenProposals(ctx, today); err != nil {
		return res, err
	}
	_, byAgent, errs, err := r.Analyze(ctx, o)
	if err != nil {
		return res, err
	}
	res.Errors = errs
	supp, err := r.St.Q.ActiveSuppressions(ctx, &today)
	if err != nil {
		return res, err
	}
	for _, a := range All() {
		for _, p := range byAgent[a.Name()] {
			used, err := r.St.Q.ProposalKeyUsed(ctx, &p.DedupeKey)
			if err != nil {
				return res, err
			}
			if used {
				res.Duplicates++
				continue
			}
			status := "proposed"
			for _, s := range supp {
				if s.Agent != nil && *s.Agent == p.Agent && s.Kind != nil && *s.Kind == p.Kind && s.DealerID != nil && p.DealerID != nil && *s.DealerID == *p.DealerID {
					status = "suppressed"
				}
			}
			auto := (p.Kind == domain.KindSODraft && p.Payload["complete"] == true) || p.Kind == domain.KindCreditHold
			if auto {
				p.Autonomy = "auto"
			}
			id, err := Insert(ctx, r.St.Q, p, status, o.CycleID)
			if err != nil {
				return res, fmt.Errorf("store %q: %w", p.Title, err)
			}
			if id == uuid.Nil {
				res.Duplicates++
				continue
			}
			if status == "suppressed" {
				res.Suppressed++
			} else if auto {
				now := r.Clock.Now()
				reason := "otonom · dalam batas kebijakan"
				if err := r.St.Q.ApproveAuto(ctx, gen.ApproveAutoParams{ID: id, DecidedAt: &now, DecisionReason: &reason}); err != nil {
					return res, err
				}
				status = "approved"
			}
			res.Stored = append(res.Stored, Stored{ID: id, Proposal: p, Status: status})
			res.ByAgent[p.Agent]++
		}
	}
	_ = events.Notify(ctx, r.St.Pool, "proposal_changed", map[string]any{"created": len(res.Stored)})
	return res, nil
}

// Insert stores one proposal (no-op when an open proposal with the same dedupe key exists).
func Insert(ctx context.Context, q *gen.Queries, p domain.Proposal, status string, cycleID *string) (uuid.UUID, error) {
	steps, _ := json.Marshal(nonNil(p.Steps))
	impact, _ := json.Marshal(nonNil(p.Impact))
	pills, _ := json.Marshal(nonNil(p.Pills))
	options, _ := json.Marshal(nonNil(p.Options))
	payload, _ := json.Marshal(p.Payload)
	var cyc *uuid.UUID
	if cycleID != nil {
		if id, err := uuid.Parse(*cycleID); err == nil {
			cyc = &id
		}
	}
	id, err := q.InsertAgentProposal(ctx, gen.InsertAgentProposalParams{
		CycleID: cyc, Agent: p.Agent, DealerID: p.DealerID, Kind: p.Kind, Title: p.Title, Why: p.Why, Prep: strp(p.Prep), Preview: strp(p.Preview),
		Steps: steps, Impact: impact, Confidence: p.Confidence, SignalIds: p.SignalIDs, Autonomy: p.Autonomy, Status: status, DueLabel: strp(p.DueLabel),
		Summary: strp(p.Summary), Button: strp(p.Button), Icon: strp(p.Icon), Pills: pills, Options: options, Queue: domain.QueueKinds[p.Kind],
		Payload: payload, DedupeKey: &p.DedupeKey,
	})
	if err != nil && err.Error() == "no rows in result set" {
		return uuid.Nil, nil
	}
	return id, err
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
