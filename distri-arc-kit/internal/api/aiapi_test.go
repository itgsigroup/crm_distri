package api_test

import (
	"context"
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

// Pengaturan → API & model AI (ADR 0040): the CEO stores both keys (checked, sealed, shown only as a hint), picks
// the second option and the models; the page shows what the API tokens cost.
func TestAIAPIPage(t *testing.T) {
	chatGPTOn(t) // the ChatGPT client itself, though GSI Orbit keeps it off (ADR 0048)
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	analyst.ListLive = func(_ context.Context, provider, _ string) ([]analyst.ModelInfo, error) {
		if provider == analyst.FallbackOpenAI {
			return []analyst.ModelInfo{{ID: "gpt-5"}, {ID: "gpt-5.1", Label: "GPT-5.1"}, {ID: "gpt-4.1-2025-04-14"}}, nil
		}
		return nil, nil
	}
	t.Cleanup(func() { analyst.ListLive = nil })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := clock.Fixed(time.Date(2026, 10, 15, 9, 0, 0, 0, clock.WIB))
	reject := func(err error) func(context.Context, string) error {
		return func(_ context.Context, key string) error {
			if strings.Contains(key, "bad") {
				return err
			}
			return nil
		}
	}
	h := api.New(config.Config{Env: "dev", SessionSecret: "0123456789abcdef0123456789abcdef"}, st, c, log).
		WithAnalystModel(nil, reject(analyst.ErrKeyRejected)).WithOpenAIVerify(reject(analyst.ErrOpenAIKeyRejected)).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	base, ceo, sales := srv.URL+"/api/ai/api", "sam@gsi.co.id", "andi@gsi.co.id"

	if _, err := st.Pool.Exec(ctx, `insert into llm_calls (agent, provider, model, purpose, tokens_in, tokens_out, cost_idr, created_at)
		values ('Analis terjadwal', 'openai', 'gpt-5', 'mcp.schedule', 1000, 200, 120, $1), ('Analis terjadwal', 'anthropic', 'claude-opus-5-5', 'mcp.schedule', 2000, 300, 250, $1)`, c.Now()); err != nil {
		t.Fatal(err)
	}
	code, info := sendJSON(t, "GET", base, ceo, nil)
	if code != 200 || info["fallback"] != "off" || info["ready"] != false || info["can_configure"] != true {
		t.Fatalf("info %d %v", code, info)
	}
	today := info["usage"].(map[string]any)["totals"].(map[string]any)["today"].(map[string]any)
	if today["cost_idr"] != float64(370) || today["tokens_in"] != float64(3000) {
		t.Fatalf("today %v", today)
	}
	if code, _ := sendJSON(t, "GET", base, sales, nil); code != http.StatusForbidden {
		t.Fatalf("sales opened the page: %d", code)
	}
	if code, _ := sendJSON(t, "PUT", base, sales, map[string]any{"fallback": "openai"}); code != http.StatusForbidden {
		t.Fatalf("sales changed the second option: %d", code)
	}

	// ADR 0043: Analisa WhatsApp otomatis — off by default; the CEO picks the hours and one API + model
	if a := info["wa_auto"].(map[string]any); a["enabled"] != false || a["engine"] != "claude" {
		t.Fatalf("wa_auto default %v", a)
	}
	auto := map[string]any{"enabled": true, "hours": []int{16, 8}, "weekend": false, "engine": "openai", "model": "gpt-5-mini"}
	if code, _ := sendJSON(t, "PUT", base+"/wa-auto", sales, auto); code != http.StatusForbidden {
		t.Fatalf("sales changed analisa otomatis: %d", code)
	}
	if code, _ := sendJSON(t, "PUT", base+"/wa-auto", ceo, map[string]any{"enabled": true, "hours": []int{8}, "engine": "openai", "model": "claude-opus-5-5"}); code != 400 {
		t.Fatalf("a Claude model accepted for ChatGPT: %d", code)
	}
	if code, out := sendJSON(t, "PUT", base+"/wa-auto", ceo, auto); code != 200 || out["wa_auto"].(map[string]any)["enabled"] != true ||
		len(out["wa_auto"].(map[string]any)["hours"].([]any)) != 2 || out["wa_auto"].(map[string]any)["model"] != "gpt-5-mini" {
		t.Fatalf("wa_auto %d %v", code, out["wa_auto"])
	}

	if code, out := sendJSON(t, "PUT", base+"/key/openai", ceo, map[string]any{"api_key": "sk-ant-api03-000000000000000000"}); code != 400 || !strings.Contains(out["error"].(map[string]any)["message"].(string), "ChatGPT") {
		t.Fatalf("a Claude key accepted as ChatGPT: %d %v", code, out)
	}
	if code, _ := sendJSON(t, "PUT", base+"/key/openai", ceo, map[string]any{"api_key": "sk-proj-bad-0000000000000000000"}); code != 400 {
		t.Fatalf("rejected key stored: %d", code)
	}
	if code, out := sendJSON(t, "PUT", base+"/key/openai", ceo, map[string]any{"api_key": "sk-proj-good-000000000000000wxyz"}); code != 200 || out["key_hint"] != "sk-proj…wxyz" {
		t.Fatalf("key %d %v", code, out)
	}
	if code, out := sendJSON(t, "PUT", base, ceo, map[string]any{"fallback": "openai", "openai_model": "gpt-5.1"}); code != 200 || out["fallback"] != "openai" || out["ready"] != true {
		t.Fatalf("config %d %v", code, out)
	}
	// with a key the list holds every chat model OpenAI returns, after the known ones; a dated copy of a listed model
	// is left out; an unknown price stays unknown
	_, info = sendJSON(t, "GET", base, ceo, nil)
	var ids []string
	var unknown string
	for _, p := range info["providers"].([]any) {
		pm := p.(map[string]any)
		if pm["id"] != "openai" {
			continue
		}
		for _, m := range pm["models"].([]any) {
			mm := m.(map[string]any)
			ids = append(ids, mm["id"].(string))
			if mm["id"] == "gpt-5.1" && mm["in_usd"] == nil {
				unknown = "ok"
			}
		}
	}
	if j := strings.Join(ids, ","); !strings.HasPrefix(j, "gpt-5,") || !strings.HasSuffix(j, ",gpt-5.1") || strings.Contains(j, "2025") || unknown != "ok" {
		t.Fatalf("openai models %s (unknown price %q)", j, unknown)
	}
	// the older form on MCP Claude (model + budget only) keeps the second option and the ChatGPT model
	if code, out := sendJSON(t, "PUT", srv.URL+"/api/mcp/analyst", ceo, map[string]any{"model": "claude-sonnet-5-5", "daily_budget_idr": 99000}); code != 200 || out["model"] != "claude-sonnet-5-5" {
		t.Fatalf("analyst form %d %v", code, out)
	}
	if _, out := sendJSON(t, "GET", base, ceo, nil); out["fallback"] != "openai" || out["daily_budget_idr"] != float64(99000) {
		t.Fatalf("second option reset by the analyst form: %v %v", out["fallback"], out["daily_budget_idr"])
	}
	if code, _ := sendJSON(t, "PUT", base, ceo, map[string]any{"openai_model": "llama-3; drop"}); code != 400 {
		t.Fatalf("unknown model accepted: %d", code)
	}
	if code, _ := sendJSON(t, "DELETE", base+"/key/openai", ceo, nil); code != http.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
	if _, out := sendJSON(t, "GET", base, ceo, nil); out["ready"] != false {
		t.Fatalf("ready without the key: %v", out["ready"])
	}
}

func chatGPTOn(t *testing.T) {
	t.Helper()
	analyst.ChatGPTOff = false
	t.Cleanup(func() { analyst.ChatGPTOff = true })
}
