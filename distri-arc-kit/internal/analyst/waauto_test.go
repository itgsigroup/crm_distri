package analyst_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"distri-arc/internal/analyst"
)

type failModel struct{}

func (failModel) Step(context.Context, anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	return nil, errors.New("overloaded")
}

// ADR 0043: Analisa WhatsApp otomatis runs at the chosen hours with the chosen API and model; when that API fails the
// run fails — ChatGPT never takes over a failed Claude run.
func TestWAAuto(t *testing.T) {
	chatGPTOn(t) // the ChatGPT client itself, though GSI Orbit keeps it off (ADR 0048)
	st, _, rn := setup(t)
	ctx := context.Background()
	a, ok := analyst.LoadWAAuto(ctx, st.Q)
	if !ok || a.Enabled || a.Engine != "claude" || len(a.Hours) != 5 || a.Weekend {
		t.Fatalf("default %+v %v", a, ok)
	}
	for _, bad := range []analyst.WAAuto{
		{Enabled: true, Hours: []int{9}, Engine: "openai", Model: "claude-opus-5-5"},
		{Enabled: true, Hours: nil, Engine: "claude", Model: "claude-opus-5-5"},
		{Enabled: true, Hours: []int{24}, Engine: "claude", Model: "claude-opus-5-5"},
		{Enabled: true, Hours: []int{9}, Engine: "gemini", Model: "x"},
	} {
		if err := analyst.SaveWAAuto(ctx, st, bad, nil, now.Now()); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	if err := analyst.SaveWAAuto(ctx, st, analyst.WAAuto{Enabled: true, Hours: []int{14, 9, 9}, Engine: "openai", Model: "gpt-5-mini"}, nil, now.Now()); err != nil {
		t.Fatal(err)
	}
	sch, err := st.Q.GetMCPScheduleByKey(ctx, ptr(analyst.KeyWAAuto))
	if err != nil || sch.Cron != "0 9,14 * * 1-6" || !sch.Enabled || sch.Runner != "server" {
		t.Fatalf("schedule %+v %v", sch, err)
	}
	if !analyst.WAAutoOn(ctx, st.Q) {
		t.Fatal("not on")
	}
	// the server ticks it: next run is 09:00 today (Monday, now 07:00)
	_ = must(analyst.Due(ctx, st.Q, now.Now()))
	if a, _ = analyst.LoadWAAuto(ctx, st.Q); a.NextRun == nil || a.NextRun.Hour() != 9 || a.Engine != "openai" || a.Model != "gpt-5-mini" {
		t.Fatalf("loaded %+v", a)
	}

	// ChatGPT chosen: the run uses ChatGPT even though the general API is Claude, and must end with a submit
	rn.EnvKey, rn.EnvOpenAI = "sk-ant-test-key-000000000000", "sk-proj-test-000000000000000000"
	rn.NewModel = func(string) analyst.Model { t.Error("Claude called for a ChatGPT schedule"); return failModel{} }
	rn.NewChat = func(string) analyst.ChatModel {
		return &chatScript{turns: []string{`{"choices":[{"finish_reason":"stop","message":{"content":"Selesai."}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`}}
	}
	run, err := rn.Run(ctx, sch.ID, nil, "manual", "Sam")
	if err != nil || run.Status != "error" || deref(run.Engine) != "openai" || deref(run.Model) != "gpt-5-mini" || !strings.Contains(deref(run.Error), "tanpa mengirim") {
		t.Fatalf("chatgpt run %s %s %s %v %v", run.Status, deref(run.Engine), deref(run.Model), deref(run.Error), err)
	}

	// Claude chosen and Claude fails: the run fails, ChatGPT is not called
	if err := analyst.SaveWAAuto(ctx, st, analyst.WAAuto{Enabled: true, Hours: []int{9}, Weekend: true, Engine: "claude", Model: "claude-sonnet-5-5"}, nil, now.Now()); err != nil {
		t.Fatal(err)
	}
	rn.NewModel = func(string) analyst.Model { return failModel{} }
	rn.NewChat = func(string) analyst.ChatModel { t.Error("ChatGPT took over a failed Claude run"); return &chatScript{} }
	run, err = rn.Run(ctx, sch.ID, nil, "manual", "Sam")
	if err != nil || run.Status != "error" || deref(run.Engine) != "claude" || deref(run.Model) != "claude-sonnet-5-5" {
		t.Fatalf("claude run %s %s %s %v %v", run.Status, deref(run.Engine), deref(run.Model), deref(run.Error), err)
	}
	if sch, _ = st.Q.GetMCPScheduleByKey(ctx, ptr(analyst.KeyWAAuto)); sch.Cron != "0 9 * * *" {
		t.Fatalf("weekend cron %q", sch.Cron)
	}
}
