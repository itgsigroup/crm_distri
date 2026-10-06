package orchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/proposals"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/testdb"
)

var now = clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB))

func setup(t *testing.T) (*store.Store, *orchestrator.Orchestrator) {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, &orchestrator.Orchestrator{St: st, Clock: now, Router: &llm.Router{Primary: llm.NewFake(nil), St: st, Log: log}, Log: log}
}

var all = domain.Scope{Kind: "all"}
var sched = domain.Trigger{Source: "schedule", Via: "api"}

// docs/stages/06 acceptance on the seed: six stages done, the two conflicts, and the plan of the mockup
// (8 steps: 3 auto, 5 approve).
func TestCycleOnSeed(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rep.Conflicts {
		t.Logf("conflict %-24s %-22v %s ↔ %s · %s → %s", c.Rule, deref(c.DealerID), c.AgentA, c.AgentB, c.Title, c.Resolution)
	}
	for _, p := range rep.Plan {
		t.Logf("plan %s %-7s %-8s %-28s %s %v", p.Time, p.Autonomy, p.Status, p.Agent, p.Text, p.Keys)
	}
	if rep.Cycle.Status != "done" {
		t.Fatalf("status %s", rep.Cycle.Status)
	}
	stages, _ := st.Q.ListCycleStages(ctx, rep.Cycle.ID)
	if len(stages) != 6 {
		t.Fatalf("%d stages", len(stages))
	}
	for _, s := range stages {
		if s.Status != "done" {
			t.Errorf("stage %s %s", s.Stage, s.Status)
		}
	}
	has := func(rule, dealer string) bool {
		for _, c := range rep.Conflicts {
			if c.Rule == rule && c.DealerID != nil && *c.DealerID == dealer && c.Visible {
				return true
			}
		}
		return false
	}
	if !has(orchestrator.RuleCreditOverStock, "mitra") {
		t.Error("credit_over_stock Mitra Jaya missing")
	}
	if !has(orchestrator.RuleCollectFirst, "nusa") {
		t.Error("collect_before_followup Nusa Teknik missing")
	}
	if !has(orchestrator.RuleMarginFloor, "indo") {
		t.Error("margin_floor Indo Vision missing")
	}
	auto, approve := 0, 0
	for _, p := range rep.Plan {
		if p.Autonomy == "auto" {
			auto++
		} else {
			approve++
		}
	}
	if len(rep.Plan) != 8 || auto != 3 || approve != 5 {
		t.Errorf("plan %d items (%d auto, %d approve), want 8 (3, 5)", len(rep.Plan), auto, approve)
	}
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

// Re-running in the same hour does not duplicate proposals and rebuilds the same plan.
func TestCycleIdempotent(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	first, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	var n1, n2 int
	_ = st.Pool.QueryRow(ctx, "select count(*) from proposals").Scan(&n1)
	second, err := o.Run(ctx, all, domain.Trigger{Source: "manual", Via: "api", By: "sam@gsi.co.id"})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Pool.QueryRow(ctx, "select count(*) from proposals").Scan(&n2)
	if n1 != n2 || second.Stored != 0 {
		t.Fatalf("proposals %d → %d (stored %d)", n1, n2, second.Stored)
	}
	if len(first.Plan) != len(second.Plan) {
		t.Fatalf("plan %d → %d", len(first.Plan), len(second.Plan))
	}
	for i := range first.Plan {
		if first.Plan[i].Text != second.Plan[i].Text {
			t.Errorf("plan step %d changed: %q → %q", i, first.Plan[i].Text, second.Plan[i].Text)
		}
	}
	if *second.Cycle.Number != *first.Cycle.Number+1 || deref(second.Cycle.SignalsCount) != 0 {
		t.Errorf("second cycle #%d signals %d", deref(second.Cycle.Number), deref(second.Cycle.SignalsCount))
	}
}

// A rejection yesterday suppresses the same (agent, dealer, kind) today.
func TestSuppressionFromCalibration(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	var dealer string
	_ = st.Pool.QueryRow(ctx, "select id from dealers where slug = 'prima'").Scan(&dealer)
	if _, err := st.Pool.Exec(ctx, `insert into calibration_events (agent, dealer_id, kind, decision, reason, suppress_until, created_at)
		values ('AI Follow-up', $1, 'followup', 'rejected', 'Tidak tepat waktu', $2, $3)`, dealer, now.Now().AddDate(0, 0, 13), now.Now().AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Run(ctx, all, sched); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := st.Pool.QueryRow(ctx, "select status from proposals where dealer_id = $1 and kind = 'followup'", dealer).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "suppressed" {
		t.Fatalf("follow-up Prima %s, want suppressed", status)
	}
}

// Only one cycle at a time: a second request while one is queued/running gets ErrRunning (→ 409).
func TestOneCycleAtATime(t *testing.T) {
	_, o := setup(t)
	ctx := context.Background()
	if _, err := o.Queue(ctx, nil, all, sched); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Queue(ctx, nil, domain.Scope{Kind: "screen", ID: "orbit"}, domain.Trigger{Source: "manual"}); !errors.Is(err, orchestrator.ErrRunning) {
		t.Fatalf("second queue: %v", err)
	}
	if _, err := o.Run(ctx, all, sched); !errors.Is(err, orchestrator.ErrRunning) {
		t.Fatalf("run while queued: %v", err)
	}
}

// Concurrent runs: exactly one succeeds.
func TestConcurrentRuns(t *testing.T) {
	_, o := setup(t)
	ctx := context.Background()
	errs := make(chan error, 2)
	for range 2 {
		go func() { _, err := o.Run(ctx, all, sched); errs <- err }()
	}
	ok, busy := 0, 0
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, orchestrator.ErrRunning):
			busy++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || busy != 1 {
		t.Fatalf("ok %d busy %d", ok, busy)
	}
}

// Scoped runs (Analisis ulang di Orbit) run follow-up and credit only and leave the plan alone.
func TestScopedRun(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := o.Run(ctx, all, sched); err != nil {
		t.Fatal(err)
	}
	rep, err := o.Run(ctx, domain.Scope{Kind: "screen", ID: "orbit"}, domain.Trigger{Source: "manual", Via: "api"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Stored != 0 {
		t.Errorf("scoped run stored %d proposals (the Lampu follow-up belongs to the LED bundle)", rep.Stored)
	}
	cid := rep.Cycle.ID
	runs, _ := st.Q.ListAgentRuns(ctx, &cid)
	if len(runs) != 2 {
		t.Fatalf("%d agent runs", len(runs))
	}
	if !strings.Contains(deref(rep.Cycle.Note), "Analisis ulang orbit") {
		t.Errorf("note %q", deref(rep.Cycle.Note))
	}
	plan, _ := st.Q.ListPlan(ctx, clock.Today(now.Now()))
	if len(plan) != 8 {
		t.Errorf("plan changed by a scoped run: %d", len(plan))
	}
}

// docs/stages/09 acceptance on the seed: bundle LED P5, collect H-3 Nusa (auto), installment Mitra (approve),
// new dealer Toko Mandiri Pati (approve). The bundle reaches the dealers the glossary rule allows (OPEN-QUESTIONS).
func TestStage09AgentsInCycle(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	cid := rep.Cycle.ID
	rows, err := st.Q.ProposalsOfCycle(ctx, &cid)
	if err != nil {
		t.Fatal(err)
	}
	find := func(kind, dealer string) (string, string, bool) {
		for _, p := range rows {
			if p.Kind == kind && (dealer == "" || deref(p.DealerSlug) == dealer || strings.Contains(p.Title, dealer)) {
				return p.Autonomy, p.Status, true
			}
		}
		return "", "", false
	}
	if a, _, ok := find("push_stock", "Modul LED P5"); !ok || a != "approve" {
		t.Errorf("push_stock LED P5: %v %s", ok, a)
	}
	if a, _, ok := find("collect", "nusa"); !ok || a != "auto" {
		t.Errorf("collect Nusa: %v %s", ok, a)
	}
	if a, s, ok := find("installment", "mitra"); !ok || a != "approve" || s != "proposed" {
		t.Errorf("installment Mitra: %v %s %s", ok, a, s)
	}
	if a, _, ok := find("new_dealer", "Toko Mandiri"); !ok || a != "approve" {
		t.Errorf("new_dealer Mandiri: %v %s", ok, a)
	}
	if len(rep.Plan) != 8 {
		t.Errorf("plan %d steps", len(rep.Plan))
	}
	titles := map[string]string{}
	for _, p := range rows {
		titles[p.Kind] += p.Title + "; "
	}
	if !strings.Contains(titles["transfer"], "Transfer 40 Kamera IP 4MP dome Jakarta → Semarang") {
		t.Errorf("transfers: %s", titles["transfer"])
	}
	if !strings.Contains(titles["po_request"], "Power supply 12V 10A untuk Surabaya") {
		t.Errorf("PO requests: %s", titles["po_request"])
	}
}

// docs/stages/10 acceptance: three rejections of HDD offers to tier C dealers with the same reason become a lesson,
// and a similar proposal does not come back for 14 days.
func TestLessonFromRejections(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	var ceo uuid.UUID
	_ = st.Pool.QueryRow(ctx, "select id from sales_users where role = 'ceo'").Scan(&ceo)
	who := proposals.Decider{SalesUserID: ceo, Name: "Sam", Role: "ceo", Email: "sam@gsi.co.id"}
	var sig uuid.UUID
	_ = st.Pool.QueryRow(ctx, "select id from signals limit 1").Scan(&sig)
	for _, slug := range []string{"cahaya", "citra", "megah"} {
		var did uuid.UUID
		_ = st.Pool.QueryRow(ctx, "select id from dealers where slug = $1", slug).Scan(&did)
		p := domain.Proposal{Agent: "AI Stok", DealerID: &did, Kind: domain.KindPushStock, Title: "Bundle HDD 4TB · " + slug, Why: "stok menua", Confidence: 0.8,
			SignalIDs: []uuid.UUID{sig}, Autonomy: "approve", Payload: map[string]any{"name": "HDD 4TB surveillance"}, DedupeKey: "test:hdd:" + slug}
		id, err := proposals.Insert(ctx, st.Q, p, "proposed", nil, now.Now().Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := proposals.Decide(ctx, st, nil, now, false, id, who, proposals.Decision{Decision: "reject", Reason: "tidak_sesuai_kebijakan", ReasonText: "mereka beli di marketplace"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := o.Run(ctx, all, sched); err != nil {
		t.Fatal(err)
	}
	ls, err := st.Q.ListLessons(ctx, 10)
	if err != nil || len(ls) != 1 {
		t.Fatalf("lessons %v %v", ls, err)
	}
	if ls[0].Text != "AI Stok tidak lagi menawarkan HDD ke dealer tier C (ditolak 3×: “mereka beli di marketplace”)." {
		t.Fatalf("lesson %q", ls[0].Text)
	}
	if ls[0].SuppressUntil == nil || ls[0].SuppressUntil.Format("2006-01-02") != now.Now().AddDate(0, 0, 14).Format("2006-01-02") {
		t.Fatalf("until %v", ls[0].SuppressUntil)
	}
}

// docs/stages/10: after a cycle the Mitra memo keeps the essentials and every sentence has a source of Mitra;
// the brief carries signal ids on every point that quotes dealers.
func TestMemoAndBriefAfterCycle(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := o.Run(ctx, all, sched); err != nil {
		t.Fatal(err)
	}
	var memo string
	var raw []byte
	var mitra uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id, memo, memo_sentences from dealers where slug = 'mitra'").Scan(&mitra, &memo, &raw); err != nil {
		t.Fatal(err)
	}
	var sentences []struct {
		Text      string      `json:"text"`
		SignalIDs []uuid.UUID `json:"signal_ids"`
	}
	if err := json.Unmarshal(raw, &sentences); err != nil || len(sentences) < 4 {
		t.Fatalf("sentences %d %v", len(sentences), err)
	}
	for _, s := range sentences {
		var n int
		_ = st.Pool.QueryRow(ctx, "select count(*) from signals where id = any($1) and dealer_id = $2", s.SignalIDs, mitra).Scan(&n)
		if len(s.SignalIDs) == 0 || n != len(s.SignalIDs) {
			t.Errorf("sentence %q: %d of %d sources are Mitra signals", s.Text, n, len(s.SignalIDs))
		}
	}
	for _, want := range []string{"tempo", "Over limit", "lewat jadwal"} {
		if !strings.Contains(memo, want) {
			t.Errorf("memo misses %q: %s", want, memo)
		}
	}
	b, err := st.Q.GetBrief(ctx, clock.Today(now.Now()))
	if err != nil {
		t.Fatal(err)
	}
	var br struct {
		Points []struct {
			Kind      string   `json:"kind"`
			Text      string   `json:"text"`
			SignalIDs []string `json:"signal_ids"`
		} `json:"points"`
	}
	_ = json.Unmarshal(b.Brief, &br)
	for _, p := range br.Points {
		if p.Text == "" || len(p.SignalIDs) == 0 {
			t.Errorf("brief point %s: text %q, %d sources", p.Kind, p.Text, len(p.SignalIDs))
		}
	}
	if !strings.Contains(br.Points[2].Text, "[[dealer:graha|Graha Sentosa]] minta rilis Rp 35 jt") {
		t.Errorf("credit point: %s", br.Points[2].Text)
	}
}
