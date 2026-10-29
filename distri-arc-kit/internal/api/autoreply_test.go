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
	"distri-arc/internal/hermes"
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

// lyraAgent is a Hermes Agent that names one product in every message.
type lyraAgent struct {
	sku   string
	calls int
}

func (a *lyraAgent) Chat(context.Context, string, []hermes.Msg) (string, error) {
	a.calls++
	return `{"tanya_produk": true, "items": [{"sku": "` + a.sku + `", "nama": "", "qty": 2}], "catatan": ""}`, nil
}

// ADR 0077: during a break a product question is answered by Lyra — the products she named, with the stock and DPP
// price from the system — and recorded as her answer; without Lyra connected it falls back to AI Trigger.
func TestAutoReplyByLyra(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 20, 0, 0, clock.WIB) // Selasa, jam istirahat
	c := clock.Fixed(now)
	srv := httptest.NewServer(api.New(config.Config{Env: "dev"}, st, c, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(srv.Close)
	on := map[string]any{"enabled": true, "lyra": true, "breaks": []map[string]any{{"days": []int{2}, "start": "12:00", "end": "13:00"}},
		"template": autoreply.DefaultTemplate, "facts": []string{"stock", "price", "order"}}
	code, out := sendJSON(t, http.MethodPut, srv.URL+"/api/auto-reply", "sam@gsi.co.id", on)
	if code != http.StatusOK || out["settings"].(map[string]any)["lyra"] != true {
		t.Fatalf("CEO turns Lyra on: %d %v", code, out)
	}
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty, unit_cost, value) values ('Semarang', 'TST-NVR', 'NVR Uji 32ch', 'Kamera & NVR', 9, 1000000, 9000000);
		insert into products (sku, name, list_price, prices) values ('TST-NVR', 'NVR Uji 32ch', 2500000, '{"A":2500000,"B":2625000,"C":2750000}')`); err != nil {
		t.Fatal(err)
	}
	agent := &lyraAgent{sku: "TST-NVR"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reply := &replyai.Service{St: st, Views: views.NewBuilder(st, c), Provider: &echoModel{}}
	svc := &autoreply.Service{St: st, Clock: c, Log: log, Reply: reply, Lyra: &hermes.Service{Pool: st.Pool, Log: log, Agent: agent}}
	th := sinarThread(t, st)

	q1 := addMessage(t, st, th, "WA-LY1", "in", "received", "harga nvr 32 channel berapa? stok ada?", now.Add(-time.Minute), nil)
	if got, err := svc.Handle(ctx, q1); err != nil || got != autoreply.Lyra {
		t.Fatalf("product question: %q %v", got, err)
	}
	var body string
	var auto bool
	if err := st.Pool.QueryRow(ctx, `select m.body, m.auto_reply from auto_replies a join chat_messages m on m.id = a.reply_id where a.message_id = $1 and a.kind = 'lyra'`, q1).Scan(&body, &auto); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NVR Uji 32ch", "Semarang 9", "/unit", "2 unit ×", "Pesan otomatis dari Lyra"} {
		if !strings.Contains(body, want) {
			t.Fatalf("Lyra's answer lacks %q:\n%s", want, body)
		}
	}
	if !auto || agent.calls != 1 {
		t.Fatalf("auto=%v calls=%d", auto, agent.calls)
	}
	if count(t, st, "select count(*) from hermes_product_reads where message_id = $1 and status = 'done'", q1) != 1 {
		t.Fatal("the reading is not kept for the chat's card")
	}

	// Lyra not connected: the AI Trigger answer
	off := &autoreply.Service{St: st, Clock: c, Log: log, Reply: reply, Lyra: &hermes.Service{Pool: st.Pool, Log: log}}
	q2 := addMessage(t, st, th, "WA-LY2", "in", "received", "stok NVR uji 32ch ada?", now.Add(-20*time.Second), nil)
	if got, err := off.Handle(ctx, q2); err != nil || got != autoreply.Fact {
		t.Fatalf("without Lyra: %q %v", got, err)
	}
}

// salesAgent is a Hermes Agent that reads every message as a question about TST-NVR and answers as a sales with the
// reply it is given.
type salesAgent struct {
	reply string
	sales int
}

func (a *salesAgent) Chat(_ context.Context, session string, _ []hermes.Msg) (string, error) {
	if session == "orbit-lyra-sales" {
		a.sales++
		b, _ := json.Marshal(map[string]any{"balasan": a.reply, "perlu_sales": false, "catatan": ""})
		return string(b), nil
	}
	return `{"tanya_produk": true, "items": [{"sku": "TST-NVR", "nama": "", "qty": 2}], "catatan": ""}`, nil
}

// ADR 0078: with "seperti sales" on, Lyra answers any message as a sales would — the customer's name put back, the
// numbers from the system — and a reply with a number the facts do not have is not sent (the product card goes out).
func TestAutoReplyLikeSales(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 20, 0, 0, clock.WIB) // Selasa, jam istirahat
	c := clock.Fixed(now)
	srv := httptest.NewServer(api.New(config.Config{Env: "dev"}, st, c, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(srv.Close)
	on := map[string]any{"enabled": true, "lyra": true, "sales_style": true, "breaks": []map[string]any{{"days": []int{2}, "start": "12:00", "end": "13:00"}},
		"template": autoreply.DefaultTemplate, "facts": []string{"stock", "price", "order"}}
	if code, out := sendJSON(t, http.MethodPut, srv.URL+"/api/auto-reply", "sam@gsi.co.id", on); code != http.StatusOK || out["settings"].(map[string]any)["sales_style"] != true {
		t.Fatalf("CEO turns on sales style: %d %v", code, out)
	}
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty, unit_cost, value) values ('Semarang', 'TST-NVR', 'NVR Uji 32ch', 'Kamera & NVR', 9, 1000000, 9000000);
		insert into products (sku, name, list_price, prices) values ('TST-NVR', 'NVR Uji 32ch', 2500000, '{"A":2500000,"B":2625000,"C":2750000}')`); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reply := &replyai.Service{St: st, Views: views.NewBuilder(st, c), Provider: &echoModel{}}
	agent := &salesAgent{}
	svc := &autoreply.Service{St: st, Clock: c, Log: log, Reply: reply, Lyra: &hermes.Service{Pool: st.Pool, Log: log, Agent: agent}}
	th := sinarThread(t, st)
	var title string
	if err := st.Pool.QueryRow(ctx, "select title from chat_threads where id = $1", th).Scan(&title); err != nil {
		t.Fatal(err)
	}
	body := func(q uuid.UUID) (string, string) {
		t.Helper()
		var b, kind string
		var auto bool
		if err := st.Pool.QueryRow(ctx, `select m.body, a.kind, m.auto_reply from auto_replies a join chat_messages m on m.id = a.reply_id where a.message_id = $1`, q).Scan(&b, &kind, &auto); err != nil {
			t.Fatal(err)
		}
		if !auto {
			t.Fatalf("not marked automatic, the chat would read as answered: %q", b)
		}
		return b, kind
	}

	// a product question: Lyra's own words, the name put back, the numbers from the system
	agent.reply = "Selamat siang {nama}, untuk *NVR Uji 32ch* stok di Semarang ada 9 unit. Harganya *Rp 2.500.000*/unit (harga DPP, belum termasuk PPN), jadi 2 unit Rp 5.000.000. Mau saya bantu siapkan, Pak?"
	q1 := addMessage(t, st, th, "WA-LS1", "in", "received", "pak nvr 32 channel stok ada? harga 2 unit berapa?", now.Add(-time.Minute), nil)
	if got, err := svc.Handle(ctx, q1); err != nil || got != autoreply.Lyra {
		t.Fatalf("product question: %q %v", got, err)
	}
	b, _ := body(q1)
	if !strings.HasPrefix(b, "Selamat siang "+title+",") || !strings.Contains(b, "Rp 5.000.000") || !strings.Contains(b, hermes.SalesFooter) {
		t.Fatalf("sales-style reply:\n%s", b)
	}

	// any other message: answered by Lyra, not the template
	agent.reply = "Baik {nama}, terima kasih kabarnya. Sales kami kembali jam 13.00 dan akan langsung menghubungi Bapak untuk jadwal mampir besok ya."
	q2 := addMessage(t, st, th, "WA-LS2", "in", "received", "Pak, besok bisa mampir ke toko?", now.Add(-50*time.Second), nil)
	if got, err := svc.Handle(ctx, q2); err != nil || got != autoreply.Lyra {
		t.Fatalf("other message: %q %v", got, err)
	}
	if b, _ := body(q2); !strings.Contains(b, "jam 13.00") || strings.Contains(b, "Sales kami sedang istirahat") {
		t.Fatalf("other message got:\n%s", b)
	}

	// a made-up number (a discount): not sent — the product card from the system goes out instead
	agent.reply = "Siap {nama}, NVR Uji 32ch ready 9 unit, khusus hari ini diskon 15% jadi Rp 2.125.000."
	q3 := addMessage(t, st, th, "WA-LS3", "in", "received", "harga nvr 32ch ada diskon?", now.Add(-40*time.Second), nil)
	if got, err := svc.Handle(ctx, q3); err != nil || got != autoreply.Lyra {
		t.Fatalf("made-up number: %q %v", got, err)
	}
	if b, _ := body(q3); strings.Contains(b, "15%") || strings.Contains(b, "2.125.000") || !strings.Contains(b, "Berikut info produk") {
		t.Fatalf("a reply with a made-up number went out:\n%s", b)
	}
	if agent.sales != 3 {
		t.Fatalf("Lyra asked %d times", agent.sales)
	}
}
