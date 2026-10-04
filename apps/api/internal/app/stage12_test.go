package app

import (
	"context"
	"strings"
	"testing"
)

// Stage 12: the flywheel verdict is computed — reversing the outcomes reverses it.
func TestStage12FlywheelVerdictFlips(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	fw, err := a.Ins.ComputeFlywheel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fw.Total == 0 || fw.WinOthers == 0 {
		t.Fatalf("no closed-deal history: %+v", fw)
	}
	if !fw.UseFlywheel || fw.WinExisting < 1.5*fw.WinOthers {
		t.Fatalf("seed should favour the flywheel: %+v", fw)
	}
	// Reverse the data: every won deal becomes lost and vice versa.
	exec(t, a, `UPDATE opportunities SET status = CASE status WHEN 'won' THEN 'lost' ELSE 'won' END WHERE status IN ('won','lost') AND source <> ''`)
	rev, err := a.Ins.ComputeFlywheel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rev.UseFlywheel || rev.WinExisting >= rev.WinOthers {
		t.Fatalf("verdict did not flip with reversed data: before %+v after %+v", fw, rev)
	}
}

// Stage 12: tender radar scores the 3 fixture tenders 92/84/71 with reasons and proposes qualification.
func TestStage12TenderRadar(t *testing.T) {
	a, _ := fresh(t) // PostSeed ran tender_radar
	want := map[string]int{"tdr-kudus": 92, "tdr-magelang": 84, "tdr-wonosari": 71}
	rows, err := a.DB.Pool.Query(context.Background(), `SELECT id, COALESCE(match_score,-1), reasons FROM tenders`)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for rows.Next() {
		var id, reasons string
		var score int
		_ = rows.Scan(&id, &score, &reasons)
		w, ok := want[id]
		if !ok {
			continue
		}
		seen++
		if score != w || strings.TrimSpace(reasons) == "" {
			t.Errorf("%s: score %d reasons %q, want %d with a reason", id, score, reasons, w)
		}
	}
	rows.Close()
	if seen != 3 {
		t.Fatalf("found %d fixture tenders", seen)
	}
	if n := count(t, a, `SELECT count(*) FROM actions WHERE type='qualify_tender' AND status='proposed' AND payload->>'tender_id' IN ('tdr-kudus','tdr-magelang','tdr-wonosari') AND jsonb_array_length(evidence) > 0`); n != 3 {
		t.Fatalf("qualify_tender proposals: %d, want 3", n)
	}
	if n, err := a.Agents.TenderRadar(context.Background()); err != nil || n != 0 {
		t.Fatalf("second radar run re-scored %d tenders (%v)", n, err)
	}
}

// Stage 12: Hotel Amarta's CCTV warranty ends in November → offer_maintenance.
func TestStage12RenewalAmarta(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	// The seed already carries the mockup's maintenance proposal for Amarta: the
	// Renewal agent must not offer it twice.
	if n, err := a.Agents.Renewal(ctx); err != nil || count(t, a, `SELECT count(*) FROM actions WHERE account_id='amarta' AND type='offer_maintenance'`) != 0 {
		t.Fatalf("renewal duplicated the existing Amarta maintenance proposal (%d, %v)", n, err)
	}
	// Without it, Renewal derives the offer from the installed system's warranty.
	exec(t, a, `UPDATE opportunities SET next_action_id=NULL WHERE next_action_id='amarta'`)
	exec(t, a, `DELETE FROM actions WHERE id='amarta'`)
	n, err := a.Agents.Renewal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var why, title string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT why, title FROM actions WHERE account_id='amarta' AND type='offer_maintenance' AND status='proposed' AND jsonb_array_length(evidence) > 0`).Scan(&why, &title); err != nil {
		t.Fatalf("no offer_maintenance for Amarta (created %d): %v", n, err)
	}
	if !strings.Contains(why, "November") || !strings.Contains(title, "CCTV") {
		t.Fatalf("Amarta offer: %q / %q", title, why)
	}
	// Warranties far away or already expired are not offered.
	if c := count(t, a, `SELECT count(*) FROM actions WHERE type='offer_maintenance' AND account_id IN ('bsd','semarang')`); c != 0 {
		t.Fatalf("offer for expired warranty: %d", c)
	}
	if again, _ := a.Agents.Renewal(ctx); again != 0 {
		t.Fatalf("second renewal run created %d", again)
	}
}

// Stage 12: team scorecard — Fajar has 0 lead and 102 WhatsApp messages.
func TestStage12TeamFajar(t *testing.T) {
	a, srv := fresh(t)
	team, err := a.Ins.Team(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range team {
		if r.UserID != "fajar" {
			continue
		}
		found = true
		if r.Pipeline != 0 || r.WAMessages30d != 102 || r.NoOppContacts == 0 {
			t.Fatalf("Fajar: pipeline %.0f, %d messages, %d contacts without lead", r.Pipeline, r.WAMessages30d, r.NoOppContacts)
		}
	}
	if !found {
		t.Fatal("Fajar missing from the team scorecard")
	}
	if n := count(t, a, `SELECT count(*) FROM opportunities WHERE owner_user_id='fajar' AND status='open' AND NOT historical`); n != 0 {
		t.Fatalf("Fajar owns %d open leads", n)
	}
	var pipe struct {
		Team []struct {
			Name     string  `json:"name"`
			Pipeline float64 `json:"pipeline"`
			Coach    string  `json:"coach"`
		} `json:"team"`
	}
	login(t, srv, "sam@gsi.co.id").json("GET", "/api/pipeline", nil, 200, &pipe)
	for _, r := range pipe.Team {
		if r.Name == "Fajar" {
			if r.Pipeline != 0 || !strings.Contains(r.Coach, "102 pesan") || !strings.Contains(r.Coach, "tanpa satu pun lead") {
				t.Fatalf("Fajar row: %+v", r)
			}
			return
		}
	}
	t.Fatal("Fajar not in /api/pipeline team")
}
