package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// Balasan cepat (ADR 0065): anyone in Chat saves a "/shortcut" text for the team; shortcuts are unique whatever
// their case or "/"; only its maker or an admin changes or deletes it; the most used come first; nothing is sent.
func TestQuickReplies(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	h := api.New(config.Config{Env: "dev", ChatWindowDays: 14}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	ceo, andi, dewi := "sam@gsi.co.id", "andi@gsi.co.id", "dewi@gsi.co.id"
	url := srv.URL + "/api/chat/quick-replies"

	code, q := sendJSON(t, "POST", url, andi, map[string]any{"shortcut": " /Rekening ", "body": "Transfer ke BCA 123 a.n. PT GSI\r\nTerima kasih "})
	if code != http.StatusCreated || q["shortcut"] != "rekening" || q["body"] != "Transfer ke BCA 123 a.n. PT GSI\nTerima kasih" || q["can_edit"] != true {
		t.Fatalf("create %d %v", code, q)
	}
	id := q["id"].(string)
	if code, _ := sendJSON(t, "POST", url, dewi, map[string]any{"shortcut": "REKENING", "body": "lain"}); code != http.StatusConflict {
		t.Fatalf("duplicate shortcut %d", code)
	}
	for _, bad := range []map[string]any{{"shortcut": "dua kata", "body": "x"}, {"shortcut": "/", "body": "x"}, {"shortcut": "ok", "body": "  "}, {"shortcut": "ok", "body": strings.Repeat("x", 2001)}} {
		if code, _ := sendJSON(t, "POST", url, andi, bad); code != http.StatusBadRequest {
			t.Fatalf("invalid %v accepted: %d", bad, code)
		}
	}
	code, q2 := sendJSON(t, "POST", url, dewi, map[string]any{"shortcut": "alamat", "body": "Jl. Contoh 1, Semarang"})
	if code != http.StatusCreated {
		t.Fatalf("second %d", code)
	}

	// another sales may use it but not change it; its maker and an admin may
	if code, _ := sendJSON(t, "PUT", url+"/"+id, dewi, map[string]any{"shortcut": "rek", "body": "x"}); code != http.StatusForbidden {
		t.Fatalf("another sales edited %d", code)
	}
	if code, _ := sendJSON(t, "DELETE", url+"/"+id, dewi, nil); code != http.StatusForbidden {
		t.Fatalf("another sales deleted %d", code)
	}
	if code, out := sendJSON(t, "PUT", url+"/"+id, andi, map[string]any{"shortcut": "rek", "body": "BCA 123"}); code != 200 || out["shortcut"] != "rek" {
		t.Fatalf("maker edit %d %v", code, out)
	}
	if code, _ := sendJSON(t, "PUT", url+"/"+id, andi, map[string]any{"shortcut": "alamat", "body": "x"}); code != http.StatusConflict {
		t.Fatalf("edit onto a taken shortcut %d", code)
	}

	// the most used first; the list says who may change each
	if code, _ := sendJSON(t, "POST", url+"/"+q2["id"].(string)+"/use", andi, nil); code != http.StatusNoContent {
		t.Fatalf("use %d", code)
	}
	_, list := get(t, srv, "/api/chat/quick-replies", andi)
	items := list["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["shortcut"] != "alamat" || items[0].(map[string]any)["uses"] != float64(1) {
		t.Fatalf("list %v", items)
	}
	if items[0].(map[string]any)["can_edit"] != false || items[1].(map[string]any)["can_edit"] != true {
		t.Fatalf("can_edit for andi %v", items)
	}
	if code, _ := sendJSON(t, "DELETE", url+"/"+q2["id"].(string), ceo, nil); code != http.StatusNoContent {
		t.Fatalf("admin delete %d", code)
	}
	if _, list = get(t, srv, "/api/chat/quick-replies", andi); len(list["items"].([]any)) != 1 {
		t.Fatalf("after delete %v", list)
	}
	// a quick reply only fills the box: nothing was queued to WhatsApp
	var n int
	if err := st.Pool.QueryRow(ctx, "select count(*) from outbox").Scan(&n); err != nil || n != 0 {
		t.Fatalf("outbox rows %d %v", n, err)
	}
}
