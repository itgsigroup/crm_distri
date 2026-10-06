package api_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
)

// Stage 08 acceptance: 4 sales + 18 dealers, strongest pair Rizky ↔ Indo Vision (104 in 30 days), the period
// changes the numbers, and the Mitra Jaya insight.
func TestRelasiOnSeed(t *testing.T) {
	srv, _ := chatServer(t)
	_, g := get(t, srv, "/api/relasi?period=30", "sam@gsi.co.id")
	nodes := g["nodes"].([]any)
	sales, dealers := 0, 0
	for _, n := range nodes {
		switch n.(map[string]any)["type"] {
		case "sales":
			sales++
		case "dealer":
			dealers++
		}
	}
	if sales != 4 || dealers != 18 {
		t.Fatalf("%d sales, %d dealers", sales, dealers)
	}
	top := g["pairs"].([]any)[0].(map[string]any)
	if top["sales"] != "Rizky" || top["dealer"] != "Indo Vision Security" || top["w"].(float64) != 104 {
		t.Fatalf("strongest pair %v", top)
	}
	_, g6 := get(t, srv, "/api/relasi?period=180", "sam@gsi.co.id")
	if w := g6["pairs"].([]any)[0].(map[string]any)["w"].(float64); w != 553 {
		t.Fatalf("180 days: %v", w)
	}
	if g6["interactions"].(float64) <= g["interactions"].(float64) {
		t.Fatal("period does not change the totals")
	}
	_, f := get(t, srv, "/api/relasi?period=30&sales=dewi", "sam@gsi.co.id")
	for _, p := range f["pairs"].([]any) {
		if p.(map[string]any)["sales"] != "Dewi" {
			t.Fatalf("filter leaked %v", p)
		}
	}

	_, ins := get(t, srv, "/api/relasi/insights?period=30", "sam@gsi.co.id")
	found := false
	for _, x := range ins["items"].([]any) {
		title := x.(map[string]any)["title"].(string)
		if strings.HasPrefix(title, "Mitra Jaya") && strings.Contains(title, "30 hari tanpa order (siklus 21) — intensitas WA turun") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Mitra Jaya insight missing: %v", ins)
	}
}

// PIC aktif comes from contacts: Mitra Jaya has 2 active PICs; a dealer with one is flagged by pic_active ≤ 1.
func TestPICActive(t *testing.T) {
	srv, st := chatServer(t)
	_, d := get(t, srv, "/api/dealers/mitra", "sam@gsi.co.id")
	if n := d["metrics"].(map[string]any)["pic_active"].(float64); n != 2 {
		t.Fatalf("Mitra Jaya PIC aktif %v", n)
	}
	// live messages after the imported history count: a reply from a contact refreshes last_interaction_at
	var contact, number string
	if err := st.Pool.QueryRow(context.Background(), `select c.id, c.wa_number from contacts c join dealers d on d.id = c.dealer_id
		where d.slug = 'sinar' and c.interactions_90d = 0 limit 1`).Scan(&contact, &number); err != nil {
		t.Skip("no silent contact in seed")
	}
	var thread string
	_ = st.Pool.QueryRow(context.Background(), "select t.id from chat_threads t join dealers d on d.id = t.dealer_id where d.slug = 'sinar' limit 1").Scan(&thread)
	if _, err := st.Pool.Exec(context.Background(), `insert into chat_messages (thread_id, wa_msg_id, direction, from_number, body, sent_at, status)
		values ($1, 'test:pic', 'in', $2, 'Pak, saya teknisi toko, mau tanya stok', $3, 'received')`, thread, number, time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := dealersvc.New(st, clock.Fixed(time.Date(2026, 10, 6, 12, 0, 0, 0, clock.WIB))).Recompute(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	var last time.Time
	if err := st.Pool.QueryRow(context.Background(), "select interactions_90d, last_interaction_at from contacts where id = $1", contact).Scan(&n, &last); err != nil {
		t.Fatal(err)
	}
	if n != 1 || !last.Equal(time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("recount: %d %v", n, last)
	}
}
