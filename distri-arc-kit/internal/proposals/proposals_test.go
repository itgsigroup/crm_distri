package proposals_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/llm"
	"distri-arc/internal/outbox"
	"distri-arc/internal/proposals"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

var now = clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB))

func setup(t *testing.T) (*store.Store, *proposals.Runner) {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, &proposals.Runner{St: st, Clock: now, Router: &llm.Router{Primary: llm.NewFake(nil), St: st, Log: log}, Log: log}
}

func find(res proposals.Result, kind, dealerPart string) *proposals.Stored {
	for i, s := range res.Stored {
		if s.Proposal.Kind == kind && strings.Contains(s.Proposal.Title, dealerPart) {
			return &res.Stored[i]
		}
	}
	return nil
}

// docs/stages/05 acceptance: ≥ 8 proposals incl. credit_release Graha (DP 50%), follow-up Prima/Jaya/Borneo,
// price_counter Indo Vision (below the floor → approve) and an automatic SO draft for Toko Sinar.
func TestAgentsOnSeed(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res, err := r.Run(ctx, proposals.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stored) < 8 {
		t.Fatalf("only %d proposals", len(res.Stored))
	}
	g := find(res, "credit_release", "Graha")
	if g == nil || !strings.Contains(g.Proposal.Title, "DP 50%") || g.Proposal.Autonomy != "approve" {
		t.Fatalf("credit_release Graha: %+v", g)
	}
	for _, d := range []string{"Prima", "Jaya Abadi", "Borneo"} {
		if f := find(res, "followup", d); f == nil || f.Proposal.DueLabel != "Besok" {
			t.Errorf("follow-up %s missing: %+v", d, f)
		}
	}
	pc := find(res, "price_counter", "Indo Vision")
	if pc == nil || pc.Proposal.Autonomy != "approve" || pc.Proposal.Payload["margin_asked"].(float64) >= 9 {
		t.Fatalf("price counter: %+v", pc)
	}
	so := find(res, "so_draft", "Sinar")
	if so == nil || so.Status != "approved" || so.Proposal.Autonomy != "auto" || !strings.Contains(so.Proposal.Title, "Rp 18,4 jt") {
		t.Fatalf("so draft: %+v", so)
	}
	for _, s := range res.Stored {
		if len(s.Proposal.SignalIDs) == 0 || s.Proposal.Confidence <= 0 {
			t.Errorf("%s without provenance/confidence", s.Proposal.Title)
		}
		if s.Proposal.Preview != "" && strings.Count(s.Proposal.Preview, ". ")+strings.Count(s.Proposal.Preview, "? ") > 3 {
			t.Errorf("draft longer than 3 sentences: %s", s.Proposal.Preview)
		}
	}
	// idempotent: a second run stores nothing new
	res2, err := r.Run(ctx, proposals.RunOptions{})
	if err != nil || len(res2.Stored) != 0 {
		t.Fatalf("second run stored %d (err %v)", len(res2.Stored), err)
	}
	_ = st
}

func sam(t *testing.T, st *store.Store) proposals.Decider {
	var id uuid.UUID
	if err := st.Pool.QueryRow(context.Background(), "select id from sales_users where role = 'ceo'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return proposals.Decider{SalesUserID: id, Name: "Sam Setiadi", Role: "ceo", Email: "sam@gsi.co.id"}
}

// Approving the Prima follow-up queues a WhatsApp from Dewi's number that the sender delivers.
func TestApproveSendsThroughOutbox(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res, _ := r.Run(ctx, proposals.RunOptions{})
	p := find(res, "followup", "Prima")
	out, err := proposals.Decide(ctx, st, nil, now, false, p.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	if err != nil || out.OutboxID == nil || out.Status != "approved" {
		t.Fatalf("decide: %+v %v", out, err)
	}
	fake := wa.NewFake()
	rules := outbox.Rules{DailyCapOverride: 12}
	if _, err := outbox.NewSender(st, fake, now, rules).Send(ctx, *out.OutboxID); err != nil {
		t.Fatal(err)
	}
	if len(fake.Sent()) != 1 || fake.Sent()[0].Account != "6281534509032" || !strings.Contains(fake.Sent()[0].Text, "Pak Bayu") {
		t.Fatalf("sent %+v", fake.Sent())
	}
	var status string
	_ = st.Pool.QueryRow(ctx, "select status from proposals where id = $1", p.ID).Scan(&status)
	if status != "executed" {
		t.Fatalf("status %s", status)
	}
	var audit int
	_ = st.Pool.QueryRow(ctx, "select count(*) from audit_log where action = 'proposal.decide' and actor = 'sam@gsi.co.id'").Scan(&audit)
	if audit != 1 {
		t.Fatal("decision not audited")
	}
}

// Rejecting with a reason is calibration: the same agent/dealer/kind is suppressed for 14 days.
func TestRejectSuppresses(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res, _ := r.Run(ctx, proposals.RunOptions{})
	p := find(res, "followup", "Megah")
	if _, err := proposals.Decide(ctx, st, nil, now, false, p.ID, sam(t, st), proposals.Decision{Decision: "reject"}); !errors.Is(err, proposals.ErrReason) {
		t.Fatalf("reject without reason: %v", err)
	}
	if _, err := proposals.Decide(ctx, st, nil, now, false, p.ID, sam(t, st), proposals.Decision{Decision: "reject", Reason: "tidak_sesuai_kebijakan", ReasonText: "jangan sapa toko kecil"}); err != nil {
		t.Fatal(err)
	}
	// next day the agent proposes again → suppressed
	next := &proposals.Runner{St: st, Clock: clock.Fixed(time.Date(2026, 10, 6, 9, 0, 0, 0, clock.WIB)), Router: r.Router}
	res2, err := next.Run(ctx, proposals.RunOptions{Agent: "AI Follow-up"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Suppressed == 0 {
		t.Fatalf("no suppression: %+v", res2)
	}
	if f := find(res2, "followup", "Megah"); f == nil || f.Status != "suppressed" {
		t.Fatalf("Megah follow-up not suppressed: %+v", f)
	}
}

// credit_release needs the CEO.
func TestReleaseNeedsCEO(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res, _ := r.Run(ctx, proposals.RunOptions{})
	g := find(res, "credit_release", "Graha")
	who := sam(t, st)
	who.Role = "sales"
	if _, err := proposals.Decide(ctx, st, nil, now, false, g.ID, who, proposals.Decision{Decision: "approve"}); err == nil || !strings.Contains(err.Error(), "CEO") {
		t.Fatalf("sales approved a release: %v", err)
	}
	out, err := proposals.Decide(ctx, st, nil, now, false, g.ID, sam(t, st), proposals.Decision{Decision: "option", Option: "hold"})
	if err != nil || out.Status != "executed" || out.OutboxID != nil {
		t.Fatalf("hold: %+v %v", out, err)
	}
}
