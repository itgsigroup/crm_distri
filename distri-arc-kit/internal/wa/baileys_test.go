package wa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const secret = "test-bridge-secret-123"

// fakeBridge answers like apps/wa-bridge: every send is confirmed against the worker first.
func fakeBridge(t *testing.T, worker func() string, refuse map[string][2]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/health" && r.Header.Get("X-ARC-Signature") != sign(secret, body) {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"ok":true,"sessions":{"6281234504471":"connected","6281299990000":"pairing"},"limits":{}}`))
		case r.Method == "POST" && r.URL.Path == "/sessions":
			_, _ = w.Write([]byte(`{"session":"6281299990000","status":"pairing","phone":"","qr":"2@abc"}`))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/send"):
			var in struct {
				ChatID   string `json:"chat_id"`
				ActionID string `json:"action_id"`
			}
			_ = json.Unmarshal(body, &in)
			if rf, ok := refuse[in.ChatID]; ok {
				if rf[0] == "quiet_hours" {
					w.Header().Set("Retry-After", "3600")
					w.WriteHeader(409)
				} else {
					w.WriteHeader(403)
				}
				_, _ = w.Write([]byte(`{"error":"` + rf[1] + `","code":"` + rf[0] + `"}`))
				return
			}
			req, _ := http.NewRequest("GET", "http://"+worker()+"/bridge/actions/"+in.ActionID, nil)
			req.Header.Set("X-ARC-Signature", sign(secret, []byte(in.ActionID)))
			res, err := http.DefaultClient.Do(req)
			if err != nil || res.StatusCode != 200 {
				w.WriteHeader(403)
				_, _ = w.Write([]byte(`{"error":"kirim ditolak — action belum disetujui manusia","code":"not_approved"}`))
				return
			}
			_, _ = w.Write([]byte(`{"wamid":"WAMID-1","duplicate":false}`))
		default:
			w.WriteHeader(404)
		}
	}))
}

func startBaileys(t *testing.T, approved map[string]bool, refuse map[string][2]string) *Baileys {
	t.Helper()
	b := &Baileys{Secret: secret, Listen: "127.0.0.1:0", Approved: func(_ context.Context, id string) (bool, error) { return approved[id], nil }}
	br := fakeBridge(t, func() string { return b.Addr() }, refuse)
	t.Cleanup(br.Close)
	b.BridgeURL = br.URL
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := b.Start(ctx); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBaileysSendNeedsApprovedOutbox(t *testing.T) {
	b := startBaileys(t, map[string]bool{"ob-1": true}, map[string][2]string{
		"cold@s.whatsapp.net":  {"first_contact", "Kontak belum pernah mengirim pesan ke nomor ini"},
		"night@s.whatsapp.net": {"quiet_hours", "Jam tenang 21.00–07.00 WIB"},
	})
	ctx := context.Background()
	var refused *SendRefused
	if _, err := b.Send(ctx, "6281234504471", "62811@s.whatsapp.net", "halo"); !errors.As(err, &refused) || refused.Code != "not_approved" {
		t.Fatalf("send without outbox id: %v", err)
	}
	if _, err := b.Send(WithAction(ctx, "ob-x"), "6281234504471", "62811@s.whatsapp.net", "halo"); !errors.As(err, &refused) || refused.Temporary() {
		t.Fatalf("unapproved outbox: %v", err)
	}
	id, err := b.Send(WithAction(ctx, "ob-1"), "6281234504471", "62811@s.whatsapp.net", "halo")
	if err != nil || id != "WAMID-1" {
		t.Fatalf("approved send: %q %v", id, err)
	}
	if _, err := b.Send(WithAction(ctx, "ob-1"), "6281234504471", "night@s.whatsapp.net", "halo"); !errors.As(err, &refused) || !refused.Temporary() || refused.RetryAfter != time.Hour {
		t.Fatalf("quiet hours: %+v", refused)
	}
	if _, err := b.Send(WithAction(ctx, "ob-1"), "6281234504471", "cold@s.whatsapp.net", "halo"); !errors.As(err, &refused) || refused.Temporary() || refused.Code != "first_contact" {
		t.Fatalf("cold contact: %+v", refused)
	}
}

func TestBaileysEventsAndStatus(t *testing.T) {
	b := startBaileys(t, nil, nil)
	post := func(body string, sig string) int {
		req, _ := http.NewRequest("POST", "http://"+b.Addr()+"/webhooks/wa", bytes.NewBufferString(body))
		req.Header.Set("X-ARC-Signature", sig)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	// statuses announced on start
	seen := map[string]string{}
	for range 2 {
		e := <-b.Events()
		seen[e.Status.Account] = e.Status.State
	}
	if seen["6281234504471"] != "connected" || seen["6281299990000"] != "pairing" {
		t.Fatalf("start statuses %v", seen)
	}
	ev := `{"event":{"wamid":"W1","session":"6281234504471","from":"6281900300102","to":"6281234504471","chat_id":"6281900300102@s.whatsapp.net","is_group":false,"sender_name":"Mbak Rina","text":"","media_meta":{"kind":"image","mime":"image/jpeg","file_name":"","size":1},"timestamp":"2026-10-05T07:00:00Z","from_me":false,"is_history":false,"transport":"bridge"}}`
	if code := post(ev, "bad"); code != 401 {
		t.Fatalf("unsigned event accepted: %d", code)
	}
	if code := post(ev, sign(secret, []byte(ev))); code != 200 {
		t.Fatalf("event: %d", code)
	}
	m := (<-b.Events()).Message
	if m == nil || m.Account != "6281234504471" || m.Text != "[gambar]" || m.FromName != "Mbak Rina" {
		t.Fatalf("message %+v", m)
	}
	st := `{"status":{"session":"6281234504471","status":"disconnected","phone":"","qr":"","reason":"logged_out"}}`
	_ = post(st, sign(secret, []byte(st)))
	if s := (<-b.Events()).Status; s == nil || s.State != "logged_out" {
		t.Fatalf("status %+v", s)
	}
	if qr, err := b.Pair(context.Background(), "0812-9999-0000"); err != nil || qr != "2@abc" {
		t.Fatalf("pair %q %v", qr, err)
	}
}
