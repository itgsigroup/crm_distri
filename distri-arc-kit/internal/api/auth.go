package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/auth"
	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
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
		Code     string `json:"code"` // 2FA TOTP, when the user turned it on
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
	if u.TotpEnabledAt != nil {
		if strings.TrimSpace(body.Code) == "" {
			httpx.Fail(w, http.StatusUnauthorized, "totp_required", "Masukkan kode 6 digit dari aplikasi authenticator")
			return
		}
		if !s.checkTOTP(r, u.ID, deref(u.TotpSecret), body.Code, now) {
			loginLimiter.Fail(key, now)
			httpx.Fail(w, http.StatusUnauthorized, "totp_invalid", "Kode 2FA salah atau sudah dipakai")
			return
		}
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
	r.Post("/auth/totp/setup", s.totpSetup)
	r.Post("/auth/totp/enable", s.totpEnable)
	r.Post("/auth/totp/disable", s.totpDisable)
	r.Get("/users", s.listUsers)
	r.Post("/users", s.createUser)
	r.Put("/users/{id}", s.updateUser)
}

// isAdmin: the role manages the team or the settings (pages Pengguna or Pengaturan).
func isAdmin(u User) bool { return can(u, "users") || can(u, "conn") }

// can reports whether the user's role opens a page (role master).
func can(u User, screen string) bool { return slices.Contains(roleOf(u).Screens, screen) }

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListUsers(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

type userBody struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	RoleKey  string `json:"role_key"` // a role of the role master
	Role     string `json:"role"`     // older clients
	Password string `json:"password"`
	Sales    string `json:"sales"`  // sales_users name the account belongs to (imported sales)
	Branch   string `json:"branch"` // a branch of the branch master
	Active   *bool  `json:"active"`
}

// userRole resolves the requested role; a role with policy rights is given only by a policy holder.
func (s *Server) userRole(w http.ResponseWriter, r *http.Request, me User, b userBody) (gen.Role, bool) {
	key := b.RoleKey
	if key == "" {
		key = b.Role
	}
	ro, err := s.st.Q.GetRole(r.Context(), key)
	if err != nil || !ro.Active {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih peran dari master Peran & akses")
		return ro, false
	}
	if ro.Policies && deref(me.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Peran dengan hak kebijakan hanya diberikan oleh pemegang hak kebijakan (CEO)")
		return ro, false
	}
	return ro, true
}

// userBranch checks a branch against the branch master ("" = all branches).
func (s *Server) userBranch(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Semua cabang", true
	}
	b, err := s.st.Q.BranchByName(r.Context(), name)
	if err != nil || !b.Active {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih cabang dari master Cabang")
		return "", false
	}
	return b.Name, true
}

// createUser adds a login account (page Pengguna). Its WhatsApp number is not typed here: it is linked in Chat
// (scan, then pick the user).
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	var b userBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || !strings.Contains(b.Email, "@") || strings.TrimSpace(b.Name) == "" || len(b.Password) < 10 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi email, nama, peran dan kata sandi ≥ 10 karakter")
		return
	}
	ro, ok := s.userRole(w, r, me, b)
	if !ok {
		return
	}
	branch, ok := s.userBranch(w, r, b.Branch)
	if !ok {
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
	if sales == nil { // every account needs a person profile to record its decisions
		id, err := s.st.Q.CreatePersonProfile(r.Context(), gen.CreatePersonProfileParams{Name: strings.TrimSpace(b.Name), Branch: branch, Role: ro.Base, Email: &b.Email})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		sales = &id
	}
	name := strings.TrimSpace(b.Name)
	id, err := s.st.Q.CreateUser(r.Context(), gen.CreateUserParams{Email: &b.Email, Name: &name, Role: &ro.Base, PasswordHash: &hash, SalesUserID: sales, RoleKey: &ro.Key})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Email sudah dipakai")
		return
	}
	s.auditUser(r, "user.create", "user:"+id.String(), map[string]any{"email": b.Email, "role": ro.Key, "branch": branch})
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// updateUser changes role, branch, active flag, name or password. There is always one active policy holder; an
// account holding a WhatsApp number cannot move to a role without WhatsApp before the number is released in Chat.
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pengguna tidak ditemukan")
		return
	}
	cur, err := s.st.Q.GetUserByID(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pengguna tidak ditemukan")
		return
	}
	var b userBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || (b.Password != "" && len(b.Password) < 10) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid atau kata sandi < 10 karakter")
		return
	}
	if deref(cur.Role) == "ceo" && deref(me.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Akun dengan hak kebijakan hanya diubah oleh pemegang hak kebijakan")
		return
	}
	if id == me.ID && b.Active != nil && !*b.Active {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tidak bisa menonaktifkan akun sendiri")
		return
	}
	var ro *gen.Role
	if b.RoleKey != "" || b.Role != "" {
		x, ok := s.userRole(w, r, me, b)
		if !ok {
			return
		}
		ro = &x
	}
	losesPolicies := deref(cur.Role) == "ceo" && ((ro != nil && !ro.Policies) || (b.Active != nil && !*b.Active))
	if losesPolicies {
		if n, _ := s.st.Q.PolicyHoldersExcept(r.Context(), gen.PolicyHoldersExceptParams{RoleKey: "", UserID: id}); n == 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Harus tetap ada satu pengguna aktif dengan hak kebijakan (setara CEO)")
			return
		}
	}
	if ro != nil && !ro.WaAllowed {
		if held, err := s.st.Q.GetWANumberByUser(r.Context(), &id); err == nil {
			httpx.Fail(w, http.StatusConflict, "wa_linked", "Pengguna ini memegang "+wa.MaskNumber(held.WaNumber)+" — lepas nomornya di Chat sebelum pindah ke peran tanpa WhatsApp")
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	branch := ""
	if strings.TrimSpace(b.Branch) != "" {
		var ok bool
		if branch, ok = s.userBranch(w, r, b.Branch); !ok {
			return
		}
	}
	p := gen.UpdateUserParams{ID: id, Active: b.Active}
	if n := strings.TrimSpace(b.Name); n != "" {
		p.Name = &n
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
	if p.Name != nil && *p.Name != deref(cur.Name) { // a new name shows everywhere the person appears
		if err := s.renamePerson(r.Context(), id, cur.SalesUserID, *p.Name); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	if ro != nil {
		if err := s.st.Q.SetUserProfile(r.Context(), gen.SetUserProfileParams{ID: id, RoleKey: &ro.Key, Role: &ro.Base}); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	if cur.SalesUserID != nil && (branch != "" || ro != nil) {
		sp := gen.SetSalesProfileWAParams{ID: *cur.SalesUserID, WaNumber: deref(cur.WaNumber)}
		if branch != "" {
			sp.Branch = &branch
		}
		if ro != nil {
			sp.Role = &ro.Base
		}
		if err := s.st.Q.SetSalesProfileWA(r.Context(), sp); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	roleKey := ""
	if ro != nil {
		roleKey = ro.Key
	}
	s.auditUser(r, "user.update", "user:"+id.String(), map[string]any{"role": roleKey, "branch": branch, "active": b.Active, "password_reset": b.Password != "", "name": p.Name})
	w.WriteHeader(http.StatusNoContent)
}

// checkTOTP verifies a code against a sealed secret and burns its time step (no replay).
func (s *Server) checkTOTP(r *http.Request, id uuid.UUID, sealed, code string, now time.Time) bool {
	secret, err := auth.Open(sealed, s.secret())
	if err != nil {
		return false
	}
	step, ok := auth.VerifyTOTP(secret, code, now)
	if !ok {
		return false
	}
	n, err := s.st.Q.UseTOTPStep(r.Context(), gen.UseTOTPStepParams{ID: id, TotpLastStep: &step})
	return err == nil && n == 1
}

// totpRoles may turn on 2FA (09-policies-security: policy holders and administrators; everyone with all data).
func totpRoles(role string) bool { return role == "ceo" || role == "admin" || role == "finance" }

// totpSetup creates a new secret (not active until the first code is confirmed) and returns it once.
func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !totpRoles(deref(u.Role)) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "2FA tersedia untuk CEO dan admin")
		return
	}
	if u.TotpEnabledAt != nil {
		httpx.Fail(w, http.StatusConflict, "totp_on", "2FA sudah aktif — matikan dulu untuk mengganti perangkat")
		return
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "totp", err.Error())
		return
	}
	sealed, err := auth.Seal(secret, s.secret())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "totp", err.Error())
		return
	}
	if err := s.st.Q.SetTOTPSecret(r.Context(), gen.SetTOTPSecretParams{ID: u.ID, TotpSecret: &sealed}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "totp", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": auth.TOTPURI("GSI Orbit", deref(u.Email), secret)})
}

func (s *Server) totpCode(w http.ResponseWriter, r *http.Request) (User, gen.GetUserTOTPRow, string, bool) {
	u, _ := CurrentUser(r.Context())
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body)
	t, err := s.st.Q.GetUserTOTP(r.Context(), u.ID)
	if err != nil || t.TotpSecret == nil {
		httpx.Fail(w, http.StatusBadRequest, "totp_missing", "Mulai dari Aktifkan 2FA")
		return u, t, "", false
	}
	return u, t, body.Code, true
}

// totpEnable confirms the first code and turns 2FA on.
func (s *Server) totpEnable(w http.ResponseWriter, r *http.Request) {
	u, t, code, ok := s.totpCode(w, r)
	if !ok {
		return
	}
	secret, err := auth.Open(*t.TotpSecret, s.secret())
	step, valid := auth.VerifyTOTP(secret, code, time.Now())
	if err != nil || !valid {
		httpx.Fail(w, http.StatusBadRequest, "totp_invalid", "Kode salah — cek jam ponsel dan coba kode berikutnya")
		return
	}
	now := time.Now()
	if err := s.st.Q.EnableTOTP(r.Context(), gen.EnableTOTPParams{ID: u.ID, TotpEnabledAt: &now, TotpLastStep: &step}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "totp", err.Error())
		return
	}
	s.auditUser(r, "auth.totp_enabled", "user", map[string]any{"user_id": u.ID})
	httpx.JSON(w, http.StatusOK, map[string]any{"totp_enabled": true, "message": "2FA aktif · login berikutnya meminta kode"})
}

// totpDisable turns 2FA off with a valid current code (lost phone: arc ctl user totp-reset).
func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request) {
	u, t, code, ok := s.totpCode(w, r)
	if !ok {
		return
	}
	if t.TotpEnabledAt != nil && !s.checkTOTP(r, u.ID, *t.TotpSecret, code, time.Now()) {
		httpx.Fail(w, http.StatusBadRequest, "totp_invalid", "Kode 2FA salah")
		return
	}
	if err := s.st.Q.DisableTOTP(r.Context(), u.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "totp", err.Error())
		return
	}
	s.auditUser(r, "auth.totp_disabled", "user", map[string]any{"user_id": u.ID})
	httpx.JSON(w, http.StatusOK, map[string]any{"totp_enabled": false, "message": "2FA dimatikan"})
}

// renamePerson carries a user's new name to their own sales profile (dealer owner, decisions, agenda) and to their
// Claude connections ("Claude · <name>"). A profile imported from BigQuery keeps the source's spelling (the import
// would write it back anyway; Mapping sales links it to the user).
func (s *Server) renamePerson(ctx context.Context, userID uuid.UUID, profile *uuid.UUID, name string) error {
	return s.st.Tx(ctx, func(_ *gen.Queries, tx pgx.Tx) error {
		if profile != nil {
			if _, err := tx.Exec(ctx, "update sales_users set name = $2 where id = $1 and source_system is distinct from 'import'", *profile, name); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `update mcp_clients set name = case when position(' · ' in name) > 0 then split_part(name, ' · ', 1) || ' · ' || $2 else name end
			where user_id = $1 and kind = 'oauth'`, userID, name)
		return err
	})
}
