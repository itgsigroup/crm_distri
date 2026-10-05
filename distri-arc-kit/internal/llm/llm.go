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

// Prices of the models the routing policy may name (Anthropic first-party, 2026-09; OpenAI fallback).
var Prices = map[string]Price{
	"claude-opus-5-5":   {4, 20},
	"claude-sonnet-5-5": {2, 10},
	"claude-haiku-4-5":  {1, 5},
	"claude-fable-5-1":  {10, 50},
	"gpt-4.1":           {2, 8},
}

// IDRPerUSD converts cost estimates (LLM_IDR_PER_USD overrides).
var IDRPerUSD = 16500.0

// Cost estimates the rupiah cost of a call.
func Cost(model string, in, out int64) int64 {
	p, ok := Prices[model]
	if !ok {
		p = Price{4, 20}
	}
	usd := float64(in)/1e6*p.In + float64(out)/1e6*p.Out
	return int64(usd*IDRPerUSD + 0.5)
}
