// Package api serves the REST + SSE API under /api (docs/design/07-api.md).
package api

import (
	"context"
	"distri-arc/internal/access"
	"distri-arc/internal/analyst"
	"distri-arc/internal/oauth"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/ask"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/events"
	"distri-arc/internal/httpx"
	"distri-arc/internal/identify"
	"distri-arc/internal/llm"
	"distri-arc/internal/mcp"
	"distri-arc/internal/odoo"
	"distri-arc/internal/ops"
	"distri-arc/internal/policy"
	"distri-arc/internal/proposals"
	"distri-arc/internal/replyai"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
)

// Server wires handlers to the store.
type Server struct {
	cfg      config.Config
	st       *store.Store
	clock    clock.Clock
	log      *slog.Logger
	views    *views.Builder
	hub      *events.Hub
	threads  threadList
	jobs     *river.Client[pgx.Tx]
	cloud    *wa.CloudAPI
	media    wa.MediaFetcher // chat media from WhatsApp; nil = the Baileys bridge when WA_TRANSPORT=baileys
	roster   wa.GroupRoster  // group members from WhatsApp; nil = the Baileys bridge when WA_TRANSPORT=baileys
	contacts wa.ContactSaver // saves names to a number's WhatsApp contacts; nil = the Baileys bridge
	// rosters: group members as each GSI number's phone names them, kept a few minutes (every group page shows them)
	rostersMu sync.Mutex
	rosters   map[string]rosterEntry
	odoo      odoo.Source
	mcp       *mcp.Server
	idf       *identify.Service
	ask       *ask.Service
	replyAI   *replyai.Service
	http      *ops.HTTPMetrics
	// scheduled analysis seams (tests): the Claude client of inline runs and the API-key check
	analystModel func(key string) analyst.Model
	verifyKey    func(ctx context.Context, key string) error
	verifyOpenAI func(ctx context.Context, key string) error
}

// WithMedia sets where chat media is fetched from (tests; production uses the Baileys bridge).
func (s *Server) WithMedia(f wa.MediaFetcher) *Server { s.media = f; return s }

// WithContacts sets where names are saved as WhatsApp contacts (tests; production uses the Baileys bridge).
func (s *Server) WithContacts(c wa.ContactSaver) *Server { s.contacts = c; return s }

// WithRoster sets where group members are read from (tests; production uses the Baileys bridge).
func (s *Server) WithRoster(r wa.GroupRoster) *Server { s.roster = r; return s }

// WithAsk enables POST /ask (the command bar).
func (s *Server) WithAsk(a *ask.Service) *Server {
	s.ask = a
	if a != nil {
		s.replyAI.Env = a.Router // AI Trigger drafts: LLM_PROVIDER when no key is saved in the app
	}
	return s
}

// WithReplyModel sets the model AI Trigger drafts are written with (tests).
func (s *Server) WithReplyModel(p llm.Provider) *Server { s.replyAI.Provider = p; return s }

// WithReplyMatcher sets the Claude model that finds the products of a message (tests).
func (s *Server) WithReplyMatcher(p llm.Provider) *Server { s.replyAI.Matcher = p; return s }

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
	srv := &Server{cfg: cfg, st: st, clock: c, log: log, views: views.NewBuilder(st, c), hub: events.NewHub(), http: ops.NewHTTPMetrics()}
	srv.hub.OnPublish = srv.threads.changed
	srv.replyAI = &replyai.Service{St: st, Views: srv.views, Log: log, SessionKey: cfg.SessionKey(), EnvClaude: cfg.AnthropicKey, EnvOpenAI: cfg.OpenAIKey}
	return srv
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
		(&oauth.Server{St: s.st, Base: s.publicBase, Log: s.log}).Routes(r) // OAuth 2.1 for Claude (ADR 0021)
	}
	r.Route("/api", func(r chi.Router) {
		// JSON is gzip-compressed (a board of thousands of dealers is ~10× smaller); the SSE stream is not touched
		r.Use(middleware.Compress(5, "application/json"))
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
			r.Use(s.freshAfterWrite)
			s.userRoutes(r)
			s.roleRoutes(r)
			s.branchRoutes(r)
			s.salesMapRoutes(r)
			r.Get("/me", s.me)
			s.readRoutes(r)
			s.chatRoutes(r)
			s.replyRuleRoutes(r)
			s.waAnalysisRoutes(r)
			s.connectionRoutes(r)
			s.proposalRoutes(r)
			s.cycleRoutes(r)
			s.mcpRoutes(r)
			s.analystRoutes(r)
			s.aiAPIRoutes(r)
			s.autoReplyRoutes(r)
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
	if u, ok := s.userFrom(r); ok && isAdmin(u) {
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

// secret is the session signing key: SESSION_SECRET, or a fixed development key when APP_ENV=dev.
func (s *Server) secret() []byte { return s.cfg.SessionKey() }

// auth resolves the user: the session cookie first; the X-Dev-User header (an email) only when APP_ENV=dev, so
// the web dev server works without logging in while the login flow stays testable.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email, cl, err := s.sessionEmail(r)
		if err != nil {
			// a database hiccup is no logout: 503, so the page retries instead of sending the person to log in
			httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Server sedang sibuk, coba lagi sebentar")
			return
		}
		if email == "" {
			httpx.Fail(w, http.StatusUnauthorized, "unauthenticated", "Login diperlukan")
			return
		}
		u, err := s.st.Q.GetUserByEmail(r.Context(), email)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, http.StatusUnauthorized, "unknown_user", "Pengguna tidak dikenal")
			return
		} else if err != nil {
			httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Server sedang sibuk, coba lagi sebentar")
			return
		}
		s.renewSession(w, r, u, cl)
		// a role without a screen cannot call that screen's API either (not only a hidden menu)
		if sc := access.ScreenForPath(r.URL.Path); sc != "" && !slices.Contains(roleOf(u).Screens, sc) {
			httpx.Fail(w, http.StatusForbidden, "forbidden", "Peran Anda tidak membuka menu ini — atur di Pengaturan → Peran & akses")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// freshAfterWrite drops the cached dealer board after any change made through the API (a policy, a dealer's
// master data, a mapping, a decision), so the next read shows it at once.
func (s *Server) freshAfterWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.views.Invalidate()
		}
	})
}

// userFrom resolves the user on a public route (no 401 when there is none).
func (s *Server) userFrom(r *http.Request) (User, bool) {
	email, _, _ := s.sessionEmail(r)
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

// roleOf is the user's effective access (role master narrowing their base role).
func roleOf(u User) access.Role {
	return access.Resolve(deref(u.RoleKey), deref(u.RoleName), deref(u.Role), u.RoleScreens, u.RoleDecide, u.RoleScope, u.RolePolicies, u.RoleWa)
}

// decider is the person deciding a proposal, with the kinds their role lets them decide.
func decider(u User) proposals.Decider {
	ro := roleOf(u)
	return proposals.Decider{SalesUserID: *u.SalesUserID, Name: deref(u.Name), Role: deref(u.Role), Email: deref(u.Email), Decide: ro.Decide, RoleName: ro.Name}
}

// personOf is the signed-in user with a person profile to record their decisions against (proposals.decided_by).
// Accounts made before every account got one (createUser) get it here, so replying in Chat or deciding never stops
// at "not linked to sales data".
func (s *Server) personOf(w http.ResponseWriter, r *http.Request) (User, bool) {
	u, _ := CurrentUser(r.Context())
	if u.SalesUserID != nil {
		return u, true
	}
	name := strings.TrimSpace(deref(u.Name))
	if name == "" {
		name = deref(u.Email)
	}
	role := deref(u.Role)
	if !slices.Contains([]string{"ceo", "sales", "admin", "finance", "warehouse"}, role) {
		role = "admin"
	}
	id, err := s.st.Q.CreatePersonProfile(r.Context(), gen.CreatePersonProfileParams{Name: name, Branch: "Semua cabang", Role: role, Email: u.Email})
	if err == nil {
		var linked *uuid.UUID
		if linked, err = s.st.Q.LinkPersonProfile(r.Context(), gen.LinkPersonProfileParams{ID: u.ID, SalesUserID: &id}); err == nil {
			u.SalesUserID = linked
		}
	}
	if err != nil || u.SalesUserID == nil {
		httpx.Fail(w, http.StatusInternalServerError, "no_sales_user", "Profil pengguna belum bisa dibuat — coba lagi")
		return u, false
	}
	return u, true
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	role := deref(u.Role)
	ro := roleOf(u)
	decide := ro.Decide
	pilotMode := "off"
	if pol, err := policy.Load(r.Context(), s.st.Q); err == nil {
		pilotMode = pol.Pilot.Mode
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role, "branch": u.Branch,
		"screens": ro.Screens, "decide": decide, "edit_policies": role == "ceo", "manage_users": can(u, "users"), "scope": ro.Scope,
		"role_key": ro.Key, "role_name": ro.Name, "wa_number": u.WaNumber, "wa_allowed": ro.WAAllowed, "chat_scope": map[bool]string{true: "own", false: "all"}[waOwnOnly(u)],
		"totp_available": totpRoles(role), "totp_enabled": u.TotpEnabledAt != nil, "pilot_mode": pilotMode})
}
