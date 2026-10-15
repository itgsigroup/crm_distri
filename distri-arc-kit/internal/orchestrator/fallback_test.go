package orchestrator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ADR 0040: Claude through MCP first; when the API second option is ready, Analisis waits the extra window the hook
// returns, so answers the API submits after MCP_WAIT still count for the running cycle.
func TestMCPWaitExtendedForAPIFallback(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `update policies set value = value || '{"mode": "mcp", "require_ai": true}'::jsonb where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	o.MCPWait = 300 * time.Millisecond
	var mu sync.Mutex
	var asked []string
	var cycle uuid.UUID
	o.OnMCPWait = func(_ context.Context, cid uuid.UUID, _ int64, agents []string, wait time.Duration) time.Duration {
		mu.Lock()
		defer mu.Unlock()
		if wait != o.MCPWait {
			t.Errorf("wait %s", wait)
		}
		asked, cycle = agents, cid
		go func() { // the API answers every agent after the MCP wait is over
			time.Sleep(600 * time.Millisecond)
			for _, a := range agents {
				if _, err := o.Submit(context.Background(), cid, a, []byte("[]"), nil); err != nil {
					t.Errorf("submit %s: %v", a, err)
				}
			}
		}()
		return time.Minute // generous: Analisis ends as soon as every agent is answered (a loaded test run is slow)
	}
	rep, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatalf("cycle answered by the API second option failed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(asked) == 0 || cycle != rep.Cycle.ID {
		t.Fatalf("hook asked %v for %s", asked, cycle)
	}
	var failed int
	if err := st.Pool.QueryRow(ctx, "select count(*) from agent_runs where cycle_id = $1 and status = 'failed'", cycle).Scan(&failed); err != nil || failed != 0 {
		t.Fatalf("%d agents failed (%v)", failed, err)
	}
}
