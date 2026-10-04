package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"arc/packages/connectors/whatsapp"
	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

type rpcResult struct {
	Result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		IsError           bool             `json:"isError"`
		Content           []map[string]any `json:"content"`
		StructuredContent map[string]any   `json:"structuredContent"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *client) mcp(method string, params any) rpcResult {
	c.t.Helper()
	var out rpcResult
	c.json("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}, 200, &out)
	return out
}

func mcpText(r rpcResult) string {
	if len(r.Result.Content) == 0 {
		return ""
	}
	s, _ := r.Result.Content[0]["text"].(string)
	return s
}

// Stage 06: MCP tools are registered; the account brief carries evidence; decide is
// refused for machine tokens.
func TestStage06MCPToolsBriefAndDecide(t *testing.T) {
	a, srv := fresh(t)
	sam := login(t, srv, "sam@gsi.co.id")
	key := sam.apiKey("Claude Desktop", domain.ScopeRead, domain.ScopePropose)
	m := bearerClient(t, srv, key)

	init := m.mcp("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	if init.Error != nil {
		t.Fatalf("initialize: %+v", init.Error)
	}
	list := m.mcp("tools/list", map[string]any{})
	names := map[string]bool{}
	for _, tl := range list.Result.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"arc_accounts_brief", "arc_accounts_list", "arc_actions_propose", "arc_actions_decide", "arc_deals_forecast", "arc_cash_l2c"} {
		if !names[want] {
			t.Fatalf("tool %s not listed (%d tools)", want, len(names))
		}
	}

	brief := m.mcp("tools/call", map[string]any{"name": "arc_accounts_brief", "arguments": map[string]any{"account_id": "rsud"}})
	if brief.Result.IsError {
		t.Fatalf("brief error: %s", mcpText(brief))
	}
	sc := brief.Result.StructuredContent
	if sc["name"] != "RSUD Kota Yogyakarta" {
		t.Fatalf("brief for wrong account: %v", sc["name"])
	}
	mem, _ := sc["memory"].(map[string]any)
	prov, _ := mem["provenance"].([]any)
	if s, _ := mem["text"].(string); s == "" || len(prov) == 0 {
		t.Fatalf("brief memory without provenance: %v", mem)
	}
	deal, _ := sc["deal"].(map[string]any)
	flags, _ := deal["flags"].([]any)
	if len(flags) == 0 {
		t.Fatalf("brief without deal evidence (signals): %v", deal)
	}
	ev, _ := sc["evidence"].([]any)
	if len(ev) == 0 {
		t.Fatalf("brief carries no evidence list: keys %v", keys(sc))
	}
	for _, e := range ev {
		em, _ := e.(map[string]any)
		if em["quote"] == nil || em["quote"] == "" || (em["interaction_id"] == nil && em["source"] == nil && em["document_id"] == nil) {
			t.Fatalf("evidence entry without source/quote: %v", em)
		}
	}

	decide := m.mcp("tools/call", map[string]any{"name": "arc_actions_decide", "arguments": map[string]any{"action_id": "unmer", "decision": "approve"}})
	if !decide.Result.IsError || !strings.Contains(mcpText(decide), "manusia") {
		t.Fatalf("machine decide must be refused: %+v", decide.Result)
	}
	if st := actionStatus(t, a, "unmer"); st != "proposed" {
		t.Fatalf("unmer status changed to %s by a machine token", st)
	}
	if a.FakeWA.SentCount() != 0 {
		t.Fatal("machine token caused a send")
	}
	// A machine may propose, and the proposal waits for a human.
	prop := m.mcp("tools/call", map[string]any{"name": "arc_actions_propose", "arguments": map[string]any{"account_id": "rsud", "title": "Cek jadwal rapat", "why": "Diminta klien AI", "source": "mcp:test"}})
	if prop.Result.IsError {
		t.Fatalf("propose: %s", mcpText(prop))
	}
	id, _ := prop.Result.StructuredContent["action_id"].(string)
	if st := actionStatus(t, a, id); st != "proposed" {
		t.Fatalf("MCP proposal status %s", st)
	}
	noSrc := m.mcp("tools/call", map[string]any{"name": "arc_actions_propose", "arguments": map[string]any{"title": "Tanpa bukti", "why": "x"}})
	if !noSrc.Result.IsError {
		t.Fatal("proposal without provenance accepted")
	}
	// Read-only key cannot use write tools.
	ro := bearerClient(t, srv, sam.apiKey("Read only", domain.ScopeRead))
	if r := ro.mcp("tools/call", map[string]any{"name": "arc_actions_propose", "arguments": map[string]any{"title": "x", "why": "y", "source": "z"}}); !r.Result.IsError {
		t.Fatal("read-only key could propose")
	}
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func actionStatus(t *testing.T, a *App, id string) string {
	t.Helper()
	var s string
	if err := a.DB.Pool.QueryRow(context.Background(), `SELECT status FROM actions WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatalf("action %s: %v", id, err)
	}
	return s
}

// Stage 06: a Semarang sales token cannot read a Yogyakarta account (UI, /api/v1, MCP).
func TestStage06BranchScope(t *testing.T) {
	_, srv := fresh(t)
	andi := login(t, srv, "andi@gsi.co.id")
	if code, _ := andi.do("GET", "/api/accounts/semarang", nil); code != 200 {
		t.Fatalf("own branch account → %d", code)
	}
	if code, body := andi.do("GET", "/api/accounts/rsud", nil); code != 403 && code != 404 {
		t.Fatalf("Andi read Yogyakarta account rsud: %d %s", code, body)
	}
	key := andi.apiKey("Andi agent", domain.ScopeRead)
	m := bearerClient(t, srv, key)
	if code, _ := m.do("GET", "/api/v1/accounts/rsud/brief", nil); code != 403 && code != 404 {
		t.Fatalf("Andi key read rsud via /api/v1: %d", code)
	}
	if code, _ := m.do("GET", "/api/v1/accounts/semarang/brief", nil); code != 200 {
		t.Fatalf("Andi key own account via /api/v1: %d", code)
	}
	r := m.mcp("tools/call", map[string]any{"name": "arc_accounts_brief", "arguments": map[string]any{"account_id": "rsud"}})
	if !r.Result.IsError {
		t.Fatal("Andi key read rsud via MCP")
	}
	// Lists are scoped too: no Yogyakarta deal in Andi's accounts list.
	var accs struct {
		Items []map[string]any `json:"items"`
	}
	andi.json("GET", "/api/accounts", nil, 200, &accs)
	if len(accs.Items) == 0 {
		t.Fatal("Andi sees no accounts")
	}
	for _, x := range accs.Items {
		if x["id"] == "rsud" || x["id"] == "bsd" || x["id"] == "sleman" {
			t.Fatalf("Andi's account list contains %v", x["id"])
		}
	}
}

// Stage 06: POST /api/ask "Deal mana yang berisiko?" answers with ≥ 2 evidence ids that exist.
func TestStage06AskEvidence(t *testing.T) {
	a, srv := fresh(t)
	var ans struct {
		Paragraphs  []string `json:"paragraphs"`
		EvidenceIDs []string `json:"evidence_ids"`
	}
	login(t, srv, "sam@gsi.co.id").json("POST", "/api/ask", map[string]string{"question": "Deal mana yang berisiko?"}, 200, &ans)
	if len(ans.Paragraphs) == 0 || len(ans.EvidenceIDs) < 2 {
		t.Fatalf("answer: %+v", ans)
	}
	for _, id := range ans.EvidenceIDs {
		var ok bool
		switch {
		case strings.HasPrefix(id, "signal:"):
			n, _ := strconv.Atoi(strings.TrimPrefix(id, "signal:"))
			ok = count(t, a, `SELECT count(*) FROM signals WHERE id=$1`, n) == 1
		case strings.HasPrefix(id, "interaction:"):
			n, _ := strconv.Atoi(strings.TrimPrefix(id, "interaction:"))
			ok = count(t, a, `SELECT count(*) FROM interactions WHERE id=$1`, n) == 1
		default:
			ok = count(t, a, `SELECT count(*) FROM opportunities WHERE id=$1`, id)+count(t, a, `SELECT count(*) FROM accounts WHERE id=$1`, id) > 0
		}
		if !ok {
			t.Fatalf("evidence id %q does not exist in the seed", id)
		}
	}
}

// Stage 06: a new Action fires the outgoing webhook with a valid HMAC signature.
func TestStage06OutgoingWebhookSignature(t *testing.T) {
	a, srv := fresh(t)
	type delivery struct {
		body []byte
		sig  string
		evt  string
	}
	var mu sync.Mutex
	got := make(chan delivery, 4)
	recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		got <- delivery{b, r.Header.Get("X-ARC-Signature"), r.Header.Get("X-ARC-Event")}
		w.WriteHeader(204)
	}))
	defer recv.Close()
	var sub struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	login(t, srv, "sam@gsi.co.id").json("POST", "/api/webhooks/subscriptions", map[string]any{"url": recv.URL + "/hook", "events": []string{"action.proposed"}}, 200, &sub)
	if sub.Secret == "" {
		t.Fatal("no webhook secret returned")
	}
	id, created, err := a.Actions.Propose(context.Background(), actions.Proposal{Agent: "Follow-up agent", Type: "send_wa", AccountID: "semarang", Title: "Test webhook",
		Evidence: []domain.Evidence{{Source: "test", Quote: "bukti"}}}, storage.Actor{ID: "test", Type: "agent"})
	if err != nil || !created {
		t.Fatalf("propose: %v %v", created, err)
	}
	select {
	case d := <-got:
		if d.evt != "action.proposed" || !whatsapp.Verify(sub.Secret, d.body, d.sig) || !strings.HasPrefix(d.sig, "sha256=") {
			t.Fatalf("bad delivery: event=%s sig=%s", d.evt, d.sig)
		}
		if whatsapp.Verify("other-secret", d.body, d.sig) {
			t.Fatal("signature verifies with the wrong secret")
		}
		var payload struct {
			Event string         `json:"event"`
			Data  map[string]any `json:"data"`
		}
		_ = json.Unmarshal(d.body, &payload)
		if payload.Data["id"] != id {
			t.Fatalf("payload for wrong action: %s", d.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no webhook delivered")
	}
	// The delivery is logged (async) — wait briefly.
	deadline := time.Now().Add(3 * time.Second)
	for count(t, a, `SELECT count(*) FROM webhook_deliveries WHERE subscription_id=$1 AND status_code=204`, sub.ID) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("delivery not logged")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
