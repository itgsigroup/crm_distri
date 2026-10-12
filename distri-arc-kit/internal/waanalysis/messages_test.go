package waanalysis

import (
	"testing"
	"time"

	"distri-arc/internal/agents"
)

func waitOf(m MessageInsight) int {
	if m.WaitMin == nil {
		return -1
	}
	return *m.WaitMin
}

// Every bubble: the customer's question answered after 60 min, the order still waiting 5 h, the follow-up 4 h.
func TestAnalyzeMessages(t *testing.T) {
	th := thread("dealer", "Andi",
		msg(false, "Selamat pagi pak, ada promo NVR", 31*time.Hour), // sales writes first
		msg(true, "Pak, stok kamera ready?", 30*time.Hour),
		msg(true, "Yang 4MP", 29*time.Hour+30*time.Minute),
		msg(false, "Ready pak", 29*time.Hour),
		msg(true, "Order 10 kamera ip 4mp dan 2 HDD 2TB ya", 5*time.Hour),
		msg(true, "Kirim Senin", 4*time.Hour),
	)
	got := AnalyzeMessages(th, catalog, now)
	if len(got) != 6 {
		t.Fatalf("%d messages", len(got))
	}
	want := []struct {
		state, intent string
		wait          int
	}{
		{MsgFollowUp, IntentNone, -1},
		{MsgReplied, agents.IntentStock, 60},
		{MsgReplied, got[2].Intent, 30},
		{MsgReply, IntentNone, 60}, // the customer had waited since their first question
		{MsgNeedsReply, agents.IntentOrder, 300},
		{MsgNeedsReply, got[5].Intent, 300}, // the customer waits since the order, 5 h
	}
	for i, w := range want {
		g := got[i]
		if g.State != w.state || g.Intent != w.intent || waitOf(g) != w.wait {
			t.Errorf("message %d: %s %s %d, want %s %s %d", i, g.State, g.Intent, waitOf(g), w.state, w.intent, w.wait)
		}
		if g.MessageID != th.Messages[i].ID || g.ThreadID != th.ID {
			t.Errorf("message %d: ids", i)
		}
	}
	if ItemsText(got[4].Items) != "10 Kamera IP 4MP + 2 HDD 2TB" {
		t.Errorf("items %q", ItemsText(got[4].Items))
	}
}

// A closing "Ok siap pak 👍" after our answer waits for nobody; a fresh question is waiting, not yet due.
func TestAnalyzeMessagesClosingAndRecent(t *testing.T) {
	closed := AnalyzeMessages(thread("dealer", "Andi",
		msg(true, "harga NVR 16ch berapa?", 3*time.Hour), msg(false, "1,2 jt pak", 2*time.Hour), msg(true, "Ok siap pak 👍", time.Hour)), catalog, now)
	if closed[2].State != MsgClosing || closed[2].WaitMin != nil || closed[0].Intent != agents.IntentPrice {
		t.Errorf("closing: %+v / %s", closed[2], closed[0].Intent)
	}
	recent := AnalyzeMessages(thread("new", "Budi", msg(true, "Halo, ini toko kamera?", 30*time.Minute)), catalog, now)
	if recent[0].State != MsgWaiting || waitOf(recent[0]) != 30 {
		t.Errorf("recent: %s %d", recent[0].State, waitOf(recent[0]))
	}
}
