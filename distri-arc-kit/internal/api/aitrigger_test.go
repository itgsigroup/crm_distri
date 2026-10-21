package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/llm"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// stubModel answers every request with one draft, as a real model would (not the fake provider).
type stubModel struct {
	draft string
	last  llm.Request
}

func (m *stubModel) Name() string  { return "test" }
func (m *stubModel) Model() string { return "test-model" }
func (m *stubModel) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	m.last = r
	b, _ := json.Marshal(map[string]string{"draft": m.draft})
	return llm.Response{JSON: b, Provider: "test", Model: "test-model"}, nil
}

// ADR 0041: AI Trigger — kinds of answer beyond stock (price, credit, order), several cards on one message, and an
// AI draft that keeps the numbers: one that adds a number is rejected, and no template draft is ever shown in its
// place (Sam 21 Oct). Nothing is sent.
func TestAITrigger(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	model := &stubModel{}
	h := api.New(config.Config{Env: "dev"}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).WithReplyModel(model).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ceo := "sam@gsi.co.id"
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty, unit_cost, value) values ('Semarang', 'TST-NVR', 'NVR Uji 32ch', 'Kamera & NVR', 9, 1000000, 9000000);
		insert into products (sku, name, list_price, prices) values ('TST-NVR', 'NVR Uji 32ch', 2500000, '{"A":2500000,"B":2625000,"C":2750000}')`); err != nil {
		t.Fatal(err)
	}
	code, list := get(t, srv, "/api/reply-rules", ceo)
	if code != 200 || list["ai_ready"] != true || list["placeholders"].(map[string]any)["credit"] == nil {
		t.Fatalf("rules: %d %v", code, list)
	}
	byAction := map[string]map[string]any{}
	for _, x := range list["items"].([]any) {
		r := x.(map[string]any)
		byAction[r["action"].(string)] = r
	}

	// several kinds on one sentence, for a dealer: the dealer's tier price and its credit
	_, res := post(t, srv.URL+"/api/reply-rules/test", ceo, map[string]any{"text": "harga NVR uji 32ch berapa, sisa limit saya berapa?", "dealer": "mitra"})
	cards := res["suggestions"].([]any)
	if len(cards) != 2 || res["dealer"] == "" {
		t.Fatalf("price + credit cards: %v", res)
	}
	kinds := map[string]map[string]any{}
	for _, c := range cards {
		kinds[c.(map[string]any)["action"].(string)] = c.(map[string]any)
	}
	if p := kinds["price"]; p == nil || p["ready"] != true || p["product"] != "NVR Uji 32ch" {
		t.Fatalf("price card: %v", kinds["price"])
	}
	if c := kinds["credit"]; c == nil || c["headline"] == "" || len(c["facts"].([]any)) == 0 {
		t.Fatalf("credit card: %v", kinds["credit"])
	}

	// the stock rule asks the AI; a sales cannot change rules
	stock := byAction["stock"]
	stock["ai_phrase"], stock["ai_instruction"] = true, "Tutup dengan 'Terima kasih'."
	if code, _ := sendJSON(t, "PUT", srv.URL+"/api/reply-rules/"+stock["id"].(string), "andi@gsi.co.id", stock); code != http.StatusForbidden {
		t.Fatalf("sales changed a rule: %d", code)
	}
	if code, out := sendJSON(t, "PUT", srv.URL+"/api/reply-rules/"+stock["id"].(string), ceo, stock); code != 200 || out["ai_phrase"] != true {
		t.Fatalf("save: %d %v", code, out)
	}
	if code, _ := sendJSON(t, "PUT", srv.URL+"/api/reply-rules/"+stock["id"].(string), ceo, map[string]any{"name": "x", "action": "diskon", "keywords": []string{"x"}, "template_ready": "a", "template_empty": "b"}); code != http.StatusBadRequest {
		t.Fatalf("unknown action saved: %d", code)
	}
	model.draft = "Siap Pak, NVR Uji 32ch ready 9 pcs di Semarang. Terima kasih."
	_, res = post(t, srv.URL+"/api/reply-rules/test", ceo, map[string]any{"text": "stok NVR uji 32ch ada?", "branch": "Semarang", "ai": true})
	if c := res["suggestions"].([]any)[0].(map[string]any); c["ai"] != true || c["draft"] != model.draft {
		t.Fatalf("AI preview: %v", res)
	}

	var tid, mid string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000301@s.whatsapp.net', 'Toko Uji', '6281234504471', now()) returning id").Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, 'AT1', 'in', 'stok NVR uji 32ch ada?', now() - interval '1 hour', 'received') returning id", tid).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	draftURL := srv.URL + "/api/chat/threads/" + tid + "/messages/" + mid + "/ai-draft"
	// a draft with a number that is not in the facts is rejected
	model.draft = "Ready 10 pcs Pak."
	if code, out := post(t, draftURL, ceo, map[string]any{"rule_id": stock["id"]}); code != http.StatusConflict {
		t.Fatalf("invented number accepted: %d %v", code, out)
	}
	model.draft = "Siap Pak, NVR Uji 32ch ready 9 pcs di Semarang. Terima kasih."
	if code, out := post(t, draftURL, ceo, map[string]any{"rule_id": stock["id"]}); code != 200 || out["written"].(float64) != 1 {
		t.Fatalf("ai draft: %d %v", code, out)
	}
	_, th := get(t, srv, "/api/chat/threads/"+tid, ceo)
	c := th["messages"].([]any)[0].(map[string]any)["suggest"].([]any)[0].(map[string]any)
	if c["ai"] != true || c["draft"] != model.draft || c["ai_model"] != "test-model" {
		t.Fatalf("card with the AI draft: %v", c)
	}
	// new facts (the stock changed): the old AI draft no longer applies
	if _, err := st.Pool.Exec(ctx, "update stock_items set qty = 0 where sku = 'TST-NVR'"); err != nil {
		t.Fatal(err)
	}
	_, th = get(t, srv, "/api/chat/threads/"+tid, ceo)
	if c := th["messages"].([]any)[0].(map[string]any)["suggest"].([]any)[0].(map[string]any); c["ai"] == true || c["ready"] == true ||
		c["draft"] != nil || c["ai_error"] == nil {
		// nor the template in its place: the AI writes again (here no job queue, so the card says the AI did not write)
		t.Fatalf("stale AI draft or the template shown: %v", c)
	}

	// "stoknya ada?" names no product: the card and the AI read the chat before it
	var t2, m2 string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000302@s.whatsapp.net', 'Toko Dua', '6281234504471', now()) returning id").Scan(&t2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values
		($1, 'AH1', 'in', 'pak NVR uji 32ch harganya berapa?', now() - interval '3 hours', 'received'),
		($1, 'AH2', 'out', 'Baik pak kami cek dulu', now() - interval '2 hours', 'sent'),
		($1, 'AH3', 'in', 'stoknya ada?', now() - interval '1 hour', 'received')`, t2); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "select id from chat_messages where wa_msg_id = 'AH3'").Scan(&m2); err != nil {
		t.Fatal(err)
	}
	_, th = get(t, srv, "/api/chat/threads/"+t2, ceo)
	last := th["messages"].([]any)[2].(map[string]any)
	if cs, _ := last["suggest"].([]any); len(cs) != 1 || cs[0].(map[string]any)["product"] != "NVR Uji 32ch" || cs[0].(map[string]any)["from_history"] != "pak NVR uji 32ch harganya berapa?" {
		t.Fatalf("product from the chat history: %v", last["suggest"])
	}
	model.draft = "Untuk NVR Uji 32ch saat ini sedang kosong Pak."
	if code, out := post(t, srv.URL+"/api/chat/threads/"+t2+"/messages/"+m2+"/ai-draft", ceo, map[string]any{"rule_id": stock["id"]}); code != 200 {
		t.Fatalf("ai draft with history: %d %v", code, out)
	}
	var in struct {
		Riwayat []map[string]string `json:"riwayat"`
		Produk  string              `json:"produk_dari_riwayat"`
	}
	if err := json.Unmarshal(model.last.Input, &in); err != nil || len(in.Riwayat) != 2 || in.Riwayat[0]["dari"] != "pelanggan" || in.Riwayat[1]["dari"] != "kami" || in.Produk == "" {
		t.Fatalf("the AI did not get the chat history (oldest first): %s", model.last.Input)
	}
	if n := countRows(t, st, "select count(*) from outbox"); n != 0 {
		t.Errorf("AI Trigger queued %d messages", n)
	}
}
