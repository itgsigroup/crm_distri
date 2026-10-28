package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/autoreply"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/llm"
	"distri-arc/internal/outbox"
	"distri-arc/internal/replyai"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
)

// echoModel writes the facts' draft back as its answer: a model that keeps every number.
type echoModel struct{ calls int }

func (m *echoModel) Name() string  { return "test" }
func (m *echoModel) Model() string { return "test-model" }
func (m *echoModel) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	m.calls++
	var in struct {
		Draf string `json:"draf"`
	}
	_ = json.Unmarshal(r.Input, &in)
	b, _ := json.Marshal(map[string]string{"draft": in.Draf})
	return llm.Response{JSON: b, Provider: "test", Model: "test-model"}, nil
}

// ADR 0066: Balas otomatis saat istirahat — only the CEO turns it on, the template must read as an automatic message,
// a fact question gets the system's answer written by AI, anything else the template once per break, nothing when a
// person answered lately or outside the break, and a job run twice sends once.
func TestAutoReplyDuringBreak(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 20, 0, 0, clock.WIB) // Selasa, jam istirahat
	c := clock.Fixed(now)
	srv := httptest.NewServer(api.New(config.Config{Env: "dev"}, st, c, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(srv.Close)
	ceo := "sam@gsi.co.id"

	if code, _ := get(t, srv, "/api/auto-reply", "andi@gsi.co.id"); code != http.StatusForbidden {
		t.Fatalf("sales reads the setting: %d", code)
	}
	on := map[string]any{"enabled": true, "breaks": []map[string]any{{"days": []int{1, 2, 3, 4, 5, 6}, "start": "12:00", "end": "13:00"}},
		"template": autoreply.DefaultTemplate, "facts": []string{"stock", "price", "order"}}
	if code, _ := sendJSON(t, http.MethodPut, srv.URL+"/api/auto-reply", "andi@gsi.co.id", on); code != http.StatusForbidden {
		t.Fatalf("sales turns it on: %d", code)
	}
	bad := map[string]any{"enabled": true, "breaks": on["breaks"], "template": "Halo, sales sedang makan siang, nanti dibalas ya Kak.", "facts": []string{}}
	if code, _ := sendJSON(t, http.MethodPut, srv.URL+"/api/auto-reply", ceo, bad); code != http.StatusBadRequest {
		t.Fatalf("a template that does not read as automatic: %d", code)
	}
	late := map[string]any{"enabled": true, "breaks": []map[string]any{{"days": []int{1}, "start": "18:30", "end": "19:00"}}, "template": autoreply.DefaultTemplate}
	if code, _ := sendJSON(t, http.MethodPut, srv.URL+"/api/auto-reply", ceo, late); code != http.StatusBadRequest {
		t.Fatalf("a break outside the send window: %d", code)
	}
	code, out := sendJSON(t, http.MethodPut, srv.URL+"/api/auto-reply", ceo, on)
	if code != http.StatusOK || out["active_now"] != true || out["settings"].(map[string]any)["approved_name"] == "" {
		t.Fatalf("CEO turns it on: %d %v", code, out)
	}

	model := &echoModel{}
	svc := &autoreply.Service{St: st, Clock: c, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Reply: &replyai.Service{St: st, Views: views.NewBuilder(st, c), Provider: model}}
	th := sinarThread(t, st)
	handle := func(id uuid.UUID, want autoreply.Outcome) {
		t.Helper()
		got, err := svc.Handle(ctx, id)
		if err != nil || got != want {
			t.Fatalf("handle: got %q %v, want %q", got, err, want)
		}
	}

	// anything else: the template, once per break
	q1 := addMessage(t, st, th, "WA-AR1", "in", "received", "Pak, besok bisa mampir ke toko jam 10?", now.Add(-time.Minute), nil)
	handle(q1, autoreply.Template)
	handle(q1, autoreply.Skipped) // the job again: sent once
	var body, kind string
	var auto bool
	if err := st.Pool.QueryRow(ctx, `select m.body, m.auto_reply, o.payload->>'kind' from auto_replies a join chat_messages m on m.id = a.reply_id
		join outbox o on o.proposal_id = a.proposal_id where a.message_id = $1`, q1).Scan(&body, &auto, &kind); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "13.00") || !auto || kind != "auto" {
		t.Fatalf("template reply: %q auto=%v kind=%s", body, auto, kind)
	}
	// it leaves through the outbox like any approved message, outside the live path (anti-ban caps apply)
	var ob uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select o.id from outbox o join auto_replies a on a.proposal_id = o.proposal_id where a.message_id = $1", q1).Scan(&ob); err != nil {
		t.Fatal(err)
	}
	fake := wa.NewFake()
	if _, err := outbox.NewSender(st, fake, c, outbox.Rules{SendFrom: 8, SendTo: 18}).Send(ctx, ob); err != nil {
		t.Fatal(err)
	}
	if s := fake.Sent(); len(s) != 1 || s[0].Text != body {
		t.Fatalf("transport got %+v", s)
	}
	if count(t, st, "select count(*) from proposals where agent = 'Balas otomatis' and status in ('approved', 'executed') and decided_by = (select sales_user_id from users where email = 'sam@gsi.co.id')") != 1 {
		t.Fatal("automatic reply not recorded as decided by the approver")
	}
	q2 := addMessage(t, st, th, "WA-AR2", "in", "received", "Halo?", now.Add(-30*time.Second), nil)
	handle(q2, autoreply.Skipped) // the template went out already this break

	// a fact question: the system's answer, written by AI
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty, unit_cost, value) values ('Semarang', 'TST-NVR', 'NVR Uji 32ch', 'Kamera & NVR', 9, 1000000, 9000000);
		insert into products (sku, name, list_price, prices) values ('TST-NVR', 'NVR Uji 32ch', 2500000, '{"A":2500000,"B":2625000,"C":2750000}')`); err != nil {
		t.Fatal(err)
	}
	q3 := addMessage(t, st, th, "WA-AR3", "in", "received", "stok NVR uji 32ch ada?", now.Add(-20*time.Second), nil)
	handle(q3, autoreply.Fact)
	if model.calls == 0 || count(t, st, "select count(*) from auto_replies a join chat_messages m on m.id = a.reply_id where a.message_id = $1 and a.kind = 'fact' and m.body like '%9%'", q3) != 1 {
		t.Fatal("fact answer not sent from the stock")
	}

	// a person answered from the phone a moment ago: they are at work
	addMessage(t, st, th, "WA-AR4", "out", "sent", "Sebentar ya Pak saya cek", now.Add(-2*time.Minute), nil)
	q5 := addMessage(t, st, th, "WA-AR5", "in", "received", "stok NVR uji 32ch masih ada?", now.Add(-10*time.Second), nil)
	handle(q5, autoreply.Skipped)

	// outside the break: nothing
	after := &autoreply.Service{St: st, Clock: clock.Fixed(now.Add(time.Hour)), Reply: svc.Reply}
	q6 := addMessage(t, st, th, "WA-AR6", "in", "received", "Pak?", now.Add(time.Hour-time.Minute), nil)
	if got, err := after.Handle(ctx, q6); err != nil || got != autoreply.Skipped {
		t.Fatalf("after the break: %q %v", got, err)
	}
}
