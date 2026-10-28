package wa_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/wa"
)

// ADR 0072: a contact's chat stored under their LID joins the chat of their number on the same GSI number — messages,
// stars, labels (a label on both kept once), pins, the unread count and the name saved on the phone — and the LID
// chat is gone; again is a no-op. Without a chat under the number, the LID chat just takes the number's JID.
func TestMergeLIDThread(t *testing.T) {
	st, _ := setup(t)
	ctx := context.Background()
	mk := func(jid, title string, unread int, phoneName *string) uuid.UUID {
		var id uuid.UUID
		if err := st.Pool.QueryRow(ctx, `insert into chat_threads (kind, wa_jid, title, account, unread, phone_name, last_message_at) values ('new', $1, $2, $3, $4, $5, now()) returning id`,
			jid, title, andi, unread, phoneName).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	msg := func(th uuid.UUID, wid string, at time.Time) uuid.UUID {
		var id uuid.UUID
		if err := st.Pool.QueryRow(ctx, `insert into chat_messages (thread_id, wa_msg_id, direction, body, sent_at, status) values ($1, $2, 'in', 'test', $3, 'received') returning id`, th, wid, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	saved := "Mas Rizal IT - GSI"
	phone := mk("6282335180530@s.whatsapp.net", "rizal fachrezi", 1, nil)
	lid := mk("215030000000001@lid", "rizal fachrezi", 2, &saved)
	now := time.Now().Truncate(time.Second)
	msg(phone, "P1", now.Add(-2*time.Hour))
	m2 := msg(lid, "L1", now.Add(-time.Hour))
	var user, label uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id from users limit 1").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, "insert into chat_labels (name, color) values ('Prospek', 'blue') returning id").Scan(&label); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`insert into chat_message_stars (user_id, message_id, thread_id) values ('` + user.String() + `', '` + m2.String() + `', '` + lid.String() + `')`,
		`insert into chat_conv_labels (conv, label_id) values ('` + lid.String() + `', '` + label.String() + `'), ('` + phone.String() + `', '` + label.String() + `')`,
		`insert into chat_pins (conv, message_id, sent_at) values ('` + lid.String() + `', '` + m2.String() + `', now())`,
		`insert into chat_thread_pins (user_id, conv) values ('` + user.String() + `', '` + lid.String() + `')`,
	} {
		if _, err := st.Pool.Exec(ctx, q); err != nil {
			t.Fatal(q, err)
		}
	}

	if kept, err := wa.MergeLIDThread(ctx, st, lid, "6282335180530@s.whatsapp.net"); err != nil || kept != phone {
		t.Fatalf("merge: %v %v", kept, err)
	}
	if kept, err := wa.MergeLIDThread(ctx, st, lid, "6282335180530@s.whatsapp.net"); err != nil || kept != uuid.Nil {
		t.Fatalf("again: %v %v", kept, err)
	}
	checks := map[string]int{
		"select count(*) from chat_threads where id = '" + lid.String() + "'":                                         0,
		"select count(*) from chat_messages where thread_id = '" + phone.String() + "'":                               2,
		"select count(*) from chat_message_stars where thread_id = '" + phone.String() + "'":                          1,
		"select count(*) from chat_conv_labels where conv = '" + phone.String() + "'":                                 1,
		"select count(*) from chat_pins where conv = '" + phone.String() + "'":                                        1,
		"select count(*) from chat_thread_pins where conv = '" + phone.String() + "'":                                 1,
		"select unread from chat_threads where id = '" + phone.String() + "'":                                         3,
		"select count(*) from chat_threads where id = '" + phone.String() + "' and phone_name = 'Mas Rizal IT - GSI'": 1,
	}
	for q, want := range checks {
		if n := count(t, st, q); n != want {
			t.Errorf("%s = %d, want %d", q, n, want)
		}
	}

	// no chat under the number yet: the LID chat takes its JID
	alone := mk("215030000000002@lid", "Bu Sari", 0, nil)
	kept, err := wa.MergeLIDThread(ctx, st, alone, "6281300000555@s.whatsapp.net")
	if err != nil || kept != alone || count(t, st, "select count(*) from chat_threads where id = '"+alone.String()+"' and wa_jid = '6281300000555@s.whatsapp.net'") != 1 {
		t.Fatalf("rename: %v %v", kept, err)
	}
	if _, err := wa.MergeLIDThread(ctx, st, alone, "215030000000003@lid"); err == nil {
		t.Fatal("a LID is no phone JID")
	}
}
