package orchestrator_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
)

// ADR 0046: "Sistem GSI Orbit" runs the cycle on the server with the agents' rules and the glossary formulas, even
// under llm.routing mode mcp + require_ai — no Input published for Claude, no waiting, proposals stored.
func TestSystemCycleRunsWithoutClaude(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `update policies set value = value || '{"mode": "mcp", "require_ai": true}'::jsonb where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	o.MCPWait = time.Hour // a cycle that waited for Claude would hang the test
	o.OnMCPWait = func(context.Context, uuid.UUID, int64, []string, time.Duration) time.Duration {
		t.Error("a Sistem GSI Orbit cycle waited for Claude")
		return 0
	}
	rep, err := o.Run(ctx, all, domain.Trigger{Source: "manual", By: "ceo@gsi", Via: "system"})
	if err != nil {
		t.Fatalf("system cycle: %v", err)
	}
	if rep.Cycle.Status == "failed" {
		t.Fatalf("system cycle failed: %v", rep.Errors)
	}
	if rep.Stored == 0 {
		t.Fatal("system cycle stored no proposals")
	}
	var inputs int
	if err := st.Pool.QueryRow(ctx, "select count(*) from cycle_inputs where cycle_id = $1", rep.Cycle.ID).Scan(&inputs); err != nil || inputs != 0 {
		t.Fatalf("%d MCP inputs published (%v)", inputs, err)
	}
	var via string
	if err := st.Pool.QueryRow(ctx, "select via from cycles where id = $1", rep.Cycle.ID).Scan(&via); err != nil || via != "system" {
		t.Fatalf("via %q (%v)", via, err)
	}
}
