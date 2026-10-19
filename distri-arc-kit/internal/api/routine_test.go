package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// ADR 0049: the CEO stores the Claude routine (id + API-trigger token); the token never comes back, other roles cannot
// change it, and an Analisis ulang through MCP says the routine runs it.
func TestClaudeRoutineSetting(t *testing.T) {
	srv, _ := chatServer(t)
	token := "sk-ant-oat01-abcdefghijklmnopqrstuv"
	if code, _ := put(t, srv.URL+"/api/ai/routine", "andi@gsi.co.id", map[string]string{"routine_id": "trig_01ABCDEFGH", "token": token}); code != http.StatusForbidden {
		t.Fatalf("sales saved the routine: %d", code)
	}
	if code, out := put(t, srv.URL+"/api/ai/routine", "sam@gsi.co.id", map[string]string{"routine_id": "trig_01ABCDEFGH", "token": "sk-ant-api03-wrongkindoftoken"}); code != http.StatusBadRequest {
		t.Fatalf("API key accepted as a routine token: %d %v", code, out)
	}
	code, out := put(t, srv.URL+"/api/ai/routine", "sam@gsi.co.id", map[string]string{"routine_id": "trig_01ABCDEFGH", "token": token})
	if code != http.StatusOK || out["set"] != true || out["url"] != "https://claude.ai/code/routines/trig_01ABCDEFGH" {
		t.Fatalf("save: %d %v", code, out)
	}
	_, got := get(t, srv, "/api/ai/routine", "sam@gsi.co.id")
	for _, v := range got {
		if s, ok := v.(string); ok && strings.Contains(s, "oat01") {
			t.Fatalf("token leaked: %v", got)
		}
	}
	code, cyc := post(t, srv.URL+"/api/cycles", "sam@gsi.co.id", map[string]string{"scope": "all", "via": "mcp"})
	m, _ := cyc["mcp"].(map[string]any)
	if code != http.StatusAccepted || m == nil || m["routine"] != true {
		t.Fatalf("cycle: %d %v", code, cyc)
	}
}
