package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider calls Claude through the official Go SDK. Structured output
// uses output_config.format (JSON schema) so responses always parse.
type AnthropicProvider struct {
	client anthropic.Client
}

// NewAnthropic builds the provider for an API key.
func NewAnthropic(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{client: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

func (p *AnthropicProvider) Name() string   { return "anthropic" }
func (p *AnthropicProvider) External() bool { return true }

func (p *AnthropicProvider) Complete(ctx context.Context, model string, req Request) (Response, error) {
	if model == "" {
		model = "claude-sonnet-5"
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 16000
	}
	msgs := make([]anthropic.MessageParam, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "assistant" {
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		} else {
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		}
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages:  msgs,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	if req.Schema != nil {
		params.OutputConfig = anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: req.Schema}}
	}
	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, err
	}
	if msg.StopReason == anthropic.StopReasonRefusal {
		return Response{}, fmt.Errorf("anthropic: request declined (%s)", msg.StopDetails.Category)
	}
	var sb strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return Response{Text: sb.String(), Provider: p.Name(), Model: string(msg.Model),
		TokensIn: int(msg.Usage.InputTokens), TokensOut: int(msg.Usage.OutputTokens)}, nil
}

// OpenAIProvider calls the OpenAI chat completions REST API (no SDK dependency).
type OpenAIProvider struct {
	key   string
	model string
	http  *http.Client
}

// NewOpenAI builds the provider.
func NewOpenAI(key, model string) *OpenAIProvider {
	if model == "" {
		model = "gpt-4.1-mini"
	}
	return &OpenAIProvider{key: key, model: model, http: &http.Client{}}
}

func (p *OpenAIProvider) Name() string   { return "openai" }
func (p *OpenAIProvider) External() bool { return true }

func (p *OpenAIProvider) Complete(ctx context.Context, model string, req Request) (Response, error) {
	if model == "" || strings.HasPrefix(model, "claude") {
		model = p.model
	}
	msgs := []map[string]string{}
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body := map[string]any{"model": model, "messages": msgs}
	if req.Schema != nil {
		body["response_format"] = map[string]any{"type": "json_object"}
	}
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := postJSON(ctx, p.http, "https://api.openai.com/v1/chat/completions", "Bearer "+p.key, body, &out); err != nil {
		return Response{}, err
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("openai: empty response")
	}
	return Response{Text: out.Choices[0].Message.Content, Provider: p.Name(), Model: out.Model,
		TokensIn: out.Usage.PromptTokens, TokensOut: out.Usage.CompletionTokens}, nil
}

// OllamaProvider calls a self-hosted Ollama server. Data never leaves GSI.
type OllamaProvider struct {
	url   string
	model string
	http  *http.Client
}

// NewOllama builds the provider.
func NewOllama(url, model string) *OllamaProvider {
	if model == "" {
		model = "llama3.1"
	}
	return &OllamaProvider{url: strings.TrimRight(url, "/"), model: model, http: &http.Client{}}
}

func (p *OllamaProvider) Name() string   { return "ollama" }
func (p *OllamaProvider) External() bool { return false }

func (p *OllamaProvider) Complete(ctx context.Context, model string, req Request) (Response, error) {
	if model == "" || strings.HasPrefix(model, "claude") {
		model = p.model
	}
	msgs := []map[string]string{}
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body := map[string]any{"model": model, "messages": msgs, "stream": false}
	if req.Schema != nil {
		body["format"] = req.Schema
	}
	var out struct {
		Model           string                   `json:"model"`
		Message         struct{ Content string } `json:"message"`
		PromptEvalCount int                      `json:"prompt_eval_count"`
		EvalCount       int                      `json:"eval_count"`
	}
	if err := postJSON(ctx, p.http, p.url+"/api/chat", "", body, &out); err != nil {
		return Response{}, err
	}
	return Response{Text: out.Message.Content, Provider: p.Name(), Model: "ollama/" + out.Model,
		TokensIn: out.PromptEvalCount, TokensOut: out.EvalCount}, nil
}

func postJSON(ctx context.Context, c *http.Client, url, auth string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}

// FakeProvider is deterministic: each purpose registers a handler that builds
// the output from Request.FakeInput (fixtures). Used in tests and when no API
// key is configured (status done-with-mocks).
type FakeProvider struct {
	handlers map[string]func(Request) (any, error)
}

// NewFake returns an empty fake provider.
func NewFake() *FakeProvider { return &FakeProvider{handlers: map[string]func(Request) (any, error){}} }

func (p *FakeProvider) Name() string   { return "fake" }
func (p *FakeProvider) External() bool { return false }

// Handle registers the deterministic generator of a purpose.
func (p *FakeProvider) Handle(purpose string, fn func(Request) (any, error)) {
	p.handlers[purpose] = fn
}

func (p *FakeProvider) Complete(_ context.Context, _ string, req Request) (Response, error) {
	fn, ok := p.handlers[req.Purpose]
	if !ok {
		return Response{}, fmt.Errorf("fake: no handler for %q", req.Purpose)
	}
	v, err := fn(req)
	if err != nil {
		return Response{}, err
	}
	var text string
	if s, ok := v.(string); ok {
		text = s
	} else {
		b, err := json.Marshal(v)
		if err != nil {
			return Response{}, err
		}
		text = string(b)
	}
	in := len(req.System)
	for _, m := range req.Messages {
		in += len(m.Content)
	}
	return Response{Text: text, Provider: "fake", Model: "fake-deterministic", TokensIn: in / 4, TokensOut: len(text) / 4}, nil
}
