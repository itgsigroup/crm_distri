// Package api serves the REST + SSE API under /api (docs/design/07-api.md).
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/ask"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/events"
	"distri-arc/internal/httpx"
	"distri-arc/internal/identify"
	"distri-arc/internal/mcp"
	"distri-arc/internal/odoo"
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
	return &Server{cfg: cfg, st: st, clock: c, log: log, views: views.NewBuilder(st, c), hub: events.NewHub()}
}

// Hub returns the SSE hub (fed by events.Listen).
func (s *Server) Hub() *events.Hub { return s.hub }

// Handler returns the HTTP handler with middleware and routes.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, httpx.Logger(s.log), middleware.Recoverer)
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
		r.Group(func(r chi.Router) {
			r.Use(s.auth)
			r.Get("/me", s.me)
			s.readRoutes(r)
			s.chatRoutes(r)
			s.connectionRoutes(r)
			s.proposalRoutes(r)
			s.cycleRoutes(r)
			s.mcpRoutes(r)
			s.relasiRoutes(r)
			s.identifyRoutes(r)
			r.Post("/ask", s.askHandler)
			r.Get("/events", s.events)
			r.Get("/brief/today", s.briefToday)
		})
	})
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	res := map[string]any{"db": "ok", "queue": "ok", "now": s.clock.Now()}
	status := http.StatusOK
	if _, err := s.st.Q.Ping(r.Context()); err != nil {
		res["db"], status = "error", http.StatusServiceUnavailable
	}
	var sample bool
	_ = s.st.Pool.QueryRow(r.Context(), "select exists(select 1 from signals where dedupe_key like 'seed:%')").Scan(&sample)
	res["sample_data"] = sample
	if depth, err := s.st.Q.QueueDepth(r.Context()); err != nil {
		res["queue"], status = "error", http.StatusServiceUnavailable
	} else {
		res["queue_depth"] = depth
	}
	httpx.JSON(w, status, res)
}

type ctxKey int

const userKey ctxKey = 1

// User is the authenticated human user of a request.
type User = gen.GetUserByEmailRow

// auth resolves the user. Until stage 11 (sessions) the only mechanism is the X-Dev-User header (an email),
// accepted only when APP_ENV=dev; the web dev server sends the CEO's address.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := strings.TrimSpace(r.Header.Get("X-Dev-User"))
		if !s.cfg.IsDev() || email == "" {
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

// CurrentUser returns the authenticated user.
func CurrentUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role, "branch": u.Branch})
}
