// Package whatsapp defines the standard WaEvent and the WhatsAppTransport
// interface (ADR 0002) with three implementations: the wa-bridge sidecar
// (linked device via QR), the official Cloud API, and a Fake for fixtures/tests.
// No transport ever sends on its own: Send requires the id of an approved Action.
package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GroupMember is a participant of a group chat.
type GroupMember struct {
	JID   string `json:"jid"`
	Phone string `json:"phone"`
	Name  string `json:"name"`
}

// GroupMeta describes a group.
type GroupMeta struct {
	Name    string        `json:"name"`
	Members []GroupMember `json:"members"`
}

// MediaMeta describes an attachment (content is not stored unless allowed by policy).
type MediaMeta struct {
	Kind     string `json:"kind"`
	Mime     string `json:"mime"`
	FileName string `json:"file_name"`
	Size     int64  `json:"size"`
}

// WaEvent is the transport-independent message event.
type WaEvent struct {
	Wamid      string     `json:"wamid"`
	Session    string     `json:"session"` // ARC wa_sessions.id that received/sent it
	From       string     `json:"from"`    // sender phone (E.164 digits)
	To         string     `json:"to"`
	ChatID     string     `json:"chat_id"` // jid of the chat (person or group)
	IsGroup    bool       `json:"is_group"`
	GroupMeta  *GroupMeta `json:"group_meta,omitempty"`
	SenderName string     `json:"sender_name"`
	Text       string     `json:"text"`
	Media      *MediaMeta `json:"media_meta,omitempty"`
	Timestamp  time.Time  `json:"timestamp"`
	Quoted     string     `json:"quoted,omitempty"`
	FromMe     bool       `json:"from_me"`
	IsHistory  bool       `json:"is_history"`
	Transport  string     `json:"transport"` // bridge | cloud | fake | export
}

// SessionStatus is reported by the bridge.
type SessionStatus struct {
	Session string `json:"session"`
	Status  string `json:"status"` // pairing | connected | disconnected
	Phone   string `json:"phone"`
	QR      string `json:"qr,omitempty"`
}

// Profile is a contact profile (WhatsApp Business name/about).
type Profile struct {
	Name     string `json:"name"`
	About    string `json:"about"`
	HasPhoto bool   `json:"has_photo"`
	Business string `json:"business"`
}

// Transport sends approved messages and fetches profiles.
type Transport interface {
	Name() string
	Send(ctx context.Context, session, chatJID, text, actionID string) (wamid string, err error)
	Profile(ctx context.Context, session, jid string) (Profile, error)
}

// BridgeError is a refusal explained by wa-bridge (e.g. anti-ban limits, quiet hours, opt-out).
type BridgeError struct {
	Status  int
	Code    string
	Message string
}

func (e *BridgeError) Error() string { return "WhatsApp: " + e.Message }

// ErrNotApproved is returned when a send has no approved action.
var ErrNotApproved = errors.New("whatsapp: kirim ditolak — action belum disetujui manusia")

// Sign returns the hex HMAC-SHA256 of body with secret.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a hex signature (optionally prefixed with "sha256=").
func Verify(secret string, body []byte, sig string) bool {
	sig = strings.TrimPrefix(sig, "sha256=")
	want := Sign(secret, body)
	return hmac.Equal([]byte(want), []byte(sig))
}

// ---------- Transport A: wa-bridge sidecar ----------

// BridgeTransport talks to apps/wa-bridge over HTTP with the shared secret.
type BridgeTransport struct {
	URL    string
	Secret string
	HTTP   *http.Client
}

// NewBridge returns a bridge client.
func NewBridge(url, secret string) *BridgeTransport {
	// Generous timeout: the bridge paces sends like a person (typing + random gap, serialised per number).
	return &BridgeTransport{URL: strings.TrimRight(url, "/"), Secret: secret, HTTP: &http.Client{Timeout: 90 * time.Second}}
}

func (b *BridgeTransport) Name() string { return "bridge" }

func (b *BridgeTransport) do(ctx context.Context, method, path string, body any, out any) error {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, b.URL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ARC-Signature", Sign(b.Secret, raw))
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		// The bridge explains refusals (anti-ban guard, approval, linking) in Indonesian.
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return &BridgeError{Status: resp.StatusCode, Code: e.Code, Message: e.Error}
		}
		return fmt.Errorf("bridge %s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Send asks the bridge to deliver; the bridge re-checks the action with the API.
func (b *BridgeTransport) Send(ctx context.Context, session, chatJID, text, actionID string) (string, error) {
	if actionID == "" {
		return "", ErrNotApproved
	}
	var out struct {
		Wamid string `json:"wamid"`
	}
	err := b.do(ctx, http.MethodPost, "/sessions/"+session+"/send", map[string]string{"chat_id": chatJID, "text": text, "action_id": actionID}, &out)
	return out.Wamid, err
}

// Profile fetches a contact profile.
func (b *BridgeTransport) Profile(ctx context.Context, session, jid string) (Profile, error) {
	var p Profile
	err := b.do(ctx, http.MethodGet, "/sessions/"+session+"/contacts/"+jid, nil, &p)
	return p, err
}

// CreateSession starts pairing a number and returns the QR.
func (b *BridgeTransport) CreateSession(ctx context.Context, id, label string, historyDays int) (SessionStatus, error) {
	var st SessionStatus
	err := b.do(ctx, http.MethodPost, "/sessions", map[string]any{"id": id, "label": label, "history_days": historyDays}, &st)
	return st, err
}

// QR returns the current QR string of a pairing session.
func (b *BridgeTransport) QR(ctx context.Context, id string) (SessionStatus, error) {
	var st SessionStatus
	err := b.do(ctx, http.MethodGet, "/sessions/"+id+"/qr", nil, &st)
	return st, err
}

// Groups lists groups of a session.
func (b *BridgeTransport) Groups(ctx context.Context, id string) ([]struct {
	JID  string    `json:"jid"`
	Meta GroupMeta `json:"meta"`
}, error) {
	var out []struct {
		JID  string    `json:"jid"`
		Meta GroupMeta `json:"meta"`
	}
	err := b.do(ctx, http.MethodGet, "/sessions/"+id+"/groups", nil, &out)
	return out, err
}

// Health pings the bridge.
func (b *BridgeTransport) Health(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := b.do(ctx, http.MethodGet, "/health", nil, &out)
	return out, err
}

// ---------- Transport B: WhatsApp Cloud API (official) ----------

// CloudTransport sends through the Meta Graph API (1:1 only, no groups).
type CloudTransport struct {
	Token, PhoneID, AppSecret string
	HTTP                      *http.Client
	BaseURL                   string
}

// NewCloud returns a Cloud API client.
func NewCloud(token, phoneID, appSecret string) *CloudTransport {
	return &CloudTransport{Token: token, PhoneID: phoneID, AppSecret: appSecret, HTTP: &http.Client{Timeout: 20 * time.Second}, BaseURL: "https://graph.facebook.com/v20.0"}
}

func (c *CloudTransport) Name() string { return "cloud" }

// Send delivers a text message. Outside the 24h window a template is required (not automated).
func (c *CloudTransport) Send(ctx context.Context, _ string, chatJID, text, actionID string) (string, error) {
	if actionID == "" {
		return "", ErrNotApproved
	}
	if strings.HasSuffix(chatJID, "@g.us") {
		return "", errors.New("cloud api: grup tidak didukung")
	}
	to := strings.Split(chatJID, "@")[0]
	body, _ := json.Marshal(map[string]any{"messaging_product": "whatsapp", "to": to, "type": "text", "text": map[string]string{"body": text}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/messages", c.BaseURL, c.PhoneID), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 || out.Error != nil {
		msg := resp.Status
		if out.Error != nil {
			msg = out.Error.Message
		}
		return "", fmt.Errorf("cloud api: %s", msg)
	}
	if len(out.Messages) == 0 {
		return "", errors.New("cloud api: no message id")
	}
	return out.Messages[0].ID, nil
}

// Profile returns the business profile name when available.
func (c *CloudTransport) Profile(_ context.Context, _, _ string) (Profile, error) {
	return Profile{}, errors.New("cloud api: profil kontak tidak tersedia untuk nomor pribadi")
}

// VerifySignature checks X-Hub-Signature-256.
func (c *CloudTransport) VerifySignature(body []byte, header string) bool {
	return c.AppSecret != "" && Verify(c.AppSecret, body, header)
}

// ParseCloudWebhook converts a Cloud API webhook body into WaEvents.
func ParseCloudWebhook(body []byte, session string) ([]WaEvent, error) {
	var p struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Metadata struct {
						DisplayPhone string `json:"display_phone_number"`
					} `json:"metadata"`
					Contacts []struct {
						Profile struct {
							Name string `json:"name"`
						} `json:"profile"`
						WaID string `json:"wa_id"`
					} `json:"contacts"`
					Messages []struct {
						ID        string `json:"id"`
						From      string `json:"from"`
						Timestamp string `json:"timestamp"`
						Type      string `json:"type"`
						Text      struct {
							Body string `json:"body"`
						} `json:"text"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	var out []WaEvent
	for _, e := range p.Entry {
		for _, ch := range e.Changes {
			names := map[string]string{}
			for _, c := range ch.Value.Contacts {
				names[c.WaID] = c.Profile.Name
			}
			for _, m := range ch.Value.Messages {
				var sec int64
				fmt.Sscanf(m.Timestamp, "%d", &sec)
				ev := WaEvent{Wamid: m.ID, Session: session, From: m.From, To: digits(ch.Value.Metadata.DisplayPhone), ChatID: m.From + "@s.whatsapp.net",
					SenderName: names[m.From], Text: m.Text.Body, Timestamp: time.Unix(sec, 0), Transport: "cloud"}
				if m.Type != "text" {
					ev.Media = &MediaMeta{Kind: m.Type}
				}
				out = append(out, ev)
			}
		}
	}
	return out, nil
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------- Fake transport ----------

// Sent records a fake delivery.
type Sent struct {
	Session, ChatJID, Text, ActionID, Wamid string
	At                                      time.Time
}

// FakeTransport records sends and serves canned profiles (tests and mock mode).
type FakeTransport struct {
	mu       sync.Mutex
	Sent     []Sent
	Profiles map[string]Profile
}

// NewFake returns an empty fake.
func NewFake() *FakeTransport { return &FakeTransport{Profiles: map[string]Profile{}} }

func (f *FakeTransport) Name() string { return "fake" }

// Send records the message.
func (f *FakeTransport) Send(_ context.Context, session, chatJID, text, actionID string) (string, error) {
	if actionID == "" {
		return "", ErrNotApproved
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("fake-%d", len(f.Sent)+1)
	f.Sent = append(f.Sent, Sent{session, chatJID, text, actionID, id, time.Now()})
	return id, nil
}

// Profile returns a canned profile.
func (f *FakeTransport) Profile(_ context.Context, _, jid string) (Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.Profiles[jid]; ok {
		return p, nil
	}
	return Profile{}, errors.New("profil tidak ditemukan")
}

// SentCount returns how many messages were sent.
func (f *FakeTransport) SentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Sent)
}
