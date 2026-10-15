package analyst

import (
	"testing"

	"distri-arc/internal/llm"
)

// Adaptive thinking goes only to Claude 4.6 and later (Haiku 4.5 and older answer 400 to it).
func TestAdaptiveThinking(t *testing.T) {
	for id, want := range map[string]bool{
		"claude-opus-5-5": true, "claude-fable-5-1": true, "claude-sonnet-5": true, "claude-haiku-5-5": true,
		"claude-opus-4-6": true, "claude-sonnet-4-6": true, "claude-haiku-4-5": false, "claude-sonnet-4-5-20250929": false,
		"claude-opus-4-20250514": false, "claude-3-5-haiku-20241022": false,
	} {
		if got := adaptiveThinking(id); got != want {
			t.Errorf("%s: %v want %v", id, got, want)
		}
	}
}

func TestModelIDsAndPrices(t *testing.T) {
	for _, id := range []string{"claude-opus-5-5", "claude-3-5-haiku-20241022"} {
		if !ValidModel(id) {
			t.Error(id)
		}
	}
	for _, id := range []string{"gpt-5.1", "o3", "o4-mini", "chatgpt-4o-latest"} {
		if !ValidOpenAIModel(id) {
			t.Error(id)
		}
	}
	for _, id := range []string{"gpt 5", "llama-3", "claude-opus-5-5; drop", ""} {
		if ValidModel(id) || ValidOpenAIModel(id) {
			t.Errorf("%q accepted", id)
		}
	}
	if p, ok := llm.PriceOf("gpt-4.1-2025-04-14"); !ok || p.In != 2 {
		t.Errorf("dated snapshot price %v %v", p, ok)
	}
	if _, ok := llm.PriceOf("gpt-7"); ok {
		t.Error("unknown model priced")
	}
	for _, m := range append(append([]ModelInfo{}, Models...), OpenAIModels...) {
		if _, ok := llm.PriceOf(m.ID); !ok {
			t.Errorf("known model %s has no price", m.ID)
		}
	}
}
