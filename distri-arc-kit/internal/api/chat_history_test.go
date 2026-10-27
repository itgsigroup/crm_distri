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

// A chat opened in GSI Orbit is read on WhatsApp at once (blue ticks, ADR 0069) through the number holding it; an
// internal DM between employees is not; a bridge that fails never stops the chat from being read in GSI Orbit.
func TestChatReadSendsBlueTicks(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	var read []string
	down := false
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		read = append(read, r.URL.Path+" "+in["chat_id"].(string))
		if down {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"sesi belum terhubung"}`))
			return
		}
		_, _ = w.Write([]byte(`{"read":2}`))
	}))
	t.Cleanup(bridge.Close)
	h := api.New(config.Config{Env: "dev", WATransport: "baileys", BridgeURL: bridge.URL, BridgeSecret: "s"}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	thread := func(kind, jid string) string {
		var id string
		if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at, unread) values ($1, $2, $2, '6281234504471', now(), 2) returning id", kind, jid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	customer := thread("new", "6281300000401@s.whatsapp.net")
	internal := thread("internal", "6281300000402@s.whatsapp.net")

	if code, _ := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+customer+"/read", "sam@gsi.co.id", map[string]any{}); code != http.StatusOK {
		t.Fatalf("read: %d", code)
	}
	if len(read) != 1 || !strings.HasSuffix(read[0], "/read 6281300000401@s.whatsapp.net") {
		t.Fatalf("bridge asked %v", read)
	}
	if code, _ := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+internal+"/read", "sam@gsi.co.id", map[string]any{}); code != http.StatusOK || len(read) != 1 {
		t.Fatalf("internal DM read on WhatsApp: %d %v", code, read)
	}
	down = true
	if _, err := st.Pool.Exec(ctx, "update chat_threads set unread = 3 where id = $1", customer); err != nil {
		t.Fatal(err)
	}
	if code, _ := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+customer+"/read", "sam@gsi.co.id", map[string]any{}); code != http.StatusOK {
		t.Fatalf("bridge down failed the read: %d", code)
	}
	if count(t, st, "select count(*) from chat_threads where id = $1 and unread = 0", customer) != 1 {
		t.Fatal("not read in GSI Orbit while the bridge was down")
	}
}
