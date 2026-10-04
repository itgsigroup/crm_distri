package agents

import "testing"

func sig(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

func intp(v int) *int { return &v }

// Stage 05: next-best-action rules (rules/nba.json) — table-driven, first match wins.
func TestNBARules(t *testing.T) {
	rules := LoadNBARules()
	if len(rules) < 10 {
		t.Fatalf("rule table has %d rules, want ≥ 10", len(rules))
	}
	for _, r := range rules {
		if r.ID == "" || r.Then.Type == "" || r.Then.Agent == "" || r.Then.Title == "" || r.Then.Why == "" {
			t.Fatalf("incomplete rule %+v", r)
		}
	}
	cases := []struct {
		name     string
		st       NBAState
		wantRule string // "" = no action
		wantType string
	}{
		{"PO overdue + meeting tomorrow → raise in kick-off", NBAState{Signals: sig("po_overdue"), MeetingTomorrow: true, Stage: "Penawaran"}, "po_overdue_meeting", "meeting_brief_point"},
		{"PO overdue, no meeting → gentle WhatsApp", NBAState{Signals: sig("po_overdue"), Stage: "Penawaran"}, "po_overdue", "send_wa"},
		{"champion moved → call prep for candidate", NBAState{Signals: sig("champion_moved"), Stage: "Penawaran"}, "champion_moved", "call_prep"},
		{"competitor + single-threaded → letter to decision maker", NBAState{Signals: sig("competitor_mentioned", "single_threaded"), Stage: "Penawaran"}, "competitor_single", "send_email"},
		{"competitor only → ask for the competing quote", NBAState{Signals: sig("competitor_mentioned"), Stage: "Penawaran"}, "competitor", "send_email"},
		{"our commitment due today", NBAState{Signals: sig(), CommitmentDueToday: "revisi Senin", Stage: "Penawaran"}, "commitment_due_today", "send_email"},
		{"spec pending + presentation tomorrow", NBAState{Signals: sig("spec_pending"), MeetingTomorrow: true, Stage: "Berkualifikasi"}, "spec_pending_meeting", "prepare_document"},
		{"spec pending without meeting → nothing", NBAState{Signals: sig("spec_pending"), Stage: "Berkualifikasi"}, "", ""},
		{"legal cycle → survey report + BoQ", NBAState{Signals: sig("legal_cycle"), Stage: "Penawaran"}, "legal_cycle", "prepare_document"},
		{"silent tender account → re-engage on e-katalog", NBAState{Signals: sig("silent"), Tags: []string{"Tender"}, Stage: "Berkualifikasi"}, "silent_tender", "send_wa"},
		{"silent (tag match is case-insensitive)", NBAState{Signals: sig("silent"), Tags: []string{"led indoor"}, Stage: "Berkualifikasi"}, "silent_tender", "send_wa"},
		{"silent other account → generic re-engage", NBAState{Signals: sig("silent"), Tags: []string{"CCTV"}, Stage: "Penawaran"}, "silent", "send_wa"},
		{"single-threaded only → open path to decision maker", NBAState{Signals: sig("single_threaded"), Stage: "Penawaran"}, "single_threaded", "send_email"},
		{"paid on time + warranty ends in 60 days → maintenance quote", NBAState{Signals: sig("payment_on_time"), WarrantyDays: intp(60), Stage: "Penawaran"}, "maintenance", "create_quotation"},
		{"paid on time + warranty far away → nothing", NBAState{Signals: sig("payment_on_time"), WarrantyDays: intp(200), Stage: "Penawaran"}, "", ""},
		{"paid on time, no installed system → nothing", NBAState{Signals: sig("payment_on_time"), Stage: "Penawaran"}, "", ""},
		{"new lead → discovery call", NBAState{Signals: sig(), Stage: "Baru"}, "new_lead", "schedule_meeting"},
		{"healthy deal, no evidence → nothing", NBAState{Signals: sig(), Stage: "Penawaran"}, "", ""},
		{"meeting tomorrow alone is not a trigger", NBAState{Signals: sig(), MeetingTomorrow: true, Stage: "Penawaran"}, "", ""},
		{"priority: PO overdue beats competitor", NBAState{Signals: sig("po_overdue", "competitor_mentioned"), Stage: "Penawaran"}, "po_overdue", "send_wa"},
		{"priority: champion moved beats competitor + single-threaded", NBAState{Signals: sig("champion_moved", "competitor_mentioned", "single_threaded"), Stage: "Penawaran"}, "champion_moved", "call_prep"},
		{"priority: signal beats new-lead stage", NBAState{Signals: sig("competitor_mentioned"), Stage: "Baru"}, "competitor", "send_email"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, ok := SelectNBA(rules, c.st)
			if c.wantRule == "" {
				if ok {
					t.Fatalf("expected no action, got %s", r.ID)
				}
				return
			}
			if !ok || r.ID != c.wantRule || r.Then.Type != c.wantType {
				t.Fatalf("got %q/%q (match=%v), want %q/%q", r.ID, r.Then.Type, ok, c.wantRule, c.wantType)
			}
		})
	}
}

// Placeholders in rule texts are filled from the opportunity's evidence.
func TestNBAFill(t *testing.T) {
	got := fill("Kirim surat pengantar ke {decision} cc {champion}", map[string]string{"decision": "Bu Lestari", "champion": "Pak Hendra"})
	if got != "Kirim surat pengantar ke Bu Lestari cc Pak Hendra" {
		t.Fatal(got)
	}
}
