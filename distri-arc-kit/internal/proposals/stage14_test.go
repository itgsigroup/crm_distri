package proposals_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/outbox"
	"distri-arc/internal/pilot"
	"distri-arc/internal/proposals"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

func startPilot(t *testing.T, st *store.Store, mode string) pilot.Service {
	t.Helper()
	svc := pilot.Service{St: st, Clock: now}
	if _, err := svc.SetMode(context.Background(), "shadow", "Semarang", nil); err != nil {
		t.Fatal(err)
	}
	if mode != "shadow" {
		if _, err := svc.SetMode(context.Background(), mode, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	return svc
}

// docs/stages/14: in shadow mode the Orchestrator proposes but nothing is automatic and nothing is sent; decisions
// still calibrate, the audit stays clean.
func TestShadowModeSendsNothing(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	svc := startPilot(t, st, "shadow")
	res := run(t, r, "all")
	for _, p := range res.Stored {
		if p.Autonomy == "auto" || p.Status != "proposed" {
			t.Fatalf("shadow: %s %q is %s/%s", p.Kind, p.Title, p.Autonomy, p.Status)
		}
	}
	so := find(res, "so_draft", "Sinar")
	if so == nil {
		t.Fatal("SO draft Sinar still proposed for a human")
	}
	p := find(res, "followup", "Prima")
	out, err := proposals.Decide(ctx, st, nil, now, true, p.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	if err != nil || !strings.Contains(out.Result, "mode bayangan") {
		t.Fatalf("decide %+v %v", out, err)
	}
	var status string
	_ = st.Pool.QueryRow(ctx, "select status from outbox where id = $1", *out.OutboxID).Scan(&status)
	if status != "shadow" {
		t.Fatalf("outbox %s", status)
	}
	var notes, bubbles int
	_ = st.Pool.QueryRow(ctx, "select count(*) from outbox where channel = 'odoo_note'").Scan(&notes)
	_ = st.Pool.QueryRow(ctx, "select count(*) from chat_messages where proposal_id = $1 and status = 'pending'", p.ID).Scan(&bubbles)
	if notes != 0 || bubbles != 0 {
		t.Fatalf("odoo notes %d, pending bubbles %d", notes, bubbles)
	}
	fake := wa.NewFake()
	if _, err := outbox.NewSender(st, fake, now, outbox.Rules{}).Send(ctx, *out.OutboxID); err != nil || len(fake.Sent()) != 0 {
		t.Fatalf("sender delivered in shadow: %v %d", err, len(fake.Sent()))
	}
	if _, err := proposals.ApproveBySystem(ctx, st, nil, now, so.ID); !errors.Is(err, proposals.ErrShadow) {
		t.Fatalf("system approval in shadow: %v", err)
	}
	from, to, _ := svc.Period(ctx)
	rep, err := svc.Build(ctx, from, to)
	if err != nil || !rep.AuditOK || rep.Mode != "shadow" || rep.Day != 1 {
		t.Fatalf("report %+v %v", rep.Audit, err)
	}
	// a row delivered during the shadow weeks is a violation
	_, _ = st.Pool.Exec(ctx, "update outbox set status = 'sent', sent_at = $2 where id = $1", *out.OutboxID, now.Now())
	rep, _ = svc.Build(ctx, from, to)
	if rep.AuditOK || rep.SentInShadow != 1 {
		t.Fatalf("shadow send not caught: %+v", rep.Audit)
	}
}

// The audit catches a stored DM between internal numbers and a send without a decision.
func TestPilotAuditViolations(t *testing.T) {
	st, _ := setup(t)
	ctx := context.Background()
	svc := startPilot(t, st, "live")
	_, _ = st.Pool.Exec(ctx, "insert into internal_numbers (wa_number, label) values ('6281111111111', 'Gudang')")
	var thread uuid.UUID
	_ = st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title) values ('dealer', '6281111111111@s.whatsapp.net', 'x') returning id").Scan(&thread)
	if _, err := st.Q.InsertChatMessage(ctx, chatParams(thread, now.Now())); err != nil {
		t.Fatal(err)
	}
	var prop uuid.UUID
	if err := st.Pool.QueryRow(ctx, `insert into proposals (agent, kind, title, why, confidence, signal_ids, autonomy, status)
			values ('AI Order', 'followup', 't', 'w', 0.9, array[(select id from signals limit 1)], 'approve', 'proposed') returning id`).Scan(&prop); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "insert into outbox (proposal_id, channel, status, sent_at) values ($1, 'wa', 'sent', $2)", prop, now.Now()); err != nil {
		t.Fatal(err)
	}
	from, to, _ := svc.Period(ctx)
	rep, err := svc.Build(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, c := range rep.Audit {
		got[c.Key] = c.Violations
	}
	if got["internal_dm"] != 1 || got["unapproved_sends"] != 1 || rep.AuditOK {
		t.Fatalf("audit %v", got)
	}
	csv := string(pilot.CSV(rep))
	if !strings.Contains(csv, "agen,saran,disetujui") || !strings.Contains(csv, "audit_internal_dm,1,0") {
		t.Fatalf("csv:\n%s", csv)
	}
}

// Two complete weeks with ≥ 5 decisions at ≥ 80% open an agent; in live mode only unlocked agents act alone.
func TestUnlockRule(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	svc := startPilot(t, st, "live")
	ceo := sam(t, st)
	decide := func(agent string, day time.Time, accepted, rejected int) {
		for i := range accepted + rejected {
			status := "approved"
			if i >= accepted {
				status = "rejected"
			}
			if _, err := st.Pool.Exec(ctx, `insert into proposals (agent, kind, title, why, confidence, signal_ids, autonomy, status, decided_by, decided_at, created_at)
				values ($1, 'so_draft', 'x', 'w', 0.9, array[(select id from signals limit 1)], 'approve', $2, $3, $4, $4)`, agent, status, ceo.SalesUserID, day.Add(time.Duration(i)*time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
	}
	thisWeek := pilot.Week(now.Now())
	decide("AI Order", thisWeek.AddDate(0, 0, -7), 5, 1) // 83%
	if _, err := svc.Unlock(ctx, "AI Order", nil); !errors.Is(err, pilot.ErrNotEligible) {
		t.Fatalf("one week must not unlock: %v", err)
	}
	decide("AI Order", thisWeek.AddDate(0, 0, -14), 4, 1) // 80%
	decide("AI Follow-up", thisWeek.AddDate(0, 0, -7), 3, 3)
	decide("AI Follow-up", thisWeek.AddDate(0, 0, -14), 6, 0)
	from, to, _ := svc.Period(ctx)
	rep, _ := svc.Build(ctx, from.AddDate(0, 0, -21), to)
	el := map[string]pilot.AgentStat{}
	for _, a := range rep.Agents {
		el[a.Agent] = a
	}
	if !el["AI Order"].Eligible || el["AI Follow-up"].Eligible {
		t.Fatalf("eligibility: order %+v follow-up %+v", el["AI Order"].EligibleNote, el["AI Follow-up"].EligibleNote)
	}
	pp, err := svc.Unlock(ctx, "AI Order", nil)
	if err != nil || len(pp.Unlocked) != 1 {
		t.Fatalf("unlock %v %v", pp, err)
	}
	_, _ = st.Pool.Exec(ctx, "delete from proposals where title = 'x'")
	res := run(t, r, "all")
	if so := find(res, "so_draft", "Sinar"); so == nil || so.Autonomy != "auto" {
		t.Fatalf("unlocked AI Order SO draft: %+v", so)
	}
	for _, p := range res.Stored {
		if p.Autonomy == "auto" && p.Kind != "so_draft" {
			t.Fatalf("locked agent acted alone: %s %q", p.Kind, p.Title)
		}
	}
}

// A dealer that went lewat jadwal, got a decided proposal and ordered again counts as caught before churn.
func TestDriftCaughtBeforeChurn(t *testing.T) {
	st, _ := setup(t)
	ctx := context.Background()
	svc := startPilot(t, st, "shadow")
	var dealer uuid.UUID
	_ = st.Pool.QueryRow(ctx, "select id from dealers where branch = 'Semarang' order by slug limit 1").Scan(&dealer)
	day := clock.Today(now.Now()).AddDate(0, 0, -10)
	_, _ = st.Pool.Exec(ctx, "insert into dealer_metrics_daily (dealer_id, as_of, status) values ($1, $2, 'At risk') on conflict (dealer_id, as_of) do update set status = 'At risk'", dealer, day)
	_, _ = st.Pool.Exec(ctx, `insert into proposals (agent, dealer_id, kind, title, why, confidence, signal_ids, autonomy, status, decided_at)
		values ('AI Follow-up', $1, 'followup', 'f', 'w', 0.9, array[(select id from signals limit 1)], 'approve', 'approved', $2)`, dealer, day.Add(time.Hour))
	_, _ = st.Pool.Exec(ctx, "insert into orders (dealer_id, number, state, ordered_at, total, lines) values ($1, 'T1', 'order', $2, 1000, '[]')", dealer, day.AddDate(0, 0, 3))
	rep, err := svc.Build(ctx, day.AddDate(0, 0, -1), clock.Today(now.Now()).AddDate(0, 0, 1))
	if err != nil || rep.AtRisk < 1 || rep.Caught < 1 {
		t.Fatalf("caught %d of %d: %v", rep.Caught, rep.AtRisk, err)
	}
}

func chatParams(thread uuid.UUID, at time.Time) gen.InsertChatMessageParams {
	id, dir, from, body := "dm-1", "in", "6281111111111", "stok aman?"
	return gen.InsertChatMessageParams{ThreadID: &thread, WaMsgID: &id, Direction: &dir, FromNumber: &from, Body: &body, SentAt: at, Status: "received"}
}
