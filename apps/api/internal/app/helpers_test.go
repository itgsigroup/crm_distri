package app

// Shared harness for the per-stage acceptance tests. Every test that touches the
// database resets and re-seeds the dedicated arc_test database, so these tests
// must never run in parallel (no t.Parallel anywhere in this package).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"arc/apps/api/internal/seed"
	"arc/packages/core/config"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

const defaultTestDSN = "postgres://rizalfahrezi@localhost:5433/arc_test?sslmode=disable"

// demoAnchor is the "now" of the approved mockup; fixture dates are relative to it.
var demoAnchor = time.Date(2026, 9, 28, 17, 2, 0, 0, domain.Jakarta)

func testDSN() string {
	if v := os.Getenv("ARC_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return defaultTestDSN
}

// testConfig builds a config that can never reach a real external service.
func testConfig() config.Config {
	cfg := config.Load()
	cfg.Env = "test"
	cfg.DatabaseURL = testDSN()
	cfg.LLMProvider = "auto"
	cfg.AnthropicKey, cfg.OpenAIKey, cfg.OllamaURL = "", "", ""
	cfg.WACloudToken, cfg.WACloudPhoneID, cfg.WACloudAppSecret = "", "", ""
	cfg.TruecallerKey, cfg.WebSearchKey = "", ""
	cfg.OdooURL, cfg.OdooDB, cfg.OdooUser, cfg.OdooAPIKey, cfg.OdooCompanies = "", "", "", "", ""
	cfg.GoogleClientID, cfg.GoogleClientSecret = "", ""
	cfg.BasecampToken, cfg.BasecampAccountID, cfg.BasecampProjectID = "", "", ""
	cfg.SMTPHost, cfg.SMTPUser, cfg.SMTPPass = "", "", ""
	cfg.BridgeURL = "http://127.0.0.1:9" // discard port: the bridge is never reachable in tests
	cfg.BridgeSecret = "test-bridge-secret"
	cfg.SchedulerEnabled = false
	cfg.PublicURL = "http://arc.test"
	cfg.ClockAnchor = time.Time{}
	return cfg
}

// fresh resets arc_test, seeds the fixtures exactly like `arc seed`, and returns
// the app plus an HTTP test server serving its routes.
func fresh(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	domain.SetClockAnchor(demoAnchor)
	cfg := testConfig()
	db, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Skipf("test database unreachable (%v); set ARC_TEST_DATABASE_URL", err)
	}
	t.Cleanup(db.Close)
	if err := db.ResetSchema(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	f := loadFixtures(t)
	if err := seed.Run(ctx, db, f); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a, err := New(ctx, cfg, db)
	if err != nil {
		t.Fatalf("app: %v", err)
	}
	if !a.OdooMock || !a.GoogleMock || !a.LLM.IsFake("heavy") {
		t.Fatalf("test app must run with mocks only (odoo=%v google=%v llm-fake=%v)", a.OdooMock, a.GoogleMock, a.LLM.IsFake("heavy"))
	}
	// No background capture timers: tests trigger capture explicitly.
	a.debounce = nil
	if err := a.PostSeed(ctx, f); err != nil {
		t.Fatalf("post-seed: %v", err)
	}
	srv := httptest.NewServer(a.Routes())
	t.Cleanup(srv.Close)
	return a, srv
}

func loadFixtures(t *testing.T) *seed.Fixtures {
	t.Helper()
	f, err := seed.Load(seed.FixtureDir(config.RepoRoot()))
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	return f
}

// client is an HTTP client bound to the test server, logged in as a user or
// carrying a bearer token.
type client struct {
	t      *testing.T
	base   string
	http   *http.Client
	bearer string
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

// login opens a browser-style session (arc_session cookie).
func login(t *testing.T, srv *httptest.Server, email string) *client {
	t.Helper()
	c := newClient(t, srv)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"email": email, "password": seed.DefaultPassword})
	if code != 200 {
		t.Fatalf("login %s: %d %s", email, code, body)
	}
	return c
}

// bearerClient authenticates with a token only (no cookie).
func bearerClient(t *testing.T, srv *httptest.Server, token string) *client {
	c := newClient(t, srv)
	c.bearer = token
	return c
}

func (c *client) do(method, path string, body any) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		if raw, ok := body.([]byte); ok {
			rd = bytes.NewReader(raw)
		} else {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// json performs a request, requires the status code and decodes the body.
func (c *client) json(method, path string, body any, want int, out any) {
	c.t.Helper()
	code, raw := c.do(method, path, body)
	if code != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, code, want, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode: %v: %s", method, path, err, raw)
		}
	}
}

// apiKey creates an API key owned by the session user and returns the secret.
func (c *client) apiKey(name string, scopes ...string) string {
	c.t.Helper()
	var res struct {
		Key string `json:"key"`
	}
	c.json("POST", "/api/api-keys", map[string]any{"name": name, "scopes": scopes}, 200, &res)
	if !strings.HasPrefix(res.Key, "arc_live_") {
		c.t.Fatalf("unexpected key %q", res.Key)
	}
	return res.Key
}

// count runs a COUNT(*) style query.
func count(t *testing.T, a *App, q string, args ...any) int {
	t.Helper()
	var n int
	if err := a.DB.Pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

func exec(t *testing.T, a *App, q string, args ...any) {
	t.Helper()
	if _, err := a.DB.Pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// within reports |got-want| <= tol.
func within(got, want, tol float64) bool { return got >= want-tol && got <= want+tol }
