// Package memory keeps the dealer memo ("Memori dealer"): a short brief of at most 120 words in which every
// sentence carries the signals it was written from. A sentence without a source is never stored (CLAUDE.md §2).
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
)

// MaxWords bounds a memo.
const MaxWords = 120

// Sentence is one claim of the memo with its sources.
type Sentence struct {
	Text      string      `json:"text"`
	SignalIDs []uuid.UUID `json:"signal_ids"`
}

// Memo is the whole brief.
type Memo []Sentence

// Text joins the sentences.
func (m Memo) Text() string {
	var xs []string
	for _, s := range m {
		xs = append(xs, s.Text)
	}
	return strings.Join(xs, " ")
}

// SignalIDs is the union of the sources, in order of appearance.
func (m Memo) SignalIDs() []uuid.UUID {
	var out []uuid.UUID
	for _, s := range m {
		for _, id := range s.SignalIDs {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

// Errors of Validate.
var (
	ErrNoSource = errors.New("kalimat tanpa sumber")
	ErrTooLong  = fmt.Errorf("memo lebih dari %d kata", MaxWords)
)

// Validate rejects a memo with a sentence without sources, a source outside allowed, or more than MaxWords.
func Validate(m Memo, allowed []uuid.UUID) error {
	if len(strings.Fields(m.Text())) > MaxWords {
		return ErrTooLong
	}
	for _, s := range m {
		if strings.TrimSpace(s.Text) == "" {
			return errors.New("kalimat kosong")
		}
		if len(s.SignalIDs) == 0 {
			return fmt.Errorf("%w: %q", ErrNoSource, s.Text)
		}
		for _, id := range s.SignalIDs {
			if !slices.Contains(allowed, id) {
				return fmt.Errorf("%w: sinyal %s bukan milik dealer ini (%q)", ErrNoSource, id, s.Text)
			}
		}
	}
	return nil
}

var reSentence = regexp.MustCompile(`[^.!?]+(?:[.!?]+["”]?|$)`)

// Split cuts a text into sentences (keeps "Rp 1,28 M" and "INV/0889" intact).
func Split(text string) []string {
	var out []string
	for _, m := range reSentence.FindAllString(text, -1) {
		if t := strings.TrimSpace(m); t != "" {
			// a dot inside a number ("1.284") is not a sentence end: glue fragments that start with a digit
			if len(out) > 0 && t[0] >= '0' && t[0] <= '9' && strings.HasSuffix(out[len(out)-1], ".") {
				out[len(out)-1] += t
				continue
			}
			out = append(out, t)
		}
	}
	return out
}

// Topic of a sentence decides which signals can support it.
type Topic int

// Topics.
const (
	TopicOther  Topic = iota
	TopicOrbit        // order cycle, last order, drift
	TopicCredit       // limit, invoices, payments
)

var (
	reOrbit  = regexp.MustCompile(`(?i)siklus order|lewat jadwal|order rutin|order terakhir|tanpa order|order turun|jadwal order|churn`)
	reCredit = regexp.MustCompile(`(?i)limit|invoice|bayar|exposure|piutang|tepat waktu|jatuh tempo|tempo|cicilan`)
)

// TopicOf classifies a sentence.
func TopicOf(s string) Topic {
	switch {
	case reCredit.MatchString(s):
		return TopicCredit
	case reOrbit.MatchString(s):
		return TopicOrbit
	}
	return TopicOther
}

var stop = map[string]bool{"yang": true, "dan": true, "untuk": true, "dari": true, "dengan": true, "sudah": true, "masih": true, "tidak": true,
	"order": true, "dealer": true, "minggu": true, "bulan": true, "hari": true, "lewat": true, "kami": true, "saya": true, "bisa": true, "lebih": true}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '/'
	}) {
		if len(w) >= 4 && !stop[w] {
			out[w] = true
		}
	}
	return out
}

func overlap(a, b map[string]bool) int {
	n := 0
	for w := range a {
		if b[w] {
			n++
		}
	}
	return n
}

// Attribute finds the signals that support a sentence: by topic first (orbit → SO, credit → invoice/payment and
// WhatsApp about paying), then by shared words with the signal text or its annotation. Empty = unsupported.
func Attribute(sentence string, sigs []agents.Signal) []uuid.UUID {
	w := words(sentence)
	type scored struct {
		id uuid.UUID
		n  int
	}
	var best []scored
	for _, s := range sigs {
		n := overlap(w, words(s.Text+" "+s.Conclusion))
		switch TopicOf(sentence) {
		case TopicOrbit:
			if s.Kind == "so" {
				n += 2
			}
		case TopicCredit:
			if s.Kind == "invoice" || s.Kind == "payment" {
				n += 2
			} else if s.Kind == "wa" && reCredit.MatchString(s.Text+" "+s.Conclusion) {
				n++
			}
		}
		if n >= 2 {
			best = append(best, scored{s.ID, n})
		}
	}
	slices.SortStableFunc(best, func(a, b scored) int { return b.n - a.n })
	var out []uuid.UUID
	for _, b := range best {
		if len(out) == 3 {
			break
		}
		out = append(out, b.id)
	}
	return out
}

// FromText attributes an existing memo (imported or written before sentence sources existed). A sentence no
// signal supports by topic or words falls back to the signals the memo as a whole was written from (memoIDs);
// without those it is dropped and returned.
func FromText(text string, sigs []agents.Signal, memoIDs ...uuid.UUID) (Memo, []string) {
	var m Memo
	var dropped []string
	for _, s := range Split(text) {
		ids := Attribute(s, sigs)
		if len(ids) == 0 && len(memoIDs) > 0 {
			ids = memoIDs
		}
		if len(ids) > 0 {
			m = append(m, Sentence{Text: s, SignalIDs: ids})
		} else {
			dropped = append(dropped, s)
		}
	}
	return m, dropped
}

func ofKind(sigs []agents.Signal, n int, kinds ...string) []uuid.UUID {
	var out []uuid.UUID
	for _, s := range sigs {
		if slices.Contains(kinds, s.Kind) && len(out) < n {
			out = append(out, s.ID)
		}
	}
	return out
}

// Facts are the sentences Go computes for a dealer (orbit and credit), each with its sources.
func Facts(d *agents.Dealer) Memo {
	var m Memo
	mt := d.Metrics
	if mt.Rhythm != nil && mt.Last != nil {
		so := ofKind(d.Signals, 2, "so")
		var t string
		switch {
		case mt.Status == domain.StatusChurn:
			t = fmt.Sprintf("Tanpa order %d hari dari siklus order %d — churn, prioritas rendah.", *mt.Last, *mt.Rhythm)
		case mt.Cyc > 1.2:
			t = fmt.Sprintf("Lewat jadwal: %d hari tanpa order dari siklus order %d.", *mt.Last, *mt.Rhythm)
		case mt.DueIn != nil && *mt.DueIn <= 7:
			t = fmt.Sprintf("Siklus order %d hari; jadwal order %s.", *mt.Rhythm, map[bool]string{true: "besok", false: fmt.Sprintf("%d hari lagi", *mt.DueIn)}[*mt.DueIn == 1])
		default:
			t = fmt.Sprintf("Order rutin tiap %d hari, terakhir %d hari lalu.", *mt.Rhythm, *mt.Last)
		}
		if len(so) > 0 {
			m = append(m, Sentence{Text: t, SignalIDs: so})
		}
	}
	if d.CreditLimit > 0 {
		inv := ofKind(d.Signals, 2, "invoice", "payment")
		cr := mt.Credit
		t := fmt.Sprintf("Sisa limit %s (exposure %s / %s), pola bayar %d hari, %d%% tepat waktu.", cr.State, agents.Rp(cr.Exposure), agents.Rp(d.CreditLimit), cr.PayDays, cr.OnTime)
		for _, i := range d.OpenInvoices {
			if i.LateDays > 0 {
				t = strings.TrimSuffix(t, ".") + fmt.Sprintf("; %s %s lewat %d hari.", i.Number, agents.Rp(i.Residual), i.LateDays)
				break
			}
		}
		if len(inv) > 0 {
			m = append(m, Sentence{Text: t, SignalIDs: inv})
		}
	}
	return m
}

// Refresh rebuilds a memo when the dealer has new signals: the orbit and credit sentences are replaced by the
// computed facts, the other sentences (context people told us) stay with their sources, oldest first trimmed to
// MaxWords.
func Refresh(prev Memo, d *agents.Dealer) Memo {
	facts := Facts(d)
	var keep Memo
	for _, s := range prev {
		if TopicOf(s.Text) == TopicOther || (TopicOf(s.Text) == TopicCredit && !reLimitNumbers.MatchString(s.Text)) {
			keep = append(keep, s)
		}
	}
	out := append(Memo{}, keep...)
	out = append(out, facts...)
	for len(strings.Fields(out.Text())) > MaxWords && len(out) > len(facts) {
		out = out[1:]
	}
	return out
}

// reLimitNumbers marks credit sentences that state numbers (they are superseded by the computed credit fact).
var reLimitNumbers = regexp.MustCompile(`(?i)(exposure|limit)[^.]*\d|\d+\s*/\s*\d+\s*jt`)

// Polish asks the model to rewrite the memo in natural Indonesian, sentence by sentence, keeping each sentence's
// sources; any answer that breaks the mapping falls back to the input.
func Polish(ctx context.Context, r *llm.Router, cycleID *string, d *agents.Dealer, m Memo, allowed []uuid.UUID) Memo {
	if r == nil || len(m) == 0 {
		return m
	}
	type out struct {
		Sentences []Sentence `json:"sentences"`
	}
	fallback, _ := json.Marshal(out{Sentences: m})
	input, _ := json.Marshal(map[string]any{"dealer": d.Name, "sentences": m, "max_words": MaxWords})
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"sentences"}, "properties": map[string]any{
		"sentences": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"text", "signal_ids"},
			"properties": map[string]any{"text": map[string]any{"type": "string"}, "signal_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}}}}
	var names []string
	for _, c := range d.Contacts {
		names = append(names, c.Name)
	}
	res := r.Complete(ctx, llm.Request{Purpose: "memo.write", Agent: "Orchestrator", CycleID: cycleID, System: llm.Prompt("system") + "\n\n" + llm.Prompt("memo"),
		Input: input, Schema: schema, MaxTokens: 1200, Names: append(names, d.Owner.Name), Fallback: fallback}, func(b json.RawMessage) error {
		var o out
		if err := json.Unmarshal(b, &o); err != nil {
			return err
		}
		return Validate(o.Sentences, allowed)
	})
	var o out
	if json.Unmarshal(res.JSON, &o) != nil || Validate(o.Sentences, allowed) != nil {
		return m
	}
	return o.Sentences
}
