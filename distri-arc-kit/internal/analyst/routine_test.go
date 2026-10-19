package analyst_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"distri-arc/internal/analyst"
)

// ADR 0049: a waiting cycle fires the Claude routine through its API trigger — the documented request, and the
// session link back; a rejected token says how to fix it.
func TestFireRoutine(t *testing.T) {
	var got struct {
		path, auth, version string
		text                string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path, got.auth, got.version = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("anthropic-version")
		var b struct{ Text string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		got.text = b.Text
		if got.auth != "Bearer sk-ant-oat01-good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid token"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"type":"routine_fire","claude_code_session_id":"session_1","claude_code_session_url":"https://claude.ai/code/session_1"}`))
	}))
	defer srv.Close()
	defer analyst.SetRoutineBase(srv.URL)()

	url, err := analyst.FireRoutine(t.Context(), "trig_01ABCDEFGH", "sk-ant-oat01-good", "Siklus #5 menunggu")
	if err != nil || url != "https://claude.ai/code/session_1" {
		t.Fatalf("fire: %q %v", url, err)
	}
	if got.path != "/v1/claude_code/routines/trig_01ABCDEFGH/fire" || got.version != "2023-06-01" || got.text != "Siklus #5 menunggu" {
		t.Fatalf("request %+v", got)
	}
	if _, err := analyst.FireRoutine(t.Context(), "trig_01ABCDEFGH", "sk-ant-oat01-bad", "x"); err == nil || !strings.Contains(err.Error(), "token routine Claude ditolak") {
		t.Fatalf("bad token: %v", err)
	}
}

func TestValidRoutine(t *testing.T) {
	if err := analyst.ValidRoutine("trig_01ABCDEFGH", "sk-ant-oat01-abcdefghijklmnop"); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{{"routine-1", "sk-ant-oat01-abcdefghijklmnop"}, {"trig_01ABCDEFGH", "sk-ant-api03-abcdefghijklmnop"}, {"trig_01ABCDEFGH", "sk-ant-oat01 x"}} {
		if analyst.ValidRoutine(c[0], c[1]) == nil {
			t.Errorf("accepted %v", c)
		}
	}
}
