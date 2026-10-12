package dealersvc_test

import (
	"context"
	"testing"
	"time"

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
