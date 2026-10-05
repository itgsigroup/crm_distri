package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenAI is the backup provider (Chat Completions with a strict JSON schema), used when Anthropic fails twice.
type OpenAI struct {
	APIKey, ModelID, BaseURL string
	HTTP                     *http.Client
}

// NewOpenAI builds the backup provider.
func NewOpenAI(key, model string) *OpenAI {
	return &OpenAI{APIKey: key, ModelID: model, BaseURL: "https://api.openai.com/v1", HTTP: &http.Client{Timeout: 60 * time.Second}}
}

func (o *OpenAI) Name() string  { return "openai" }
func (o *OpenAI) Model() string { return o.ModelID }

// Complete sends one structured request.
func (o *OpenAI) Complete(ctx context.Context, r Request) (Response, error) {
	start := time.Now()
	body, _ := json.Marshal(map[string]any{
		"model": o.ModelID,
		"messages": []map[string]string{
			{"role": "system", "content": r.System},
			{"role": "user", "content": string(r.Input)},
		},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "schema": r.Schema, "strict": true}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := o.HTTP.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return Response{}, err
	}
	if out.Error != nil || len(out.Choices) == 0 {
		msg := res.Status
		if out.Error != nil {
			msg = out.Error.Message
		}
		return Response{}, fmt.Errorf("openai: %s", msg)
	}
	return Response{JSON: []byte(out.Choices[0].Message.Content), Provider: "openai", Model: o.ModelID, TokensIn: out.Usage.PromptTokens,
		TokensOut: out.Usage.CompletionTokens, CostIDR: Cost(o.ModelID, out.Usage.PromptTokens, out.Usage.CompletionTokens), Duration: time.Since(start)}, nil
}
