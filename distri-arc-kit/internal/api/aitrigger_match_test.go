package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
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

// stubJSON answers every request with one JSON (Claude finding products).
type stubJSON struct {
	out   string
	calls int
	last  llm.Request
}

func (m *stubJSON) Name() string  { return "claude" }
func (m *stubJSON) Model() string { return "claude-haiku-5-5" }
func (m *stubJSON) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	m.calls++
	m.last = r
	return llm.Response{JSON: json.RawMessage(m.out), Provider: "claude", Model: "claude-haiku-5-5"}, nil
}

// Sam 21 Oct: Claude finds the products of a message — both of "H8c 2mp : 8 … sekalian nvr ezviz X5S-8W 1 unit" —
// among candidates from the stock; the card shows each with its stock against what was asked; the reading is stored
// once per message; a SKU that is not a candidate is refused.
func TestAITriggerClaudeMatch(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	drafts := &stubModel{draft: "Siap Pak, H8c 2mp ready 207 unit jadi 8 cukup, NVR X5S 8W ready 42 unit."}
	claude := &stubJSON{out: `{"items":[{"sku":"H8c 2MP","qty":8},{"sku":"X5S_8W","qty":1}],"catatan":""}`}
	h := api.New(config.Config{Env: "dev"}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).WithReplyModel(drafts).WithReplyMatcher(claude).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ceo := "sam@gsi.co.id"
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty) values
		('Semarang', 'H8c 2MP', 'Ezviz Camera H8c 2mp', 'Kamera & NVR', 5), ('Surabaya', 'H8c 2MP', 'Ezviz Camera H8c 2mp', 'Kamera & NVR', 202),
		('Semarang', 'H8C_3MP_Poe', 'Ezviz Camera H8c 3MP PoE', 'Kamera & NVR', 110), ('Semarang', 'X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 'Kamera & NVR', 42)`); err != nil {
		t.Fatal(err)
	}
	text := "Ok, aku pesen\nH8c 2mp : 8 @580rb\nsekalian nvr ezviz X5S-8W → Ready @ 865.000 : 1 unit"

	// Coba kalimat: Claude reads it there too (nothing stored)
	_, res := post(t, srv.URL+"/api/reply-rules/test", ceo, map[string]any{"text": text, "branch": "Semarang"})
	c := res["suggestions"].([]any)[0].(map[string]any)
	if items, _ := c["items"].([]any); len(items) != 2 || c["matched_by"] != "claude-haiku-5-5" {
		t.Fatalf("test sentence: %v", res)
	}
	var in struct {
		Kandidat []map[string]any `json:"kandidat"`
	}
	if err := json.Unmarshal(claude.last.Input, &in); err != nil || len(in.Kandidat) < 3 || claude.last.Purpose != "reply.match" ||
		in.Kandidat[0]["sku"] != "X5S_8W" || in.Kandidat[1]["sku"] != "H8c 2MP" || in.Kandidat[2]["sku"] != "H8C_3MP_Poe" {
		t.Fatalf("candidates offered: %s", claude.last.Input)
	}

	var tid, mid string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000401@s.whatsapp.net', 'Fenny', '6281234504471', now()) returning id").Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, 'CM1', 'in', $2, now() - interval '1 hour', 'received') returning id", tid, text).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	var rule string
	if err := st.Pool.QueryRow(ctx, "select id from reply_rules where action = 'stock' limit 1").Scan(&rule); err != nil {
		t.Fatal(err)
	}
	// the draft job (here: the card's button) has Claude read the message first, then the AI writes the draft
	calls := claude.calls
	if code, out := post(t, srv.URL+"/api/chat/threads/"+tid+"/messages/"+mid+"/ai-draft", ceo, map[string]any{"rule_id": rule}); code != 200 {
		t.Fatalf("draft: %d %v", code, out)
	}
	if claude.calls != calls+1 || countRows(t, st, "select count(*) from reply_ai_matches") != 1 {
		t.Fatalf("Claude read the message %d times; matches stored %d", claude.calls-calls, countRows(t, st, "select count(*) from reply_ai_matches"))
	}
	_, th := get(t, srv, "/api/chat/threads/"+tid, ceo)
	c = th["messages"].([]any)[0].(map[string]any)["suggest"].([]any)[0].(map[string]any)
	items, _ := c["items"].([]any)
	if len(items) != 2 || c["ai"] != true || c["draft"] != drafts.draft || c["headline"] != "2 barang · semua ready" {
		t.Fatalf("chat card: %v", c)
	}
	if h8 := items[0].(map[string]any); h8["asked"] != float64(8) || h8["qty"] != float64(207) || h8["ready"] != true {
		t.Fatalf("H8c line: %v", h8)
	}
	// opening the chat again does not ask Claude again
	_, _ = get(t, srv, "/api/chat/threads/"+tid, ceo)
	if claude.calls != calls+1 {
		t.Fatalf("Claude asked again: %d", claude.calls-calls)
	}

	// a SKU that is not a candidate is refused: no reading stored, the card says Claude did not manage
	claude.out = `{"items":[{"sku":"KARANGAN","qty":1}],"catatan":""}`
	_, res = post(t, srv.URL+"/api/reply-rules/test", ceo, map[string]any{"text": "H8c 2mp ready?"})
	if c := res["suggestions"].([]any)[0].(map[string]any); c["matched_by"] != nil || c["note"] == nil {
		t.Fatalf("invented SKU accepted: %v", c)
	}
}
