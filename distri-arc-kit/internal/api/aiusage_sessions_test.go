package api

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/store/gen"
)

// Claude's MCP calls group into sessions per connection; a quiet gap over 30 minutes starts a new one.
func TestMCPSessions(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	t0 := time.Date(2026, 10, 10, 3, 0, 0, 0, time.UTC)
	ms := int32(500)
	call := func(c uuid.UUID, at time.Duration, tool, status string) gen.MCPCallsSinceRow {
		return gen.MCPCallsSinceRow{ClientID: &c, ClientName: "Claude Sam", ClientKind: "oauth", UserName: "Sam", Tool: tool, Status: status, DurationMs: &ms, CreatedAt: t0.Add(at)}
	}
	got := mcpSessions([]gen.MCPCallsSinceRow{
		call(a, 0, "kpi_utama", "ok"), call(a, 2*time.Minute, "dealer_list", "ok"), call(a, 5*time.Minute, "kpi_utama", "ok"),
		call(a, 50*time.Minute, "actions_decide", "human_only"),
		call(b, time.Minute, "stok_aging", "ok"),
	})
	if len(got) != 3 {
		t.Fatalf("sessions: %d %+v", len(got), got)
	}
	if got[0].Calls != 3 || got[0].Title != "Claude lewat MCP · Claude Sam · dealer_list, kpi_utama" || *got[0].DurationMs != 5*60*1000+500 || got[0].Status != "ok" {
		t.Fatalf("first session: %+v", got[0])
	}
	if got[1].Calls != 1 || got[1].Status != "partial" {
		t.Fatalf("second session after a 45-minute gap: %+v", got[1])
	}
	if got[0].ID == got[1].ID || got[2].By != "Sam" {
		t.Fatalf("ids/people: %+v", got)
	}
}
