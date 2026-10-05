package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/outbox"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

func chatServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	h := api.New(config.Config{Env: "dev"}, st, clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB)), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st
}

// Stage 03 acceptance: a reply from the UI is a human decision (proposal reply, decided_by, audit) that leaves
// only through the outbox and is delivered by the transport.
func TestReplyGoesThroughOutbox(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var threadID uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where seed_key = 'seed:thread:c-sinar'").Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	_, list := get(t, srv, "/api/chat/threads?tab=dealer", "andi@gsi.co.id")
	if len(list["items"].([]any)) == 0 {
		t.Fatal("no dealer threads")
	}
	body, _ := json.Marshal(map[string]string{"body": "Siap Mbak, dikirim Senin pagi."})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/chat/threads/"+threadID.String()+"/messages", bytes.NewReader(body))
	req.Header.Set("X-Dev-User", "andi@gsi.co.id")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusAccepted {
		t.Fatalf("send: %v %v", err, res.Status)
	}
	var out map[string]string
	_ = json.NewDecoder(res.Body).Decode(&out)
	obID := uuid.MustParse(out["outbox_id"])

	var decidedBy *uuid.UUID
	var status, kind string
	if err := st.Pool.QueryRow(ctx, "select decided_by, status, kind from proposals where id = $1", out["proposal_id"]).Scan(&decidedBy, &status, &kind); err != nil {
		t.Fatal(err)
	}
	if decidedBy == nil || status != "approved" || kind != "reply" {
		t.Fatalf("proposal decided_by=%v status=%s kind=%s", decidedBy, status, kind)
	}
	if count(t, st, "select count(*) from audit_log where action = 'chat.reply'") != 1 {
		t.Fatal("reply not audited")
	}

	fake := wa.NewFake()
	r := outbox.DefaultRules()
	r.ReplyMin, r.ReplyMax = 0, 0
	if _, err := outbox.NewSender(st, fake, clock.Fixed(time.Now()), r).Send(ctx, obID); err != nil {
		t.Fatal(err)
	}
	if len(fake.Sent()) != 1 || fake.Sent()[0].Text != "Siap Mbak, dikirim Senin pagi." {
		t.Fatalf("fake sent %+v", fake.Sent())
	}
	if count(t, st, "select count(*) from outbox where id = $1 and status = 'sent'", obID) != 1 {
		t.Fatal("outbox not sent")
	}
	if count(t, st, "select count(*) from proposals where id = $1 and status = 'executed'", out["proposal_id"]) != 1 {
		t.Fatal("proposal not executed")
	}
	if count(t, st, "select count(*) from chat_messages where proposal_id = $1 and status = 'sent' and wa_msg_id like 'FAKE%'", out["proposal_id"]) != 1 {
		t.Fatal("chat message not marked sent")
	}
}

// Proactive messages obey the per-sales daily cap and the random gap.
func TestOutboxDailyCapAndGap(t *testing.T) {
	_, st := chatServer(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 5, 10, 0, 0, 0, clock.WIB)
	mk := func(sent *time.Time) uuid.UUID {
		var sig, pid, oid uuid.UUID
		if err := st.Pool.QueryRow(ctx, "select id from signals limit 1").Scan(&sig); err != nil {
			t.Fatal(err)
		}
		if err := st.Pool.QueryRow(ctx, `insert into proposals (agent, kind, title, why, confidence, signal_ids, autonomy, status)
			values ('AI Follow-up','followup','t','w',0.9,$1,'approve','approved') returning id`, []uuid.UUID{sig}).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		st0, sentAt := "pending", (*time.Time)(nil)
		if sent != nil {
			st0, sentAt = "sent", sent
		}
		if err := st.Pool.QueryRow(ctx, `insert into outbox (proposal_id, channel, to_ref, payload, status, sent_at)
			values ($1,'wa','x', $2, $3, $4) returning id`, pid, `{"from":"6281234504471","to":"62811@s.whatsapp.net","text":"halo","kind":"followup"}`, st0, sentAt).Scan(&oid); err != nil {
			t.Fatal(err)
		}
		return oid
	}
	fake := wa.NewFake()
	rules := outbox.Rules{GapMin: time.Minute, GapMax: time.Minute, DailyCapOverride: 2}
	s := outbox.NewSender(st, fake, clock.Fixed(now), rules)
	recent := now.Add(-10 * time.Second)
	mk(&recent)
	wait, err := s.Send(ctx, mk(nil))
	if err != nil || wait < 49*time.Second || wait > 51*time.Second || len(fake.Sent()) != 0 {
		t.Fatalf("gap: wait %v err %v sent %d", wait, err, len(fake.Sent()))
	}
	older := now.Add(-2 * time.Hour)
	mk(&older)
	wait, err = s.Send(ctx, mk(nil))
	if err != outbox.ErrCapReached || wait <= 0 || len(fake.Sent()) != 0 {
		t.Fatalf("cap: wait %v err %v", wait, err)
	}
}

func count(t *testing.T, st *store.Store, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
