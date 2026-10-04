package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"arc/packages/connectors/whatsapp"
)

// Stage 13: a bridge session the bridge reported as disconnected for more than
// 10 minutes alerts the CEO once per outage.
func TestStage13BridgeWatchdog(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	report := func(session string) {
		body, _ := json.Marshal(map[string]any{"type": "status", "status": whatsapp.SessionStatus{Session: session, Status: "disconnected"}})
		req, _ := http.NewRequest("POST", srv.URL+"/webhooks/wa", strings.NewReader(string(body)))
		req.Header.Set("X-ARC-Signature", whatsapp.Sign(a.Cfg.BridgeSecret, body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("status report: %v %v", resp, err)
		}
		resp.Body.Close()
	}
	report("s-andi") // down for 15 minutes → alert
	report("s-dewi") // down for 5 minutes → not yet
	exec(t, a, `UPDATE wa_sessions SET updated_at = now() - interval '15 minutes' WHERE id='s-andi'`)
	exec(t, a, `UPDATE wa_sessions SET updated_at = now() - interval '5 minutes' WHERE id='s-dewi'`)
	// Marked disconnected without a bridge report (e.g. never linked) → no alert.
	exec(t, a, `UPDATE wa_sessions SET status='disconnected', updated_at = now() - interval '2 hours' WHERE id='s-fajar'`)

	before := a.FakeNotify.Count()
	n, err := a.watchBridgeSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || a.FakeNotify.Count() != before+1 {
		t.Fatalf("watchdog alerts: %v (notifier +%d), want exactly 1", n, a.FakeNotify.Count()-before)
	}
	msg := a.FakeNotify.Sent[len(a.FakeNotify.Sent)-1]
	if !strings.Contains(msg.Subject, "s-andi") || !strings.Contains(strings.Join(msg.To, ","), "sam@gsi.co.id") {
		t.Fatalf("alert: %+v", msg)
	}
	// Second run through the registered job: no repeat alert for the same outage.
	out, err := a.RunJob(ctx, "bridge_watch")
	if err != nil || out != "0" || a.FakeNotify.Count() != before+1 {
		t.Fatalf("second watchdog run alerted again (%s, %v)", out, err)
	}
}

// Stage 13: GET /api/metrics is CEO-only and reports jobs, LLM usage, queue and sessions.
func TestStage13MetricsCEOOnly(t *testing.T) {
	_, srv := fresh(t)
	if code, _ := login(t, srv, "andi@gsi.co.id").do("GET", "/api/metrics", nil); code != 403 {
		t.Fatalf("sales → %d, want 403", code)
	}
	sam := login(t, srv, "sam@gsi.co.id")
	key := sam.apiKey("metrics bot", "read")
	if code, _ := bearerClient(t, srv, key).do("GET", "/api/metrics", nil); code != 403 {
		t.Fatalf("API key → %d, want 403", code)
	}
	var m map[string]any
	sam.json("GET", "/api/metrics", nil, 200, &m)
	for _, k := range []string{"jobs", "llm_24h", "actions", "wa_sessions", "bridge"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("metrics lacks %s: %v", k, m)
		}
	}
	if jobs, _ := m["jobs"].([]any); len(jobs) == 0 {
		t.Fatal("no job runs reported after PostSeed")
	}
}
