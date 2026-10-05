package proposals

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/domain"
	"distri-arc/internal/store/gen"
)

// All returns the six agents (AI Stok, AI Penagihan, AI Prospek in their v1 form; stage 09 deepens them).
func All() []agents.Agent {
	return []agents.Agent{agents.Order{}, agents.Followup{}, agents.Credit{}, agents.Stock{}, agents.Collect{}, agents.Prospect{}}
}

// Insert stores one proposal (no-op when an open proposal with the same dedupe key exists).
func Insert(ctx context.Context, q *gen.Queries, p domain.Proposal, status string, cycleID *string, at time.Time) (uuid.UUID, error) {
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
		Payload: payload, DedupeKey: &p.DedupeKey, DealerIds: p.DealerIDs, CreatedAt: at,
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
