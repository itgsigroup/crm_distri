package worker

import (
	"testing"
	"time"
)

// ADR 0040: a cycle waiting for Claude through MCP and then the API second option is not cut off before both had
// their time; the API run on a cycle gets a scheduled analysis' time, not river's one-minute default.
func TestCycleTimeoutsCoverTheSecondOption(t *testing.T) {
	if got := (&CycleWorker{mcpWait: 10 * time.Minute}).Timeout(nil); got < 10*time.Minute+FallbackWindow {
		t.Errorf("cycle timeout %s shorter than MCP wait + fallback window", got)
	}
	if got := (&CycleWorker{}).Timeout(nil); got < 10*time.Minute+FallbackWindow {
		t.Errorf("default cycle timeout %s", got)
	}
	if got := (&CycleMCPWorker{}).Timeout(nil); got != runTimeout {
		t.Errorf("fallback run timeout %s, want %s", got, runTimeout)
	}
}
