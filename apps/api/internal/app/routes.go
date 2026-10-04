package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"arc/packages/core/domain"
)

// Routes builds the HTTP handler.
func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc { return a.requireAuth("", h) }

	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/me", auth(a.handleMe))
	mux.HandleFunc("GET /api/shell", auth(a.handleShell))

	// Actions (decision endpoints are human-only by construction).
	mux.HandleFunc("GET /api/actions", auth(a.handleActions))
	mux.HandleFunc("GET /api/actions/{id}", auth(a.handleAction))
	mux.HandleFunc("POST /api/actions/{id}/decision", auth(a.handleDecision))
	mux.HandleFunc("GET /api/meta/reject-reasons", auth(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, domain.RejectReasons) }))

	mux.HandleFunc("GET /api/today", auth(a.handleToday))
	mux.HandleFunc("POST /api/ask", auth(a.handleAsk))
	mux.HandleFunc("GET /api/ask/history", auth(a.handleAskHistory))
	mux.HandleFunc("GET /api/ask/suggestions", auth(a.handleAskSuggestions))

	mux.HandleFunc("GET /api/chat/threads", auth(a.handleChatThreads))
	mux.HandleFunc("GET /api/chat/threads/{id}", auth(a.handleChatThread))
	mux.HandleFunc("GET /api/chat/threads/{id}/messages", auth(a.handleChatMessages))
	mux.HandleFunc("GET /api/chat/threads/{id}/suggestions", auth(a.handleChatSuggestions))
	mux.HandleFunc("POST /api/chat/threads/{id}/reply", auth(a.handleChatReply))
	mux.HandleFunc("POST /api/chat/tasks/{id}/send", auth(a.handleTaskSend))
	mux.HandleFunc("POST /api/chat/annotations/{id}/act", auth(a.handleAnnotationAct))
	mux.HandleFunc("POST /api/chat/groups/{id}/policy", auth(a.handleGroupPolicy))

	mux.HandleFunc("GET /api/accounts", auth(a.handleAccounts))
	mux.HandleFunc("GET /api/accounts/{id}", auth(a.handleAccount))
	mux.HandleFunc("POST /api/accounts/{id}/alternative", auth(a.handleAlternative))
	mux.HandleFunc("POST /api/accounts/{id}/whitespace", auth(a.handleWhitespace))
	mux.HandleFunc("POST /api/accounts/{id}/memory", auth(a.handleMemoryAppend))
	mux.HandleFunc("POST /api/signals/{id}/act", auth(a.handleSignalAct))
	mux.HandleFunc("POST /api/opportunities/{id}/override", auth(a.handleOverride))
	mux.HandleFunc("POST /api/opportunities/{id}/stage", auth(a.handleStageMove))
	mux.HandleFunc("POST /api/opportunities/{id}/write-probability", auth(a.handleWriteProbability))
	mux.HandleFunc("GET /api/people", auth(a.handlePeople))
	mux.HandleFunc("GET /api/opportunities", auth(a.handleOpportunities))
	mux.HandleFunc("GET /api/stages", auth(a.handleStages))
	mux.HandleFunc("GET /api/network", auth(a.handleNetwork))

	mux.HandleFunc("GET /api/pipeline", auth(a.handlePipeline))
	mux.HandleFunc("GET /api/forecast", auth(a.handleForecast))
	mux.HandleFunc("POST /api/sync/odoo", auth(a.handleSyncOdoo))
	mux.HandleFunc("POST /api/tenders/{id}/qualify", auth(a.handleTenderQualify))
	mux.HandleFunc("POST /api/tenders/{id}/skip", auth(a.handleTenderSkip))
	mux.HandleFunc("POST /api/tenders/import", auth(a.handleTenderImport))

	mux.HandleFunc("GET /api/prospects", auth(a.handleProspects))
	mux.HandleFunc("GET /api/funnel", auth(a.handleFunnel))
	mux.HandleFunc("GET /api/prospects/{id}", auth(a.handleProspect))
	mux.HandleFunc("POST /api/prospects/{id}/{verb}", auth(a.handleProspectAct))

	mux.HandleFunc("GET /api/cash", auth(a.handleCash))
	mux.HandleFunc("GET /api/cash/forecast", auth(a.handleCashForecast))
	mux.HandleFunc("GET /api/cash/forecast.csv", auth(a.handleCashForecastCSV))

	mux.HandleFunc("GET /api/settings/sources", auth(a.handleSettingsSources))
	mux.HandleFunc("GET /api/settings/whatsapp", auth(a.handleSettingsWhatsApp))
	mux.HandleFunc("POST /api/settings/whatsapp", auth(a.handleSettingsWhatsAppUpdate))
	mux.HandleFunc("POST /api/settings/privacy/{id}", auth(a.handlePrivacyRule))
	mux.HandleFunc("POST /api/wa/sessions/{id}/link", auth(a.handleSessionLink))
	mux.HandleFunc("POST /api/wa/sessions/{id}/instructions", auth(a.handleSessionInstructions))
	mux.HandleFunc("POST /api/wa/import", auth(a.handleExportUpload))
	mux.HandleFunc("GET /api/internal-numbers", auth(a.handleInternal))
	mux.HandleFunc("POST /api/internal-numbers", auth(a.handleInternalAdd))
	mux.HandleFunc("DELETE /api/internal-numbers/{id}", auth(a.handleInternalDelete))
	mux.HandleFunc("POST /api/internal-numbers/sync-talenta", auth(a.handleTalentaSync))
	mux.HandleFunc("POST /api/internal-numbers/import", auth(a.handleInternalImport))
	mux.HandleFunc("POST /api/internal-suspects/{id}/{verb}", auth(a.handleSuspect))
	mux.HandleFunc("GET /api/settings/ai", auth(a.handleSettingsAI))
	mux.HandleFunc("POST /api/settings/mcp-groups/{id}", auth(a.handleMCPGroup))
	mux.HandleFunc("POST /api/settings/routing", auth(a.handleRouting))
	mux.HandleFunc("GET /api/settings/api", auth(a.handleSettingsAPI))
	mux.HandleFunc("POST /api/api-keys", auth(a.handleKeyCreate))
	mux.HandleFunc("DELETE /api/api-keys/{id}", auth(a.handleKeyRevoke))
	mux.HandleFunc("GET /api/policies", auth(a.handlePolicies))
	mux.HandleFunc("PUT /api/policies/{key}", auth(a.handlePolicySet))
	mux.HandleFunc("GET /api/llm/usage", auth(a.handleLLMUsage))
	mux.HandleFunc("GET /api/metrics", auth(a.handleMetrics))
	mux.HandleFunc("POST /api/jobs/{name}/run", auth(a.handleJobRun))
	mux.HandleFunc("POST /jobs/{name}/run", auth(a.handleJobRun))
	mux.HandleFunc("POST /api/webhooks/subscriptions", auth(a.handleWebhookSubscribe))
	mux.HandleFunc("GET /api/google/connect", auth(a.handleGoogleConnect))
	mux.HandleFunc("GET /api/google/callback", auth(a.handleGoogleCallback))

	// Webhooks (signature-verified, no session).
	mux.HandleFunc("POST /webhooks/wa", a.handleBridgeWebhook)
	mux.HandleFunc("GET /webhooks/wa-cloud", a.handleCloudWebhook)
	mux.HandleFunc("POST /webhooks/wa-cloud", a.handleCloudWebhook)
	mux.HandleFunc("GET /bridge/actions/{id}", a.handleBridgeActionCheck)

	// Public AI API v1 (API keys with scopes; OAuth tokens).
	a.publicRoutes(mux)
	// MCP + OAuth 2.1.
	a.mcpRoutes(mux)

	// Static web build (Vite) served by the API in production.
	dist := a.Cfg.WebDist
	if !filepath.IsAbs(dist) {
		dist = filepath.Join(a.root, dist)
	}
	fs := http.FileServer(http.Dir(dist))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dist, filepath.Clean(r.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(dist, "index.html")
		if _, err := os.Stat(index); err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"service": "ARC API", "hint": "jalankan web di :5173 (make dev) atau build apps/web"})
			return
		}
		http.ServeFile(w, r, index)
	})
	return a.logRequests(securityHeaders(mux))
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		h.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (a *App) logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		h.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		a.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	db := "ok"
	if err := a.DB.Pool.Ping(r.Context()); err != nil {
		db = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "stage": a.stage(), "db": db, "llm": a.LLM.RouteFor("heavy").Provider,
		"mocks": map[string]bool{"odoo": a.OdooMock, "google": a.GoogleMock, "llm": a.LLM.IsFake("heavy")}, "clock": domain.Now().Format(time.RFC3339)})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	writeJSON(w, http.StatusOK, meView(p))
}
