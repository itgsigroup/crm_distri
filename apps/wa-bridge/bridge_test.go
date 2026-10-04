package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(t *testing.T, api string) config {
	return config{Secret: "s3cret", APIURL: api, DataDir: t.TempDir(), MaxPerHour: 20, MinDelay: 0, MaxDelay: 0}
}

func TestSignedRejectsBadSignature(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1")
	h := newHandler(cfg, newManager(cfg, nil, newForwarder(cfg)), newForwarder(cfg))
	body := []byte(`{"chat_id":"6281@s.whatsapp.net","text":"hi","action_id":"a1"}`)

	req := httptest.NewRequest(http.MethodPost, "/sessions/x/send", bytes.NewReader(body))
	req.Header.Set("X-ARC-Signature", "deadbeef")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: want 401, got %d", rec.Code)
	}

	// Valid signature but unknown session → 404 (never reaches WhatsApp).
	req = httptest.NewRequest(http.MethodPost, "/sessions/x/send", bytes.NewReader(body))
	req.Header.Set("X-ARC-Signature", sign(cfg.Secret, body))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown session: want 404, got %d", rec.Code)
	}
}

func TestSendRequiresApprovedAction(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/bridge/actions/")
		if !verify("s3cret", []byte(id), r.Header.Get("X-ARC-Signature")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "approved": id == "approved-1"})
	}))
	defer api.Close()
	cfg := testConfig(t, api.URL)
	m := newManager(cfg, nil, newForwarder(cfg))
	m.sessions["s1"] = &session{meta: sessionMeta{ID: "s1"}}

	for _, id := range []string{"", "proposed-1"} {
		if _, err := m.send(context.Background(), "s1", "6281@s.whatsapp.net", "halo", id); err != errNotApproved {
			t.Fatalf("action %q: want errNotApproved, got %v", id, err)
		}
	}
	ok, err := m.fwd.actionApproved(context.Background(), "approved-1")
	if err != nil || !ok {
		t.Fatalf("approved action should pass the check: ok=%v err=%v", ok, err)
	}
}

func TestRateLimit(t *testing.T) {
	s := &session{}
	for i := 0; i < 20; i++ {
		if !s.allow(20) {
			t.Fatalf("send %d should be allowed", i+1)
		}
	}
	if s.allow(20) {
		t.Fatal("21st send within an hour must be rejected")
	}
	s.sent[0] = time.Now().Add(-61 * time.Minute)
	if !s.allow(20) {
		t.Fatal("slot should free up after an hour")
	}
}

func TestQueueWhenAPIDown(t *testing.T) {
	var up atomic.Bool
	var got atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		if !verify("s3cret", buf.Bytes(), r.Header.Get("X-ARC-Signature")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got.Add(1)
	}))
	defer api.Close()
	f := newForwarder(testConfig(t, api.URL))
	ctx := context.Background()
	f.send(ctx, map[string]any{"event": waEvent{Wamid: "w1"}})
	f.send(ctx, map[string]any{"event": waEvent{Wamid: "w2"}})
	if n := f.queueLen(); n != 2 {
		t.Fatalf("want 2 queued, got %d", n)
	}
	up.Store(true)
	f.flush(ctx)
	if n := f.queueLen(); n != 0 {
		t.Fatalf("queue should be empty after flush, got %d", n)
	}
	if got.Load() != 2 {
		t.Fatalf("API should receive 2 payloads, got %d", got.Load())
	}
}

func TestMessageTextSkipsEmpty(t *testing.T) {
	if txt, media, _ := messageText(nil); txt != "" || media != nil {
		t.Fatal("nil message should yield nothing")
	}
}
