// Package hermes connects GSI Orbit to Lyra, the company's Hermes Agent, through Hermes' OpenAI-compatible API
// server (`hermes gateway`). The Hermes page is a window to Lyra (ADR 0067): Orbit passes each question to a
// conversation Lyra keeps itself (POST /v1/responses) and shows the answer. The Lyra card under a chat bubble asks
// Lyra which products a message is about (POST /v1/chat/completions, ADR 0070). Lyra reads GSI Orbit through its own
// MCP token, so the same rules hold: it reads and proposes, it never decides or sends.
package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Secret keys (table secrets, sealed with SESSION_SECRET).
const (
	SecretURL = "hermes.api_url"
	SecretKey = "hermes.api_key"
)

// Msg is one chat turn sent to Hermes.
type Msg struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
	// Images: photos sent with the turn as data URLs (a customer's product photo, ADR 0070)
	Images []string `json:"-"`
}

// MarshalJSON writes a turn with photos as OpenAI content parts (text first, then each image_url).
func (m Msg) MarshalJSON() ([]byte, error) {
	type plain struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if len(m.Images) == 0 {
		return json.Marshal(plain{m.Role, m.Content})
	}
	parts := []map[string]any{{"type": "text", "text": m.Content}}
	for _, u := range m.Images {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": u}})
	}
	return json.Marshal(map[string]any{"role": m.Role, "content": parts})
}

// Window is Lyra seen from the Hermes page: Orbit passes one question of a conversation Lyra keeps itself and shows
// the answer. Lyra holds the history, memory, tools and model, like in Telegram.
type Window interface {
	Respond(ctx context.Context, sessionKey, conversation, text string) (string, error)
}

// Agent answers a conversation. sessionKey scopes Hermes' long-term memory to one Orbit user.
type Agent interface {
	Chat(ctx context.Context, sessionKey string, msgs []Msg) (string, error)
}

// Client talks to a Hermes API server.
type Client struct {
	BaseURL string // e.g. http://100.64.0.5:8642 (with or without /v1)
	Key     string // API_SERVER_KEY
	HTTP    *http.Client
}

// ErrRejected means Hermes answered 401/403: the API key is wrong.
var ErrRejected = errors.New("hermes menolak API key (401) — cek API_SERVER_KEY di ~/.hermes/.env")

// NormalizeURL checks an http(s) URL and drops a trailing / or /v1.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("URL Hermes harus diawali http:// atau https://, mis. http://100.64.0.5:8642")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1")
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

func (c Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	// an agent run with several MCP tool calls can take minutes
	return &http.Client{Timeout: 5 * time.Minute}
}

func (c Client) do(ctx context.Context, method, path string, body any, sessionKey string, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	if sessionKey != "" {
		req.Header.Set("X-Hermes-Session-Key", sessionKey)
	}
	res, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("hermes tidak bisa dihubungi: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return ErrRejected
	case res.StatusCode == http.StatusTooManyRequests:
		return errors.New("hermes sedang sibuk (terlalu banyak permintaan) — coba lagi sebentar")
	case res.StatusCode >= 300:
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("hermes menjawab %d: %s", res.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("jawaban Hermes tidak terbaca: %w", err)
	}
	return nil
}

// Verify checks the server is up and the key is accepted (GET /v1/models needs the key).
func (c Client) Verify(ctx context.Context) error {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	return c.do(ctx, http.MethodGet, "/v1/models", nil, "", &out)
}

// Chat sends the conversation and returns Hermes' answer.
func (c Client) Chat(ctx context.Context, sessionKey string, msgs []Msg) (string, error) {
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/chat/completions", map[string]any{"model": "hermes-agent", "messages": msgs, "stream": false}, sessionKey, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("hermes tidak memberi jawaban")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// Respond sends one question to a named conversation on Hermes' Responses API (POST /v1/responses, store = true):
// Hermes keeps the conversation, so only the new question goes over the wire.
func (c Client) Respond(ctx context.Context, sessionKey, conversation, text string) (string, error) {
	var out struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	body := map[string]any{"model": "hermes-agent", "input": text, "conversation": conversation, "store": true}
	if err := c.do(ctx, http.MethodPost, "/v1/responses", body, sessionKey, &out); err != nil {
		return "", err
	}
	var parts []string
	for _, o := range out.Output {
		if o.Type != "message" {
			continue
		}
		for _, p := range o.Content {
			if t := strings.TrimSpace(p.Text); p.Type == "output_text" && t != "" {
				parts = append(parts, t)
			}
		}
	}
	if len(parts) == 0 {
		if out.Error != nil && out.Error.Message != "" {
			return "", errors.New("hermes gagal menjawab: " + out.Error.Message)
		}
		return "", errors.New("hermes tidak memberi jawaban")
	}
	return strings.Join(parts, "\n\n"), nil
}
