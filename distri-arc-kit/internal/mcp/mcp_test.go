package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
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

// answerInputs plays Claude through MCP until the cycle is over (a value on done, put back for the caller): each
// agent's published Input is answered with its own candidates.
func answerInputs[T any](t *testing.T, e *env, cs *sdk.ClientSession, done chan T) {
	t.Helper()
	ctx := context.Background()
	answered := map[string]bool{}
	for i := 0; i < 150; i++ {
		select {
		case v := <-done:
			done <- v
			return
		case <-time.After(100 * time.Millisecond):
		}
		rows, err := e.st.Pool.Query(ctx, "select cycle_id::text, agent from cycle_inputs where submitted_at is null")
		if err != nil {
			t.Fatal(err)
		}
		var todo [][2]string
		for rows.Next() {
			var cid, agent string
			_ = rows.Scan(&cid, &agent)
			if !answered[cid+agent] {
				todo = append(todo, [2]string{cid, agent})
			}
		}
		rows.Close()
		for _, x := range todo {
			answered[x[0]+x[1]] = true
			in, txt, ok := call(t, cs, "orchestrator_input_get", map[string]any{"cycle_id": x[0], "agent": x[1]})
			if !ok {
				t.Fatalf("input %s: %s", x[1], txt)
			}
			props := []any{}
			cands, _ := in["candidates"].([]any)
			for j, c := range cands {
				p := c.(map[string]any)
				p["dedupe_key"] = fmt.Sprintf("mcp:%s:%d", x[1], j)
				props = append(props, p)
			}
			if _, txt, ok := call(t, cs, "orchestrator_submit", map[string]any{"cycle_id": x[0], "agent": x[1], "proposals": props}); !ok {
				t.Fatalf("submit %s: %s", x[1], txt)
			}
		}
	}
	var st string
	_ = e.st.Pool.QueryRow(ctx, "select status || coalesce(' ' || (select string_agg(agent || '=' || (submitted_at is not null)::text, ',') from cycle_inputs), '') from cycles order by number desc limit 1").Scan(&st)
	t.Fatal("the cycle did not finish: " + st)
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

	out, _, ok := call(t, full, "jadwal_due", map[string]any{"sales": "Dewi"})
	if !ok || out["total"].(float64) < 2 {
		t.Fatalf("jadwal.due Dewi: %v", out)
	}
	// Claude accepts tool names of letters, digits, _ and - only
	for _, tl := range tools.Tools {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(tl.Name) {
			t.Fatalf("tool name %q not accepted by Claude", tl.Name)
		}
	}
	// lists are paged: a page, the total, where the next page starts
	pg, _, ok := call(t, full, "dealer_list", map[string]any{"limit": 5, "sort": "omzet"})
	if !ok || len(pg["items"].([]any)) != 5 || pg["total"].(float64) != 18 || pg["next_offset"].(float64) != 5 {
		t.Fatalf("dealer_list page: total %v next %v", pg["total"], pg["next_offset"])
	}
	first := pg["items"].([]any)[0].(map[string]any)["omzet_bln"].(float64)
	second := pg["items"].([]any)[1].(map[string]any)["omzet_bln"].(float64)
	if first < second {
		t.Fatal("dealer_list not sorted by omzet")
	}
	// the whole book in one call, and the analysis tools behind it
	sum, _, ok := call(t, full, "data_ringkasan", map[string]any{})
	if !ok || sum["dealer"].(map[string]any)["total"].(float64) != 18 || len(sum["dealer_terbesar"].([]any)) != 10 || sum["kpi"] == nil || sum["stok"] == nil {
		t.Fatalf("data_ringkasan: %v", sum)
	}
	if m, _, ok := call(t, full, "penjualan_bulanan", map[string]any{"months": 6}); !ok || len(m["months"].([]any)) != 6 {
		t.Fatalf("penjualan_bulanan: %v", m)
	}
	if p, _, ok := call(t, full, "produk_terlaris", map[string]any{"days": 90, "limit": 5}); !ok || len(p["items"].([]any)) == 0 {
		t.Fatalf("produk_terlaris: %v", p)
	}
	if a, _, ok := call(t, full, "piutang_ringkas", map[string]any{"limit": 3}); !ok || a["ringkasan"] == nil {
		t.Fatalf("piutang_ringkas: %v", a)
	}
	for _, it := range out["items"].([]any) {
		if it.(map[string]any)["sales"] != "Dewi" {
			t.Fatalf("not Dewi's dealer: %v", it)
		}
	}

	// A cycle Claude asks for is analysed by Claude through MCP (ADR 0025): without a job queue it runs inside the
	// tool call, so a second session plays Claude answering each agent's Input while the first one waits.
	type reply struct {
		out map[string]any
		txt string
		ok  bool
	}
	got := make(chan reply, 1)
	go func() {
		res, err := full.CallTool(ctx, &sdk.CallToolParams{Name: "orchestrator_reanalyze", Arguments: map[string]any{"scope": "dealer:mitra"}})
		if err != nil {
			got <- reply{txt: err.Error()}
			return
		}
		var r reply
		r.ok = !res.IsError
		for _, c := range res.Content {
			if tc, ok := c.(*sdk.TextContent); ok {
				r.txt += tc.Text
			}
		}
		b, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(b, &r.out)
		got <- r
	}()
	answerInputs(t, e, e.connect(t, e.token(t, "Claude analis", "read", "analyze", "orchestrate")), got)
	r := <-got
	out, txt, ok := r.out, r.txt, r.ok
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
	if lastCall(t, e.st, "orchestrator_reanalyze") != "ok" {
		t.Fatal("reanalyze not recorded")
	}

	read := e.connect(t, e.token(t, "ChatGPT tim sales", "read"))
	if _, txt, ok := call(t, read, "orchestrator_run", map[string]any{}); ok || !strings.Contains(txt, "forbidden") {
		t.Fatalf("read token ran the orchestrator: %s", txt)
	}
	if lastCall(t, e.st, "orchestrator_run") != "forbidden" {
		t.Fatal("forbidden call not recorded")
	}
	if _, txt, ok := call(t, full, "actions_decide", map[string]any{"proposal_id": uuid.NewString(), "decision": "approve"}); ok || !strings.Contains(txt, "human_only") {
		t.Fatalf("decide: %s", txt)
	}
	if lastCall(t, e.st, "actions_decide") != "human_only" {
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
	out, txt, ok := call(t, cs, "chat_thread", map[string]any{"dealer_id": "sinar"})
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
		if _, txt, ok := call(t, cs, "cycles_recent", map[string]any{"limit": 1}); !ok {
			limited = strings.Contains(txt, "rate_limited")
		}
	}
	if !limited || lastCall(t, e.st, "cycles_recent") != "rate_limited" {
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
	in, txt, ok := call(t, cs, "orchestrator_input_get", map[string]any{"cycle_id": cid, "agent": "AI Kredit"})
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
	if _, txt, ok := call(t, cs, "orchestrator_submit", map[string]any{"cycle_id": cid, "agent": "AI Kredit", "proposals": []any{bad}}); ok || !strings.Contains(txt, "tidak ada di Input") {
		t.Fatalf("foreign signal accepted: %s", txt)
	}
	good := map[string]any{}
	for k, v := range cand {
		good[k] = v
	}
	good["dedupe_key"] = "mcp:test:good"
	good["why"] = "Dari model eksternal: " + cand["why"].(string)
	out, txt, ok := call(t, cs, "orchestrator_submit", map[string]any{"cycle_id": cid, "agent": "AI Kredit", "proposals": []any{good}})
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
	in, txt, ok := call(t, cs, "orchestrator_input_get", map[string]any{"cycle_id": cid, "agent": "AI Kredit"})
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
	out, txt, ok := call(t, cs, "orchestrator_submit", map[string]any{"cycle_id": cid, "agent": "AI Kredit", "proposals": props})
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

// wa.analisis reads the last cycle's Analisa WhatsApp (ADR 0026) with the people's names masked.
func TestWAAnalisisMasked(t *testing.T) {
	e := setup(t)
	cs := e.connect(t, e.token(t, "Claude", "read"))
	if out, _, ok := call(t, cs, "wa_analisis", map[string]any{}); !ok || out["catatan"] == nil || out["percakapan"] != nil {
		t.Fatalf("before a cycle: %v", out)
	}
	if _, err := e.o.Run(context.Background(), domain.Scope{Kind: "all"}, domain.Trigger{Source: "schedule"}); err != nil {
		t.Fatal(err)
	}
	out, txt, ok := call(t, cs, "wa_analisis", map[string]any{})
	if !ok || out["masked"] != true || len(out["percakapan"].([]any)) == 0 {
		t.Fatalf("wa.analisis: %v %s", out, txt)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "Pak Hasan") || strings.Contains(string(b), "Mbak Rina") {
		t.Fatalf("PII leaked: %s", b)
	}
}

// Claude (MCP, no API key) reads the chats of a cycle and sends the Analisa WhatsApp lines back (ADR 0026).
func TestWAThroughMCP(t *testing.T) {
	e := setup(t)
	rep, err := e.o.Run(context.Background(), domain.Scope{Kind: "all"}, domain.Trigger{Source: "schedule"})
	if err != nil {
		t.Fatal(err)
	}
	cs := e.connect(t, e.token(t, "Claude", "read", "analyze", "orchestrate"))
	cid := rep.Cycle.ID.String()
	in, txt, ok := call(t, cs, "orchestrator_input_get", map[string]any{"cycle_id": cid, "agent": "Analisa WhatsApp"})
	if !ok || len(in["threads"].([]any)) == 0 {
		t.Fatalf("input: %v %s", in, txt)
	}
	id := in["threads"].([]any)[0].(map[string]any)["id"].(string)
	out, txt, ok := call(t, cs, "orchestrator_submit", map[string]any{"cycle_id": cid, "agent": "Analisa WhatsApp", "overview": "Dua dealer menunggu balasan.",
		"threads": []map[string]any{{"id": id, "summary": "Menunggu konfirmasi PO dari sales.", "confidence": 0.8}}})
	if !ok || out["accepted"].(float64) != 1 {
		t.Fatalf("submit: %v %s", out, txt)
	}
	// a client with the older tool schema sends it inside proposals
	out, txt, ok = call(t, cs, "orchestrator_submit", map[string]any{"cycle_id": cid, "agent": "Analisa WhatsApp", "proposals": []map[string]any{
		{"overview": "Dua dealer menunggu balasan."}, {"id": id, "summary": "Menunggu konfirmasi PO.", "confidence": 0.7}}})
	if !ok || out["accepted"].(float64) != 1 {
		t.Fatalf("submit in proposals: %v %s", out, txt)
	}
	a, _ := e.st.Q.LatestWAAnalysis(context.Background())
	if a.Source != "ai" || a.Overview == nil || *a.Overview != "Dua dealer menunggu balasan." {
		t.Fatalf("stored %s %v", a.Source, a.Overview)
	}
}

// "Nothing to propose" is an answer: an empty list is accepted while the cycle waits, the agent is done (not failed
// after MCPWait under require_ai) and the cycle finishes at once.
func TestRoutingMCPEmptyAnswer(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.st.Pool.Exec(ctx, `update policies set value = jsonb_set(value, '{mode}', '"mcp"') where key = 'llm.routing'`); err != nil {
		t.Fatal(err)
	}
	e.o.MCPWait = 30 * time.Second
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
	start := time.Now()
	out, txt, ok := call(t, cs, "orchestrator_submit", map[string]any{"cycle_id": cid, "agent": "AI Kredit", "proposals": []any{}})
	if !ok || out["mode"] != "waiting" || out["accepted"] != float64(0) {
		t.Fatalf("empty submit: %v %s", out, txt)
	}
	rep := <-done
	if rep == nil || rep.Cycle.Status != "done" || time.Since(start) > 10*time.Second {
		t.Fatalf("cycle %+v after %s", rep, time.Since(start))
	}
	var status string
	_ = e.st.Pool.QueryRow(ctx, "select status from agent_runs where cycle_id = $1 and agent = 'AI Kredit'", rep.Cycle.ID).Scan(&status)
	if status != "done" {
		t.Fatalf("agent run %q", status)
	}
}
