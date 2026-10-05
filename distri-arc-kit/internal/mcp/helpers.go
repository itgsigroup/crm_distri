package mcp

import (
	"context"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/proposals"
)

func insertProposal(ctx context.Context, s *Server, p domain.Proposal) (uuid.UUID, error) {
	if err := p.Validate([]string{p.Kind}); err != nil {
		return uuid.Nil, err
	}
	id, err := proposals.Insert(ctx, s.St.Q, p, "proposed", nil, s.Clock.Now())
	if err == nil {
		_ = events.Notify(ctx, s.St.Pool, "proposal_changed", map[string]any{"id": id, "source": "mcp"})
	}
	return id, err
}
