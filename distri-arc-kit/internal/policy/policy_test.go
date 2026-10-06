package policy_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/policy"
	"distri-arc/internal/seed"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
)

// Every seeded policy satisfies its schema, and every key has a schema.
func TestSeedPoliciesValidate(t *testing.T) {
	b, err := os.ReadFile("../../db/seed/policies.json")
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]json.RawMessage
	_ = json.Unmarshal(b, &all)
	if len(all) != len(policy.Keys()) {
		t.Fatalf("%d seeded keys, %d schemas", len(all), len(policy.Keys()))
	}
	for k, v := range all {
		if err := policy.Validate(k, v); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		key, value string
		want       error
	}{
		{"orbit.thresholds", `{"drift": 0.9, "churn": 2, "key_account": {"sow_min": 50, "on_time_min": 85}}`, policy.ErrInvalid},
		{"orbit.thresholds", `{"drift": 2.5, "churn": 2, "key_account": {"sow_min": 50, "on_time_min": 85}}`, policy.ErrInvalid},
		{"margin.floor", `{"pct": "9"}`, policy.ErrInvalid},
		{"margin.floor", `{"pct": 9, "extra": 1}`, policy.ErrInvalid},
		{"mcp.permissions", `{"allow_reanalyze":true,"allow_plan_update_proposal":true,"allow_send":true,"mask_pii_in_read":true,"max_cycles_per_hour":6}`, policy.ErrInvalid},
		{"autonomy.matrix", `{"AI Kredit": {"auto": ["credit_release"], "approve": [], "never": []}}`, policy.ErrLocked},
		{"nope", `{}`, policy.ErrUnknownKey},
	}
	for _, c := range cases {
		if err := policy.Validate(c.key, json.RawMessage(c.value)); !errors.Is(err, c.want) {
			t.Errorf("%s %s: %v, want %v", c.key, c.value, err, c.want)
		}
	}
	credit, _ := os.ReadFile("../../db/seed/policies.json")
	var all map[string]map[string]any
	_ = json.Unmarshal(credit, &all)
	c := all["credit.rules"]
	c["sop_sec_001_required"] = false
	b, _ := json.Marshal(c)
	if err := policy.Validate("credit.rules", b); !errors.Is(err, policy.ErrInvalid) {
		t.Fatalf("SOP-SEC-001 switched off: %v", err)
	}
}

// Save bumps the version, keeps history and is read back by Load.
func TestSaveVersionsAndHistory(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	v := json.RawMessage(`{"drift": 1.5, "churn": 2, "key_account": {"sow_min": 50, "on_time_min": 85}}`)
	p, err := policy.Save(ctx, st, "orbit.thresholds", v, nil, "sam@gsi.co.id", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 2 {
		t.Fatalf("version %d", p.Version)
	}
	ps, _ := policy.Load(ctx, st.Q)
	if ps.Orbit.Drift != 1.5 {
		t.Fatalf("drift %v", ps.Orbit.Drift)
	}
	h, _ := st.Q.ListPolicyHistory(ctx, gen.ListPolicyHistoryParams{Key: "orbit.thresholds", Limit: 10})
	if len(h) != 1 || h[0].Version != 2 {
		t.Fatalf("history %+v", h)
	}
	if _, err := policy.Save(ctx, st, "orbit.thresholds", json.RawMessage(`{"drift": 9}`), nil, "x", time.Now()); !errors.Is(err, policy.ErrInvalid) {
		t.Fatalf("invalid saved: %v", err)
	}
}
