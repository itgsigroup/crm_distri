// Package oauth is the OAuth 2.1 authorization server of the MCP endpoint (ADR 0021), so Claude (claude.ai,
// Claude Desktop, Claude Code) can connect as a custom connector: protected-resource and authorization-server
// metadata (RFC 9728, RFC 8414), dynamic client registration (RFC 7591, public clients only), the authorization
// code flow with PKCE S256 (mandatory) and rotating refresh tokens. A person approves every connection on the
// consent page (/claude/izin, served by the web app); this package only issues codes it was given and tokens.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/mcp"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Lifetimes of the flow's short-lived records.
const (
	RequestTTL = 10 * time.Minute // a person has this long to approve
	CodeTTL    = 5 * time.Minute
	// RegistrationsPerHour limits dynamic client registration (it is open by design).
	RegistrationsPerHour = 100
)

// DefaultScopes are granted when a client asks for none.
var DefaultScopes = []string{mcp.ScopeRead, mcp.ScopeAnalyze}

// ConsentPath is the web page where a person approves a request (?req=<id>).
const ConsentPath = "/claude/izin"

// Server serves the OAuth endpoints.
type Server struct {
	St   *store.Store
	Base func(r *http.Request) string // public base URL, e.g. https://crm-distri.gsiindo.id
	Log  *slog.Logger
}

// Routes registers the endpoints at the root (they are public).
func (s *Server) Routes(r chi.Router) {
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		r.Get(p, s.protectedResource)
		r.Options(p, s.preflight)
	}
	r.Get("/.well-known/oauth-authorization-server", s.authServer)
	r.Options("/.well-known/oauth-authorization-server", s.preflight)
	r.Post("/oauth/register", s.register)
	r.Options("/oauth/register", s.preflight)
	r.Get("/oauth/authorize", s.authorize)
	r.Post("/oauth/token", s.token)
	r.Options("/oauth/token", s.preflight)
}

func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, mcp-protocol-version")
}

func (s *Server) preflight(w http.ResponseWriter, _ *http.Request) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	cors(w)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, code int, kind, desc string) {
	writeJSON(w, code, map[string]string{"error": kind, "error_description": desc})
}

// Resource is the MCP endpoint the tokens are for.
func (s *Server) Resource(r *http.Request) string { return s.Base(r) + "/mcp" }

func (s *Server) protectedResource(w http.ResponseWriter, r *http.Request) {
	base := s.Base(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource": base + "/mcp", "authorization_servers": []string{base}, "scopes_supported": mcp.ValidScopes,
		"bearer_methods_supported": []string{"header"}, "resource_name": "Distri ARC Orbit",
	})
}

func (s *Server) authServer(w http.ResponseWriter, r *http.Request) {
	base := s.Base(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token",
		"registration_endpoint": base + "/oauth/register", "scopes_supported": mcp.ValidScopes,
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"service_documentation": base + "/claude",
	})
}

// ValidRedirect accepts https URLs and loopback http URLs (desktop and CLI clients), never fragments.
func ValidRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Hash is how codes are stored.
func Hash(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "body JSON tidak valid")
		return
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 10 {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris wajib (1–10)")
		return
	}
	for _, u := range in.RedirectURIs {
		if !ValidRedirect(u) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri harus https atau http://localhost: "+u)
			return
		}
	}
	if in.AuthMethod != "" && in.AuthMethod != "none" {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "hanya klien publik (token_endpoint_auth_method none) dengan PKCE")
		return
	}
	if n, err := s.St.Q.CountOAuthClientsSince(r.Context(), time.Now().Add(-time.Hour)); err == nil && n >= RegistrationsPerHour {
		oauthError(w, http.StatusTooManyRequests, "temporarily_unavailable", "terlalu banyak pendaftaran klien, coba lagi nanti")
		return
	}
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		name = "Klien MCP"
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	c, err := s.St.Q.InsertOAuthClient(r.Context(), gen.InsertOAuthClientParams{ClientID: "arcc_" + random(16), ClientName: name, RedirectUris: in.RedirectURIs})
	if err != nil {
		oauthError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": c.ClientID, "client_id_issued_at": c.CreatedAt.Unix(), "client_name": c.ClientName, "redirect_uris": c.RedirectUris,
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
}

// RedirectWith adds query parameters to a client's redirect URI.
func RedirectWith(redirect string, params map[string]string) string {
	u, err := url.Parse(redirect)
	if err != nil {
		return redirect
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// ParseScopes keeps the known scopes of a space-separated list ("" → DefaultScopes).
func ParseScopes(raw string) []string {
	var out []string
	for _, f := range strings.Fields(raw) {
		if slices.Contains(mcp.ValidScopes, f) && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return slices.Clone(DefaultScopes)
	}
	return out
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx := r.Context()
	_ = s.St.Q.PurgeOAuth(ctx)
	client, err := s.St.Q.GetOAuthClient(ctx, q.Get("client_id"))
	redirect := q.Get("redirect_uri")
	if err != nil || !slices.Contains(client.RedirectUris, redirect) {
		// never redirect to an unverified URI: tell the person instead
		http.Error(w, "Permintaan koneksi tidak valid: klien atau redirect_uri tidak terdaftar. Hubungkan ulang dari Claude.", http.StatusBadRequest)
		return
	}
	fail := func(kind, desc string) {
		http.Redirect(w, r, RedirectWith(redirect, map[string]string{"error": kind, "error_description": desc, "state": q.Get("state")}), http.StatusFound)
	}
	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "hanya response_type=code")
		return
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		fail("invalid_request", "PKCE S256 wajib")
		return
	}
	if res := q.Get("resource"); res != "" && strings.TrimRight(res, "/") != s.Resource(r) && strings.TrimRight(res, "/") != s.Base(r) {
		fail("invalid_target", "resource harus "+s.Resource(r))
		return
	}
	id := random(24)
	if err := s.St.Q.InsertOAuthRequest(ctx, gen.InsertOAuthRequestParams{ID: id, ClientID: client.ClientID, RedirectUri: redirect, State: q.Get("state"),
		CodeChallenge: q.Get("code_challenge"), Scopes: ParseScopes(q.Get("scope")), Resource: q.Get("resource"), ExpiresAt: time.Now().Add(RequestTTL)}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, s.Base(r)+ConsentPath+"?req="+url.QueryEscape(id), http.StatusFound)
}

// IssueCode turns an approved request into a one-time code and returns the client's redirect URL.
func IssueCode(ctx context.Context, q *gen.Queries, req gen.GetOAuthRequestRow, user gen.GetUserByIDRow, scopes []string) (string, error) {
	code := random(32)
	if err := q.InsertOAuthCode(ctx, gen.InsertOAuthCodeParams{CodeHash: Hash(code), ClientID: req.ClientID, UserID: user.ID, RedirectUri: req.RedirectUri,
		CodeChallenge: req.CodeChallenge, Scopes: scopes, ExpiresAt: time.Now().Add(CodeTTL)}); err != nil {
		return "", err
	}
	if err := q.DeleteOAuthRequest(ctx, req.ID); err != nil {
		return "", err
	}
	return RedirectWith(req.RedirectUri, map[string]string{"code": code, "state": req.State}), nil
}

// VerifyPKCE checks an S256 code verifier against its challenge.
func VerifyPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:]) == challenge
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "form tidak valid")
		return
	}
	f := r.PostForm
	ctx := r.Context()
	var (
		g   mcp.Grant
		err error
	)
	switch f.Get("grant_type") {
	case "authorization_code":
		g, err = s.exchange(ctx, f)
	case "refresh_token":
		g, err = s.refresh(ctx, f)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "authorization_code atau refresh_token")
		return
	}
	if err != nil {
		var ge grantError
		if errors.As(err, &ge) {
			oauthError(w, http.StatusBadRequest, string(ge), err.Error())
			return
		}
		s.Log.Error("oauth token", "err", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "gagal menerbitkan token")
		return
	}
	_ = s.St.Q.TouchOAuthClient(ctx, f.Get("client_id"))
	writeJSON(w, http.StatusOK, map[string]any{"access_token": g.Access, "token_type": "Bearer", "expires_in": int(time.Until(g.Expires).Seconds()),
		"refresh_token": g.Refresh, "scope": strings.Join(g.Client.Scopes, " ")})
}

type grantError string

func (e grantError) Error() string { return string(e) }

func (s *Server) exchange(ctx context.Context, f url.Values) (mcp.Grant, error) {
	code, err := s.St.Q.UseOAuthCode(ctx, Hash(f.Get("code")))
	if errors.Is(err, pgx.ErrNoRows) {
		return mcp.Grant{}, grantError("invalid_grant")
	}
	if err != nil {
		return mcp.Grant{}, err
	}
	if code.ClientID != f.Get("client_id") || code.RedirectUri != f.Get("redirect_uri") || !VerifyPKCE(f.Get("code_verifier"), code.CodeChallenge) {
		return mcp.Grant{}, grantError("invalid_grant")
	}
	user, err := s.St.Q.GetUserByID(ctx, code.UserID)
	if err != nil || !user.Active {
		return mcp.Grant{}, grantError("invalid_grant")
	}
	client, err := s.St.Q.GetOAuthClient(ctx, code.ClientID)
	if err != nil {
		return mcp.Grant{}, grantError("invalid_client")
	}
	name := client.ClientName + " · " + deref(user.Name)
	return mcp.IssueOAuth(ctx, s.St.Q, name, code.Scopes, user.SalesUserID, &user.ID, client.ClientID, time.Now())
}

func (s *Server) refresh(ctx context.Context, f url.Values) (mcp.Grant, error) {
	rh := mcp.RefreshHash(f.Get("refresh_token"))
	c, err := s.St.Q.GetMCPClientByRefresh(ctx, &rh)
	if errors.Is(err, pgx.ErrNoRows) {
		return mcp.Grant{}, grantError("invalid_grant")
	}
	if err != nil {
		return mcp.Grant{}, err
	}
	if c.OauthClientID == nil || *c.OauthClientID != f.Get("client_id") {
		return mcp.Grant{}, grantError("invalid_grant")
	}
	if c.UserID != nil {
		if u, err := s.St.Q.GetUserByID(ctx, *c.UserID); err != nil || !u.Active {
			return mcp.Grant{}, grantError("invalid_grant")
		}
	}
	return mcp.Rotate(ctx, s.St.Q, c, time.Now())
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
