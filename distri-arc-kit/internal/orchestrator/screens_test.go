package orchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
)

type screenInput struct {
	Items []map[string]any `json:"items"`
}

// Analisa Stok and Analisa Kas (ADR 0030): the page readings are written by Claude through MCP, checked against the
// Input it was given (keys, lengths), unmasked and stored as AI; GSI Orbit writes none itself.
func TestScreenReadingsThroughMCP(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	rep, err := o.Run(ctx, all, sched)
	if err != nil {
		t.Fatal(err)
	}
	cid := rep.Cycle.ID
	if _, err := st.Q.LatestScreenAnalysis(ctx, "stock"); err == nil {
		t.Fatal("no AI answered: nothing may be stored for the page")
	}

	raw, err := o.InputFor(ctx, cid, orchestrator.AgentStockRead)
	if err != nil {
		t.Fatal(err)
	}
	var in screenInput
	_ = json.Unmarshal(raw, &in)
	if len(in.Items) == 0 || in.Items[0]["key"] == nil || in.Items[0]["age_days"] == nil {
		t.Fatalf("stock input %s", raw)
	}
	key := in.Items[0]["key"].(string)
	ans := orchestrator.ParseScreenAnswer("Stok menua terbesar ada di Semarang; dorong ke dealer yang jadwal ordernya minggu ini.",
		[]map[string]any{{"key": key, "note": "Tawarkan ke dua dealer dengan jadwal order terdekat minggu ini.", "confidence": 0.8}})
	if _, err := o.ScreenSubmit(ctx, cid, orchestrator.AgentStockRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	a, err := st.Q.LatestScreenAnalysis(ctx, "stock")
	if err != nil || a.Source != "ai" || a.CycleID != cid || !strings.Contains(string(a.Lines), key) || !strings.Contains(string(a.Lines), `"confidence": 80`) && !strings.Contains(string(a.Lines), `"confidence":80`) {
		t.Fatalf("stored %+v %s %v", a, a.Lines, err)
	}
	bad := orchestrator.ParseScreenAnswer("x", []map[string]any{{"key": "not-in-input", "note": "y", "confidence": 0.5}})
	if _, err := o.ScreenSubmit(ctx, cid, orchestrator.AgentStockRead, bad, nil); !errors.Is(err, orchestrator.ErrBadSubmit) {
		t.Fatalf("unknown item accepted: %v", err)
	}

	// Analisa Kas: the sales' names leave the server masked, and come back unmasked
	raw, err = o.InputFor(ctx, cid, orchestrator.AgentCashRead)
	if err != nil {
		t.Fatal(err)
	}
	in = screenInput{}
	_ = json.Unmarshal(raw, &in)
	if len(in.Items) == 0 {
		t.Fatalf("cash input %s", raw)
	}
	sales, _ := in.Items[0]["sales"].(string)
	if !strings.HasPrefix(sales, "<PIC_") {
		t.Fatalf("sales name not masked: %q", sales)
	}
	ans = orchestrator.ParseScreenAnswer("Kas 30 hari bergantung pada tiga dealer besar.",
		[]map[string]any{{"key": in.Items[0]["key"], "note": sales + " menagih invoice terlama minggu ini.", "confidence": 0.7}})
	if _, err := o.ScreenSubmit(ctx, cid, orchestrator.AgentCashRead, ans, nil); err != nil {
		t.Fatal(err)
	}
	k, err := st.Q.LatestScreenAnalysis(ctx, "credit")
	if err != nil || strings.Contains(string(k.Lines), "<PIC_") {
		t.Fatalf("cash reading %s %v", k.Lines, err)
	}
}

// The Analisis ulang prompt asks Claude for the page readings of the agents in scope.
func TestMCPPromptAsksForPageReadings(t *testing.T) {
	p := orchestrator.MCPPrompt(7, "c1", []string{"AI Stok"}, 0)
	if !strings.Contains(p, `"agent": "Analisa Stok"`) || strings.Contains(p, "Analisa Kas") {
		t.Errorf("stock prompt:\n%s", p)
	}
	p = orchestrator.MCPPrompt(7, "c1", []string{"AI Kredit", "AI Penagihan"}, 0)
	if !strings.Contains(p, `"agent": "Analisa Kas"`) || strings.Contains(p, "Analisa Stok") {
		t.Errorf("credit prompt:\n%s", p)
	}
}

// screenStub answers the page readings like a model would (every other purpose: its fallback).
type screenStub struct{ purposes []string }

func (m *screenStub) Name() string  { return "stub" }
func (m *screenStub) Model() string { return "claude-sonnet-5-5" }
func (m *screenStub) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	if !strings.HasPrefix(r.Purpose, "screen.") {
		return llm.Response{JSON: r.Fallback, Provider: "stub", Model: "claude-sonnet-5-5"}, nil
	}
	m.purposes = append(m.purposes, r.Purpose)
	var in screenInput
	_ = json.Unmarshal(r.Input, &in)
	var lines []map[string]any
	for _, it := range in.Items {
		lines = append(lines, map[string]any{"key": it["key"], "note": "Langkah berikutnya untuk sales.", "confidence": 0.6})
	}
	b, _ := json.Marshal(map[string]any{"overview": "Ringkasan untuk CEO.", "lines": lines})
	return llm.Response{JSON: b, Provider: "stub", Model: "claude-sonnet-5-5"}, nil
}

// With the server's model, a screen cycle writes that page's reading as AI; a cycle sent to MCP leaves it to Claude.
func TestScreenReadingByModel(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	stub := &screenStub{}
	o.Router = &llm.Router{Primary: stub, St: st, Log: o.Log}
	if _, err := o.Run(ctx, domain.Scope{Kind: "screen", ID: "stock"}, sched); err != nil {
		t.Fatal(err)
	}
	if len(stub.purposes) != 1 || stub.purposes[0] != "screen.stock" {
		t.Fatalf("model asked for %v", stub.purposes)
	}
	if a, err := st.Q.LatestScreenAnalysis(ctx, "stock"); err != nil || a.Overview != "Ringkasan untuk CEO." {
		t.Fatalf("stock reading %+v %v", a, err)
	}
	if _, err := st.Q.LatestScreenAnalysis(ctx, "credit"); err == nil {
		t.Error("a stock cycle wrote the cash reading")
	}
}
