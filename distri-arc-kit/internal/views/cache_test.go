package views_test

import (
	"context"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
	"distri-arc/internal/views"
)

// The computed board is reused within BoardTTL (a request does not recompute thousands of dealers); every call
// still gets its own copy of the items; Invalidate forces a rebuild.
func TestBoardCache(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	b := views.NewBuilder(st, clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB)))
	first, err := b.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "update dealers set name = 'Ganti Nama' where slug = 'mitra'"); err != nil {
		t.Fatal(err)
	}
	second, _ := b.Board(ctx)
	if second.Items[0].Name != first.Items[0].Name || &second.Items[0] == &first.Items[0] {
		t.Fatal("second call should reuse the computed board, on its own copy of the items")
	}
	for _, it := range second.Items {
		if it.ID == "mitra" && it.Name == "Ganti Nama" {
			t.Fatal("cached board recomputed")
		}
	}
	b.Invalidate()
	third, _ := b.Board(ctx)
	found := false
	for _, it := range third.Items {
		found = found || (it.ID == "mitra" && it.Name == "Ganti Nama")
	}
	if !found {
		t.Fatal("Invalidate did not rebuild")
	}
}
