package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// WhatsApp's 24-hour rule: Chat writes to a customer only within 24 hours of their last message. A chat they never
// wrote in, or last wrote in yesterday morning, says so (the composer becomes the banner) and a send is refused
// before it reaches the outbox; once they write, Chat may answer. A group is not bound by it.
func TestReplyWindow24h(t *testing.T) {
	srv, st := mediaServer(t, &fakeMedia{})
	ctx := context.Background()
	src := sinarThread(t, st)
	thread := func(kind, jid string) uuid.UUID {
		var id uuid.UUID
		if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, sales_id, account, last_message_at)
			select $1, $2, 'Toko Baru', sales_id, account, now() from chat_threads where id = $3 returning id`, kind, jid, src).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	closed := func(id uuid.UUID) bool {
		_, out := get(t, srv, "/api/chat/threads/"+id.String(), "andi@gsi.co.id")
		v, _ := out["thread"].(map[string]any)["reply_closed"].(bool)
		return v
	}
	send := func(id uuid.UUID) int {
		code, _ := sendJSON(t, http.MethodPost, srv.URL+"/api/chat/threads/"+id.String()+"/messages", "andi@gsi.co.id", map[string]any{"body": "Halo Pak"})
		return code
	}
	fresh := thread("new", "6281300000401@s.whatsapp.net")
	if !closed(fresh) || send(fresh) != http.StatusConflict {
		t.Fatal("a chat the customer never wrote in is open to a send")
	}
	old := addMessage(t, st, fresh, "RW1", "in", "received", "Halo, ada stok?", chatNow.Add(-25*time.Hour), nil)
	if !closed(fresh) || send(fresh) != http.StatusConflict {
		t.Fatal("a chat whose last customer message is 25 hours old is open to a send")
	}
	// a reaction revives a chat too
	if code, out := sendJSON(t, http.MethodPut, srv.URL+"/api/chat/threads/"+fresh.String()+"/messages/"+old.String()+"/reaction", "andi@gsi.co.id", map[string]string{"emoji": "👍"}); code != http.StatusConflict {
		t.Fatalf("reaction outside 24h: %d %v", code, out)
	}
	if n := count(t, st, "select count(*) from chat_messages where thread_id = $1 and direction = 'out'", fresh); n != 0 {
		t.Fatalf("%d messages stored for refused sends", n)
	}
	addMessage(t, st, fresh, "RW2", "in", "received", "Pak, jadi kirim?", chatNow.Add(-2*time.Hour), nil)
	if closed(fresh) || send(fresh) != http.StatusAccepted {
		t.Fatal("a customer who wrote 2 hours ago cannot be answered")
	}
	group := thread("group", "120363000000081@g.us")
	if closed(group) || send(group) != http.StatusAccepted {
		t.Fatal("a group is bound by the 24-hour window")
	}
	// Pengaturan → Chat WhatsApp → Dibuka (CEO, ADR 0082): a customer who never wrote can be written to.
	quiet := thread("dealer", "6281300000402@s.whatsapp.net")
	rules := map[string]any{"work_days": []int{1, 2, 3, 4, 5, 6}, "work_start": "08:00", "work_end": "17:00", "reply_sla_min": 30, "escalate_hours": 24, "reply_window_open": true}
	if code, out := sendJSON(t, http.MethodPut, srv.URL+"/api/policies/chat.rules", "andi@gsi.co.id", rules); code != http.StatusForbidden {
		t.Fatalf("a sales opened Chat: %d %v", code, out)
	}
	if code, out := sendJSON(t, http.MethodPut, srv.URL+"/api/policies/chat.rules", "sam@gsi.co.id", rules); code != http.StatusOK {
		t.Fatalf("open Chat: %d %v", code, out)
	}
	if closed(quiet) || send(quiet) != http.StatusAccepted {
		t.Fatal("Chat opened in Pengaturan still refuses a quiet customer")
	}
	rules["reply_window_open"] = false
	if code, out := sendJSON(t, http.MethodPut, srv.URL+"/api/policies/chat.rules", "sam@gsi.co.id", rules); code != http.StatusOK {
		t.Fatalf("back to 24 hours: %d %v", code, out)
	}
	if !closed(quiet) {
		t.Fatal("back to 24 hours, a quiet customer is still open")
	}
}
