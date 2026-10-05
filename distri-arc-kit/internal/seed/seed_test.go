package seed_test

import (
	"context"
	"testing"

	"distri-arc/db"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

func TestSeedIsIdempotent(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	first, err := seed.Run(ctx, st, db.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if first.Dealers != 18 || first.SalesUsers < 4 || first.Signals < 60 {
		t.Fatalf("unexpected seed counts: %+v", first)
	}
	second, err := seed.Run(ctx, st, db.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("seed not idempotent:\n first %+v\nsecond %+v", first, second)
	}
}
