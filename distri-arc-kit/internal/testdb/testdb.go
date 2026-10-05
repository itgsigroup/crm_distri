// Package testdb gives integration tests an isolated, migrated PostgreSQL schema inside DATABASE_URL_TEST
// (no Docker needed; CI provides a postgres service). Each call creates a fresh schema and drops it afterwards,
// so packages can run in parallel against one database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"distri-arc/internal/store"
)

// URL returns DATABASE_URL_TEST from the environment or the nearest .env file.
func URL() string {
	if v := os.Getenv("DATABASE_URL_TEST"); v != "" {
		return v
	}
	dir, _ := os.Getwd()
	for range 6 {
		if b, err := os.ReadFile(filepath.Join(dir, ".env")); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(l), "DATABASE_URL_TEST="); ok {
					return strings.Trim(v, `"'`)
				}
			}
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// New returns a migrated store on a throw-away schema.
func New(t testing.TB) *store.Store {
	t.Helper()
	base := URL()
	if base == "" {
		t.Skip("DATABASE_URL_TEST not set")
	}
	ctx := context.Background()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "t_" + hex.EncodeToString(b)

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	defer admin.Close(ctx)
	// Extensions are database-wide: keep them in public so every test schema sees the same operators.
	if _, err := admin.Exec(ctx, "create extension if not exists pgcrypto with schema public; create extension if not exists pg_trgm with schema public"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), base)
		if err == nil {
			_, _ = c.Exec(context.Background(), "drop schema "+schema+" cascade")
			_ = c.Close(context.Background())
		}
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	st, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}
