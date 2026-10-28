package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/outbox"
	"distri-arc/internal/wa"
)

// reactionsOf is the reactions under one bubble of the thread.
func reactionsOf(t *testing.T, srv string, thread, user string, mid uuid.UUID) []any {
	t.Helper()
	code, out := sendJSON(t, http.MethodGet, srv+"/api/chat/threads/"+thread, user, nil)
	if code != http.StatusOK {
		t.Fatalf("thread: %d", code)
	}
	for _, m := range out["messages"].([]any) {
		if mm := m.(map[string]any); mm["id"] == mid.String() {
			rs, _ := mm["reactions"].([]any)
			return rs
		}
	}
	t.Fatalf("message %s not in the thread", mid)
	return nil
}

// Reaksi per pesan (Sam 28 Oct): the team reacts to a bubble as the conversation's number — a human decision carried
// out by the outbox through the transport, shown under the bubble as "mine"; a customer's reaction from WhatsApp is
// stored per person and taken back with an empty emoji; nothing else runs for it.
func TestChatReactions(t *testing.T) {
	srv, st := mediaServer(t, nil)
	ctx := context.Background()
	th := sinarThread(t, st)
	base := srv.URL + "/api/chat/threads/" + th.String() + "/messages/"
	theirs := addMessage(t, st, th, "WRE-IN", "in", "received", "Order 3 unit ya", chatNow.Add(-time.Hour), nil)
	ours := addMessage(t, st, th, "WRE-OUT", "out", "sent", "Siap, kami proses", chatNow.Add(-30*time.Minute), nil)
	const andi = "andi@gsi.co.id"

	if code, _ := sendJSON(t, http.MethodPut, base+theirs.String()+"/reaction", andi, map[string]string{"emoji": "oke"}); code != http.StatusBadRequest {
		t.Fatalf("words as a reaction: %d", code)
	}
	code, out := sendJSON(t, http.MethodPut, base+theirs.String()+"/reaction", andi, map[string]string{"emoji": "👍"})
	if code != http.StatusAccepted || count(t, st, "select count(*) from audit_log where action = 'chat.react'") != 1 {
		t.Fatalf("react: %d %v", code, out)
	}
	fake := wa.NewFake()
	send := func(id string) {
		t.Helper()
		if _, err := outbox.NewSender(st, fake, clock.Fixed(chatNow), outbox.Rules{}).Send(ctx, uuid.MustParse(id)); err != nil {
			t.Fatal(err)
		}
	}
	send(out["outbox_id"].(string))
	if s := fake.Sent(); len(s) != 1 || s[0].ReactTo != "WRE-IN" || s[0].Text != "👍" {
		t.Fatalf("transport got %+v", s)
	}
	if rs := reactionsOf(t, srv.URL, th.String(), andi, theirs); len(rs) != 1 || rs[0].(map[string]any)["emoji"] != "👍" || rs[0].(map[string]any)["mine"] != true {
		t.Fatalf("our reaction: %v", rs)
	}
	if code, _ := sendJSON(t, http.MethodPut, base+theirs.String()+"/reaction", andi, map[string]string{"emoji": "👍"}); code != http.StatusOK {
		t.Fatalf("same reaction again: %d", code)
	}

	// the customer reacts on WhatsApp, then takes it back; an older reaction arriving later does not win
	in := wa.NewIngestor(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	react := func(emoji string, at time.Time) {
		t.Helper()
		if _, err := in.Process(ctx, wa.Message{ID: "R" + at.Format("150405") + emoji, Account: "6281234504471", ChatJID: "6281300000001@s.whatsapp.net", FromNumber: "6281300000001", ReactTo: "WRE-OUT", Text: emoji, Time: at}); err != nil {
			t.Fatal(err)
		}
	}
	react("❤️", chatNow.Add(-10*time.Minute))
	react("😂", chatNow.Add(-20*time.Minute))
	if rs := reactionsOf(t, srv.URL, th.String(), andi, ours); len(rs) != 1 || rs[0].(map[string]any)["emoji"] != "❤️" || rs[0].(map[string]any)["mine"] == true {
		t.Fatalf("customer's reaction: %v", rs)
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id like 'R%'"); n != 0 {
		t.Fatalf("a reaction stored as a message: %d", n)
	}
	react("", chatNow.Add(-5*time.Minute))
	if rs := reactionsOf(t, srv.URL, th.String(), andi, ours); len(rs) != 0 {
		t.Fatalf("taken back: %v", rs)
	}

	// we take ours back
	code, out = sendJSON(t, http.MethodPut, base+theirs.String()+"/reaction", andi, map[string]string{"emoji": ""})
	if code != http.StatusAccepted {
		t.Fatalf("take back: %d %v", code, out)
	}
	send(out["outbox_id"].(string))
	if s := fake.Sent(); len(s) != 2 || s[1].ReactTo != "WRE-IN" || s[1].Text != "" {
		t.Fatalf("transport got %+v", s)
	}
	if rs := reactionsOf(t, srv.URL, th.String(), andi, theirs); len(rs) != 0 {
		t.Fatalf("ours taken back: %v", rs)
	}
}
