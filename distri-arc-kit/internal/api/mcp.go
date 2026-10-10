package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/httpx"
	"distri-arc/internal/mcp"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
)

func (s *Server) mcpRoutes(r chi.Router) {
	r.Get("/mcp/info", s.mcpInfo)
	r.Get("/mcp/clients", s.mcpClients)
	r.Post("/mcp/clients", s.createMCPClient)
	r.Delete("/mcp/clients/{id}", s.revokeMCPClient)
	r.Get("/mcp/calls", s.mcpCalls)
	r.Get("/mcp/usage", s.aiUsage)
	r.Get("/oauth/requests/{id}", s.oauthRequest)
	r.Post("/oauth/requests/{id}/approve", s.oauthDecide(true))
	r.Post("/oauth/requests/{id}/deny", s.oauthDecide(false))
	r.Get("/policies/mcp", s.getMCPPolicy)
	r.Put("/policies/mcp", s.putMCPPolicy)
	r.Get("/policies/llm", s.getLLMPolicy)
	r.Put("/policies/llm", s.putLLMPolicy)
	r.Put("/policies/{key}", s.putPolicy)
	r.Get("/policies/{key}/history", s.policyHistory)
}

// publicBase is the public URL of the app (PUBLIC_URL, else the request's scheme and host).
func (s *Server) publicBase(r *http.Request) string {
	if b := strings.TrimRight(s.cfg.PublicURL, "/"); b != "" {
		return b
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func ceo(r *http.Request) (User, bool) {
	u, _ := CurrentUser(r.Context())
	return u, deref(u.Role) == "ceo"
}

// mcpInfo: endpoint, tools and the LLM path (Pengaturan → Koneksi AI).
func (s *Server) mcpInfo(w http.ResponseWriter, r *http.Request) {
	endpoint := s.publicBase(r)
	var tools []mcp.ToolInfo
	if s.mcp != nil {
		tools = s.mcp.Tools()
	}
	pol, _ := policy.Load(r.Context(), s.st.Q)
	httpx.JSON(w, http.StatusOK, map[string]any{"endpoint": endpoint + "/mcp", "enabled": s.mcp != nil, "tools": nonNil(tools), "llm": map[string]any{
		"mode": pol.LLM.Mode, "provider": s.cfg.LLMProvider, "model": pol.LLM.Model, "api_key": s.cfg.AnthropicKey != "", "fallback": pol.LLM.Fallback}})
}

type mcpClientView struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Scopes     []string  `json:"scopes"`
	Prefix     string    `json:"token_prefix"`
	Active     bool      `json:"active"`
	LastSeenAt any       `json:"last_seen_at"`
	CreatedAt  any       `json:"created_at"`
	CallsToday int64     `json:"calls_today"`
	Kind       string    `json:"kind"` // bearer (manual token) | oauth (Claude connector)
	User       string    `json:"user,omitempty"`
	RefreshAt  any       `json:"refresh_expires_at,omitempty"`
}

func (s *Server) mcpClients(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListMCPClients(r.Context(), clock.Today(s.clock.Now()))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]mcpClientView, 0, len(rows))
	for _, c := range rows {
		out = append(out, mcpClientView{ID: c.ID, Name: deref(c.Name), Scopes: c.Scopes, Prefix: deref(c.TokenPrefix), Active: c.Active, LastSeenAt: c.LastSeenAt, CreatedAt: c.CreatedAt, CallsToday: c.CallsToday,
			Kind: deref(c.Kind), User: deref(c.UserName), RefreshAt: c.RefreshExpiresAt})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

// createMCPClient creates a token (CEO only); the token is returned once and never stored in clear.
func (s *Server) createMCPClient(w http.ResponseWriter, r *http.Request) {
	u, ok := ceo(r)
	if !ok {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang dapat membuat token MCP")
		return
	}
	var body struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	token, c, err := mcp.CreateToken(r.Context(), s.st.Q, body.Name, body.Scopes, u.SalesUserID)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	s.auditUser(r, "mcp.token.create", "mcp_client:"+c.ID.String(), map[string]any{"name": body.Name, "scopes": body.Scopes})
	httpx.JSON(w, http.StatusCreated, map[string]any{"token": token, "client": mcpClientView{ID: c.ID, Name: deref(c.Name), Scopes: c.Scopes, Prefix: deref(c.TokenPrefix), Active: true, CreatedAt: c.CreatedAt}})
}

func (s *Server) revokeMCPClient(w http.ResponseWriter, r *http.Request) {
	if _, ok := ceo(r); !ok {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang dapat mencabut token MCP")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Klien tidak ditemukan")
		return
	}
	if err := s.st.Q.RevokeMCPClient(r.Context(), id); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if s.mcp != nil {
		s.mcp.Verifier().Forget()
	}
	s.auditUser(r, "mcp.token.revoke", "mcp_client:"+id.String(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mcpCalls(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 100 {
		n = 20
	}
	rows, err := s.st.Q.ListMCPCalls(r.Context(), int32(n))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

func (s *Server) getMCPPolicy(w http.ResponseWriter, r *http.Request) {
	pol, err := policy.Load(r.Context(), s.st.Q)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	m := pol.MCP
	m.AllowSend = false
	httpx.JSON(w, http.StatusOK, m)
}

// putMCPPolicy edits mcp.permissions (CEO). allow_send is locked to false in code: true → 400.
func (s *Server) putMCPPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := ceo(r); !ok {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang dapat mengubah izin MCP")
		return
	}
	var m domain.MCPPermissions
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format kebijakan tidak valid")
		return
	}
	if m.AllowSend {
		httpx.Fail(w, http.StatusBadRequest, "locked", "allow_send terkunci: klien MCP tidak pernah mengirim ke dealer")
		return
	}
	if m.MaxCyclesPerHour <= 0 || m.MaxCyclesPerHour > 30 {
		m.MaxCyclesPerHour = 6
	}
	if err := s.writePolicy(r, "mcp.permissions", m); err != nil {
		policyError(w, err)
		return
	}
	s.getMCPPolicy(w, r)
}

func (s *Server) getLLMPolicy(w http.ResponseWriter, r *http.Request) {
	pol, err := policy.Load(r.Context(), s.st.Q)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, pol.LLM)
}

// putLLMPolicy switches the analysis path (api | mcp | both) — CEO only; model and fallback are kept.
func (s *Server) putLLMPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := ceo(r); !ok {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang dapat mengubah jalur analisis")
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Mode != "api" && body.Mode != "mcp" && body.Mode != "both") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "mode: api | mcp | both")
		return
	}
	pol, err := policy.Load(r.Context(), s.st.Q)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	l := pol.LLM
	l.Mode = body.Mode
	if err := s.writePolicy(r, "llm.routing", l); err != nil {
		policyError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, l)
}

func (s *Server) writePolicy(r *http.Request, key string, v any) error {
	u, _ := CurrentUser(r.Context())
	b, _ := json.Marshal(v)
	_, err := policy.Save(r.Context(), s.st, key, b, u.SalesUserID, deref(u.Email), time.Now())
	return err
}

func (s *Server) auditUser(r *http.Request, action, entity string, after any) {
	u, _ := CurrentUser(r.Context())
	b, _ := json.Marshal(after)
	actor, kind := deref(u.Email), "user"
	_ = s.st.Q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, After: b})
}
