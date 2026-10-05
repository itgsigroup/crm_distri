// Package api serves the REST + SSE API under /api (docs/design/07-api.md).
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/httpx"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Server wires handlers to the store.
type Server struct {
	cfg   config.Config
	st    *store.Store
	clock clock.Clock
	log   *slog.Logger
}

// New builds the API server.
func New(cfg config.Config, st *store.Store, c clock.Clock, log *slog.Logger) *Server {
	return &Server{cfg: cfg, st: st, clock: c, log: log}
}

// Handler returns the HTTP handler with middleware and routes.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, httpx.Logger(s.log), middleware.Recoverer)
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.health)
		r.Group(func(r chi.Router) {
			r.Use(s.auth)
			r.Get("/me", s.me)
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
