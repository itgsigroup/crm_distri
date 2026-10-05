package llm

import (
	"log/slog"
	"strings"

	"distri-arc/internal/store"
)

// Config selects the providers (LLM_PROVIDER, keys) and models (policy llm.routing, LLM_MODEL overrides).
type Config struct {
	Provider     string // fake | anthropic
	AnthropicKey string
	OpenAIKey    string
	Model        string // e.g. claude-sonnet-5-5
	Fallback     string // "openai:gpt-4.1"
	IDRPerUSD    float64
}

// NewRouter builds the router; without an API key it falls back to the fake provider.
func NewRouter(c Config, st *store.Store, log *slog.Logger) *Router {
	if c.IDRPerUSD > 0 {
		IDRPerUSD = c.IDRPerUSD
	}
	r := &Router{St: st, Log: log}
	switch c.Provider {
	case "anthropic":
		r.Primary = NewAnthropic(c.AnthropicKey, c.Model)
	default:
		r.Primary = NewFake(nil)
	}
	if prov, model, ok := strings.Cut(c.Fallback, ":"); ok && prov == "openai" && c.OpenAIKey != "" && c.Provider != "fake" {
		r.Backup = NewOpenAI(c.OpenAIKey, model)
	}
	return r
}
