package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

// Stage 07: `arc brief` → 4 points, each with evidence; delivery recorded by the FakeNotifier.
func TestStage07BriefFourPointsDelivered(t *testing.T) {
	a, _ := fresh(t)
	before := a.FakeNotify.Count()
	b, err := a.Agents.GenerateBrief(context.Background(), "manual", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Points) != 4 {
		t.Fatalf("brief has %d points, want 4", len(b.Points))
	}
	kinds := map[string]bool{}
	for _, p := range b.Points {
		kinds[p.Kind] = true
		if len(p.Evidence) == 0 || strings.TrimSpace(p.Text) == "" {
			t.Fatalf("point without evidence/text: %+v", p)
		}
	}
	for _, k := range []string{"gap", "risk", "people", "opportunity"} {
		if !kinds[k] {
			t.Fatalf("brief lacks a %s point: %v", k, kinds)
		}
	}
	if a.FakeNotify.Count() != before+1 {
		t.Fatalf("FakeNotifier recorded %d deliveries, want 1", a.FakeNotify.Count()-before)
	}
	last := a.FakeNotify.Sent[len(a.FakeNotify.Sent)-1]
	if !strings.Contains(strings.Join(last.To, ","), "sam@gsi.co.id") || !strings.Contains(last.Subject, "Brief ARC") || last.Text == "" {
		t.Fatalf("delivery: %+v", last)
	}
	if count(t, a, `SELECT count(*) FROM notifications WHERE channel='fake' AND subject LIKE 'Brief ARC%'`) != 1 {
		t.Fatal("delivery not logged in notifications")
	}
	if count(t, a, `SELECT count(*) FROM briefs WHERE id=$1 AND sent ? 'fake'`, b.ID) != 1 {
		t.Fatal("brief not marked as sent")
	}
}

// Stage 07: the scheduler registers the briefs at 06.45 and 16.00 (Asia/Jakarta).
func TestStage07BriefSchedule(t *testing.T) {
	at := func(d, h, m int) time.Time { return time.Date(2026, 9, d, h, m, 0, 0, domain.Jakarta) }
	briefs := func(s *schedule, now time.Time) int {
		n := 0
		for _, j := range s.due(now) {
			if j == "brief" {
				n++
			}
		}
		return n
	}
	s := newSchedule()
	steps := []struct {
		now  time.Time
		want int
	}{
		{at(29, 6, 0), 0}, {at(29, 6, 44), 0}, {at(29, 6, 45), 1}, {at(29, 6, 46), 0}, {at(29, 12, 0), 0},
		{at(29, 15, 59), 0}, {at(29, 16, 0), 1}, {at(29, 16, 30), 0}, {at(29, 23, 59), 0},
		{at(30, 6, 44), 0}, {at(30, 6, 45), 1}, {at(30, 16, 0), 1},
	}
	for _, st := range steps {
		if got := briefs(s, st.now); got != st.want {
			t.Fatalf("%s: %d brief runs, want %d", st.now.Format("02 15:04"), got, st.want)
		}
	}
	if briefSlots != [2][2]int{{6, 45}, {16, 0}} {
		t.Fatalf("brief slots %v", briefSlots)
	}
	// Daily analytics at 06:30, once.
	d := newSchedule()
	has := func(list []string, name string) bool {
		for _, x := range list {
			if x == name {
				return true
			}
		}
		return false
	}
	if has(d.due(at(29, 6, 29)), "forecast") || !has(d.due(at(29, 6, 30)), "forecast") || has(d.due(at(29, 7, 0)), "forecast") {
		t.Fatal("daily jobs must run once at 06:30")
	}
}

// Stage 07: approving the unmer send_wa action as a human sends it via the FakeTransport.
func TestStage07ApproveSendsViaFakeTransport(t *testing.T) {
	a, srv := fresh(t)
	andi := login(t, srv, "andi@gsi.co.id")
	var res struct {
		Action map[string]any `json:"action"`
		Toast  string         `json:"toast"`
	}
	andi.json("POST", "/api/actions/unmer/decision", map[string]string{"decision": "approve"}, 200, &res)
	if res.Action["status"] != "executed" {
		t.Fatalf("status %v", res.Action["status"])
	}
	if a.FakeWA.SentCount() != 1 {
		t.Fatalf("sends: %d", a.FakeWA.SentCount())
	}
	s := a.FakeWA.Sent[0]
	if s.ActionID != "unmer" || s.Session != "s-andi" || s.ChatJID != "6281390112233@s.whatsapp.net" || strings.TrimSpace(s.Text) == "" {
		t.Fatalf("sent: %+v", s)
	}
	if count(t, a, `SELECT count(*) FROM interactions WHERE raw_ref='out:unmer' AND direction='out' AND delivery_status='terkirim'`) != 1 {
		t.Fatal("outbound message not recorded")
	}
	if count(t, a, `SELECT count(*) FROM action_decisions WHERE action_id='unmer' AND user_id='andi' AND decision='approve'`) != 1 {
		t.Fatal("decision not recorded")
	}
	// Deciding twice is refused; nothing is re-sent.
	if code, _ := andi.do("POST", "/api/actions/unmer/decision", map[string]string{"decision": "approve"}); code != 400 {
		t.Fatalf("second decision → %d", code)
	}
	if a.FakeWA.SentCount() != 1 {
		t.Fatal("re-sent on second decision")
	}
}

// Stage 07: rejecting rsud (valid reason) suppresses the same agent/type/account for 14 days.
func TestStage07RejectSuppresses14Days(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	dewi := login(t, srv, "dewi@gsi.co.id")
	if code, _ := dewi.do("POST", "/api/actions/rsud/decision", map[string]string{"decision": "reject", "reason": "karena saya mau"}); code != 400 {
		t.Fatalf("reject with a free-text reason → %d, want 400", code)
	}
	dewi.json("POST", "/api/actions/rsud/decision", map[string]string{"decision": "reject", "reason": "Salah kontak / jalur", "note": "Bu Lestari sedang cuti"}, 200, nil)
	act, err := a.Actions.Get(ctx, "rsud")
	if err != nil || act.Status != "rejected" {
		t.Fatalf("rsud: %v %v", act.Status, err)
	}
	repropose := func() bool {
		_, created, err := a.Actions.Propose(ctx, actions.Proposal{Agent: act.Agent, Type: act.Type, AccountID: "rsud", Title: "Kirim surat pengantar lagi " + time.Now().String(),
			Evidence: []domain.Evidence{{Source: "test", Quote: "sinyal sama"}}}, storage.Actor{ID: "follow_up", Type: "agent"})
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	if repropose() {
		t.Fatal("same agent/type/account re-proposed right after a rejection")
	}
	domain.SetClockAnchor(demoAnchor.AddDate(0, 0, 13))
	if repropose() {
		t.Fatal("re-proposed on day 13")
	}
	// A different type on the same account is not suppressed.
	if _, created, _ := a.Actions.Propose(ctx, actions.Proposal{Agent: act.Agent, Type: "call_prep", AccountID: "rsud", Title: "Telepon",
		Evidence: []domain.Evidence{{Source: "test", Quote: "x"}}}, storage.Actor{ID: "follow_up", Type: "agent"}); !created {
		t.Fatal("unrelated proposal suppressed")
	}
	domain.SetClockAnchor(demoAnchor.AddDate(0, 0, 15))
	if !repropose() {
		t.Fatal("suppression still active after 14 days")
	}
}

// Stage 07: three "Tidak sesuai kebijakan" rejections of one agent+type create a learned rule.
func TestStage07ThreePolicyRejectsLearnRule(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	sam := login(t, srv, "sam@gsi.co.id")
	const reason = "Tidak sesuai kebijakan"
	found := false
	for _, r := range domain.RejectReasons {
		found = found || r == reason
	}
	if !found {
		t.Fatalf("%q is not a reject reason: %v", reason, domain.RejectReasons)
	}
	propose := func(acc string) (string, bool) {
		id, created, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Pricing agent", Type: "discount_offer", AccountID: acc, Title: "Tawarkan diskon 8% " + acc,
			Evidence: []domain.Evidence{{Source: "test", Quote: "kompetitor lebih murah"}}}, storage.Actor{ID: "pricing", Type: "agent"})
		if err != nil {
			t.Fatal(err)
		}
		return id, created
	}
	notesBefore := a.FakeNotify.Count()
	for i, acc := range []string{"sleman", "baja", "pelindo"} {
		id, created := propose(acc)
		if !created {
			t.Fatalf("proposal %d not created", i)
		}
		if n := count(t, a, `SELECT count(*) FROM learned_rules WHERE pattern='pricing:discount_offer'`); n != 0 {
			t.Fatalf("learned rule before rejection %d", i+1)
		}
		sam.json("POST", "/api/actions/"+id+"/decision", map[string]string{"decision": "reject", "reason": reason}, 200, nil)
	}
	if n := count(t, a, `SELECT count(*) FROM learned_rules WHERE pattern='pricing:discount_offer' AND active`); n != 1 {
		t.Fatalf("learned rules after 3 policy rejections: %d", n)
	}
	if a.FakeNotify.Count() <= notesBefore || !strings.Contains(a.FakeNotify.Sent[len(a.FakeNotify.Sent)-1].Subject, "aturan baru") {
		t.Fatal("CEO not notified about the learned rule")
	}
	if _, created := propose("semarang"); created {
		t.Fatal("learned rule did not block a new proposal of the same pattern")
	}
}

// Stage 07: machine principals can never decide — /api/v1 and UI decision endpoints → 403.
func TestStage07MachineCannotDecide(t *testing.T) {
	a, srv := fresh(t)
	sam := login(t, srv, "sam@gsi.co.id")
	key := sam.apiKey("Automation", domain.ScopeRead, domain.ScopePropose, domain.ScopeEvents, domain.ScopeSignals, domain.ScopeHuman)
	m := bearerClient(t, srv, key)
	for _, path := range []string{"/api/v1/actions/unmer/decision", "/api/actions/unmer/decision"} {
		if code, body := m.do("POST", path, map[string]string{"decision": "approve"}); code != 403 {
			t.Fatalf("machine POST %s → %d %s, want 403", path, code, body)
		}
	}
	if st := actionStatus(t, a, "unmer"); st != "proposed" || a.FakeWA.SentCount() != 0 {
		t.Fatalf("machine decision had an effect: status %s, sends %d", st, a.FakeWA.SentCount())
	}
	// The service layer refuses non-user actors too.
	if _, _, err := a.Actions.Decide(context.Background(), "unmer", storage.Actor{ID: "sam:apikey:x", Type: "machine"}, actions.Decision{Decision: "approve"}); err != actions.ErrHumanOnly {
		t.Fatalf("Decide by machine actor: %v", err)
	}
	// The same decision by the human owner of the key works.
	sam.json("POST", "/api/v1/actions/unmer/decision", map[string]string{"decision": "approve"}, 200, nil)
	if a.FakeWA.SentCount() != 1 {
		t.Fatal("human approval via /api/v1 did not send")
	}
}
