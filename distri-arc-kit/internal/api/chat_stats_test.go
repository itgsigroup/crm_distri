package api_test

import (
	"context"
	"testing"
)

// Statistik reads the whole stored conversation, with what Analisa WhatsApp read on its customer messages, and
// another number's conversation stays hidden from a sales user.
func TestChatStats(t *testing.T) {
	srv, st := chatServer(t)
	ctx := context.Background()
	var thread string
	var stored int
	if err := st.Pool.QueryRow(ctx, "select t.id, (select count(*) from chat_messages m where m.thread_id = t.id and m.deleted_at is null) from chat_threads t where seed_key = 'seed:thread:c-sinar'").Scan(&thread, &stored); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into wa_message_insights (message_id, thread_id, sent_at, direction, state, intent, items, analyzed_at)
		values (gen_random_uuid(), $1, now(), 'in', 'needs_reply', 'ask_stock', '[{"name":"NVR 8ch","qty":2}]', now())`, thread); err != nil {
		t.Fatal(err)
	}
	code, out := get(t, srv, "/api/chat/threads/"+thread+"/stats", "sam@gsi.co.id")
	if code != 200 || int(out["messages"].(float64)) != stored || stored == 0 {
		t.Fatalf("%d messages %v stored %d", code, out["messages"], stored)
	}
	if len(out["months"].([]any)) != 6 || len(out["hours"].([]any)) != 24 || len(out["weekdays"].([]any)) != 7 {
		t.Fatalf("shape: %v", out)
	}
	items := out["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != "NVR 8ch" || out["intents"].([]any)[0].(map[string]any)["name"] != "tanya stok" {
		t.Fatalf("asked: %v %v", items, out["intents"])
	}
	if len(out["insights"].([]any)) == 0 {
		t.Fatal("no insight")
	}
	if code, _ := get(t, srv, "/api/chat/threads/"+thread+"/stats", "dewi@gsi.co.id"); code != 404 {
		t.Fatalf("another sales' chat: %d", code)
	}
}
