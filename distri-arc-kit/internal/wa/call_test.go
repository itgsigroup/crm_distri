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

// ADR 0063: a group's own line ("Anda membuat grup", "Budi ditambahkan") opens the group's chat the moment the group
// exists; it is nobody's message — no unread, no signal, no AI Trigger draft, and the chat does not wait for an answer.
func TestGroupSystemLineOpensThread(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	jid := "120363041100000777@g.us"
	now := time.Now().Truncate(time.Second)
	created := wa.Message{ID: "G1", Account: andi, ChatJID: jid, IsGroup: true, GroupName: "Proyek Toko Baru", FromNumber: andi, FromMe: true,
		Text: `Anda membuat grup "Proyek Toko Baru"`, Time: now, GroupMembers: []string{andi, "6285700001234"}, Media: &wa.Media{Kind: "system", System: "create"}}
	r, err := in.Process(ctx, created)
	if err != nil || !r.Stored {
		t.Fatalf("create: %+v %v", r, err)
	}
	if r.Inbound != uuid.Nil || r.SignalID != uuid.Nil {
		t.Fatal("a group line is no message to draft a reply to or to analyse")
	}
	if n := count(t, st, "select count(*) from chat_threads t join wa_groups g on g.id = t.group_id where g.jid = $1 and g.name = 'Proyek Toko Baru' and t.unread = 0", jid); n != 1 {
		t.Fatalf("group thread %d", n)
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id = 'G1' and direction = 'in' and not internal and media->>'kind' = 'system' and signal_id is null"); n != 1 {
		t.Fatalf("system line %d", n)
	}
	// a customer added by the group's creator: a line from someone else, still read by no one and waiting for nothing
	added := wa.Message{ID: "G2", Account: andi, ChatJID: jid, IsGroup: true, FromNumber: "6285700001234", Text: "Bu Sinta menambahkan +6285700009999", Time: now.Add(time.Minute),
		Media: &wa.Media{Kind: "system", System: "add"}}
	if r, err := in.Process(ctx, added); err != nil || !r.Stored || r.Inbound != uuid.Nil {
		t.Fatalf("added: %+v %v", r, err)
	}
	var unread int32
	var waiting *time.Time
	if err := st.Pool.QueryRow(ctx, `select t.unread, (select min(m.sent_at) from chat_messages m where m.thread_id = t.id and m.direction = 'in' and not m.internal
		and coalesce(m.media->>'kind', '') <> 'system') from chat_threads t where t.wa_jid = $1 and t.account = $2`, jid, andi).Scan(&unread, &waiting); err != nil {
		t.Fatal(err)
	}
	if unread != 0 || waiting != nil {
		t.Fatalf("unread %d waiting %v: group lines count as nothing to answer", unread, waiting)
	}
	// the first real message then lands in the chat that already exists
	if r, err := in.Process(ctx, wa.Message{ID: "G3", Account: andi, ChatJID: jid, IsGroup: true, FromNumber: "6285700001234", FromName: "Bu Sinta", Text: "pesan 10 kamera", Time: now.Add(2 * time.Minute)}); err != nil || !r.Stored || r.Inbound == uuid.Nil {
		t.Fatalf("first text: %+v %v", r, err)
	}
	if n := count(t, st, "select count(*) from chat_threads where wa_jid = $1 and account = $2 and unread = 1", jid, andi); n != 1 {
		t.Fatal("the first text is the first unread")
	}
}
