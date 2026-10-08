package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/analyst"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

func sendJSON(t *testing.T, method, url, user string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
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

// Scheduled analysis: cron validated (format and minimum gap), only people who may connect Claude edit schedules,
// only the CEO sets the API key (checked, sealed, never shown again), "Jalankan sekarang" produces a report.
func TestAnalystSchedules(t *testing.T) {
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB))
	h := api.New(config.Config{Env: "dev", SessionSecret: "0123456789abcdef0123456789abcdef"}, st, c, log).
		WithAnalystModel(nil, func(_ context.Context, key string) error {
			if strings.Contains(key, "bad") {
				return analyst.ErrKeyRejected
			}
			return nil
		}).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	base, ceo, sales := srv.URL+"/api/mcp", "sam@gsi.co.id", "andi@gsi.co.id"

	code, info := sendJSON(t, "GET", base+"/analyst", ceo, nil)
	if code != 200 || info["engine"] != "template" || info["model"] != "claude-opus-5-5" || info["can_configure"] != true {
		t.Fatalf("info %d %v", code, info)
	}
	if code, out := sendJSON(t, "GET", base+"/cron?expr=0+7+*+*+1-6", ceo, nil); code != 200 || out["description"] != "Senin–Sabtu pukul 07.00" || len(out["next"].([]any)) != 5 {
		t.Fatalf("cron preview %v", out)
	}
	if _, out := sendJSON(t, "GET", base+"/cron?expr=*/5+*+*+*+*", ceo, nil); out["ok"] != false || !strings.Contains(out["error"].(string), "15 menit") {
		t.Fatalf("too frequent accepted %v", out)
	}

	body := map[string]any{"name": "Piutang mingguan", "prompt": "Analisis piutang_ringkas dan beri dealer yang perlu ditagih.", "cron": "0 8 * * 1", "scopes": []string{"analyze", "orchestrate"}}
	if code, _ := sendJSON(t, "POST", base+"/schedules", sales, body); code != http.StatusForbidden {
		t.Fatalf("sales created a schedule: %d", code)
	}
	if code, out := sendJSON(t, "POST", base+"/schedules", ceo, map[string]any{"name": "x", "prompt": "cukup panjang ya", "cron": "0 25 * * *"}); code != 400 || !strings.Contains(out["error"].(map[string]any)["message"].(string), "jam") {
		t.Fatalf("bad cron %d %v", code, out)
	}
	code, sch := sendJSON(t, "POST", base+"/schedules", ceo, body)
	if code != 200 || sch["description"] != "Setiap Senin pukul 08.00" || len(sch["scopes"].([]any)) != 3 {
		t.Fatalf("create %d %v", code, sch)
	}
	id := sch["id"].(string)
	if code, out := sendJSON(t, "PUT", base+"/schedules/"+id, ceo, map[string]any{"name": "Piutang harian", "prompt": body["prompt"], "cron": "30 6 * * *", "enabled": false}); code != 200 || out["enabled"] != false || len(out["next_runs"].([]any)) != 0 {
		t.Fatalf("update %d %v", code, out)
	}
	if _, out := sendJSON(t, "GET", base+"/schedules", ceo, nil); len(out["items"].([]any)) != 2 {
		t.Fatalf("list %v", out)
	}

	// key: CEO only, rejected keys are not stored, the key is never returned
	if code, _ := sendJSON(t, "PUT", base+"/analyst/key", "admin@gsi.co.id", map[string]any{"api_key": "sk-ant-api03-goodkey-000000000000"}); code != http.StatusForbidden {
		t.Fatalf("admin set key: %d", code)
	}
	if code, _ := sendJSON(t, "PUT", base+"/analyst/key", ceo, map[string]any{"api_key": "not-a-key"}); code != 400 {
		t.Fatalf("format: %d", code)
	}
	if code, _ := sendJSON(t, "PUT", base+"/analyst/key", ceo, map[string]any{"api_key": "sk-ant-api03-badkey-000000000000"}); code != 400 {
		t.Fatalf("rejected key stored: %d", code)
	}
	if code, out := sendJSON(t, "PUT", base+"/analyst/key", ceo, map[string]any{"api_key": "sk-ant-api03-goodkey-000000000000"}); code != 200 || out["key_hint"] != "sk-ant-…0000" {
		t.Fatalf("set key %d %v", code, out)
	}
	_, info = sendJSON(t, "GET", base+"/analyst", ceo, nil)
	if info["engine"] != "claude" || info["key_source"] != "ui" {
		t.Fatalf("after key %v", info)
	}
	raw, _ := json.Marshal(info)
	if strings.Contains(string(raw), "goodkey") {
		t.Fatal("key leaked in info")
	}
	if code, _ := sendJSON(t, "PUT", base+"/analyst", ceo, map[string]any{"model": "gpt-4", "daily_budget_idr": 1000}); code != 400 {
		t.Fatalf("unknown model accepted: %d", code)
	}
	if code, out := sendJSON(t, "PUT", base+"/analyst", ceo, map[string]any{"model": "claude-sonnet-5-5", "daily_budget_idr": 0}); code != 200 || out["model"] != "claude-sonnet-5-5" {
		t.Fatalf("config %d %v", code, out)
	}
	if code, _ := sendJSON(t, "DELETE", base+"/analyst/key", ceo, nil); code != http.StatusNoContent {
		t.Fatalf("delete key %d", code)
	}

	// run now (no queue in tests → inline): without a key → template report, listed under the schedule
	code, out := sendJSON(t, "POST", base+"/schedules/"+id+"/run", ceo, nil)
	run, _ := out["run"].(map[string]any)
	if code != 200 || run["status"] != "template" || !strings.Contains(run["report"].(string), "## Ringkasan bisnis") {
		t.Fatalf("run %d %v", code, out)
	}
	_, runs := sendJSON(t, "GET", base+"/schedules/"+id+"/runs", ceo, nil)
	items := runs["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["step_count"].(float64) != 2 {
		t.Fatalf("runs %v", runs)
	}
	if code, full := sendJSON(t, "GET", base+"/runs/"+run["id"].(string), ceo, nil); code != 200 || full["schedule_name"] != "Piutang harian" {
		t.Fatalf("run detail %d %v", code, full)
	}
	if code, _ := sendJSON(t, "DELETE", base+"/schedules/"+id, ceo, nil); code != http.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
	var active bool
	if err := st.Pool.QueryRow(context.Background(), `select active from mcp_clients where kind = 'schedule' and name = 'Terjadwal · Piutang harian'`).Scan(&active); err != nil || active {
		t.Fatalf("schedule client not revoked: %v %v", active, err)
	}
	if code, _ := sendJSON(t, "GET", base+"/analyst", sales, nil); code != http.StatusForbidden {
		t.Fatalf("sales opened MCP page API: %d", code)
	}
}
