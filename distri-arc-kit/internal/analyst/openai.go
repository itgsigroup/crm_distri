package analyst

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"distri-arc/internal/auth"
	"distri-arc/internal/clock"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
)

// ChatGPT (OpenAI Chat Completions with function calling) as the server's API for analysis (ADR 0040): the same MCP
// tools, prompt, masking, step limit and cost log as Claude's loop — only the wire format differs.

// SecretOpenAI is the secrets row of the ChatGPT API key.
const SecretOpenAI = "openai.api_key"

// Schedule rows the API second option runs through (ADR 0040).
const (
	FallbackScheduleCycle = "API cadangan · siklus Orchestrator"
	FallbackScheduleWA    = "API cadangan · Analisa WhatsApp"
)

// openAIBase is the Chat Completions endpoint root (tests point it at a fake server).
var openAIBase = "https://api.openai.com/v1"

// ProviderLabel names an API in the UI.
func ProviderLabel(p string) string {
	if p == FallbackOpenAI {
		return "ChatGPT API"
	}
	return "Claude API"
}

// LoadOpenAIKey returns the ChatGPT API key and where it came from: "ui" (sealed in secrets), "env" or "".
func LoadOpenAIKey(ctx context.Context, q *gen.Queries, sessionKey []byte, envKey string) (string, string) {
	if sealed, err := q.GetSecret(ctx, SecretOpenAI); err == nil {
		if k, err := auth.Open(sealed, sessionKey); err == nil && k != "" {
			return k, "ui"
		}
	}
	if envKey != "" {
		return envKey, "env"
	}
	return "", ""
}

// ErrOpenAIKeyRejected means OpenAI answered 401/403 for the key.
var ErrOpenAIKeyRejected = errors.New("kunci ChatGPT API ditolak OpenAI — periksa kuncinya (platform.openai.com → API keys)")

// VerifyOpenAIKey checks a key against the Models API (free; no tokens are used). Network errors are returned as-is.
func VerifyOpenAIKey(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAIBase+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == 401 || res.StatusCode == 403:
		return ErrOpenAIKeyRejected
	case res.StatusCode >= 300:
		return fmt.Errorf("OpenAI menjawab %s", res.Status)
	}
	return nil
}

// ChatRequest / ChatResponse are the parts of Chat Completions the loop uses.
type ChatRequest struct {
	Model               string           `json:"model"`
	Messages            []map[string]any `json:"messages"`
	Tools               []map[string]any `json:"tools,omitempty"`
	MaxCompletionTokens int              `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string           `json:"reasoning_effort,omitempty"` // reasoning models only (gpt-5, o3, o4)
}

// reasoningModel: the ChatGPT models that take reasoning_effort.
func reasoningModel(id string) bool {
	return strings.HasPrefix(id, "gpt-5") || strings.HasPrefix(id, "o3") || strings.HasPrefix(id, "o4")
}

// ChatToolCall is one function call of the model.
type ChatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatResponse is one completion.
type ChatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   *string        `json:"content"`
			Refusal   *string        `json:"refusal"`
			ToolCalls []ChatToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

// ChatModel is one turn with ChatGPT (scripted in tests).
type ChatModel interface {
	Chat(ctx context.Context, r ChatRequest) (*ChatResponse, error)
}

// ChatError is an HTTP error from OpenAI.
type ChatError struct {
	Status  int
	Message string
}

func (e *ChatError) Error() string { return fmt.Sprintf("OpenAI %d: %s", e.Status, e.Message) }

type openAIChat struct {
	key  string
	http *http.Client
}

// NewOpenAIChat calls Chat Completions with key.
func NewOpenAIChat(key string) ChatModel {
	return openAIChat{key: key, http: &http.Client{Timeout: 5 * time.Minute}}
}

func (c openAIChat) Chat(ctx context.Context, r ChatRequest) (*ChatResponse, error) {
	body, _ := json.Marshal(r)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return nil, &ChatError{Status: res.StatusCode, Message: e.Error.Message}
	}
	var out ChatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func chatError(err error) error {
	var ce *ChatError
	if errors.As(err, &ce) {
		switch ce.Status {
		case 401, 403:
			return errors.New("kunci ChatGPT API ditolak (401) — periksa atau ganti kunci")
		case 429:
			return errors.New("batas pemakaian atau saldo ChatGPT API habis (429) — periksa billing OpenAI")
		}
		return fmt.Errorf("galat ChatGPT API %d: %s", ce.Status, ce.Message)
	}
	return err
}

// openai runs the schedule's task with ChatGPT and the MCP tools (the counterpart of claude).
func (r *Runner) openai(ctx context.Context, cs *sdk.ClientSession, m ChatModel, model string, sch gen.McpSchedule, allowed map[string]bool) outcome {
	o := outcome{engine: FallbackOpenAI, model: model}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		o.err = err
		return o
	}
	var tools []map[string]any
	for _, t := range list.Tools {
		if !allowed[t.Name] {
			continue
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": t.Name, "description": t.Description, "parameters": functionSchema(t.InputSchema)}})
	}
	now := r.Clock.Now().In(clock.WIB)
	system := llm.Prompt("analyst") + fmt.Sprintf("\nSaat ini: %s pukul %s WIB. Batas: %d panggilan tool.", longDate(now), now.Format("15.04"), sch.MaxSteps)
	masker := llm.NewMasker()
	req := ChatRequest{Model: model, MaxCompletionTokens: maxTokens, Tools: tools, Messages: []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": sch.Prompt},
	}}
	if r.Effort != "" && reasoningModel(model) {
		req.ReasoningEffort = r.Effort
	}
	calls := 0
	for turn := 0; turn < int(sch.MaxSteps)+stepBuffer; turn++ {
		t0 := time.Now()
		res, err := m.Chat(ctx, req)
		if err != nil {
			o.err = chatError(err)
			r.record(ctx, model, 0, 0, time.Since(t0), true)
			return o
		}
		o.in, o.out = o.in+res.Usage.PromptTokens, o.out+res.Usage.CompletionTokens
		o.cost += llm.Cost(model, res.Usage.PromptTokens, res.Usage.CompletionTokens)
		r.record(ctx, model, res.Usage.PromptTokens, res.Usage.CompletionTokens, time.Since(t0), false)
		if len(res.Choices) == 0 {
			o.err = errors.New("jawaban kosong dari ChatGPT")
			return o
		}
		ch := res.Choices[0]
		if ch.Message.Refusal != nil && *ch.Message.Refusal != "" {
			o.err = errors.New("permintaan ditolak ChatGPT — ubah prompt jadwal")
			return o
		}
		if len(ch.Message.ToolCalls) > 0 {
			msg := map[string]any{"role": "assistant", "tool_calls": ch.Message.ToolCalls}
			if ch.Message.Content != nil {
				msg["content"] = *ch.Message.Content
			}
			req.Messages = append(req.Messages, msg)
			for _, tc := range ch.Message.ToolCalls {
				var text string
				if calls >= int(sch.MaxSteps) {
					text = "Batas panggilan tool tercapai. Tulis laporan akhir sekarang dari data yang sudah ada."
				} else {
					calls++
					args := json.RawMessage(tc.Function.Arguments)
					if !json.Valid(args) {
						args = json.RawMessage("{}")
					}
					var st Step
					text, st = r.call(ctx, cs, tc.Function.Name, args, allowed, masker)
					o.steps = append(o.steps, st)
				}
				req.Messages = append(req.Messages, map[string]any{"role": "tool", "tool_call_id": tc.ID, "content": text})
			}
			continue
		}
		if ch.Message.Content != nil {
			o.report = strings.TrimSpace(masker.Unmask(*ch.Message.Content))
		}
		if ch.FinishReason == "length" {
			o.report += "\n\n_(Laporan terpotong: batas panjang jawaban tercapai.)_"
		}
		if o.report == "" {
			o.err = errors.New("laporan kosong dari ChatGPT")
		}
		return o
	}
	o.err = fmt.Errorf("analisis belum selesai setelah %d langkah — naikkan batas langkah atau persempit prompt", int(sch.MaxSteps)+stepBuffer)
	return o
}

// functionSchema converts an MCP tool's JSON schema to a function's parameters (an object, without $schema).
func functionSchema(s any) map[string]any {
	m := map[string]any{}
	if b, err := json.Marshal(s); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	delete(m, "$schema")
	m["type"] = "object"
	if _, ok := m["properties"]; !ok {
		m["properties"] = map[string]any{}
	}
	return m
}

// FallbackReady reports whether the API second option (ADR 0040) is on, its key is present and today's budget is
// not spent; it returns the provider and model that would run.
func FallbackReady(ctx context.Context, q *gen.Queries, now time.Time, sessionKey []byte, envClaude, envOpenAI string) (bool, string, string) {
	cfg := LoadConfig(ctx, q)
	if cfg.Fallback == FallbackOff {
		return false, "", ""
	}
	var key string
	if cfg.Provider() == FallbackOpenAI {
		key, _ = LoadOpenAIKey(ctx, q, sessionKey, envOpenAI)
	} else {
		key, _ = LoadKey(ctx, q, sessionKey, envClaude)
	}
	if key == "" {
		return false, cfg.Provider(), cfg.ProviderModel()
	}
	if cfg.DailyBudgetIDR > 0 {
		if spent, err := q.AnalystCostSince(ctx, clock.Today(now)); err == nil && spent.Cost >= cfg.DailyBudgetIDR {
			return false, cfg.Provider(), cfg.ProviderModel()
		}
	}
	return true, cfg.Provider(), cfg.ProviderModel()
}

// APIReady reports whether the API chosen on Pengaturan → API & model AI can analyse now, whatever the second
// option is set to (the Orchestrator's "Analisis ulang" with the API, 16 Oct): its key is present and today's budget
// is not spent. It returns the provider, the model and, when not ready, why.
func APIReady(ctx context.Context, q *gen.Queries, now time.Time, sessionKey []byte, envClaude, envOpenAI string) (ok bool, provider, model, why string) {
	cfg := LoadConfig(ctx, q)
	provider, model = cfg.Provider(), cfg.ProviderModel()
	var key string
	if provider == FallbackOpenAI {
		key, _ = LoadOpenAIKey(ctx, q, sessionKey, envOpenAI)
	} else {
		key, _ = LoadKey(ctx, q, sessionKey, envClaude)
	}
	if key == "" {
		return false, provider, model, "Kunci " + ProviderLabel(provider) + " belum diisi (Pengaturan → API & model AI)"
	}
	if cfg.DailyBudgetIDR > 0 {
		if spent, err := q.AnalystCostSince(ctx, clock.Today(now)); err == nil && spent.Cost >= cfg.DailyBudgetIDR {
			return false, provider, model, "Anggaran harian " + ProviderLabel(provider) + " sudah terpakai"
		}
	}
	return true, provider, model, ""
}

// ModelLabel is a model's name as the API & model AI page shows it ("Claude Opus 5.5"); the id when unknown.
func ModelLabel(id string) string {
	for _, m := range append(append([]ModelInfo{}, Models...), OpenAIModels...) {
		if m.ID == id {
			return m.Label
		}
	}
	return id
}
