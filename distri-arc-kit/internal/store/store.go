// Package store owns the PostgreSQL connection pool, migrations (goose) and the sqlc-generated queries.
package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"distri-arc/db"
	"distri-arc/internal/store/gen"
)

// Store bundles the pool and the typed queries.
type Store struct {
	Pool *pgxpool.Pool
	Q    *gen.Queries
}

// Open connects to PostgreSQL and verifies the connection.
//
// JIT is off for every connection: the planner's cost estimate of the chat list and the Orbit queries passes
// jit_above_cost, and compiling them took ~1.9 s of a 2.1 s chat list on the 2-CPU server for ~240 ms of work
// (20 Oct, EXPLAIN ANALYZE); short OLTP queries never win it back.
func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["jit"] = "off"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool, Q: gen.New(pool)}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.Pool.Close() }

// Tx runs fn in a transaction; the transaction is rolled back when fn returns an error.
func (s *Store) Tx(ctx context.Context, fn func(q *gen.Queries, tx pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(s.Q.WithTx(tx), tx); err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	return tx.Commit(ctx)
}

// Migrate applies all embedded goose migrations.
func (s *Store) Migrate(ctx context.Context) error {
	sub, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	sqlDB := stdlib.OpenDBFromPool(s.Pool)
	defer sqlDB.Close()
	// out of order: a migration numbered below one already applied (work merged in another order) still runs
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub, goose.WithAllowOutofOrder(true))
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

// Reset drops and recreates the public schema (tests and `arc ctl reset` in dev only).
func (s *Store) Reset(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `drop schema public cascade; create schema public;`)
	return err
}
