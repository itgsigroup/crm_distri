package wa_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
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
	if count(t, st, "select count(*) from chat_threads where id = $1 and kind = 'new' and title = '+62 857-••••-1234'", r.ThreadID) != 1 {
		t.Fatal("new-number thread missing or not masked")
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
