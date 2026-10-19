// Package wareport builds Report WhatsApp (ADR 0050): how each sales handles their WhatsApp conversations over a
// period — response speed, SLA, unanswered and hanging chats, broken "cek dulu" promises, new numbers, order requests
// that became orders — ranked into a score with good/bad feedback. Every number is measured from the stored chats
// with the same reply logic as Analisa WhatsApp (waanalysis); no model writes any of it.
package wareport

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/waanalysis"
)

// NoSales labels conversations on a number held by no sales (admin, billing, unassigned).
const NoSales = "Tanpa sales"

// Config is the period and the rules the report measures against.
type Config struct {
	Now, Since time.Time
	Days       int
	Hours      waanalysis.WorkHours
	SLAMin     int // first-response target in working minutes (policy chat.rules.reply_sla_min)
	HangMin    int // a customer waiting this many working minutes (or a promise kept open) is hanging
	WinDays    int // an order request is won when the dealer orders within this many days
}

// Order is a dealer's order (for won order requests).
type Order struct {
	DealerID uuid.UUID
	At       time.Time
	Total    int64
}

// Report is the whole page.
type Report struct {
	Since      time.Time  `json:"since"`
	Until      time.Time  `json:"until"`
	Days       int        `json:"days"`
	SLAMin     int        `json:"sla_min"`
	HangMin    int        `json:"hang_min"`
	WinDays    int        `json:"win_days"`
	Team       SalesRow   `json:"team"`
	Sales      []SalesRow `json:"sales"`
	Daily      []Day      `json:"daily"`
	Heat       [7][24]int `json:"heat"` // customer messages by weekday (0 Sunday) × hour, WIB
	Hanging    []Hang     `json:"hanging"`
	NewNumbers []NewNum   `json:"new_numbers"`
	Insights   []string   `json:"insights"` // for the director: what stands out across the team
}

// SalesRow is one sales' card (and the team total).
type SalesRow struct {
	Name     string `json:"name"`
	Rank     int    `json:"rank"`
	Score    int    `json:"score"` // 0–100, see score()
	Threads  int    `json:"threads"`
	Dealers  int    `json:"dealers"`
	New      int    `json:"new_numbers"`
	Groups   int    `json:"groups"`
	Inbound  int    `json:"inbound"`
	Outbound int    `json:"outbound"`
	// Requests: customer turns that wanted an answer (a run of customer messages); Answered of them got a real reply,
	// WithinSLA within SLAMin working minutes
	Requests     int `json:"requests"`
	Answered     int `json:"answered"`
	AnsweredWork int `json:"answered_work"` // of them, asked within working hours (the SLA counts only these)
	WithinSLA    int `json:"within_sla"`
	// first-response time in working minutes over the answered requests; OffMedian: requests outside working hours
	// (wall-clock minutes until the reply)
	ReplyMedian *int     `json:"reply_median_min"`
	ReplyP90    *int     `json:"reply_p90_min"`
	OffMedian   *int     `json:"off_median_min"`
	NewReply    *int     `json:"new_reply_median_min"` // first answer to a new number
	Waiting     int      `json:"waiting"`              // customers waiting for a reply now
	Hanging     int      `json:"hanging"`              // of them, waiting ≥ HangMin working minutes
	Promises    int      `json:"promises"`             // "cek dulu / mohon ditunggu" sent, no real answer after ≥ HangMin
	OrderAsks   int      `json:"order_asks"`           // dealer conversations with an order request
	OrderWon    int      `json:"order_won"`            // of them, the dealer ordered within WinDays
	OrderValue  int64    `json:"order_value"`
	Good        []string `json:"good"`
	Bad         []string `json:"bad"`

	work, off, newFirst []int
	dealers             map[uuid.UUID]bool
}

// Day is one day of the period.
type Day struct {
	Date        string         `json:"date"` // YYYY-MM-DD WIB
	Inbound     int            `json:"inbound"`
	Outbound    int            `json:"outbound"`
	ReplyMedian *int           `json:"reply_median_min"`
	BySales     map[string]int `json:"by_sales"` // customer messages per sales
	work        []int
}

// Hang is a conversation left hanging: a customer waiting, or a promise not kept.
type Hang struct {
	ThreadID   uuid.UUID `json:"thread_id"`
	Title      string    `json:"title"`
	DealerSlug string    `json:"dealer_slug,omitempty"`
	Kind       string    `json:"kind"` // dealer | new | group
	Sales      string    `json:"sales"`
	Why        string    `json:"why"` // menunggu | janji
	Since      time.Time `json:"since"`
	WaitMin    int       `json:"wait_min"` // working minutes
	LastText   string    `json:"last_text"`
	Intent     string    `json:"intent"`
}

// NewNum is a new number that wrote in the period.
type NewNum struct {
	ThreadID   uuid.UUID `json:"thread_id"`
	Title      string    `json:"title"`
	Sales      string    `json:"sales"`
	FirstAt    time.Time `json:"first_at"`
	Inbound    int       `json:"inbound"`
	FirstReply *int      `json:"first_reply_min"` // working minutes to the first real answer; nil: not answered
	Intent     string    `json:"intent"`
	LastText   string    `json:"last_text"`
}

// Build measures the report over the threads (messages within the period, oldest first) and the dealers' orders.
func Build(threads []waanalysis.Thread, orders []Order, c Config) Report {
	if c.SLAMin <= 0 {
		c.SLAMin = 30
	}
	if c.HangMin <= 0 {
		c.HangMin = int(waanalysis.ReplyDue / time.Minute)
	}
	if c.WinDays <= 0 {
		c.WinDays = 3
	}
	rep := Report{Since: c.Since, Until: c.Now, Days: c.Days, SLAMin: c.SLAMin, HangMin: c.HangMin, WinDays: c.WinDays,
		Sales: []SalesRow{}, Hanging: []Hang{}, NewNumbers: []NewNum{}, Insights: []string{}}
	byDealer := map[uuid.UUID][]Order{}
	for _, o := range orders {
		byDealer[o.DealerID] = append(byDealer[o.DealerID], o)
	}
	rows := map[string]*SalesRow{}
	row := func(name string) *SalesRow {
		if name == "" {
			name = NoSales
		}
		r, ok := rows[name]
		if !ok {
			r = &SalesRow{Name: name, dealers: map[uuid.UUID]bool{}}
			rows[name] = r
		}
		return r
	}
	days := map[string]*Day{}
	day := func(t time.Time) *Day {
		k := t.In(clock.WIB).Format("2006-01-02")
		d, ok := days[k]
		if !ok {
			d = &Day{Date: k, BySales: map[string]int{}}
			days[k] = d
		}
		return d
	}
	for i := 0; i <= c.Days && c.Days > 0; i++ { // every day of the period appears, quiet ones too
		day(c.Since.AddDate(0, 0, i))
	}
	for k := range days {
		if k > c.Now.In(clock.WIB).Format("2006-01-02") {
			delete(days, k)
		}
	}

	// the reply clock runs on the period's working hours (Analyze reads them from each thread)
	threads = append([]waanalysis.Thread(nil), threads...)
	for i := range threads {
		if threads[i].Hours.Days == nil {
			threads[i].Hours = c.Hours
		}
	}
	insights := waanalysis.Analyze(threads, nil, c.Now)
	at := map[uuid.UUID]waanalysis.Insight{}
	for _, in := range insights {
		at[in.ThreadID] = in
	}
	for _, t := range threads {
		in := at[t.ID]
		r := row(t.SalesName)
		r.Threads++
		switch t.Kind {
		case "new":
			r.New++
		case "group":
			r.Groups++
		}
		if t.DealerID != nil {
			r.dealers[*t.DealerID] = true
		}
		// requests and reply times: the same runs as Analisa WhatsApp
		r.work = append(r.work, in.ReplyWorkMin...)
		r.off = append(r.off, in.ReplyOffMin...)
		r.Answered += len(in.ReplyWorkMin) + len(in.ReplyOffMin)
		r.Requests += len(in.ReplyWorkMin) + len(in.ReplyOffMin)
		for _, m := range in.ReplyWorkMin {
			if m <= c.SLAMin {
				r.WithinSLA++
			}
		}
		for _, m := range t.Messages {
			d := day(m.At)
			if m.In {
				r.Inbound++
				d.Inbound++
				d.BySales[r.Name]++
				w := m.At.In(clock.WIB)
				rep.Heat[w.Weekday()][w.Hour()]++
			} else {
				r.Outbound++
				d.Outbound++
			}
		}
		for _, run := range replyRuns(t, c) { // per day reply medians (working hours)
			d := day(run.start)
			d.work = append(d.work, run.min)
		}
		last := lastReal(t)
		if in.WaitingSince != nil {
			r.Requests++
			r.Waiting++
			wait := waitMin(c, *in.WaitingSince)
			if wait >= c.HangMin {
				why := "menunggu"
				if promised(t) {
					why = "janji"
					r.Promises++
				} else {
					r.Hanging++
				}
				rep.Hanging = append(rep.Hanging, Hang{ThreadID: t.ID, Title: title(t), DealerSlug: t.DealerSlug, Kind: t.Kind, Sales: r.Name, Why: why,
					Since: *in.WaitingSince, WaitMin: wait, LastText: last, Intent: in.Intent})
			}
		}
		if t.Kind == "new" && len(t.Messages) > 0 {
			n := NewNum{ThreadID: t.ID, Title: title(t), Sales: r.Name, FirstAt: firstIn(t), Inbound: in.Inbound, Intent: in.Intent, LastText: last}
			if m, ok := firstReply(t, c); ok {
				n.FirstReply = &m
				r.newFirst = append(r.newFirst, m)
			}
			rep.NewNumbers = append(rep.NewNumbers, n)
		}
		// order requests: a dealer conversation with a customer message asking to order; won when the dealer orders
		// within WinDays after the first ask
		if t.DealerID != nil {
			if ask, ok := firstOrderAsk(t); ok {
				r.OrderAsks++
				for _, o := range byDealer[*t.DealerID] {
					if !o.At.Before(ask) && o.At.Sub(ask) <= time.Duration(c.WinDays)*24*time.Hour {
						r.OrderWon++
						r.OrderValue += o.Total
						break
					}
				}
			}
		}
	}

	team := &SalesRow{Name: "Semua sales", dealers: map[uuid.UUID]bool{}}
	for _, r := range rows {
		r.Dealers = len(r.dealers)
		r.AnsweredWork = len(r.work)
		r.ReplyMedian, r.ReplyP90 = median(r.work), percentile(r.work, 90)
		r.OffMedian, r.NewReply = median(r.off), median(r.newFirst)
		team.Threads += r.Threads
		team.New += r.New
		team.Groups += r.Groups
		team.Inbound += r.Inbound
		team.Outbound += r.Outbound
		team.Requests += r.Requests
		team.Answered += r.Answered
		team.WithinSLA += r.WithinSLA
		team.Waiting += r.Waiting
		team.Hanging += r.Hanging
		team.Promises += r.Promises
		team.OrderAsks += r.OrderAsks
		team.OrderWon += r.OrderWon
		team.OrderValue += r.OrderValue
		team.work = append(team.work, r.work...)
		team.off = append(team.off, r.off...)
		team.newFirst = append(team.newFirst, r.newFirst...)
		for d := range r.dealers {
			team.dealers[d] = true
		}
	}
	team.Dealers = len(team.dealers)
	team.AnsweredWork = len(team.work)
	team.ReplyMedian, team.ReplyP90 = median(team.work), percentile(team.work, 90)
	team.OffMedian, team.NewReply = median(team.off), median(team.newFirst)
	team.Score = score(*team, c)

	for _, r := range rows {
		r.Score = score(*r, c)
		r.Good, r.Bad = feedback(*r, *team, c)
		rep.Sales = append(rep.Sales, *r)
	}
	// ranked: the score, then more customers handled; a number held by no sales is listed last, unranked
	sort.SliceStable(rep.Sales, func(i, j int) bool {
		a, b := rep.Sales[i], rep.Sales[j]
		if (a.Name == NoSales) != (b.Name == NoSales) {
			return b.Name == NoSales
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Requests != b.Requests {
			return a.Requests > b.Requests
		}
		return a.Name < b.Name
	})
	rank := 0
	for i := range rep.Sales {
		if rep.Sales[i].Name != NoSales && rep.Sales[i].Requests > 0 {
			rank++
			rep.Sales[i].Rank = rank
		}
	}
	rep.Team = *team

	for _, d := range days {
		d.ReplyMedian = median(d.work)
		rep.Daily = append(rep.Daily, *d)
	}
	sort.Slice(rep.Daily, func(i, j int) bool { return rep.Daily[i].Date < rep.Daily[j].Date })
	sort.SliceStable(rep.Hanging, func(i, j int) bool { return rep.Hanging[i].WaitMin > rep.Hanging[j].WaitMin })
	sort.SliceStable(rep.NewNumbers, func(i, j int) bool { return rep.NewNumbers[i].FirstAt.After(rep.NewNumbers[j].FirstAt) })
	rep.Insights = teamInsights(rep, c)
	return rep
}

// score: 0–100 from what a customer feels — answered (35), within SLA (30), speed against the SLA (20), and
// nothing left hanging (15). A sales without requests in the period scores 0 and is not ranked.
func score(r SalesRow, c Config) int {
	if r.Requests == 0 {
		return 0
	}
	answered := float64(r.Answered) / float64(r.Requests)
	sla := 0.0
	if len(r.work) > 0 {
		sla = float64(r.WithinSLA) / float64(len(r.work))
	} else if r.Answered > 0 {
		sla = 0.5 // answered only outside working hours: neither good nor bad
	}
	speed := 0.5
	if r.ReplyMedian != nil {
		speed = 1 - float64(*r.ReplyMedian)/float64(4*c.SLAMin)
		speed = max(0, min(1, speed))
	}
	open := float64(r.Hanging+r.Promises) / float64(r.Requests)
	tidy := max(0, 1-2*open)
	return int(35*answered + 30*sla + 20*speed + 15*tidy + 0.5)
}

// feedback: what this sales does well and what needs fixing, from the numbers (and against the team).
func feedback(r SalesRow, team SalesRow, c Config) (good, bad []string) {
	good, bad = []string{}, []string{}
	if r.Requests == 0 {
		return good, append(bad, "Belum ada chat pelanggan di periode ini")
	}
	if len(r.work) > 0 {
		pct := pctOf(r.WithinSLA, len(r.work))
		switch {
		case pct >= 80:
			good = append(good, fmt.Sprintf("%d%% chat dibalas ≤ %s di jam kerja", pct, waanalysis.WaitLabel(c.SLAMin)))
		case pct < 50:
			bad = append(bad, fmt.Sprintf("Hanya %d%% chat dibalas ≤ %s di jam kerja", pct, waanalysis.WaitLabel(c.SLAMin)))
		}
	}
	if r.ReplyMedian != nil && team.ReplyMedian != nil {
		switch {
		case *r.ReplyMedian <= c.SLAMin && *r.ReplyMedian < *team.ReplyMedian:
			good = append(good, fmt.Sprintf("Balas cepat: median %s (tim %s)", waanalysis.WaitLabel(*r.ReplyMedian), waanalysis.WaitLabel(*team.ReplyMedian)))
		case *r.ReplyMedian > 2*c.SLAMin:
			bad = append(bad, fmt.Sprintf("Balas lambat: median %s, target ≤ %s", waanalysis.WaitLabel(*r.ReplyMedian), waanalysis.WaitLabel(c.SLAMin)))
		}
	}
	if r.ReplyP90 != nil && *r.ReplyP90 > 4*c.SLAMin {
		bad = append(bad, fmt.Sprintf("1 dari 10 chat baru dibalas setelah %s", waanalysis.WaitLabel(*r.ReplyP90)))
	}
	if r.Hanging == 0 && r.Promises == 0 {
		good = append(good, "Tidak ada chat gantung")
	}
	if r.Hanging > 0 {
		bad = append(bad, fmt.Sprintf("%d chat pelanggan menunggu ≥ %s tanpa balasan", r.Hanging, waanalysis.WaitLabel(c.HangMin)))
	}
	if r.Promises > 0 {
		bad = append(bad, fmt.Sprintf("%d janji \"cek dulu / mohon ditunggu\" belum ditindaklanjuti ≥ %s", r.Promises, waanalysis.WaitLabel(c.HangMin)))
	}
	if r.New > 0 {
		if r.NewReply != nil && *r.NewReply <= c.SLAMin {
			good = append(good, fmt.Sprintf("%d nomor baru, dibalas pertama kali dalam %s", r.New, waanalysis.WaitLabel(*r.NewReply)))
		} else if r.NewReply != nil {
			bad = append(bad, fmt.Sprintf("Nomor baru menunggu %s sebelum dibalas pertama kali", waanalysis.WaitLabel(*r.NewReply)))
		}
	}
	if r.OrderAsks > 0 {
		pct := pctOf(r.OrderWon, r.OrderAsks)
		if r.OrderWon > 0 {
			good = append(good, fmt.Sprintf("%d dari %d permintaan order jadi SO (%d%%)", r.OrderWon, r.OrderAsks, pct))
		}
		if pct < 50 && r.OrderAsks >= 2 {
			bad = append(bad, fmt.Sprintf("%d permintaan order belum jadi SO dalam %d hari", r.OrderAsks-r.OrderWon, c.WinDays))
		}
	}
	if r.OffMedian != nil && *r.OffMedian > 12*60 {
		bad = append(bad, fmt.Sprintf("Chat di luar jam kerja baru dibalas setelah %s", waanalysis.WaitLabel(*r.OffMedian)))
	}
	return good, bad
}

// teamInsights: what a director should see first.
func teamInsights(rep Report, c Config) []string {
	out := []string{}
	t := rep.Team
	if t.Requests == 0 {
		return append(out, "Belum ada chat pelanggan di periode ini.")
	}
	if len(t.work) > 0 {
		out = append(out, fmt.Sprintf("Tim membalas %d%% chat ≤ %s di jam kerja; median %s.", pctOf(t.WithinSLA, len(t.work)), waanalysis.WaitLabel(c.SLAMin), labelOf(t.ReplyMedian)))
	}
	var best, worst *SalesRow
	for i := range rep.Sales {
		s := &rep.Sales[i]
		if s.Rank == 0 || s.Requests < 3 {
			continue
		}
		if best == nil || s.Score > best.Score {
			best = s
		}
		if worst == nil || s.Score < worst.Score {
			worst = s
		}
	}
	if best != nil {
		out = append(out, fmt.Sprintf("Sales terbaik: %s (skor %d).", best.Name, best.Score))
	}
	if worst != nil && best != nil && worst.Name != best.Name {
		out = append(out, fmt.Sprintf("Perlu dibantu: %s (skor %d) — %s.", worst.Name, worst.Score, strings.Join(firstN(worst.Bad, 1), "")))
	}
	if n := t.Hanging + t.Promises; n > 0 {
		out = append(out, fmt.Sprintf("%d chat gantung sekarang (%d menunggu, %d janji belum ditepati).", n, t.Hanging, t.Promises))
	}
	if t.OrderAsks > 0 {
		out = append(out, fmt.Sprintf("%d dari %d permintaan order di chat jadi SO dalam %d hari.", t.OrderWon, t.OrderAsks, c.WinDays))
	}
	// the busiest hour for customers
	bd, bh, bn := 0, 0, 0
	for d := range rep.Heat {
		for h, n := range rep.Heat[d] {
			if n > bn {
				bd, bh, bn = d, h, n
			}
		}
	}
	if bn > 0 {
		out = append(out, fmt.Sprintf("Pelanggan paling ramai chat %s pukul %02d.00–%02d.00.", dayName[bd], bh, bh+1))
	}
	return out
}

var dayName = [7]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

type run struct {
	start time.Time
	min   int
}

// replyRuns: the working-hours reply times with the time the customer's turn started (per-day medians).
func replyRuns(t waanalysis.Thread, c Config) []run {
	var out []run
	var start *time.Time
	for _, m := range t.Messages {
		switch {
		case m.In && (m.Closing || waanalysis.IsAck(m.Text)):
		case m.In:
			if start == nil {
				at := m.At
				start = &at
			}
		case m.Holding:
		default:
			if start != nil && c.Hours.In(*start) {
				out = append(out, run{*start, c.Hours.Minutes(*start, m.At)})
			}
			start = nil
		}
	}
	return out
}

// promised: the last message of ours is a holding reply ("cek dulu", "mohon ditunggu") after the customer's turn.
func promised(t waanalysis.Thread) bool {
	for i := len(t.Messages) - 1; i >= 0; i-- {
		m := t.Messages[i]
		if m.In {
			if m.Closing || waanalysis.IsAck(m.Text) {
				continue
			}
			return false
		}
		return m.Holding
	}
	return false
}

// firstReply: working minutes from the first customer message to our first real answer.
func firstReply(t waanalysis.Thread, c Config) (int, bool) {
	var start *time.Time
	for _, m := range t.Messages {
		if m.In && start == nil {
			at := m.At
			start = &at
			continue
		}
		if !m.In && !m.Holding && start != nil {
			if c.Hours.In(*start) {
				return c.Hours.Minutes(*start, m.At), true
			}
			return int(m.At.Sub(*start).Minutes()), true
		}
	}
	return 0, false
}

func firstIn(t waanalysis.Thread) time.Time {
	for _, m := range t.Messages {
		if m.In {
			return m.At
		}
	}
	return t.Messages[0].At
}

func firstOrderAsk(t waanalysis.Thread) (time.Time, bool) {
	for _, m := range t.Messages {
		if m.In && !m.Closing && agents.Extract(m.Text, nil).Intent == agents.IntentOrder {
			return m.At, true
		}
	}
	return time.Time{}, false
}

func lastReal(t waanalysis.Thread) string {
	for i := len(t.Messages) - 1; i >= 0; i-- {
		if m := t.Messages[i]; m.In && strings.TrimSpace(m.Text) != "" {
			return clip(m.Text, 160)
		}
	}
	return ""
}

func title(t waanalysis.Thread) string {
	if t.DealerName != "" {
		return t.DealerName
	}
	return t.Title
}

func waitMin(c Config, since time.Time) int {
	if c.Hours.Days == nil {
		return int(c.Now.Sub(since).Minutes())
	}
	return c.Hours.Minutes(since, c.Now)
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func pctOf(a, b int) int {
	if b == 0 {
		return 0
	}
	return int(float64(a)*100/float64(b) + 0.5)
}

func labelOf(m *int) string {
	if m == nil {
		return "—"
	}
	return waanalysis.WaitLabel(*m)
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func median(xs []int) *int { return percentile(xs, 50) }

// percentile: nearest-rank percentile of xs; nil when empty.
func percentile(xs []int, p int) *int {
	if len(xs) == 0 {
		return nil
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	i := (p*len(s)+99)/100 - 1
	i = max(0, min(len(s)-1, i))
	v := s[i]
	return &v
}
