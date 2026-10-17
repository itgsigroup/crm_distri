package api_test

import (
	"context"
	"testing"
)

// Pusat kendali shows what the AI is analysing: an agent whose Input Claude opened is "reading", one it sent is
// "done" with what it covered, the rest wait; the AI's tool calls on the cycle are listed newest first in words.
func TestCycleProgress(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var id string
	if err := st.Pool.QueryRow(ctx, `insert into cycles (trigger, scope, via, status) values ('schedule', 'all', 'mcp', 'running') returning id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into cycle_stages (cycle_id, stage, status, detail) values
		($1, 'ingest', 'done', '{"text":"WhatsApp 963 chat · 288 menunggu"}'), ($1, 'analyze', 'running', null)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into cycle_inputs (cycle_id, agent, input, signal_ids, submitted_at) values
		($1, 'AI Kredit', '{"dealers":[{},{},{}],"messages":[{}]}', '{}', now()),
		($1, 'AI Stok', '{"dealers":[{}],"stock":[{},{}]}', '{}', null),
		($1, 'AI Order', '{"dealers":[]}', '{}', null)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into mcp_calls (tool, args, result_summary, cycle_id, created_at) values
		('orchestrator_input_get', jsonb_build_object('agent', 'AI Kredit', 'cycle_id', $1::text), 'AI Kredit', $1::uuid, now() - interval '3 minutes'),
		('orchestrator_submit', jsonb_build_object('agent', 'AI Kredit', 'cycle_id', $1::text), 'AI Kredit · 2 saran', $1::uuid, now() - interval '2 minutes'),
		('orchestrator_input_get', jsonb_build_object('agent', 'AI Stok', 'cycle_id', $1::text), 'AI Stok', null, now() - interval '1 minute')`, id); err != nil {
		t.Fatal(err)
	}
	code, out := sendJSON(t, "GET", srv.URL+"/api/cycles/"+id+"/progress", "sam@gsi.co.id", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	state := map[string]map[string]any{}
	for _, a := range out["agents"].([]any) {
		m := a.(map[string]any)
		state[m["agent"].(string)] = m
	}
	if state["AI Kredit"]["state"] != "done" || state["AI Kredit"]["covers"] != "3 dealer · 1 pesan" {
		t.Fatalf("kredit %v", state["AI Kredit"])
	}
	if state["AI Stok"]["state"] != "reading" || state["AI Stok"]["covers"] != "1 dealer · 2 stok" {
		t.Fatalf("stok %v", state["AI Stok"])
	}
	if state["AI Order"]["state"] != "waiting" {
		t.Fatalf("order %v", state["AI Order"])
	}
	steps := out["steps"].([]any)
	if len(steps) != 3 || steps[0].(map[string]any)["text"] != "membaca data AI Stok" || steps[1].(map[string]any)["text"] != "mengirim analisis AI Kredit · 2 saran" {
		t.Fatalf("steps %v", steps)
	}
	if st := out["stages"].([]any); len(st) != 2 || st[0].(map[string]any)["text"] != "WhatsApp 963 chat · 288 menunggu" {
		t.Fatalf("stages %v", st)
	}
}
