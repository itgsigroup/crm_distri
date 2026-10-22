package chatstats

import (
	"strings"
	"testing"
	"time"
)

var wib = time.FixedZone("WIB", 7*3600)

func at(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, wib) }

// A customer's turn is answered by our first real answer (not a holding reply); "ok" needs no answer; the median,
// the share under 1 hour, the waiting turn, who starts the talks and when the customer writes come from the messages.
func TestCompute(t *testing.T) {
	now := at(22, 12, 0)
	msgs := []Msg{
		{At: at(13, 9, 0)}, // Tue: customer opens (talk 1)
		{At: at(13, 9, 10), Out: true, Holding: true}, // "tunggu sebentar" — not an answer
		{At: at(13, 9, 30), Out: true},                // answered in 30 min
		{At: at(13, 9, 35), Closing: true},            // "ok" — no turn
		{At: at(14, 10, 0), Out: true},                // Wed: we open (talk 2)
		{At: at(14, 10, 5)},                           // customer turn
		{At: at(14, 13, 5), Internal: true},           // a GSI employee in the group answers after 3 h
		{At: at(20, 9, 15), From: "Tisa"},             // Tue: customer opens (talk 3)
		{At: at(20, 9, 20), Out: true},                // 5 min
		{At: at(22, 10, 0), From: "Tisa"},             // waiting since (talk 4)
		{At: at(22, 10, 1), Failed: true, Out: true},
	}
	asks := []Ask{{Intent: "ask_stock", Items: []Item{{Name: "NVR 8ch", Qty: 2}}}, {Intent: "ask_stock", Items: []Item{{Name: "nvr 8ch", Qty: 1}}}, {Intent: "order"}}
	s := Compute(msgs, asks, func(k string) string { return map[string]string{"ask_stock": "Tanya stok", "order": "Minta order"}[k] }, now, wib)

	if s.Messages != 10 || s.Inbound != 5 || s.Outbound != 5 {
		t.Fatalf("counts %d %d %d", s.Messages, s.Inbound, s.Outbound)
	}
	r := s.Reply
	if r.Turns != 4 || r.Answered != 3 || *r.MedianMin != 30 || *r.Within1hPct != 66 || r.WaitingSince == nil || !r.WaitingSince.Equal(at(22, 10, 0)) {
		t.Fatalf("reply %+v median %v", r, *r.MedianMin)
	}
	if s.Sessions != 4 || s.CustomerOpen != 3 {
		t.Fatalf("talks %d opened by customer %d", s.Sessions, s.CustomerOpen)
	}
	if s.Weekdays[1] != 3 || s.Hours[9] != 3 || s.ActiveDays != 4 {
		t.Fatalf("when: weekdays %v hours[9] %d days %d", s.Weekdays, s.Hours[9], s.ActiveDays)
	}
	if len(s.Items) != 1 || s.Items[0].N != 2 || s.Items[0].Qty != 3 || s.Intents[0].Name != "Tanya stok" || s.Intents[0].N != 2 {
		t.Fatalf("asked: %+v %+v", s.Items, s.Intents)
	}
	if len(s.Months) != 6 || s.Months[5].Month != "2026-10" || s.Months[5].In != 5 {
		t.Fatalf("months %+v", s.Months)
	}
	text := ""
	for _, i := range s.Insights {
		text += i.Tone + ": " + i.Text + "\n"
	}
	for _, want := range []string{"belum dibalas sejak 2 jam", "median) dalam 30 menit", "NVR 8ch (2×)", "Permintaan terbanyak: Tanya stok (66%", "Pelanggan memulai 75% dari 4 percakapan"} {
		if !strings.Contains(text, want) {
			t.Errorf("insight %q missing in:\n%s", want, text)
		}
	}
}

func TestComputeEmptyAndTrend(t *testing.T) {
	now := at(22, 12, 0)
	if s := Compute(nil, nil, func(string) string { return "" }, now, wib); s.Messages != 0 || len(s.Insights) != 0 || s.Reply.MedianMin != nil {
		t.Fatalf("empty: %+v", s)
	}
	var msgs []Msg
	for i := range 4 {
		msgs = append(msgs, Msg{At: now.AddDate(0, 0, -40-i)}) // 4 in the 30 days before
	}
	for i := range 8 {
		msgs = append(msgs, Msg{At: now.AddDate(0, 0, -10+i)}) // 8 in the last 30
	}
	s := Compute(msgs, nil, func(string) string { return "" }, now, wib)
	if s.TrendPct == nil || *s.TrendPct != 100 {
		t.Fatalf("trend %v", s.TrendPct)
	}
	if !strings.Contains(s.Insights[len(s.Insights)-1].Text+s.Insights[0].Text+s.Insights[1].Text, "Aktivitas naik 100%") {
		t.Fatalf("insights %+v", s.Insights)
	}
}
