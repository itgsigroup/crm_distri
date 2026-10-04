// Package storage owns the PostgreSQL connection, schema migrations, audit
// trail and the repositories for every ARC aggregate.
package storage

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB wraps a pgx pool. Repositories hang off it as methods.
type DB struct {
	Pool *pgxpool.Pool
}

// Querier is satisfied by both the pool and a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ErrNotFound is returned by single-row getters.
var ErrNotFound = errors.New("not found")

// Open connects to PostgreSQL using a DSN such as
// postgres://user@localhost:5433/arc?sslmode=disable.
func Open(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 16
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Close releases the pool.
func (db *DB) Close() { db.Pool.Close() }

// Migrate applies embedded migrations in filename order. Each migration runs
// in its own transaction and is recorded in schema_migrations.
func (db *DB) Migrate(ctx context.Context) ([]string, error) {
	if _, err := db.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var applied []string
	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		var exists bool
		if err := db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return applied, err
		}
		if exists {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return applied, err
		}
		err = db.Tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return applied, err
		}
		applied = append(applied, version)
	}
	return applied, nil
}

// Tx runs fn inside a transaction, committing on success.
func (db *DB) Tx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Actor identifies who performed an audited operation.
type Actor struct {
	ID   string
	Type string // user | agent | machine | system
}

// SystemActor is used by seeds, migrations and schedulers.
var SystemActor = Actor{ID: "system", Type: "system"}

// Audit appends an entry to the append-only audit log.
func Audit(ctx context.Context, q Querier, actor Actor, verb, objType, objID string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if payload == nil {
		raw = []byte("{}")
	}
	sum := sha256.Sum256(raw)
	_, err = q.Exec(ctx, `INSERT INTO audit_log(actor, actor_type, verb, object_type, object_id, payload_hash, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, actor.ID, actor.Type, verb, objType, objID, hex.EncodeToString(sum[:]), raw)
	return err
}

// JSON marshals v for a jsonb column, defaulting nil slices/maps sensibly.
func JSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return []byte("[]")
	}
	return b
}

// JSONObj marshals v, defaulting to an empty object.
func JSONObj(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return []byte("{}")
	}
	return b
}

// Hash returns a short stable hex digest of the joined parts.
func Hash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])[:24]
}

// ResetSchema drops everything in the public schema. Used only by tests.
func (db *DB) ResetSchema(ctx context.Context) error {
	_, err := db.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	return err
}
