package wa

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
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Baileys is the WhatsApp transport through apps/wa-bridge (ADR 0017): a Node sidecar that links many numbers as
// companion devices with Baileys and applies the anti-ban guard. The worker owns this transport: it listens for the
// bridge's events (POST /webhooks/wa) and answers its approval checks (GET /bridge/actions/{outbox id}); sends go
// to the bridge with the outbox id, which the bridge confirms here before it sends anything.
type Baileys struct {
	BridgeURL   string // http://127.0.0.1:8111
	Secret      string // BRIDGE_SECRET (HMAC-SHA256, both directions)
	Listen      string // 127.0.0.1:8112 — where the bridge posts events
	HistoryDays int
	// Approved reports whether an outbox row may be sent (a recorded human decision); required.
	Approved func(ctx context.Context, outboxID string) (bool, error)
	// Label names a number for its linked-device entry (sales name or team label).
	Label func(ctx context.Context, account string) string
	// Accounts lists the session ids of the registered numbers; those the bridge does not hold are announced
	// "unpaired" on start (no number looks connected when it is not).
	Accounts func(ctx context.Context) []string
	// Session resolves a number to its bridge session id (numbers linked from Chat get a session id first).
	Session func(ctx context.Context, account string) string
	Log     *slog.Logger

	events chan Event
	client *http.Client
	once   sync.Once
	srv    *http.Server
	addr   string
}

// Addr is the address the transport listens on for the bridge (after Start).
func (b *Baileys) Addr() string { return b.addr }

func (b *Baileys) init() {
	b.once.Do(func() {
		b.events = make(chan Event, 256)
		b.client = &http.Client{Timeout: 45 * time.Second}
		if b.Log == nil {
			b.Log = slog.Default()
		}
	})
}

func (b *Baileys) Name() string { return "baileys" }

func (b *Baileys) Events() <-chan Event { b.init(); return b.events }

// sign is the shared-secret signature of the bridge contract (hex HMAC-SHA256 of the raw body).
func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func (b *Baileys) verify(body []byte, sig string) bool {
	sig = strings.TrimPrefix(sig, "sha256=")
	return sig != "" && hmac.Equal([]byte(sign(b.Secret, body)), []byte(sig))
}

// bridgeEvent mirrors apps/wa-bridge/src/events.ts (WaEvent).
type bridgeEvent struct {
	WAMID      string `json:"wamid"`
	Session    string `json:"session"`
	From       string `json:"from"`
	To         string `json:"to"`
	ChatID     string `json:"chat_id"`
	IsGroup    bool   `json:"is_group"`
	SenderName string `json:"sender_name"`
	Text       string `json:"text"`
	Timestamp  string `json:"timestamp"`
	FromMe     bool   `json:"from_me"`
	IsHistory  bool   `json:"is_history"`
	ChatName   string `json:"chat_name"`
	Account    string `json:"account"` // the linked phone's own number
	EditOf     string `json:"edit_of"`
	Quoted     string `json:"quoted"`
	RevokeOf   string `json:"revoke_of"`
	GroupMeta  *struct {
		Name    string `json:"name"`
		Members []struct {
			Phone string `json:"phone"`
		} `json:"members"`
	} `json:"group_meta"`
	MediaMeta *Media `json:"media_meta"`
}

type bridgeStatus struct {
	Session string `json:"session"`
	Status  string `json:"status"` // pairing | connected | disconnected
	Phone   string `json:"phone"`
	QR      string `json:"qr"`
	Reason  string `json:"reason"`
}

var mediaLabel = map[string]string{"image": "[gambar]", "video": "[video]", "gif": "[GIF]", "audio": "[pesan suara]", "document": "[dokumen]", "sticker": "[stiker]", "location": "[lokasi]", "contact": "[kontak]", "poll": "[polling]"}

// MediaLabel is the short label a media message's text starts with ("[gambar] caption").
func MediaLabel(kind, fileName string) string {
	label := mediaLabel[kind]
	if label == "" {
		label = "[lampiran]"
	}
	if fileName != "" {
		label = strings.TrimSuffix(label, "]") + " " + fileName + "]"
	}
	return label
}

// message converts a bridge event; media gets a short label before its caption so the conversation (and the
// analysis, which reads text only) stays readable. The media's download reference is kept, never its content.
func (e bridgeEvent) message() Message {
	text := strings.TrimSpace(e.Text)
	if e.MediaMeta != nil && e.EditOf == "" {
		label := MediaLabel(e.MediaMeta.Kind, e.MediaMeta.FileName)
		if text == "" {
			text = label
		} else {
			text = label + " " + text
		}
	}
	t, err := time.Parse(time.RFC3339Nano, e.Timestamp)
	if err != nil {
		t = time.Now()
	}
	acc := Digits(e.Account)
	if acc == "" && Digits(e.Session) == e.Session {
		acc = e.Session // sessions named after their number; never the digits inside a "link-…" id
	}
	m := Message{ID: e.WAMID, Account: acc, ChatJID: e.ChatID, FromNumber: Digits(e.From), FromName: e.SenderName,
		IsGroup: e.IsGroup, FromMe: e.FromMe, Text: text, Time: t, ChatName: strings.TrimSpace(e.ChatName), History: e.IsHistory,
		Media: e.MediaMeta, EditOf: e.EditOf, QuotedID: e.Quoted, RevokeOf: e.RevokeOf}
	if e.GroupMeta != nil {
		m.GroupName = e.GroupMeta.Name
		for _, x := range e.GroupMeta.Members {
			if d := Digits(x.Phone); d != "" {
				m.GroupMembers = append(m.GroupMembers, d)
			}
		}
	}
	return m
}

func (s bridgeStatus) status() Status {
	state := s.Status
	switch {
	case s.Status == "disconnected" && (s.Reason == "logged_out" || s.Reason == "unlinked"):
		state = "logged_out"
	case s.Status == "disconnected" && s.Reason == "forbidden":
		state = "logged_out" // WhatsApp refused the device (403): a person must re-link after checking the number
	}
	out := Status{Session: s.Session, Account: Digits(s.Phone), State: state, QR: s.QR}
	if s.Phone != "" {
		out.JID = Digits(s.Phone) + "@s.whatsapp.net"
	}
	return out
}

// Start listens for the bridge and announces the state of every session it holds.
func (b *Baileys) Start(ctx context.Context) error {
	b.init()
	if b.Approved == nil {
		return errors.New("baileys: Approved is required (every send needs a recorded decision)")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/wa", b.webhook)
	mux.HandleFunc("GET /bridge/actions/{id}", b.action)
	ln, err := net.Listen("tcp", b.Listen)
	if err != nil {
		return fmt.Errorf("baileys listener %s: %w", b.Listen, err)
	}
	b.addr = ln.Addr().String()
	b.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = b.srv.Serve(ln) }()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.srv.Shutdown(sh)
	}()
	go func() {
		live := map[string]bool{}
		for _, s := range b.Status(ctx) {
			live[s.Session] = true
			b.events <- Event{Status: &s}
		}
		if b.Accounts != nil {
			for _, sid := range b.Accounts(ctx) {
				if !live[sid] {
					b.events <- Event{Status: &Status{Session: sid, State: "unpaired"}}
				}
			}
		}
	}()
	b.Log.Info("baileys transport listening", "listen", b.Listen, "bridge", b.BridgeURL)
	return nil
}

func (b *Baileys) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil || !b.verify(body, r.Header.Get("X-ARC-Signature")) {
		http.Error(w, `{"error":"signature tidak valid"}`, http.StatusUnauthorized)
		return
	}
	var in struct {
		Event  *bridgeEvent  `json:"event"`
		Events []bridgeEvent `json:"events"`
		Status *bridgeStatus `json:"status"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, `{"error":"json"}`, http.StatusBadRequest)
		return
	}
	if in.Event != nil {
		m := in.Event.message()
		b.events <- Event{Message: &m}
	}
	for _, e := range in.Events {
		m := e.message()
		b.events <- Event{Message: &m}
	}
	if in.Status != nil {
		s := in.Status.status()
		b.events <- Event{Status: &s}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// action answers the bridge's pre-send check: only an outbox row of an approved proposal may be sent.
func (b *Baileys) action(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !b.verify([]byte(id), r.Header.Get("X-ARC-Signature")) {
		http.Error(w, `{"error":"signature tidak valid"}`, http.StatusUnauthorized)
		return
	}
	ok, err := b.Approved(r.Context(), id)
	if err != nil {
		b.Log.Warn("approval check failed", "outbox", id, "err", err)
	}
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"approved":false}`))
		return
	}
	_, _ = w.Write([]byte(`{"approved":true}`))
}

func (b *Baileys) call(ctx context.Context, method, path string, in any, out any) (int, http.Header, error) {
	b.init()
	var body []byte
	if in != nil {
		body, _ = json.Marshal(in)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(b.BridgeURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ARC-Signature", sign(b.Secret, body))
	res, err := b.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("wa-bridge tidak terjangkau: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(raw, &e)
		return res.StatusCode, res.Header, &bridgeError{Status: res.StatusCode, Code: e.Code, Msg: e.Error}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return res.StatusCode, res.Header, err
		}
	}
	return res.StatusCode, res.Header, nil
}

type bridgeError struct {
	Status int
	Code   string
	Msg    string
}

func (e *bridgeError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "wa-bridge HTTP " + strconv.Itoa(e.Status)
}

// session is the bridge session of a number.
func (b *Baileys) session(ctx context.Context, account string) string {
	if b.Session != nil {
		if s := b.Session(ctx, Digits(account)); s != "" {
			return s
		}
	}
	return Digits(account)
}

func (b *Baileys) open(ctx context.Context, session, label, phone string, code bool) (bridgeStatus, error) {
	days := b.HistoryDays
	if days <= 0 {
		days = 30
	}
	body := map[string]any{"id": session, "label": label, "history_days": days}
	if code {
		body["phone_code"], body["phone"] = true, Digits(phone)
	}
	var snap bridgeStatus
	_, _, err := b.call(ctx, http.MethodPost, "/sessions", body, &snap)
	return snap, err
}

func (b *Baileys) label(ctx context.Context, account string) string {
	if b.Label != nil {
		return b.Label(ctx, Digits(account))
	}
	return Digits(account)
}

// Pair re-links a registered number: the bridge returns the first QR (or "connected" when it is already linked).
func (b *Baileys) Pair(ctx context.Context, account string) (string, error) {
	snap, err := b.open(ctx, b.session(ctx, account), b.label(ctx, account), "", false)
	return snap.QR, err
}

// PairCode re-links a registered number with a code typed on the phone; the bridge returns "code:XXXX-XXXX".
func (b *Baileys) PairCode(ctx context.Context, account string) (string, error) {
	snap, err := b.open(ctx, b.session(ctx, account), b.label(ctx, account), account, true)
	if err != nil || snap.Status == "connected" {
		return "", err
	}
	if !strings.HasPrefix(snap.QR, PairCodePrefix) {
		return "", errors.New("wa-bridge tidak memberi kode tautan — coba lagi atau pakai QR")
	}
	return snap.QR, nil
}

// Link starts linking a new device under a fresh session id; its number arrives with the "connected" status.
func (b *Baileys) Link(ctx context.Context, session, phone string, code bool) (string, error) {
	snap, err := b.open(ctx, session, "GSI Orbit", phone, code)
	if err != nil {
		return "", err
	}
	if code && !strings.HasPrefix(snap.QR, PairCodePrefix) {
		return "", errors.New("wa-bridge tidak memberi kode tautan — coba lagi atau pakai QR")
	}
	return snap.QR, nil
}

// Status reads every session from the bridge's /health.
func (b *Baileys) Status(ctx context.Context) []Status {
	var h struct {
		Sessions map[string]string `json:"sessions"`
	}
	if _, _, err := b.call(ctx, http.MethodGet, "/health", nil, &h); err != nil {
		b.Log.Warn("wa-bridge health", "err", err)
		return nil
	}
	out := make([]Status, 0, len(h.Sessions))
	for id, st := range h.Sessions {
		out = append(out, Status{Session: id, State: st})
	}
	return out
}

// Health is the bridge's /health: sessions, forward queue and the anti-ban counters per number.
func (b *Baileys) Health(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	_, _, err := b.call(ctx, http.MethodGet, "/health", nil, &raw)
	return raw, err
}

// Send asks the bridge to send; the outbox id travels in ctx (WithAction) and is confirmed by the bridge.
func (b *Baileys) Send(ctx context.Context, account, chatJID, text string) (string, error) {
	action := ActionFrom(ctx)
	if action == "" {
		return "", &SendRefused{Code: "not_approved", Reason: "kirim tanpa outbox yang disetujui ditolak"}
	}
	var out struct {
		WAMID string `json:"wamid"`
	}
	in := map[string]any{"chat_id": chatJID, "text": text, "action_id": action}
	if q := QuoteFrom(ctx); q != nil {
		in["quote"] = q
	}
	_, hdr, err := b.call(ctx, http.MethodPost, "/sessions/"+url.PathEscape(b.session(ctx, account))+"/send", in, &out)
	var be *bridgeError
	if errors.As(err, &be) && (be.Status == http.StatusForbidden || be.Status == http.StatusConflict || be.Status == http.StatusTooManyRequests) {
		r := &SendRefused{Code: be.Code, Reason: be.Msg}
		if s, err := strconv.Atoi(hdr.Get("Retry-After")); err == nil && s > 0 {
			r.RetryAfter = time.Duration(s) * time.Second
		}
		if be.Code == "not_linked" {
			r.RetryAfter = 10 * time.Minute
		}
		return "", r
	}
	if err != nil {
		return "", err
	}
	return out.WAMID, nil
}

// SendContent asks the bridge to send a file, voice note or poll (same approval and anti-ban path as Send).
func (b *Baileys) SendContent(ctx context.Context, account, chatJID, text string, c Content) (string, string, error) {
	action := ActionFrom(ctx)
	if action == "" {
		return "", "", &SendRefused{Code: "not_approved", Reason: "kirim tanpa outbox yang disetujui ditolak"}
	}
	in := map[string]any{"chat_id": chatJID, "text": text, "action_id": action, "content": c}
	if q := QuoteFrom(ctx); q != nil {
		in["quote"] = q
	}
	var out struct {
		WAMID string `json:"wamid"`
		Ref   string `json:"ref"`
	}
	_, hdr, err := b.call(ctx, http.MethodPost, "/sessions/"+url.PathEscape(b.session(ctx, account))+"/send", in, &out)
	var be *bridgeError
	if errors.As(err, &be) && be.Status == http.StatusBadRequest && be.Code == "bad_content" {
		return "", "", &SendRefused{Code: be.Code, Reason: be.Msg}
	}
	if errors.As(err, &be) && (be.Status == http.StatusForbidden || be.Status == http.StatusConflict || be.Status == http.StatusTooManyRequests) {
		r := &SendRefused{Code: be.Code, Reason: be.Msg}
		if s, err := strconv.Atoi(hdr.Get("Retry-After")); err == nil && s > 0 {
			r.RetryAfter = time.Duration(s) * time.Second
		}
		if be.Code == "not_linked" {
			r.RetryAfter = 10 * time.Minute
		}
		return "", "", r
	}
	if err != nil {
		return "", "", err
	}
	return out.WAMID, out.Ref, nil
}

// Edit asks the bridge to edit a text this number sent; the outbox id of the approved edit travels in ctx
// (WithAction) and is confirmed by the bridge like a send.
func (b *Baileys) Edit(ctx context.Context, account, chatJID, waMsgID, text string) error {
	action := ActionFrom(ctx)
	if action == "" {
		return &SendRefused{Code: "not_approved", Reason: "edit tanpa outbox yang disetujui ditolak"}
	}
	_, _, err := b.call(ctx, http.MethodPost, "/sessions/"+url.PathEscape(b.session(ctx, account))+"/edit",
		map[string]string{"chat_id": chatJID, "wamid": waMsgID, "text": text, "action_id": action}, nil)
	var be *bridgeError
	if errors.As(err, &be) && (be.Status == http.StatusForbidden || be.Status == http.StatusConflict) {
		r := &SendRefused{Code: be.Code, Reason: be.Msg}
		if be.Code == "not_linked" {
			r.RetryAfter = time.Minute
		}
		return r
	}
	return err
}

// Delete asks the bridge to delete a message this number sent, for everyone (approved action in ctx, like Edit).
func (b *Baileys) Delete(ctx context.Context, account, chatJID, waMsgID string) error {
	action := ActionFrom(ctx)
	if action == "" {
		return &SendRefused{Code: "not_approved", Reason: "hapus tanpa outbox yang disetujui ditolak"}
	}
	_, _, err := b.call(ctx, http.MethodPost, "/sessions/"+url.PathEscape(b.session(ctx, account))+"/delete",
		map[string]string{"chat_id": chatJID, "wamid": waMsgID, "action_id": action}, nil)
	var be *bridgeError
	if errors.As(err, &be) && (be.Status == http.StatusForbidden || be.Status == http.StatusConflict) {
		r := &SendRefused{Code: be.Code, Reason: be.Msg}
		if be.Code == "not_linked" {
			r.RetryAfter = time.Minute
		}
		return r
	}
	return err
}

// mediaClient fetches files: no overall timeout (a video may take a while), the request context bounds it.
var mediaClient = &http.Client{}

// Media streams a message's file from WhatsApp through the bridge (ADR 0029): nothing is stored on the way.
func (b *Baileys) Media(ctx context.Context, account string, m Media) (io.ReadCloser, error) {
	body, _ := json.Marshal(map[string]string{"ref": m.Ref, "mime": m.Mime})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.BridgeURL, "/")+"/sessions/"+url.PathEscape(b.session(ctx, account))+"/media", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ARC-Signature", sign(b.Secret, body))
	res, err := mediaClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: wa-bridge tidak terjangkau", ErrMediaUnavailable)
	}
	switch res.StatusCode {
	case http.StatusOK:
		return res.Body, nil
	case http.StatusGone:
		res.Body.Close()
		return nil, ErrMediaGone
	case http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests:
		res.Body.Close()
		return nil, ErrMediaUnavailable
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	return nil, &bridgeError{Status: res.StatusCode, Msg: e.Error}
}

// FetchHistory asks the bridge for an on-demand history sync of one chat: the linked phone sends up to `count`
// messages before the anchor, which come back as history events (ADR 0028). A number not connected or throttled by
// the bridge is ErrHistoryUnavailable (asked again on the next run).
func (b *Baileys) FetchHistory(ctx context.Context, account, chatJID string, before HistoryAnchor, count int) error {
	in := map[string]any{"chat_id": chatJID, "id": before.ID, "from_me": before.FromMe, "ts": before.At.UnixMilli(), "count": count}
	_, _, err := b.call(ctx, http.MethodPost, "/sessions/"+url.PathEscape(b.session(ctx, account))+"/history", in, nil)
	var be *bridgeError
	if errors.As(err, &be) && (be.Status == http.StatusNotFound || be.Status == http.StatusConflict || be.Status == http.StatusTooManyRequests) {
		return fmt.Errorf("%w: %s", ErrHistoryUnavailable, be.Error())
	}
	return err
}

// Profile reads a contact's public profile through the bridge (cached and rate-limited there).
func (b *Baileys) Profile(ctx context.Context, account, jid string) (Profile, error) {
	var p struct {
		Name     string `json:"name"`
		Business string `json:"business"`
	}
	if _, _, err := b.call(ctx, http.MethodGet, "/sessions/"+url.PathEscape(b.session(ctx, account))+"/contacts/"+url.PathEscape(jid), nil, &p); err != nil {
		return Profile{}, err
	}
	name := p.Business
	if name == "" {
		name = p.Name
	}
	return Profile{Name: name, Business: p.Business != ""}, nil
}

// Unpair logs the linked device out and forgets it.
func (b *Baileys) Unpair(ctx context.Context, account string) error {
	_, _, err := b.call(ctx, http.MethodDelete, "/sessions/"+url.PathEscape(b.session(ctx, account)), nil, nil)
	var be *bridgeError
	if errors.As(err, &be) && be.Status == http.StatusNotFound {
		return nil
	}
	return err
}
