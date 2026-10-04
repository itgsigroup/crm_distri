package llm

import (
	"context"
	"strings"
	"testing"
)

type recordingProvider struct{ last Request }

func (p *recordingProvider) Name() string   { return "anthropic" }
func (p *recordingProvider) External() bool { return true }
func (p *recordingProvider) Complete(_ context.Context, _ string, req Request) (Response, error) {
	p.last = req
	return Response{Text: `{"echo":"` + strings.ReplaceAll(req.Messages[0].Content, `"`, `'`) + `"}`, Model: "claude-haiku-4-5", TokensIn: 10, TokensOut: 5}, nil
}

func TestMaskUnmask(t *testing.T) {
	m := NewMasker()
	in := "Hubungi Pak Rudi di +62 812-3456-5521 atau 0812 3456 5521, email rudi@primakarya.co.id, rek: 1234-5678-90"
	out := m.Mask(in)
	if ContainsRawPhone(out) || strings.Contains(out, "rudi@") || strings.Contains(out, "1234-5678-90") {
		t.Fatalf("PII leaked: %s", out)
	}
	if !strings.Contains(out, "[PHONE_1]") || !strings.Contains(out, "[EMAIL_1]") || !strings.Contains(out, "[REK_1]") {
		t.Fatalf("tokens missing: %s", out)
	}
	if back := m.Unmask(out); back != in {
		t.Fatalf("unmask mismatch:\n%s\n%s", back, in)
	}
}

func TestRouterMasksExternalPayloadAndLogs(t *testing.T) {
	var calls []Call
	r := NewRouter(func(c Call) { calls = append(calls, c) })
	p := &recordingProvider{}
	r.Register(p)
	r.SetRoute(Light, Route{Provider: "anthropic", Model: "claude-haiku-4-5"})
	var seen []string
	r.Payloads = func(_ string, req Request) {
		for _, m := range req.Messages {
			seen = append(seen, m.Content)
		}
	}
	var out struct{ Echo string }
	_, err := r.CompleteJSON(context.Background(), Request{Tier: Light, Purpose: "capture", Messages: []Message{{Role: "user", Content: "nomor saya 081234567890"}}}, &out)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range seen {
		if ContainsRawPhone(s) {
			t.Fatalf("raw phone reached provider: %s", s)
		}
	}
	if !strings.Contains(out.Echo, "081234567890") {
		t.Fatalf("response not unmasked: %s", out.Echo)
	}
	if len(calls) != 1 || calls[0].Purpose != "capture" || calls[0].InputHash == "" || calls[0].CostEst <= 0 {
		t.Fatalf("call log wrong: %+v", calls)
	}
}

func TestExtractJSON(t *testing.T) {
	if got := extractJSON("```json\n{\"a\":1}\n```"); got != `{"a":1}` {
		t.Fatal(got)
	}
	if got := extractJSON("Berikut: {\"a\":1} selesai"); got != `{"a":1}` {
		t.Fatal(got)
	}
}
