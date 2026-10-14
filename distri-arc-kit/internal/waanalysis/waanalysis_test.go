package waanalysis

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
)

var now = time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)

var catalog = []agents.Product{{Name: "Kamera IP 4MP"}, {Name: "HDD 2TB"}, {Name: "NVR 16ch"}}

func msg(in bool, text string, ago time.Duration) Message {
	sig := uuid.New()
	m := Message{ID: uuid.New(), In: in, Text: text, At: now.Add(-ago)}
	if in {
		m.SignalID = &sig
	}
	return m
}

func thread(kind, sales string, ms ...Message) Thread {
	return Thread{ID: uuid.New(), Kind: kind, Title: "T", SalesName: sales, Messages: ms}
}

func TestAnalyzeWaitingOrder(t *testing.T) {
	th := thread("dealer", "Andi",
		msg(true, "Pak, stok kamera ready?", 30*time.Hour),
		msg(false, "Ready pak", 29*time.Hour),                             // answered in 60 min
		msg(true, "Order 10 kamera ip 4mp dan 2 HDD 2TB ya", 5*time.Hour), // waiting 5 h
		msg(true, "Kirim Senin", 4*time.Hour),
	)
	got := Analyze([]Thread{th}, catalog, now)
	if len(got) != 1 {
		t.Fatalf("%d insights", len(got))
	}
	in := got[0]
	if in.Intent != agents.IntentOrder || !in.NeedsReply || in.WaitMin != 300 {
		t.Errorf("intent %s needs %v wait %d", in.Intent, in.NeedsReply, in.WaitMin)
	}
	if ItemsText(in.Items) != "10 Kamera IP 4MP + 2 HDD 2TB" {
		t.Errorf("items %q", ItemsText(in.Items))
	}
	if in.Inbound != 3 || in.Outbound != 1 || len(in.SignalIDs) != 3 {
		t.Errorf("in %d out %d signals %d", in.Inbound, in.Outbound, len(in.SignalIDs))
	}
	if in.ReplyMedianMin == nil || *in.ReplyMedianMin != 60 {
		t.Errorf("reply median %v", in.ReplyMedianMin)
	}
	if in.LastText != "Kirim Senin" {
		t.Errorf("last text %q", in.LastText)
	}
}

func TestAnalyzeAnsweredAndRecent(t *testing.T) {
	answered := thread("dealer", "Andi", msg(true, "harga NVR 16ch berapa?", 3*time.Hour), msg(false, "1,2 jt pak", 2*time.Hour))
	recent := thread("new", "Budi", msg(true, "Halo, ini toko kamera?", 30*time.Minute)) // waiting, not yet due
	ours := thread("dealer", "Budi", msg(false, "Promo minggu ini", time.Hour))
	got := Analyze([]Thread{answered, recent, ours}, catalog, now)
	if len(got) != 3 {
		t.Fatalf("%d insights", len(got))
	}
	byID := map[uuid.UUID]Insight{}
	for _, i := range got {
		byID[i.ThreadID] = i
	}
	if a := byID[answered.ID]; a.WaitingSince != nil || a.NeedsReply || a.Intent != agents.IntentPrice {
		t.Errorf("answered: %+v", a)
	}
	if r := byID[recent.ID]; r.WaitingSince == nil || r.NeedsReply || r.WaitMin != 30 {
		t.Errorf("recent: waiting %v needs %v wait %d", r.WaitingSince, r.NeedsReply, r.WaitMin)
	}
	if o := byID[ours.ID]; o.Intent != IntentNone || o.Inbound != 0 {
		t.Errorf("ours: %+v", o)
	}
	// the conversation still waiting comes first
	if got[0].ThreadID != recent.ID {
		t.Errorf("first %s", got[0].Title)
	}
}

// ADR 0033: a customer group is read like a one-to-one chat (internal groups never reach Analyze: the query leaves
// them out); a GSI employee's message in it (In false) answers the customer.
func TestAnalyzeReadsCustomerGroups(t *testing.T) {
	waiting := thread("group", "", msg(true, "stok kamera 4mp ada?", 5*time.Hour))
	answered := thread("group", "", msg(true, "stok habis?", 5*time.Hour), msg(false, "ready 20 pcs", 4*time.Hour))
	other := thread("broadcast", "", msg(true, "promo", time.Hour))
	got := Analyze([]Thread{waiting, answered, other}, catalog, now)
	if len(got) != 2 {
		t.Fatalf("%d insights, want the two customer groups: %+v", len(got), got)
	}
	by := map[uuid.UUID]Insight{}
	for _, x := range got {
		by[x.ThreadID] = x
	}
	if w := by[waiting.ID]; !w.NeedsReply || w.Kind != "group" {
		t.Errorf("waiting group %+v", w)
	}
	if a := by[answered.ID]; a.NeedsReply || a.WaitingSince != nil || a.Outbound != 1 {
		t.Errorf("answered group %+v", a)
	}
}

func TestSummarize(t *testing.T) {
	ins := Analyze([]Thread{
		thread("dealer", "Andi", msg(true, "order 5 HDD 2TB", 6*time.Hour)),
		thread("dealer", "Andi", msg(true, "stok NVR ready?", 4*time.Hour), msg(false, "ready", 3*time.Hour)),
		thread("new", "Budi", msg(true, "rilis dong pak", 3*time.Hour)),
	}, catalog, now)
	tot, rows := Summarize(ins)
	if tot.Threads != 3 || tot.Dealers != 2 || tot.NewNumbers != 1 || tot.NeedsReply != 2 || tot.Waiting != 2 {
		t.Errorf("totals %+v", tot)
	}
	if tot.ByIntent[agents.IntentOrder] != 1 || tot.ByIntent[agents.IntentStock] != 1 || tot.ByIntent[agents.IntentRelease] != 1 {
		t.Errorf("by intent %v", tot.ByIntent)
	}
	if len(rows) != 2 || rows[0].Name != "Andi" || rows[0].Threads != 2 || rows[0].NeedsReply != 1 {
		t.Errorf("rows %+v", rows)
	}
	if rows[0].ReplyMedianMin == nil || *rows[0].ReplyMedianMin != 60 {
		t.Errorf("Andi reply %v", rows[0].ReplyMedianMin)
	}
}

func TestLabels(t *testing.T) {
	for _, c := range []struct {
		min  int
		want string
	}{{45, "45 menit"}, {300, "5 jam"}, {3 * 24 * 60, "3 hari"}} {
		if got := WaitLabel(c.min); got != c.want {
			t.Errorf("WaitLabel(%d) = %q", c.min, got)
		}
	}
	if IntentLabel(agents.IntentOrder) != "minta order" {
		t.Error("label")
	}
}

func TestIsAck(t *testing.T) {
	for _, s := range []string{"Ok Bu", "Okwy", "sama samaa", "siap pak", "Terima kasih banyak", "👍"} {
		if !IsAck(s) {
			t.Errorf("%q should be a closing message", s)
		}
	}
	for _, s := range []string{"Klo upsnya gmna Bu", "ready kak?", "[gambar]", "hardisk 10tb 4unit", "Type apa? Tak ajukan dulu"} {
		if IsAck(s) {
			t.Errorf("%q needs a reply", s)
		}
	}
}

// ADR 0034: a holding reply ("tunggu sebentar ya", "saya konfirmasi dulu") does not answer the customer — the
// response time runs until the real answer, and with no answer yet the customer is still waiting.
func TestHoldingReplyDoesNotStopTheClock(t *testing.T) {
	hold := func(text string, ago time.Duration) Message {
		m := msg(false, text, ago)
		m.Holding = true
		return m
	}
	answered := thread("dealer", "Andi",
		msg(true, "Pak, stok kamera 4mp ada?", 5*time.Hour),
		hold("Tunggu sebentar ya pak, saya cek dulu", 299*time.Minute), // 1 min later: not an answer
		msg(false, "Ready 20 pcs pak", 3*time.Hour),                    // the answer, 120 min after the question
	)
	waiting := thread("dealer", "Andi",
		msg(true, "Harga NVR 16ch berapa?", 4*time.Hour),
		hold("Baik bu, saya konfirmasi dulu ya", 239*time.Minute),
	)
	got := Analyze([]Thread{answered, waiting}, catalog, now)
	by := map[uuid.UUID]Insight{}
	for _, x := range got {
		by[x.ThreadID] = x
	}
	a := by[answered.ID]
	if a.ReplyMedianMin == nil || *a.ReplyMedianMin != 120 || a.WaitingSince != nil || a.Outbound != 2 {
		t.Errorf("answered: reply %v min (want 120, not 1), waiting %v, outbound %d", a.ReplyMedianMin, a.WaitingSince, a.Outbound)
	}
	w := by[waiting.ID]
	if w.WaitingSince == nil || !w.NeedsReply || w.WaitMin != 240 || w.ReplyMedianMin != nil {
		t.Errorf("held only: %+v", w)
	}
	ms := AnalyzeMessages(answered, nil, now)
	if ms[0].State != MsgReplied || *ms[0].WaitMin != 120 || ms[1].State != MsgHolding || *ms[1].WaitMin != 1 || ms[2].State != MsgReply || *ms[2].WaitMin != 120 {
		t.Errorf("bubbles: %s %v · %s %v · %s %v", ms[0].State, *ms[0].WaitMin, ms[1].State, *ms[1].WaitMin, ms[2].State, *ms[2].WaitMin)
	}
	if ws := AnalyzeMessages(waiting, nil, now); ws[0].State != MsgNeedsReply || ws[1].State != MsgHolding {
		t.Errorf("held bubbles: %s %s", ws[0].State, ws[1].State)
	}
}
