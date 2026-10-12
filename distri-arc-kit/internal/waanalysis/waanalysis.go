// Package waanalysis is Analisa WhatsApp (ADR 0026): what the conversations with dealers and new numbers of the last
// days say — the request in each conversation (the extractor AI Order uses), the items asked for, who is still
// waiting for a reply and how fast each sales answers. Pure Go: the numbers come from here; the LLM only writes the
// overview and one line per conversation. Internal groups and DMs between employees are never read (09-policies).
package waanalysis

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
)

const (
	// WindowDays is how far back a cycle reads the conversations.
	WindowDays = 7
	// ReplyDue: a customer waiting longer than this for a reply needs one (Rencana hari ini).
	ReplyDue = 2 * time.Hour
	// MaxPlanSteps caps the "Balas chat" steps of Rencana hari ini; the rest stays in the Analisa WhatsApp tab.
	MaxPlanSteps = 3
	// IntentNone: the conversation has no customer message in the window (only our messages).
	IntentNone = "none"
)

// Message is one chat bubble of a conversation.
type Message struct {
	ID       uuid.UUID
	SignalID *uuid.UUID
	In       bool // the customer wrote it
	Text     string
	At       time.Time
}

// Thread is one conversation with its messages of the window, oldest first.
type Thread struct {
	ID         uuid.UUID
	Kind       string // dealer | new
	Title      string
	DealerID   *uuid.UUID
	DealerSlug string
	DealerName string
	SalesID    *uuid.UUID
	SalesName  string
	Messages   []Message
}

// Item is a product asked for in a conversation.
type Item struct {
	Name string `json:"name"`
	Qty  int64  `json:"qty"`
}

// Insight is what one conversation says.
type Insight struct {
	ThreadID       uuid.UUID   `json:"thread_id"`
	Kind           string      `json:"kind"`
	Title          string      `json:"title"`
	DealerID       *uuid.UUID  `json:"-"`
	DealerSlug     string      `json:"dealer_id,omitempty"`
	DealerName     string      `json:"dealer_name,omitempty"`
	SalesID        *uuid.UUID  `json:"-"`
	SalesName      string      `json:"sales"`
	Intent         string      `json:"intent"`
	Items          []Item      `json:"items"`
	LastText       string      `json:"last_text"`
	Inbound        int         `json:"inbound"`
	Outbound       int         `json:"outbound"`
	LastInAt       *time.Time  `json:"last_in_at"`
	LastOutAt      *time.Time  `json:"last_out_at"`
	WaitingSince   *time.Time  `json:"waiting_since"`
	WaitMin        int         `json:"wait_min"` // minutes waiting at analysis time (0 when answered)
	ReplyMedianMin *int        `json:"reply_median_min"`
	NeedsReply     bool        `json:"needs_reply"`
	Priority       int         `json:"priority"`
	SignalIDs      []uuid.UUID `json:"signal_ids"`
	ProposalIDs    []uuid.UUID `json:"proposal_ids"`
	Summary        string      `json:"summary"`
	Confidence     *int        `json:"confidence"`
}

// intentRank orders the intents of one conversation: the most consequential request names it.
var intentRank = map[string]int{agents.IntentOrder: 6, agents.IntentRelease: 5, agents.IntentReturn: 4, agents.IntentPrice: 3, agents.IntentStock: 2, agents.IntentOther: 1}

// intentWeight is the priority a request adds to a waiting conversation.
var intentWeight = map[string]int{agents.IntentOrder: 50, agents.IntentRelease: 40, agents.IntentReturn: 30, agents.IntentPrice: 20, agents.IntentStock: 20}

// Analyze reads every conversation at now against the catalog. Conversations come back most urgent first.
func Analyze(threads []Thread, catalog []agents.Product, now time.Time) []Insight {
	out := make([]Insight, 0, len(threads))
	for _, t := range threads {
		if len(t.Messages) == 0 || (t.Kind != "dealer" && t.Kind != "new") {
			continue
		}
		out = append(out, analyzeThread(t, catalog, now))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return later(out[i].LastInAt, out[j].LastInAt)
	})
	return out
}

func later(a, b *time.Time) bool {
	switch {
	case a == nil:
		return false
	case b == nil:
		return true
	}
	return a.After(*b)
}

func analyzeThread(t Thread, catalog []agents.Product, now time.Time) Insight {
	in := Insight{ThreadID: t.ID, Kind: t.Kind, Title: t.Title, DealerID: t.DealerID, DealerSlug: t.DealerSlug, DealerName: t.DealerName,
		SalesID: t.SalesID, SalesName: t.SalesName, Intent: IntentNone, Items: []Item{}, SignalIDs: []uuid.UUID{}, ProposalIDs: []uuid.UUID{}}
	var delays []int
	var runStart *time.Time // first customer message not answered yet
	var pending []Message   // the customer's messages since our last reply
	var lastIn *Message
	for i := range t.Messages {
		m := t.Messages[i]
		at := m.At
		if m.In {
			in.Inbound++
			in.LastInAt = &at
			lastIn = &t.Messages[i]
			if m.SignalID != nil {
				in.SignalIDs = append(in.SignalIDs, *m.SignalID)
			}
			if runStart == nil {
				runStart = &at
			}
			pending = append(pending, m)
			continue
		}
		in.Outbound++
		in.LastOutAt = &at
		if runStart != nil {
			delays = append(delays, int(at.Sub(*runStart).Minutes()+0.5))
		}
		runStart, pending = nil, nil
	}
	if runStart != nil && allAck(pending) {
		runStart = nil // "Ok Bu", "siap", "terima kasih 👍": the conversation is closed, nobody waits for a reply
	}
	if runStart != nil {
		in.WaitingSince = runStart
		in.WaitMin = max(0, int(now.Sub(*runStart).Minutes()))
		in.NeedsReply = now.Sub(*runStart) >= ReplyDue
	}
	in.ReplyMedianMin = median(delays)
	// the request: what the customer is waiting for, else the latest thing they asked
	read := pending
	if len(read) == 0 && lastIn != nil {
		read = []Message{*lastIn}
	}
	for _, m := range read {
		e := agents.Extract(m.Text, catalog)
		if intentRank[e.Intent] > intentRank[in.Intent] {
			in.Intent = e.Intent
			in.Items = in.Items[:0]
		}
		if e.Intent == in.Intent {
			for _, it := range e.Items {
				in.Items = addItem(in.Items, Item{Name: it.Product.Name, Qty: it.Qty})
			}
		}
	}
	if lastIn != nil {
		in.LastText = clip(lastIn.Text, 160)
	}
	if in.NeedsReply {
		in.Priority = 100 + intentWeight[in.Intent] + min(in.WaitMin/60, 48)
		if in.Kind == "dealer" {
			in.Priority += 10
		}
	} else if in.WaitingSince != nil {
		in.Priority = 50 + intentWeight[in.Intent]/2
	} else {
		in.Priority = intentWeight[in.Intent] / 5
	}
	return in
}

// ackWords are the words of a closing message that needs no reply.
var ackWords = map[string]bool{"ok": true, "oke": true, "okee": true, "okey": true, "okay": true, "okeh": true, "okwy": true, "okk": true, "okok": true, "siap": true,
	"baik": true, "sip": true, "sipp": true, "mantap": true, "mantab": true, "terima": true, "kasih": true, "makasih": true, "trims": true, "tq": true,
	"thx": true, "thanks": true, "thank": true, "you": true, "sama": true, "samaa": true, "noted": true, "iya": true, "ya": true, "yaa": true, "y": true,
	"bu": true, "pak": true, "mba": true, "mbak": true, "mas": true, "kak": true, "ko": true, "ci": true, "gan": true, "bro": true, "sis": true, "banyak": true}

// IsAck: a short closing message ("Ok Bu", "siap pak", "terima kasih", "👍") that needs no reply.
func IsAck(text string) bool {
	low := strings.ToLower(strings.TrimSpace(text))
	if low == "" || utf8.RuneCountInString(low) > 40 {
		return false
	}
	words := strings.FieldsFunc(low, func(r rune) bool { return r < 'a' || r > 'z' })
	for _, w := range words {
		if !ackWords[w] {
			return false
		}
	}
	return len(words) > 0 || !strings.ContainsAny(low, "0123456789?") // only emoji / punctuation ("👍", "🙏🏻")
}

func allAck(ms []Message) bool {
	for _, m := range ms {
		if !IsAck(m.Text) {
			return false
		}
	}
	return len(ms) > 0
}

func addItem(xs []Item, it Item) []Item {
	for i := range xs {
		if xs[i].Name == it.Name {
			xs[i].Qty += it.Qty
			return xs
		}
	}
	return append(xs, it)
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

func median(xs []int) *int {
	if len(xs) == 0 {
		return nil
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	m := s[len(s)/2]
	if len(s)%2 == 0 {
		m = (s[len(s)/2-1] + s[len(s)/2] + 1) / 2
	}
	return &m
}

// Totals are the counts over a set of conversations (all, or one sales' page).
type Totals struct {
	Threads        int            `json:"threads"`
	Dealers        int            `json:"dealers"`
	NewNumbers     int            `json:"new_numbers"`
	Inbound        int            `json:"inbound"`
	Outbound       int            `json:"outbound"`
	Waiting        int            `json:"waiting"`     // a customer message without a reply
	NeedsReply     int            `json:"needs_reply"` // waiting longer than ReplyDue
	ReplyMedianMin *int           `json:"reply_median_min"`
	ByIntent       map[string]int `json:"by_intent"` // conversations by their request
}

// SalesRow is one sales' conversations.
type SalesRow struct {
	Name           string `json:"name"`
	Threads        int    `json:"threads"`
	Inbound        int    `json:"inbound"`
	Waiting        int    `json:"waiting"`
	NeedsReply     int    `json:"needs_reply"`
	ReplyMedianMin *int   `json:"reply_median_min"` // median of the conversations' reply medians
}

// Summarize counts a set of conversations, overall and per sales (most conversations first).
func Summarize(ins []Insight) (Totals, []SalesRow) {
	t := Totals{ByIntent: map[string]int{}}
	var all []int
	type acc struct {
		row     SalesRow
		medians []int
	}
	by := map[string]*acc{}
	for _, i := range ins {
		t.Threads++
		if i.Kind == "dealer" {
			t.Dealers++
		} else {
			t.NewNumbers++
		}
		t.Inbound += i.Inbound
		t.Outbound += i.Outbound
		if i.WaitingSince != nil {
			t.Waiting++
		}
		if i.NeedsReply {
			t.NeedsReply++
		}
		if i.Intent != IntentNone {
			t.ByIntent[i.Intent]++
		}
		name := i.SalesName
		if name == "" {
			name = "Tanpa sales"
		}
		a := by[name]
		if a == nil {
			a = &acc{row: SalesRow{Name: name}}
			by[name] = a
		}
		a.row.Threads++
		a.row.Inbound += i.Inbound
		if i.WaitingSince != nil {
			a.row.Waiting++
		}
		if i.NeedsReply {
			a.row.NeedsReply++
		}
		if i.ReplyMedianMin != nil {
			all = append(all, *i.ReplyMedianMin)
			a.medians = append(a.medians, *i.ReplyMedianMin)
		}
	}
	t.ReplyMedianMin = median(all)
	rows := make([]SalesRow, 0, len(by))
	for _, a := range by {
		a.row.ReplyMedianMin = median(a.medians)
		rows = append(rows, a.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].NeedsReply != rows[j].NeedsReply {
			return rows[i].NeedsReply > rows[j].NeedsReply
		}
		if rows[i].Threads != rows[j].Threads {
			return rows[i].Threads > rows[j].Threads
		}
		return rows[i].Name < rows[j].Name
	})
	return t, rows
}

// ItemsText renders "10 Kamera IP 4MP + 2 HDD 2TB".
func ItemsText(xs []Item) string {
	var parts []string
	for _, it := range xs {
		if it.Qty > 0 {
			parts = append(parts, strconv.FormatInt(it.Qty, 10)+" "+it.Name)
		} else {
			parts = append(parts, it.Name)
		}
	}
	return strings.Join(parts, " + ")
}

// IntentLabel is the request in the words of the UI.
func IntentLabel(intent string) string {
	switch intent {
	case agents.IntentOrder:
		return "minta order"
	case agents.IntentPrice:
		return "tanya harga"
	case agents.IntentStock:
		return "tanya stok"
	case agents.IntentRelease:
		return "minta rilis kredit"
	case agents.IntentReturn:
		return "retur / klaim"
	case IntentNone:
		return "tanpa pesan pelanggan"
	}
	return "lainnya"
}

// WaitLabel renders a waiting time: "45 menit", "5 jam", "2 hari".
func WaitLabel(min int) string {
	switch {
	case min < 60:
		return strconv.Itoa(min) + " menit"
	case min < 48*60:
		return strconv.Itoa(min/60) + " jam"
	}
	return strconv.Itoa(min/(24*60)) + " hari"
}
