package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/mcp"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

func jsonOf(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	return m
}

// Claude connects as a custom connector: discovery, dynamic registration, authorization code + PKCE approved by a
// person allowed to (not a sales with own-data scope), a short access token used on /mcp, a rotating refresh token.
func TestMCPOAuthFlow(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	c := clock.Fixed(time.Date(2026, 10, 5, 9, 0, 0, 0, clock.WIB))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := mcp.New(st, c, log, nil)
	srv := httptest.NewUnstartedServer(nil)
	base := "http://" + srv.Listener.Addr().String()
	m.ResourceMetadataURL = base + "/.well-known/oauth-protected-resource"
	srv.Config.Handler = api.New(config.Config{Env: "dev", PublicURL: base}, st, c, log).WithMCP(m).Handler()
	srv.Start()
	t.Cleanup(srv.Close)
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// 401 points to the metadata; metadata points to the authorization server
	res, _ := http.Post(base+"/mcp", "application/json", strings.NewReader(`{}`))
	if res.StatusCode != 401 || !strings.Contains(res.Header.Get("WWW-Authenticate"), "resource_metadata") {
		t.Fatalf("mcp 401: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
	res.Body.Close()
	res, _ = http.Get(base + "/.well-known/oauth-protected-resource")
	if pr := jsonOf(t, res); pr["resource"] != base+"/mcp" {
		t.Fatalf("resource metadata %v", pr)
	}
	res, _ = http.Get(base + "/.well-known/oauth-authorization-server")
	as := jsonOf(t, res)
	if as["token_endpoint"] != base+"/oauth/token" || as["code_challenge_methods_supported"].([]any)[0] != "S256" {
		t.Fatalf("as metadata %v", as)
	}

	// registration: https or loopback redirects only
	res, _ = http.Post(base+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"x","redirect_uris":["http://evil.example/cb"]}`))
	if res.StatusCode != 400 {
		t.Fatalf("http redirect accepted: %d", res.StatusCode)
	}
	res.Body.Close()
	cb := "https://claude.ai/api/mcp/auth_callback"
	res, _ = http.Post(base+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"Claude","redirect_uris":["`+cb+`"],"token_endpoint_auth_method":"none"}`))
	reg := jsonOf(t, res)
	clientID, _ := reg["client_id"].(string)
	if res.StatusCode != 201 || clientID == "" {
		t.Fatalf("register %d %v", res.StatusCode, reg)
	}

	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	authz := func(extra url.Values) *http.Response {
		v := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {cb}, "state": {"s1"}, "scope": {"read analyze orchestrate"}}
		for k, x := range extra {
			v[k] = x
		}
		r, err := noFollow.Get(base + "/oauth/authorize?" + v.Encode())
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r
	}
	if r := authz(nil); r.StatusCode != 302 || !strings.Contains(r.Header.Get("Location"), "error=invalid_request") {
		t.Fatalf("authorize without PKCE: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r := authz(url.Values{"redirect_uri": {"https://evil.example/cb"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}); r.StatusCode != 400 {
		t.Fatalf("unregistered redirect: %d", r.StatusCode)
	}
	r := authz(url.Values{"code_challenge": {challenge}, "code_challenge_method": {"S256"}})
	loc, _ := url.Parse(r.Header.Get("Location"))
	reqID := loc.Query().Get("req")
	if r.StatusCode != 302 || loc.Path != "/claude/izin" || reqID == "" {
		t.Fatalf("authorize: %d %s", r.StatusCode, r.Header.Get("Location"))
	}

	// a sales user (own data) cannot approve; the CEO can, with orchestrate
	if _, info := get(t, srv, "/api/oauth/requests/"+reqID, "andi@gsi.co.id"); info["allowed"] != false {
		t.Fatalf("sales allowed: %v", info)
	}
	if code, _ := post(t, base+"/api/oauth/requests/"+reqID+"/approve", "andi@gsi.co.id", map[string]any{}); code != 403 {
		t.Fatalf("sales approved: %d", code)
	}
	code, out := post(t, base+"/api/oauth/requests/"+reqID+"/approve", "sam@gsi.co.id", map[string]any{"scopes": []string{"read", "analyze", "orchestrate"}})
	back, _ := url.Parse(out["redirect"].(string))
	authCode := back.Query().Get("code")
	if code != 200 || !strings.HasPrefix(out["redirect"].(string), cb) || back.Query().Get("state") != "s1" || authCode == "" {
		t.Fatalf("approve %d %v", code, out)
	}

	tok := func(v url.Values) (int, map[string]any) {
		res, err := http.PostForm(base+"/oauth/token", v)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, jsonOf(t, res)
	}
	if code, out := tok(url.Values{"grant_type": {"authorization_code"}, "code": {authCode}, "client_id": {clientID}, "redirect_uri": {cb}, "code_verifier": {strings.Repeat("w", 50)}}); code != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("wrong verifier: %d %v", code, out)
	}
	// the code was spent by the failed attempt: a real client starts over; issue a fresh one
	r = authz(url.Values{"code_challenge": {challenge}, "code_challenge_method": {"S256"}})
	loc, _ = url.Parse(r.Header.Get("Location"))
	_, out = post(t, base+"/api/oauth/requests/"+loc.Query().Get("req")+"/approve", "sam@gsi.co.id", map[string]any{})
	back, _ = url.Parse(out["redirect"].(string))
	authCode = back.Query().Get("code")
	code, grant := tok(url.Values{"grant_type": {"authorization_code"}, "code": {authCode}, "client_id": {clientID}, "redirect_uri": {cb}, "code_verifier": {verifier}})
	access, _ := grant["access_token"].(string)
	refresh, _ := grant["refresh_token"].(string)
	if code != 200 || access == "" || refresh == "" || grant["expires_in"].(float64) > 3600 {
		t.Fatalf("token: %d %v", code, grant)
	}
	if code, _ := tok(url.Values{"grant_type": {"authorization_code"}, "code": {authCode}, "client_id": {clientID}, "redirect_uri": {cb}, "code_verifier": {verifier}}); code != 400 {
		t.Fatalf("code reused: %d", code)
	}

	// the token opens /mcp
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
	req, _ := http.NewRequest("POST", base+"/mcp", strings.NewReader(init))
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("mcp with oauth token: %v %v", err, res.StatusCode)
	}
	res.Body.Close()

	// refresh rotates; the old refresh token is dead
	code, g2 := tok(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}})
	if code != 200 || g2["refresh_token"] == refresh || g2["access_token"] == access {
		t.Fatalf("refresh: %d %v", code, g2)
	}
	if code, _ := tok(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}); code != 400 {
		t.Fatalf("old refresh accepted: %d", code)
	}
	// the connection is listed and its scopes are what the CEO gave
	_, clients := get(t, srv, "/api/mcp/clients", "sam@gsi.co.id")
	found := false
	for _, x := range clients["items"].([]any) {
		cl := x.(map[string]any)
		if cl["kind"] == "oauth" && strings.HasPrefix(cl["name"].(string), "Claude · ") {
			found = len(cl["scopes"].([]any)) == 3
		}
	}
	if !found {
		t.Fatalf("connection not listed: %v", clients)
	}
}
