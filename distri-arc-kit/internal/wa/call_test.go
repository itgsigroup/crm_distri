package wa_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/wa"
)

// ADR 0062: a WhatsApp call is one bubble per call id; a missed call waits for an answer ("Belum dibalas") like a
// message, without an AI Trigger draft or a commitment; a call picked up on the phone answers what came before it.
func TestCallRecords(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	var contactNo string
	if err := st.Pool.QueryRow(ctx, "select wa_number from contacts where name = 'Mbak Rina'").Scan(&contactNo); err != nil {
		t.Fatal(err)
	}
	jid := wa.UserJID(contactNo)
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	missed := wa.Message{ID: "call:C1", Account: andi, ChatJID: jid, FromNumber: contactNo, Text: "Panggilan suara tak terjawab", Time: start,
		Media: &wa.Media{Kind: "call", Call: "missed"}}
	r, err := in.Process(ctx, missed)
	if err != nil || !r.Stored {
		t.Fatalf("missed: %+v %v", r, err)
	}
	if r.Inbound != uuid.Nil {
		t.Fatal("a call must not ask AI Trigger for a reply draft")
	}
	if r, _ := in.Process(ctx, missed); r.Stored {
		t.Fatal("the same call stored twice")
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id = 'call:C1' and body = 'Panggilan suara tak terjawab' and media->>'call' = 'missed' and direction = 'in'"); n != 1 {
		t.Fatalf("call bubble %d", n)
	}
	// waiting: the newest customer message came after the last "answered" mark
	waiting := func() bool {
		t.Helper()
		var b bool
		if err := st.Pool.QueryRow(ctx, `select coalesce(t.replied_mark, '-infinity') < (select max(sent_at) from chat_messages m where m.thread_id = t.id and m.direction = 'in')
			from chat_threads t where t.wa_jid = $1 and t.account = $2`, jid, andi).Scan(&b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	if !waiting() {
		t.Fatal("a missed call waits for an answer")
	}
	answered := wa.Message{ID: "call:C2", Account: andi, ChatJID: jid, FromNumber: contactNo, Text: "Panggilan suara masuk · 3:12", Time: start.Add(10 * time.Minute),
		Media: &wa.Media{Kind: "call", Call: "answered", Seconds: 192}}
	if r, err := in.Process(ctx, answered); err != nil || !r.Stored {
		t.Fatalf("answered: %+v %v", r, err)
	}
	if waiting() {
		t.Fatal("a call picked up answers the chat")
	}
}
