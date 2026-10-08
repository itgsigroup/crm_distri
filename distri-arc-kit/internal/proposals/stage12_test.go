package proposals_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/odoo"
	"distri-arc/internal/outbox"
	"distri-arc/internal/proposals"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
)

func fakeOdoo(t *testing.T) *odoo.Fake {
	f, err := odoo.NewFake(db.Seed, "seed/odoo")
	if err != nil {
		t.Fatal(err)
	}
	f.Write = true
	return f
}

// docs/stages/12 acceptance: the automatic SO draft for Toko Sinar becomes a draft sale order in Odoo with its
// source note; GSI Orbit records the order, a signal and a Kami commitment, and the Timeline shows the chain.
func TestSODraftToOdoo(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	r.OdooWrite = true
	res := run(t, r, "all")
	so := find(res, "so_draft", "Sinar")
	if so == nil {
		t.Fatal("no SO draft")
	}
	var obID uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id from outbox where proposal_id = $1 and channel = 'odoo_so_draft'", so.ID).Scan(&obID); err != nil {
		t.Fatalf("no odoo_so_draft row: %v", err)
	}
	fo := fakeOdoo(t)
	sender := outbox.NewSender(st, wa.NewFake(), now, outbox.Rules{}).WithOdoo(fo)

	fo.FailNext = errors.New("odoo down")
	if _, err := sender.Send(ctx, obID); err == nil {
		t.Fatal("failure not reported")
	}
	var status, pstatus string
	_ = st.Pool.QueryRow(ctx, "select o.status, p.status from outbox o join proposals p on p.id = o.proposal_id where o.id = $1", obID).Scan(&status, &pstatus)
	if status != "failed" || pstatus != "approved" {
		t.Fatalf("after failure: outbox %s proposal %s", status, pstatus)
	}

	if _, err := sender.Send(ctx, obID); err != nil {
		t.Fatal(err)
	}
	created := fo.Created()
	if len(created) != 1 || !strings.Contains(created[0]["note"].(string), "proposal "+so.ID.String()) || created[0]["partner_id"] != 3001 {
		t.Fatalf("odoo create %+v", created)
	}
	var by, number string
	if err := st.Pool.QueryRow(ctx, "select created_by, number from orders where source_id like 'sale.order:9%'").Scan(&by, &number); err != nil || by != "ai_order_draft" {
		t.Fatalf("order %s %s %v", by, number, err)
	}
	// the delivery sales already promised in the chat is adopted, not listed twice
	var kami, detail, due string
	var n int
	if err := st.Pool.QueryRow(ctx, "select title, detail, due_at::text from commitments where proposal_id = $1 and side = 'kami'", so.ID).Scan(&kami, &detail, &due); err != nil {
		t.Fatal(err)
	}
	_ = st.Pool.QueryRow(ctx, "select count(*) from commitments c join dealers d on d.id = c.dealer_id where d.slug = 'sinar' and c.side = 'kami' and c.title ilike 'Kirim%'").Scan(&n)
	if kami != "Kirim 10 kamera + 1 NVR Senin" || !strings.HasPrefix(detail, "SO draft #") || due != "2026-10-12" || n != 1 {
		t.Fatalf("kami %q %q due %s (%d)", kami, detail, due, n)
	}
	_ = st.Pool.QueryRow(ctx, "select o.status, p.status from outbox o join proposals p on p.id = o.proposal_id where o.id = $1", obID).Scan(&status, &pstatus)
	if status != "sent" || pstatus != "executed" {
		t.Fatalf("after success: outbox %s proposal %s", status, pstatus)
	}
	b, err := views.NewBuilder(st, now).Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	it, _ := b.Get("sinar")
	d, err := views.NewBuilder(st, now).DealerDetail(ctx, b, it)
	if err != nil {
		t.Fatal(err)
	}
	chain := map[string]bool{}
	for _, e := range d.Timeline {
		switch {
		case e.Kind == "decision" && strings.HasPrefix(e.Text, "Otonom: Buat SO Toko Sinar"):
			chain["decision"] = true
		case e.Kind == "so" && strings.Contains(e.Text, "SO draft #"):
			chain["odoo"] = true
		}
	}
	if !chain["decision"] || !chain["odoo"] {
		t.Fatalf("timeline chain %v: %+v", chain, d.Timeline[:3])
	}
	if len(d.Commitments["kami"]) == 0 {
		t.Fatal("Kami commitment not on the dealer page")
	}
}

// Approving a follow-up writes a decision note on the partner when ODOO_WRITE is on; transfers stay manual.
func TestDecisionNoteAndManualRows(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res := run(t, r, "all")
	p := find(res, "followup", "Prima")
	if _, err := proposals.Decide(ctx, st, nil, now, true, p.ID, sam(t, st), proposals.Decision{Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	var note uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id from outbox where proposal_id = $1 and channel = 'odoo_note'", p.ID).Scan(&note); err != nil {
		t.Fatalf("no decision note: %v", err)
	}
	fo := fakeOdoo(t)
	sender := outbox.NewSender(st, wa.NewFake(), now, outbox.Rules{}).WithOdoo(fo)
	if _, err := sender.Send(ctx, note); err != nil {
		t.Fatal(err)
	}
	if n := fo.Notes(); len(n) != 1 || n[0].ID != 3013 || !strings.Contains(n[0].Body, "Disetujui oleh Sam Setiadi") {
		t.Fatalf("notes %+v", n)
	}
	tr := find(res, "transfer", "Kamera IP 4MP")
	out, err := proposals.Decide(ctx, st, nil, now, true, tr.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Send(ctx, *out.OutboxID); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = st.Pool.QueryRow(ctx, "select status from outbox where id = $1", *out.OutboxID).Scan(&status)
	if status != "manual" {
		t.Fatalf("transfer outbox %s", status)
	}
}

// A dealer's answer to a sent follow-up is linked to the proposal and becomes a Mereka commitment; a payment
// promise with a weekday gets its date.
func TestReplyTracking(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res := run(t, r, "all")
	p := find(res, "followup", "Prima")
	out, err := proposals.Decide(ctx, st, nil, now, false, p.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	fake := wa.NewFake()
	if _, err := outbox.NewSender(st, fake, now, outbox.Rules{}).Send(ctx, *out.OutboxID); err != nil {
		t.Fatal(err)
	}
	var contact, account string
	_ = st.Pool.QueryRow(ctx, `select c.wa_number, s.wa_number from contacts c join dealers d on d.id = c.dealer_id join sales_users s on s.id = d.owner_id
		where d.slug = 'prima' and c.name = 'Pak Bayu'`).Scan(&contact, &account)
	in := wa.NewIngestor(st, nil)
	at := now.Now().Add(2 * time.Hour)
	if _, err := in.Process(ctx, wa.Message{ID: "R1", Account: account, ChatJID: wa.UserJID(contact), FromNumber: contact, FromName: "Pak Bayu", Text: "Ya mbak, kirim aja seperti biasa", Time: at}); err != nil {
		t.Fatal(err)
	}
	var replyTo *uuid.UUID
	_ = st.Pool.QueryRow(ctx, "select proposal_id from chat_messages where wa_msg_id = 'R1'").Scan(&replyTo)
	if replyTo == nil || *replyTo != p.ID {
		t.Fatalf("reply not linked: %v", replyTo)
	}
	var title string
	if err := st.Pool.QueryRow(ctx, "select title from commitments where proposal_id = $1 and side = 'mereka'", p.ID).Scan(&title); err != nil || title != "Order sesuai rekomendasi" {
		t.Fatalf("mereka %q %v", title, err)
	}
	if _, err := in.Process(ctx, wa.Message{ID: "R2", Account: account, ChatJID: wa.UserJID(contact), FromNumber: contact, FromName: "Pak Bayu", Text: "INV/0964 saya transfer hari Kamis ya", Time: at.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var promise, due string
	if err := st.Pool.QueryRow(ctx, "select title, due_at::text from commitments where source_key like 'wa:%' and title like 'Bayar%'").Scan(&promise, &due); err != nil {
		t.Fatal(err)
	}
	if promise != "Bayar INV/0964 Rp 22 jt" || due != "2026-10-08" {
		t.Fatalf("promise %q due %s", promise, due)
	}
	b, _ := views.NewBuilder(st, now).Board(ctx)
	it, _ := b.Get("prima")
	d, _ := views.NewBuilder(st, now).DealerDetail(ctx, b, it)
	found := false
	for _, e := range d.Timeline {
		if strings.HasPrefix(e.Conclusion, "Balasan untuk: Follow-up Prima") {
			found = true
		}
	}
	if !found {
		t.Fatal("reply not on the timeline")
	}
}

// A broken payment promise makes AI Penagihan propose a firm reminder that waits for a human.
func TestLatePromiseTriggersProposal(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	var dealer, sig uuid.UUID
	_ = st.Pool.QueryRow(ctx, "select id from dealers where slug = 'jaya'").Scan(&dealer)
	_ = st.Pool.QueryRow(ctx, "select id from signals where dealer_id = $1 limit 1", dealer).Scan(&sig)
	if _, err := st.Pool.Exec(ctx, `insert into commitments (dealer_id, side, title, detail, status, due_at, signal_ids, source_key)
		values ($1, 'mereka', 'Bayar INV/0990 Rp 26 jt', 'Janji via WhatsApp · 2 Okt', 'open', '2026-10-02', $2, 'test:promise')`, dealer, []uuid.UUID{sig}); err != nil {
		t.Fatal(err)
	}
	res := run(t, r, "all")
	var c *stored
	for i := range res.Stored {
		if res.Stored[i].Kind == "collect" && strings.Contains(res.Stored[i].Title, "Jaya Abadi") {
			c = &res.Stored[i]
		}
	}
	if c == nil || !strings.HasPrefix(c.Title, "Janji bayar terlewat") || c.Autonomy != "approve" {
		t.Fatalf("collect: %+v", c)
	}
	var status string
	_ = st.Pool.QueryRow(ctx, "select status from commitments where source_key = 'test:promise'").Scan(&status)
	if status != "late" {
		t.Fatalf("commitment %s", status)
	}
}

// Proactive sends wait for 08–18 WIB.
func TestSendWindow(t *testing.T) {
	st, r := setup(t)
	ctx := context.Background()
	res := run(t, r, "all")
	p := find(res, "followup", "Prima")
	out, _ := proposals.Decide(ctx, st, nil, now, false, p.ID, sam(t, st), proposals.Decision{Decision: "approve"})
	early := clock.Fixed(time.Date(2026, 10, 5, 6, 30, 0, 0, clock.WIB))
	fake := wa.NewFake()
	wait, err := outbox.NewSender(st, fake, early, outbox.Rules{SendFrom: 8, SendTo: 18}).Send(ctx, *out.OutboxID)
	if err != nil || wait != 90*time.Minute || len(fake.Sent()) != 0 {
		t.Fatalf("wait %v err %v sent %d", wait, err, len(fake.Sent()))
	}
}
