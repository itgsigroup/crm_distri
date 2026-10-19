package orchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
)

type screenInput struct {
	Items []map[string]any `json:"items"`
}

// Analisa Stok and Analisa Kas (ADR 0030): the page readings are written by Claude through MCP, checked against the
// Input it was given (keys, lengths), unmasked and stored as AI; GSI Orbit writes none itself.
func TestScreenReadingsThroughMCP(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	cid := rep.Cycle.ID
	if _, err := st.Q.LatestScreenAnalysis(ctx, "stock"); err == nil {
		t.Fatal("no AI answered: nothing may be stored for the page")
	}

	raw, err := o.InputFor(ctx, cid, orchestrator.AgentStockRead)
	if err != nil {
		t.Fatal(err)
	}
	var in screenInput
	_ = json.Unmarshal(raw, &in)
	if len(in.Items) == 0 || in.Items[0]["key"] == nil || in.Items[0]["age_days"] == nil {
		t.Fatalf("stock input %s", raw)
	}
	key := in.Items[0]["key"].(string)
	ans := orchestrator.ParseScreenAnswer("Stok menua terbesar ada di Semarang; dorong ke dealer yang jadwal ordernya minggu ini.",
		[]map[string]any{{"key": key, "note": "Tawarkan ke dua dealer dengan jadwal order terdekat minggu ini.", "confidence": 0.8}})
	if _, err := o.ScreenSubmit(ctx, cid, orchestrator.AgentStockRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	a, err := st.Q.LatestScreenAnalysis(ctx, "stock")
	if err != nil || a.Source != "ai" || a.CycleID != cid || !strings.Contains(string(a.Lines), key) || !strings.Contains(string(a.Lines), `"confidence": 80`) && !strings.Contains(string(a.Lines), `"confidence":80`) {
		t.Fatalf("stored %+v %s %v", a, a.Lines, err)
	}
	bad := orchestrator.ParseScreenAnswer("x", []map[string]any{{"key": "not-in-input", "note": "y", "confidence": 0.5}})
	if _, err := o.ScreenSubmit(ctx, cid, orchestrator.AgentStockRead, bad, nil); !errors.Is(err, orchestrator.ErrBadSubmit) {
		t.Fatalf("unknown item accepted: %v", err)
	}

	// Analisa Kas: the sales' names leave the server masked, and come back unmasked
	raw, err = o.InputFor(ctx, cid, orchestrator.AgentCashRead)
	if err != nil {
		t.Fatal(err)
	}
	in = screenInput{}
	_ = json.Unmarshal(raw, &in)
	if len(in.Items) == 0 {
		t.Fatalf("cash input %s", raw)
	}
	sales, _ := in.Items[0]["sales"].(string)
	if !strings.HasPrefix(sales, "<PIC_") {
		t.Fatalf("sales name not masked: %q", sales)
	}
	ans = orchestrator.ParseScreenAnswer("Kas 30 hari bergantung pada tiga dealer besar.",
		[]map[string]any{{"key": in.Items[0]["key"], "note": sales + " menagih invoice terlama minggu ini.", "confidence": 0.7}})
	if _, err := o.ScreenSubmit(ctx, cid, orchestrator.AgentCashRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	k, err := st.Q.LatestScreenAnalysis(ctx, "credit")
	if err != nil || strings.Contains(string(k.Lines), "<PIC_") {
		t.Fatalf("cash reading %s %v", k.Lines, err)
	}
}

// ADR 0052: Analisa Orbit — the dealers in orbit go to Claude with their position (from the order cycle) and their
// WhatsApp messages; the answer is checked against that Input and stored for the Orbit page.
func TestOrbitReadingThroughMCP(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, domain.Scope{Kind: "screen", ID: "orbit"}, sched)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := o.InputFor(ctx, rep.Cycle.ID, orchestrator.AgentOrbitRead)
	if err != nil {
		t.Fatal(err)
	}
	var in screenInput
	_ = json.Unmarshal(raw, &in)
	chatting := 0
	for _, it := range in.Items {
		if it["key"] == nil || it["status"] == nil || it["cyc"] == nil {
			t.Fatalf("orbit item %v", it)
		}
		if c, _ := it["chat_terakhir"].([]any); len(c) > 0 {
			chatting++
		}
	}
	if len(in.Items) == 0 || chatting == 0 || !strings.Contains(string(raw), "siklus order") {
		t.Fatalf("orbit input (%d with chats) %s", chatting, raw)
	}
	key := in.Items[0]["key"].(string)
	ans := orchestrator.ParseScreenAnswer("Dua dealer At risk menunda order karena proyek belum cair; sapa minggu depan.",
		[]map[string]any{{"key": key, "note": "Proyek belum cair menurut chat — sapa lagi Senin dengan stok yang dia tanyakan.", "confidence": 0.7}})
	if _, err := o.ScreenSubmit(ctx, rep.Cycle.ID, orchestrator.AgentOrbitRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	a, err := st.Q.LatestScreenAnalysis(ctx, "orbit")
	if err != nil || a.Source != "ai" || !strings.Contains(string(a.Lines), key) {
		t.Fatalf("orbit reading %+v %v", a, err)
	}
}

// ADR 0053: Analisa Segmen — the dealers moving between segments go to Claude with their segment now and three months
// ago (from orders) and their WhatsApp messages; the answer is stored for the Segmen page.
func TestSegmenReadingThroughMCP(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, domain.Scope{Kind: "screen", ID: "segmen"}, sched)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := o.InputFor(ctx, rep.Cycle.ID, orchestrator.AgentSegmenRead)
	if err != nil {
		t.Fatal(err)
	}
	var in screenInput
	_ = json.Unmarshal(raw, &in)
	if len(in.Items) == 0 || in.Items[0]["segmen"] == nil || !strings.Contains(string(raw), "chat_terakhir") || !strings.Contains(string(raw), "dihitung dari order") {
		t.Fatalf("segmen input %s", raw)
	}
	key := in.Items[0]["key"].(string)
	ans := orchestrator.ParseScreenAnswer("Satu dealer turun dari A ke B karena order dipecah kecil menunggu limit.",
		[]map[string]any{{"key": key, "note": "Order dipecah kecil menurut chat — tawarkan bundle untuk menaikkan nilai order.", "confidence": 0.7}})
	if _, err := o.ScreenSubmit(ctx, rep.Cycle.ID, orchestrator.AgentSegmenRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	if a, err := st.Q.LatestScreenAnalysis(ctx, "segmen"); err != nil || !strings.Contains(string(a.Lines), key) {
		t.Fatalf("segmen reading %+v %v", a, err)
	}
}

// ADR 0054: Analisa Relasi — dealers with who writes to them (per sales) and their messages go to Claude; Analisis
// ulang on one page asks only that page's reading, a full cycle all of them.
func TestRelasiReadingThroughMCP(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, domain.Scope{Kind: "screen", ID: "relasi"}, sched)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := o.InputFor(ctx, rep.Cycle.ID, orchestrator.AgentRelasiRead)
	if err != nil {
		t.Fatal(err)
	}
	var in screenInput
	_ = json.Unmarshal(raw, &in)
	if len(in.Items) == 0 || in.Items[0]["chat_per_sales"] == nil || !strings.Contains(string(raw), "interaksi WA + order") {
		t.Fatalf("relasi input %s", raw)
	}
	key := in.Items[0]["key"].(string)
	ans := orchestrator.ParseScreenAnswer("Hubungan menguat di dealer yang aktif chat; satu dealer besar hanya bicara tagihan.",
		[]map[string]any{{"key": key, "note": "Responsif dan banyak tanya stok — pertahankan sapaan mingguan dari sales pemilik.", "confidence": 0.7}})
	if _, err := o.ScreenSubmit(ctx, rep.Cycle.ID, orchestrator.AgentRelasiRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	if a, err := st.Q.LatestScreenAnalysis(ctx, "relasi"); err != nil || !strings.Contains(string(a.Lines), key) {
		t.Fatalf("relasi reading %+v %v", a, err)
	}
	fu := []string{"AI Follow-up", "AI Kredit"}
	if got := orchestrator.ScreenReadingsIn("screen:relasi", fu); len(got) != 2 || got[0] != orchestrator.AgentRelasiRead {
		t.Fatalf("relasi page asks %v", got)
	}
	if got := orchestrator.ScreenReadingsIn("all", fu); len(got) != 4 {
		t.Fatalf("full cycle asks %v", got)
	}
	if p := orchestrator.MCPPrompt(3, "c", fu, 0, "screen:orbit"); !strings.Contains(p, orchestrator.AgentOrbitRead) || strings.Contains(p, orchestrator.AgentSegmenRead) {
		t.Fatalf("orbit prompt %s", p)
	}
}

// ADR 0055: Analisa Dealer — Analisa AI on a dealer asks Claude for that dealer only: its numbers and chats in, a
// status from three, reasons and a WhatsApp draft out; anything else is refused.
func TestDealerReadingThroughMCP(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, domain.Scope{Kind: "dealer", ID: "mitra"}, sched)
	if err != nil {
		t.Fatal(err)
	}
	if got := orchestrator.ScreenReadingsIn("dealer:mitra", []string{"AI Follow-up", "AI Kredit", "AI Stok"}); len(got) != 1 || got[0] != orchestrator.AgentDealerRead {
		t.Fatalf("dealer scope asks %v", got)
	}
	raw, err := o.InputFor(ctx, rep.Cycle.ID, orchestrator.AgentDealerRead)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"status"`) || !strings.Contains(string(raw), `"wa"`) || !strings.Contains(string(raw), "invoice_terbuka") {
		t.Fatalf("dealer input %s", raw)
	}
	bad := orchestrator.ParseScreenAnswer("x", []map[string]any{{"key": "status", "note": "gawat", "confidence": 0.5}, {"key": "wa", "note": "Halo", "confidence": 0.5}})
	if _, err := o.ScreenSubmit(ctx, rep.Cycle.ID, orchestrator.AgentDealerRead, bad, nil); !errors.Is(err, orchestrator.ErrBadSubmit) {
		t.Fatalf("unknown status accepted: %v", err)
	}
	ans := orchestrator.ParseScreenAnswer("Order lewat jadwal 9 hari dan satu invoice telat; dealer menunggu proyek cair.", []map[string]any{
		{"key": "status", "note": "bermasalah", "confidence": 0.8},
		{"key": "alasan_1", "note": "Invoice telat 12 hari.", "confidence": 0.8},
		{"key": "wa", "note": "Selamat siang Pak, mengingatkan invoice yang jatuh tempo minggu lalu. Ada kendala yang bisa kami bantu?", "confidence": 0.8}})
	if _, err := o.ScreenSubmit(ctx, rep.Cycle.ID, orchestrator.AgentDealerRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	var status, draft string
	if err := st.Pool.QueryRow(ctx, "select a.status, a.wa_draft from dealer_analyses a join dealers d on d.id = a.dealer_id where d.slug = 'mitra'").Scan(&status, &draft); err != nil || status != "bermasalah" || !strings.Contains(draft, "invoice") {
		t.Fatalf("stored %q %q %v", status, draft, err)
	}
}

// The Analisis ulang prompt asks Claude for the page readings of the agents in scope.
func TestMCPPromptAsksForPageReadings(t *testing.T) {
	p := orchestrator.MCPPrompt(7, "c1", []string{"AI Stok"}, 0)
	if !strings.Contains(p, `"agent": "Analisa Stok"`) || strings.Contains(p, "Analisa Kas") {
		t.Errorf("stock prompt:\n%s", p)
	}
	p = orchestrator.MCPPrompt(7, "c1", []string{"AI Kredit", "AI Penagihan"}, 0)
	if !strings.Contains(p, `"agent": "Analisa Kas"`) || strings.Contains(p, "Analisa Stok") {
		t.Errorf("credit prompt:\n%s", p)
	}
}

// screenStub answers the page readings like a model would (every other purpose: its fallback).
type screenStub struct{ purposes []string }

func (m *screenStub) Name() string  { return "stub" }
func (m *screenStub) Model() string { return "claude-sonnet-5-5" }
func (m *screenStub) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	if !strings.HasPrefix(r.Purpose, "screen.") {
		return llm.Response{JSON: r.Fallback, Provider: "stub", Model: "claude-sonnet-5-5"}, nil
	}
	m.purposes = append(m.purposes, r.Purpose)
	var in screenInput
	_ = json.Unmarshal(r.Input, &in)
	var lines []map[string]any
	for _, it := range in.Items {
		lines = append(lines, map[string]any{"key": it["key"], "note": "Langkah berikutnya untuk sales.", "confidence": 0.6})
	}
	b, _ := json.Marshal(map[string]any{"overview": "Ringkasan untuk CEO.", "lines": lines})
	return llm.Response{JSON: b, Provider: "stub", Model: "claude-sonnet-5-5"}, nil
}

// With the server's model, a screen cycle writes that page's reading as AI; a cycle sent to MCP leaves it to Claude.
func TestScreenReadingByModel(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	stub := &screenStub{}
	o.Router = &llm.Router{Primary: stub, St: st, Log: o.Log}
	if _, err := o.Run(ctx, domain.Scope{Kind: "screen", ID: "stock"}, sched); err != nil {
		t.Fatal(err)
	}
	if len(stub.purposes) != 1 || stub.purposes[0] != "screen.stock" {
		t.Fatalf("model asked for %v", stub.purposes)
	}
	if a, err := st.Q.LatestScreenAnalysis(ctx, "stock"); err != nil || a.Overview != "Ringkasan untuk CEO." {
		t.Fatalf("stock reading %+v %v", a, err)
	}
	if _, err := st.Q.LatestScreenAnalysis(ctx, "credit"); err == nil {
		t.Error("a stock cycle wrote the cash reading")
	}
}

// Chat → Analisa lanjut on one conversation (scope thread): Claude gets that conversation through MCP and its note per
// customer bubble is stored (ADR 0027), although the cycle is not a full one.
func TestOneConversationBubblesThroughMCP(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	var thread string
	if err := st.Pool.QueryRow(ctx, `select t.id::text from chat_threads t join dealers d on d.id = t.dealer_id where d.slug = 'graha' limit 1`).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	sc, err := domain.ParseScope("thread:" + thread)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := o.Run(ctx, sc, sched)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := o.InputFor(ctx, rep.Cycle.ID, orchestrator.AgentWA)
	if err != nil {
		t.Fatal(err)
	}
	var in struct {
		Threads []struct {
			ID       string `json:"id"`
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
		} `json:"threads"`
	}
	_ = json.Unmarshal(raw, &in)
	if len(in.Threads) != 1 || in.Threads[0].ID != thread {
		t.Fatalf("reading of one conversation: %s", raw)
	}
	var msg string
	for _, m := range in.Threads[0].Messages {
		if m.ID != "" {
			msg = m.ID
		}
	}
	if msg == "" {
		t.Fatal("no customer bubble to annotate")
	}
	ans := orchestrator.ParseWAAnswer("Graha menunggu konfirmasi.", []map[string]any{{"id": thread, "summary": "Menunggu konfirmasi PO.", "confidence": 0.8}},
		map[string]any{"id": msg, "note": "Minta konfirmasi jadwal kirim — jawab hari ini.", "confidence": 0.7})
	if _, err := o.WASubmit(ctx, rep.Cycle.ID, ans, nil); err != nil {
		t.Fatal(err)
	}
	var note string
	if err := st.Pool.QueryRow(ctx, "select coalesce(note, '') from wa_message_insights where message_id = $1", msg).Scan(&note); err != nil || note == "" {
		t.Fatalf("bubble note %q %v", note, err)
	}
}

// ADR 0032: with llm.routing mode "mcp" every analysis comes from Claude through MCP. Nobody answering means no
// proposals (no Go templates, even with require_ai off) and no WhatsApp overview or page reading from the server model.
func TestMCPOnlyWritesNothingWithoutClaude(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `update policies set value = value || '{"mode": "mcp", "require_ai": false}'::jsonb where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	o.MCPWait = 300 * time.Millisecond
	o.Router = &llm.Router{Primary: &modelStub{}, St: st, Log: o.Log}
	if _, err := o.Run(ctx, all, sched); err == nil {
		t.Error("a cycle nobody analysed through MCP should not succeed")
	}
	c, err := st.Q.LatestCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, "select count(*) from proposals where cycle_id = $1", c.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d proposals stored without Claude answering through MCP", n)
	}
	if a, err := st.Q.LatestWAAnalysis(ctx); err == nil && (a.Source == "ai" || a.Overview != nil) {
		t.Fatalf("WhatsApp overview written outside MCP: %+v", a)
	}
	if _, err := st.Q.LatestScreenAnalysis(ctx, "stock"); err == nil {
		t.Fatal("page reading written outside MCP")
	}
}
