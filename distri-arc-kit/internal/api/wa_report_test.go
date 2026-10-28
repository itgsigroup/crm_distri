package api_test

import (
	"context"
	"net/http"
	"testing"
)

// ADR 0050: Report WhatsApp — the CEO sees every sales; a sales sees only their own conversations.
func TestWAReport(t *testing.T) {
	srv, st := chatServer(t)
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
	// sales do not get Report WhatsApp (migration 0065) until a role gives it; then only their own numbers
	if code, _ := get(t, srv, "/api/wa/report?days=30", "dewi@gsi.co.id"); code != http.StatusForbidden {
		t.Fatalf("sales opens Report WhatsApp: %d", code)
	}
	if _, err := st.Pool.Exec(context.Background(), "update roles set screens = array_append(screens, 'reportwa') where key = 'sales'"); err != nil {
		t.Fatal(err)
	}
	code, own := get(t, srv, "/api/wa/report?days=30", "dewi@gsi.co.id")
	rows, _ := own["sales"].([]any)
	if code != http.StatusOK || len(rows) != 1 || rows[0].(map[string]any)["name"] != "Dewi" {
		t.Fatalf("sales dewi sees %d %v", code, own["sales"])
	}
	// a logged-out number locks Chat, not the reports: its chats still count (Sam 27 Oct)
	if _, err := st.Pool.Exec(context.Background(), "update wa_numbers set state = 'logged_out'"); err != nil {
		t.Fatal(err)
	}
	if _, locked := get(t, srv, "/api/wa/report?days=30", "sam@gsi.co.id"); len(locked["sales"].([]any)) != len(all["sales"].([]any)) {
		t.Fatalf("logged out numbers dropped from the CEO report: %v", locked["sales"])
	}
	if _, locked := get(t, srv, "/api/wa/report?days=30", "dewi@gsi.co.id"); len(locked["sales"].([]any)) != 1 {
		t.Fatalf("logged out number dropped from its sales' report: %v", locked["sales"])
	}
	if code, out := get(t, srv, "/api/wa/report?days=9", "sam@gsi.co.id"); code != http.StatusOK || out["days"] != float64(7) {
		t.Fatalf("unknown period falls back to 7: %d %v", code, out["days"])
	}
}
