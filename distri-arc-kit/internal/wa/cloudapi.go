package wa

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// CloudAPI is the official WhatsApp Business Cloud API for one business number (no groups, no history).
// Incoming messages arrive on the webhook (/api/wa/cloud/webhook); sending uses the Graph API.
type CloudAPI struct {
	Account     string // business number digits
	PhoneID     string // Graph phone_number_id
	Token       string
	VerifyToken string
	AppSecret   string
	GraphURL    string // default https://graph.facebook.com/v21.0
	HTTP        *http.Client
	events      chan Event
}

// NewCloudAPI configures the transport.
func NewCloudAPI(account, phoneID, token, verify, secret string) *CloudAPI {
	return &CloudAPI{Account: Digits(account), PhoneID: phoneID, Token: token, VerifyToken: verify, AppSecret: secret,
		GraphURL: "https://graph.facebook.com/v21.0", HTTP: &http.Client{Timeout: 20 * time.Second}, events: make(chan Event, 256)}
}

func (c *CloudAPI) Name() string                { return "cloudapi" }
func (c *CloudAPI) Start(context.Context) error { return nil }
func (c *CloudAPI) Events() <-chan Event        { return c.events }

// Pair is not needed for the Cloud API (the number is registered in Meta Business Manager).
func (c *CloudAPI) Pair(context.Context, string) (string, error) { return "", nil }

// Status reports the business number as connected when it is configured.
func (c *CloudAPI) Status(context.Context) []Status {
	state := "connected"
	if c.Token == "" || c.PhoneID == "" {
		state = "unpaired"
	}
	return []Status{{Account: c.Account, State: state}}
}

// Send posts a text message.
func (c *CloudAPI) Send(ctx context.Context, _, chatJID, text string) (string, error) {
	body, _ := json.Marshal(map[string]any{"messaging_product": "whatsapp", "to": NumberOfJID(chatJID), "type": "text", "text": map[string]string{"body": text}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/messages", c.GraphURL, c.PhoneID), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		Messages []struct{ ID string }     `json:"messages"`
		Error    *struct{ Message string } `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode >= 300 || len(out.Messages) == 0 {
		msg := res.Status
		if out.Error != nil {
			msg = out.Error.Message
		}
		return "", fmt.Errorf("cloud api send: %s", msg)
	}
	return out.Messages[0].ID, nil
}

// VerifySignature checks X-Hub-Signature-256 against the app secret.
func (c *CloudAPI) VerifySignature(body []byte, header string) bool {
	if c.AppSecret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.AppSecret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(header))
}

// ParseWebhook converts a webhook payload into messages for the business number.
func (c *CloudAPI) ParseWebhook(body []byte) ([]Message, error) {
	var p struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Contacts []struct {
						WaID    string                `json:"wa_id"`
						Profile struct{ Name string } `json:"profile"`
					} `json:"contacts"`
					Messages []struct {
						From      string                `json:"from"`
						ID        string                `json:"id"`
						Timestamp string                `json:"timestamp"`
						Type      string                `json:"type"`
						Text      struct{ Body string } `json:"text"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	var out []Message
	for _, e := range p.Entry {
		for _, ch := range e.Changes {
			names := map[string]string{}
			for _, ct := range ch.Value.Contacts {
				names[ct.WaID] = ct.Profile.Name
			}
			for _, m := range ch.Value.Messages {
				if m.Type != "text" || m.Text.Body == "" {
					continue
				}
				ts, _ := strconv.ParseInt(m.Timestamp, 10, 64)
				out = append(out, Message{ID: m.ID, Account: c.Account, ChatJID: UserJID(m.From), FromNumber: Digits(m.From), FromName: names[m.From], Text: m.Text.Body, Time: time.Unix(ts, 0)})
			}
		}
	}
	return out, nil
}

// Webhook serves the Meta verification handshake (GET) and message delivery (POST); handle is called per message.
func (c *CloudAPI) Webhook(handle func(context.Context, Message) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			q := r.URL.Query()
			if q.Get("hub.mode") == "subscribe" && c.VerifyToken != "" && q.Get("hub.verify_token") == c.VerifyToken {
				_, _ = io.WriteString(w, q.Get("hub.challenge"))
				return
			}
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil || !c.VerifySignature(body, r.Header.Get("X-Hub-Signature-256")) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		msgs, err := c.ParseWebhook(body)
		if err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		for _, m := range msgs {
			if err := handle(r.Context(), m); err != nil {
				http.Error(w, "ingest failed", http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}
