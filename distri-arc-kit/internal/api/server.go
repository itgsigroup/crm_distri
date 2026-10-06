// Package api serves the REST + SSE API under /api (docs/design/07-api.md).
package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/ask"
	"distri-arc/internal/auth"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/httpx"
	"distri-arc/internal/identify"
	"distri-arc/internal/mcp"
	"distri-arc/internal/odoo"
	"distri-arc/internal/ops"
	"distri-arc/internal/policy"
	"distri-arc/internal/proposals"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
)

// Server wires handlers to the store.
type Server struct {
	cfg   config.Config
	st    *store.Store
	clock clock.Clock
	log   *slog.Logger
	views *views.Builder
	hub   *events.Hub
	jobs  *river.Client[pgx.Tx]
	cloud *wa.CloudAPI
	odoo  odoo.Source
	mcp   *mcp.Server
	idf   *identify.Service
	ask   *ask.Service
	http  *ops.HTTPMetrics
}

// WithAsk enables POST /ask (the command bar).
func (s *Server) WithAsk(a *ask.Service) *Server { s.ask = a; return s }

// WithIdentify enables POST /chat/identify (sources the API can read; the worker adds the WA Business profile).
func (s *Server) WithIdentify(i *identify.Service) *Server { s.idf = i; return s }

// WithMCP mounts the MCP server at /mcp (bearer tokens, not the user session).
func (s *Server) WithMCP(m *mcp.Server) *Server { s.mcp = m; return s }

// WithJobs lets the API enqueue jobs (outbox.send, wa.pair) in the same transaction as its writes.
func (s *Server) WithJobs(c *river.Client[pgx.Tx]) *Server { s.jobs = c; return s }

// WithCloudWebhook mounts the WhatsApp Cloud API webhook (public, signature-verified).
func (s *Server) WithCloudWebhook(c *wa.CloudAPI) *Server { s.cloud = c; return s }

// New builds the API server.
func New(cfg config.Config, st *store.Store, c clock.Clock, log *slog.Logger) *Server {
	return &Server{cfg: cfg, st: st, clock: c, log: log, views: views.NewBuilder(st, c), hub: events.NewHub(), http: ops.NewHTTPMetrics()}
}

// Hub returns the SSE hub (fed by events.Listen).
func (s *Server) Hub() *events.Hub { return s.hub }

// Handler returns the HTTP handler with middleware and routes.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, httpx.Logger(s.log), middleware.Recoverer, s.http.Middleware)
	r.Get("/metrics", s.metrics)
	if s.mcp != nil {
		r.Handle("/mcp", s.mcp.Handler())
	}
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.health)
		if s.cloud != nil {
			ingest := wa.NewIngestor(s.st, s.log)
			r.Method(http.MethodGet, "/wa/cloud/webhook", s.cloud.Webhook(func(ctx context.Context, m wa.Message) error { _, err := ingest.Process(ctx, m); return err }))
			r.Method(http.MethodPost, "/wa/cloud/webhook", s.cloud.Webhook(func(ctx context.Context, m wa.Message) error { _, err := ingest.Process(ctx, m); return err }))
		}
		r.Post("/auth/login", s.login)
		r.Post("/auth/logout", s.logout)
		r.Group(func(r chi.Router) {
			r.Use(s.auth)
			s.userRoutes(r)
			r.Get("/me", s.me)
			s.readRoutes(r)
			s.chatRoutes(r)
			s.connectionRoutes(r)
			s.proposalRoutes(r)
			s.cycleRoutes(r)
			s.mcpRoutes(r)
			s.relasiRoutes(r)
			s.identifyRoutes(r)
			s.pilotRoutes(r)
			s.dataRoutes(r)
			r.Post("/ask", s.askHandler)
			r.Get("/events", s.events)
			r.Get("/brief/today", s.briefToday)
		})
	})
	return r
}

func (s *Server) opsEnv() ops.Env { return ops.EnvFrom(s.cfg) }

// health is public for uptime checks: anonymous callers get the status of each part; CEO/admin (Pengaturan →
// Status sistem) get the details (numbers, sync models, alerts).
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	h := ops.Check(r.Context(), s.st, s.opsEnv(), s.clock.Now())
	status := http.StatusOK
	if h.Status == "down" {
		status = http.StatusServiceUnavailable
	}
	if u, ok := s.userFrom(r); ok && (deref(u.Role) == "ceo" || deref(u.Role) == "admin") {
		httpx.JSON(w, status, h)
		return
	}
	connected := 0
	for _, n := range h.WA {
		if n.State == "connected" {
			connected++
		}
	}
	httpx.JSON(w, status, map[string]any{"status": h.Status, "now": h.Now, "version": h.Version, "db": h.DB, "queue": h.Queue,
		"queue_depth": h.QueueDepth, "sample_data": h.SampleData, "wa": map[string]int{"connected": connected, "total": len(h.WA)},
		"odoo": h.Odoo["status"], "llm": h.LLM["status"], "alerts": len(h.Alerts)})
}

// metrics serves Prometheus text. With METRICS_TOKEN set it needs that bearer token; without, only loopback
// (Caddy never proxies /metrics).
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MetricsToken != "" {
		if r.Header.Get("Authorization") != "Bearer "+s.cfg.MetricsToken {
			httpx.Fail(w, http.StatusUnauthorized, "unauthenticated", "Token metrics salah")
			return
		}
	} else if host, _, _ := strings.Cut(r.RemoteAddr, ":"); !s.cfg.IsDev() && host != "127.0.0.1" && !strings.HasPrefix(r.RemoteAddr, "[::1]") {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Metrics hanya dari localhost atau dengan METRICS_TOKEN")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.http.Write(w, ops.Check(r.Context(), s.st, s.opsEnv(), s.clock.Now()))
}

type ctxKey int

const userKey ctxKey = 1

// User is the authenticated human user of a request.
type User = gen.GetUserByEmailRow

// sessionCookie carries the signed session token (HttpOnly, SameSite=Strict).
const sessionCookie = "arc_session"

// sessionTTL is how long a login lasts.
const sessionTTL = 12 * time.Hour

// secret is the session signing key: SESSION_SECRET, or a fixed development key when APP_ENV=dev.
func (s *Server) secret() []byte { return s.cfg.SessionKey() }

// auth resolves the user: the session cookie first; the X-Dev-User header (an email) only when APP_ENV=dev, so
// the web dev server works without logging in while the login flow stays testable.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := s.sessionEmail(r)
		if email == "" {
			httpx.Fail(w, http.StatusUnauthorized, "unauthenticated", "Login diperlukan")
			return
		}
		u, err := s.st.Q.GetUserByEmail(r.Context(), email)
		if err != nil {
			httpx.Fail(w, http.StatusUnauthorized, "unknown_user", "Pengguna tidak dikenal")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (s *Server) sessionEmail(r *http.Request) string {
	email := ""
	if c, err := r.Cookie(sessionCookie); err == nil && s.secret() != nil {
		if cl, err := auth.Verify(c.Value, s.secret(), time.Now()); err == nil {
			email = cl.Email
		}
	}
	if email == "" && s.cfg.IsDev() {
		email = strings.TrimSpace(r.Header.Get("X-Dev-User"))
	}
	return email
}

// userFrom resolves the user on a public route (no 401 when there is none).
func (s *Server) userFrom(r *http.Request) (User, bool) {
	email := s.sessionEmail(r)
	if email == "" {
		return User{}, false
	}
	u, err := s.st.Q.GetUserByEmail(r.Context(), email)
	return u, err == nil
}

// CurrentUser returns the authenticated user.
func CurrentUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	role := deref(u.Role)
	decide := []string{}
	for _, k := range domain.AllKinds {
		if slices.Contains(proposals.RolesFor(k), role) {
			decide = append(decide, k)
		}
	}
	pilotMode := "off"
	if pol, err := policy.Load(r.Context(), s.st.Q); err == nil {
		pilotMode = pol.Pilot.Mode
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role, "branch": u.Branch,
		"screens": screensFor(role), "decide": decide, "edit_policies": role == "ceo", "manage_users": role == "ceo" || role == "admin",
		"totp_available": totpRoles(role), "totp_enabled": u.TotpEnabledAt != nil, "pilot_mode": pilotMode})
}

// screensFor is the menu of a role (Pengaturan only for CEO and admin; finance works on credit, warehouse on stock).
func screensFor(role string) []string {
	switch role {
	case "sales":
		return []string{"today", "orch", "chat", "orbit", "kuad", "net", "dealer", "stock", "konsep"}
	case "finance":
		return []string{"today", "orch", "orbit", "kuad", "dealer", "ar", "konsep"}
	case "warehouse":
		return []string{"today", "chat", "stock", "dealer", "konsep"}
	}
	return []string{"today", "orch", "chat", "orbit", "kuad", "net", "dealer", "stock", "ar", "conn", "konsep"}
}
