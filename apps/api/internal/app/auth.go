package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/storage"
)

// Principal is the authenticated caller.
type Principal struct {
	UserID string
	Name   string
	Role   string
	Branch string
	Kind   string // session | apikey | oauth
	KeyID  string
	Scopes map[string]bool
}

// Human reports whether the caller is a person in a browser session. Only humans decide.
func (p Principal) Human() bool { return p.Kind == "session" }

// Can checks a scope (sessions hold every scope).
func (p Principal) Can(scope string) bool {
	if p.Kind == "session" {
		return true
	}
	if scope == domain.ScopeHuman {
		return false
	}
	return p.Scopes[scope]
}

// Scope maps the principal to data access: CEO/manager/finance see everything,
// others their branch plus accounts they own. Machine clients inherit their owner.
func (p Principal) Scope() insights.Scope {
	switch p.Role {
	case domain.RoleCEO, domain.RoleManager, domain.RoleFinance:
		return insights.Scope{All: true}
	}
	return insights.Scope{Branch: p.Branch, UserID: p.UserID}
}

// Actor converts to an audit actor.
func (p Principal) Actor() storage.Actor {
	if p.Human() {
		return storage.Actor{ID: p.UserID, Type: "user"}
	}
	return storage.Actor{ID: p.UserID + ":" + p.Kind + ":" + p.KeyID, Type: "machine"}
}

// RoleLabel renders "CEO · semua cabang".
func (p Principal) RoleLabel() string {
	switch p.Role {
	case domain.RoleCEO:
		return "CEO · semua cabang"
	case domain.RoleManager:
		return "Manager · semua cabang"
	case domain.RoleFinance:
		return "Finance · " + p.Branch
	case domain.RoleOps:
		return "Operasional · " + p.Branch
	}
	return "Sales · " + p.Branch
}

type ctxKey struct{}

func withPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// PrincipalFrom returns the caller (zero value when anonymous).
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

const sessionCookie = "arc_session"

func hashToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *App) loadUser(ctx context.Context, id string) (Principal, error) {
	p := Principal{UserID: id}
	err := a.DB.Pool.QueryRow(ctx, `SELECT name, role, branch FROM users WHERE id=$1`, id).Scan(&p.Name, &p.Role, &p.Branch)
	return p, err
}

// authenticate resolves the session cookie, API key (Bearer arc_live_…) or OAuth token.
func (a *App) authenticate(r *http.Request) (Principal, error) {
	ctx := r.Context()
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		var uid string
		if err := a.DB.Pool.QueryRow(ctx, `SELECT user_id FROM user_sessions WHERE token=$1 AND expires_at > now()`, hashToken(c.Value)).Scan(&uid); err == nil {
			p, err := a.loadUser(ctx, uid)
			p.Kind = "session"
			return p, err
		}
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return Principal{}, errors.New("unauthorized")
	}
	tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if strings.HasPrefix(tok, "arc_live_") {
		var id, owner string
		var scopes []string
		var rpm int
		err := a.DB.Pool.QueryRow(ctx, `SELECT id, owner_user_id, scopes, rate_limit_rpm FROM api_keys WHERE key_hash=$1 AND revoked_at IS NULL`, hashToken(tok)).Scan(&id, &owner, &scopes, &rpm)
		if err != nil {
			return Principal{}, errors.New("unauthorized")
		}
		if !a.limiter.allow(id, rpm) {
			return Principal{}, errRateLimited
		}
		p, err := a.loadUser(ctx, owner)
		if err != nil {
			return Principal{}, err
		}
		p.Kind, p.KeyID, p.Scopes = "apikey", id, map[string]bool{}
		for _, s := range scopes {
			if s != domain.ScopeHuman {
				p.Scopes[s] = true
			}
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE api_keys SET calls_count=calls_count+1, last_used_at=now() WHERE id=$1`, id)
		return p, nil
	}
	var uid, client, scope string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT user_id, client_id, scope FROM oauth_tokens WHERE token_hash=$1 AND kind='access' AND NOT revoked AND expires_at > now()`, hashToken(tok)).Scan(&uid, &client, &scope); err == nil {
		if !a.limiter.allow("oauth:"+client, 120) {
			return Principal{}, errRateLimited
		}
		p, err := a.loadUser(ctx, uid)
		if err != nil {
			return Principal{}, err
		}
		p.Kind, p.KeyID, p.Scopes = "oauth", client, map[string]bool{domain.ScopeRead: true, domain.ScopePropose: true}
		return p, nil
	}
	return Principal{}, errors.New("unauthorized")
}

var errRateLimited = errors.New("rate limit")

// requireAuth wraps a handler; scope "" means any authenticated caller.
func (a *App) requireAuth(scope string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := a.authenticate(r)
		if errors.Is(err, errRateLimited) {
			writeErr(w, http.StatusTooManyRequests, "batas panggilan per menit terlampaui")
			return
		}
		if err != nil {
			if strings.HasPrefix(r.URL.Path, "/mcp") {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+a.Cfg.PublicURL+`/.well-known/oauth-protected-resource"`)
			}
			writeErr(w, http.StatusUnauthorized, "perlu login")
			return
		}
		if scope != "" && !p.Can(scope) {
			writeErr(w, http.StatusForbidden, "scope "+scope+" diperlukan")
			return
		}
		if !p.Human() && r.Method != http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			writeErr(w, http.StatusForbidden, "endpoint UI hanya untuk sesi pengguna")
			return
		}
		if p.Human() && r.Method != http.MethodGet && r.Method != http.MethodHead {
			// CSRF mitigation for cookie sessions: SameSite=Lax + JSON/text bodies only.
			ct := r.Header.Get("Content-Type")
			if r.ContentLength > 0 && !strings.HasPrefix(ct, "application/json") && !strings.HasPrefix(ct, "text/") && !strings.HasPrefix(ct, "multipart/") && !strings.HasPrefix(ct, "application/octet-stream") {
				writeErr(w, http.StatusUnsupportedMediaType, "content-type tidak didukung")
				return
			}
		}
		h(w, r.WithContext(withPrincipal(r.Context(), p)))
	}
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "format tidak valid")
		return
	}
	if !a.limiter.allow("login:"+clientIP(r), 10) {
		writeErr(w, http.StatusTooManyRequests, "terlalu banyak percobaan, coba lagi sebentar")
		return
	}
	var id, hash string
	err := a.DB.Pool.QueryRow(r.Context(), `SELECT id, password_hash FROM users WHERE lower(email)=lower($1)`, strings.TrimSpace(req.Email)).Scan(&id, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "email atau kata sandi salah")
		return
	}
	tok := randomToken(32)
	if _, err := a.DB.Pool.Exec(r.Context(), `INSERT INTO user_sessions(token,user_id,expires_at) VALUES ($1,$2,$3)`, hashToken(tok), id, time.Now().Add(14*24*time.Hour)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: strings.HasPrefix(a.Cfg.PublicURL, "https://"), MaxAge: 14 * 24 * 3600})
	p, _ := a.loadUser(r.Context(), id)
	p.Kind = "session"
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "login", "user", id, nil)
	writeJSON(w, http.StatusOK, meView(p))
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_, _ = a.DB.Pool.Exec(r.Context(), `DELETE FROM user_sessions WHERE token=$1`, hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"toast": "Keluar"})
}

func meView(p Principal) map[string]any {
	initials := domain.Initials(p.Name)
	if p.UserID == "sam" {
		initials = "SR"
	}
	return map[string]any{"id": p.UserID, "name": p.Name, "role": p.Role, "branch": p.Branch, "initials": initials, "role_label": p.RoleLabel()}
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

// limiter is a per-key token bucket (requests per minute).
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter() *limiter { return &limiter{buckets: map[string]*bucket{}} }

func (l *limiter) allow(key string, rpm int) bool {
	if rpm <= 0 {
		rpm = 60
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	now := time.Now()
	if !ok {
		b = &bucket{tokens: float64(rpm), last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Minutes() * float64(rpm)
	if b.tokens > float64(rpm) {
		b.tokens = float64(rpm)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	return dec.Decode(v)
}

func toast(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusOK, map[string]string{"toast": msg})
}
