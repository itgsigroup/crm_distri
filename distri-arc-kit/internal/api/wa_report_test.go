package api_test

import (
	"net/http"
	"testing"
)

// ADR 0050: Report WhatsApp — the CEO sees every sales; a sales sees only their own conversations.
func TestWAReport(t *testing.T) {
	srv, _ := chatServer(t)
	code, all := get(t, srv, "/api/wa/report?days=30", "sam@gsi.co.id")
	if code != http.StatusOK || all["days"] != float64(30) || all["team"] == nil {
		t.Fatalf("ceo: %d %v", code, all)
	}
	names := map[string]bool{}
	for _, x := range all["sales"].([]any) {
		names[x.(map[string]any)["name"].(string)] = true
	}
	if !names["Dewi"] || !names["Fajar"] {
		t.Fatalf("ceo sees %v", names)
	}
	code, own := get(t, srv, "/api/wa/report?days=30", "dewi@gsi.co.id")
	rows, _ := own["sales"].([]any)
	if code != http.StatusOK || len(rows) != 1 || rows[0].(map[string]any)["name"] != "Dewi" {
		t.Fatalf("sales dewi sees %d %v", code, own["sales"])
	}
	if code, out := get(t, srv, "/api/wa/report?days=9", "sam@gsi.co.id"); code != http.StatusOK || out["days"] != float64(7) {
		t.Fatalf("unknown period falls back to 7: %d %v", code, out["days"])
	}
}
