package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeLyra is a Hermes API server that reads products: it answers the product prompt with one JSON.
func fakeLyra(t *testing.T, key, answer string, input *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"hermes-agent"}]}`))
		case "/v1/chat/completions":
			var body struct {
				Messages []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("X-Hermes-Session-Key") != "orbit-lyra-produk" || len(body.Messages) != 2 || body.Messages[0].Role != "system" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var user string
			_ = json.Unmarshal(body.Messages[1].Content, &user)
			input.Store(user)
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": answer}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// lyraCards polls the chat until no Lyra card is pending and returns the cards by message id.
func lyraCards(t *testing.T, srv *httptest.Server, tid, user string) map[string]map[string]any {
	t.Helper()
	for range 200 {
		_, out := get(t, srv, "/api/chat/threads/"+tid, user)
		cards := map[string]map[string]any{}
		pending := false
		for _, m := range out["messages"].([]any) {
			mv := m.(map[string]any)
			if c, ok := mv["lyra"].(map[string]any); ok {
				cards[mv["id"].(string)] = c
				pending = pending || c["status"] == "pending"
			}
		}
		if !pending {
			return cards
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("lyra card still pending")
	return nil
}

// ADR 0070: a customer message asking about a product gets Lyra's card under its bubble — stock per city and
// warehouse and the DPP price computed by GSI Orbit — read once; a message that asks nothing gets none. In demo mode
// the product is picked by words; once Hermes is connected, "Baca ulang" has Lyra name the products among the
// catalog candidates, with the customer's number masked. Internal chats are read as well.
func TestLyraProductCard(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	const ceo = "sam@gsi.co.id"
	if _, err := st.Pool.Exec(ctx, `delete from stock_items; delete from products where sku = 'X5S_8W';
		insert into products (sku, name, list_price, prices) values ('X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 865000, '{"A": 865000, "B": 900000}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into stock_items (branch, sku, name, category, qty) values
		('Semarang', 'X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 'Kamera & NVR', 12), ('Surabaya', 'X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 'Kamera & NVR', 30),
		('Jakarta', 'X5S_8W', 'Ezviz NVR 8Ch X5S 8W', 'Kamera & NVR', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into import_rows (entity, key, data, source) values
		('stock', 'X5S_8W|gudang rungkut', '{"sku":"X5S_8W","warehouse":"Gudang Rungkut","qty":"20"}', 'csv'),
		('stock', 'X5S_8W|gudang margomulyo', '{"sku":"X5S_8W","warehouse":"Gudang Margomulyo","qty":"10"}', 'csv');
		insert into data_mappings (kind, source_value, target) values ('warehouse', 'Gudang Rungkut', 'Surabaya'), ('warehouse', 'Gudang Margomulyo', 'Surabaya')
		on conflict do nothing`); err != nil {
		t.Fatal(err)
	}
	var tid, ask, thanks string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('new', '6281300000402@s.whatsapp.net', 'Fenny', '6281234504471', now()) returning id").Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, 'LY1', 'in', 'stok nvr ezviz X5S 8W ada? 2 unit, wa saya 081234567890', now() - interval '1 hour', 'received') returning id", tid).Scan(&ask); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, 'LY2', 'in', 'terima kasih pak', now() - interval '50 minutes', 'received') returning id", tid).Scan(&thanks); err != nil {
		t.Fatal(err)
	}

	// demo mode: picked by words, numbers from GSI Orbit
	cards := lyraCards(t, srv, tid, ceo)
	c := cards[ask]
	if c == nil || cards[thanks] != nil || c["status"] != "done" || c["demo"] != true {
		t.Fatalf("demo cards: %v", cards)
	}
	items := c["items"].([]any)
	l := items[0].(map[string]any)
	if len(items) != 1 || l["sku"] != "X5S_8W" || l["price"] != float64(865000) || l["asked"] != float64(2) || l["stock"] != float64(42) || c["tier"] != "" {
		t.Fatalf("demo line: %v", c)
	}
	cities := l["cities"].([]any)
	sby := cities[0].(map[string]any)
	if len(cities) != 2 || sby["city"] != "Surabaya" || len(sby["warehouses"].([]any)) != 2 {
		t.Fatalf("cities: %v", cities)
	}
	var n int
	_ = st.Pool.QueryRow(ctx, "select count(*) from hermes_product_reads").Scan(&n)
	if n != 1 {
		t.Fatalf("reads stored: %d", n)
	}

	// a photo without a stock/price/product word is not read, and a reading the earlier trigger stored for it
	// (any photo) is hidden; asked from the bubble, it shows
	var photo string
	if err := st.Pool.QueryRow(ctx, `insert into chat_messages (thread_id, wa_msg_id, direction, body, media, sent_at, status)
		values ($1, 'LY3', 'in', '[Foto] disini bisa mbak gran', '{"kind":"image"}', now() - interval '40 minutes', 'received') returning id`, tid).Scan(&photo); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into hermes_product_reads (message_id, thread_id, status, items, demo, finished_at) values ($1, $2, 'done', '[{"sku":"X5S_8W","qty":0}]', true, now())`, photo, tid); err != nil {
		t.Fatal(err)
	}
	if cards := lyraCards(t, srv, tid, ceo); cards[photo] != nil {
		t.Fatalf("photo without a trigger word: %v", cards[photo])
	}
	if code, _ := post(t, srv.URL+"/api/chat/threads/"+tid+"/messages/"+photo+"/lyra", ceo, nil); code != http.StatusAccepted {
		t.Fatalf("ask lyra on the photo: %d", code)
	}
	if cards := lyraCards(t, srv, tid, ceo); cards[photo] == nil {
		t.Fatal("a reading asked from the bubble is shown")
	}

	// an internal chat between staff is read too (Sam 28 Oct): a stock question there gets the card
	var itid, imsg string
	if err := st.Pool.QueryRow(ctx, "insert into chat_threads (kind, wa_jid, title, account, last_message_at) values ('internal', '6281234504472@s.whatsapp.net', 'Catarina', '6281234504471', now()) returning id").Scan(&itid); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, 'LY4', 'in', 'kak.. x5s 8w ready?', now() - interval '10 minutes', 'received') returning id", itid).Scan(&imsg); err != nil {
		t.Fatal(err)
	}
	if c := lyraCards(t, srv, itid, ceo)[imsg]; c == nil || c["status"] != "done" {
		t.Fatalf("internal chat not read: %v", c)
	}

	// Hermes connected: read again → Lyra's products
	var input atomic.Value
	h := fakeLyra(t, "rahasia-lyra", `Siap.`+"\n```json\n"+`{"tanya_produk":true,"items":[{"sku":"x5s_8w","qty":5}],"catatan":"NVR 8 channel"}`+"\n```", &input)
	if code, out := put(t, srv.URL+"/api/mcp/hermes/config", ceo, map[string]any{"url": h.URL + "/v1/", "api_key": "rahasia-lyra"}); code != http.StatusOK {
		t.Fatalf("config: %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/chat/threads/"+tid+"/messages/"+ask+"/lyra", ceo, nil); code != http.StatusAccepted {
		t.Fatalf("read again: %d", code)
	}
	c = lyraCards(t, srv, tid, ceo)[ask]
	l = c["items"].([]any)[0].(map[string]any)
	if c["demo"] == true || c["note"] != "NVR 8 channel" || l["sku"] != "X5S_8W" || l["asked"] != float64(5) {
		t.Fatalf("lyra card: %v", c)
	}
	sent, _ := input.Load().(string)
	if !strings.Contains(sent, `"kandidat"`) || !strings.Contains(sent, "X5S_8W") || strings.Contains(sent, "081234567890") {
		t.Fatalf("sent to lyra: %s", sent)
	}
	if code, _ := post(t, srv.URL+"/api/chat/threads/"+tid+"/messages/"+thanks+"/lyra", ceo, nil); code != http.StatusAccepted {
		t.Fatalf("ask any customer bubble: %d", code)
	}
}
