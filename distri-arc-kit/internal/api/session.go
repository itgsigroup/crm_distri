package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/auth"
)

// Sesi login tanpa batas waktu (Sam 28 Oct): a login lasts until the person logs out — no expiry, no idle timeout.
// The token carries its session id (user_sessions) and no expiry; logging out revokes the row, so the token (and any
// copy of it) stops working. Deactivating a user ends their sessions too (the user lookup needs an active user).

// sessionCookie carries the signed session token (HttpOnly, SameSite=Strict).
const sessionCookie = "arc_session"

// cookieMaxAge: browsers keep a cookie at most 400 days (Chrome's cap), so the cookie is set again on use (renewAfter)
// and a login in use never runs out.
const cookieMaxAge = 400 * 24 * time.Hour

// renewAfter: a session's cookie is set again (and its last_seen_at noted) at most this often.
const renewAfter = time.Hour

// startSession records a new login of the user and sets its cookie.
func (s *Server) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID uuid.UUID, email, role string) error {
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	var sid uuid.UUID
	if err := s.st.Pool.QueryRow(ctx, "insert into user_sessions (user_id, ip, user_agent) values ($1, $2, $3) returning id", userID, clientIP(r), ua).Scan(&sid); err != nil {
		return err
	}
	s.setSessionToken(w, r, auth.Claims{Sub: userID.String(), Email: email, Role: role, Sid: sid.String(), Iat: time.Now().Unix()})
	return nil
}

func (s *Server) setSessionToken(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	s.setSession(w, r, auth.Sign(c, s.secret()), int(cookieMaxAge.Seconds()))
}

// sessionClaims reads the session cookie: a valid token whose login was not logged out. err is a server failure (the
// database), never "not logged in" — the caller must not send the person to the login page for it.
func (s *Server) sessionClaims(r *http.Request) (auth.Claims, bool, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || s.secret() == nil {
		return auth.Claims{}, false, nil
	}
	cl, err := auth.Verify(c.Value, s.secret(), time.Now())
	if err != nil {
		return auth.Claims{}, false, nil
	}
	if cl.Sid == "" {
		return cl, true, nil // a token from before 28 Oct: valid until its expiry, moved to a session on first use
	}
	var user uuid.UUID
	var revoked *time.Time
	err = s.st.Pool.QueryRow(r.Context(), "select user_id, revoked_at from user_sessions where id = $1", cl.Sid).Scan(&user, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Claims{}, false, nil
	}
	if err != nil {
		return auth.Claims{}, false, err
	}
	if revoked != nil || user.String() != cl.Sub {
		return auth.Claims{}, false, nil
	}
	return cl, true, nil
}

// renewSession sets the cookie again once an hour while the person works (the browser's 400-day clock restarts) and
// notes when the session was last used; a token from before 28 Oct becomes a session of its own.
func (s *Server) renewSession(w http.ResponseWriter, r *http.Request, u User, cl auth.Claims) {
	now := time.Now()
	if cl.Sub != u.ID.String() || (cl.Sid != "" && now.Sub(time.Unix(cl.Iat, 0)) < renewAfter) {
		return
	}
	if cl.Sid == "" {
		if err := s.startSession(r.Context(), w, r, u.ID, deref(u.Email), deref(u.Role)); err != nil {
			s.log.Warn("session upgrade", "err", err) // the old token keeps working until its expiry
		}
		return
	}
	if _, err := s.st.Pool.Exec(r.Context(), "update user_sessions set last_seen_at = now() where id = $1", cl.Sid); err != nil {
		return
	}
	s.setSessionToken(w, r, auth.Claims{Sub: cl.Sub, Email: cl.Email, Role: deref(u.Role), Sid: cl.Sid, Iat: now.Unix()})
}

// sessionEmail is the signed-in email: the session cookie, else the X-Dev-User header when APP_ENV=dev.
func (s *Server) sessionEmail(r *http.Request) (string, auth.Claims, error) {
	cl, ok, err := s.sessionClaims(r)
	if err != nil {
		return "", auth.Claims{}, err
	}
	if ok {
		return cl.Email, cl, nil
	}
	if s.cfg.IsDev() {
		return strings.TrimSpace(r.Header.Get("X-Dev-User")), auth.Claims{}, nil
	}
	return "", auth.Claims{}, nil
}
