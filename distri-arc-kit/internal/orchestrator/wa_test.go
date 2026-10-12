package orchestrator_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/store/gen"
)

// Analisa WhatsApp (ADR 0026): a full cycle reads the conversations next to the BigQuery data, keeps one row per
// conversation, puts the ones waiting too long into Rencana hari ini, and a re-run replaces instead of duplicating.
func TestCycleReadsWhatsApp(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	a, err := st.Q.LatestWAAnalysis(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.CycleID != rep.Cycle.ID || a.Threads == 0 || a.Source != "rules" || a.Overview != nil {
		t.Errorf("analysis %+v (the fake provider writes no AI overview)", a)
	}
	ins, err := st.Q.ListWAInsights(ctx, rep.Cycle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if int32(len(ins)) != a.Threads {
		t.Fatalf("%d insights for %d threads", len(ins), a.Threads)
	}
	var graha *gen.ListWAInsightsRow
	for i := range ins {
		if ins[i].Kind != "dealer" && ins[i].Kind != "new" {
			t.Errorf("group read: %s", ins[i].Title)
		}
		if deref(ins[i].DealerSlug) == "graha" {
			graha = &ins[i]
		}
	}
	if graha == nil || !graha.NeedsReply || graha.WaitingSince == nil || len(graha.SignalIds) == 0 {
		t.Fatalf("graha: %+v", graha)
	}
	var step *string
	for _, p := range rep.Plan {
		if p.Agent == orchestrator.AgentWA {
			if p.Link != "chat:"+graha.ThreadID.String() || !strings.Contains(p.Text, "[[dealer:graha|") || p.Autonomy != "approve" {
				t.Errorf("chat step %+v", p)
			}
			step = &p.Text
		}
	}
	if step == nil {
		t.Fatal("no Balas chat step")
	}
	stages, _ := st.Q.ListCycleStages(ctx, rep.Cycle.ID)
	if !strings.Contains(string(stages[0].Detail), "WhatsApp") || !strings.Contains(string(stages[0].Detail), "BigQuery") {
		t.Errorf("ingest detail %s", stages[0].Detail)
	}

	// the sales answers: the next cycle no longer asks for it
	dir, body, sent := "out", "Siap Pak, PO kami proses.", now.Now().Add(-time.Minute)
	id := "test-reply-" + uuid.NewString()
	if _, err := st.Q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &graha.ThreadID, WaMsgID: &id, Direction: &dir, Body: &body, SentAt: sent, Status: "sent"}); err != nil {
		t.Fatal(err)
	}
	rep2, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range rep2.Plan {
		if p.Agent == orchestrator.AgentWA {
			t.Errorf("answered chat still planned: %s", p.Text)
		}
	}
	var n int
	_ = st.Pool.QueryRow(ctx, "select count(*) from wa_insights where cycle_id = $1", rep2.Cycle.ID).Scan(&n)
	if n != len(ins) {
		t.Errorf("second cycle %d insights, want %d", n, len(ins))
	}
}

// modelStub answers like a real model: the WhatsApp reading gets a line per conversation, the rest its fallback.
type modelStub struct{ masked []string }

func (m *modelStub) Name() string  { return "stub" }
func (m *modelStub) Model() string { return "claude-sonnet-5-5" }
func (m *modelStub) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	if r.Purpose != "wa.summary" {
		return llm.Response{JSON: r.Fallback, Provider: "stub", Model: "claude-sonnet-5-5"}, nil
	}
	m.masked = append(m.masked, string(r.Input))
	var in struct {
		Threads []struct{ ID string } `json:"threads"`
	}
	_ = json.Unmarshal(r.Input, &in)
	var lines []map[string]any
	for _, t := range in.Threads {
		lines = append(lines, map[string]any{"id": t.ID, "summary": "Pelanggan menunggu konfirmasi dari sales.", "confidence": 0.8})
	}
	b, _ := json.Marshal(map[string]any{"overview": "Satu dealer menunggu balasan; sales perlu membalas hari ini.", "threads": lines})
	return llm.Response{JSON: b, Provider: "stub", Model: "claude-sonnet-5-5"}, nil
}

// With a model the overview and the conversation lines are stored as AI (source ai, confidence), and the people's
// names left the server masked.
func TestWhatsAppSummaryByModel(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	stub := &modelStub{}
	o.Router = &llm.Router{Primary: stub, St: st, Log: o.Log}
	rep, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	a, err := st.Q.LatestWAAnalysis(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Source != "ai" || deref(a.Overview) == "" {
		t.Fatalf("analysis source %s overview %q", a.Source, deref(a.Overview))
	}
	ins, _ := st.Q.ListWAInsights(ctx, rep.Cycle.ID)
	lines := 0
	for _, i := range ins {
		if i.Summary != nil {
			lines++
			if i.Confidence == nil || *i.Confidence != 80 {
				t.Errorf("confidence %v", i.Confidence)
			}
		}
	}
	if lines == 0 {
		t.Error("no AI line stored")
	}
	if len(stub.masked) != 1 || strings.Contains(stub.masked[0], "Pak Hasan") {
		t.Errorf("names not masked: %v", stub.masked)
	}
	var calls int
	_ = st.Pool.QueryRow(ctx, "select count(*) from llm_calls where purpose = 'wa.summary' and cycle_id = $1", rep.Cycle.ID).Scan(&calls)
	if calls != 1 {
		t.Errorf("%d wa.summary calls logged", calls)
	}
}

// A cycle that fails at Analisis (AI required, no model, nobody answers through MCP) still read WhatsApp in Ingest
// and still put the waiting chat into Rencana hari ini (ADR 0026).
func TestFailedCycleStillPlansChats(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `update policies set value = jsonb_set(value, '{require_ai}', 'true') where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	o.MCPWait = 500 * time.Millisecond
	if _, err := o.Run(ctx, all, sched); err == nil {
		t.Fatal("cycle without any AI answer should fail")
	}
	c, _ := st.Q.LatestCycle(ctx)
	if c.Status != "failed" {
		t.Fatalf("status %s", c.Status)
	}
	if a, err := st.Q.LatestWAAnalysis(ctx); err != nil || a.CycleID != c.ID || a.Threads == 0 {
		t.Fatalf("no WhatsApp reading on the failed cycle: %+v %v", a, err)
	}
	var n int
	_ = st.Pool.QueryRow(ctx, "select count(*) from plan_items where link like 'chat:%' and text_html like '%[[dealer:graha|%'").Scan(&n)
	if n != 1 {
		t.Fatalf("%d Balas chat steps for Graha", n)
	}
	// Claude answers through MCP afterwards: the reading of the failed cycle gets its AI lines
	raw, err := o.InputFor(ctx, c.ID, orchestrator.AgentWA)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Pak Hasan") {
		t.Fatal("names not masked in the MCP input")
	}
	var in struct {
		Threads []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"threads"`
	}
	_ = json.Unmarshal(raw, &in)
	if len(in.Threads) == 0 {
		t.Fatalf("input %s", raw)
	}
	person, _, _ := strings.Cut(in.Threads[0].Title, " · ") // a placeholder like <PIC_1>
	ans := orchestrator.ParseWAAnswer("Satu dealer menunggu balasan sejak kemarin.", []map[string]any{{"id": in.Threads[0].ID, "summary": person + " menunggu konfirmasi PO.", "confidence": 0.7}})
	if _, err := o.WASubmit(ctx, c.ID, ans, nil); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Q.LatestWAAnalysis(ctx)
	if a.Source != "ai" {
		t.Errorf("source %s", a.Source)
	}
	var sum string
	_ = st.Pool.QueryRow(ctx, "select summary from wa_insights where cycle_id = $1 and thread_id = $2", c.ID, uuid.MustParse(in.Threads[0].ID)).Scan(&sum)
	if strings.Contains(sum, "<PIC") || sum == "" {
		t.Errorf("summary not unmasked: %q", sum)
	}
	bad := orchestrator.ParseWAAnswer("x", []map[string]any{{"id": uuid.NewString(), "summary": "y", "confidence": 0.5}})
	if _, err := o.WASubmit(ctx, c.ID, bad, nil); err == nil {
		t.Error("unknown conversation accepted")
	}
}
