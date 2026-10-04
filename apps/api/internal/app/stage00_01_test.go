package app

import (
	"context"
	"strings"
	"testing"

	"arc/apps/api/internal/seed"
)

// Stage 00: the fixture loader reads the mockup data set.
func TestStage00FixtureLoaderCounts(t *testing.T) {
	f := loadFixtures(t)
	inbound := len(f.Inbound)
	if len(f.Deals) < 8 || len(f.Edges.Edges) < 25 || len(f.Chats) < 8 || inbound < 4 {
		t.Fatalf("fixtures too small: deals=%d edges=%d chats=%d inbound=%d", len(f.Deals), len(f.Edges.Edges), len(f.Chats), inbound)
	}
}

// Stage 00: /health answers with the database up.
func TestStage00Health(t *testing.T) {
	_, srv := fresh(t)
	var h map[string]any
	newClient(t, srv).json("GET", "/health", nil, 200, &h)
	if h["status"] != "ok" || h["db"] != "ok" {
		t.Fatalf("health: %v", h)
	}
}

// Stage 01: GET /api/accounts/rsud → health 41 ± 3 with a component breakdown.
func TestStage01AccountHealthWithBreakdown(t *testing.T) {
	_, srv := fresh(t)
	sam := login(t, srv, "sam@gsi.co.id")
	var acc struct {
		Health int `json:"health"`
		Deal   struct {
			Breakdown []struct {
				Label string `json:"label"`
				Value int    `json:"value"`
			} `json:"breakdown"`
		} `json:"deal"`
	}
	sam.json("GET", "/api/accounts/rsud", nil, 200, &acc)
	if acc.Health < 38 || acc.Health > 44 {
		t.Fatalf("rsud health = %d, want 41 ± 3", acc.Health)
	}
	if len(acc.Deal.Breakdown) < 4 {
		t.Fatalf("breakdown too short: %+v", acc.Deal.Breakdown)
	}
	for _, b := range acc.Deal.Breakdown {
		if b.Label == "" || b.Value < 0 || b.Value > 100 {
			t.Fatalf("bad breakdown row %+v", b)
		}
	}
}

var mainTables = []string{"users", "accounts", "people", "opportunities", "health_components", "wa_sessions", "chat_groups", "chat_threads",
	"interactions", "extractions", "commitments", "signals", "actions", "action_decisions", "tasks", "inbound_contacts", "funnel_events",
	"installed_systems", "whitespace", "cash_items", "invoices", "expected_receipts", "payment_history", "tenders", "internal_numbers",
	"learned_rules", "policies", "privacy_rules", "settings", "stage_definitions"}

// Stage 01: seeding twice adds no rows; the audit log is filled and append-only.
func TestStage01SeedIdempotentAndAuditAppendOnly(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	before := map[string]int{}
	for _, tbl := range mainTables {
		before[tbl] = count(t, a, `SELECT count(*) FROM `+tbl)
	}
	if err := seed.Run(ctx, a.DB, loadFixtures(t)); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	for _, tbl := range mainTables {
		if n := count(t, a, `SELECT count(*) FROM `+tbl); n != before[tbl] {
			t.Errorf("%s: %d rows after second seed, was %d", tbl, n, before[tbl])
		}
	}
	if count(t, a, `SELECT count(*) FROM audit_log WHERE verb='seed'`) != 1 {
		t.Fatal("expected exactly one seed audit entry")
	}
	if count(t, a, `SELECT count(*) FROM audit_log`) < 5 {
		t.Fatal("audit log nearly empty")
	}
	for _, q := range []string{`UPDATE audit_log SET verb='tampered'`, `DELETE FROM audit_log`} {
		_, err := a.DB.Pool.Exec(ctx, q)
		if err == nil {
			t.Fatalf("%q succeeded on the append-only audit log", q)
		}
		if !strings.Contains(strings.ToLower(err.Error()), "audit") {
			t.Logf("audit guard error: %v", err)
		}
	}
}
