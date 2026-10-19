package replyai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"distri-arc/internal/analyst"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
)

// The AI that writes AI Trigger recommendations (ADR 0051): chosen on the AI Trigger page, apart from the analysis
// engine. ChatGPT is allowed here only — a card's draft from facts the system computed, called only when a customer
// message matches a trigger — while the rest of GSI Orbit never uses ChatGPT (ADR 0048).

// PolicyKey is the policies row of the engine.
const PolicyKey = "reply.ai"

// Engine is the provider and model AI Trigger drafts are written with.
type Engine struct {
	Provider string `json:"provider"` // openai | claude
	Model    string `json:"model"`
}

// DefaultEngine: ChatGPT gpt-4.1-mini — answers in a second or two, about Rp 15 a draft.
var DefaultEngine = Engine{Provider: analyst.FallbackOpenAI, Model: "gpt-4.1-mini"}

// EngineModels are the models offered per provider: fast ones, a draft must appear while the sales reads the chat.
var EngineModels = map[string][]string{
	analyst.FallbackOpenAI: {"gpt-4.1-mini", "gpt-4.1-nano", "gpt-4o-mini", "gpt-5-mini"},
	analyst.FallbackClaude: {"claude-haiku-5-5", "claude-sonnet-5-5"},
}

// ValidEngine checks provider and model.
func ValidEngine(e Engine) error {
	for _, m := range EngineModels[e.Provider] {
		if m == e.Model {
			return nil
		}
	}
	return errors.New("pilih ChatGPT atau Claude dengan salah satu model yang ditawarkan")
}

// LoadEngine reads the engine; the default when none is saved.
func LoadEngine(ctx context.Context, q *gen.Queries) Engine {
	if p, err := q.GetPolicy(ctx, PolicyKey); err == nil {
		var e Engine
		if json.Unmarshal(p.Value, &e) == nil && ValidEngine(e) == nil {
			return e
		}
	}
	return DefaultEngine
}

// SaveEngine stores the engine as a new policy version.
func SaveEngine(ctx context.Context, q *gen.Queries, e Engine, now time.Time) error {
	if err := ValidEngine(e); err != nil {
		return err
	}
	b, _ := json.Marshal(e)
	_, err := q.SetPolicy(ctx, gen.SetPolicyParams{Key: PolicyKey, Value: b, UpdatedAt: now})
	return err
}

// engineModel is the provider of the engine when its key is set.
func (s *Service) engineModel(ctx context.Context) llm.Provider {
	e := LoadEngine(ctx, s.St.Q)
	if e.Provider == analyst.FallbackOpenAI {
		if k, _ := analyst.LoadOpenAIKeyAITrigger(ctx, s.St.Q, s.SessionKey, s.EnvOpenAI); k != "" {
			return llm.NewOpenAI(k, e.Model)
		}
		return nil
	}
	if k, _ := analyst.LoadKey(ctx, s.St.Q, s.SessionKey, s.EnvClaude); k != "" {
		return llm.NewAnthropic(k, e.Model)
	}
	return nil
}
