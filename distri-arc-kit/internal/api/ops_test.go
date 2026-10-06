package api_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/auth"
	"distri-arc/internal/store/gen"
)

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

// 2FA: setup → confirm → login needs the code; a code works once; a wrong code fails.
func TestTOTPLogin(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	h, _ := auth.Hash("rahasia-admin-123")
	if err := st.Q.SetUserPassword(ctx, gen.SetUserPasswordParams{Lower: "admin@gsi.co.id", PasswordHash: &h}); err != nil {
		t.Fatal(err)
	}
	login := func(code string) (int, map[string]any, *http.Client) {
		c := client(t)
		n, b := do(t, c, http.MethodPost, srv.URL+"/api/auth/login", map[string]string{"email": "admin@gsi.co.id", "password": "rahasia-admin-123", "code": code})
		return n, b, c
	}
	_, _, c := login("")
	n, setup := do(t, c, http.MethodPost, srv.URL+"/api/auth/totp/setup", nil)
	secret, _ := setup["secret"].(string)
	if n != 200 || secret == "" || !strings.HasPrefix(setup["uri"].(string), "otpauth://totp/") {
		t.Fatalf("setup %d %v", n, setup)
	}
	if n, b := do(t, c, http.MethodPost, srv.URL+"/api/auth/totp/enable", map[string]string{"code": "000000"}); n != 400 || errCode(b) != "totp_invalid" {
		t.Fatalf("wrong first code: %d %v", n, b)
	}
	now := time.Now()
	first, _ := auth.TOTPCode(secret, auth.TOTPStep(now)-1)
	if n, b := do(t, c, http.MethodPost, srv.URL+"/api/auth/totp/enable", map[string]string{"code": first}); n != 200 {
		t.Fatalf("enable %d %v", n, b)
	}
	if _, me := do(t, c, http.MethodGet, srv.URL+"/api/me", nil); me["totp_enabled"] != true {
		t.Fatalf("me %v", me)
	}
	if n, b, _ := login(""); n != 401 || errCode(b) != "totp_required" {
		t.Fatalf("login without code: %d %v", n, b)
	}
	code, _ := auth.TOTPCode(secret, auth.TOTPStep(now))
	if n, b, _ := login(code); n != 200 {
		t.Fatalf("login with code: %d %v", n, b)
	}
	if n, b, _ := login(code); n != 401 || errCode(b) != "totp_invalid" {
		t.Fatalf("replayed code: %d %v", n, b)
	}
	var sealed string
	_ = st.Pool.QueryRow(ctx, "select totp_secret from users where email = 'admin@gsi.co.id'").Scan(&sealed)
	if sealed == "" || strings.Contains(sealed, secret) {
		t.Fatal("secret stored in clear")
	}
	if n, _ := do(t, c, http.MethodPost, srv.URL+"/api/auth/totp/setup", nil); n != http.StatusConflict {
		t.Fatalf("setup while on: %d", n)
	}
	// sales cannot turn it on
	if n, _ := post(t, srv.URL+"/api/auth/totp/setup", "andi@gsi.co.id", nil); n != http.StatusForbidden {
		t.Fatalf("sales setup: %d", n)
	}
}

// /api/health: anonymous gets part statuses only; the CEO gets numbers, Odoo models, LLM and alerts. /metrics is
// Prometheus text.
func TestHealthDetailAndMetrics(t *testing.T) {
	srv := newServer(t)
	_, anon := get(t, srv, "/api/health", "")
	if anon["status"] == nil || anon["wa"] == nil {
		t.Fatalf("anonymous %v", anon)
	}
	if wa, _ := anon["wa"].(map[string]any); wa["total"] == nil {
		t.Fatalf("anonymous wa %v", anon["wa"])
	}
	_, full := get(t, srv, "/api/health", "sam@gsi.co.id")
	if ws, _ := full["wa"].([]any); len(ws) == 0 || full["odoo"] == nil || full["llm"] == nil || full["counts"] == nil {
		t.Fatalf("ceo %v", full)
	}
	_, _ = get(t, srv, "/api/orbit", "sam@gsi.co.id")
	res, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, want := range []string{"arc_up 1", "arc_queue_depth", `arc_wa_connected{number="6281234504471"`, `arc_http_request_duration_seconds_count{method="GET",route="/api/orbit"} 1`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}
