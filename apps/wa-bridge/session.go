package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var (
	errNoSession   = errors.New("sesi tidak ditemukan")
	errNotLinked   = errors.New("sesi belum terhubung")
	errNotApproved = errors.New("kirim ditolak — action belum disetujui manusia")
	errRateLimited = errors.New("batas kirim per jam tercapai")
)

// sessionMeta is persisted in data/sessions.json to map ARC session ids to devices.
type sessionMeta struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	HistoryDays int    `json:"history_days"`
	JID         string `json:"jid,omitempty"`
}

type session struct {
	meta   sessionMeta
	client *whatsmeow.Client
	mu     sync.Mutex
	status string // pairing | connected | disconnected
	qr     string
	phone  string
	sent   []time.Time
	groups map[string]cachedGroup
	sendMu sync.Mutex // serialises sends so the random delay applies between messages
}

type cachedGroup struct {
	meta *groupMeta
	at   time.Time
}

func (s *session) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"session": s.meta.ID, "status": s.status, "phone": s.phone, "qr": s.qr}
}

type manager struct {
	cfg       config
	container *sqlstore.Container
	fwd       *forwarder
	mu        sync.Mutex
	sessions  map[string]*session
	metaPath  string
}

func newManager(cfg config, c *sqlstore.Container, fwd *forwarder) *manager {
	return &manager{cfg: cfg, container: c, fwd: fwd, sessions: map[string]*session{}, metaPath: filepath.Join(cfg.DataDir, "sessions.json")}
}

func (m *manager) saveMeta() {
	m.mu.Lock()
	metas := make([]sessionMeta, 0, len(m.sessions))
	for _, s := range m.sessions {
		metas = append(metas, s.meta)
	}
	m.mu.Unlock()
	data, _ := json.MarshalIndent(metas, "", "  ")
	tmp := m.metaPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		_ = os.Rename(tmp, m.metaPath)
	}
}

func (m *manager) get(id string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, errNoSession
	}
	return s, nil
}

// restore reconnects every paired session found in sessions.json.
func (m *manager) restore(ctx context.Context) error {
	data, err := os.ReadFile(m.metaPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var metas []sessionMeta
	if err := json.Unmarshal(data, &metas); err != nil {
		return err
	}
	for _, meta := range metas {
		if meta.JID == "" {
			continue // pairing never finished
		}
		jid, err := types.ParseJID(meta.JID)
		if err != nil {
			continue
		}
		dev, err := m.container.GetDevice(ctx, jid)
		if err != nil || dev == nil {
			slog.Warn("device missing, session must be re-paired", "session", meta.ID)
			continue
		}
		s := m.attach(meta, dev)
		if err := s.client.Connect(); err != nil {
			slog.Warn("reconnect failed", "session", meta.ID, "err", err)
		}
	}
	return nil
}

func (m *manager) attach(meta sessionMeta, dev *store.Device) *session {
	s := &session{meta: meta, status: "disconnected"}
	s.client = whatsmeow.NewClient(dev, waLog.Stdout("wa:"+meta.ID, "WARN", false))
	s.client.AddEventHandler(func(evt any) { m.handleEvent(s, evt) })
	if dev.ID != nil {
		s.phone = dev.ID.User
	}
	m.mu.Lock()
	m.sessions[meta.ID] = s
	m.mu.Unlock()
	return s
}

// create starts pairing a new number. It returns once the first QR is available.
func (m *manager) create(ctx context.Context, meta sessionMeta) (map[string]any, error) {
	if existing, err := m.get(meta.ID); err == nil {
		existing.mu.Lock()
		linked := existing.status == "connected"
		existing.mu.Unlock()
		if linked {
			return existing.snapshot(), nil
		}
		existing.client.Disconnect()
	}
	if meta.HistoryDays <= 0 {
		meta.HistoryDays = 90
	}
	dev := m.container.NewDevice()
	s := m.attach(meta, dev)
	m.saveMeta()

	qrCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	qrChan, err := s.client.GetQRChannel(qrCtx)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := s.client.Connect(); err != nil {
		cancel()
		return nil, err
	}
	first := make(chan struct{})
	go func() {
		defer cancel()
		once := sync.Once{}
		for item := range qrChan {
			switch item.Event {
			case "code":
				s.mu.Lock()
				s.status, s.qr = "pairing", item.Code
				s.mu.Unlock()
				m.fwd.send(context.Background(), map[string]any{"status": s.snapshot()})
				once.Do(func() { close(first) })
			case "success":
				// Connected event updates the status.
			default:
				slog.Warn("pairing ended", "session", meta.ID, "event", item.Event, "err", item.Error)
				m.setStatus(s, "disconnected")
				once.Do(func() { close(first) })
			}
		}
	}()
	select {
	case <-first:
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
	}
	return s.snapshot(), nil
}

func (m *manager) setStatus(s *session, status string) {
	s.mu.Lock()
	s.status = status
	if status != "pairing" {
		s.qr = ""
	}
	s.mu.Unlock()
	m.fwd.send(context.Background(), map[string]any{"status": s.snapshot()})
}

func (m *manager) remove(ctx context.Context, id string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	if s.client.IsLoggedIn() {
		_ = s.client.Logout(ctx)
	}
	s.client.Disconnect()
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	m.saveMeta()
	m.setStatus(s, "disconnected")
	return nil
}

func (m *manager) disconnectAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.client.Disconnect()
	}
}

func (m *manager) statuses() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for id, s := range m.sessions {
		s.mu.Lock()
		out[id] = s.status
		s.mu.Unlock()
	}
	return out
}

func (m *manager) handleEvent(s *session, evt any) {
	ctx := context.Background()
	switch e := evt.(type) {
	case *events.PairSuccess:
		s.mu.Lock()
		s.meta.JID = e.ID.String()
		s.phone = e.ID.User
		s.mu.Unlock()
		m.saveMeta()
	case *events.Connected:
		if id := s.client.Store.ID; id != nil {
			s.mu.Lock()
			s.meta.JID, s.phone = id.String(), id.User
			s.mu.Unlock()
			m.saveMeta()
		}
		m.setStatus(s, "connected")
	case *events.Disconnected:
		m.setStatus(s, "disconnected")
	case *events.LoggedOut:
		m.setStatus(s, "disconnected")
	case *events.Message:
		if ev, ok := m.toWaEvent(ctx, s, e, false); ok {
			m.fwd.send(ctx, map[string]any{"event": ev})
		}
	case *events.HistorySync:
		m.forwardHistory(ctx, s, e)
	}
}

// forwardHistory sends history-sync messages within history_days as is_history events, in batches.
func (m *manager) forwardHistory(ctx context.Context, s *session, e *events.HistorySync) {
	if e.Data == nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -s.meta.HistoryDays)
	batch := make([]waEvent, 0, 200)
	flush := func() {
		if len(batch) > 0 {
			m.fwd.send(ctx, map[string]any{"events": batch})
			batch = make([]waEvent, 0, 200)
		}
	}
	for _, conv := range e.Data.GetConversations() {
		chatJID, err := types.ParseJID(conv.GetID())
		if err != nil {
			continue
		}
		for _, hm := range conv.GetMessages() {
			msg, err := s.client.ParseWebMessage(chatJID, hm.GetMessage())
			if err != nil || msg.Info.Timestamp.Before(cutoff) {
				continue
			}
			if ev, ok := m.toWaEvent(ctx, s, msg, true); ok {
				batch = append(batch, ev)
				if len(batch) == cap(batch) {
					flush()
				}
			}
		}
	}
	flush()
}

// allow applies the per-session rate limit (≤ MaxPerHour sends in a sliding hour).
func (s *session) allow(maxPerHour int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-time.Hour)
	kept := s.sent[:0]
	for _, t := range s.sent {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	s.sent = kept
	if len(s.sent) >= maxPerHour {
		return false
	}
	s.sent = append(s.sent, time.Now())
	return true
}

// send delivers text to chatID only after the API confirms the action is approved.
func (m *manager) send(ctx context.Context, id, chatID, text, actionID string) (string, error) {
	s, err := m.get(id)
	if err != nil {
		return "", err
	}
	if actionID == "" {
		return "", errNotApproved
	}
	ok, err := m.fwd.actionApproved(ctx, actionID)
	if err != nil {
		return "", fmt.Errorf("cek action gagal: %w", err)
	}
	if !ok {
		return "", errNotApproved
	}
	if !s.client.IsConnected() || !s.client.IsLoggedIn() {
		return "", errNotLinked
	}
	to, err := types.ParseJID(chatID)
	if err != nil {
		return "", fmt.Errorf("chat_id tidak valid: %w", err)
	}
	if !s.allow(m.cfg.MaxPerHour) {
		return "", errRateLimited
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	delay := m.cfg.MinDelay + time.Duration(rand.Int64N(int64(m.cfg.MaxDelay-m.cfg.MinDelay)+1))
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	resp, err := s.client.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(text)})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}
