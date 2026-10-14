// Package replyrule is Rekomendasi balasan (ADR 0038): a customer message with one of a rule's words gets a suggested
// answer from the data — for a stock rule, the stock of the product the message names, per branch. Pure Go; the
// sales decides whether to use the draft, and nothing is ever sent by it.
package replyrule

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"distri-arc/internal/domain"
)

// Rule is one row of reply_rules.
type Rule struct {
	ID            string
	Name          string
	Action        string // stock
	Keywords      []string
	TemplateReady string
	TemplateEmpty string
	ShowQty       bool
}

// BranchQty is the stock of the matched product in one branch.
type BranchQty struct {
	Branch string `json:"branch"`
	Qty    int    `json:"qty"`
}

// Suggestion is what appears under a customer's bubble.
type Suggestion struct {
	RuleID   string      `json:"rule_id"`
	Rule     string      `json:"rule"`
	Keyword  string      `json:"keyword"`
	Product  string      `json:"product,omitempty"` // empty: the message names no product the stock knows
	SKU      string      `json:"sku,omitempty"`
	Branch   string      `json:"branch,omitempty"` // the conversation's branch
	Qty      int         `json:"qty"`              // in that branch
	Others   []BranchQty `json:"others"`           // other branches with stock, most first
	Ready    bool        `json:"ready"`
	Draft    string      `json:"draft,omitempty"`
	Note     string      `json:"note,omitempty"`
	Products []string    `json:"candidates,omitempty"` // several products fit equally: the sales picks
}

var reWord = regexp.MustCompile(`[\pL\d]+`)

func words(s string) []string { return reWord.FindAllString(strings.ToLower(s), -1) }

// Matches returns the first keyword of the rule found in text (whole words; a keyword of several words in order).
func Matches(r Rule, text string) string {
	w := " " + strings.Join(words(text), " ") + " "
	for _, k := range r.Keywords {
		kw := strings.Join(words(k), " ")
		if kw != "" && strings.Contains(w, " "+kw+" ") {
			return k
		}
	}
	return ""
}

// stopWords never identify a product.
var stopWords = map[string]bool{"stok": true, "stock": true, "ready": true, "redy": true, "ada": true, "barang": true, "tersedia": true,
	"masih": true, "gak": true, "ga": true, "tidak": true, "kosong": true, "inden": true, "pak": true, "bu": true, "mas": true, "mbak": true,
	"kak": true, "ko": true, "ci": true, "min": true, "nya": true, "yang": true, "untuk": true, "dan": true, "atau": true, "ya": true, "yg": true,
	"berapa": true, "harga": true, "unit": true, "pcs": true, "buah": true, "sama": true, "ini": true, "itu": true, "dong": true, "kah": true,
	"apa": true, "mau": true, "order": true, "pesan": true, "ambil": true, "cek": true, "tolong": true, "di": true, "ke": true}

type product struct {
	sku, name string
	toks      []string
	byBranch  map[string]int
}

// catalog groups the stock rows per product (SKU, else name) with their quantity per branch.
func catalog(stock []domain.StockItem) []*product {
	by := map[string]*product{}
	var out []*product
	for _, s := range stock {
		k := s.SKU
		if k == "" {
			k = strings.ToLower(s.Name)
		}
		p := by[k]
		if p == nil {
			p = &product{sku: s.SKU, name: s.Name, toks: words(s.Name), byBranch: map[string]int{}}
			by[k] = p
			out = append(out, p)
		}
		p.byBranch[s.Branch] += s.Qty
	}
	return out
}

// find picks the product the message names: the most shared words (a model code like "4mp" or a SKU counts double);
// ties go to the plainer name; a tie between equal names returns every candidate.
func find(text string, cat []*product) (*product, []string) {
	msg := map[string]bool{}
	for _, w := range words(text) {
		if !stopWords[w] {
			msg[w] = true
		}
	}
	type scored struct {
		p     *product
		score int
	}
	var best []scored
	top := 0
	for _, p := range cat {
		score := 0
		for _, t := range p.toks {
			if msg[t] {
				score++
				if strings.ContainsAny(t, "0123456789") {
					score++
				}
			}
		}
		if p.sku != "" && msg[strings.ToLower(p.sku)] {
			score += 3
		}
		switch {
		case score == 0 || score < top:
		case score > top:
			top, best = score, []scored{{p, score}}
		default:
			best = append(best, scored{p, score})
		}
	}
	if top < 2 || len(best) == 0 { // one shared word ("kamera") is not a product
		return nil, nil
	}
	sort.SliceStable(best, func(i, j int) bool { return len(best[i].p.toks) < len(best[j].p.toks) })
	if len(best) > 1 && len(best[0].p.toks) == len(best[1].p.toks) {
		var names []string
		for _, b := range best[:min(len(best), 5)] {
			names = append(names, b.p.name)
		}
		return nil, names
	}
	return best[0].p, nil
}

// Suggest is the answer of the first enabled rule that matches text, for a conversation of branch; nil when none.
func Suggest(rules []Rule, text, branch string, stock []domain.StockItem) *Suggestion {
	for _, r := range rules {
		k := Matches(r, text)
		if k == "" || r.Action != "stock" {
			continue
		}
		s := &Suggestion{RuleID: r.ID, Rule: r.Name, Keyword: k, Branch: branch, Others: []BranchQty{}}
		p, cands := find(text, catalog(stock))
		if p == nil {
			s.Products = cands
			s.Note = "Produk di pesan tidak dikenali di data stok — sebutkan produknya atau cek di Push stok."
			if len(cands) > 0 {
				s.Note = "Beberapa produk cocok — pilih yang dimaksud pelanggan."
			}
			return s
		}
		s.Product, s.SKU = p.name, p.sku
		s.Qty = max(p.byBranch[branch], 0)
		for b, q := range p.byBranch {
			if b != branch && q > 0 {
				s.Others = append(s.Others, BranchQty{Branch: b, Qty: q})
			}
		}
		sort.Slice(s.Others, func(i, j int) bool { return s.Others[i].Qty > s.Others[j].Qty })
		where := branch
		if where == "" { // no branch for the conversation: every branch counts
			for _, o := range s.Others {
				s.Qty += o.Qty
			}
			s.Others, where = []BranchQty{}, "gudang kami"
		}
		s.Ready = s.Qty > 0
		tpl := r.TemplateEmpty
		if s.Ready {
			tpl = r.TemplateReady
		}
		qty := fmt.Sprint(s.Qty)
		if !r.ShowQty {
			qty = ""
		}
		other := ""
		if len(s.Others) > 0 {
			other = s.Others[0].Branch
		}
		s.Draft = fill(tpl, map[string]string{"produk": p.name, "jumlah": qty, "cabang": where, "cabang_lain": other})
		if !s.Ready && len(s.Others) > 0 {
			s.Note = fmt.Sprintf("Kosong di %s, ada di %s (%d) — bisa ditawarkan mutasi.", where, s.Others[0].Branch, s.Others[0].Qty)
		}
		return s
	}
	return nil
}

// fill replaces {placeholders}; an empty {jumlah} takes its "pcs" along ("Ready untuk …").
func fill(tpl string, v map[string]string) string {
	if v["jumlah"] == "" {
		tpl = strings.NewReplacer("{jumlah} pcs ", "", "{jumlah} unit ", "", "{jumlah} ", "").Replace(tpl)
	}
	for k, x := range v {
		tpl = strings.ReplaceAll(tpl, "{"+k+"}", x)
	}
	return strings.Join(strings.Fields(tpl), " ")
}
