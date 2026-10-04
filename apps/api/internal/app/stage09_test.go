package app

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// Stage 09: 12 e-mail fixtures → 12 interactions (9 linked, 2 unresolved, 1 ignored),
// bodies without quoted replies, and no duplicates on a second run.
func TestStage09GmailCapture(t *testing.T) {
	a, _ := fresh(t) // PostSeed ran capture_gmail once
	ctx := context.Background()
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE raw_ref LIKE 'email:%'`); n != 12 {
		t.Fatalf("%d email interactions, want 12", n)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE raw_ref LIKE 'email:%' AND account_id IS NOT NULL`); n != 9 {
		t.Fatalf("%d emails linked to an account, want 9", n)
	}
	if n := count(t, a, `SELECT count(*) FROM unresolved_identities u JOIN interactions i ON i.id=u.interaction_id WHERE u.kind='email' AND i.raw_ref LIKE 'email:%' AND i.account_id IS NULL`); n != 2 {
		t.Fatalf("%d unresolved senders, want 2", n)
	}
	if n := count(t, a, `SELECT count(*) FROM interactions WHERE raw_ref LIKE 'email:%' AND summary LIKE 'diabaikan%' AND account_id IS NULL AND extracted`); n != 1 {
		t.Fatalf("%d ignored emails, want 1", n)
	}
	quoted := regexp.MustCompile(`(?m)^\s*>|wrote:|menulis:|-----Original Message-----|^-- ?$`)
	rows, err := a.DB.Pool.Query(ctx, `SELECT raw_ref, body_text FROM interactions WHERE raw_ref LIKE 'email:%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var ref, body string
		_ = rows.Scan(&ref, &body)
		if strings.TrimSpace(body) == "" || quoted.MatchString(body) {
			t.Errorf("%s: body empty or still quoted: %q", ref, body)
		}
	}
	rows.Close()
	res, err := a.CaptureGmail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 0 {
		t.Fatalf("second run stored %d", res.Stored)
	}
	// Even a full re-fetch (watermark lost) does not duplicate.
	exec(t, a, `DELETE FROM sync_watermarks WHERE system='gmail'`)
	res, err = a.CaptureGmail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 12 || res.Stored != 0 || count(t, a, `SELECT count(*) FROM interactions WHERE raw_ref LIKE 'email:%'`) != 12 {
		t.Fatalf("re-fetch: %+v", res)
	}
}

// Stage 09: the 3 calendar meetings of tomorrow get an H-1 meeting-prep action each.
func TestStage09CalendarMeetingPrep(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	ids := []string{"ev-bsd-kickoff", "ev-forecast-q4", "ev-baja-presentasi"}
	// Start from the Google Calendar connector alone (drop the seeded copies).
	exec(t, a, `DELETE FROM calendar_events`)
	if n, err := a.CaptureCalendar(ctx); err != nil || n != 3 {
		t.Fatalf("calendar capture: %d %v", n, err)
	}
	if n := count(t, a, `SELECT count(*) FROM calendar_events WHERE id = ANY($1) AND source='google'`, ids); n != 3 {
		t.Fatalf("calendar events captured: %d", n)
	}
	if count(t, a, `SELECT count(*) FROM calendar_events WHERE (id='ev-bsd-kickoff' AND account_id='bsd') OR (id='ev-baja-presentasi' AND account_id='baja') OR (id='ev-forecast-q4' AND internal)`) != 3 {
		t.Fatal("meetings not linked to accounts via attendee e-mail (internal review must stay internal)")
	}
	if _, err := a.CaptureCalendar(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, a, `SELECT count(*) FROM calendar_events`); n != 3 {
		t.Fatalf("calendar re-capture duplicated events: %d", n)
	}
	n, err := a.Agents.MeetingPrep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("meeting prep created %d actions, want 3", n)
	}
	if c := count(t, a, `SELECT count(*) FROM actions WHERE type='meeting_brief' AND status='proposed' AND payload->>'event_id' = ANY($1) AND due_label LIKE 'Besok %' AND jsonb_array_length(evidence) > 0`, ids); c != 3 {
		t.Fatalf("meeting_brief actions: %d", c)
	}
	if count(t, a, `SELECT count(*) FROM actions WHERE type='meeting_brief' AND payload->>'event_id'='ev-bsd-kickoff' AND account_id='bsd' AND prep LIKE '%PO%'`) != 1 {
		t.Fatal("kick-off prep should raise the overdue PO")
	}
	if n, _ := a.Agents.MeetingPrep(ctx); n != 0 {
		t.Fatalf("second meeting prep run created %d", n)
	}
}
