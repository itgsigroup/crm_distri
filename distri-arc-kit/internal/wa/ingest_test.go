package wa_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/db"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

const (
	andi = "6281234504471"
	dewi = "6281534509032"
)

func setup(t *testing.T) (*store.Store, *wa.Ingestor) {
	t.Helper()
	st := testdb.New(t)
	if _, err := seed.Run(context.Background(), st, db.Seed); err != nil {
		t.Fatal(err)
	}
	return st, wa.NewIngestor(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func count(t *testing.T, st *store.Store, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDealerMessageIsStoredOnceAndTouchesContact(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	var contactNo string
	if err := st.Pool.QueryRow(ctx, "select wa_number from contacts where name = 'Mbak Rina'").Scan(&contactNo); err != nil {
		t.Fatal(err)
	}
	before := count(t, st, "select count(*) from signals")
	now := time.Now()
	m := wa.Message{ID: "TEST1", Account: andi, ChatJID: wa.UserJID(contactNo), FromNumber: contactNo, FromName: "Rina", Text: "order 10 kamera", Time: now}
	r, err := in.Process(ctx, m)
	if err != nil || !r.Stored || r.DealerID == nil {
		t.Fatalf("process: %+v %v", r, err)
	}
	if got := count(t, st, "select count(*) from signals"); got != before+1 {
		t.Fatalf("signals %d want %d", got, before+1)
	}
	if count(t, st, "select count(*) from contacts where wa_number = $1 and last_interaction_at >= $2", contactNo, now.Add(-time.Second)) != 1 {
		t.Fatal("contact last_interaction not updated")
	}
	// same WhatsApp id again: nothing new
	r2, err := in.Process(ctx, m)
	if err != nil || r2.Stored || r2.Reason != wa.DropDuplicate {
		t.Fatalf("dedupe: %+v %v", r2, err)
	}
	if got := count(t, st, "select count(*) from signals"); got != before+1 {
		t.Fatalf("duplicate created a signal")
	}
}

// A DM between GSI employees stays in Chat (Sam 16 Oct: "DM antar karyawan tetap ada"), but it is internal: its own
// thread kind that no analysis reads, no signal, no "waiting for a reply", no customer-reply suggestion.
func TestInternalDMIsKeptOutOfAnalysis(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	sigs := count(t, st, "select count(*) from signals")
	// an employee's own number (internal, not linked to GSI Orbit)
	var own string
	if err := st.Pool.QueryRow(ctx, "select wa_number from internal_numbers where wa_number not in (select wa_number from wa_numbers) limit 1").Scan(&own); err != nil {
		t.Fatal(err)
	}
	r, err := in.Process(ctx, wa.Message{ID: "DM1", Account: andi, ChatJID: wa.UserJID(own), FromNumber: own, FromName: "Dewi", Text: "nanti makan siang?", Time: time.Now()})
	if err != nil || !r.Stored || r.Inbound != uuid.Nil || r.NewNumber != "" {
		t.Fatalf("internal DM: %+v %v", r, err)
	}
	if n := count(t, st, `select count(*) from chat_messages m join chat_threads t on t.id = m.thread_id
		where m.wa_msg_id = 'DM1' and t.kind = 'internal' and m.internal and m.signal_id is null`); n != 1 {
		t.Fatalf("internal DM stored as %d internal messages, want 1", n)
	}
	if count(t, st, "select count(*) from signals") != sigs {
		t.Fatal("internal DM made a signal")
	}
	if n := count(t, st, "select count(*) from chat_threads where account = $1 and wa_jid = $2 and unread = 1", andi, wa.UserJID(own)); n != 1 {
		t.Error("internal DM not counted unread in Chat")
	}
	// a number that becomes a GSI number later (a new employee): its earlier chat turns internal with the next message
	const hire = "6281299990001"
	if _, err := in.Process(ctx, wa.Message{ID: "DM2", Account: dewi, ChatJID: wa.UserJID(hire), FromNumber: hire, Text: "halo", Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "insert into internal_numbers (wa_number, label) values ($1, 'Rina')", hire); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Process(ctx, wa.Message{ID: "DM3", Account: dewi, ChatJID: wa.UserJID(hire), FromNumber: hire, Text: "jadi?", Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, st, "select count(*) from chat_threads where account = $1 and wa_jid = $2 and kind = 'internal' and title = 'Rina'", dewi, wa.UserJID(hire)); n != 1 {
		t.Error("chat with a number that became internal did not turn internal")
	}
}

func TestInternalGroupDoesNotTouchDealerSignals(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	dealerSigs := count(t, st, "select count(*) from signals where dealer_id is not null")
	r, err := in.Process(ctx, wa.Message{ID: "G1", Account: andi, ChatJID: "120363041100000001@g.us", IsGroup: true, GroupName: "Gudang Semarang", FromNumber: "6281100001001", FromName: "Pak Joko", Text: "Kamera 4MP sisa 18", Time: time.Now()})
	if err != nil || !r.Stored {
		t.Fatalf("group: %+v %v", r, err)
	}
	if count(t, st, "select count(*) from signals where dealer_id is not null") != dealerSigs {
		t.Fatal("internal group message became a dealer signal")
	}
	if count(t, st, "select count(*) from signals where kind = 'wa_group' and dedupe_key = 'wa:G1'") != 1 {
		t.Fatal("group signal missing")
	}
	if _, err := st.Pool.Exec(ctx, "update wa_groups set read_enabled = false where jid = '120363041100000001@g.us'"); err != nil {
		t.Fatal(err)
	}
	r, err = in.Process(ctx, wa.Message{ID: "G2", Account: andi, ChatJID: "120363041100000001@g.us", IsGroup: true, FromNumber: "6281100001001", Text: "lagi", Time: time.Now()})
	if err != nil || r.Stored || r.Reason != wa.DropGroupDisabled {
		t.Fatalf("disabled group: %+v %v", r, err)
	}
}

func TestUnknownNumberOpensNewThread(t *testing.T) {
	st, in := setup(t)
	r, err := in.Process(context.Background(), wa.Message{ID: "N1", Account: andi, ChatJID: wa.UserJID("6285700001234"), FromNumber: "6285700001234", FromName: "Toko Baru", Text: "harga kamera?", Time: time.Now()})
	if err != nil || !r.Stored || r.DealerID != nil {
		t.Fatalf("new: %+v %v", r, err)
	}
	if count(t, st, "select count(*) from chat_threads where id = $1 and kind = 'new' and title = 'Toko Baru' and subtitle = '+62 857-••••-1234' and unread = 1", r.ThreadID) != 1 {
		t.Fatal("new-number thread missing, unnamed or number not masked")
	}
	// the phone's saved contact name wins once it is known
	if _, err := in.Process(context.Background(), wa.Message{ID: "N2", Account: andi, ChatJID: wa.UserJID("6285700001234"), FromNumber: "6285700001234", ChatName: "Pak Budi Toko Baru", Text: "jadi?", Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if count(t, st, "select count(*) from chat_threads where id = $1 and title = 'Pak Budi Toko Baru'", r.ThreadID) != 1 {
		t.Fatal("contact name not applied")
	}
	// synced history after linking: no name → masked title, not unread, no identification
	h, err := in.Process(context.Background(), wa.Message{ID: "H1", Account: andi, ChatJID: wa.UserJID("6285700009999"), FromNumber: "6285700009999", Text: "lama", Time: time.Now().Add(-48 * time.Hour), History: true})
	if err != nil || !h.Stored || h.NewNumber != "" {
		t.Fatalf("history: %+v %v", h, err)
	}
	if count(t, st, "select count(*) from chat_threads where id = $1 and title = '+62 857-••••-9999' and unread = 0", h.ThreadID) != 1 {
		t.Fatal("history thread should be masked and read")
	}
}

func TestDigitsAndMask(t *testing.T) {
	for in, want := range map[string]string{"+62 812-3450-4471": "6281234504471", "0812 3450 4471": "6281234504471"} {
		if got := wa.Digits(in); got != want {
			t.Errorf("Digits(%q)=%q want %q", in, got, want)
		}
	}
	if wa.NumberOfJID("6281234504471:12@s.whatsapp.net") != andi {
		t.Error("NumberOfJID")
	}
	if wa.MaskNumber("6282212343310") != "+62 822-••••-3310" {
		t.Error(wa.MaskNumber("6282212343310"))
	}
}

// Two GSI numbers in one group receive the same messages (same WhatsApp id): each is stored once, yet the group
// thread of either number lists all of them and both threads move (12 Oct: Catharina's group looked incomplete).
func TestGroupMessagesShownOnEveryNumber(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	g := "120363041100000001@g.us"
	t0 := time.Now().Add(-time.Hour)
	msgs := []wa.Message{
		{ID: "S1", Account: andi, ChatJID: g, IsGroup: true, FromNumber: "6281100001001", FromName: "Donny", Text: "bisa dikirim hari ini?", Time: t0},
		{ID: "S1", Account: dewi, ChatJID: g, IsGroup: true, FromNumber: "6281100001001", FromName: "Donny", Text: "bisa dikirim hari ini?", Time: t0},
		{ID: "S2", Account: dewi, ChatJID: g, IsGroup: true, FromNumber: "6281100001002", FromName: "Zaky", Text: "sedang diusahakan", Time: t0.Add(time.Minute)},
		{ID: "S2", Account: andi, ChatJID: g, IsGroup: true, FromNumber: "6281100001002", FromName: "Zaky", Text: "sedang diusahakan", Time: t0.Add(time.Minute)},
		{ID: "S3", Account: dewi, ChatJID: g, IsGroup: true, FromMe: true, FromNumber: dewi, Text: "siap", Time: t0.Add(2 * time.Minute)},
	}
	for _, m := range msgs {
		if _, err := in.Process(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id in ('S1','S2','S3')"); n != 3 {
		t.Fatalf("%d rows, want each message once", n)
	}
	for _, acc := range []string{andi, dewi} {
		var id uuid.UUID
		if err := st.Pool.QueryRow(ctx, "select id from chat_threads where wa_jid = $1 and account = $2", g, acc).Scan(&id); err != nil {
			t.Fatal(acc, err)
		}
		rows, err := st.Q.ListThreadMessages(ctx, gen.ListThreadMessagesParams{ThreadID: &id, SentAt: time.Now().Add(time.Hour), Lim: 30})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, r := range rows {
			if r.WaMsgID != nil && strings.HasPrefix(*r.WaMsgID, "S") {
				ids = append(ids, *r.WaMsgID)
			}
		}
		if strings.Join(ids, ",") != "S1,S2,S3" {
			t.Errorf("number %s sees %v", acc, ids)
		}
	}
	// andi's thread moved with S2 even though dewi stored it
	if n := count(t, st, "select count(*) from chat_threads where wa_jid = $1 and account = $2 and last_message_at >= $3", g, andi, t0.Add(time.Minute)); n != 1 {
		t.Error("andi's group thread did not move")
	}
}

// A 1:1 chat between two linked GSI numbers — internal numbers too, as every linked number with a holder is: the
// sender's phone and the receiver's phone carry the same WhatsApp id, and each number's Chat shows the message
// (15 Oct: Catharina → Granike reached only Catharina's thread; 16 Oct: Tantri → Granike reached neither, dropped as
// an internal DM). The receiver's copy is made as soon as the sender's is stored, in real time; the receiver's
// phone delivering it later, or any re-delivery, stores nothing new; an edit and a delete reach both copies.
func TestDMBetweenLinkedNumbersShownOnBoth(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	if count(t, st, "select count(*) from internal_numbers where wa_number in ($1, $2)", andi, dewi) != 2 {
		t.Fatal("both linked numbers should be internal numbers")
	}
	t0 := time.Now().Add(-time.Hour)
	out := wa.Message{ID: "LD1", Account: andi, ChatJID: wa.UserJID(dewi), FromMe: true, FromNumber: andi, FromName: "Andi", Text: "stok 4MP masih?", Time: t0}
	in1 := wa.Message{ID: "LD1", Account: dewi, ChatJID: wa.UserJID(andi), FromNumber: andi, FromName: "Andi", Text: "stok 4MP masih?", Time: t0}
	for i, m := range []wa.Message{out, in1, out, in1} {
		r, err := in.Process(ctx, m)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && count(t, st, `select count(*) from chat_messages m join chat_threads t on t.id = m.thread_id
			where m.wa_msg_id = 'LD1' and t.account = $1`, dewi) != 1 {
			t.Fatal("the receiving number has no copy right after the sender's phone stored it")
		}
		if want := i == 0; r.Stored != want {
			t.Fatalf("message %d: stored=%v want %v (%s)", i, r.Stored, want, r.Reason)
		}
	}
	for acc, dir := range map[string]string{andi: "out", dewi: "in"} {
		if n := count(t, st, `select count(*) from chat_messages m join chat_threads t on t.id = m.thread_id
			where m.wa_msg_id = 'LD1' and t.account = $1 and m.direction = $2`, acc, dir); n != 1 {
			t.Errorf("number %s: %d copies, want 1", acc, n)
		}
	}
	if n := count(t, st, "select count(*) from chat_threads where account = $1 and wa_jid = $2 and unread = 1", dewi, wa.UserJID(andi)); n != 1 {
		t.Error("receiver's thread did not count the message as unread")
	}

	// the edit and the delete arrive on both phones; every copy follows
	edit := wa.Message{ID: "LD1E", Account: dewi, ChatJID: wa.UserJID(andi), FromNumber: andi, Text: "stok 5MP masih?", EditOf: "LD1", Time: t0.Add(time.Minute)}
	if r, err := in.Process(ctx, edit); err != nil || !r.Stored {
		t.Fatalf("edit: %+v %v", r, err)
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id = 'LD1' and body = 'stok 5MP masih?'"); n != 2 {
		t.Errorf("edit reached %d copies, want 2", n)
	}
	if r, err := in.Process(ctx, wa.Message{ID: "LD1R", Account: andi, ChatJID: wa.UserJID(dewi), FromMe: true, RevokeOf: "LD1", Time: t0.Add(2 * time.Minute)}); err != nil || !r.Stored {
		t.Fatalf("revoke: %+v %v", r, err)
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id = 'LD1' and deleted_at is not null"); n != 2 {
		t.Errorf("delete reached %d copies, want 2", n)
	}
}

// A reply sent from Chat to another linked GSI number whose phone delivered it first: the queued bubble still becomes
// "sent" under its own key, and the sender phone's echo adds nothing.
func TestSentFromChatAfterOtherNumberStoredIt(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, "delete from internal_numbers where wa_number in ($1, $2)", andi, dewi); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-time.Hour)
	if _, err := in.Process(ctx, wa.Message{ID: "LD0", Account: andi, ChatJID: wa.UserJID(dewi), FromNumber: dewi, Text: "halo", Time: t0}); err != nil {
		t.Fatal(err)
	}
	var thread uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where account = $1 and wa_jid = $2", andi, wa.UserJID(dewi)).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Process(ctx, wa.Message{ID: "LD2", Account: dewi, ChatJID: wa.UserJID(andi), FromNumber: andi, Text: "siap", Time: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	pending, dir := "pending:LD2", "out"
	mid, err := st.Q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &thread, WaMsgID: &pending, Direction: &dir, Body: strp("siap"), SentAt: t0.Add(time.Minute), Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	sent := "LD2"
	// as outbox.Sender does once WhatsApp accepted it
	if err := st.Q.DropOtherCopies(ctx, gen.DropOtherCopiesParams{ID: mid, WaMsgID: sent}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.UpdateChatMessageSent(ctx, gen.UpdateChatMessageSentParams{ID: mid, WaMsgID: &sent, Status: "sent", SentAt: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, st, "select count(*) from chat_messages where id = $1 and status = 'sent' and wa_msg_id = 'LD2'", mid); n != 1 {
		t.Fatal("queued bubble did not become sent")
	}
	r, err := in.Process(ctx, wa.Message{ID: "LD2", Account: andi, ChatJID: wa.UserJID(dewi), FromMe: true, FromNumber: andi, Text: "siap", Time: t0.Add(time.Minute)})
	if err != nil || r.Stored {
		t.Fatalf("echo: %+v %v", r, err)
	}
	for _, acc := range []string{andi, dewi} {
		if n := count(t, st, `select count(*) from chat_messages m join chat_threads t on t.id = m.thread_id
			where m.wa_msg_id = 'LD2' and t.account = $1`, acc); n != 1 {
			t.Errorf("number %s: %d copies, want 1", acc, n)
		}
	}
	if n := count(t, st, "select count(*) from chat_messages where id = $1", mid); n != 1 {
		t.Error("the queued bubble was dropped instead of the earlier copy")
	}
}

// A reply sent from Chat into a group that another GSI number in it stored first: the group shows it once — the
// queued bubble — not the bubble plus the other number's copy.
func TestGroupReplyFromChatShownOnce(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	g := "120363041100000099@g.us"
	t0 := time.Now().Add(-time.Hour)
	if _, err := in.Process(ctx, wa.Message{ID: "G0", Account: andi, ChatJID: g, IsGroup: true, GroupName: "Proyek Y", FromNumber: "6281100009001", Text: "halo", Time: t0}); err != nil {
		t.Fatal(err)
	}
	var thread uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where account = $1 and wa_jid = $2", andi, g).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	// dewi's phone hands the group message over before the outbox marks andi's bubble sent
	if _, err := in.Process(ctx, wa.Message{ID: "G1", Account: dewi, ChatJID: g, IsGroup: true, GroupName: "Proyek Y", FromNumber: andi, Text: "siap", Time: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	pending, dir := "pending:G1", "out"
	mid, err := st.Q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &thread, WaMsgID: &pending, Direction: &dir, Body: strp("siap"), SentAt: t0.Add(time.Minute), Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	sent := "G1"
	drop := gen.DropOtherCopiesParams{ID: mid, WaMsgID: sent}
	if err := st.Q.DropOtherCopies(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.UpdateChatMessageSent(ctx, gen.UpdateChatMessageSentParams{ID: mid, WaMsgID: &sent, Status: "sent", SentAt: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.DropOtherCopies(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if n := count(t, st, "select count(*) from chat_messages where wa_msg_id = 'G1'"); n != 1 {
		t.Fatalf("%d copies in the group, want 1", n)
	}
	if n := count(t, st, "select count(*) from chat_message_keys where wa_msg_id = 'G1' and message_id = $1", mid); n != 1 {
		t.Fatal("the queued bubble does not hold the WhatsApp id")
	}
}

type fakeGroupNames map[string]map[string]string

func (f fakeGroupNames) GroupNames(_ context.Context, account string) (map[string]string, error) {
	return f[account], nil
}

// A group whose first message came without a name (the bridge could not read it) takes the name the next message
// carries, on every GSI number's thread, and a renamed group follows; a group still unnamed is named from the
// numbers' group lists.
func TestGroupNamesComeLater(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	g, g2 := "120363041100000555@g.us", "120363041100000556@g.us"
	t0 := time.Now().Add(-time.Hour)
	for _, m := range []wa.Message{
		{ID: "N1", Account: andi, ChatJID: g, IsGroup: true, FromNumber: "6281100005001", Text: "halo", Time: t0},
		{ID: "N1", Account: dewi, ChatJID: g, IsGroup: true, FromNumber: "6281100005001", Text: "halo", Time: t0},
		{ID: "N2", Account: andi, ChatJID: g2, IsGroup: true, FromNumber: "6281100005001", Text: "pagi", Time: t0},
	} {
		if _, err := in.Process(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	titles := func(jid string) string {
		var s string
		if err := st.Pool.QueryRow(ctx, "select coalesce(string_agg(distinct coalesce(title, ''), '|'), '') from chat_threads where wa_jid = $1", jid).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if titles(g) != "" {
		t.Fatalf("unnamed group titled %q", titles(g))
	}
	if _, err := in.Process(ctx, wa.Message{ID: "N3", Account: andi, ChatJID: g, IsGroup: true, GroupName: "Proyek Bandara", FromNumber: "6281100005001", Text: "lanjut", Time: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got := titles(g); got != "Proyek Bandara" {
		t.Fatalf("after a named message: %q (want it on both numbers' threads)", got)
	}
	if _, err := in.Process(ctx, wa.Message{ID: "N4", Account: dewi, ChatJID: g, IsGroup: true, GroupName: "Proyek Bandara 2", FromNumber: "6281100005001", Text: "ok", Time: t0.Add(2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got := titles(g); got != "Proyek Bandara 2" {
		t.Fatalf("renamed group: %q", got)
	}
	// an empty name never overwrites a known one
	if _, err := in.Process(ctx, wa.Message{ID: "N5", Account: andi, ChatJID: g, IsGroup: true, FromNumber: "6281100005001", Text: "x", Time: t0.Add(3 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got := titles(g); got != "Proyek Bandara 2" {
		t.Fatalf("name lost: %q", got)
	}
	n, err := in.NameGroups(ctx, fakeGroupNames{andi: {g2: "Gudang Solo"}})
	if err != nil || n != 1 || titles(g2) != "Gudang Solo" {
		t.Fatalf("named %d (%v), title %q", n, err, titles(g2))
	}
}

// Messages kept on one linked number only (before the copy was made in real time) are caught up from the stored
// copy, read, without the other phone; a second run adds nothing.
func TestMirrorLinkedCatchUp(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, "delete from internal_numbers where wa_number in ($1, $2)", andi, dewi); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-48 * time.Hour)
	// dewi not linked yet: andi's message stays on andi's side
	if _, err := st.Pool.Exec(ctx, "update wa_numbers set wa_number = '0' where wa_number = $1", dewi); err != nil {
		t.Fatal(err)
	}
	if r, err := in.Process(ctx, wa.Message{ID: "LD3", Account: andi, ChatJID: wa.UserJID(dewi), FromMe: true, FromNumber: andi, Text: "besok kirim", Time: t0}); err != nil || !r.Stored {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := st.Pool.Exec(ctx, "update wa_numbers set wa_number = $1 where wa_number = '0'", dewi); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := in.MirrorLinkedSince(ctx, t0.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, st, `select count(*) from chat_messages m join chat_threads t on t.id = m.thread_id
		where m.wa_msg_id = 'LD3' and t.account = $1 and m.direction = 'in' and t.unread = 0`, dewi); n != 1 {
		t.Errorf("dewi holds %d read copies, want 1", n)
	}
}

func strp(s string) *string { return &s }

// ADR 0029: a media message keeps its download reference (never the file); a message stored before that gets it when
// history delivers it again; a WhatsApp edit replaces the stored text.
func TestMediaReferenceAndEdits(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	cust := "6281900300777"
	now := time.Now().Truncate(time.Second)
	photo := &wa.Media{Kind: "image", Mime: "image/jpeg", Size: 2048, Ref: "REF-PHOTO"}
	if r, err := in.Process(ctx, wa.Message{ID: "MEDIA1", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Text: "[gambar] kamera rusak", Time: now, Media: photo}); err != nil || !r.Stored {
		t.Fatalf("media: %+v %v", r, err)
	}
	if count(t, st, "select count(*) from chat_messages where wa_msg_id = 'MEDIA1' and media->>'ref' = 'REF-PHOTO' and media->>'kind' = 'image'") != 1 {
		t.Fatal("media reference not stored")
	}

	// stored without a reference (before ADR 0029), then synced again with one
	if _, err := in.Process(ctx, wa.Message{ID: "OLD1", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Text: "[pesan suara]", Time: now.Add(-time.Hour), Media: &wa.Media{Kind: "audio"}}); err != nil {
		t.Fatal(err)
	}
	vn := &wa.Media{Kind: "audio", Mime: "audio/ogg; codecs=opus", Seconds: 9, PTT: true, Ref: "REF-VN"}
	if r, _ := in.Process(ctx, wa.Message{ID: "OLD1", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Text: "[pesan suara]", Time: now.Add(-time.Hour), Media: vn, History: true}); r.Reason != wa.DropDuplicate {
		t.Fatalf("second delivery: %+v", r)
	}
	if count(t, st, "select count(*) from chat_messages where wa_msg_id = 'OLD1' and media->>'ref' = 'REF-VN'") != 1 {
		t.Fatal("reference not filled in on the second delivery")
	}

	// edits: the customer's, then the same edit again (idempotent); an edit of an unknown message is dropped
	edit := wa.Message{ID: "EDIT1", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Text: "kamera rusak 2 unit", Time: now.Add(time.Minute), EditOf: "MEDIA1"}
	for range 2 {
		if r, err := in.Process(ctx, edit); err != nil || !r.Stored {
			t.Fatalf("edit: %+v %v", r, err)
		}
	}
	if count(t, st, "select count(*) from chat_messages where wa_msg_id = 'MEDIA1' and body = 'kamera rusak 2 unit' and edited_at is not null") != 1 {
		t.Fatal("edit not applied")
	}
	if count(t, st, "select count(*) from chat_message_keys where wa_msg_id = 'EDIT1'") != 0 {
		t.Fatal("an edit must not be stored as a message of its own")
	}
	if r, _ := in.Process(ctx, wa.Message{ID: "EDIT2", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Text: "x", Time: now, EditOf: "NOPE"}); r.Reason != wa.DropEditUnknown {
		t.Fatalf("unknown edit: %+v", r)
	}
}

// A customer's reply keeps the id of the message it quotes (WhatsApp "Balas").
func TestReplyKeepsQuotedMessage(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	cust := "6281900300888"
	if _, err := in.Process(ctx, wa.Message{ID: "R1", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Text: "Yang ini ya", Time: time.Now(), QuotedID: "ORIG9"}); err != nil {
		t.Fatal(err)
	}
	if count(t, st, "select count(*) from chat_messages where wa_msg_id = 'R1' and quoted_wa_id = 'ORIG9'") != 1 {
		t.Fatal("quoted id not stored")
	}
}

// A customer's delete for everyone marks the stored message deleted (its text kept for the audit trail).
func TestRevokeMarksDeleted(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	cust := "6281900300999"
	now := time.Now()
	if _, err := in.Process(ctx, wa.Message{ID: "V1", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Text: "salah kirim", Time: now}); err != nil {
		t.Fatal(err)
	}
	if r, err := in.Process(ctx, wa.Message{ID: "V2", Account: andi, ChatJID: wa.UserJID(cust), FromNumber: cust, Time: now.Add(time.Minute), RevokeOf: "V1"}); err != nil || !r.Stored {
		t.Fatalf("revoke: %+v %v", r, err)
	}
	if count(t, st, "select count(*) from chat_messages where wa_msg_id = 'V1' and deleted_at is not null and body = 'salah kirim'") != 1 {
		t.Fatal("not marked deleted")
	}
}

// ADR 0033: a group with anyone outside GSI is a customer group (read by Analisa WhatsApp) — a member WhatsApp hides
// (LID), a customer not in the master, or a sender; a group of GSI numbers only, or named for internal work, stays
// internal; a kind a person chose is never changed by the rule; an employee's message in a group is our side.
func TestCustomerGroupIsRecognised(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	staff := "6281100001001"
	if _, err := st.Pool.Exec(ctx, "insert into internal_numbers (wa_number, label, department) values ($1, 'Pak Joko', 'Gudang') on conflict do nothing", staff); err != nil {
		t.Fatal(err)
	}
	kind := func(jid string) string {
		var k string
		if err := st.Pool.QueryRow(ctx, "select kind from wa_groups where jid = $1", jid).Scan(&k); err != nil {
			t.Fatal(err)
		}
		return k
	}
	now := time.Now()
	process := func(m wa.Message) {
		t.Helper()
		if _, err := in.Process(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	// a member whose number WhatsApp hides: not GSI as far as anyone knows
	hidden := "120363041100000101@g.us"
	process(wa.Message{ID: "CG1", Account: andi, ChatJID: hidden, IsGroup: true, GroupName: "Proyek Toko Sinar", FromNumber: staff, FromName: "Pak Joko",
		Text: "Jadwal pasang besok", Time: now, GroupMembers: []string{andi, staff}, GroupHidden: 1})
	if k := kind(hidden); k != "external" {
		t.Fatalf("group with a hidden member: %s, want external", k)
	}
	// GSI numbers only: internal …
	team := "120363041100000102@g.us"
	process(wa.Message{ID: "CG2", Account: andi, ChatJID: team, IsGroup: true, GroupName: "Tim Semarang", FromNumber: staff, Text: "rapat jam 9", Time: now, GroupMembers: []string{andi, staff, dewi}})
	if k := kind(team); k != "internal" {
		t.Fatalf("GSI-only group: %s, want internal", k)
	}
	var internalMsg bool
	_ = st.Pool.QueryRow(ctx, "select internal from chat_messages where wa_msg_id = 'CG2'").Scan(&internalMsg)
	if !internalMsg {
		t.Error("an employee's group message is not our side")
	}
	// … until someone outside GSI (not in the customer master either) writes
	process(wa.Message{ID: "CG3", Account: andi, ChatJID: team, IsGroup: true, FromNumber: "6285700001234", FromName: "Bu Sinta", Text: "pesan 10 kamera", Time: now.Add(time.Minute)})
	if k := kind(team); k != "external" {
		t.Fatalf("outsider wrote: %s, want external", k)
	}
	if n := count(t, st, "select count(*) from chat_threads t join wa_groups g on g.id = t.group_id where g.jid = $1 and t.subtitle = 'Grup internal'", hidden); n != 0 {
		t.Fatal("customer group thread labelled internal")
	}
	// named for internal work: internal whoever is in it
	gudang := "120363041100000104@g.us"
	process(wa.Message{ID: "CG6", Account: andi, ChatJID: gudang, IsGroup: true, GroupName: "Gudang Yogya", FromNumber: "6285700001234", Text: "stok?", Time: now, GroupHidden: 3})
	if k := kind(gudang); k != "internal" {
		t.Fatalf("gudang group: %s, want internal", k)
	}
	// a person chose internal: the rule leaves it
	manual := "120363041100000103@g.us"
	process(wa.Message{ID: "CG4", Account: andi, ChatJID: manual, IsGroup: true, GroupName: "Arisan", FromNumber: staff, Text: "halo", Time: now})
	if _, err := st.Pool.Exec(ctx, "update wa_groups set kind = 'internal', kind_manual = true where jid = $1", manual); err != nil {
		t.Fatal(err)
	}
	process(wa.Message{ID: "CG5", Account: andi, ChatJID: manual, IsGroup: true, FromNumber: "6285700001234", Text: "ikut", Time: now.Add(time.Minute)})
	if k := kind(manual); k != "internal" {
		t.Fatalf("manual kind changed to %s", k)
	}
}

// Read like WhatsApp: customer messages count as unread until the chat is opened; an answer from the phone means
// the chat was read there; opening a group under one GSI number reads it under every number.
func TestUnreadClearsOnReadAndPhoneReply(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	cust, t0 := wa.UserJID("6285700004321"), time.Now().Add(-time.Hour)
	for i, txt := range []string{"pagi", "stok IPC ada?"} {
		if _, err := in.Process(ctx, wa.Message{ID: "U" + string(rune('1'+i)), Account: andi, ChatJID: cust, FromNumber: "6285700004321", Text: txt, Time: t0.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, st, "select unread from chat_threads where wa_jid = $1 and account = $2", cust, andi); n != 2 {
		t.Fatalf("unread %d, want 2", n)
	}
	if _, err := in.Process(ctx, wa.Message{ID: "U3", Account: andi, ChatJID: cust, FromNumber: andi, FromMe: true, Text: "ada pak", Time: t0.Add(3 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, st, "select unread from chat_threads where wa_jid = $1 and account = $2", cust, andi); n != 0 {
		t.Fatalf("answered from the phone, still %d unread", n)
	}

	g := "120363041100000077@g.us"
	for _, acc := range []string{andi, dewi} {
		if _, err := in.Process(ctx, wa.Message{ID: "G1", Account: acc, ChatJID: g, IsGroup: true, FromNumber: "6281100001001", FromName: "Donny", Text: "PO masuk", Time: t0}); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, st, "select count(*) from chat_threads where wa_jid = $1 and unread > 0", g); n != 2 {
		t.Fatalf("%d group threads unread, want 2", n)
	}
	var id uuid.UUID
	if err := st.Pool.QueryRow(ctx, "select id from chat_threads where wa_jid = $1 and account = $2", g, andi).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.MarkThreadRead(ctx, id); err != nil {
		t.Fatal(err)
	}
	if n := count(t, st, "select count(*) from chat_threads where wa_jid = $1 and unread > 0", g); n != 0 {
		t.Fatalf("group read under one number, %d copies still unread", n)
	}
}

// The name the phone saved for a contact is kept for search, also on a dealer's conversation whose title stays the
// master name (15 Oct: "Pak Yohanes" on the phone is the dealer "JOHANNES YUDHAYANTO").
func TestPhoneNameKeptForSearch(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	r, err := in.Process(ctx, wa.Message{ID: "PN1", Account: andi, ChatJID: wa.UserJID("6285700007777"), FromNumber: andi, FromMe: true, ChatName: "Pak Yohanes", Text: "dengan senang hati pak", Time: time.Now()})
	if err != nil || !r.Stored {
		t.Fatalf("%+v %v", r, err)
	}
	if count(t, st, "select count(*) from chat_threads where id = $1 and phone_name = 'Pak Yohanes'", r.ThreadID) != 1 {
		t.Fatal("phone name not kept")
	}
}

// A group message that tags members ("@Dewi @Budi"): the message keeps the tags with the names Chat shows; the
// tagged GSI number's chat row counts the tag until that chat is opened — reading the group under another number
// leaves it — and the other number in the group, not tagged, gets none.
func TestGroupTagMarksTheTaggedNumber(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	const budi = "6285700001234"
	g := "120363999@g.us"
	text := "@" + dewi + " tolong cek stok, @" + budi + " juga @77889"
	ms := []wa.Mention{{Token: dewi, Number: dewi}, {Token: budi, Number: budi, Name: "Budi"}, {Token: "77889", Name: ""}}
	t0 := time.Now().Add(-time.Minute)
	for _, acc := range []string{andi, dewi} {
		m := wa.Message{ID: "TAG1", Account: acc, ChatJID: g, IsGroup: true, GroupName: "Proyek RSUD", GroupMembers: []string{budi}, FromNumber: budi, FromName: "Budi", Text: text, Mentions: ms, Time: t0}
		if _, err := in.Process(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	tagged := func(acc string) int {
		return count(t, st, "select mentioned from chat_threads where account = $1 and wa_jid = $2", acc, g)
	}
	if tagged(dewi) != 1 || tagged(andi) != 0 {
		t.Fatalf("tags: dewi %d andi %d, want 1 and 0", tagged(dewi), tagged(andi))
	}
	var raw []byte
	if err := st.Pool.QueryRow(ctx, "select mentions from chat_messages where wa_msg_id = 'TAG1'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got []wa.Mention
	if err := json.Unmarshal(raw, &got); err != nil || len(got) != 3 || got[0].Name == "" || got[0].Name == wa.MaskNumber(dewi) || got[1].Name != "Budi" || got[2].Token != "77889" {
		t.Fatalf("mentions kept as %s", raw)
	}
	// history never counts; the group read under andi leaves dewi's tag; dewi opening the chat clears it
	if _, err := in.Process(ctx, wa.Message{ID: "TAG0", Account: dewi, ChatJID: g, IsGroup: true, FromNumber: budi, Text: "@" + dewi + " lama", Mentions: ms[:1], Time: t0.Add(-time.Hour), History: true}); err != nil {
		t.Fatal(err)
	}
	var andiThread, dewiThread uuid.UUID
	_ = st.Pool.QueryRow(ctx, "select id from chat_threads where account = $1 and wa_jid = $2", andi, g).Scan(&andiThread)
	_ = st.Pool.QueryRow(ctx, "select id from chat_threads where account = $1 and wa_jid = $2", dewi, g).Scan(&dewiThread)
	if err := st.Q.MarkThreadRead(ctx, andiThread); err != nil {
		t.Fatal(err)
	}
	if tagged(dewi) != 1 {
		t.Fatal("reading the group under another number cleared the tag")
	}
	if err := st.Q.MarkThreadRead(ctx, dewiThread); err != nil {
		t.Fatal(err)
	}
	if tagged(dewi) != 0 {
		t.Fatal("opening the chat did not clear the tag")
	}
	if got := wa.MentionTokens("@" + dewi + " dan @" + dewi + ", @12 @77889"); len(got) != 2 || got[0] != dewi || got[1] != "77889" {
		t.Errorf("MentionTokens = %v", got)
	}
}
