package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
)

// Analisa Claude on the chat's tab Main (Sam 22 Oct): what Claude wrote through the Orchestrator about this
// customer, start to end — the company (Analisa Dealer), the conversation or customer group (Analisa WhatsApp lines,
// oldest first) and the cooperation (Claude's proposals and what a person decided). Only AI-written text, each with
// its cycle, time and confidence (provenance); counts of messages are facts from the chat.

// AIOverview is GET /chat/threads/{id}/ai-overview.
type AIOverview struct {
	Company   *AICompany   `json:"company"`
	Chat      AIChatFacts  `json:"chat"`
	Timeline  []AILine     `json:"timeline"`
	Proposals []AIProposal `json:"proposals"`
}

// AICompany is the latest Analisa Dealer Claude wrote for the dealer of the chat.
type AICompany struct {
	Status      string    `json:"status"` // bermasalah | perlu_perhatian | aman
	Overview    string    `json:"overview"`
	Reasons     []string  `json:"reasons"`
	Confidence  *int32    `json:"confidence"`
	CycleNumber *int64    `json:"cycle_number"`
	At          time.Time `json:"at"`
}

// AIChatFacts is the conversation from its first stored message to its last.
type AIChatFacts struct {
	Messages int        `json:"messages"`
	Inbound  int        `json:"inbound"`
	FirstAt  *time.Time `json:"first_at"`
	LastAt   *time.Time `json:"last_at"`
	Notes    int        `json:"notes"` // messages with a Claude note
}

// AILine is one line Claude wrote on the conversation in a cycle.
type AILine struct {
	CycleNumber *int64          `json:"cycle_number"`
	At          time.Time       `json:"at"`
	Intent      string          `json:"intent"`
	Items       json.RawMessage `json:"items"`
	Summary     string          `json:"summary"`
	Confidence  *int16          `json:"confidence"`
}

// AIProposal is one proposal Claude made for the dealer, with the person's decision.
type AIProposal struct {
	ID          uuid.UUID  `json:"id"`
	Agent       string     `json:"agent"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Why         string     `json:"why"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
	DecidedBy   string     `json:"decided_by,omitempty"`
	CycleNumber *int64     `json:"cycle_number"`
}

// maxAILines keeps the timeline readable: the first line (the start) and the latest ones.
const maxAILines = 20

func (s *Server) chatAIOverview(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	out := AIOverview{Timeline: []AILine{}, Proposals: []AIProposal{}}
	fail := func(err error) { httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error()) }

	f, err := s.st.Q.ThreadAIFacts(ctx, t.ID)
	if err != nil {
		fail(err)
		return
	}
	out.Chat = AIChatFacts{Messages: int(f.Messages), Inbound: int(f.Inbound), Notes: int(f.Notes)}
	if f.Messages > 0 {
		out.Chat.FirstAt, out.Chat.LastAt = &f.FirstAt, &f.LastAt
	}

	rows, err := s.st.Q.ThreadAISummaries(ctx, t.ID)
	if err != nil {
		fail(err)
		return
	}
	for _, x := range rows {
		// a line copied unchanged to a later cycle (written before summary_at was kept) is the same reading
		if n := len(out.Timeline); n > 0 && out.Timeline[n-1].Summary == x.Summary {
			continue
		}
		out.Timeline = append(out.Timeline, AILine{CycleNumber: x.CycleNumber, At: x.At, Intent: x.Intent, Items: x.Items, Summary: x.Summary, Confidence: x.Confidence})
	}
	if n := len(out.Timeline); n > maxAILines {
		out.Timeline = append(out.Timeline[:1], out.Timeline[n-maxAILines+1:]...)
	}

	if t.DealerID != nil {
		a, err := s.st.Q.LatestDealerAnalysis(ctx, *t.DealerID)
		switch {
		case err == nil:
			var reasons []string
			_ = json.Unmarshal(a.Reasons, &reasons)
			if reasons == nil {
				reasons = []string{}
			}
			out.Company = &AICompany{Status: a.Status, Overview: a.Overview, Reasons: reasons, Confidence: a.Confidence, CycleNumber: a.CycleNumber, At: a.CreatedAt}
		case !errors.Is(err, pgx.ErrNoRows):
			fail(err)
			return
		}
		ps, err := s.st.Q.DealerClaudeProposals(ctx, *t.DealerID)
		if err != nil {
			fail(err)
			return
		}
		for _, p := range ps {
			out.Proposals = append(out.Proposals, AIProposal{ID: p.ID, Agent: p.Agent, Kind: p.Kind, Title: p.Title, Why: p.Why, Status: p.Status,
				CreatedAt: p.CreatedAt, DecidedAt: p.DecidedAt, DecidedBy: p.DecidedByName, CycleNumber: p.CycleNumber})
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}
