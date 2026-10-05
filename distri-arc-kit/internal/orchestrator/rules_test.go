package orchestrator

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/views"
)

var today = time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)

func ip(n int) *int         { return &n }
func fp(f float64) *float64 { return &f }

func dealer(slug, status, credit string, due int) *agents.Dealer {
	d := &agents.Dealer{}
	d.ID, d.UUID, d.Name, d.Tier = slug, uuid.New(), "CV "+slug, "A"
	d.CreditLimit = 100_000_000
	d.Metrics.Status, d.Metrics.Credit.State, d.Metrics.DueIn = status, credit, ip(due)
	d.Metrics.Rhythm, d.Metrics.Last, d.Metrics.Cyc = ip(14), ip(14-due), float64(14-due)/14
	d.Metrics.Credit.Room = fp(0.5)
	d.Owner.Name = "Dewi"
	return d
}

func cand(agent, kind string, d *agents.Dealer, conf float64) *Cand {
	p := domain.Proposal{Agent: agent, Kind: kind, Title: kind + " " + d.ID, Confidence: conf, SignalIDs: []uuid.UUID{uuid.New()}, Autonomy: "approve",
		Payload: map[string]any{}, DedupeKey: kind + ":" + d.ID}
	id := d.UUID
	p.DealerID = &id
	return &Cand{P: p, Dealer: d}
}

func policies() domain.PolicySet {
	p := domain.DefaultPolicies()
	p.Autonomy = map[string]domain.AutonomyRow{
		"AI Order":     {Auto: []string{"so_draft"}, Approve: []string{"price_counter"}},
		"AI Follow-up": {Auto: []string{"followup"}, Approve: []string{"followup"}},
		"AI Kredit":    {Auto: []string{"credit_hold", "credit_release"}, Approve: []string{"credit_release"}},
		"AI Stok":      {Approve: []string{"push_stock"}},
		"AI Penagihan": {Auto: []string{"collect"}, Approve: []string{"installment"}},
	}
	return p
}

func ruleInput(ds ...*agents.Dealer) RuleInput {
	by := map[uuid.UUID]*agents.Dealer{}
	for _, d := range ds {
		by[d.UUID] = d
	}
	return RuleInput{Policies: policies(), Today: today, LastFollowup: map[uuid.UUID]time.Time{}, Interactions: map[uuid.UUID][]SalesCount{},
		Dealer: func(id uuid.UUID) *agents.Dealer { return by[id] },
		MakeCollect: func(d *agents.Dealer) *domain.Proposal {
			id := d.UUID
			return &domain.Proposal{Agent: "AI Penagihan", DealerID: &id, Kind: domain.KindCollect, Title: "collect " + d.ID, Confidence: 0.84,
				SignalIDs: []uuid.UUID{uuid.New()}, Autonomy: "approve", Payload: map[string]any{"tone": "tegas"}, DedupeKey: "collect:" + d.ID}
		}}
}

func rules(cs []domain.Conflict, rule string) int {
	n := 0
	for _, c := range cs {
		if c.Rule == rule {
			n++
		}
	}
	return n
}

// credit_over_stock: a dealer over limit in a push → left out of the push, a collect is created, one conflict.
func TestCreditOverStock(t *testing.T) {
	bad := dealer("mitra", domain.StatusAtRisk, domain.CreditOverLimit, -9)
	bad.OpenInvoices = []views.OpenInvoice{{Number: "INV/0889", Residual: 46_000_000, LateDays: 11}}
	ok := dealer("lampu", domain.StatusAktif, domain.CreditCash, 2)
	push := &Cand{P: domain.Proposal{Agent: "AI Stok", Kind: domain.KindPushStock, Title: "push", Confidence: 0.8, SignalIDs: []uuid.UUID{uuid.New()},
		DealerIDs: []uuid.UUID{bad.UUID, ok.UUID}, Payload: map[string]any{"name": "Modul LED P5 outdoor"}, DedupeKey: "push:led"}}
	out, cs := Synthesize([]*Cand{push}, ruleInput(bad, ok))
	if n := rules(cs, RuleCreditOverStock); n != 1 {
		t.Fatalf("%d credit_over_stock conflicts", n)
	}
	if len(push.P.DealerIDs) != 1 || push.P.DealerIDs[0] != ok.UUID {
		t.Errorf("push still targets the dealer over limit: %v", push.P.DealerIDs)
	}
	if len(out) != 2 || out[1].P.Kind != domain.KindCollect || *out[1].P.DealerID != bad.UUID {
		t.Fatalf("collect not created: %+v", out)
	}

	// a follow-up to the same dealer waits for the payment and never runs on its own
	f := cand("AI Follow-up", domain.KindFollowup, bad, 0.9)
	_, cs = Synthesize([]*Cand{f}, ruleInput(bad))
	if rules(cs, RuleCreditOverStock) != 1 || f.WaitFor != "payment:INV/0889" || Evaluate(f, policies()).Auto {
		t.Errorf("follow-up over limit: wait %q auto %v", f.WaitFor, Evaluate(f, policies()).Auto)
	}
}

// collect_before_followup: the reminder first, the follow-up waits (plan item waiting after the collect).
func TestCollectBeforeFollowup(t *testing.T) {
	d := dealer("nusa", domain.StatusAktif, domain.CreditTipis, 4)
	d.Metrics.Credit.Room = fp(0.02)
	f := cand("AI Follow-up", domain.KindFollowup, d, 0.85)
	c := cand("AI Penagihan", domain.KindCollect, d, 0.9)
	c.P.Payload = map[string]any{"tone": "ramah", "invoices": []string{"INV/0901"}}
	_, cs := Synthesize([]*Cand{f, c}, ruleInput(d))
	if rules(cs, RuleCollectFirst) != 1 {
		t.Fatalf("conflicts %+v", cs)
	}
	if f.WaitFor != "payment:INV/0901" || !Evaluate(f, policies()).Auto || !Evaluate(c, policies()).Auto {
		t.Fatalf("wait %q follow-up auto %v collect auto %v", f.WaitFor, Evaluate(f, policies()), Evaluate(c, policies()))
	}
	f.P.Payload["wait_for"] = f.WaitFor
	plan := Build([]PlanCand{
		{ID: uuid.New(), Key: f.P.DedupeKey, Agent: f.P.Agent, Kind: f.P.Kind, Autonomy: "auto", Status: "proposed", Payload: f.P.Payload, Dealer: d},
		{ID: uuid.New(), Key: c.P.DedupeKey, Agent: c.P.Agent, Kind: c.P.Kind, Autonomy: "auto", Status: "proposed", Payload: c.P.Payload, Dealer: d},
	}, today)
	if len(plan) != 2 || plan[0].Keys[0] != c.P.DedupeKey || plan[1].Keys[0] != f.P.DedupeKey || plan[1].Status != domain.PlanWaiting {
		t.Fatalf("plan order: %+v", plan)
	}
	if plan[0].Time != "07.30" {
		t.Errorf("first slot %s", plan[0].Time)
	}
}

// margin_floor: a counter at 8,3% margin never runs on its own.
func TestMarginFloorNeverAuto(t *testing.T) {
	d := dealer("indo", domain.StatusKeyAccount, domain.CreditAman, 8)
	for _, kind := range []string{domain.KindSODraft, domain.KindPriceCounter} {
		c := cand("AI Order", kind, d, 0.95)
		c.P.Payload = map[string]any{"margin_asked": 8.3, "margin_pct": 8.3, "complete": true, "discount_pct": 3.0, "counter_pct": 1.5}
		_, cs := Synthesize([]*Cand{c}, ruleInput(d))
		if rules(cs, RuleMarginFloor) != 1 || Evaluate(c, policies()).Auto {
			t.Errorf("%s at 8,3%% margin: conflicts %d auto %v", kind, rules(cs, RuleMarginFloor), Evaluate(c, policies()).Auto)
		}
	}
}

// Autonomy: the matrix alone is not enough.
func TestAutonomyGuard(t *testing.T) {
	d := dealer("prima", domain.StatusKeyAccount, domain.CreditAman, 1)
	p := policies()
	if v := Evaluate(cand("AI Follow-up", domain.KindFollowup, d, 0.9), p); !v.Auto {
		t.Fatalf("H-1 follow-up at 0.9 should be auto: %+v", v)
	}
	if v := Evaluate(cand("AI Follow-up", domain.KindFollowup, d, 0.75), p); v.Auto {
		t.Error("confidence 0.75 must not be auto")
	}
	// credit_release is never auto, even when a (wrong) matrix lists it
	if v := Evaluate(cand("AI Kredit", domain.KindCreditRelease, d, 0.99), p); v.Auto {
		t.Error("credit_release must never be auto")
	}
	if v := Evaluate(cand("AI Follow-up", domain.KindFollowup, dealer("x", domain.StatusAtRisk, domain.CreditAman, 1), 0.9), p); v.Auto {
		t.Error("At risk dealer must not be auto")
	}
	if v := Evaluate(cand("AI Follow-up", domain.KindFollowup, dealer("y", domain.StatusAktif, domain.CreditTipis, 1), 0.9), p); v.Auto {
		t.Error("tight limit must not be auto")
	}
	two := cand("AI Follow-up", domain.KindFollowup, d, 0.9)
	two.P.Payload["second"] = true
	if Evaluate(two, p).Auto {
		t.Error("second follow-up must not be auto")
	}
	if v := Evaluate(cand("AI Follow-up", domain.KindFollowup, dealer("z", domain.StatusAktif, domain.CreditAman, 3), 0.9), p); v.Auto {
		t.Error("follow-up 3 days ahead is not H-1")
	}
}

// suppression and followup_gap keep a proposal out of the queue.
func TestSuppressionAndGap(t *testing.T) {
	d := dealer("borneo", domain.StatusKeyAccount, domain.CreditAman, 1)
	in := ruleInput(d)
	in.Suppressions = []Suppression{{Agent: "AI Follow-up", Kind: domain.KindFollowup, DealerID: d.UUID, Until: today.AddDate(0, 0, 13)}}
	c := cand("AI Follow-up", domain.KindFollowup, d, 0.9)
	_, cs := Synthesize([]*Cand{c}, in)
	if c.Suppressed == "" || rules(cs, RuleSuppression) != 1 {
		t.Fatalf("not suppressed: %+v", cs)
	}
	in = ruleInput(d)
	in.LastFollowup[d.UUID] = today.AddDate(0, 0, -5)
	c = cand("AI Follow-up", domain.KindFollowup, d, 0.9)
	_, cs = Synthesize([]*Cand{c}, in)
	if c.Suppressed == "" || rules(cs, RuleFollowupGap) != 1 {
		t.Fatalf("gap not applied: %+v", cs)
	}
}

// dedupe: a kind covered by a combined proposal is merged into it, provenance united.
func TestDedupeCovers(t *testing.T) {
	d := dealer("mitra", domain.StatusAtRisk, domain.CreditOverLimit, -9)
	f := cand("AI Follow-up + AI Penagihan", domain.KindFollowup, d, 0.8)
	f.P.Payload["covers"] = []string{domain.KindCollect}
	c := cand("AI Penagihan", domain.KindCollect, d, 0.84)
	_, cs := Synthesize([]*Cand{f, c}, ruleInput(d))
	if !c.Dropped || len(f.P.SignalIDs) != 2 || rules(cs, RuleDedupe) != 1 {
		t.Fatalf("dropped %v signals %d", c.Dropped, len(f.P.SignalIDs))
	}
}
