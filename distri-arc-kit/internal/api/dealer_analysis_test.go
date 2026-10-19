package api_test

import (
	"context"
	"net/http"
	"testing"
)

// ADR 0055: the dealer page reads Claude's Analisa Dealer (null before), and the WhatsApp buttons get that dealer's
// conversations the person may open — a sales only theirs, a role without Chat none.
func TestDealerAnalysisAndThreads(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	if code, out := get(t, srv, "/api/dealers/mitra/analysis", "sam@gsi.co.id"); code != http.StatusOK || out["analysis"] != nil {
		t.Fatalf("before any reading: %d %v", code, out)
	}
	if _, err := st.Pool.Exec(ctx, `insert into cycles (trigger, scope, via, status) values ('manual', 'dealer:mitra', 'mcp', 'done')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into dealer_analyses (cycle_id, dealer_id, status, overview, reasons, wa_draft, confidence)
		select c.id, d.id, 'bermasalah', 'Invoice telat.', '["Invoice telat 12 hari."]', 'Selamat siang Pak, mengingatkan invoice.', 80
		from cycles c, dealers d where c.scope = 'dealer:mitra' and d.slug = 'mitra'`); err != nil {
		t.Fatal(err)
	}
	code, out := get(t, srv, "/api/dealers/mitra/analysis", "sam@gsi.co.id")
	a, _ := out["analysis"].(map[string]any)
	if code != http.StatusOK || a == nil || a["status"] != "bermasalah" || a["wa_draft"] == "" || len(a["reasons"].([]any)) != 1 {
		t.Fatalf("analysis %d %v", code, out)
	}
	code, out = get(t, srv, "/api/dealers/mitra/threads", "sam@gsi.co.id")
	if items, _ := out["items"].([]any); code != http.StatusOK || len(items) == 0 {
		t.Fatalf("CEO dealer threads %d %v", code, out)
	}
}
