package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// GET /screen-analysis/{screen} (ADR 0030): null until an AI read the page; the Kas notes come keyed by the page's
// dealer id (stored per dealer uuid).
func TestScreenAnalysisEndpoint(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	code, out := get(t, srv, "/api/screen-analysis/credit", "sam@gsi.co.id")
	if code != http.StatusOK || out["analysis"] != nil {
		t.Fatalf("before any reading: %d %v", code, out)
	}
	var cyc, dealer uuid.UUID
	if err := st.Pool.QueryRow(ctx, "insert into cycles (trigger, status) values ('manual', 'done') returning id").Scan(&cyc); err != nil {
		t.Fatal(err)
	}
	var slug string
	if err := st.Pool.QueryRow(ctx, "select id, slug from dealers where slug = 'graha'").Scan(&dealer, &slug); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into screen_analyses (cycle_id, screen, overview, lines) values ($1, 'credit', 'Kas aman.', jsonb_build_object($2::text, '{"note":"Tagih INV lama dulu","confidence":70}'::jsonb))`, cyc, dealer.String()); err != nil {
		t.Fatal(err)
	}
	code, out = get(t, srv, "/api/screen-analysis/credit", "sam@gsi.co.id")
	a, _ := out["analysis"].(map[string]any)
	lines, _ := a["lines"].(map[string]any)
	if code != http.StatusOK || a["overview"] != "Kas aman." || a["source"] != "ai" || lines[slug] == nil {
		t.Fatalf("credit reading: %d %v", code, out)
	}
	if code, _ := get(t, srv, "/api/screen-analysis/dealer", "sam@gsi.co.id"); code != http.StatusNotFound { // orbit, segmen, relasi are read since ADR 0052–0054
		t.Fatalf("unknown screen: %d", code)
	}
}
