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
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
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

	if code, out := sendJSON(t, "PUT", base+"/key/openai", ceo, map[string]any{"api_key": "sk-ant-api03-000000000000000000"}); code != 400 || !strings.Contains(out["error"].(map[string]any)["message"].(string), "ChatGPT") {
		t.Fatalf("a Claude key accepted as ChatGPT: %d %v", code, out)
	}
	if code, _ := sendJSON(t, "PUT", base+"/key/openai", ceo, map[string]any{"api_key": "sk-proj-bad-0000000000000000000"}); code != 400 {
		t.Fatalf("rejected key stored: %d", code)
	}
	if code, out := sendJSON(t, "PUT", base+"/key/openai", ceo, map[string]any{"api_key": "sk-proj-good-000000000000000wxyz"}); code != 200 || out["key_hint"] != "sk-proj…wxyz" {
		t.Fatalf("key %d %v", code, out)
	}
	if code, out := sendJSON(t, "PUT", base, ceo, map[string]any{"fallback": "openai", "openai_model": "gpt-5-mini"}); code != 200 || out["fallback"] != "openai" || out["ready"] != true {
		t.Fatalf("config %d %v", code, out)
	}
	if code, _ := sendJSON(t, "PUT", base, ceo, map[string]any{"openai_model": "gpt-9"}); code != 400 {
		t.Fatalf("unknown model accepted: %d", code)
	}
	if code, _ := sendJSON(t, "DELETE", base+"/key/openai", ceo, nil); code != http.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
	if _, out := sendJSON(t, "GET", base, ceo, nil); out["ready"] != false {
		t.Fatalf("ready without the key: %v", out["ready"])
	}
}
