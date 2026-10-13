package ops_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/ops"
	"distri-arc/internal/outbox"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

var now = time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)

func setup(t *testing.T) *store.Store {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	return st
}

func count(t *testing.T, st *store.Store, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// Ingest stays idempotent across partitions: the same WhatsApp message twice is one signal and one chat message,
// and rows land in their month's partition.
func TestPartitionedIngestIsIdempotent(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	in := wa.NewIngestor(st, nil)
	m := wa.Message{ID: "P1", Account: "6281234504471", ChatJID: wa.UserJID("6281900300102"), FromNumber: "6281900300102", FromName: "Mbak Rina",
		Text: "Mas, stok NVR 16ch masih ada?", Time: now}
	for range 2 {
		if _, err := in.Process(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id = 'P1'"); n != 1 {
		t.Fatalf("chat messages %d", n)
	}
	if n := count(t, st, "select count(*) from signals_y2026m10 s join chat_messages m on m.signal_id = s.id where m.wa_msg_id = 'P1'"); n != 1 {
		t.Fatalf("signal not in the October partition: %d", n)
	}
	created, err := ops.EnsurePartitions(ctx, st, now.AddDate(0, 6, 0))
	if err != nil || created == 0 {
		t.Fatalf("ensure partitions %d %v", created, err)
	}
	if again, _ := ops.EnsurePartitions(ctx, st, now.AddDate(0, 6, 0)); again != 0 {
		t.Fatalf("ensure is not idempotent: %d", again)
	}
}

// docs/stages/13 acceptance: purge removes chat older than retention.chat_days (365, ADR 0028) without touching
// aggregates.
func TestPurgeKeepsAggregates(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	svc := dealersvc.New(st, clock.Fixed(now))
	if _, err := svc.Recompute(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, 0, -380) // September 2025: the only whole month before the cutoff
	if _, err := st.Q.EnsurePartitions(ctx, gen.EnsurePartitionsParams{Tbl: "chat_messages", Col: "sent_at", FromMonth: old, NowAt: now, MonthsAhead: 1}); err != nil {
		t.Fatal(err)
	}
	in := wa.NewIngestor(st, nil)
	if _, err := in.Process(ctx, wa.Message{ID: "OLD1", Account: "6281234504471", ChatJID: wa.UserJID("6281900300102"), FromNumber: "6281900300102",
		FromName: "Mbak Rina", Text: "Order biasa ya mas", Time: old}); err != nil {
		t.Fatal(err)
	}
	var fp1 string
	_ = st.Pool.QueryRow(ctx, "select md5(string_agg(slug || metrics_current::text, ',' order by slug)) from dealers").Scan(&fp1)
	daily := count(t, st, "select count(*) from dealer_metrics_daily")
	orders := count(t, st, "select count(*) from orders")
	recent := count(t, st, "select count(*) from chat_messages where sent_at >= $1", now.AddDate(0, 0, -365))
	signals := count(t, st, "select count(*) from signals")

	r, err := ops.Purge(ctx, st, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.PartitionsDropped != 1 || count(t, st, "select count(*) from pg_class where relname = 'chat_messages_y2025m09'") != 0 {
		t.Fatalf("September 2025 chat partition not dropped: %+v", r)
	}
	if r.ChatMessages != 1 || count(t, st, "select count(*) from chat_messages where sent_at < $1", now.AddDate(0, 0, -365)) != 0 {
		t.Fatalf("purge %+v", r)
	}
	if count(t, st, "select count(*) from chat_message_keys where wa_msg_id = 'OLD1'") != 0 {
		t.Fatal("dedupe key of a purged message kept")
	}
	var fp2 string
	_ = st.Pool.QueryRow(ctx, "select md5(string_agg(slug || metrics_current::text, ',' order by slug)) from dealers").Scan(&fp2)
	if fp1 != fp2 || daily != count(t, st, "select count(*) from dealer_metrics_daily") || orders != count(t, st, "select count(*) from orders") {
		t.Fatal("purge touched aggregates")
	}
	if a, b := recent, count(t, st, "select count(*) from chat_messages"); a != b {
		t.Fatalf("recent chat %d → %d", a, b)
	}
	if a, b := signals, count(t, st, "select count(*) from signals"); a != b {
		t.Fatalf("signals %d → %d (kept 24 months)", a, b)
	}
}

// pdp delete removes a person and their conversations; orders and metrics stay; the audit keeps only a hash.
func TestPDPDeleteContact(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	exp, err := ops.ExportDealer(ctx, st, "sinar")
	if err != nil {
		t.Fatal(err)
	}
	var e struct {
		Dealer   map[string]any   `json:"dealer"`
		Contacts []map[string]any `json:"contacts"`
		Messages []map[string]any `json:"chat_messages"`
		Orders   []map[string]any `json:"orders"`
	}
	if err := json.Unmarshal(exp, &e); err != nil || e.Dealer["slug"] != "sinar" || len(e.Contacts) == 0 || len(e.Messages) == 0 || len(e.Orders) == 0 {
		t.Fatalf("export: %v %d contacts %d messages %d orders", err, len(e.Contacts), len(e.Messages), len(e.Orders))
	}
	orders := count(t, st, "select count(*) from orders")
	r, err := ops.DeleteContact(ctx, st, "+62 819-0030-0102", "sam@gsi.co.id")
	if err != nil {
		t.Fatal(err)
	}
	if r.Contacts != 1 || r.Messages == 0 || r.Threads == 0 {
		t.Fatalf("delete %+v", r)
	}
	if count(t, st, "select count(*) from contacts where wa_number = '6281900300102'")+
		count(t, st, "select count(*) from chat_messages where from_number = '6281900300102'") != 0 {
		t.Fatal("person still stored")
	}
	if orders != count(t, st, "select count(*) from orders") {
		t.Fatal("orders touched")
	}
	var after string
	_ = st.Pool.QueryRow(ctx, "select after::text from audit_log where action = 'pdp.delete'").Scan(&after)
	if strings.Contains(after, "6281900300102") || !strings.Contains(after, "number_sha256") {
		t.Fatalf("audit %s", after)
	}
	if _, err := ops.DeleteContact(ctx, st, "6281900300102", "sam"); !errors.Is(err, ops.ErrUnknownNumber) {
		t.Fatalf("second delete: %v", err)
	}
}

// Alerts open once per incident and resolve, inside GSI Orbit only: nothing is queued or sent to WhatsApp, and an
// older system row still in the outbox is refused (Sam, 12 Oct: no automatic message).
func TestAlertsNeverSendWhatsApp(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, "update wa_numbers set state = 'disconnected' where wa_number = '6281534509032'"); err != nil {
		t.Fatal(err)
	}
	n := ops.Notifier{St: st}
	h := ops.Check(ctx, st, ops.Env{}, now)
	if h.Status != "degraded" {
		t.Fatalf("health %s %v", h.Status, h.Problems)
	}
	lines, err := n.Run(ctx, h, now)
	if err != nil || len(lines) != 1 || !strings.Contains(lines[0], "Dewi") {
		t.Fatalf("alert %v %v", lines, err)
	}
	if again, _ := n.Run(ctx, ops.Check(ctx, st, ops.Env{}, now), now.Add(5*time.Minute)); len(again) != 0 {
		t.Fatalf("alert repeated: %v", again)
	}
	_, _ = st.Pool.Exec(ctx, "update wa_numbers set state = 'connected'")
	rec, _ := n.Run(ctx, ops.Check(ctx, st, ops.Env{}, now), now.Add(10*time.Minute))
	if len(rec) != 1 || !strings.HasPrefix(rec[0], "✅ Pulih") {
		t.Fatalf("recovery %v", rec)
	}
	var queued int
	_ = st.Pool.QueryRow(ctx, "select count(*) from outbox where channel = 'wa_system'").Scan(&queued)
	if queued != 0 {
		t.Fatalf("%d system messages queued", queued)
	}
	// an old system row, even to an internal group: refused by the sender and by the bridge's approval check
	var id uuid.UUID
	_ = st.Pool.QueryRow(ctx, `insert into outbox (channel, to_ref, payload) values ('wa_system', $1, $2) returning id`,
		"120363041100000001@g.us", `{"from":"6281234504471","to":"120363041100000001@g.us","text":"x"}`).Scan(&id)
	if ok, _ := st.Q.OutboxApprovedForSend(ctx, id); ok {
		t.Fatal("bridge would accept a system message")
	}
	fake := wa.NewFake()
	if _, err := outbox.NewSender(st, fake, clock.Fixed(now), outbox.Rules{}).Send(ctx, id); !errors.Is(err, outbox.ErrNoAutoMessage) {
		t.Fatalf("system row: %v", err)
	}
	if s := fake.Sent(); len(s) != 0 {
		t.Fatalf("sent %+v", s)
	}
}

// A number removed from the phone's linked devices shows a QR again after a bridge restart ("pairing"): it still does
// not sync, so the alert stays open until it is linked again. A number being linked for the first time is not down.
func TestRelinkingNumberStaysDown(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, "update wa_numbers set state = 'pairing', paired_at = $1, state_since = $2 where wa_number = '6281534509032'", now.AddDate(0, 0, -3), now.Add(-30*time.Second)); err != nil {
		t.Fatal(err)
	}
	// 30 s after a bridge restart: reconnecting, not removed
	if al := ops.Alerts(ops.Check(ctx, st, ops.Env{}, now)); len(al) != 0 {
		t.Fatalf("restart alerted: %+v", al)
	}
	if _, err := st.Pool.Exec(ctx, "update wa_numbers set state_since = $1 where wa_number = '6281534509032'", now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	h := ops.Check(ctx, st, ops.Env{}, now)
	al := ops.Alerts(h)
	if h.Status != "degraded" || len(al) != 1 || !strings.Contains(al[0].Message, "perangkat tertaut dihapus") {
		t.Fatalf("health %s alerts %+v", h.Status, al)
	}
	if _, err := st.Pool.Exec(ctx, "update wa_numbers set paired_at = null where wa_number = '6281534509032'"); err != nil {
		t.Fatal(err)
	}
	if al := ops.Alerts(ops.Check(ctx, st, ops.Env{}, now)); len(al) != 0 {
		t.Fatalf("first pairing alerted: %+v", al)
	}
}

// A cycle still running does not "recover" a failed streak (the alert used to flap every hour).
func TestRunningCycleKeepsFailedStreak(t *testing.T) {
	st := setup(t)
	ctx := context.Background()
	for i, status := range []string{"failed", "failed", "failed", "running"} {
		if _, err := st.Pool.Exec(ctx, `insert into cycles (trigger, scope, status, started_at) values ('schedule', 'all', $1, $2)`,
			status, now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	al := ops.Alerts(ops.Check(ctx, st, ops.Env{}, now.Add(4*time.Hour)))
	found := false
	for _, a := range al {
		found = found || a.Key == "cycle_failed"
	}
	if !found {
		t.Fatalf("streak broken by the running cycle: %+v", al)
	}
}
