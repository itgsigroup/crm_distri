package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type spy struct{ got Request }

func (s *spy) Name() string  { return "spy" }
func (s *spy) Model() string { return "spy" }
func (s *spy) Complete(_ context.Context, r Request) (Response, error) {
	s.got = r
	// echo the masked name back so unmasking can be checked
	return Response{JSON: []byte(`{"preview":"Halo <PIC_1>, nomor <NO_1>"}`)}, nil
}

// Phone numbers and PIC names never leave the server; the answer is unmasked.
func TestMaskingBeforeProvider(t *testing.T) {
	s := &spy{}
	r := &Router{Primary: s}
	in, _ := json.Marshal(map[string]string{"pic": "Pak Budi", "no": "6281900300101", "text": "Pak Budi minta kirim ke 0812-3450-4471"})
	res := r.Complete(context.Background(), Request{Purpose: "t", System: "sys", Input: in, Names: []string{"Pak Budi"}, Fallback: []byte(`{}`)}, nil)
	sent := string(s.got.Input)
	for _, leak := range []string{"Pak Budi", "6281900300101"} {
		if strings.Contains(sent, leak) {
			t.Fatalf("PII %q sent to provider: %s", leak, sent)
		}
	}
	if !strings.Contains(string(res.JSON), "Pak Budi") || !strings.Contains(string(res.JSON), "6281900300101") {
		t.Fatalf("answer not unmasked: %s", res.JSON)
	}
}

func TestFakeReturnsFallbackAndRouterValidates(t *testing.T) {
	r := &Router{Primary: NewFake(nil)}
	res := r.Complete(context.Background(), Request{Purpose: "x", Input: []byte(`{}`), Fallback: []byte(`{"preview":"ok"}`)}, func(b json.RawMessage) error { return nil })
	if string(res.JSON) != `{"preview":"ok"}` || !res.Fallback {
		t.Fatalf("fake: %+v", res)
	}
	if Cost("claude-sonnet-5-5", 1_000_000, 100_000) != int64(3*IDRPerUSD) {
		t.Fatalf("cost %d", Cost("claude-sonnet-5-5", 1_000_000, 100_000))
	}
}
