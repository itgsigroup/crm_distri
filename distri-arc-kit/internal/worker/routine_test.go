package worker

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/analyst"
	"distri-arc/internal/clock"
	"distri-arc/internal/testdb"
)

// ADR 0049: when Analisis starts waiting for Claude through MCP and a Claude routine is stored, the hook fires it
// with the cycle's prompt and waits RoutineWindow longer — before (and instead of) any API second option.
func TestHookFiresClaudeRoutine(t *testing.T) {
	st := testdb.New(t)
	secret := []byte("0123456789abcdef0123456789abcdef")
	var text string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Text string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		text = b.Text
		_, _ = w.Write([]byte(`{"type":"routine_fire","claude_code_session_url":"https://claude.ai/code/session_9"}`))
	}))
	defer srv.Close()
	defer analyst.SetRoutineBase(srv.URL)()
	fb := &Fallback{St: st, Clock: clock.Fixed(time.Now()), Secret: secret}
	hook := fb.Hook(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cid := uuid.New()

	if extra := hook(t.Context(), cid, 7, []string{"AI Order"}, 10*time.Minute); extra != 0 || text != "" {
		t.Fatalf("no routine stored: extra %s, fired %q", extra, text)
	}
	if err := analyst.SaveRoutine(t.Context(), st.Q, "trig_01ABCDEFGH", "sk-ant-oat01-abcdefghijklmnop", secret, nil); err != nil {
		t.Fatal(err)
	}
	extra := hook(t.Context(), cid, 7, []string{"AI Order", "AI Kredit", "Analisa Stok"}, 10*time.Minute)
	if extra != RoutineWindow {
		t.Fatalf("extra %s", extra)
	}
	if !strings.Contains(text, cid.String()) || !strings.Contains(text, "#7") || !strings.Contains(text, "AI Order, AI Kredit") || !strings.Contains(text, "orchestrator_submit") {
		t.Fatalf("prompt: %s", text)
	}
}
