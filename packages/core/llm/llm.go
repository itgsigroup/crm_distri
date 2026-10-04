// Package llm is ARC's provider abstraction: tier routing (light/heavy/interactive),
// PII masking before external providers, retry + fallback, and a log entry per call.
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Tier selects the model class for a task.
type Tier string

const (
	Light       Tier = "light"       // capture, hygiene
	Heavy       Tier = "heavy"       // deal intelligence, forecast, brief, research
	Interactive Tier = "interactive" // ask
)

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

// Request describes one structured completion.
type Request struct {
	Tier      Tier
	Purpose   string // e.g. "capture", "brief", "ask"
	System    string // system prompt (versioned prompt file)
	Messages  []Message
	Schema    map[string]any // JSON schema of the expected output; nil = free text
	MaxTokens int
	// FakeInput is handed to the deterministic FakeProvider (structured context
	// so tests and mock mode don't depend on prompt wording).
	FakeInput any
}

// Response is the provider output.
type Response struct {
	Text      string
	Provider  string
	Model     string
	TokensIn  int
	TokensOut int
}

// Provider is implemented by Anthropic, OpenAI, Ollama and Fake.
type Provider interface {
	Name() string
	External() bool // true when data leaves the GSI server (PII must be masked)
	Complete(ctx context.Context, model string, req Request) (Response, error)
}

// Call is the audit record for every LLM call (stored in llm_calls).
type Call struct {
	Tier       Tier
	Provider   string
	Model      string
	TokensIn   int
	TokensOut  int
	CostEst    float64 // USD
	Purpose    string
	InputHash  string
	DurationMS int
	OK         bool
	Error      string
}

// Route maps a tier to a provider and model.
type Route struct {
	Provider string
	Model    string
}

// Router dispatches requests by tier.
type Router struct {
	mu        sync.RWMutex
	providers map[string]Provider
	routes    map[Tier]Route
	fallback  string
	logger    func(Call)
	// Payloads captures the final provider payloads in tests (PII intercept).
	Payloads func(provider string, req Request)
}

// NewRouter builds a router; logger may be nil.
func NewRouter(logger func(Call)) *Router {
	return &Router{providers: map[string]Provider{}, routes: map[Tier]Route{}, logger: logger}
}

// Register adds a provider.
func (r *Router) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// SetRoute configures a tier.
func (r *Router) SetRoute(t Tier, route Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[t] = route
}

// SetFallback names the provider used when the primary fails twice.
func (r *Router) SetFallback(provider string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = provider
}

// RouteFor returns the configured route of a tier.
func (r *Router) RouteFor(t Tier) Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routes[t]
}

// Provider returns a registered provider by name.
func (r *Router) Provider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// IsFake reports whether the tier is served by the deterministic provider.
func (r *Router) IsFake(t Tier) bool { return r.RouteFor(t).Provider == "fake" }

// Complete runs the request with masking, two attempts on the primary and one on the fallback.
func (r *Router) Complete(ctx context.Context, req Request) (Response, error) {
	route := r.RouteFor(req.Tier)
	p, ok := r.Provider(route.Provider)
	if !ok {
		return Response{}, fmt.Errorf("llm: no provider %q for tier %s", route.Provider, req.Tier)
	}
	resp, err := r.try(ctx, p, route.Model, req, 2)
	if err == nil {
		return resp, nil
	}
	r.mu.RLock()
	fb := r.fallback
	r.mu.RUnlock()
	if fbp, ok := r.Provider(fb); ok && fb != route.Provider {
		model := route.Model
		if fbp.Name() != "anthropic" {
			model = ""
		}
		if resp2, err2 := r.try(ctx, fbp, model, req, 1); err2 == nil {
			return resp2, nil
		}
	}
	return Response{}, err
}

// CompleteJSON runs Complete and decodes the JSON output into out.
func (r *Router) CompleteJSON(ctx context.Context, req Request, out any) (Response, error) {
	resp, err := r.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	text := extractJSON(resp.Text)
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return resp, fmt.Errorf("llm: decode %s output: %w", req.Purpose, err)
	}
	return resp, nil
}

func (r *Router) try(ctx context.Context, p Provider, model string, req Request, attempts int) (Response, error) {
	send := req
	var masker *Masker
	if p.External() {
		masker = NewMasker()
		send.System = masker.Mask(req.System)
		send.Messages = make([]Message, len(req.Messages))
		for i, m := range req.Messages {
			send.Messages[i] = Message{Role: m.Role, Content: masker.Mask(m.Content)}
		}
	}
	if r.Payloads != nil {
		r.Payloads(p.Name(), send)
	}
	hash := inputHash(send)
	var lastErr error
	for i := 0; i < attempts; i++ {
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		start := time.Now()
		resp, err := p.Complete(cctx, model, send)
		cancel()
		call := Call{Tier: req.Tier, Provider: p.Name(), Model: resp.Model, TokensIn: resp.TokensIn, TokensOut: resp.TokensOut,
			Purpose: req.Purpose, InputHash: hash, DurationMS: int(time.Since(start).Milliseconds()), OK: err == nil}
		if call.Model == "" {
			call.Model = model
		}
		call.CostEst = CostUSD(call.Model, call.TokensIn, call.TokensOut)
		if err != nil {
			call.Error = err.Error()
		}
		if r.logger != nil {
			r.logger(call)
		}
		if err == nil {
			if masker != nil {
				resp.Text = masker.Unmask(resp.Text)
			}
			return resp, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) {
			break
		}
		time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
	}
	return Response{}, lastErr
}

func inputHash(req Request) string {
	h := sha256.New()
	h.Write([]byte(req.System))
	for _, m := range req.Messages {
		h.Write([]byte(m.Role + ":" + m.Content))
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// extractJSON tolerates providers that wrap JSON in prose or code fences.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return s
	}
	end := strings.LastIndexAny(s, "}]")
	if end < start {
		return s[start:]
	}
	return s[start : end+1]
}

// Pricing in USD per million tokens (input, output) for cost estimates.
var Pricing = map[string][2]float64{
	"claude-haiku-4-5":  {1, 5},
	"claude-sonnet-5":   {2, 10},
	"claude-sonnet-5-5": {2, 10},
	"claude-opus-5-5":   {4, 20},
	"claude-opus-5":     {5, 25},
	"fake":              {0, 0},
}

// CostUSD estimates the cost of a call.
func CostUSD(model string, in, out int) float64 {
	p, ok := Pricing[model]
	if !ok {
		p = [2]float64{1, 4}
		if model == "" || strings.HasPrefix(model, "fake") || strings.Contains(model, "ollama") {
			p = [2]float64{0, 0}
		}
	}
	return (float64(in)*p[0] + float64(out)*p[1]) / 1e6
}

// USDToIDR is the conversion used for the cost display.
const USDToIDR = 16300.0
