// Package ask answers free questions from the command bar (⌘K, POST /ask) from the data: a router picks the
// structured query (risk, stock, cash, schedule, drift), Go writes the facts with dealer links and sources, and
// the LLM may phrase them as long as every link and number survives. A dealer name opens that dealer instead.
package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// Intents of the router.
const (
	IntentRisk     = "risk"
	IntentStock    = "stock"
	IntentCash     = "cash"
	IntentSchedule = "schedule"
	IntentDrift    = "drift"
	IntentDealer   = "dealer"
	IntentGeneral  = "general"
)

var routes = []struct {
	intent string
	re     *regexp.Regexp
}{
	{IntentRisk, regexp.MustCompile(`(?i)berisiko|risiko|bahaya|rawan|masalah`)},
	{IntentDrift, regexp.MustCompile(`(?i)lewat jadwal|menjauh|churn|tidak order|berhenti order|hilang`)},
	{IntentCash, regexp.MustCompile(`(?i)kas|piutang|tagih|bayar|dso|invoice|lewat tempo`)},
	{IntentStock, regexp.MustCompile(`(?i)stok|push|menua|didorong|bundle|gudang`)},
	{IntentSchedule, regexp.MustCompile(`(?i)jadwal|siapa (yang )?order|order minggu|rekomendasi order|follow-?up`)},
}

func short(name string) string {
	for _, p := range []string{"pt ", "cv ", "ud ", "toko "} {
		name = strings.TrimPrefix(name, p)
	}
	return name
}

// Classify routes a question: a question about risk, stock, cash, schedule or drift goes to that query; a text that
// names a dealer and is not a question opens the dealer; anything else is answered from today's summary.
func Classify(q string, dealers []views.BoardItem) (string, *views.BoardItem) {
	ql := strings.ToLower(strings.TrimSpace(q))
	for _, r := range routes {
		if r.re.MatchString(ql) {
			return r.intent, nil
		}
	}
	for i := range dealers {
		n := strings.ToLower(dealers[i].Name)
		first := strings.Fields(short(n))
		if strings.Contains(n, ql) || (len(first) > 0 && len(first[0]) >= 4 && strings.Contains(ql, first[0])) {
			return IntentDealer, &dealers[i]
		}
	}
	return IntentGeneral, nil
}

// Source is a signal an answer rests on.
type Source struct {
	ID     uuid.UUID `json:"id"`
	Kind   string    `json:"kind"`
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	Dealer string    `json:"dealer,omitempty"`
}

// Answer is what the command bar shows.
type Answer struct {
	Q          string    `json:"q"`
	Intent     string    `json:"intent"`
	Text       string    `json:"text"` // with [[dealer:<slug>|Name]] links
	Dealer     string    `json:"dealer,omitempty"`
	Sources    []Source  `json:"sources"`
	Confidence float64   `json:"confidence"`
	Via        string    `json:"via"` // template | llm
	At         time.Time `json:"at"`
}

// Service answers questions.
type Service struct {
	St     *store.Store
	Clock  clock.Clock
	Router *llm.Router
}

func link(it views.BoardItem) string { return "[[dealer:" + it.ID + "|" + it.Name + "]]" }

func rp(v int64) string {
	if v >= 1_000_000_000 {
		return strings.Replace(fmt.Sprintf("Rp %.2f M", float64(v)/1e9), ".", ",", 1)
	}
	return fmt.Sprintf("Rp %d jt", int64(math.Round(float64(v)/1e6)))
}

func join(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " dan " + xs[len(xs)-1]
}

var countWord = map[int]string{1: "Satu", 2: "Dua", 3: "Tiga", 4: "Empat", 5: "Lima", 6: "Enam"}

// Ask answers one question.
func (s *Service) Ask(ctx context.Context, q string) (Answer, error) {
	b, err := views.NewBuilder(s.St, s.Clock).Board(ctx)
	if err != nil {
		return Answer{}, err
	}
	intent, d := Classify(q, b.Items)
	a := Answer{Q: q, Intent: intent, Via: "template", Confidence: 0.9, At: s.Clock.Now()}
	var quoted []views.BoardItem
	var kinds []string
	switch intent {
	case IntentDealer:
		a.Dealer, a.Text = d.ID, "Membuka "+link(*d)
		return a, nil
	case IntentRisk:
		a.Text, quoted = risk(b)
		kinds = []string{"invoice", "so", "wa"}
	case IntentDrift:
		a.Text, quoted = drift(b)
		kinds = []string{"so", "wa"}
	case IntentCash:
		a.Text, quoted = cash(b)
		kinds = []string{"invoice", "payment", "wa"}
	case IntentStock:
		st, err := s.St.Q.ListStockItems(ctx)
		if err != nil {
			return a, err
		}
		a.Text, quoted = stock(b, views.StockItems(st))
		kinds = []string{"stock", "so"}
	case IntentSchedule:
		a.Text, quoted = schedule(b)
		kinds = []string{"so", "wa"}
	default:
		br, err := s.St.Q.GetBrief(ctx, clock.Today(s.Clock.Now()))
		if err != nil {
			a.Text, a.Confidence = "Belum ada ringkasan hari ini. Coba tanyakan risiko dealer, stok yang perlu didorong, prediksi kas, atau jadwal order.", 0.5
			return a, nil
		}
		var v views.Brief
		_ = json.Unmarshal(br.Brief, &v)
		var parts []string
		for _, p := range v.Points {
			parts = append(parts, p.Title+": "+p.Text)
		}
		a.Text, a.Confidence = strings.Join(parts, " "), 0.7
		for _, p := range v.Points {
			for _, d := range p.Dealers {
				if it, ok := b.Get(d.ID); ok {
					quoted = append(quoted, it)
				}
			}
		}
		kinds = []string{"so", "invoice", "wa"}
	}
	for _, it := range quoted {
		id := it.UUID
		sigs, err := s.St.Q.DealerSignals(ctx, gen.DealerSignalsParams{DealerID: &id, Limit: 20})
		if err != nil {
			return a, err
		}
		for _, sg := range sigs {
			if containsKind(kinds, sg.Kind) {
				a.Sources = append(a.Sources, Source{ID: sg.ID, Kind: sg.Kind, At: sg.OccurredAt, Text: deref(sg.Summary), Dealer: it.ID})
				break
			}
		}
	}
	s.polish(ctx, &a)
	return a, nil
}

func containsKind(xs []string, k string) bool {
	for _, x := range xs {
		if x == k {
			return true
		}
	}
	return false
}

// risk: dealers whose credit is over limit / overdue or who are At risk, most issues first.
func risk(b *views.Board) (string, []views.BoardItem) {
	type r struct {
		it      views.BoardItem
		issues  []string
		overdue int64
	}
	var rs []r
	for _, it := range b.Items {
		m := it.Metrics
		var x r
		x.it = it
		if m.Credit.State == domain.CreditOverLimit {
			x.issues = append(x.issues, "exposure di atas limit")
		}
		for _, inv := range views.OpenInvoices(b.Data.Histories[it.UUID], b.Today) {
			if inv.LateDays > 0 {
				x.issues = append(x.issues, fmt.Sprintf("%s lewat %d hari", inv.Number, inv.LateDays))
				x.overdue += inv.Residual
				break
			}
		}
		if m.Rhythm != nil && m.Last != nil && m.Cyc > b.Policies.Orbit.Drift && m.Status != domain.StatusChurn {
			if *m.Last-*m.Rhythm >= 30 {
				x.issues = append(x.issues, fmt.Sprintf("%d hari tanpa order", *m.Last))
			} else {
				x.issues = append(x.issues, fmt.Sprintf("lewat siklus order %d hari", *m.Last-*m.Rhythm))
			}
		}
		if it.Next != nil && it.Next.Kind == domain.KindCreditRelease && it.Next.Status == "proposed" {
			x.issues = append(x.issues, "minta rilis di atas limit")
		}
		if len(x.issues) > 0 && (m.Status == domain.StatusAtRisk || metricsBad(m.Credit.State)) {
			rs = append(rs, x)
		}
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if len(rs[i].issues) != len(rs[j].issues) {
			return len(rs[i].issues) > len(rs[j].issues)
		}
		if rs[i].overdue != rs[j].overdue {
			return rs[i].overdue > rs[j].overdue
		}
		return rs[i].it.Metrics.OmzetBln > rs[j].it.Metrics.OmzetBln
	})
	if len(rs) > 3 {
		rs = rs[:3]
	}
	if len(rs) == 0 {
		return "Tidak ada dealer berisiko: semua sisa limit aman dan tidak ada yang lewat jadwal.", nil
	}
	var parts []string
	var quoted []views.BoardItem
	var overdue, lost int64
	for _, x := range rs {
		parts = append(parts, fmt.Sprintf("%s (%s)", link(x.it), strings.Join(x.issues, ", ")))
		quoted = append(quoted, x.it)
		overdue += x.overdue
		if x.it.Metrics.Cyc > b.Policies.Orbit.Drift {
			lost += x.it.Metrics.OmzetBln
		}
	}
	return fmt.Sprintf("%s dealer: %s. Piutang lewat tempo %s; potensi order hilang %s/bulan.", countWord[len(rs)], join(parts), rp(overdue), rp(lost)), quoted
}

func metricsBad(state string) bool {
	return state == domain.CreditOverLimit || state == domain.CreditOverdue
}

func drift(b *views.Board) (string, []views.BoardItem) {
	ds := b.Drift()
	if len(ds) == 0 {
		return "Tidak ada dealer yang lewat jadwal.", nil
	}
	var parts []string
	var total int64
	for _, it := range ds {
		parts = append(parts, fmt.Sprintf("%s (%d / %d hari, %s)", link(it), *it.Metrics.Last, *it.Metrics.Rhythm, it.Metrics.Status))
		total += it.Metrics.OmzetBln
	}
	return fmt.Sprintf("%d dealer lewat jadwal: %s. Potensi %s/bulan.", len(ds), join(parts), rp(total)), ds
}

func cash(b *views.Board) (string, []views.BoardItem) {
	ov, ar, _, fc, total := b.Credit(30)
	var quoted []views.BoardItem
	var late []string
	for _, r := range ar {
		if r.Overdue > 0 && len(late) < 2 {
			if it, ok := b.Get(r.DealerID); ok {
				late = append(late, fmt.Sprintf("%s (lewat %s, %d hari)", link(it), rp(r.Overdue), r.LateDays))
				quoted = append(quoted, it)
			}
		}
	}
	t := fmt.Sprintf("Prediksi kas masuk 30 hari %s dari piutang %s, tertimbang pola bayar tiap dealer.", rp(total), rp(ov.Receivable))
	if len(fc) > 0 {
		if it, ok := b.Get(fc[0].DealerID); ok {
			t += fmt.Sprintf(" Terbesar dari %s (%s).", link(it), rp(fc[0].Expected))
			quoted = append(quoted, it)
		}
	}
	if len(late) > 0 {
		t += " Yang paling menaikkan bila ditagih: " + join(late) + "."
	}
	return t, quoted
}

func stock(b *views.Board, st []domain.StockItem) (string, []views.BoardItem) {
	var parts []string
	var quoted []views.BoardItem
	var total int64
	for _, a := range b.StockAging(st, "") {
		if a.AgeDays <= b.Policies.Stock.AgingDays {
			continue
		}
		var names []string
		for _, c := range a.Candidates {
			if it, ok := b.Get(c.DealerID); ok && it.Metrics.Status != domain.StatusChurn {
				names = append(names, link(it))
				quoted = append(quoted, it)
			}
		}
		total += a.Value
		to := "belum ada dealer yang cocok"
		if len(names) > 0 {
			to = "ke " + join(names)
		}
		parts = append(parts, fmt.Sprintf("%s (%s, %d hari) %s", a.Name, rp(a.Value), a.AgeDays, to))
	}
	if len(parts) == 0 {
		return "Tidak ada stok di atas 90 hari.", nil
	}
	return fmt.Sprintf("%s. Total stok menua %s; harga bundle tidak pernah di bawah floor margin.", strings.Join(parts, "; "), rp(total)), quoted
}

func schedule(b *views.Board) (string, []views.BoardItem) {
	ds := b.Due(7)
	if len(ds) == 0 {
		return "Tidak ada dealer yang jadwal ordernya 7 hari ke depan.", nil
	}
	var parts []string
	for _, it := range ds {
		when := fmt.Sprintf("%d hari", *it.Metrics.DueIn)
		if *it.Metrics.DueIn == 1 {
			when = "besok"
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", link(it), when))
	}
	return fmt.Sprintf("%d dealer jadwal order 7 hari ke depan: %s. Rekomendasi order disiapkan AI Follow-up.", len(ds), join(parts)), ds
}

var reLink = regexp.MustCompile(`\[\[dealer:[^\]]+\]\]`)
var reNumber = regexp.MustCompile(`\d+(?:[.,]\d+)?`)

// KeepsFacts reports whether a rewrite kept every dealer link and number of the template.
func KeepsFacts(template, text string) bool {
	for _, l := range reLink.FindAllString(template, -1) {
		if !strings.Contains(text, l) {
			return false
		}
	}
	for _, n := range reNumber.FindAllString(reLink.ReplaceAllString(template, ""), -1) {
		if !strings.Contains(text, n) {
			return false
		}
	}
	return true
}

func (s *Service) polish(ctx context.Context, a *Answer) {
	if s.Router == nil {
		return
	}
	input, _ := json.Marshal(map[string]any{"question": a.Q, "facts": a.Text})
	fallback, _ := json.Marshal(map[string]string{"answer": a.Text})
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"answer"}, "properties": map[string]any{"answer": map[string]any{"type": "string"}}}
	check := func(b json.RawMessage) error {
		var o struct{ Answer string }
		if err := json.Unmarshal(b, &o); err != nil || !KeepsFacts(a.Text, o.Answer) {
			return fmt.Errorf("answer lost a fact")
		}
		return nil
	}
	res := s.Router.Complete(ctx, llm.Request{Purpose: "ask", Agent: "Tanya", System: llm.Prompt("system") + "\n\n" + llm.Prompt("ask"), Input: input, Schema: schema, MaxTokens: 800, Fallback: fallback}, check)
	var o struct{ Answer string }
	if check(res.JSON) == nil && json.Unmarshal(res.JSON, &o) == nil {
		a.Text = o.Answer
		if res.Provider != "fake" && res.Provider != "template" {
			a.Via, a.Confidence = "llm", 0.88
		}
	}
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
