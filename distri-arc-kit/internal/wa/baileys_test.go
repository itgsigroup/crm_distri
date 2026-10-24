package wa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
				Live     bool   `json:"live"`
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
			if in.Live {
				_, _ = w.Write([]byte(`{"wamid":"WAMID-LIVE","duplicate":false}`)) // a person chatting (ADR 0064)
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
	if id, err := b.Send(WithLive(WithAction(ctx, "ob-1")), "6281234504471", "62811@s.whatsapp.net", "halo"); err != nil || id != "WAMID-LIVE" {
		t.Fatalf("a reply typed in Chat reaches the bridge as live: %q %v", id, err)
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
		seen[e.Status.Session] = e.Status.State
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
	ev = `{"event":{"wamid":"W2","session":"6281234504471","from":"6281900300102","to":"6281234504471","chat_id":"6281900300102@s.whatsapp.net","text":"Foto unit","media_meta":{"kind":"video","mime":"video/mp4","size":9000,"seconds":14,"ref":"UkVG"},"timestamp":"2026-10-05T07:01:00Z","from_me":false}}`
	_ = post(ev, sign(secret, []byte(ev)))
	if m := (<-b.Events()).Message; m == nil || m.Text != "[video] Foto unit" || m.Media == nil || m.Media.Ref != "UkVG" || m.Media.Seconds != 14 || !m.Media.Viewable() {
		t.Fatalf("media message %+v", m)
	}
	ev = `{"event":{"wamid":"W3","session":"6281234504471","from":"6281900300102","to":"6281234504471","chat_id":"6281900300102@s.whatsapp.net","text":"Minta 20 unit","edit_of":"W1","timestamp":"2026-10-05T07:02:00Z","from_me":false}}`
	_ = post(ev, sign(secret, []byte(ev)))
	if m := (<-b.Events()).Message; m == nil || m.EditOf != "W1" || m.Text != "Minta 20 unit" {
		t.Fatalf("edit %+v", m)
	}
	st := `{"status":{"session":"6281234504471","status":"disconnected","phone":"","qr":"","reason":"logged_out"}}`
	_ = post(st, sign(secret, []byte(st)))
	if s := (<-b.Events()).Status; s == nil || s.State != "logged_out" {
		t.Fatalf("status %+v", s)
	}
	if qr, err := b.Pair(context.Background(), "0812-9999-0000"); err != nil || qr != "2@abc" {
		t.Fatalf("pair %q %v", qr, err)
	}
	// a new link: the session is named before the number; the number arrives with "connected" and on every event
	st = `{"status":{"session":"link-ab12","status":"connected","phone":"6281277770000","qr":""}}`
	_ = post(st, sign(secret, []byte(st)))
	if s := (<-b.Events()).Status; s == nil || s.Session != "link-ab12" || s.Account != "6281277770000" || s.State != "connected" {
		t.Fatalf("link status %+v", s)
	}
	ev = `{"event":{"wamid":"W2","session":"link-ab12","account":"6281277770000","from":"6281900300102","to":"6281277770000","chat_id":"6281900300102@s.whatsapp.net","is_group":false,"sender_name":"","text":"halo","timestamp":"2026-10-05T07:00:00Z","from_me":false,"is_history":false,"transport":"bridge"}}`
	_ = post(ev, sign(secret, []byte(ev)))
	if m := (<-b.Events()).Message; m == nil || m.Account != "6281277770000" {
		t.Fatalf("link message %+v", m)
	}
}

// A bridge that gave up waiting (post timeout) queues the batch and posts it again later: the API stops pushing
// the rest of it, so a slow moment does not store every history batch twice over (14 Oct: 513 MB of re-queued
// batches).
func TestWebhookStopsWhenBridgeGivesUp(t *testing.T) {
	b := startBaileys(t, nil, nil)
	for range 2 { // statuses announced on start
		<-b.Events()
	}
	var evs []string
	for i := range 300 { // more than the events buffer, which nobody reads yet
		evs = append(evs, fmt.Sprintf(`{"wamid":"H%d","session":"6281234504471","from":"6281900300102","chat_id":"6281900300102@s.whatsapp.net","text":"riwayat","timestamp":"2026-10-05T07:00:00Z","is_history":true}`, i))
	}
	body := `{"events":[` + strings.Join(evs, ",") + `]}`
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+b.Addr()+"/webhooks/wa", bytes.NewBufferString(body))
	req.Header.Set("X-ARC-Signature", sign(secret, []byte(body)))
	if _, err := http.DefaultClient.Do(req); err == nil {
		t.Fatal("expected the post to time out while the events buffer is full")
	}
	time.Sleep(100 * time.Millisecond) // the handler sees the bridge has gone
	got := 0
	for {
		select {
		case <-b.Events():
			got++
			continue
		case <-time.After(200 * time.Millisecond):
		}
		break
	}
	if got >= 300 {
		t.Fatalf("all %d events pushed after the bridge gave up", got)
	}
}

// A message from a session still named "link-…" never takes the digits inside that id as the account.
func TestBridgeEventAccountNeverFromLinkID(t *testing.T) {
	m := bridgeEvent{WAMID: "x", Session: "link-3fa9c1d2e4b5", Text: "halo", Timestamp: "2026-10-05T01:00:00Z"}.message()
	if m.Account != "" {
		t.Fatalf("account from link id = %q, want empty", m.Account)
	}
	m = bridgeEvent{WAMID: "y", Session: "6281234567890", Text: "halo", Timestamp: "2026-10-05T01:00:00Z"}.message()
	if m.Account != "6281234567890" {
		t.Fatalf("account from numbered session = %q", m.Account)
	}
	m = bridgeEvent{WAMID: "z", Session: "link-ab12", Account: "6289900000001", Text: "halo", Timestamp: "2026-10-05T01:00:00Z"}.message()
	if m.Account != "6289900000001" {
		t.Fatalf("account = %q, want the phone's own number", m.Account)
	}
}

// A bridge still starting (a deploy restarts both) is asked again: no number is announced "unpaired" because the
// bridge did not answer yet.
func TestStartWaitsForTheBridge(t *testing.T) {
	up := make(chan struct{})
	br := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-up:
			_, _ = w.Write([]byte(`{"ok":true,"sessions":{"link-a":"connected"}}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer br.Close()
	b := &Baileys{BridgeURL: br.URL, Secret: secret, Listen: "127.0.0.1:0", RetryEvery: 20 * time.Millisecond,
		Approved: func(context.Context, string) (bool, error) { return false, nil },
		Accounts: func(context.Context) []string { return []string{"link-a", "link-b"} }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := b.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-b.Events():
		t.Fatalf("announced before the bridge answered: %+v", e.Status)
	case <-time.After(100 * time.Millisecond):
	}
	close(up)
	seen := map[string]string{}
	for range 2 {
		e := <-b.Events()
		seen[e.Status.Session] = e.Status.State
	}
	if seen["link-a"] != "connected" || seen["link-b"] != "unpaired" {
		t.Fatalf("start statuses %v", seen)
	}
}
