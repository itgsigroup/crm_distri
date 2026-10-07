package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/mcp"
	"distri-arc/internal/oauth"
)

// mcpGrantable is what a user may give Claude: the MCP Claude page and all data (Claude reads every dealer);
// orchestrate (running cycles) only with policy rights.
func mcpGrantable(u User) ([]string, string) {
	ro := roleOf(u)
	if !slices.Contains(ro.Screens, "mcp") {
		return nil, "Peran Anda tidak membuka menu MCP Claude — minta pemegang hak kebijakan mengaturnya di Peran & akses"
	}
	if ro.Scope != "all" {
		return nil, "Claude membaca semua dealer; peran dengan cakupan \"hanya data miliknya\" tidak bisa menyambungkannya"
	}
	if deref(u.Role) == "ceo" {
		return mcp.ValidScopes, ""
	}
	return []string{mcp.ScopeRead, mcp.ScopeAnalyze}, ""
}

var scopeLabel = map[string]string{
	mcp.ScopeRead:        "Membaca dealer, jadwal order, kredit, stok, chat (nomor disamarkan), KPI",
	mcp.ScopeAnalyze:     "Menjalankan analisis (dealer, segmen, kas, stok) — hasilnya usulan, bukan keputusan",
	mcp.ScopeOrchestrate: "Menjalankan siklus Orchestrator dan mengirim usulan agen",
}

// oauthRequest shows a pending connection request on the consent page.
func (s *Server) oauthRequest(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	req, err := s.st.Q.GetOAuthRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "expired", "Permintaan koneksi tidak ada atau sudah kedaluwarsa — hubungkan ulang dari Claude")
		return
	}
	grantable, reason := mcpGrantable(u)
	host := req.RedirectUri
	if p, err := url.Parse(req.RedirectUri); err == nil {
		host = p.Host
	}
	scopes := []map[string]any{}
	for _, sc := range mcp.ValidScopes {
		scopes = append(scopes, map[string]any{"key": sc, "label": scopeLabel[sc], "requested": slices.Contains(req.Scopes, sc), "allowed": slices.Contains(grantable, sc)})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": req.ID, "client_name": req.ClientName, "redirect_host": host, "scopes": scopes,
		"allowed": reason == "", "reason": reason, "user": map[string]any{"name": u.Name, "email": u.Email, "role": roleOf(u).Name}, "expires_at": req.ExpiresAt})
}

// oauthDecide approves (one-time code) or denies a request and returns where the browser goes back to Claude.
func (s *Server) oauthDecide(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := CurrentUser(r.Context())
		ctx := r.Context()
		req, err := s.st.Q.GetOAuthRequest(ctx, chi.URLParam(r, "id"))
		if err != nil {
			httpx.Fail(w, http.StatusNotFound, "expired", "Permintaan koneksi tidak ada atau sudah kedaluwarsa — hubungkan ulang dari Claude")
			return
		}
		if !approve {
			_ = s.st.Q.DeleteOAuthRequest(ctx, req.ID)
			s.auditUser(r, "mcp.oauth_denied", "oauth_client:"+req.ClientID, map[string]any{"client": req.ClientName})
			httpx.JSON(w, http.StatusOK, map[string]any{"redirect": oauth.RedirectWith(req.RedirectUri, map[string]string{"error": "access_denied", "state": req.State})})
			return
		}
		grantable, reason := mcpGrantable(u)
		if reason != "" {
			httpx.Fail(w, http.StatusForbidden, "forbidden", reason)
			return
		}
		var in struct {
			Scopes []string `json:"scopes"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in)
		if len(in.Scopes) == 0 {
			in.Scopes = req.Scopes
		}
		var scopes []string
		for _, sc := range mcp.ValidScopes { // what was chosen, within what this person may give
			if slices.Contains(in.Scopes, sc) && slices.Contains(grantable, sc) {
				scopes = append(scopes, sc)
			}
		}
		if !slices.Contains(scopes, mcp.ScopeRead) {
			scopes = append([]string{mcp.ScopeRead}, scopes...)
		}
		user, err := s.st.Q.GetUserByID(ctx, u.ID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		redirect, err := oauth.IssueCode(ctx, s.st.Q, req, user, scopes)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		s.auditUser(r, "mcp.oauth_approved", "oauth_client:"+req.ClientID, map[string]any{"client": req.ClientName, "scopes": scopes})
		httpx.JSON(w, http.StatusOK, map[string]any{"redirect": redirect})
	}
}
