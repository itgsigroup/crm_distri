package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
)

func post(t *testing.T, url, user string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("X-Dev-User", user)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// POST /cycles queues a cycle (202); a second request while it is queued/running gets 409 cycle_running.
func TestPostCycles409(t *testing.T) {
	srv, _ := chatServer(t)
	code, out := post(t, srv.URL+"/api/cycles", "sam@gsi.co.id", map[string]string{"scope": "screen:orbit", "via": "api"})
	if code != http.StatusAccepted || out["status"] != "queued" || out["label"] != "orbit" {
		t.Fatalf("first: %d %v", code, out)
	}
	code, out = post(t, srv.URL+"/api/cycles", "andi@gsi.co.id", map[string]string{"scope": "all"})
	if code != http.StatusConflict || out["error"].(map[string]any)["code"] != "cycle_running" {
		t.Fatalf("second: %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/cycles", "sam@gsi.co.id", map[string]string{"scope": "planet:mars"}); code != http.StatusBadRequest {
		t.Fatalf("bad scope: %d", code)
	}
}

// After a cycle, the plan is served and "Jalankan sekarang" on an auto step approves it as the human.
func TestPlanRunNow(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	o := &orchestrator.Orchestrator{St: st, Clock: clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)), Router: &llm.Router{Primary: llm.NewFake(nil), St: st, Log: log}, Log: log}
	if _, err := o.Run(ctx, domain.Scope{Kind: "all"}, domain.Trigger{Source: "schedule"}); err != nil {
		t.Fatal(err)
	}
	_, plan := get(t, srv, "/api/plan/today", "sam@gsi.co.id")
	items := plan["items"].([]any)
	if len(items) == 0 {
		t.Fatal("empty plan")
	}
	first := items[0].(map[string]any)
	if first["autonomy"] != "auto" || len(first["proposals"].([]any)) != 3 {
		t.Fatalf("first step %v", first)
	}
	code, out := post(t, srv.URL+"/api/plan/"+first["id"].(string)+"/run", "sam@gsi.co.id", nil)
	if code != http.StatusOK || out["sent"].(float64) != 3 {
		t.Fatalf("run: %d %v", code, out)
	}
	var decided int
	_ = st.Pool.QueryRow(ctx, "select count(*) from proposals p join users u on u.sales_user_id = p.decided_by where u.email = 'sam@gsi.co.id' and p.kind = 'followup'").Scan(&decided)
	if decided != 3 {
		t.Fatalf("%d follow-ups decided by Sam", decided)
	}
	_, plan = get(t, srv, "/api/plan/today", "sam@gsi.co.id")
	if s := plan["items"].([]any)[0].(map[string]any)["status"]; s != "done" {
		t.Fatalf("step status %v", s)
	}
}
