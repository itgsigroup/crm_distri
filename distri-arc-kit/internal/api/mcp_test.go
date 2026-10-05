package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func put(t *testing.T, url, user string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	req.Header.Set("X-Dev-User", user)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// allow_send is locked: no client, not even the CEO, can let MCP send to dealers.
func TestMCPPolicyAllowSendLocked(t *testing.T) {
	srv, _ := chatServer(t)
	if code, _ := put(t, srv.URL+"/api/policies/mcp", "sam@gsi.co.id", map[string]any{"allow_reanalyze": true, "allow_send": true, "max_cycles_per_hour": 6}); code != http.StatusBadRequest {
		t.Fatalf("allow_send:true → %d", code)
	}
	if code, _ := put(t, srv.URL+"/api/policies/mcp", "andi@gsi.co.id", map[string]any{"allow_reanalyze": false}); code != http.StatusForbidden {
		t.Fatalf("sales edited MCP policy: %d", code)
	}
	code, out := put(t, srv.URL+"/api/policies/mcp", "sam@gsi.co.id", map[string]any{"allow_reanalyze": false, "allow_plan_update_proposal": true, "mask_pii_in_read": true, "max_cycles_per_hour": 4})
	if code != http.StatusOK || out["allow_reanalyze"] != false || out["allow_send"] != false {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, _ := put(t, srv.URL+"/api/policies/llm", "sam@gsi.co.id", map[string]any{"mode": "mcp"}); code != http.StatusOK {
		t.Fatalf("llm mode: %d", code)
	}
	if code, _ := put(t, srv.URL+"/api/policies/llm", "sam@gsi.co.id", map[string]any{"mode": "telepati"}); code != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", code)
	}
}

// Tokens are created by the CEO, shown once, never listed in clear.
func TestMCPTokenCreate(t *testing.T) {
	srv, _ := chatServer(t)
	if code, _ := post(t, srv.URL+"/api/mcp/clients", "andi@gsi.co.id", map[string]any{"name": "x", "scopes": []string{"read"}}); code != http.StatusForbidden {
		t.Fatalf("sales created a token: %d", code)
	}
	if code, _ := post(t, srv.URL+"/api/mcp/clients", "sam@gsi.co.id", map[string]any{"name": "x", "scopes": []string{"decide"}}); code != http.StatusBadRequest {
		t.Fatalf("decide scope accepted: %d", code)
	}
	code, out := post(t, srv.URL+"/api/mcp/clients", "sam@gsi.co.id", map[string]any{"name": "Claude Desktop Sam", "scopes": []string{"read", "orchestrate"}})
	tok, _ := out["token"].(string)
	if code != http.StatusCreated || !strings.HasPrefix(tok, "arc_") {
		t.Fatalf("create: %d %v", code, out)
	}
	_, list := get(t, srv, "/api/mcp/clients", "sam@gsi.co.id")
	b, _ := json.Marshal(list)
	if strings.Contains(string(b), tok) || strings.Contains(string(b), "argon2id") {
		t.Fatal("token or hash listed")
	}
}
