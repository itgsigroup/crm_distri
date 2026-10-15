package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
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

// Cari di dalam chat: every stored message of the conversation (not only the loaded window), case-insensitive, newest
// first, with the text around the word; typed % and _ are plain characters; a jump loads every message from the hit
// up to the loaded ones, so the chat stays continuous.
func TestSearchInChat(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	h := api.New(config.Config{Env: "dev", ChatWindowDays: 14}, st, clock.Fixed(time.Now()), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000301@s.whatsapp.net', 'Budi', '6281234504471', now()) returning id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	add := func(wid, body, ago string) {
		if _, err := st.Pool.Exec(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, $2, 'in', $3, now() - $4::interval, 'received')", id, wid, body, ago); err != nil {
			t.Fatal(err)
		}
	}
	add("S1", "Pak, harga Kamera IP 4MP berapa ya untuk 10 unit? Sekalian ongkir ke Magelang", "200 days")
	add("S2", "diskon 10% bisa?", "60 days")
	add("S3", "ok kamera ip sudah sampai", "1 day")
	add("S4", "terima kasih", "2 hours")
	add("S5", "kabel\ncoaxial  (300m) ya", "3 hours")

	search := func(q string) []map[string]any {
		_, out := get(t, srv, "/api/chat/threads/"+id+"/search?q="+url.QueryEscape(q), "sam@gsi.co.id")
		var hits []map[string]any
		for _, x := range out["items"].([]any) {
			hits = append(hits, x.(map[string]any))
		}
		return hits
	}
	hits := search("kamera ip")
	if len(hits) != 2 || !strings.Contains(hits[0]["snippet"].(string), "kamera ip sudah") || !strings.Contains(hits[1]["snippet"].(string), "Kamera IP 4MP") {
		t.Fatalf("kamera ip: %v", hits)
	}
	if hits := search("10%"); len(hits) != 1 || !strings.Contains(hits[0]["snippet"].(string), "diskon 10%") {
		t.Fatalf("10%%: %v", hits)
	}
	if hits := search("%"); len(hits) != 0 {
		t.Fatalf("one character searched: %v", hits)
	}
	if hits := search("kabel coaxial (300m)"); len(hits) != 1 || hits[0]["snippet"] != "kabel coaxial (300m) ya" {
		t.Fatalf("words across a line break: %v", hits)
	}
	if hits := search("ongkir   ke magelang"); len(hits) != 1 {
		t.Fatalf("spaces: %v", hits)
	}

	// the open chat holds the 14-day window; the jump loads from the 200-day-old hit up to it
	_, page := get(t, srv, "/api/chat/threads/"+id, "sam@gsi.co.id")
	first := page["messages"].([]any)[0].(map[string]any)["sent_at"].(string)
	old := search("4MP")[0]["sent_at"].(string)
	_, jump := get(t, srv, "/api/chat/threads/"+id+"?from="+url.QueryEscape(old)+"&before="+url.QueryEscape(first), "sam@gsi.co.id")
	var bodies []string
	for _, m := range jump["messages"].([]any) {
		bodies = append(bodies, m.(map[string]any)["body"].(string))
	}
	if len(bodies) != 2 || !strings.HasPrefix(bodies[0], "Pak, harga") || bodies[1] != "diskon 10% bisa?" || jump["has_more"] != false {
		t.Fatalf("jump %v has_more %v", bodies, jump["has_more"])
	}
}
