package api

import (
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/auth"
	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// loginLimiter: 10 failed logins per IP and email per 15 minutes.
var loginLimiter = auth.NewLimiter(10, 15*time.Minute)

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", MaxAge: maxAge})
}

// login checks email + password and sets the session cookie.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi email dan kata sandi")
		return
	}
	if s.secret() == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "no_session_secret", "SESSION_SECRET belum diatur di server")
		return
	}
	key := clientIP(r) + "|" + strings.ToLower(body.Email)
	now := time.Now()
	if loginLimiter.Blocked(key, now) {
		httpx.Fail(w, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak percobaan — coba lagi 15 menit lagi")
		return
	}
	u, err := s.st.Q.GetLoginUser(r.Context(), body.Email)
	if err != nil || !u.Active || u.PasswordHash == nil || !auth.Check(body.Password, *u.PasswordHash) {
		loginLimiter.Fail(key, now)
		httpx.Fail(w, http.StatusUnauthorized, "bad_credentials", "Email atau kata sandi salah")
		return
	}
	loginLimiter.Reset(key)
	tok := auth.Sign(auth.Claims{Sub: u.ID.String(), Email: deref(u.Email), Role: deref(u.Role), Iat: now.Unix(), Exp: now.Add(sessionTTL).Unix()}, s.secret())
	s.setSession(w, r, tok, int(sessionTTL.Seconds()))
	actor, kind, action, entity := deref(u.Email), "user", "auth.login", "user"
	_ = s.st.Q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &u.ID})
	httpx.JSON(w, http.StatusOK, map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.setSession(w, r, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) userRoutes(r chi.Router) {
	r.Get("/users", s.listUsers)
	r.Post("/users", s.createUser)
	r.Put("/users/{id}", s.updateUser)
}

// isAdmin: CEO and admin manage users.
func isAdmin(u User) bool { r := deref(u.Role); return r == "ceo" || r == "admin" }

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO dan admin yang melihat pengguna")
		return
	}
	rows, err := s.st.Q.ListUsers(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows), "roles": auth.Roles})
}

type userBody struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
	Sales    string `json:"sales"` // sales_users name the account belongs to (role sales)
	Active   *bool  `json:"active"`
}

// createUser (CEO/admin): only the CEO creates another CEO.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	var b userBody
	if !isAdmin(me) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO dan admin yang menambah pengguna")
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || !strings.Contains(b.Email, "@") || b.Name == "" || !slices.Contains(auth.Roles, b.Role) || len(b.Password) < 10 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi email, nama, peran (ceo/admin/finance/sales/warehouse) dan kata sandi ≥ 10 karakter")
		return
	}
	if b.Role == "ceo" && deref(me.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang menambah CEO")
		return
	}
	hash, err := auth.Hash(b.Password)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var sales *uuid.UUID
	if b.Sales != "" {
		if id, err := s.st.Q.SalesUserByName(r.Context(), b.Sales); err == nil {
			sales = &id
		}
	}
	id, err := s.st.Q.CreateUser(r.Context(), gen.CreateUserParams{Email: &b.Email, Name: &b.Name, Role: &b.Role, PasswordHash: &hash, SalesUserID: sales})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Email sudah dipakai")
		return
	}
	s.auditUser(r, "user.create", "user:"+id.String(), map[string]any{"email": b.Email, "role": b.Role})
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// updateUser changes role, active flag, name or password (CEO/admin; CEO role changes only by the CEO).
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	if !isAdmin(me) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO dan admin yang mengubah pengguna")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pengguna tidak ditemukan")
		return
	}
	var b userBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || (b.Role != "" && !slices.Contains(auth.Roles, b.Role)) || (b.Password != "" && len(b.Password) < 10) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Peran tidak dikenal atau kata sandi < 10 karakter")
		return
	}
	if b.Role == "ceo" && deref(me.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang menetapkan CEO")
		return
	}
	if id == me.ID && b.Active != nil && !*b.Active {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tidak bisa menonaktifkan akun sendiri")
		return
	}
	p := gen.UpdateUserParams{ID: id, Active: b.Active}
	if b.Role != "" {
		p.Role = &b.Role
	}
	if b.Name != "" {
		p.Name = &b.Name
	}
	if b.Password != "" {
		h, err := auth.Hash(b.Password)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		p.PasswordHash = &h
	}
	if err := s.st.Q.UpdateUser(r.Context(), p); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "user.update", "user:"+id.String(), map[string]any{"role": b.Role, "active": b.Active, "password_reset": b.Password != ""})
	w.WriteHeader(http.StatusNoContent)
}
