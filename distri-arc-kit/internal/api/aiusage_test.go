package api_test

import (
	"context"
	"net/http"
	"testing"
)

// MCP Claude → Pemakaian AI: the model of each analysis, its cost, schedule and history (a cycle with its calls).
func TestAIUsageShowsModelCostAndHistory(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var cycle string
	if err := st.Pool.QueryRow(ctx, "insert into cycles (trigger, status, started_at, duration_ms) values ('schedule', 'done', now(), 1200) returning id").Scan(&cycle); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into llm_calls (cycle_id, agent, provider, model, purpose, tokens_in, tokens_out, cost_idr)
		values ($1, 'AI Order', 'anthropic', 'claude-sonnet-5-5', 'so.draft', 1000, 200, 66), ($1, 'Orchestrator', 'anthropic', 'claude-sonnet-5-5', 'brief.write', 500, 100, 33)`, cycle); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, srv, "/api/mcp/usage", "andi@gsi.co.id"); code != http.StatusForbidden {
		t.Fatalf("a sales sees AI cost: %d", code)
	}
	code, out := get(t, srv, "/api/mcp/usage", "sam@gsi.co.id")
	if code != 200 {
		t.Fatalf("usage: %d %v", code, out)
	}
	if out["orchestrator"].(map[string]any)["model"] == "" {
		t.Fatalf("no orchestrator model: %v", out["orchestrator"])
	}
	today := out["cost"].(map[string]any)["today"].(map[string]any)
	if today["cost_idr"] != float64(99) || today["calls"] != float64(2) {
		t.Fatalf("today's cost: %v", today)
	}
	found := false
	for _, x := range out["runs"].([]any) {
		if run := x.(map[string]any); run["id"] == cycle {
			found = run["cost_idr"] == float64(99) && run["model"] == "claude-sonnet-5-5" && run["calls"] == float64(2)
		}
	}
	if !found {
		t.Fatalf("cycle missing or wrong in history: %v", out["runs"])
	}
}
