package mcp_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/mcp"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/testdb"
)

var now = clock.Fixed(time.Date(2026, 10, 5, 6, 45, 0, 0, clock.WIB))

type env struct {
	st   *store.Store
	o    *orchestrator.Orchestrator
	srv  *mcp.Server
	http *httptest.Server
}

func setup(t *testing.T) *env {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	o := &orchestrator.Orchestrator{St: st, Clock: now, Router: &llm.Router{Primary: llm.NewFake(nil), St: st, Log: log}, Log: log}
	s := mcp.New(st, now, log, o)
	h := httptest.NewServer(s.Handler())
	t.Cleanup(h.Close)
	return &env{st: st, o: o, srv: s, http: h}
}

func (e *env) token(t *testing.T, name string, scopes ...string) string {
	t.Helper()
	tok, _, err := mcp.CreateToken(context.Background(), e.st.Q, name, scopes, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (e *env) connect(t *testing.T, token string) *sdk.ClientSession {
	t.Helper()
	c := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1"}, nil)
	cs, err := c.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: e.http.URL, HTTPClient: &http.Client{Transport: bearer{token}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call returns the structured output (or the error text with ok=false).
func call(t *testing.T, cs *sdk.ClientSession, name string, args any) (map[string]any, string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var txt string
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			txt += tc.Text
		}
	}
	if res.IsError {
		return nil, txt, false
	}
	var out map[string]any
	if b, err := json.Marshal(res.StructuredContent); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out, txt, true
}

func lastCall(t *testing.T, st *store.Store, tool string) string {
	t.Helper()
	var status string
	if err := st.Pool.QueryRow(context.Background(), "select status from mcp_calls where tool = $1 order by created_at desc, id desc limit 1", tool).Scan(&status); err != nil {
		t.Fatalf("no mcp_calls row for %s: %v", tool, err)
	}
	return status
}

// Stage 07 acceptance: jadwal.due for Dewi, reanalyze a dealer (trigger mcp), read token cannot orchestrate,
// actions.decide is human-only.
func TestToolsAndScopes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	full := e.connect(t, e.token(t, "Claude Desktop Sam", "read", "analyze", "orchestrate"))

	tools, err := full.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) < 20 {
		t.Fatalf("tools: %d %v", len(tools.Tools), err)
	}

	out, _, ok := call(t, full, "jadwal.due", map[string]any{"sales": "Dewi"})
	if !ok || out["count"].(float64) < 2 {
		t.Fatalf("jadwal.due Dewi: %v", out)
	}
	for _, it := range out["items"].([]any) {
		if it.(map[string]any)["sales"] != "Dewi" {
			t.Fatalf("not Dewi's dealer: %v", it)
		}
	}

	out, txt, ok := call(t, full, "orchestrator.reanalyze", map[string]any{"scope": "dealer:mitra"})
	if !ok || out["status"] != "done" {
		t.Fatalf("reanalyze: %v %s", out, txt)
	}
	var trigger, scope, via, by string
	if err := e.st.Pool.QueryRow(ctx, "select trigger, scope, via, requested_by from cycles order by number desc limit 1").Scan(&trigger, &scope, &via, &by); err != nil {
		t.Fatal(err)
	}
	if trigger != "mcp" || scope != "dealer:mitra" || via != "mcp" || by != "Claude Desktop Sam" {
		t.Fatalf("cycle %s %s %s %s", trigger, scope, via, by)
	}
	var proposed int
	_ = e.st.Pool.QueryRow(ctx, "select count(*) from proposals p join dealers d on d.id = p.dealer_id where d.slug = 'mitra' and p.status = 'proposed'").Scan(&proposed)
	if proposed == 0 {
		t.Fatal("no proposed proposal for Mitra")
	}
	if lastCall(t, e.st, "orchestrator.reanalyze") != "ok" {
		t.Fatal("reanalyze not recorded")
	}

	read := e.connect(t, e.token(t, "ChatGPT tim sales", "read"))
	if _, txt, ok := call(t, read, "orchestrator.run", map[string]any{}); ok || !strings.Contains(txt, "forbidden") {
		t.Fatalf("read token ran the orchestrator: %s", txt)
	}
	if lastCall(t, e.st, "orchestrator.run") != "forbidden" {
		t.Fatal("forbidden call not recorded")
	}
	if _, txt, ok := call(t, full, "actions.decide", map[string]any{"proposal_id": uuid.NewString(), "decision": "approve"}); ok || !strings.Contains(txt, "human_only") {
		t.Fatalf("decide: %s", txt)
	}
	if lastCall(t, e.st, "actions.decide") != "human_only" {
		t.Fatal("human_only not recorded")
	}
	var audits int
	_ = e.st.Pool.QueryRow(ctx, "select count(*) from audit_log where actor_kind = 'mcp'").Scan(&audits)
	if audits < 4 {
		t.Fatalf("%d audit rows", audits)
	}
}

// Requests without a valid token never reach a tool.
func TestBearerRequired(t *testing.T) {
	e := setup(t)
	c := sdk.NewClient(&sdk.Implementation{Name: "x", Version: "1"}, nil)
	if _, err := c.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: e.http.URL, HTTPClient: &http.Client{Transport: bearer{"arc_bogus"}}, MaxRetries: -1}, nil); err == nil {
		t.Fatal("connected with an invalid token")
	}
	res, err := http.Post(e.http.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}

// chat.thread masks WhatsApp numbers when mask_pii_in_read is on (default).
func TestChatThreadMasked(t *testing.T) {
	e := setup(t)
	cs := e.connect(t, e.token(t, "Claude", "read"))
	out, txt, ok := call(t, cs, "chat.thread", map[string]any{"dealer_id": "sinar"})
	if !ok || out["masked"] != true || len(out["messages"].([]any)) == 0 {
		t.Fatalf("chat.thread: %v %s", out, txt)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "62819") || strings.Contains(string(b), "Mbak Rina") {
		t.Fatalf("PII leaked: %s", b)
	}
}

// 60 calls per minute per client.
func TestRateLimit(t *testing.T) {
	e := setup(t)
	cs := e.connect(t, e.token(t, "Bot", "read"))
	limited := false
	for i := 0; i < mcp.CallsPerMinute+1; i++ {
		if _, txt, ok := call(t, cs, "cycles.recent", map[string]any{"limit": 1}); !ok {
			limited = strings.Contains(txt, "rate_limited")
		}
	}
	if !limited || lastCall(t, e.st, "cycles.recent") != "rate_limited" {
		t.Fatal("rate limit not applied")
	}
}

// orchestrator.submit rejects provenance that is not in the Input.
func TestSubmitValidatesProvenance(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	rep, err := e.o.Run(ctx, domain.Scope{Kind: "all"}, domain.Trigger{Source: "schedule"})
	if err != nil {
		t.Fatal(err)
	}
	cs := e.connect(t, e.token(t, "Model eksternal", "orchestrate"))
	cid := rep.Cycle.ID.String()
	in, txt, ok := call(t, cs, "orchestrator.input.get", map[string]any{"cycle_id": cid, "agent": "AI Kredit"})
	if !ok || len(in["candidates"].([]any)) == 0 {
		t.Fatalf("input.get: %v %s", in, txt)
	}
	cand := in["candidates"].([]any)[0].(map[string]any)
	if strings.Contains(cand["preview"].(string), "Pak Hasan") {
		t.Fatal("Input is not masked")
	}
	bad := map[string]any{}
	for k, v := range cand {
		bad[k] = v
	}
	bad["signal_ids"] = []string{uuid.NewString()}
	bad["dedupe_key"] = "mcp:test:bad"
	if _, txt, ok := call(t, cs, "orchestrator.submit", map[string]any{"cycle_id": cid, "agent": "AI Kredit", "proposals": []any{bad}}); ok || !strings.Contains(txt, "tidak ada di Input") {
		t.Fatalf("foreign signal accepted: %s", txt)
	}
	good := map[string]any{}
	for k, v := range cand {
		good[k] = v
	}
	good["dedupe_key"] = "mcp:test:good"
	good["why"] = "Dari model eksternal: " + cand["why"].(string)
	out, txt, ok := call(t, cs, "orchestrator.submit", map[string]any{"cycle_id": cid, "agent": "AI Kredit", "proposals": []any{good}})
	if !ok || out["accepted"].(float64) != 1 || out["mode"] != "stored" {
		t.Fatalf("submit: %v %s", out, txt)
	}
	var preview, autonomy string
	if err := e.st.Pool.QueryRow(ctx, "select coalesce(preview, ''), autonomy from proposals where dedupe_key = 'mcp:test:good'").Scan(&preview, &autonomy); err != nil {
		t.Fatal(err)
	}
	if autonomy != "approve" || (preview != "" && strings.Contains(preview, "<PIC_")) {
		t.Fatalf("stored %q %s (placeholders must be unmasked, never auto)", preview, autonomy)
	}
}

// routing = mcp: Analisis waits for submissions; an agent nobody answered falls back to templates (partial).
func TestRoutingMCP(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.st.Pool.Exec(ctx, `update policies set value = jsonb_set(value, '{mode}', '"mcp"') where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	e.o.MCPWait = 4 * time.Second
	cs := e.connect(t, e.token(t, "Model eksternal", "orchestrate"))
	done := make(chan *orchestrator.Report, 1)
	go func() {
		rep, err := e.o.Run(ctx, domain.Scope{Kind: "agent", ID: "AI Kredit"}, domain.Trigger{Source: "mcp", By: "test"})
		if err != nil {
			t.Error(err)
		}
		done <- rep
	}()
	var cid string
	for i := 0; i < 50 && cid == ""; i++ {
		_ = e.st.Pool.QueryRow(ctx, "select cycle_id::text from cycle_inputs limit 1").Scan(&cid)
		time.Sleep(100 * time.Millisecond)
	}
	if cid == "" {
		t.Fatal("Analisis did not publish an Input")
	}
	in, txt, ok := call(t, cs, "orchestrator.input.get", map[string]any{"cycle_id": cid, "agent": "AI Kredit"})
	if !ok {
		t.Fatal(txt)
	}
	var props []any
	for i, c := range in["candidates"].([]any) {
		p := c.(map[string]any)
		p["why"] = "MCP: " + p["why"].(string)
		p["dedupe_key"] = "mcp:kredit:" + string(rune('a'+i))
		props = append(props, p)
	}
	out, txt, ok := call(t, cs, "orchestrator.submit", map[string]any{"cycle_id": cid, "agent": "AI Kredit", "proposals": props})
	if !ok || out["mode"] != "waiting" {
		t.Fatalf("submit: %v %s", out, txt)
	}
	rep := <-done
	if rep.Cycle.Status != "done" {
		t.Fatalf("cycle %s", rep.Cycle.Status)
	}
	var n int
	_ = e.st.Pool.QueryRow(ctx, "select count(*) from proposals where cycle_id = $1 and why like 'MCP: %' and payload->>'source' = 'mcp'", rep.Cycle.ID).Scan(&n)
	if n == 0 {
		t.Fatal("submitted proposals not stored by the cycle")
	}
}
