package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/llm"
	"distri-arc/internal/memory"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// refreshMemos (Belajar) writes the memo of every dealer in scope that never had sentence sources, or that got
// new signals since its memo was written. Only memos that pass memory.Validate are stored.
func (o *Orchestrator) refreshMemos(ctx context.Context, r *stageRun) (int, error) {
	rows, err := o.St.Q.DealerMemoState(ctx)
	if err != nil {
		return 0, err
	}
	now := o.Clock.Now()
	cid := r.cyc.ID.String()
	n := 0
	for _, st := range rows {
		d := r.in.Dealer(st.ID)
		if d == nil {
			continue
		}
		var prev memory.Memo
		_ = json.Unmarshal(st.MemoSentences, &prev)
		var m memory.Memo
		switch {
		case len(st.MemoSentences) == 0 || string(st.MemoSentences) == "null":
			var dropped []string
			m, dropped = memory.FromText(deref(st.Memo), d.Signals, st.MemoSignalIds...)
			if len(dropped) > 0 {
				o.log().Info("memo sentences without source dropped", "dealer", st.Slug, "dropped", dropped)
			}
		case st.HasNew:
			m = memory.Refresh(prev, d)
		default:
			continue
		}
		allowed := append(append([]uuid.UUID{}, st.MemoSignalIds...), prev.SignalIDs()...)
		for _, s := range d.Signals {
			allowed = append(allowed, s.ID)
		}
		m = memory.Polish(ctx, o.Router, &cid, d, m, allowed)
		if err := memory.Validate(m, allowed); err != nil || len(m) == 0 {
			o.log().Warn("memo not stored", "dealer", st.Slug, "err", err)
			continue
		}
		b, _ := json.Marshal(m)
		text := m.Text()
		if err := o.St.Q.SetDealerMemo(ctx, gen.SetDealerMemoParams{ID: st.ID, Memo: &text, MemoSignalIds: m.SignalIDs(), MemoSentences: b, MemoUpdatedAt: &now}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

var reLink = regexp.MustCompile(`\[\[dealer:[^\]]+\]\]`)
var reNumber = regexp.MustCompile(`\d+(?:[.,]\d+)?`)

// keepsFacts reports whether a rewritten brief point kept every dealer link and every number of the template.
func keepsFacts(template, text string) bool {
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

// writeBrief (full cycle) writes Ringkasan Orchestrator: the four template points with the signals of the dealers
// they quote, phrased by the LLM when it keeps every link and number, stored for /brief/today.
func (o *Orchestrator) writeBrief(ctx context.Context, r *stageRun) error {
	now := o.Clock.Now()
	b, err := views.NewBuilder(o.St, o.Clock).Board(ctx)
	if err != nil {
		return err
	}
	since := clock.Today(now).AddDate(0, 0, -1)
	c, err := o.St.Q.CountSignalsSince(ctx, since)
	if err != nil {
		return err
	}
	branches := map[string]bool{}
	for _, s := range r.in.Stock {
		branches[s.Branch] = true
	}
	br := b.TemplateBrief(r.in.Stock, views.BriefCounts{WA: c.Wa, SO: c.So, Payments: c.Payments, Branches: len(branches)})
	br.GeneratedAt, br.Cycle = now, r.cyc.Number
	kinds := map[string][]string{"on_schedule": {"so", "wa"}, "drift": {"so", "wa"}, "credit": {"invoice", "payment", "wa"}, "push": {"stock"}}
	for i := range br.Points {
		p := &br.Points[i]
		for _, bd := range p.Dealers {
			d := r.in.DealerBySlug(bd.ID)
			if d == nil {
				continue
			}
			for _, s := range d.Signals {
				if slices.Contains(kinds[p.Kind], s.Kind) {
					p.SignalIDs = append(p.SignalIDs, s.ID.String())
					break
				}
			}
		}
		if p.Item != nil {
			if id, ok := r.in.StockSignals[p.Item.Name+"|"+p.Item.Branch]; ok {
				p.SignalIDs = append(p.SignalIDs, id.String())
			}
		}
	}
	source := "template"
	type pt struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	}
	var in []pt
	for _, p := range br.Points {
		in = append(in, pt{p.Kind, p.Text})
	}
	input, _ := json.Marshal(map[string]any{"points": in, "counts": br.Counts})
	fallback, _ := json.Marshal(map[string]any{"points": in})
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"points"}, "properties": map[string]any{
		"points": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "text"},
			"properties": map[string]any{"kind": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}}}}}}
	check := func(raw json.RawMessage) error {
		var o struct{ Points []pt }
		if err := json.Unmarshal(raw, &o); err != nil || len(o.Points) != len(in) {
			return errors.New("four points expected")
		}
		for i := range in {
			if o.Points[i].Kind != in[i].Kind || !keepsFacts(in[i].Text, o.Points[i].Text) {
				return errors.New("a point lost a link or a number")
			}
		}
		return nil
	}
	if o.Router != nil {
		cid := r.cyc.ID.String()
		res := o.Router.Complete(ctx, llm.Request{Purpose: "brief.write", Agent: "Orchestrator", CycleID: &cid, System: llm.Prompt("system") + "\n\n" + llm.Prompt("brief"),
			Input: input, Schema: schema, MaxTokens: 1500, Fallback: fallback}, check)
		var out struct{ Points []pt }
		if check(res.JSON) == nil && json.Unmarshal(res.JSON, &out) == nil {
			for i := range br.Points {
				br.Points[i].Text = out.Points[i].Text
			}
			if res.Provider != "template" && res.Provider != "fake" {
				source = "llm"
				br.Confidence = 0.88
			}
		}
	}
	br.Source = source
	b2, _ := json.Marshal(br)
	cyc := r.cyc.ID
	return o.St.Q.SaveBrief(ctx, gen.SaveBriefParams{BriefDate: clock.Today(now), CycleID: &cyc, Brief: b2, Source: source, CreatedAt: now})
}
