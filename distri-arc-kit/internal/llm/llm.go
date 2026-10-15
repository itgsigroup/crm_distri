// Package llm is the only place that talks to language models (ADR 0003). Agents compute numbers in Go and ask
// a Provider only to write sentences (why, prep, WhatsApp drafts) and a confidence, as JSON that matches a
// schema. Inputs are masked before leaving the server; every call is logged with tokens and estimated cost.
package llm

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"
)

//go:embed prompts/*.md
var Prompts embed.FS

// Prompt returns an embedded prompt template.
func Prompt(name string) string {
	b, err := Prompts.ReadFile("prompts/" + name + ".md")
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Request is one structured completion.
type Request struct {
	Purpose   string          // "followup.draft", "order.extract", … (fixture key, cost report)
	Agent     string          // "AI Follow-up"
	CycleID   *string         // links llm_calls to an Orchestrator cycle
	System    string          // system prompt (Bahasa Indonesia rules, glossary)
	Input     json.RawMessage // structured facts computed in Go
	Schema    map[string]any  // JSON schema the answer must follow
	MaxTokens int
	Names     []string // person names to mask (PIC), in addition to numbers/emails
	// Fallback is the deterministic answer (Go template) used by the Fake provider and whenever the model fails.
	Fallback json.RawMessage
}

// Response is the model's answer.
type Response struct {
	JSON      json.RawMessage
	Provider  string
	Model     string
	TokensIn  int64
	TokensOut int64
	CostIDR   int64
	Duration  time.Duration
	Fallback  bool // the deterministic fallback was used
}

// Provider completes structured requests.
type Provider interface {
	Name() string
	Model() string
	Complete(ctx context.Context, r Request) (Response, error)
}

// Hash is the input hash stored in llm_calls (of the masked input; the text itself is never stored).
func Hash(system string, input []byte) string {
	h := sha256.New()
	h.Write([]byte(system))
	h.Write(input)
	return hex.EncodeToString(h.Sum(nil))
}

// Price is USD per million tokens (input, output).
type Price struct{ In, Out float64 }

// Prices of the models the API may use, USD per million tokens (input, output). Claude: Anthropic first-party rates
// (claude-api reference, cached 2026-10-06; Haiku 5.5 up to 100K-token prompts). OpenAI: list prices known to GSI
// Orbit — a model the provider lists that is not here has no known price (Cost falls back to Opus-tier rates).
var Prices = map[string]Price{
	"claude-fable-5-1":  {10, 50},
	"claude-fable-5":    {10, 50},
	"claude-opus-5-5":   {4, 20},
	"claude-opus-5":     {5, 25},
	"claude-opus-4-8":   {5, 25},
	"claude-opus-4-7":   {5, 25},
	"claude-opus-4-6":   {5, 25},
	"claude-sonnet-5-5": {2, 10},
	"claude-sonnet-5":   {2, 10},
	"claude-sonnet-4-6": {3, 15},
	"claude-haiku-5-5":  {0.10, 0.50},
	"claude-haiku-4-5":  {1, 5},
	"gpt-5":             {1.25, 10},
	"gpt-5-mini":        {0.25, 2},
	"gpt-5-nano":        {0.05, 0.40},
	"gpt-4.1":           {2, 8},
	"gpt-4.1-mini":      {0.40, 1.60},
	"gpt-4.1-nano":      {0.10, 0.40},
	"gpt-4o":            {2.50, 10},
	"gpt-4o-mini":       {0.15, 0.60},
	"o3":                {2, 8},
	"o4-mini":           {1.10, 4.40},
}

// dated: a dated snapshot of a model ("gpt-4.1-2025-04-14", "claude-sonnet-4-5-20250929") is priced as the model.
var dated = regexp.MustCompile(`-(\d{4}-\d{2}-\d{2}|\d{8})$`)

// PriceOf returns a model's price and whether it is known.
func PriceOf(model string) (Price, bool) {
	if p, ok := Prices[model]; ok {
		return p, true
	}
	p, ok := Prices[dated.ReplaceAllString(model, "")]
	return p, ok
}

// IDRPerUSD converts cost estimates (LLM_IDR_PER_USD overrides).
var IDRPerUSD = 16500.0

// Cost estimates the rupiah cost of a call.
func Cost(model string, in, out int64) int64 {
	p, ok := PriceOf(model)
	if !ok {
		p = Price{4, 20}
	}
	usd := float64(in)/1e6*p.In + float64(out)/1e6*p.Out
	return int64(usd*IDRPerUSD + 0.5)
}
