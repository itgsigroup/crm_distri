package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Env: "dev"}
	h := api.New(cfg, st, clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB)), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path, user string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if user != "" {
		req.Header.Set("X-Dev-User", user)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body
}

func TestHealth(t *testing.T) {
	srv := newServer(t)
	code, body := get(t, srv, "/api/health", "")
	if code != 200 || body["db"] != "ok" || body["queue"] != "ok" {
		t.Fatalf("health: %d %v", code, body)
	}
}

func TestMeRequiresDevUser(t *testing.T) {
	srv := newServer(t)
	if code, _ := get(t, srv, "/api/me", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous /me: %d", code)
	}
	code, body := get(t, srv, "/api/me", "sam@gsi.co.id")
	if code != 200 || body["role"] != "ceo" || body["name"] != "Sam Setiadi" {
		t.Fatalf("/me: %d %v", code, body)
	}
}
