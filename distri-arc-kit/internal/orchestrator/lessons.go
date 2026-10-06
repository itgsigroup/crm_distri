package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/store/gen"
)

// LessonMin is how many rejections with the same reason make a lesson (docs/stages/10).
const LessonMin = 3

// LessonDays is how long a lesson keeps similar proposals away.
const LessonDays = 14

// Rejection is one human rejection with what it was about.
type Rejection struct {
	Agent, Kind, Reason string // Reason: label — optional note
	Dealer, Tier        string
	Product             string // product family ("HDD") when the proposal was about one
	At                  time.Time
}

// Lesson is a rule learned from repeated rejections: what the agent no longer proposes, to whom.
type Lesson struct {
	Agent, Kind, Reason string
	Scope               string // "tier C" | dealer name | "semua dealer"
	Tier, Dealer        string // the scope as a match (one of them, or neither = every dealer)
	Product             string
	Rejections          int
	Note                string
	Text                string
	Until               time.Time
}

// Family is the product family a lesson speaks about: the first word of the name ("HDD 4TB surveillance" → "HDD").
func Family(product string) string {
	f := strings.Fields(product)
	if len(f) == 0 {
		return ""
	}
	if strings.EqualFold(f[0], "Kamera") || strings.EqualFold(f[0], "Modul") {
		if len(f) > 1 {
			return f[0] + " " + f[1]
		}
	}
	return f[0]
}

// ProductOf reads the product a proposal is about from its payload.
func ProductOf(payload map[string]any) string {
	for _, k := range []string{"name", "product", "bundle_sku"} {
		if s, ok := payload[k].(string); ok && s != "" {
			return s
		}
	}
	if rec, ok := payload["recommendation"].(map[string]any); ok {
		if s, ok := rec["sweetener"].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

var kindVerb = map[string]string{domain.KindPushStock: "menawarkan", domain.KindFollowup: "menawarkan", domain.KindPriceCounter: "memberi harga khusus",
	domain.KindCollect: "mengirim pengingat", domain.KindCreditLimit: "mengusulkan kenaikan limit", domain.KindCreditRelease: "mengusulkan rilis kredit"}

// Lessons aggregates rejections: ≥ LessonMin rejections of the same agent, kind and reason (and product family when
// there is one) become one lesson, scoped to the tier they share, else the one dealer, else every dealer.
func Lessons(rs []Rejection, today time.Time) []Lesson {
	type key struct{ agent, kind, label, family string }
	groups := map[key][]Rejection{}
	var order []key
	for _, r := range rs {
		label, _, _ := strings.Cut(r.Reason, " — ")
		k := key{r.Agent, r.Kind, label, Family(r.Product)}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	var out []Lesson
	for _, k := range order {
		g := groups[k]
		if len(g) < LessonMin {
			continue
		}
		l := Lesson{Agent: k.agent, Kind: k.kind, Reason: k.label, Product: k.family, Rejections: len(g), Until: today.AddDate(0, 0, LessonDays)}
		tiers, dealers := map[string]bool{}, map[string]bool{}
		notes := map[string]int{}
		for _, r := range g {
			tiers[r.Tier], dealers[r.Dealer] = true, true
			if _, n, ok := strings.Cut(r.Reason, " — "); ok && n != "" {
				notes[n]++
			}
		}
		switch {
		case len(dealers) == 1:
			for d := range dealers {
				l.Dealer, l.Scope = d, d
			}
		case len(tiers) == 1:
			for t := range tiers {
				if t != "" {
					l.Tier, l.Scope = t, "tier "+t
				}
			}
		}
		if l.Scope == "" {
			l.Scope = "semua dealer"
		}
		best := 0
		for n, c := range notes {
			if c > best || (c == best && n < l.Note) {
				l.Note, best = n, c
			}
		}
		verb := kindVerb[k.kind]
		if verb == "" {
			verb = "mengusulkan " + strings.ReplaceAll(k.kind, "_", " ")
		}
		what := ""
		if l.Product != "" {
			what = " " + l.Product
		}
		to := " ke dealer " + l.Scope
		if l.Dealer != "" {
			to = " ke " + l.Dealer
		} else if l.Scope == "semua dealer" {
			to = ""
		}
		why := l.Reason
		if l.Note != "" {
			why = "“" + l.Note + "”"
		}
		l.Text = fmt.Sprintf("%s tidak lagi %s%s%s (ditolak %d×: %s).", k.agent, verb, what, to, l.Rejections, why)
		out = append(out, l)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rejections > out[j].Rejections })
	return out
}

// Applies reports whether a lesson covers a candidate for a dealer.
func (l Lesson) Applies(agent, kind, product string, d *agents.Dealer) bool {
	if primaryAgent(agent) != primaryAgent(l.Agent) || kind != l.Kind {
		return false
	}
	if l.Product != "" && !strings.EqualFold(Family(product), l.Product) {
		return false
	}
	switch {
	case l.Dealer != "":
		return d != nil && d.Name == l.Dealer
	case l.Tier != "":
		return d != nil && d.Tier == l.Tier
	}
	return true
}

// learnLessons (Belajar) turns the rejections of the last 30 days into lessons.
func (o *Orchestrator) learnLessons(ctx context.Context) (int, error) {
	now := o.Clock.Now()
	rows, err := o.St.Q.RejectionsForLessons(ctx, now.AddDate(0, 0, -30))
	if err != nil {
		return 0, err
	}
	var rs []Rejection
	for _, r := range rows {
		var pl map[string]any
		_ = json.Unmarshal(r.Payload, &pl)
		rs = append(rs, Rejection{Agent: deref(r.Agent), Kind: deref(r.Kind), Reason: deref(r.Reason), Dealer: deref(r.DealerName), Tier: deref(r.Tier), Product: ProductOf(pl), At: r.CreatedAt})
	}
	ls := Lessons(rs, clock.Today(now))
	for _, l := range ls {
		until := l.Until
		if err := o.St.Q.UpsertLesson(ctx, gen.UpsertLessonParams{Agent: l.Agent, Kind: l.Kind, Reason: l.Reason, Scope: l.Scope, Product: strp(l.Product),
			Rejections: int32(l.Rejections), Text: l.Text, SuppressUntil: &until, CreatedAt: now}); err != nil {
			return 0, err
		}
	}
	return len(ls), nil
}

// LessonFromRow restores a stored lesson's match fields from its scope.
func LessonFromRow(r gen.CalibrationLesson) Lesson {
	l := Lesson{Agent: r.Agent, Kind: r.Kind, Reason: r.Reason, Scope: r.Scope, Product: deref(r.Product), Rejections: int(r.Rejections), Text: r.Text}
	switch {
	case strings.HasPrefix(r.Scope, "tier "):
		l.Tier = strings.TrimPrefix(r.Scope, "tier ")
	case r.Scope != "semua dealer":
		l.Dealer = r.Scope
	}
	if r.SuppressUntil != nil {
		l.Until = *r.SuppressUntil
	}
	return l
}
