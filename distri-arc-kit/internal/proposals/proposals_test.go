package proposals_test

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
	"distri-arc/internal/outbox"
	"distri-arc/internal/proposals"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

var now = clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB))

type stored struct {
	ID       uuid.UUID
	Kind     string
	Title    string
	Status   string
	Autonomy string
	Proposal struct {
		DueLabel   string
		Preview    string
		Confidence float64
		SignalIDs  []uuid.UUID
		Payload    map[string]any
		Autonomy   string
		Title      string
	}
}

type result struct {
	Stored     []stored
	Suppressed int
}

func setup(t *testing.T) (*store.Store, *orchestrator.Orchestrator) {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, &orchestrator.Orchestrator{St: st, Clock: now, Router: &llm.Router{Primary: llm.NewFake(nil), St: st, Log: log}, Log: log}
}

// run executes one Orchestrator cycle and returns what it stored.
func run(t *testing.T, o *orchestrator.Orchestrator, scope string) result {
	t.Helper()
	sc, _ := domain.ParseScope(scope)
	rep, err := o.Run(context.Background(), sc, domain.Trigger{Source: "manual", Via: "api"})
	if err != nil {
		t.Fatal(err)
	}
	cid := rep.Cycle.ID
	rows, err := o.St.Q.ProposalsOfCycle(context.Background(), &cid)
	if err != nil {
		t.Fatal(err)
	}
	var res result
	for _, r := range rows {
		s := stored{ID: r.ID, Kind: r.Kind, Title: r.Title, Status: r.Status, Autonomy: r.Autonomy}
		s.Proposal.DueLabel, s.Proposal.Preview, s.Proposal.SignalIDs, s.Proposal.Autonomy, s.Proposal.Title = deref(r.DueLabel), deref(r.Preview), r.SignalIds, r.Autonomy, r.Title
		s.Proposal.Confidence = r.Confidence
		_ = json.Unmarshal(r.Payload, &s.Proposal.Payload)
		if r.Status == "suppressed" {
			res.Suppressed++
		}
		res.Stored = append(res.Stored, s)
	}
	return res
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

func find(res result, kind, dealerPart string) *stored {
	for i, s := range res.Stored {
		if s.Kind == kind && strings.Contains(s.Title, dealerPart) {
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
	res := run(t, r, "all")
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
	if so == nil || (so.Status != "approved" && so.Status != "executed") || so.Proposal.Autonomy != "auto" || !strings.Contains(so.Proposal.Title, "Rp 18,4 jt") {
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
	if res2 := run(t, r, "all"); len(res2.Stored) != 0 {
		t.Fatalf("second run stored %d", len(res2.Stored))
	}
	_ = st
	_ = ctx
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
	res := run(t, r, "all")
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
	res := run(t, r, "all")
	p := find(res, "followup", "Megah")
	if _, err := proposals.Decide(ctx, st, nil, now, false, p.ID, sam(t, st), proposals.Decision{Decision: "reject"}); !errors.Is(err, proposals.ErrReason) {
		t.Fatalf("reject without reason: %v", err)
	}
	if _, err := proposals.Decide(ctx, st, nil, now, false, p.ID, sam(t, st), proposals.Decision{Decision: "reject", Reason: "tidak_sesuai_kebijakan", ReasonText: "jangan sapa toko kecil"}); err != nil {
		t.Fatal(err)
	}
	// next day the agent proposes again → suppressed
	next := &orchestrator.Orchestrator{St: st, Clock: clock.Fixed(time.Date(2026, 10, 6, 9, 0, 0, 0, clock.WIB)), Router: r.Router, Log: r.Log}
	res2 := run(t, next, "agent:AI Follow-up")
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
	res := run(t, r, "all")
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

// Approving a bundle creates one WhatsApp draft per dealer (same decision), each through the outbox.
func TestApproveBundleSpawnsDrafts(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res := run(t, r, "all")
	b := find(res, "push_stock", "Modul LED P5")
	if b == nil {
		t.Fatal("no LED P5 bundle")
	}
	out, err := proposals.Decide(ctx, st, nil, now, false, b.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	if err != nil || out.Status != "executed" {
		t.Fatalf("decide: %+v %v", out, err)
	}
	var children, rows, decided int
	_ = st.Pool.QueryRow(ctx, "select count(*) from proposals where payload->>'parent' = $1", b.ID.String()).Scan(&children)
	_ = st.Pool.QueryRow(ctx, "select count(*) from outbox o join proposals p on p.id = o.proposal_id where p.payload->>'parent' = $1 and o.channel = 'wa'", b.ID.String()).Scan(&rows)
	_ = st.Pool.QueryRow(ctx, "select count(*) from proposals where payload->>'parent' = $1 and decided_by is not null", b.ID.String()).Scan(&decided)
	if children == 0 || rows != children || decided != children {
		t.Fatalf("children %d outbox %d decided %d", children, rows, decided)
	}
}

// A new dealer is recorded for Odoo (written in stage 12), nothing is sent to the number yet.
func TestApproveNewDealer(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res := run(t, r, "all")
	nd := find(res, "new_dealer", "Toko Mandiri")
	if nd == nil {
		t.Fatal("no new dealer proposal")
	}
	out, err := proposals.Decide(ctx, st, nil, now, false, nd.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	if err != nil || out.OutboxID == nil {
		t.Fatalf("decide: %+v %v", out, err)
	}
	var channel string
	_ = st.Pool.QueryRow(ctx, "select channel from outbox where id = $1", *out.OutboxID).Scan(&channel)
	if channel != "odoo_note" {
		t.Fatalf("channel %s", channel)
	}
}

// "Kirim harga" to a new number: after approve it leaves through the outbox from the sales number that received
// the question, to that number's thread (there is no dealer yet).
func TestApprovePriceListToNewNumber(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res := run(t, r, "all")
	pl := find(res, "price_list", "Toko Mandiri")
	if pl == nil {
		t.Fatal("no price_list proposal")
	}
	out, err := proposals.Decide(ctx, st, nil, now, false, pl.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	if err != nil || out.OutboxID == nil {
		t.Fatalf("decide %+v %v", out, err)
	}
	var to, from string
	if err := st.Pool.QueryRow(ctx, "select to_ref, payload->>'from' from outbox where id = $1", *out.OutboxID).Scan(&to, &from); err != nil {
		t.Fatal(err)
	}
	var salesWA string
	_ = st.Pool.QueryRow(ctx, "select wa_number from sales_users where name = 'Andi'").Scan(&salesWA)
	if to != "6282212343310@s.whatsapp.net" || from != salesWA {
		t.Fatalf("to %s from %s (Andi %s)", to, from, salesWA)
	}
	var pending int
	_ = st.Pool.QueryRow(ctx, "select count(*) from chat_messages m join chat_threads t on t.id = m.thread_id where t.kind = 'new' and m.status = 'pending'").Scan(&pending)
	if pending != 1 {
		t.Fatalf("%d pending bubbles in the new-number thread", pending)
	}
}

// RBAC: finance may decide a collect but not a follow-up; a sales user only for their own dealers; warehouse
// decides transfers.
func TestDecideRoles(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res := run(t, r, "all")
	user := func(email string) proposals.Decider {
		var id uuid.UUID
		var role string
		if err := st.Pool.QueryRow(ctx, "select sales_user_id, role from users where email = $1", email).Scan(&id, &role); err != nil {
			t.Fatal(err)
		}
		return proposals.Decider{SalesUserID: id, Role: role, Name: email, Email: email}
	}
	decide := func(p *stored, who proposals.Decider) error {
		_, err := proposals.Decide(ctx, st, nil, now, false, p.ID, who, proposals.Decision{Decision: "approve"})
		return err
	}
	nusa := find(res, "collect", "Nusa")
	prima := find(res, "followup", "Prima")
	tr := find(res, "transfer", "Kamera IP 4MP")
	if nusa == nil || prima == nil || tr == nil {
		t.Fatal("proposals missing")
	}
	if err := decide(prima, user("finance@gsi.co.id")); !errors.Is(err, proposals.ErrForbidden) {
		t.Fatalf("finance approved a follow-up: %v", err)
	}
	if err := decide(prima, user("andi@gsi.co.id")); !errors.Is(err, proposals.ErrForbidden) {
		t.Fatalf("Andi approved Dewi's dealer: %v", err)
	}
	if err := decide(prima, user("dewi@gsi.co.id")); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if err := decide(nusa, user("finance@gsi.co.id")); err != nil {
		t.Fatalf("finance collect: %v", err)
	}
	if err := decide(tr, user("andi@gsi.co.id")); !errors.Is(err, proposals.ErrForbidden) {
		t.Fatalf("sales approved a transfer: %v", err)
	}
	if err := decide(tr, user("gudang@gsi.co.id")); err != nil {
		t.Fatalf("warehouse transfer: %v", err)
	}
}
