package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
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
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

type userBody struct {
	Email    string  `json:"email"`
	Name     string  `json:"name"`
	RoleKey  string  `json:"role_key"` // a role of the role master
	Role     string  `json:"role"`     // older clients: a base role key
	Password string  `json:"password"`
	Sales    string  `json:"sales"` // sales_users name the account belongs to (imported sales)
	Branch   string  `json:"branch"`
	WANumber *string `json:"wa_number"` // one WhatsApp number per user; "" clears
	Active   *bool   `json:"active"`
}

// userRole resolves the requested role; the CEO base is assigned only by the CEO.
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
	if ro.Base == "ceo" && deref(me.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang menetapkan peran CEO")
		return ro, false
	}
	return ro, true
}

// userWA validates a user's WhatsApp number (Indonesian, 62…); "" clears it.
func userWA(w http.ResponseWriter, raw *string, ro gen.Role) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	n := wa.Digits(*raw)
	if n == "" {
		return &n, true
	}
	if !strings.HasPrefix(n, "62") || len(n) < 10 || len(n) > 15 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nomor WhatsApp harus nomor Indonesia, mis. 0812… atau +62812…")
		return nil, false
	}
	if !ro.WaAllowed && ro.Base != "ceo" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Peran "+ro.Name+" tidak memegang nomor WhatsApp — ubah di Peran & akses")
		return nil, false
	}
	return &n, true
}

func waTaken(w http.ResponseWriter, err error) bool {
	if err != nil && strings.Contains(err.Error(), "wa_number") {
		httpx.Fail(w, http.StatusConflict, "wa_taken", "Nomor WhatsApp ini sudah dipegang pengguna lain — satu nomor untuk satu pengguna")
		return true
	}
	return false
}

// createUser (CEO/admin): only the CEO creates another CEO.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	var b userBody
	if !isAdmin(me) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO dan admin yang menambah pengguna")
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || !strings.Contains(b.Email, "@") || b.Name == "" || len(b.Password) < 10 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi email, nama, peran dan kata sandi ≥ 10 karakter")
		return
	}
	ro, ok := s.userRole(w, r, me, b)
	if !ok {
		return
	}
	waNo, ok := userWA(w, b.WANumber, ro)
	if !ok {
		return
	}
	if waNo != nil && *waNo == "" {
		waNo = nil
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
		branch := strings.TrimSpace(b.Branch)
		if branch == "" {
			branch = "Semua cabang"
		}
		id, err := s.st.Q.CreatePersonProfile(r.Context(), gen.CreatePersonProfileParams{Name: b.Name, Branch: branch, Role: ro.Base, Email: &b.Email})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		sales = &id
	}
	id, err := s.st.Q.CreateUser(r.Context(), gen.CreateUserParams{Email: &b.Email, Name: &b.Name, Role: &ro.Base, PasswordHash: &hash, SalesUserID: sales, RoleKey: &ro.Key, WaNumber: waNo})
	if waTaken(w, err) {
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Email sudah dipakai")
		return
	}
	if waNo != nil {
		_ = s.st.Q.SetSalesProfileWA(r.Context(), gen.SetSalesProfileWAParams{ID: *sales, WaNumber: *waNo})
	}
	s.auditUser(r, "user.create", "user:"+id.String(), map[string]any{"email": b.Email, "role": ro.Key, "wa_number": waNo})
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// updateUser changes role, WhatsApp number, branch, active flag, name or password (CEO/admin; CEO role changes only
// by the CEO). A number that is linked in Chat must be released there before it changes.
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
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Akun CEO hanya diubah oleh CEO")
		return
	}
	if id == me.ID && b.Active != nil && !*b.Active {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tidak bisa menonaktifkan akun sendiri")
		return
	}
	ro, err := s.st.Q.GetRole(r.Context(), coalesceStr(deref(cur.RoleKey), deref(cur.Role)))
	if err != nil {
		ro = gen.Role{Key: deref(cur.Role), Name: deref(cur.Role), Base: deref(cur.Role), WaAllowed: true}
	}
	if b.RoleKey != "" || b.Role != "" {
		var ok bool
		if ro, ok = s.userRole(w, r, me, b); !ok {
			return
		}
		if deref(cur.Role) == "ceo" && ro.Base != "ceo" {
			var others int
			_ = s.st.Pool.QueryRow(r.Context(), "select count(*) from users where role = 'ceo' and active and id <> $1", id).Scan(&others)
			if others == 0 {
				httpx.Fail(w, http.StatusBadRequest, "invalid", "Harus tetap ada satu CEO aktif")
				return
			}
		}
	}
	waNo, ok := userWA(w, b.WANumber, ro)
	if !ok {
		return
	}
	held, holdErr := s.st.Q.GetWANumberByUser(r.Context(), &id)
	if holdErr == nil {
		if waNo != nil && *waNo != held.WaNumber {
			httpx.Fail(w, http.StatusConflict, "wa_linked", "Nomor "+wa.MaskNumber(held.WaNumber)+" masih tertaut di Chat — lepas dulu di Chat → nomor itu, baru ganti")
			return
		}
		if !ro.WaAllowed && ro.Base != "ceo" {
			httpx.Fail(w, http.StatusConflict, "wa_linked", "Pengguna ini memegang nomor WhatsApp — lepas dulu sebelum pindah ke peran tanpa WhatsApp")
			return
		}
	} else if !errors.Is(holdErr, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusInternalServerError, "internal", holdErr.Error())
		return
	}
	p := gen.UpdateUserParams{ID: id, Active: b.Active}
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
	prof := gen.SetUserProfileParams{ID: id, WaNumber: waNo}
	if b.RoleKey != "" || b.Role != "" {
		prof.RoleKey, prof.Role = &ro.Key, &ro.Base
	}
	if err := s.st.Q.SetUserProfile(r.Context(), prof); err != nil {
		if waTaken(w, err) {
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if cur.SalesUserID != nil && (waNo != nil || b.Branch != "" || prof.Role != nil) {
		wn := deref(cur.WaNumber)
		if waNo != nil {
			wn = *waNo
		}
		sp := gen.SetSalesProfileWAParams{ID: *cur.SalesUserID, WaNumber: wn, Role: prof.Role}
		if br := strings.TrimSpace(b.Branch); br != "" {
			sp.Branch = &br
		}
		if err := s.st.Q.SetSalesProfileWA(r.Context(), sp); err != nil && !strings.Contains(err.Error(), "wa_number") {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	s.auditUser(r, "user.update", "user:"+id.String(), map[string]any{"role": ro.Key, "active": b.Active, "wa_number": waNo, "password_reset": b.Password != ""})
	w.WriteHeader(http.StatusNoContent)
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
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

// totpRoles may turn on 2FA (09-policies-security: CEO and admin).
func totpRoles(role string) bool { return role == "ceo" || role == "admin" }

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
	httpx.JSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": auth.TOTPURI("Distri ARC", deref(u.Email), secret)})
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
