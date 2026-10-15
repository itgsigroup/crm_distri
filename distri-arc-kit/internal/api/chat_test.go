package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// Analisa WhatsApp per bubble (ADR 0027): the thread carries, on each bubble, the request and the AI note of the
// last analysis and the reply state read live; a bubble written after the analysis says it was not analysed yet.
func TestThreadBubbleAnalysis(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var threadID, poID uuid.UUID
	var sentAt time.Time
	if err := st.Pool.QueryRow(ctx, `select t.id, m.id, m.sent_at from chat_threads t join chat_messages m on m.thread_id = t.id
		where t.seed_key = 'seed:thread:c-graha' and m.direction = 'in' order by m.sent_at desc limit 1`).Scan(&threadID, &poID, &sentAt); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into wa_message_insights (message_id, thread_id, sent_at, direction, state, intent, items, wait_min, analyzed_at, note, confidence)
		values ($1, $2, $3, 'in', 'waiting', 'order', '[{"name":"Kamera IP 4MP","qty":10}]', 30, $4, 'PO proyek apartemen — konfirmasi terima & jadwal kirim', 80)`,
		poID, threadID, sentAt, sentAt.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	code, out := get(t, srv, "/api/chat/threads/"+threadID.String(), "sam@gsi.co.id")
	if code != http.StatusOK {
		t.Fatalf("thread: %d", code)
	}
	if out["thread"].(map[string]any)["analyzed_at"] == nil {
		t.Error("thread without analyzed_at")
	}
	// the whole conversation kept (a year) in one page: nothing more to load, the oldest message named
	if out["has_more"] != false || out["history_days"] != float64(365) || out["history_from"] == nil {
		t.Errorf("history window has_more=%v days=%v from=%v", out["has_more"], out["history_days"], out["history_from"])
	}
	var po map[string]any
	for _, m := range out["messages"].([]any) {
		mm := m.(map[string]any)
		if mm["id"] == poID.String() {
			po, _ = mm["analysis"].(map[string]any)
		}
	}
	if po == nil || po["analyzed"] != true || po["intent"] != "order" || po["note"] == "" || po["confidence"] != float64(80) {
		t.Fatalf("po analysis %v", po)
	}
	// the reply state is read now: the analysis stored "waiting", the customer's first unanswered message is > 2 h old
	if po["state"] != "needs_reply" {
		t.Errorf("state %v (stored: waiting)", po["state"])
	}
	if items := po["items"].([]any); len(items) != 1 {
		t.Errorf("items %v", items)
	}
}

// Chat loads the chat window (CHAT_WINDOW_DAYS): the list holds conversations active in it, a conversation opens
// with its messages in it and loads older ones on scroll; a chat quiet for longer still opens with its last messages.
func TestChatWindow(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	h := api.New(config.Config{Env: "dev", ChatWindowDays: 14}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	thread := func(jid, ago string) string {
		var id string
		if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', $1, $1, '6281234504471', now() - $2::interval) returning id", jid, ago).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	add := func(id, wid, ago string) {
		if _, err := st.Pool.Exec(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, $2, 'in', $2, now() - $3::interval, 'received')", id, wid, ago); err != nil {
			t.Fatal(err)
		}
	}
	recent := thread("6281300000201@s.whatsapp.net", "2 days")
	add(recent, "OLD1", "40 days")
	add(recent, "NEW1", "2 days")
	quiet := thread("6281300000202@s.whatsapp.net", "40 days")
	add(quiet, "QUIET1", "40 days")

	_, list := get(t, srv, "/api/chat/threads", "sam@gsi.co.id")
	ids := map[string]bool{}
	for _, it := range list["items"].([]any) {
		ids[it.(map[string]any)["id"].(string)] = true
	}
	if !ids[recent] || ids[quiet] {
		t.Fatalf("list: recent %v, quiet %v", ids[recent], ids[quiet])
	}
	bodies := func(out map[string]any) []string {
		var b []string
		for _, m := range out["messages"].([]any) {
			b = append(b, m.(map[string]any)["body"].(string))
		}
		return b
	}
	_, page := get(t, srv, "/api/chat/threads/"+recent, "sam@gsi.co.id")
	if b := bodies(page); len(b) != 1 || b[0] != "NEW1" || page["has_more"] != true {
		t.Fatalf("first page %v has_more %v", b, page["has_more"])
	}
	first := page["messages"].([]any)[0].(map[string]any)["sent_at"].(string)
	_, older := get(t, srv, "/api/chat/threads/"+recent+"?before="+url.QueryEscape(first), "sam@gsi.co.id")
	if b := bodies(older); len(b) != 1 || b[0] != "OLD1" {
		t.Fatalf("older page %v", b)
	}
	if _, q := get(t, srv, "/api/chat/threads/"+quiet, "sam@gsi.co.id"); len(bodies(q)) != 1 {
		t.Fatalf("a quiet chat opened empty: %v", bodies(q))
	}
}
