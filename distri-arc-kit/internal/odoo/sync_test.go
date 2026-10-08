package odoo_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/odoo"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/testdb"
)

var now = clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB))

func count(t *testing.T, st *store.Store, sql string) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func syncer(t *testing.T, st *store.Store) (*odoo.Syncer, *odoo.Fake) {
	t.Helper()
	f, err := odoo.NewFake(db.Seed, "seed/odoo")
	if err != nil {
		t.Fatal(err)
	}
	return odoo.NewSyncer(st, f, now, slog.New(slog.NewTextHandler(io.Discard, nil))), f
}

func snapshot(t *testing.T, st *store.Store) string {
	t.Helper()
	ms, err := dealersvc.New(st, now).Recompute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Status, Segment, Credit string
		Score                   int
	}
	out := map[string]row{}
	for id, m := range ms {
		out[id.String()] = row{m.Status, m.Segment, m.Credit.State, m.Score}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// Syncing the fake Odoo after the seed changes nothing: same rows, same metrics; a second run adds nothing.
func TestSyncIsIdempotentWithSeed(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, st)
	tables := []string{"dealers", "contacts", "orders", "invoices", "payments", "stock_items"}
	counts := map[string]int{}
	for _, tb := range tables {
		counts[tb] = count(t, st, "select count(*) from "+tb)
	}
	s, _ := syncer(t, st)
	rep, err := s.Run(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Records["sale.order"] != 379 || rep.Signals == 0 {
		t.Fatalf("report %+v", rep)
	}
	for _, tb := range tables {
		if n := count(t, st, "select count(*) from "+tb); n != counts[tb] {
			t.Errorf("%s: %d rows after sync, %d before", tb, n, counts[tb])
		}
	}
	if after := snapshot(t, st); after != before {
		t.Fatalf("metrics changed after sync")
	}
	sigs := count(t, st, "select count(*) from signals")
	rep2, err := s.Run(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Signals != 0 || count(t, st, "select count(*) from signals") != sigs {
		t.Fatalf("second run created signals: %+v", rep2)
	}
	if count(t, st, "select count(*) from products") == 0 {
		t.Fatal("products not synced")
	}
}

// Odoo alone (no seed) builds the 18 dealers with their orders.
func TestSyncFromEmptyDatabase(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	// sales users and the category map come from the seed's reference data
	if _, err := st.Pool.Exec(ctx, `insert into sales_users (name, branch, role, odoo_user_id, wa_number) values
		('Andi','Semarang','sales',11,'1'),('Dewi','Yogyakarta','sales',12,'2'),('Rizky','Surabaya','sales',13,'3'),('Fajar','Jakarta','sales',14,'4')`); err != nil {
		t.Fatal(err)
	}
	for _, c := range seed.Categories {
		if _, err := st.Pool.Exec(ctx, "insert into category_map values ($1,$2)", c.OdooID, c.Kat); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := syncer(t, st)
	if _, err := s.Run(ctx, true); err != nil {
		t.Fatal(err)
	}
	if count(t, st, "select count(*) from dealers") != 18 || count(t, st, "select count(*) from orders") != 379 {
		t.Fatal("dealers/orders not created from Odoo")
	}
	if count(t, st, "select count(*) from dealers where slug = 'sinar-elektronik' and owner_id is not null and tier = 'A'") != 1 {
		t.Fatal("partner mapping")
	}
	if count(t, st, "select count(*) from orders where lines @> '[{\"category\":\"Kamera & NVR\"}]'") == 0 {
		t.Fatal("line categories not mapped")
	}
}

// An incremental change (a payment arrives in Odoo) moves the order to Bayar and frees the dealer's limit.
func TestIncrementalPayment(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	s, f := syncer(t, st)
	if _, err := s.Run(ctx, true); err != nil {
		t.Fatal(err)
	}
	// INV/0901 (Nusa Teknik, Rp 41 jt) gets paid
	f.Set("account.move", 901, "amount_residual", 0.0)
	f.Set("account.move", 901, "payment_state", "paid")
	f.Set("account.move", 901, "write_date", "2026-10-05 01:00:00")
	rep, err := s.Run(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Records["account.move"] == 0 {
		t.Fatalf("change not picked up: %+v", rep)
	}
	if count(t, st, "select count(*) from invoices where number = 'INV/0901' and paid = total") != 1 {
		t.Fatal("invoice not marked paid")
	}
}

func TestOrderStatePhases(t *testing.T) {
	cases := []struct {
		odoo                   string
		picked, invoiced, paid bool
		want                   string
	}{
		{"draft", false, false, false, "order"},
		{"sale", false, false, false, "siap"},
		{"sale", true, false, false, "kirim"},
		{"sale", true, true, false, "invoice"},
		{"sale", true, true, true, "bayar"},
		{"cancel", true, true, true, "cancel"},
	}
	for _, c := range cases {
		if got := odoo.OrderState(c.odoo, c.picked, c.invoiced, c.paid); got != c.want {
			t.Errorf("%+v → %s", c, got)
		}
	}
}

func TestWriteDisabled(t *testing.T) {
	f, _ := odoo.NewFake(db.Seed, "seed/odoo")
	if _, err := odoo.CreateSODraft(context.Background(), f, 3001, []odoo.DraftLine{{ProductID: 7001, Qty: 1, PriceUnit: 1}}, "p", "sam"); err != odoo.ErrWriteDisabled {
		t.Fatalf("write should be disabled, got %v", err)
	}
	f.Write = true
	id, err := odoo.CreateSODraft(context.Background(), f, 3001, []odoo.DraftLine{{ProductID: 7001, Qty: 10, PriceUnit: 1150000}}, "p1", "Sam Setiadi")
	if err != nil || id == 0 {
		t.Fatalf("draft: %d %v", id, err)
	}
	rows, _ := f.SearchRead(context.Background(), "sale.order", []any{[]any{"id", "=", id}}, nil)
	if len(rows) != 1 || rows[0].Str("note") != "Dibuat GSI Orbit · proposal p1 · disetujui Sam Setiadi" {
		t.Fatalf("draft note %v", rows)
	}
}
