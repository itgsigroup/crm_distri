package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic calls the Messages API through the official Go SDK with structured JSON output.
// Refusals are handled by server-side fallbacks ("default" routing by refusal category).
type Anthropic struct {
	client anthropic.Client
	model  string
	effort anthropic.BetaOutputConfigEffort
}

// NewAnthropic builds the provider (ANTHROPIC_API_KEY or an `ant auth login` profile).
func NewAnthropic(apiKey, model string) *Anthropic {
	opts := []option.RequestOption{option.WithRequestTimeout(60 * time.Second), option.WithMaxRetries(2)}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &Anthropic{client: anthropic.NewClient(opts...), model: model, effort: anthropic.BetaOutputConfigEffortMedium}
}

func (a *Anthropic) Name() string  { return "anthropic" }
func (a *Anthropic) Model() string { return a.model }

// ErrRefused is returned when the model (and its fallback) declined.
var ErrRefused = errors.New("model refused the request")

// Complete sends one request whose answer must match r.Schema.
func (a *Anthropic) Complete(ctx context.Context, r Request) (Response, error) {
	start := time.Now()
	max := int64(r.MaxTokens)
	if max == 0 {
		max = 4000
	}
	msg, err := a.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     a.model,
		MaxTokens: max,
		System:    []anthropic.BetaTextBlockParam{{Text: r.System, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(string(r.Input)))},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: a.effort,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: r.Schema},
		},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		Betas:     []anthropic.AnthropicBeta{"server-side-fallback-2026-07-01"},
	})
	if err != nil {
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			return Response{}, fmt.Errorf("anthropic %d: %w", apierr.StatusCode, err)
		}
		return Response{}, err
	}
	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		return Response{}, ErrRefused
	}
	var text string
	for _, b := range msg.Content {
		if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok {
			text += t.Text
		}
	}
	in := msg.Usage.InputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.CacheReadInputTokens
	return Response{JSON: []byte(text), Provider: "anthropic", Model: msg.Model, TokensIn: in, TokensOut: msg.Usage.OutputTokens,
		CostIDR: Cost(a.model, in, msg.Usage.OutputTokens), Duration: time.Since(start)}, nil
}
