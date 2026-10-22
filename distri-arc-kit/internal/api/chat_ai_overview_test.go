package api_test

import (
	"context"
	"testing"
)

// Analisa Claude on the tab Main: the company (latest Analisa Dealer), every line Claude wrote on the conversation
// oldest first and once (a line carried to a later cycle is not repeated), and only Claude's proposals (cycles via
// MCP) with the person's decision — not a rules cycle's.
func TestChatAIOverview(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var thread, dealer string
	if err := st.Pool.QueryRow(ctx, "select id, dealer_id from chat_threads where seed_key = 'seed:thread:c-sinar'").Scan(&thread, &dealer); err != nil {
		t.Fatal(err)
	}
	cycle := func(via, started string) string {
		var id string
		if err := st.Pool.QueryRow(ctx, "insert into cycles (trigger, via, status, started_at) values ('mcp', $1, 'done', $2::timestamptz) returning id", via, started).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, "insert into wa_analyses (cycle_id, window_days, source) values ($1, 14, 'ai')", id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	line := func(c, intent, summary, at string) {
		if _, err := st.Pool.Exec(ctx, `insert into wa_insights (cycle_id, thread_id, kind, dealer_id, title, intent, items, summary, confidence, summary_at)
			values ($1, $2, 'dealer', $3, 'Sinar', $4, '[{"name":"Kamera IP 4MP","qty":10}]', $5, 80, $6::timestamptz)`, c, thread, dealer, intent, summary, at); err != nil {
			t.Fatal(err)
		}
	}
	c1, c2, c3 := cycle("mcp", "2026-09-01T09:00:00Z"), cycle("mcp", "2026-09-20T09:00:00Z"), cycle("system", "2026-10-01T09:00:00Z")
	line(c1, "ask_price", "Tanya harga kamera IP untuk proyek gudang.", "2026-09-01T09:05:00Z")
	line(c2, "order", "Sepakat order 10 kamera, minta kirim minggu depan.", "2026-09-20T09:05:00Z")
	line(c3, "order", "Sepakat order 10 kamera, minta kirim minggu depan.", "2026-09-20T09:05:00Z") // carried
	if _, err := st.Pool.Exec(ctx, `insert into dealer_analyses (cycle_id, dealer_id, status, overview, reasons, confidence)
		values ($1, $2, 'perlu_perhatian', 'Dealer aktif, order turun bulan ini.', '["Order turun 30%","Piutang 20 hari"]', 75)`, c2, dealer); err != nil {
		t.Fatal(err)
	}
	prop := func(c, title string) {
		if _, err := st.Pool.Exec(ctx, `insert into proposals (cycle_id, agent, dealer_id, kind, title, why, confidence, signal_ids, autonomy)
			values ($1, 'AI Follow-up', $2, 'followup', $3, 'ritme order', 0.8, array[gen_random_uuid()], 'approve')`, c, dealer, title); err != nil {
			t.Fatal(err)
		}
	}
	prop(c2, "Follow-up order kamera")
	prop(c3, "Usulan aturan sistem")

	code, out := get(t, srv, "/api/chat/threads/"+thread+"/ai-overview", "sam@gsi.co.id")
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	co, _ := out["company"].(map[string]any)
	if co == nil || co["status"] != "perlu_perhatian" || len(co["reasons"].([]any)) != 2 || co["confidence"] != float64(75) {
		t.Fatalf("company: %v", out["company"])
	}
	tl := out["timeline"].([]any)
	if len(tl) != 2 || tl[0].(map[string]any)["intent"] != "ask_price" || tl[1].(map[string]any)["intent"] != "order" {
		t.Fatalf("timeline: %v", tl)
	}
	ps := out["proposals"].([]any)
	if len(ps) != 1 || ps[0].(map[string]any)["title"] != "Follow-up order kamera" || ps[0].(map[string]any)["status"] != "proposed" {
		t.Fatalf("only Claude's proposals: %v", ps)
	}
	if out["chat"].(map[string]any)["messages"].(float64) == 0 {
		t.Fatalf("chat facts: %v", out["chat"])
	}
}
