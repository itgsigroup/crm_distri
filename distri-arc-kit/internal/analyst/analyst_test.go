package analyst_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"distri-arc/db"
	"distri-arc/internal/analyst"
	"distri-arc/internal/clock"
	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
)

var now = clock.Fixed(time.Date(2026, 10, 5, 7, 0, 0, 0, clock.WIB))

// script plays back assistant turns (Messages API JSON) and keeps the requests it got.
type script struct {
	turns []string
	reqs  []anthropic.BetaMessageNewParams
}

func (s *script) Step(_ context.Context, p anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	s.reqs = append(s.reqs, p)
	var m anthropic.BetaMessage
	if err := json.Unmarshal([]byte(s.turns[len(s.reqs)-1]), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func toolTurn(id, name, input string) string {
	return `{"id":"msg_` + id + `","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"tool_use",
	"content":[{"type":"tool_use","id":"tu_` + id + `","name":"` + name + `","input":` + input + `}],"usage":{"input_tokens":2000,"output_tokens":120}}`
}

const finalTurn = `{"id":"msg_f","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"end_turn",
	"content":[{"type":"text","text":"## Ringkasan pagi\n\n### Tindakan prioritas\n1. Andi: telepon dealer lewat jadwal."}],"usage":{"input_tokens":3000,"output_tokens":400}}`

func setup(t *testing.T) (*store.Store, gen.McpSchedule, *analyst.Runner) {
	t.Helper()
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rows, err := st.Q.ListMCPSchedules(ctx)
	if err != nil || len(rows) != 1 || rows[0].Name != "Ringkasan pagi" {
		t.Fatalf("migration schedule: %v %v", rows, err)
	}
	o := &orchestrator.Orchestrator{St: st, Clock: now, Router: &llm.Router{Primary: llm.NewFake(nil), St: st, Log: log}, Log: log}
	return st, rows[0], &analyst.Runner{St: st, Clock: now, Log: log, Orch: o, SessionKey: []byte("0123456789abcdef0123456789abcdef")}
}

func steps(t *testing.T, run *gen.McpScheduleRun) []analyst.Step {
	t.Helper()
	var s []analyst.Step
	if err := json.Unmarshal(run.Steps, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Without an API key the run still reports real numbers from the MCP tools, logged under the schedule's client.
func TestTemplateWithoutKey(t *testing.T) {
	st, sch, rn := setup(t)
	ctx := context.Background()
	run, err := rn.Run(ctx, sch.ID, nil, "manual", "Sam")
	if err != nil || run == nil {
		t.Fatalf("run: %v %v", run, err)
	}
	if run.Status != "template" || deref(run.Engine) != "template" || run.CostIdr != 0 {
		t.Fatalf("status %s engine %s cost %d err %v", run.Status, deref(run.Engine), run.CostIdr, deref(run.Error))
	}
	rep := deref(run.Report)
	for _, want := range []string{"## Ringkasan bisnis", "kunci Claude API belum diisi", "### Kondisi", "**Dealer**: 18", "### Penjualan 3 bulan", "### Per cabang"} {
		if !strings.Contains(rep, want) {
			t.Errorf("report misses %q:\n%s", want, rep)
		}
	}
	if s := steps(t, run); len(s) != 2 || s[0].Tool != "data_ringkasan" || s[0].Status != "ok" {
		t.Fatalf("steps %+v", s)
	}
	var who string
	if err := st.Pool.QueryRow(ctx, `select c.name from mcp_calls m join mcp_clients c on c.id = m.client_id where m.tool = 'data_ringkasan'`).Scan(&who); err != nil || who != "Terjadwal · Ringkasan pagi" {
		t.Fatalf("mcp_calls client %q %v", who, err)
	}
}

// With a key Claude drives the tools: only the schedule's scopes are offered, decide never; a tool outside the
// set is refused; tokens and cost are summed and logged in llm_calls.
func TestClaudeRun(t *testing.T) {
	st, sch, rn := setup(t)
	ctx := context.Background()
	sc := &script{turns: []string{
		toolTurn("1", "data_ringkasan", `{}`),
		toolTurn("2", "orchestrator_run", `{}`),
		toolTurn("3", "dealer_list", `{"status":"Key account","limit":5}`),
		finalTurn,
	}}
	rn.EnvKey = "sk-ant-test-key-000000000000"
	rn.NewModel = func(key string) analyst.Model {
		if key != rn.EnvKey {
			t.Errorf("key %q", key)
		}
		return sc
	}
	run, err := rn.Run(ctx, sch.ID, nil, "manual", "Sam")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "ok" || deref(run.Engine) != "claude" || !strings.HasPrefix(deref(run.Report), "## Ringkasan pagi") {
		t.Fatalf("run %s %s %q %v", run.Status, deref(run.Engine), deref(run.Report), deref(run.Error))
	}
	names := map[string]bool{}
	for _, tl := range sc.reqs[0].Tools {
		names[tl.OfTool.Name] = true
	}
	if !names["data_ringkasan"] || !names["analisis_dealer"] || names["actions_decide"] || names["orchestrator_run"] {
		t.Fatalf("tools offered %v", names)
	}
	s := steps(t, run)
	if len(s) != 3 || s[0].Status != "ok" || s[1].Status != "forbidden" || s[2].Status != "ok" {
		t.Fatalf("steps %+v", s)
	}
	b, _ := json.Marshal(sc.reqs[1].Messages[len(sc.reqs[1].Messages)-1])
	if !strings.Contains(string(b), `"tool_result"`) || !strings.Contains(string(b), "omzet_bln") {
		t.Fatalf("tool result not sent back: %.300s", b)
	}
	if run.TokensIn != 2000*3+3000 || run.TokensOut != 120*3+400 || run.CostIdr <= 0 {
		t.Fatalf("tokens %d/%d cost %d", run.TokensIn, run.TokensOut, run.CostIdr)
	}
	var n int
	_ = st.Pool.QueryRow(ctx, `select count(*) from llm_calls where purpose = 'mcp.schedule'`).Scan(&n)
	if n != 4 {
		t.Fatalf("llm_calls %d", n)
	}
}

// One scheduled slot runs once; over the daily budget the run falls back to the template.
func TestSlotOnceAndBudget(t *testing.T) {
	st, sch, rn := setup(t)
	ctx := context.Background()
	rn.EnvKey = "sk-ant-test-key-000000000000"
	sc := &script{turns: []string{finalTurn}}
	rn.NewModel = func(string) analyst.Model { return sc }
	slot := now.Now()
	first, err := rn.Run(ctx, sch.ID, &slot, "schedule", "jadwal")
	if err != nil || first == nil || first.Status != "ok" {
		t.Fatalf("first %+v %v", first, err)
	}
	if again, err := rn.Run(ctx, sch.ID, &slot, "schedule", "jadwal"); again != nil || err != nil {
		t.Fatalf("slot ran twice: %+v %v", again, err)
	}
	if err := analyst.SaveConfig(ctx, st.Q, analyst.Config{Model: "claude-haiku-4-5", DailyBudgetIDR: 1}, nil, now.Now()); err != nil {
		t.Fatal(err)
	}
	run, err := rn.Run(ctx, sch.ID, nil, "manual", "Sam")
	if err != nil || run.Status != "template" || !strings.Contains(deref(run.Report), "anggaran harian") {
		t.Fatalf("over budget: %s %v", run.Status, err)
	}
}

// The tick computes next_run_at first, fires a due slot once and moves on.
func TestDue(t *testing.T) {
	st, sch, _ := setup(t)
	ctx := context.Background()
	slots, err := analyst.Due(ctx, st.Q, now.Now())
	if err != nil || len(slots) != 0 {
		t.Fatalf("first tick fired: %v %v", slots, err)
	}
	got, _ := st.Q.GetMCPSchedule(ctx, sch.ID)
	if got.NextRunAt == nil || got.NextRunAt.In(clock.WIB).Format("Mon 15:04") != "Tue 07:00" {
		t.Fatalf("next %v", got.NextRunAt) // Monday 07.00 is now: the next is Tuesday 07.00
	}
	later := got.NextRunAt.Add(30 * time.Second)
	slots, _ = analyst.Due(ctx, st.Q, later)
	if len(slots) != 1 || !slots[0].At.Equal(*got.NextRunAt) {
		t.Fatalf("due %v", slots)
	}
	if again, _ := analyst.Due(ctx, st.Q, later); len(again) != 0 {
		t.Fatalf("fired twice %v", again)
	}
	// a slot missed for longer than MaxCatchUp is skipped, not run late
	got, _ = st.Q.GetMCPSchedule(ctx, sch.ID)
	if slots, _ := analyst.Due(ctx, st.Q, got.NextRunAt.Add(analyst.MaxCatchUp+time.Hour)); len(slots) != 0 {
		t.Fatalf("stale slot ran %v", slots)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
