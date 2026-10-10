package orchestrator

import (
	"strings"
	"testing"
	"time"
)

// The task Claude receives for a cycle analysed through MCP names the cycle, every agent and both tools.
func TestMCPPrompt(t *testing.T) {
	p := MCPPrompt(8, "c-123", []string{"AI Order", "AI Kredit"}, 10*time.Minute)
	for _, want := range []string{"#8", "c-123", "AI Order, AI Kredit", "orchestrator_input_get", "orchestrator_submit", "10 menit", "keputusan tetap di aplikasi"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, p)
		}
	}
	if !strings.Contains(MCPPrompt(1, "x", nil, 0), "10 menit") {
		t.Fatal("no wait: default 10 minutes")
	}
}
