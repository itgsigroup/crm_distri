package api_test

import (
	"context"
	"encoding/json"
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

// "Muat pesan lama dari HP" (chats kept 90 days, ADR 0028 update): the linked phone is asked, through the bridge, for
// the 50 messages before the oldest one kept of the chat; a number the bridge cannot ask right now says so.
func TestChatHistoryFromPhone(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	var asked []map[string]any
	var paths []string
	busy := false
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if busy {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"permintaan riwayat terlalu cepat"}`))
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		asked = append(asked, in)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(bridge.Close)
	h := api.New(config.Config{Env: "dev", WATransport: "baileys", BridgeURL: bridge.URL, BridgeSecret: "s"}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	var thread string
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where seed_key = 'seed:thread:c-sinar'").Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status)
		values ($1, 'OLDEST-KEPT', 'out', 'pesan tertua', now() - interval '89 days', 'sent')`, thread); err != nil {
		t.Fatal(err)
	}
	code, out := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+thread+"/history", "sam@gsi.co.id", map[string]any{})
	if code != http.StatusAccepted || out["requested"] != float64(50) {
		t.Fatalf("%d %v", code, out)
	}
	if len(asked) != 1 || asked[0]["id"] != "OLDEST-KEPT" || asked[0]["from_me"] != true || asked[0]["count"] != float64(50) ||
		!strings.HasSuffix(paths[0], "/history") {
		t.Fatalf("bridge asked %v %v", paths, asked)
	}
	busy = true
	if code, out := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+thread+"/history", "sam@gsi.co.id", map[string]any{}); code != http.StatusTooManyRequests ||
		!strings.Contains(out["error"].(map[string]any)["message"].(string), "coba lagi") {
		t.Fatalf("busy: %d %v", code, out)
	}
}
