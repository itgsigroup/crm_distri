package wa_test

import (
	"context"
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

func TestInternalDMIsNeverStored(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	msgs := count(t, st, "select count(*) from chat_messages")
	sigs := count(t, st, "select count(*) from signals")
	r, err := in.Process(ctx, wa.Message{ID: "DM1", Account: andi, ChatJID: wa.UserJID(dewi), FromNumber: dewi, FromName: "Dewi", Text: "nanti makan siang?", Time: time.Now()})
	if err != nil || r.Stored || r.Reason != wa.DropInternalDM {
		t.Fatalf("internal DM: %+v %v", r, err)
	}
	if count(t, st, "select count(*) from chat_messages") != msgs || count(t, st, "select count(*) from signals") != sigs {
		t.Fatal("internal DM reached the database")
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

// ADR 0033: a group with a customer of the master is a customer group (read by Analisa WhatsApp); a group of
// employees stays internal; a kind a person chose is never changed by the rule.
func TestCustomerGroupIsRecognised(t *testing.T) {
	st, in := setup(t)
	ctx := context.Background()
	var rina string
	if err := st.Pool.QueryRow(ctx, "select wa_number from contacts where name = 'Mbak Rina'").Scan(&rina); err != nil {
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
	// a new group whose members include a customer
	withCustomer := "120363041100000101@g.us"
	if _, err := in.Process(ctx, wa.Message{ID: "CG1", Account: andi, ChatJID: withCustomer, IsGroup: true, GroupName: "Proyek Toko Sinar", FromNumber: "6281100001001", FromName: "Pak Joko",
		Text: "Jadwal pasang besok", Time: now, GroupMembers: []string{andi, "6281100001001", rina}}); err != nil {
		t.Fatal(err)
	}
	if k := kind(withCustomer); k != "external" {
		t.Fatalf("group with a customer member: %s, want external", k)
	}
	// a group of employees only stays internal …
	staff := "120363041100000102@g.us"
	if _, err := in.Process(ctx, wa.Message{ID: "CG2", Account: andi, ChatJID: staff, IsGroup: true, GroupName: "Tim Semarang", FromNumber: "6281100001001", FromName: "Pak Joko",
		Text: "rapat jam 9", Time: now, GroupMembers: []string{andi, "6281100001001"}}); err != nil {
		t.Fatal(err)
	}
	if k := kind(staff); k != "internal" {
		t.Fatalf("staff group: %s, want internal", k)
	}
	// … until a customer of the master writes in it
	if _, err := in.Process(ctx, wa.Message{ID: "CG3", Account: andi, ChatJID: staff, IsGroup: true, FromNumber: rina, FromName: "Rina", Text: "pesan 10 kamera", Time: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if k := kind(staff); k != "external" {
		t.Fatalf("customer wrote: %s, want external", k)
	}
	if n := count(t, st, "select count(*) from chat_threads t join wa_groups g on g.id = t.group_id where g.jid = $1 and t.subtitle = 'Grup internal'", withCustomer); n != 0 {
		t.Fatal("customer group thread labelled internal")
	}
	// a person chose internal: the rule leaves it
	manual := "120363041100000103@g.us"
	if _, err := in.Process(ctx, wa.Message{ID: "CG4", Account: andi, ChatJID: manual, IsGroup: true, GroupName: "Arisan", FromNumber: "6281100001001", Text: "halo", Time: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "update wa_groups set kind = 'internal', kind_manual = true where jid = $1", manual); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Process(ctx, wa.Message{ID: "CG5", Account: andi, ChatJID: manual, IsGroup: true, FromNumber: rina, Text: "ikut", Time: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if k := kind(manual); k != "internal" {
		t.Fatalf("manual kind changed to %s", k)
	}
}
