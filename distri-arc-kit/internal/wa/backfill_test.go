package wa_test

import (
	"context"
	"testing"
	"time"

	"distri-arc/internal/wa"
)

// fakePhone records history requests; deliver, when set, plays the phone answering one.
type fakePhone struct {
	calls   []wa.HistoryAnchor
	down    bool
	deliver func(a wa.HistoryAnchor)
}

func (f *fakePhone) FetchHistory(_ context.Context, account, chatJID string, a wa.HistoryAnchor, count int) error {
	if f.down {
		return wa.ErrHistoryUnavailable
	}
	if account == "" || chatJID == "" || count != wa.HistoryBatch {
		panic("bad request")
	}
	f.calls = append(f.calls, a)
	if f.deliver != nil {
		f.deliver(a)
	}
	return nil
}

const lead = "6289900001111"

// one conversation left to backfill: a new number writing to Andi ten days ago
func backfillSetup(t *testing.T) (context.Context, *wa.Ingestor, func() int, func(p *fakePhone, at time.Time, days int) wa.BackfillReport) {
	t.Helper()
	st, in := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, "update chat_threads set history_done_at = now()"); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Process(ctx, wa.Message{ID: "B1", Account: andi, ChatJID: wa.UserJID(lead), FromNumber: lead, FromName: "Pak Budi", Text: "pagi, harga DVR?", Time: base.AddDate(0, 0, -10)}); err != nil {
		t.Fatal(err)
	}
	done := func() int {
		return count(t, st, "select count(*) from chat_threads where wa_jid = $1 and history_done_at is not null", wa.UserJID(lead))
	}
	backfill := func(p *fakePhone, at time.Time, days int) wa.BackfillReport {
		t.Helper()
		r, err := wa.Backfill(ctx, st, p, at, days, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	return ctx, in, done, backfill
}

var base = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

func TestBackfillWalksBackUntilThePhoneHasNothingOlder(t *testing.T) {
	ctx, in, done, backfill := backfillSetup(t)
	p := &fakePhone{}
	if r := backfill(p, base, 365); r.Requested != 1 || len(p.calls) != 1 || p.calls[0].ID != "B1" || p.calls[0].FromMe {
		t.Fatalf("first run %+v %+v", r, p.calls)
	}
	if r := backfill(p, base.Add(time.Minute), 365); r.Requested != 0 {
		t.Fatalf("asked again before the phone could answer: %+v", r)
	}
	// the phone answers with an older message (history, deduplicated on its id): the next request starts there
	if _, err := in.Process(ctx, wa.Message{ID: "B0", Account: andi, ChatJID: wa.UserJID(lead), FromNumber: andi, FromMe: true, Text: "Siap pak", Time: base.AddDate(0, 0, -200), History: true}); err != nil {
		t.Fatal(err)
	}
	at := base.Add(4 * time.Minute)
	if r := backfill(p, at, 365); r.Requested != 1 || p.calls[1].ID != "B0" || !p.calls[1].FromMe {
		t.Fatalf("second request %+v %+v", r, p.calls)
	}
	// nothing older arrives: three more requests, then the conversation is complete
	for i := 0; i < 3 && done() == 0; i++ {
		at = at.Add(4 * time.Minute)
		backfill(p, at, 365)
	}
	if done() != 1 || len(p.calls) != 4 {
		t.Fatalf("done %d after %d requests", done(), len(p.calls))
	}
	if r := backfill(p, at.Add(time.Hour), 365); r.Requested+r.Done != 0 {
		t.Fatalf("a complete conversation asked again: %+v", r)
	}
}

func TestBackfillStopsAtTheRetentionWindowAndWaitsForAnOfflineNumber(t *testing.T) {
	_, _, done, backfill := backfillSetup(t)
	off := &fakePhone{down: true}
	if r := backfill(off, base, 365); r.Skipped != 1 || done() != 0 {
		t.Fatalf("offline %+v", r)
	}
	p := &fakePhone{}
	if r := backfill(p, base, 365); r.Requested != 1 {
		t.Fatalf("not asked after the number came back: %+v", r)
	}
	if r := backfill(p, base.Add(time.Hour), 7); r.Done != 1 || done() != 1 || len(p.calls) != 1 {
		t.Fatalf("history older than the window was asked for: %+v", r)
	}
}
