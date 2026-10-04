package app

import (
	"context"
	"math"
	"strings"
	"testing"

	"arc/packages/core/insights"
)

func billions(v float64) float64 { return math.Round(v/1e8) / 10 }

// Stage 05: health of the 8 fixture opportunities matches the mockup ± 5 and
// the evidence-based forecast is commit 4,3 / best 5,9 / pipeline 11,9 (what-if BSD → 3,4).
func TestStage05HealthAndForecast(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	f := loadFixtures(t)
	// Recompute from components/evidence (the daily scoring job), not just the seeded values.
	if _, err := a.Agents.ScoreOpportunities(ctx); err != nil {
		t.Fatal(err)
	}
	deals, err := a.Ins.Deals(ctx, insights.Scope{All: true})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]insights.Deal{}
	for _, d := range deals {
		byID[d.ID] = d
	}
	for _, fd := range f.Deals {
		d, ok := byID[fd.ID]
		if !ok || !d.HealthOK {
			t.Fatalf("deal %s missing or without health", fd.ID)
		}
		if math.Abs(float64(d.Health-fd.Health)) > 5 {
			t.Errorf("%s health %d, mockup %d", fd.ID, d.Health, fd.Health)
		}
	}
	fc, err := a.Ins.Forecast(ctx, insights.Scope{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if billions(fc.Commit) != 4.3 || billions(fc.Best) != 5.9 || billions(fc.Pipeline) != 11.9 {
		t.Fatalf("forecast commit %.2f best %.2f pipeline %.2f (M), want 4,3 / 5,9 / 11,9", fc.Commit/1e9, fc.Best/1e9, fc.Pipeline/1e9)
	}
	what, err := a.Ins.Forecast(ctx, insights.Scope{All: true}, "bsd")
	if err != nil {
		t.Fatal(err)
	}
	if billions(what.Commit) != 3.4 {
		t.Fatalf("what-if BSD commit %.2f M, want 3,4", what.Commit/1e9)
	}
}

// Stage 05: nothing is sent to a customer while actions are only proposed — even
// after every agent job ran — and a human approval is what triggers the send.
func TestStage05NoSendWithoutApproval(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	for _, job := range []string{"extract", "identify_inbound", "hygiene", "score_opportunities", "meeting_prep", "collection", "renewal", "tender_radar", "forecast", "memory", "brief"} {
		if _, err := a.RunJob(ctx, job); err != nil {
			t.Fatalf("job %s: %v", job, err)
		}
	}
	if n := a.FakeWA.SentCount(); n != 0 {
		t.Fatalf("%d WhatsApp messages sent without approval", n)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE raw_ref LIKE 'out:%'`); n != 0 {
		t.Fatalf("%d outbound deliveries recorded without approval", n)
	}
	if n := count(t, a, `SELECT count(*) FROM actions WHERE type IN ('send_wa','send_email','payment_reminder','ask_spm_documents') AND status NOT IN ('proposed','snoozed') AND type <> 'historical'`); n != 0 {
		t.Fatalf("%d send actions left the proposed state without a decision", n)
	}
	if n := count(t, a, `SELECT count(*) FROM actions WHERE status IN ('approved','edited','executed') AND type <> 'historical' AND id NOT IN (SELECT action_id FROM action_decisions)`); n != 0 {
		t.Fatalf("%d actions executed without a recorded decision", n)
	}
	login(t, srv, "andi@gsi.co.id").json("POST", "/api/actions/unmer/decision", map[string]string{"decision": "approve"}, 200, nil)
	if n := a.FakeWA.SentCount(); n != 1 {
		t.Fatalf("after human approval: %d sends, want 1", n)
	}
}

// Stage 05: the inbound funnel has 7 stages with the fixture counts, and Rudi can be
// turned into a lead from the UI with the pain-point questions attached.
func TestStage05FunnelAndInboundLead(t *testing.T) {
	a, srv := fresh(t)
	f := loadFixtures(t)
	sam := login(t, srv, "sam@gsi.co.id")
	var fun struct {
		Stages []struct {
			Stage string `json:"stage"`
			N     int    `json:"n"`
		} `json:"stages"`
	}
	sam.json("GET", "/api/funnel?month="+f.Extra.Funnel.Month, nil, 200, &fun)
	if len(fun.Stages) != 7 {
		t.Fatalf("funnel has %d stages, want 7", len(fun.Stages))
	}
	for _, s := range fun.Stages {
		if want := f.Extra.Funnel.Counts[s.Stage]; s.N != want {
			t.Errorf("funnel %s = %d, fixture %d", s.Stage, s.N, want)
		}
	}
	sam.json("POST", "/api/prospects/in1/lead", map[string]any{}, 200, nil)
	var opp, note string
	if err := a.DB.Pool.QueryRow(context.Background(), `SELECT o.id, o.note FROM inbound_contacts i JOIN opportunities o ON o.id=i.opportunity_id WHERE i.id='in1' AND i.status='lead'`).Scan(&opp, &note); err != nil {
		t.Fatalf("Rudi lead: %v", err)
	}
	if !strings.Contains(note, "Pertanyaan:") || strings.Count(note, "?") < 3 {
		t.Fatalf("lead note lacks the questions: %q", note)
	}
}
