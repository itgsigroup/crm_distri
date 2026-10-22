// Package chatstats computes the Statistik of one conversation (Sam 22 Oct: clearer, more detail, many insights):
// volume and trend, how fast the team answers, when the customer writes, who starts the talks, what is asked, who
// speaks in a group — over the whole stored history, with insight lines written from the numbers (rules, not AI).
package chatstats

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// Msg is one stored message. Ours: sent by GSI (out) or a GSI employee in a customer group (internal).
type Msg struct {
	At        time.Time
	Out       bool
	Internal  bool
	Holding   bool // "tunggu sebentar": not an answer (ADR 0034)
	AutoReply bool
	Closing   bool // "terima kasih", "ok": needs no answer
	Failed    bool
	Media     bool
	From      string // who wrote it (a customer in a group)
}

// Ask is what Analisa WhatsApp read on a customer message: the intent and the items asked.
type Ask struct {
	Intent string
	Items  []Item
}

// Item is a product asked, with its quantity (0 unknown).
type Item struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

// Stats is the Statistik tab.
type Stats struct {
	Messages     int        `json:"messages"`
	Inbound      int        `json:"inbound"`
	Outbound     int        `json:"outbound"`
	Media        int        `json:"media"`
	FirstAt      *time.Time `json:"first_at"`
	LastAt       *time.Time `json:"last_at"`
	LastInAt     *time.Time `json:"last_in_at"`
	ActiveDays   int        `json:"active_days"`
	Last30       int        `json:"last_30"`
	Prev30       int        `json:"prev_30"`
	TrendPct     *int       `json:"trend_pct"` // last 30 days against the 30 before, nil when too few
	Reply        Reply      `json:"reply"`
	Sessions     int        `json:"sessions"`       // talks: messages with less than SessionGap between them
	CustomerOpen int        `json:"customer_opens"` // talks the customer started
	Months       []Month    `json:"months"`         // the last 6 calendar months, oldest first
	Hours        [24]int    `json:"hours"`          // customer messages per hour of day (WIB)
	Weekdays     [7]int     `json:"weekdays"`       // customer messages per weekday, Monday first
	OffHoursPct  int        `json:"off_hours_pct"`  // customer messages outside 08–17 WIB Mon–Sat
	Intents      []Count    `json:"intents"`
	Items        []Count    `json:"items"`
	Senders      []Count    `json:"senders"` // customers who write in a group, most first
	Insights     []Insight  `json:"insights"`
}

// Reply is how the team answers a customer's turn (the first customer message after our last answer).
type Reply struct {
	Turns        int        `json:"turns"`
	Answered     int        `json:"answered"`
	MedianMin    *int       `json:"median_min"`
	P90Min       *int       `json:"p90_min"`
	Within1hPct  *int       `json:"within_1h_pct"`
	WaitingSince *time.Time `json:"waiting_since"` // a customer turn not answered yet
}

// Month is one calendar month's messages.
type Month struct {
	Month string `json:"month"` // 2026-10
	In    int    `json:"in"`
	Out   int    `json:"out"`
}

// Count is a name with how often it came.
type Count struct {
	Name string `json:"name"`
	N    int    `json:"n"`
	Qty  int    `json:"qty,omitempty"`
}

// Insight is one line read from the numbers; Tone good | warn | bad | info.
type Insight struct {
	Tone string `json:"tone"`
	Text string `json:"text"`
}

// SessionGap separates two talks.
const SessionGap = 8 * time.Hour

var days = [7]string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}

// Compute reads the messages (oldest first) and the asks; now is the clock, loc the business time zone (WIB).
func Compute(msgs []Msg, asks []Ask, intentLabel func(string) string, now time.Time, loc *time.Location) Stats {
	s := Stats{Months: []Month{}, Intents: []Count{}, Items: []Count{}, Senders: []Count{}, Insights: []Insight{}}
	// the last 6 months, oldest first
	cur := time.Date(now.In(loc).Year(), now.In(loc).Month(), 1, 0, 0, 0, 0, loc)
	monthIdx := map[string]int{}
	for i := 5; i >= 0; i-- {
		k := cur.AddDate(0, -i, 0).Format("2006-01")
		monthIdx[k] = len(s.Months)
		s.Months = append(s.Months, Month{Month: k})
	}
	activeDays := map[string]bool{}
	senders := map[string]int{}
	var waits []float64
	var turnAt *time.Time // the customer turn waiting for an answer
	var prev time.Time
	for i, m := range msgs {
		if m.Failed {
			continue
		}
		ours := m.Out || m.Internal
		s.Messages++
		if ours {
			s.Outbound++
		} else {
			s.Inbound++
		}
		if m.Media {
			s.Media++
		}
		at := m.At
		if s.FirstAt == nil {
			s.FirstAt = &msgs[i].At
		}
		s.LastAt = &msgs[i].At
		local := at.In(loc)
		activeDays[local.Format("2006-01-02")] = true
		if k, ok := monthIdx[local.Format("2006-01")]; ok {
			if ours {
				s.Months[k].Out++
			} else {
				s.Months[k].In++
			}
		}
		age := now.Sub(at)
		if age <= 30*24*time.Hour {
			s.Last30++
		} else if age <= 60*24*time.Hour {
			s.Prev30++
		}
		// a talk starts after a long silence; who wrote first
		if prev.IsZero() || at.Sub(prev) > SessionGap {
			s.Sessions++
			if !ours {
				s.CustomerOpen++
			}
		}
		prev = at
		if !ours {
			s.LastInAt = &msgs[i].At
			s.Hours[local.Hour()]++
			wd := (int(local.Weekday()) + 6) % 7
			s.Weekdays[wd]++
			if wd == 6 || local.Hour() < 8 || local.Hour() >= 17 {
				s.OffHoursPct++ // counted here, made a share below
			}
			if name := strings.TrimSpace(m.From); name != "" {
				senders[name]++
			}
			if turnAt == nil && !m.Closing && !m.AutoReply {
				t := at
				turnAt = &t
				s.Reply.Turns++
			}
			continue
		}
		// our answer closes the customer's turn (a holding reply or an automatic one does not)
		if turnAt != nil && !m.Holding && !m.AutoReply {
			waits = append(waits, at.Sub(*turnAt).Minutes())
			s.Reply.Answered++
			turnAt = nil
		}
	}
	s.ActiveDays = len(activeDays)
	if s.Inbound > 0 {
		s.OffHoursPct = s.OffHoursPct * 100 / s.Inbound
	}
	if s.Last30+s.Prev30 >= 10 && s.Prev30 > 0 {
		t := (s.Last30 - s.Prev30) * 100 / s.Prev30
		s.TrendPct = &t
	}
	s.Reply.WaitingSince = turnAt
	if len(waits) > 0 {
		sort.Float64s(waits)
		med, p90 := int(waits[len(waits)/2]+0.5), int(waits[min(len(waits)-1, len(waits)*9/10)]+0.5)
		in1h := 0
		for _, w := range waits {
			if w <= 60 {
				in1h++
			}
		}
		pct := in1h * 100 / len(waits)
		s.Reply.MedianMin, s.Reply.P90Min, s.Reply.Within1hPct = &med, &p90, &pct
	}
	// what is asked
	intents, items := map[string]int{}, map[string]*Count{}
	for _, a := range asks {
		if a.Intent != "" && a.Intent != "none" && a.Intent != "other" {
			intents[a.Intent]++
		}
		for _, it := range a.Items {
			n := strings.TrimSpace(it.Name)
			if n == "" {
				continue
			}
			k := strings.ToLower(n)
			if items[k] == nil {
				items[k] = &Count{Name: n}
			}
			items[k].N++
			items[k].Qty += max(0, it.Qty)
		}
	}
	for k, n := range intents {
		s.Intents = append(s.Intents, Count{Name: intentLabel(k), N: n})
	}
	for _, c := range items {
		s.Items = append(s.Items, *c)
	}
	for k, n := range senders {
		s.Senders = append(s.Senders, Count{Name: k, N: n})
	}
	byN := func(a, b Count) int {
		if a.N != b.N {
			return b.N - a.N
		}
		return strings.Compare(a.Name, b.Name)
	}
	slices.SortFunc(s.Intents, byN)
	slices.SortFunc(s.Items, byN)
	slices.SortFunc(s.Senders, byN)
	s.Items = s.Items[:min(len(s.Items), 8)]
	s.Senders = s.Senders[:min(len(s.Senders), 8)]
	s.Insights = insights(s, now)
	return s
}

func insights(s Stats, now time.Time) []Insight {
	out := []Insight{}
	add := func(tone, f string, a ...any) { out = append(out, Insight{Tone: tone, Text: fmt.Sprintf(f, a...)}) }
	if s.Messages == 0 {
		return out
	}
	// waiting now
	if w := s.Reply.WaitingSince; w != nil {
		tone := "warn"
		if now.Sub(*w) > 24*time.Hour {
			tone = "bad"
		}
		add(tone, "Pesan pelanggan belum dibalas sejak %s lalu — balas dulu sebelum follow-up lain.", Dur(now.Sub(*w)))
	}
	// speed of answers
	if r := s.Reply; r.MedianMin != nil && r.Answered >= 3 {
		tone := "good"
		if *r.MedianMin > 60 {
			tone = "warn"
		}
		if *r.MedianMin > 240 {
			tone = "bad"
		}
		add(tone, "Tim membalas rata-rata (median) dalam %s; %d%% dibalas di bawah 1 jam dari %d giliran pelanggan.", Dur(time.Duration(*r.MedianMin)*time.Minute), *r.Within1hPct, r.Answered)
		if *r.P90Min > 24*60 {
			add("warn", "10%% balasan paling lambat butuh lebih dari %s — ada pesan yang terlewat.", Dur(time.Duration(*r.P90Min)*time.Minute))
		}
	}
	// trend
	if t := s.TrendPct; t != nil {
		switch {
		case *t >= 20:
			add("good", "Aktivitas naik %d%%: %d pesan 30 hari terakhir, %d pada 30 hari sebelumnya.", *t, s.Last30, s.Prev30)
		case *t <= -20:
			add("warn", "Aktivitas turun %d%%: %d pesan 30 hari terakhir, %d pada 30 hari sebelumnya — perlu disapa.", -*t, s.Last30, s.Prev30)
		default:
			add("info", "Aktivitas stabil: %d pesan 30 hari terakhir (sebelumnya %d).", s.Last30, s.Prev30)
		}
	}
	// silence
	if s.LastInAt != nil {
		if d := now.Sub(*s.LastInAt); d > 14*24*time.Hour {
			add("warn", "Pelanggan tidak menghubungi selama %s — waktunya follow-up.", Dur(d))
		}
	}
	// when they write
	if s.Inbound >= 5 {
		h := bestWindow(s.Hours)
		d := 0
		for i, n := range s.Weekdays {
			if n > s.Weekdays[d] {
				d = i
			}
		}
		add("info", "Pelanggan paling aktif hari %s, pukul %02d.00–%02d.00 WIB — waktu terbaik menghubungi.", days[d], h, (h+2)%24)
		if s.OffHoursPct >= 30 {
			add("warn", "%d%% pesan pelanggan datang di luar jam kerja (sebelum 08.00, setelah 17.00, atau Minggu).", s.OffHoursPct)
		}
	}
	// who starts
	if s.Sessions >= 3 {
		p := s.CustomerOpen * 100 / s.Sessions
		switch {
		case p >= 75:
			add("warn", "Pelanggan memulai %d%% dari %d percakapan — tim jarang menyapa lebih dulu.", p, s.Sessions)
		case p <= 30:
			add("good", "Tim memulai %d%% dari %d percakapan — follow-up proaktif berjalan.", 100-p, s.Sessions)
		default:
			add("info", "Percakapan dimulai seimbang: pelanggan %d%%, tim %d%% dari %d percakapan.", p, 100-p, s.Sessions)
		}
	}
	// what they ask
	if len(s.Items) > 0 {
		var names []string
		for _, c := range s.Items[:min(3, len(s.Items))] {
			names = append(names, fmt.Sprintf("%s (%d×)", c.Name, c.N))
		}
		add("info", "Barang paling sering disebut: %s.", strings.Join(names, ", "))
	}
	if len(s.Intents) > 0 {
		total := 0
		for _, c := range s.Intents {
			total += c.N
		}
		add("info", "Permintaan terbanyak: %s (%d%% dari %d pesan yang terbaca).", s.Intents[0].Name, s.Intents[0].N*100/total, total)
	}
	if len(s.Senders) >= 2 {
		add("info", "%d pelanggan aktif menulis di grup; paling aktif %s (%d pesan).", len(s.Senders), s.Senders[0].Name, s.Senders[0].N)
	}
	return out
}

// bestWindow is the start of the busiest two hours.
func bestWindow(h [24]int) int {
	best, at := -1, 0
	for i := range 24 {
		if n := h[i] + h[(i+1)%24]; n > best {
			best, at = n, i
		}
	}
	return at
}

// Dur is "45 menit", "5 jam", "3 hari".
func Dur(d time.Duration) string {
	m := int(d.Minutes() + 0.5)
	switch {
	case m < 60:
		return fmt.Sprintf("%d menit", max(1, m))
	case m < 48*60:
		return fmt.Sprintf("%d jam", m/60)
	default:
		return fmt.Sprintf("%d hari", m/(24*60))
	}
}
