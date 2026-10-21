package replyrule

import (
	"fmt"
	"sort"
	"strings"

	"distri-arc/internal/domain"
)

// Claude finds the products a customer means (Sam 21 Oct: "pakai AI Claude untuk mencari stok"): word matching
// missed the second product of "H8c 2mp : 8 … sekalian nvr ezviz X5S-8W : 1 unit" and picked one NVR for "nvr ezviz
// 8ch". The system offers Claude candidates from the stock catalogue (Candidates), Claude names the SKUs and the
// quantity asked (Match), and the numbers — stock per branch, price, enough or not — stay computed here.

// MatchItem is one product Claude found in a message: a SKU of the catalogue and the quantity asked (0: not said).
type MatchItem struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

// Match is Claude's reading of one message: the products it means (none: no product of the catalogue).
type Match struct {
	Items []MatchItem `json:"items"`
	Note  string      `json:"note,omitempty"`
	Model string      `json:"model"`
}

// Candidate is a product offered to Claude: its SKU, name and stock over every branch.
type Candidate struct {
	SKU  string `json:"sku"`
	Name string `json:"nama"`
	Qty  int    `json:"stok"`
}

// Candidates are up to n products of the stock that share a word with the message or, less, with the chat before it
// (newest first): the most shared words first, a model code ("x5s", "2mp") or the SKU counting more. Words of the
// SKU count too ("H8C_3MP_Poe" → h8c 3mp poe), so a customer may write either.
func Candidates(text string, history []Line, stock []domain.StockItem, n int) []Candidate {
	weight := map[string]int{}
	add := func(t string, w int) {
		for _, x := range words(t) {
			if !stopWords[x] && weight[x] < w {
				weight[x] = w
			}
		}
	}
	for i := len(history[:min(len(history), HistoryLines)]) - 1; i >= 0; i-- {
		add(history[i].Text, 1)
	}
	add(text, 3)
	type scored struct {
		c     Candidate
		score int
	}
	var out []scored
	for _, p := range catalog(stock) {
		toks := map[string]bool{}
		for _, t := range p.toks {
			toks[t] = true
		}
		for _, t := range words(p.sku) {
			toks[t] = true
		}
		score := 0
		for t := range toks {
			if w := weight[t]; w > 0 {
				score += w
				if strings.ContainsAny(t, "0123456789") {
					score += w
				}
			}
		}
		if score == 0 {
			continue
		}
		total := 0
		for _, q := range p.byBranch {
			total += max(q, 0)
		}
		out = append(out, scored{Candidate{SKU: p.sku, Name: p.name, Qty: total}, score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].c.Name < out[j].c.Name
	})
	res := make([]Candidate, 0, min(len(out), n))
	for _, x := range out[:min(len(out), n)] {
		res = append(res, x.c)
	}
	return res
}

// StockLine is one product of a stock card that names several (or asks a quantity).
type StockLine struct {
	Product  string      `json:"product"`
	SKU      string      `json:"sku"`
	Headline string      `json:"headline"`
	Tone     string      `json:"tone"`
	Qty      int         `json:"qty"`    // in the conversation's branch, or every branch
	Asked    int         `json:"asked"`  // quantity the customer asked; 0: not said
	Ready    bool        `json:"ready"`  // the stock covers what is asked (at least 1)
	Price    string      `json:"price"`  // "Rp 865.000" or ""
	Others   []BranchQty `json:"others"` // other branches with stock
	Note     string      `json:"note,omitempty"`
}

// SuggestMatched is Suggest with the products Claude found in the message (m nil: found by words, as Suggest).
func SuggestMatched(rules []Rule, text string, history []Line, d Data, m *Match) []*Suggestion {
	out := Suggest(rules, text, history, d)
	if m == nil {
		return out
	}
	for i, s := range out {
		if s.Action != ActionStock {
			continue
		}
		out[i] = matchedStock(ruleOf(rules, s.RuleID), s, m, d)
	}
	return out
}

func ruleOf(rules []Rule, id string) Rule {
	for _, r := range rules {
		if r.ID == id {
			return r
		}
	}
	return Rule{}
}

// matchedStock rebuilds a stock card from the products Claude found: one product as before (with the quantity
// asked), several as lines, each with its stock and price; the draft answers them all.
func matchedStock(r Rule, base *Suggestion, m *Match, d Data) *Suggestion {
	s := &Suggestion{RuleID: base.RuleID, Rule: base.Rule, Action: ActionStock, Keyword: base.Keyword, Branch: base.Branch, Others: []BranchQty{},
		Facts: []Fact{}, Tone: "neutral", AIPhrase: base.AIPhrase, MatchedBy: m.Model}
	cat := map[string]*product{}
	for _, p := range catalog(d.Stock) {
		cat[strings.ToLower(p.sku)] = p
	}
	var lines []StockLine
	var subs []*Suggestion
	for _, it := range m.Items {
		p := cat[strings.ToLower(it.SKU)]
		if p == nil {
			continue
		}
		x := &Suggestion{RuleID: s.RuleID, Action: ActionStock, Others: []BranchQty{}, Facts: []Fact{}, Tone: "neutral"}
		stockAnswer(x, r, p, d)
		asked(x, r, it.Qty)
		price := ""
		for _, f := range x.Facts {
			if f.Label == "Harga" {
				price = f.Value
			}
		}
		lines = append(lines, StockLine{Product: x.Product, SKU: x.SKU, Headline: x.Headline, Tone: x.Tone, Qty: x.Qty, Asked: it.Qty, Ready: x.Ready,
			Price: price, Others: x.Others, Note: x.Note})
		subs = append(subs, x)
	}
	switch len(subs) {
	case 0:
		s.Headline, s.Note = "Produk tidak dikenali", "Claude tidak menemukan produk pesan ini di data stok — tanyakan produknya atau cek manual."
		if m.Note != "" {
			s.Note += " (" + m.Note + ")"
		}
		return s
	case 1:
		x := subs[0]
		x.Rule, x.Keyword, x.Branch, x.AIPhrase, x.MatchedBy = s.Rule, s.Keyword, s.Branch, s.AIPhrase, m.Model
		return x
	}
	// several products: one line each; the draft answers each, in the customer's order
	ready := 0
	vars := map[string]string{}
	var drafts []string
	for i, x := range subs {
		if x.Ready {
			ready++
		}
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("_%d", i+1)
		}
		for k, v := range x.Vars() {
			vars[k+suffix] = v
		}
		drafts = append(drafts, x.Draft)
	}
	s.Items = lines
	s.Product = fmt.Sprintf("%d barang", len(lines))
	s.Ready = ready == len(lines)
	switch {
	case s.Ready:
		s.Headline, s.Tone = fmt.Sprintf("%d barang · semua ready", len(lines)), "good"
	case ready == 0:
		s.Headline, s.Tone = fmt.Sprintf("%d barang · tidak ada yang cukup", len(lines)), "bad"
	default:
		s.Headline, s.Tone = fmt.Sprintf("%d barang · %d ready, %d kurang/kosong", len(lines), ready, len(lines)-ready), "warn"
	}
	s.vars = vars
	s.Draft = strings.Join(drafts, " ")
	return s
}

// asked compares the stock with the quantity the customer asked: enough is ready; less is "kurang" with what there
// is.
func asked(x *Suggestion, r Rule, n int) {
	if n <= 0 {
		return
	}
	x.Facts = append(x.Facts, Fact{"Diminta", fmt.Sprintf("%s unit", num(int64(n)))})
	v := x.Vars()
	if v == nil {
		v = map[string]string{}
	}
	v["diminta"] = fmt.Sprint(n)
	x.draft(r, v) // a partial stock keeps the ready frame ("Ready 5 pcs …"); the AI says it is less than asked
	switch {
	case x.Qty >= n:
		x.Headline += fmt.Sprintf(" · diminta %s, cukup", num(int64(n)))
	case x.Qty > 0:
		x.Ready, x.Tone = false, "warn"
		x.Headline = fmt.Sprintf("Kurang · %s dari %s diminta", num(int64(x.Qty)), num(int64(n)))
		if x.Note == "" && len(x.Others) > 0 {
			x.Note = fmt.Sprintf("Kurang %s; %s ada %s — bisa ditawarkan mutasi.", num(int64(n-x.Qty)), x.Others[0].Branch, num(int64(x.Others[0].Qty)))
		}
	}
}

// Searching turns a stock card into "Claude is looking for the products" while Claude reads the message: no product
// guessed by words is shown meanwhile.
func Searching(s *Suggestion) {
	*s = Suggestion{RuleID: s.RuleID, Rule: s.Rule, Action: s.Action, Keyword: s.Keyword, Branch: s.Branch, Others: []BranchQty{}, Facts: []Fact{},
		Tone: "neutral", AIPhrase: s.AIPhrase, AIPending: true, Headline: "Claude mencari barang di data stok…"}
}
