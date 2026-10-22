package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
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

// AILine is one thing Claude wrote on the conversation: its line in a cycle (summary), the overview of a cycle run
// for this chat alone (overview), or its note on one customer message (note).
type AILine struct {
	Kind        string          `json:"kind"` // summary | overview | note
	CycleNumber *int64          `json:"cycle_number"`
	At          time.Time       `json:"at"`
	Intent      string          `json:"intent,omitempty"`       // key (order, ask_price…)
	IntentLabel string          `json:"intent_label,omitempty"` // as Claude was given it ("Minta order")
	Items       json.RawMessage `json:"items,omitempty"`
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
const maxAILines = 30

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

	lines, err := s.threadAILines(r, t.ID)
	if err != nil {
		fail(err)
		return
	}
	out.Timeline = lines

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

// threadAILines gathers what Claude wrote on a conversation, oldest first, each text once: the lines kept on
// Analisa WhatsApp (wa_insights), the lines and overviews of Claude's MCP answers (a cycle for this chat alone keeps
// them only there) and the notes per customer message. Long histories keep the first (the start) and the latest.
func (s *Server) threadAILines(r *http.Request, thread uuid.UUID) ([]AILine, error) {
	ctx := r.Context()
	var all []AILine
	rows, err := s.st.Q.ThreadAISummaries(ctx, thread)
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		all = append(all, AILine{Kind: "summary", CycleNumber: x.CycleNumber, At: x.At, Intent: x.Intent, Items: x.Items, Summary: x.Summary, Confidence: x.Confidence})
	}
	sub, err := s.st.Q.ThreadAISubmitted(ctx, thread)
	if err != nil {
		return nil, err
	}
	for _, x := range sub {
		all = append(all, AILine{Kind: "summary", CycleNumber: x.CycleNumber, At: x.At, IntentLabel: x.IntentLabel, Items: x.Items, Summary: x.Summary, Confidence: pct(x.Confidence)})
	}
	ovs, err := s.st.Q.ThreadAIOverviews(ctx, thread)
	if err != nil {
		return nil, err
	}
	for _, x := range ovs {
		all = append(all, AILine{Kind: "overview", CycleNumber: x.CycleNumber, At: x.At, Summary: x.Overview})
	}
	notes, err := s.st.Q.ThreadAINotes(ctx, thread)
	if err != nil {
		return nil, err
	}
	for _, x := range notes {
		all = append(all, AILine{Kind: "note", CycleNumber: x.CycleNumber, At: x.SentAt, Intent: x.Intent, Items: x.Items, Summary: x.Note, Confidence: x.Confidence})
	}
	slices.SortStableFunc(all, func(a, b AILine) int { return a.At.Compare(b.At) })
	out := []AILine{}
	seen := map[string]bool{}
	for _, l := range all {
		k := strings.ToLower(strings.Join(strings.Fields(l.Summary), " "))
		if k == "" || seen[k] { // a line carried to later cycles, or stored both on wa_insights and in the answer
			continue
		}
		seen[k] = true
		if string(l.Items) == "[]" || string(l.Items) == "null" {
			l.Items = nil
		}
		out = append(out, l)
	}
	if n := len(out); n > maxAILines {
		out = append(out[:1], out[n-maxAILines+1:]...)
	}
	return out, nil
}

// pct is a 0–1 confidence as 0–100.
func pct(c float64) *int16 {
	if c <= 0 {
		return nil
	}
	v := int16(max(0, min(100, c*100+0.5)))
	return &v
}
