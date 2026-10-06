package api_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/pilot"
)

// Konfirmasi share of wallet: sales confirm their own dealers (metrics recomputed as "confirmed"), not others'.
func TestSOWConfirm(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	code, top := get(t, srv, "/api/sow/top?limit=20", "andi@gsi.co.id")
	items, _ := top["items"].([]any)
	if code != 200 || len(items) == 0 || top["quarter"] != "2026-Q4" {
		t.Fatalf("top %d %v", code, top)
	}
	first := items[0].(map[string]any)
	var other string
	_ = st.Pool.QueryRow(ctx, "select d.id::text from dealers d join sales_users s on s.id = d.owner_id where s.name <> 'Andi' limit 1").Scan(&other)
	if code, _ := post(t, srv.URL+"/api/sow/confirm", "andi@gsi.co.id", map[string]any{"items": []map[string]any{{"dealer_id": other, "sow": 40}}}); code != http.StatusForbidden {
		t.Fatalf("other sales' dealer: %d", code)
	}
	code, out := post(t, srv.URL+"/api/sow/confirm", "andi@gsi.co.id", map[string]any{"items": []map[string]any{{"dealer_id": first["id"], "sow": 64, "note": "kunjungan"}}})
	if code != 200 || out["confirmed"] != float64(1) {
		t.Fatalf("confirm %d %v", code, out)
	}
	var sow, src string
	_ = st.Pool.QueryRow(ctx, "select metrics_current->>'sow', metrics_current->>'sow_source' from dealers where id::text = $1", first["id"]).Scan(&sow, &src)
	if sow != "64" || src != "confirmed" {
		t.Fatalf("metrics sow %s %s", sow, src)
	}
}

// Pilot dashboard and CSV are for CEO/admin; in shadow mode a chat reply from the app is refused.
func TestPilotEndpoints(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	if code, _ := post(t, srv.URL+"/api/pilot/mode", "andi@gsi.co.id", map[string]string{"mode": "shadow"}); code != http.StatusForbidden {
		t.Fatalf("sales starts pilot: %d", code)
	}
	if code, out := post(t, srv.URL+"/api/pilot/mode", "sam@gsi.co.id", map[string]string{"mode": "shadow", "branch": "Semarang"}); code != 200 || !strings.Contains(out["message"].(string), "mode bayangan") {
		t.Fatalf("start: %d %v", code, out)
	}
	code, rep := get(t, srv, "/api/pilot", "sam@gsi.co.id")
	r, _ := rep["report"].(map[string]any)
	if code != 200 || r["mode"] != "shadow" || r["audit_ok"] != true || len(r["agents"].([]any)) != 6 {
		t.Fatalf("report %d %v", code, rep)
	}
	if code, _ := get(t, srv, "/api/pilot", "andi@gsi.co.id"); code != http.StatusForbidden {
		t.Fatalf("sales dashboard: %d", code)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/pilot/export.csv?week=2026-10-05", nil)
	req.Header.Set("X-Dev-User", "sam@gsi.co.id")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(res.Header.Get("Content-Disposition"), "pilot-minggu-20261005.csv") || !strings.Contains(string(b), "order_tepat_jadwal_pct") {
		t.Fatalf("csv %s\n%s", res.Header.Get("Content-Disposition"), b)
	}
	var thread string
	_ = st.Pool.QueryRow(ctx, "select t.id::text from chat_threads t join sales_users s on s.id = t.sales_id where s.name = 'Andi' and t.kind = 'dealer' limit 1").Scan(&thread)
	if code, out := post(t, srv.URL+"/api/chat/threads/"+thread+"/messages", "andi@gsi.co.id", map[string]string{"body": "siap"}); code != http.StatusConflict {
		t.Fatalf("reply in shadow: %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/pilot/unlock", "sam@gsi.co.id", map[string]string{"agent": "AI Order"}); code != http.StatusConflict {
		t.Fatalf("unlock during shadow: %d", code)
	}
	at := time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)
	r2, err := pilot.Service{St: st, Clock: clock.Fixed(at)}.Snapshot(ctx, at)
	if err != nil || r2.Branch != "Semarang" {
		t.Fatal(err)
	}
	if _, rep = get(t, srv, "/api/pilot", "sam@gsi.co.id"); len(rep["weeks"].([]any)) != 1 {
		t.Fatalf("weeks %v", rep["weeks"])
	}
}
