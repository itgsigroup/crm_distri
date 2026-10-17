package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"distri-arc/internal/auth"
	"distri-arc/internal/config"
	"distri-arc/internal/store/gen"
)

func client(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func do(t *testing.T, c *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, rd)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// Login sets an HttpOnly session; the session alone authenticates; sales see only their dealers and cannot change
// policies; wrong passwords are rate-limited.
func TestLoginSessionAndRoles(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	h, _ := auth.Hash("rahasia-demo-123")
	for _, e := range []string{"andi@gsi.co.id", "sam@gsi.co.id", "finance@gsi.co.id"} {
		if err := st.Q.SetUserPassword(ctx, gen.SetUserPasswordParams{Lower: e, PasswordHash: &h}); err != nil {
			t.Fatal(err)
		}
	}
	c := client(t)
	if code, _ := do(t, c, http.MethodGet, srv.URL+"/api/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", code)
	}
	if code, _ := do(t, c, http.MethodPost, srv.URL+"/api/auth/login", map[string]string{"email": "andi@gsi.co.id", "password": "salah"}); code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", code)
	}
	code, me := do(t, c, http.MethodPost, srv.URL+"/api/auth/login", map[string]string{"email": "ANDI@gsi.co.id", "password": "rahasia-demo-123"})
	if code != http.StatusOK || me["role"] != "sales" {
		t.Fatalf("login %d %v", code, me)
	}
	_, orbit := do(t, c, http.MethodGet, srv.URL+"/api/orbit", nil)
	for _, it := range orbit["items"].([]any) {
		if it.(map[string]any)["owner"].(map[string]any)["name"] != "Andi" {
			t.Fatalf("Andi sees %v", it.(map[string]any)["id"])
		}
	}
	if code, _ := do(t, c, http.MethodGet, srv.URL+"/api/dealers/mitra", nil); code != http.StatusOK {
		t.Fatalf("Andi cannot open a dealer page: %d", code)
	}
	if code, _ := do(t, c, http.MethodPut, srv.URL+"/api/policies/margin.floor", map[string]any{"pct": 12}); code != http.StatusForbidden {
		t.Fatalf("sales changed a policy: %d", code)
	}
	if code, _ := do(t, c, http.MethodPost, srv.URL+"/api/auth/logout", nil); code != http.StatusNoContent {
		t.Fatalf("logout %d", code)
	}
	if code, _ := do(t, c, http.MethodGet, srv.URL+"/api/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", code)
	}

	ceo := client(t)
	do(t, ceo, http.MethodPost, srv.URL+"/api/auth/login", map[string]string{"email": "sam@gsi.co.id", "password": "rahasia-demo-123"})
	if code, out := do(t, ceo, http.MethodPut, srv.URL+"/api/policies/margin.floor", map[string]any{"pct": 1}); code != http.StatusBadRequest {
		t.Fatalf("out of range accepted: %d %v", code, out)
	}
	code, out := do(t, ceo, http.MethodPut, srv.URL+"/api/policies/margin.floor", map[string]any{"pct": 10})
	if code != http.StatusOK || out["version"].(float64) != 2 {
		t.Fatalf("ceo policy: %d %v", code, out)
	}
	_, h2 := do(t, ceo, http.MethodGet, srv.URL+"/api/policies/margin.floor/history", nil)
	if len(h2["items"].([]any)) != 1 {
		t.Fatalf("history %v", h2)
	}
	_, users := do(t, ceo, http.MethodGet, srv.URL+"/api/users", nil)
	if len(users["items"].([]any)) < 8 {
		t.Fatalf("users %v", users)
	}

	bad := client(t)
	for range 10 {
		do(t, bad, http.MethodPost, srv.URL+"/api/auth/login", map[string]string{"email": "finance@gsi.co.id", "password": "tebakan"})
	}
	if code, _ := do(t, bad, http.MethodPost, srv.URL+"/api/auth/login", map[string]string{"email": "finance@gsi.co.id", "password": "rahasia-demo-123"}); code != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", code)
	}
}

// Sam 17 Oct: a session renews itself while the user works (no re-login after 12 hours), and ends only at logout.
func TestSessionRenews(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	h, _ := auth.Hash("rahasia-demo-123")
	if err := st.Q.SetUserPassword(ctx, gen.SetUserPasswordParams{Lower: "sam@gsi.co.id", PasswordHash: &h}); err != nil {
		t.Fatal(err)
	}
	u, _ := st.Q.GetUserByEmail(ctx, "sam@gsi.co.id")
	// a token issued 11 hours ago that expires in one hour (as the old 12-hour sessions)
	old := time.Now().Add(-11 * time.Hour)
	tok := auth.Sign(auth.Claims{Sub: u.ID.String(), Email: "sam@gsi.co.id", Role: "ceo", Iat: old.Unix(), Exp: time.Now().Add(time.Hour).Unix()}, config.Config{Env: "dev"}.SessionKey())
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "arc_session", Value: tok})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var renewed *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "arc_session" {
			renewed = c
		}
	}
	if res.StatusCode != 200 || renewed == nil || renewed.MaxAge < 29*24*3600 {
		t.Fatalf("status %d cookie %+v", res.StatusCode, renewed)
	}
	cl, err := auth.Verify(renewed.Value, config.Config{Env: "dev"}.SessionKey(), time.Now().Add(20*24*time.Hour))
	if err != nil || cl.Sub != u.ID.String() {
		t.Fatalf("renewed token: %v %+v", err, cl)
	}
}
