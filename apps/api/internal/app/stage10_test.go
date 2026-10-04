package app

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"arc/packages/connectors/odoo"
)

func proposeWriteProbability(t *testing.T, c *client, opp string) string {
	t.Helper()
	var res struct {
		Action struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"action"`
	}
	c.json("POST", "/api/opportunities/"+opp+"/write-probability", map[string]any{}, 200, &res)
	if res.Action.Type != "write_probability" || res.Action.Status != "proposed" {
		t.Fatalf("write-probability proposal: %+v", res.Action)
	}
	return res.Action.ID
}

// Stage 10: approving write_probability writes exactly probability to the linked
// crm.lead (guarded by write_date) plus an evidence note marked "via ARC".
func TestStage10WriteProbability(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	f := fakeOdoo(t, a)
	var leadID int64
	var health, manual int
	if err := a.DB.Pool.QueryRow(ctx, `SELECT source_id::bigint, health, probability FROM opportunities WHERE id='rsud' AND source_system='odoo'`).Scan(&leadID, &health, &manual); err != nil {
		t.Fatalf("rsud must be linked to Odoo: %v", err)
	}
	dewi := login(t, srv, "dewi@gsi.co.id")
	id := proposeWriteProbability(t, dewi, "rsud")
	if len(f.Writes) != 0 {
		t.Fatal("proposal alone wrote to Odoo")
	}
	dewi.json("POST", "/api/actions/"+id+"/decision", map[string]string{"decision": "approve"}, 200, nil)
	var write, note *odoo.WriteOp
	for i := range f.Writes {
		w := &f.Writes[i]
		switch {
		case w.Model == "crm.lead" && w.Method == "write":
			write = w
		case w.Model == "mail.message" && w.Method == "message_post":
			note = w
		}
	}
	if write == nil || write.ID != leadID || len(write.Values) != 1 || write.Values["probability"] != health || write.ExpectedWriteDate == "" {
		t.Fatalf("crm.lead write payload: %+v (want only probability=%d on #%d with write_date guard)", write, health, leadID)
	}
	if note == nil {
		t.Fatal("no evidence note posted")
	}
	body, _ := note.Values["body"].(string)
	if note.ID != leadID || note.Values["model"] != "crm.lead" || !strings.Contains(body, "via ARC") || !strings.Contains(body, "bukti") {
		t.Fatalf("evidence note: %+v", note)
	}
	for _, w := range f.Writes {
		if err := odoo.Validate(w); err != nil {
			t.Fatalf("non-whitelisted write reached Odoo: %+v", w)
		}
	}
	if count(t, a, `SELECT count(*) FROM opportunities WHERE id='rsud' AND probability=$1`, health) != 1 {
		t.Fatal("ARC copy of the probability not updated")
	}
	if count(t, a, `SELECT count(*) FROM odoo_writes WHERE action_id=$1 AND ok`, id) != 2 {
		t.Fatal("Odoo writes not audited")
	}
	if actionStatus(t, a, id) != "executed" {
		t.Fatal("action not executed")
	}
}

// Stage 10: when the Odoo record changed since the last sync nothing is written and a
// sync_conflict signal is raised.
func TestStage10WriteConflict(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	f := fakeOdoo(t, a)
	var leadID int64
	var manual int
	if err := a.DB.Pool.QueryRow(ctx, `SELECT source_id::bigint, probability FROM opportunities WHERE id='rsud'`).Scan(&leadID, &manual); err != nil {
		t.Fatal(err)
	}
	dewi := login(t, srv, "dewi@gsi.co.id")
	id := proposeWriteProbability(t, dewi, "rsud")
	f.ConflictOn["crm.lead#"+strconv.FormatInt(leadID, 10)] = true
	code, body := dewi.do("POST", "/api/actions/"+id+"/decision", map[string]string{"decision": "approve"})
	if code < 400 || !strings.Contains(string(body), "konflik") {
		t.Fatalf("conflicting write: %d %s", code, body)
	}
	if len(f.Writes) != 0 {
		t.Fatalf("Odoo received writes despite the conflict: %+v", f.Writes)
	}
	if count(t, a, `SELECT count(*) FROM signals WHERE type='sync_conflict' AND opportunity_id='rsud' AND resolved_at IS NULL AND jsonb_array_length(evidence) > 0`) != 1 {
		t.Fatal("no sync_conflict signal")
	}
	if count(t, a, `SELECT count(*) FROM opportunities WHERE id='rsud' AND probability=$1`, manual) != 1 {
		t.Fatal("ARC probability changed despite the conflict")
	}
	if st := actionStatus(t, a, id); st == "executed" {
		t.Fatal("conflicting action marked executed")
	}
}

// Stage 10: an open "kami" commitment becomes an Odoo activity on the linked lead
// (with the ARC marker); once fulfilled the activity is marked done — each exactly once.
func TestStage10CommitmentActivities(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	f := fakeOdoo(t, a)
	open := count(t, a, `SELECT count(*) FROM commitments c WHERE c.who='kami' AND c.status IN ('open','late')
		AND EXISTS (SELECT 1 FROM opportunities o WHERE o.source_system='odoo' AND (o.id=c.opportunity_id OR (c.opportunity_id IS NULL AND o.account_id=c.account_id AND o.status='open')))`)
	if open == 0 {
		t.Fatal("fixture has no open kami commitment on an Odoo-linked deal")
	}
	res, err := a.SyncCommitmentActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res["created"] != open || res["failed"] != 0 {
		t.Fatalf("activities created %v, want %d", res, open)
	}
	creates := 0
	for _, w := range f.Writes {
		if w.Model == "mail.activity" && w.Method == "create" {
			creates++
			note, _ := w.Values["note"].(string)
			if !strings.Contains(note, "via ARC") || w.Values["res_model"] != "crm.lead" || w.Values["res_id"] == nil || w.Values["date_deadline"] == nil {
				t.Fatalf("activity payload: %+v", w.Values)
			}
		}
	}
	if creates != open {
		t.Fatalf("Odoo received %d activity creates, want %d", creates, open)
	}
	// Idempotent: nothing new on the second run.
	if res, _ := a.SyncCommitmentActivities(ctx); res["created"] != 0 || res["done"] != 0 {
		t.Fatalf("second run wrote again: %v", res)
	}
	// Fulfil one commitment → its activity is marked done exactly once.
	var cid string
	var actID int64
	if err := a.DB.Pool.QueryRow(ctx, `SELECT id, odoo_activity_id FROM commitments WHERE odoo_activity_id IS NOT NULL LIMIT 1`).Scan(&cid, &actID); err != nil {
		t.Fatal(err)
	}
	exec(t, a, `UPDATE commitments SET status='done' WHERE id=$1`, cid)
	before := len(f.Writes)
	if res, _ := a.SyncCommitmentActivities(ctx); res["done"] != 1 {
		t.Fatalf("done sync: %v", res)
	}
	last := f.Writes[len(f.Writes)-1]
	if len(f.Writes) != before+1 || last.Model != "mail.activity" || last.Method != "action_done" || last.ID != actID {
		t.Fatalf("expected one action_done on activity %d, got %+v", actID, f.Writes[before:])
	}
	if res, _ := a.SyncCommitmentActivities(ctx); res["done"] != 0 {
		t.Fatal("activity marked done twice")
	}
	if count(t, a, `SELECT count(*) FROM odoo_writes WHERE model='mail.activity' AND ok`) != open+1 {
		t.Fatal("activity writes not recorded in odoo_writes")
	}
}
