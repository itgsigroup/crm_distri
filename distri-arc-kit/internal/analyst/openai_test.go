package analyst_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"distri-arc/internal/analyst"
)

// chatScript plays back ChatGPT turns and keeps the requests it got.
type chatScript struct {
	turns []string
	reqs  []analyst.ChatRequest
}

func (s *chatScript) Chat(_ context.Context, r analyst.ChatRequest) (*analyst.ChatResponse, error) {
	s.reqs = append(s.reqs, r)
	var out analyst.ChatResponse
	if err := json.Unmarshal([]byte(s.turns[len(s.reqs)-1]), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ADR 0040: with ChatGPT as the API, the run drives the same MCP tools (schedule scopes, never decide), sends tool
// results back as tool messages, and logs tokens and cost under provider openai.
func TestOpenAIRun(t *testing.T) {
	chatGPTOn(t) // the ChatGPT client itself, though GSI Orbit keeps it off (ADR 0048)
	st, sch, rn := setup(t)
	ctx := context.Background()
	if err := analyst.SaveConfig(ctx, st.Q, analyst.Config{Model: "claude-opus-5-5", OpenAIModel: "gpt-5-mini", Fallback: analyst.FallbackOpenAI}, nil, now.Now()); err != nil {
		t.Fatal(err)
	}
	sc := &chatScript{turns: []string{
		`{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"data_ringkasan","arguments":"{}"}}]}}],"usage":{"prompt_tokens":1500,"completion_tokens":80}}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":"## Ringkasan pagi\n\nOmzet stabil."}}],"usage":{"prompt_tokens":2500,"completion_tokens":300}}`,
	}}
	rn.EnvOpenAI = "sk-proj-test-000000000000000000"
	rn.NewChat = func(key string) analyst.ChatModel {
		if key != rn.EnvOpenAI {
			t.Errorf("key %q", key)
		}
		return sc
	}
	run, err := rn.Run(ctx, sch.ID, nil, "manual", "Sam")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "ok" || deref(run.Engine) != "openai" || deref(run.Model) != "gpt-5-mini" || !strings.HasPrefix(deref(run.Report), "## Ringkasan pagi") {
		t.Fatalf("run %s %s %s %q %v", run.Status, deref(run.Engine), deref(run.Model), deref(run.Report), deref(run.Error))
	}
	names := map[string]bool{}
	for _, tl := range sc.reqs[0].Tools {
		names[tl["function"].(map[string]any)["name"].(string)] = true
	}
	if !names["data_ringkasan"] || names["actions_decide"] {
		t.Fatalf("tools offered %v", names)
	}
	last := sc.reqs[1].Messages[len(sc.reqs[1].Messages)-1]
	if last["role"] != "tool" || last["tool_call_id"] != "c1" || !strings.Contains(last["content"].(string), "omzet_bln") {
		t.Fatalf("tool result not sent back: %v", last)
	}
	if run.TokensIn != 4000 || run.TokensOut != 380 || run.CostIdr <= 0 {
		t.Fatalf("tokens %d/%d cost %d", run.TokensIn, run.TokensOut, run.CostIdr)
	}
	var n int
	_ = st.Pool.QueryRow(ctx, `select count(*) from llm_calls where provider = 'openai' and model = 'gpt-5-mini' and cost_idr > 0`).Scan(&n)
	if n != 2 {
		t.Fatalf("llm_calls %d", n)
	}
}

// The second option is ready only when it is switched on, its key is there and today's budget is not spent.
func TestFallbackReady(t *testing.T) {
	chatGPTOn(t) // the ChatGPT client itself, though GSI Orbit keeps it off (ADR 0048)
	st, _, _ := setup(t)
	ctx := context.Background()
	secret := []byte("0123456789abcdef0123456789abcdef")
	if ok, _, _ := analyst.FallbackReady(ctx, st.Q, now.Now(), secret, "sk-ant-x", "sk-x"); ok {
		t.Fatal("off by default")
	}
	_ = analyst.SaveConfig(ctx, st.Q, analyst.Config{Model: "claude-opus-5-5", Fallback: analyst.FallbackOpenAI}, nil, now.Now())
	if ok, _, _ := analyst.FallbackReady(ctx, st.Q, now.Now(), secret, "sk-ant-x", ""); ok {
		t.Fatal("ready without a ChatGPT key")
	}
	ok, prov, model := analyst.FallbackReady(ctx, st.Q, now.Now(), secret, "", "sk-x")
	if !ok || prov != "openai" || model != "gpt-5" {
		t.Fatalf("%v %s %s", ok, prov, model)
	}
}

// chatGPTOn lets a test reach the ChatGPT client (off in GSI Orbit since 19 Oct).
func chatGPTOn(t *testing.T) {
	t.Helper()
	analyst.ChatGPTOff = false
	t.Cleanup(func() { analyst.ChatGPTOff = true })
}

// ADR 0048: ChatGPT is never used — a second option set to ChatGPT with its key present is not ready.
func TestChatGPTNeverUsed(t *testing.T) {
	st, _, _ := setup(t)
	ctx := context.Background()
	secret := []byte("0123456789abcdef0123456789abcdef")
	_ = analyst.SaveConfig(ctx, st.Q, analyst.Config{Model: "claude-opus-5-5", Fallback: analyst.FallbackOpenAI}, nil, now.Now())
	if ok, _, _ := analyst.FallbackReady(ctx, st.Q, now.Now(), secret, "", "sk-x"); ok {
		t.Fatal("ChatGPT second option ready while ChatGPT is off")
	}
	if k, _ := analyst.LoadOpenAIKey(ctx, st.Q, secret, "sk-x"); k != "" {
		t.Fatal("ChatGPT key read while ChatGPT is off")
	}
}
