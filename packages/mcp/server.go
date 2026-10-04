// Package mcp implements ARC's MCP server (Streamable HTTP transport, JSON-RPC 2.0).
// Tools mirror the approved mockup. Write tools only create proposed Actions;
// arc.actions.decide refuses machine tokens; arc.policy.set is CEO-only.
// Authorization (OAuth 2.1 bearer tokens) is enforced by the HTTP layer.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// ProtocolVersion advertised to clients.
const ProtocolVersion = "2025-06-18"

// Group of tools that can be toggled in Pengaturan → AI & model.
type Group struct{ ID, Label, Note string }

// Tool describes one MCP tool.
type Tool struct {
	Name     string // wire name, e.g. arc_accounts_list
	Display  string // arc.accounts.list
	Group    string
	Kind     string // read | write | human-only | ceo
	Approval bool
	Short    string
	Desc     string
	Schema   map[string]any
}

func obj(props map[string]any, req ...string) map[string]any {
	if req == nil {
		req = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": req}
}

var s = map[string]any{"type": "string"}
var n = map[string]any{"type": "number"}

// Groups returns the tool groups in display order.
func Groups() []Group {
	return []Group{{"accounts", "Akun & relasi", ""}, {"deals", "Deal & pipeline", "sinkron Odoo"}, {"chat", "Chat & WhatsApp", ""}, {"cash", "Cash", ""}, {"actions", "Tindakan & kebijakan", ""}}
}

// Tools returns the registry.
func Tools() []Tool {
	return []Tool{
		{"arc_accounts_list", "arc.accounts.list", "accounts", "read", false, "", "Daftar akun aktif dengan health, nilai, owner, dan interaksi terakhir.", obj(map[string]any{"query": s})},
		{"arc_accounts_brief", "arc.accounts.brief", "accounts", "read", false, "memori akun + stakeholder map", "Brief akun: memori, stakeholder, komitmen, deal intelligence, langkah berikutnya, dan bukti.", obj(map[string]any{"account_id": s}, "account_id")},
		{"arc_accounts_memory_append", "arc.accounts.memory.append", "accounts", "write", false, "", "Tambahkan catatan ke memori akun (tercatat di audit log).", obj(map[string]any{"account_id": s, "note": s}, "account_id", "note")},
		{"arc_commitments_list", "arc.commitments.list", "accounts", "read", false, "", "Ledger komitmen dua arah (kami/mereka) per akun atau yang jatuh tempo.", obj(map[string]any{"account_id": s, "status": s})},
		{"arc_commitments_create", "arc.commitments.create", "accounts", "write", false, "", "Catat komitmen baru (kami/mereka) dengan sumber.", obj(map[string]any{"account_id": s, "who": map[string]any{"type": "string", "enum": []string{"kami", "mereka"}}, "text": s, "due": s, "source": s}, "account_id", "who", "text", "source")},
		{"arc_deals_list", "arc.deals.list", "deals", "read", false, "stage Odoo, health, sinyal", "Daftar opportunity: stage Odoo, nilai, health ARC, sinyal, probabilitas manual vs ARC.", obj(map[string]any{})},
		{"arc_deals_forecast", "arc.deals.forecast", "deals", "read", false, "", "Forecast kuartal berbasis bukti (commit/best/pipeline vs target), dengan what-if.", obj(map[string]any{"exclude": map[string]any{"type": "array", "items": s}})},
		{"arc_deals_probability_propose", "arc.deals.probability.propose", "deals", "write", false, "", "Usulkan penulisan probabilitas ARC ke Odoo (masuk antrean approval).", obj(map[string]any{"opportunity_id": s}, "opportunity_id")},
		{"arc_tenders_list", "arc.tenders.list", "deals", "read", false, "", "Tender radar: tender LPSE/e-katalog dengan skor kecocokan.", obj(map[string]any{})},
		{"arc_chat_threads", "arc.chat.threads", "chat", "read", false, "", "Daftar thread WhatsApp (pelanggan, grup eksternal/internal). Chat pribadi internal tidak pernah dibuka.", obj(map[string]any{"type": s})},
		{"arc_chat_read", "arc.chat.read", "chat", "read", false, "", "Baca pesan satu thread beserta anotasi ARC.", obj(map[string]any{"thread_id": s}, "thread_id")},
		{"arc_chat_reply_draft", "arc.chat.reply.draft", "chat", "write", true, "", "Buat draf balasan WhatsApp — selalu masuk antrean approval, tidak pernah dikirim langsung.", obj(map[string]any{"thread_id": s, "text": s}, "thread_id", "text")},
		{"arc_network_graph", "arc.network.graph(period)", "chat", "read", false, "", "Graf koneksi WhatsApp sales ↔ kontak untuk periode 1/2/3/6 bulan.", obj(map[string]any{"period": n, "sales": s})},
		{"arc_cash_l2c", "arc.cash.l2c", "cash", "read", false, "Won → Lunas", "Lead-to-cash per project: stage, hari vs benchmark, tindakan.", obj(map[string]any{})},
		{"arc_cash_aging", "arc.cash.aging", "cash", "read", false, "", "Umur piutang per bucket.", obj(map[string]any{})},
		{"arc_cash_forecast", "arc.cash.forecast", "cash", "read", false, "", "Prediksi kas masuk 30 hari tertimbang pola bayar (what-if create_invoice:<id>).", obj(map[string]any{"what_if": s})},
		{"arc_cash_reminder_draft", "arc.cash.reminder.draft", "cash", "write", true, "", "Draf pengingat pembayaran — masuk antrean approval.", obj(map[string]any{"account_id": s}, "account_id")},
		{"arc_actions_list", "arc.actions.list", "actions", "read", false, "saran menunggu keputusan", "Saran agen yang menunggu keputusan manusia.", obj(map[string]any{"status": s})},
		{"arc_actions_propose", "arc.actions.propose", "actions", "write", false, "AI mengusulkan, manusia memutuskan", "Usulkan tindakan baru; selalu berstatus proposed dan wajib membawa sumber.", obj(map[string]any{"account_id": s, "title": s, "why": s, "preview": s, "type": s, "source": s}, "title", "why", "source")},
		{"arc_actions_decide", "arc.actions.decide", "actions", "human-only", false, "hanya pengguna manusia", "Putuskan saran — DITOLAK untuk token mesin/klien AI.", obj(map[string]any{"action_id": s, "decision": s, "reason": s}, "action_id", "decision")},
		{"arc_policy_get", "arc.policy.get", "actions", "ceo", false, "", "Baca policy bisnis (diskon, kredit, ambang sunyi, benchmark).", obj(map[string]any{"key": s})},
		{"arc_policy_set", "arc.policy.set", "actions", "ceo", false, "", "Ubah policy — hanya CEO.", obj(map[string]any{"key": s, "value": map[string]any{}}, "key", "value")},
		{"arc_prospects_list", "arc.prospects.list", "deals", "read", false, "", "Nomor/kontak baru masuk beserta skor identifikasi.", obj(map[string]any{})},
		{"arc_prospects_brief", "arc.prospects.brief", "deals", "read", false, "", "Detail identifikasi, overview, tangga solusi, dan pertanyaan pain point satu prospek.", obj(map[string]any{"prospect_id": s}, "prospect_id")},
	}
}

// Caller executes a tool for the authenticated principal in ctx.
type Caller func(ctx context.Context, tool string, args map[string]any) (any, error)

// Server handles MCP JSON-RPC.
type Server struct {
	Version string
	Call    Caller
	Enabled func(ctx context.Context) map[string]bool
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ErrForbidden marks authorization failures (returned as tool errors).
var ErrForbidden = errors.New("forbidden")

func (sv *Server) toolByName(ctx context.Context, name string) (Tool, bool) {
	en := map[string]bool{}
	if sv.Enabled != nil {
		en = sv.Enabled(ctx)
	}
	for _, t := range Tools() {
		if t.Name == name || t.Display == name {
			if on, ok := en[t.Group]; ok && !on {
				return t, false
			}
			return t, true
		}
	}
	return Tool{}, false
}

func (sv *Server) handle(ctx context.Context, req rpcReq) (any, *rpcErr) {
	switch req.Method {
	case "initialize":
		return map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":   map[string]any{"name": "arc", "title": "ARC Relationship Core", "version": sv.Version},
			"instructions": "ARC adalah CRM AI-native GSI. Tool tulis hanya membuat usulan (proposed) yang harus diputuskan manusia di ARC; tidak ada pengiriman langsung ke pelanggan."}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		en := map[string]bool{}
		if sv.Enabled != nil {
			en = sv.Enabled(ctx)
		}
		list := []map[string]any{}
		for _, t := range Tools() {
			if on, ok := en[t.Group]; ok && !on {
				continue
			}
			ann := map[string]any{"readOnlyHint": t.Kind == "read", "title": t.Display}
			list = append(list, map[string]any{"name": t.Name, "title": t.Display, "description": t.Desc, "inputSchema": t.Schema, "annotations": ann})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcErr{-32602, "invalid params"}
		}
		t, ok := sv.toolByName(ctx, p.Name)
		if !ok {
			return nil, &rpcErr{-32602, "tool tidak dikenal atau dinonaktifkan: " + p.Name}
		}
		if p.Arguments == nil {
			p.Arguments = map[string]any{}
		}
		res, err := sv.Call(ctx, t.Name, p.Arguments)
		if err != nil {
			return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}, nil
		}
		raw, _ := json.Marshal(res)
		out := map[string]any{"content": []map[string]any{{"type": "text", "text": string(raw)}}, "isError": false}
		if m, ok := res.(map[string]any); ok {
			out["structuredContent"] = m
		}
		return out, nil
	}
	if strings.HasPrefix(req.Method, "notifications/") {
		return nil, nil
	}
	return nil, &rpcErr{-32601, "method not found: " + req.Method}
}

// ServeHTTP implements the Streamable HTTP transport (POST JSON-RPC; GET is not offered).
func (sv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Allow", "POST")
		http.Error(w, "SSE stream tidak disediakan; gunakan POST", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if sid := r.Header.Get("Mcp-Session-Id"); sid != "" {
		w.Header().Set("Mcp-Session-Id", sid)
	} else {
		w.Header().Set("Mcp-Session-Id", "arc-session")
	}
	trim := strings.TrimSpace(string(body))
	var reqs []rpcReq
	batch := strings.HasPrefix(trim, "[")
	if batch {
		if err := json.Unmarshal(body, &reqs); err != nil {
			writeRPC(w, []any{map[string]any{"jsonrpc": "2.0", "id": nil, "error": rpcErr{-32700, "parse error"}}})
			return
		}
	} else {
		var one rpcReq
		if err := json.Unmarshal(body, &one); err != nil {
			writeRPC(w, map[string]any{"jsonrpc": "2.0", "id": nil, "error": rpcErr{-32700, "parse error"}})
			return
		}
		reqs = []rpcReq{one}
	}
	var out []any
	for _, q := range reqs {
		res, e := sv.handle(r.Context(), q)
		if len(q.ID) == 0 {
			continue // notification
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": q.ID}
		if e != nil {
			resp["error"] = e
		} else {
			resp["result"] = res
		}
		out = append(out, resp)
	}
	if len(out) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if batch {
		writeRPC(w, out)
		return
	}
	writeRPC(w, out[0])
}

func writeRPC(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
