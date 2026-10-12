package dealersvc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// A cycle and the worker recompute at the same time (production: deadlock on RecountContacts); they take turns.
func TestConcurrentRecompute(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	svc := dealersvc.New(st, clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)))
	var g errgroup.Group
	for range 6 {
		g.Go(func() error { _, err := svc.Recompute(ctx); return err })
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
}

// A message of one dealer recounts only that dealer's contacts (the old recompute touched all 2.5k on every message);
// the periodic full recompute (no ids) brings every other contact up to date.
func TestRecomputeRecountsOnlyGivenDealers(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	svc := dealersvc.New(st, clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)))
	// stale counters everywhere, then recompute one dealer
	if _, err := st.Pool.Exec(ctx, "update contacts set interactions_90d = -1 where wa_number is not null"); err != nil {
		t.Fatal(err)
	}
	var dealer uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select dealer_id from contacts where wa_number is not null and dealer_id is not null limit 1").Scan(&dealer); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Recompute(ctx, dealer); err != nil {
		t.Fatal(err)
	}
	var own, others int
	_ = st.Pool.QueryRow(ctx, "select count(*) filter (where dealer_id = $1 and interactions_90d >= 0), count(*) filter (where dealer_id is distinct from $1 and interactions_90d = -1) from contacts where wa_number is not null", dealer).Scan(&own, &others)
	if own == 0 || others == 0 {
		t.Fatalf("own recounted %d, others left alone %d", own, others)
	}
	if _, err := svc.Recompute(ctx); err != nil {
		t.Fatal(err)
	}
	var stale int
	_ = st.Pool.QueryRow(ctx, "select count(*) from contacts where wa_number is not null and interactions_90d = -1").Scan(&stale)
	if stale != 0 {
		t.Fatalf("%d contacts still stale after the full recompute", stale)
	}
}
