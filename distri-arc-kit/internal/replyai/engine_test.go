package replyai

import (
	"context"
	"testing"
	"time"

	"distri-arc/internal/analyst"
	"distri-arc/internal/auth"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
)

// ADR 0051: AI Trigger drafts are written by the engine chosen on its page — ChatGPT by default — even though ChatGPT
// is off for every analysis (ADR 0048); an unknown model is refused.
func TestEngine(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	secret := []byte("0123456789abcdef0123456789abcdef")
	if e := LoadEngine(ctx, st.Q); e != DefaultEngine || e.Provider != analyst.FallbackOpenAI {
		t.Fatalf("default %+v", e)
	}
	s := &Service{St: st, SessionKey: secret}
	if s.engineModel(ctx) != nil {
		t.Fatal("a model without any key")
	}
	sealed, _ := auth.Seal("sk-proj-abcdefghijklmnopqrstuvwxyz", secret)
	if err := st.Q.SetSecret(ctx, gen.SetSecretParams{Key: analyst.SecretOpenAI, Value: sealed}); err != nil {
		t.Fatal(err)
	}
	if k, _ := analyst.LoadOpenAIKey(ctx, st.Q, secret, ""); k != "" {
		t.Fatal("ChatGPT key read for analysis")
	}
	if p, ok := s.engineModel(ctx).(*llm.OpenAI); !ok || p == nil {
		t.Fatalf("AI Trigger did not get ChatGPT: %T", s.engineModel(ctx))
	}
	if err := SaveEngine(ctx, st.Q, Engine{Provider: "openai", Model: "gpt-9"}, time.Now()); err == nil {
		t.Fatal("unknown model saved")
	}
	if err := SaveEngine(ctx, st.Q, Engine{Provider: "claude", Model: "claude-haiku-5-5"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if s.engineModel(ctx) != nil { // Claude chosen, no Claude key: no draft model (and never a silent switch to ChatGPT)
		t.Fatal("fell back to ChatGPT")
	}
}
