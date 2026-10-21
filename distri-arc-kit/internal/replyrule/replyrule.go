// Package replyrule is AI Trigger (ADR 0038, ADR 0041): a customer message with one of a rule's words gets an answer
// card from the data under its bubble — the stock or the price of the product the message names, the dealer's
// credit and open invoices, or the status of its last order — with a draft the sales may use. Pure Go: the numbers
// are computed here; an AI may only rephrase the draft (internal/replyai), and nothing is ever sent by it.
package replyrule

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"distri-arc/internal/domain"
)

// Actions a rule can take.
const (
	ActionStock  = "stock"  // stock of the product named in the message, per branch
	ActionPrice  = "price"  // price of the product named in the message, at the dealer's tier
	ActionCredit = "credit" // the dealer's credit room and open invoices
	ActionOrder  = "order"  // the status of the dealer's last order
)

// Actions lists every action, in the order the settings page offers them.
var Actions = []string{ActionStock, ActionPrice, ActionCredit, ActionOrder}

// ValidAction reports whether a is a known action.
func ValidAction(a string) bool {
	for _, x := range Actions {
		if x == a {
			return true
		}
	}
	return false
}

// Rule is one row of reply_rules.
type Rule struct {
	ID            string
	Name          string
	Action        string
	Keywords      []string
	TemplateReady string // the draft when the data answers yes (in stock, price known, credit fine, order found)
	TemplateEmpty string // the draft otherwise
	ShowQty       bool   // stock: the draft tells the quantity, else just "ready"
	AIPhrase      bool   // an AI rewrites the draft from the same facts (replyai)
	AIInstruction string // extra instruction for that AI (tone, greeting)
}

// BranchQty is the stock of the matched product in one branch.
type BranchQty struct {
	Branch string `json:"branch"`
	Qty    int    `json:"qty"`
}

// Fact is one line of the card ("Sisa limit · Rp 12.500.000").
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Suggestion is the card under a customer's bubble.
type Suggestion struct {
	RuleID   string      `json:"rule_id"`
	Rule     string      `json:"rule"`
	Action   string      `json:"action"`
	Keyword  string      `json:"keyword"`
	Headline string      `json:"headline"`
	Tone     string      `json:"tone"` // good | warn | bad | neutral
	Facts    []Fact      `json:"facts"`
	Product  string      `json:"product,omitempty"` // empty: the message names no product the data knows
	SKU      string      `json:"sku,omitempty"`
	Branch   string      `json:"branch,omitempty"` // the conversation's branch
	Qty      int         `json:"qty"`              // stock: in that branch
	Others   []BranchQty `json:"others"`           // stock: other branches with stock, most first
	Ready    bool        `json:"ready"`            // the data answers yes
	Draft    string      `json:"draft,omitempty"`
	Note     string      `json:"note,omitempty"`
	Products []string    `json:"candidates,omitempty"` // several products fit equally: the sales picks
	// FromHistory: the message does not name the product; it was taken from this earlier message of the chat
	FromHistory string `json:"from_history,omitempty"`
	// AI: the draft was rewritten by an AI from these facts; AIPending: the AI is still writing it
	AI        bool   `json:"ai,omitempty"`
	AIModel   string `json:"ai_model,omitempty"`
	AIPending bool   `json:"ai_pending,omitempty"`
	// Items: the products of a stock card that names several, one line each (Claude, Match)
	Items []StockLine `json:"items,omitempty"`
	// MatchedBy: the model that found the products in the message ("" — found by words)
	MatchedBy string `json:"matched_by,omitempty"`
	// AIError: no AI draft for these facts — why (no key, or ChatGPT and Claude did not keep to the facts)
	AIError  string `json:"ai_error,omitempty"`
	AIPhrase bool   `json:"ai_phrase,omitempty"` // the rule asks an AI to write the draft

	vars map[string]string // template values, for the AI's input
}

// Vars are the values the draft was filled with (the facts an AI may use).
func (s *Suggestion) Vars() map[string]string { return s.vars }

// Product is a sellable product with its prices (products, synced from Odoo).
type Product struct {
	SKU       string
	Name      string
	ListPrice int64            // tier A
	Prices    map[string]int64 // per tier: A, B, C
}

// Invoice is an unpaid invoice of the dealer.
type Invoice struct {
	Number   string
	DueAt    time.Time
	Residual int64
	LateDays int
}

// Order is the dealer's last order.
type Order struct {
	Number string
	Date   *time.Time
	Phase  int // metrics.Phase: 0 order … 4 bayar
	Total  int64
}

// Dealer is what the credit, order and price rules read of the conversation's dealer.
type Dealer struct {
	Name      string
	Tier      string
	Credit    domain.Credit
	Invoices  []Invoice // open, oldest first
	LastOrder *Order
}

// Data is everything the rules read for one conversation.
type Data struct {
	Branch   string
	Stock    []domain.StockItem
	Products []Product
	Dealer   *Dealer // nil: the conversation is not linked to a dealer
}

// Needs reports which data the matching rules of text read (to load only that).
func Needs(rules []Rule, text string) map[string]bool {
	out := map[string]bool{}
	for _, r := range rules {
		if Matches(r, text) != "" {
			out[r.Action] = true
		}
	}
	return out
}

var reWord = regexp.MustCompile(`[\pL\d]+`)

func words(s string) []string { return reWord.FindAllString(strings.ToLower(s), -1) }

// Matches returns the first keyword of the rule found in text (whole words; a keyword of several words in order).
// A word with the suffix -nya, -kah or -lah counts as the word ("stoknya" → stok, "harganya" → harga).
func Matches(r Rule, text string) string {
	ws := words(text)
	w := " " + strings.Join(ws, " ") + " "
	for i, x := range ws {
		ws[i] = stem(x)
	}
	stemmed := " " + strings.Join(ws, " ") + " "
	for _, k := range r.Keywords {
		kw := strings.Join(words(k), " ")
		if kw != "" && (strings.Contains(w, " "+kw+" ") || strings.Contains(stemmed, " "+kw+" ")) {
			return k
		}
	}
	return ""
}

// stem drops an Indonesian particle suffix when at least three letters stay ("punya" stays).
func stem(w string) string {
	for _, suf := range []string{"nya", "kah", "lah"} {
		if strings.HasSuffix(w, suf) && len([]rune(w))-3 >= 3 {
			return strings.TrimSuffix(w, suf)
		}
	}
	return w
}

// Line is an earlier message of the conversation (history), for a question that does not name its product
// ("stoknya ada?" after "kamera 4MP berapa?").
type Line struct {
	In   bool   `json:"in"` // from the customer
	Text string `json:"text"`
}

// HistoryLines is how many earlier messages a card reads (newest first) to find the product.
const HistoryLines = 10

// Suggest is one card per action for the enabled rules that match text (the first rule of an action wins).
// history holds the conversation's earlier messages, newest first (nil: none).
func Suggest(rules []Rule, text string, history []Line, d Data) []*Suggestion {
	var out []*Suggestion
	done := map[string]bool{}
	for _, r := range rules {
		if done[r.Action] {
			continue
		}
		k := Matches(r, text)
		if k == "" {
			continue
		}
		s := &Suggestion{RuleID: r.ID, Rule: r.Name, Action: r.Action, Keyword: k, Branch: d.Branch, Others: []BranchQty{}, Facts: []Fact{},
			Tone: "neutral", AIPhrase: r.AIPhrase}
		switch r.Action {
		case ActionStock:
			stockCard(s, r, text, history, d)
		case ActionPrice:
			priceCard(s, r, text, history, d)
		case ActionCredit:
			creditCard(s, r, d)
		case ActionOrder:
			orderCard(s, r, d)
		default:
			continue
		}
		done[r.Action] = true
		out = append(out, s)
	}
	return out
}

// draft fills the rule's template (ready or empty) and keeps the values for an AI.
func (s *Suggestion) draft(r Rule, v map[string]string) {
	s.vars = v
	tpl := r.TemplateEmpty
	if s.Ready {
		tpl = r.TemplateReady
	}
	s.Draft = fill(tpl, v)
}

func stockCard(s *Suggestion, r Rule, text string, history []Line, d Data) {
	p, cands := s.findIn(text, history, catalog(d.Stock))
	if p == nil {
		unknownProduct(s, cands, "data stok")
		return
	}
	stockAnswer(s, r, p, d)
}

// StockFor is the stock card of rule r for one SKU, as Chat would show it in a conversation of d.Branch (Cek stok:
// what the AI Trigger answers, to check against the real stock). Nil when no stock row has the SKU.
func StockFor(r Rule, sku string, d Data) *Suggestion {
	for _, p := range catalog(d.Stock) {
		if strings.EqualFold(p.sku, sku) {
			s := &Suggestion{RuleID: r.ID, Rule: r.Name, Action: ActionStock, Others: []BranchQty{}, Facts: []Fact{}, Tone: "neutral"}
			stockAnswer(s, r, p, d)
			return s
		}
	}
	return nil
}

func stockAnswer(s *Suggestion, r Rule, p *product, d Data) {
	branch := stockBranch(d.Branch, d.Stock)
	s.Branch = branch
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
	wh := where
	if branch == "" {
		wh = "semua cabang"
	}
	if s.Ready {
		s.Headline, s.Tone = fmt.Sprintf("Ready · %s di %s", num(int64(s.Qty)), wh), "good"
	} else {
		s.Headline, s.Tone = "Kosong di "+wh, "bad"
	}
	qty := fmt.Sprint(s.Qty)
	if !r.ShowQty {
		qty = ""
	}
	other := ""
	if len(s.Others) > 0 {
		other = s.Others[0].Branch
	}
	v := map[string]string{"produk": p.name, "jumlah": qty, "cabang": where, "cabang_lain": other}
	stockPrice(s, p, d, v)
	s.draft(r, v)
	if !s.Ready && len(s.Others) > 0 {
		s.Note = fmt.Sprintf("Kosong di %s, ada di %s (%d) — bisa ditawarkan mutasi.", where, s.Others[0].Branch, s.Others[0].Qty)
	}
}

// stockBranch is the conversation's branch as a stock branch: a branch no stock row has ("Semua cabang", the
// branch of a sales who covers every branch) is no branch, so every branch counts instead of reading 0.
func stockBranch(branch string, stock []domain.StockItem) string {
	for _, x := range stock {
		if strings.EqualFold(x.Branch, branch) {
			return x.Branch
		}
	}
	return ""
}

func priceCard(s *Suggestion, r Rule, text string, history []Line, d Data) {
	cat := make([]*product, 0, len(d.Products))
	for _, x := range d.Products {
		cat = append(cat, &product{sku: x.SKU, name: x.Name, toks: words(x.Name), list: x.ListPrice, prices: x.Prices})
	}
	p, cands := s.findIn(text, history, cat)
	if p == nil {
		unknownProduct(s, cands, "daftar harga")
		if len(d.Products) == 0 {
			s.Note = "Daftar harga belum tersinkron dari Odoo."
		}
		return
	}
	s.Product, s.SKU = p.name, p.sku
	price, label, tier := tierPrice(p.list, p.prices, d.Dealer)
	s.Ready = price > 0
	if s.Ready {
		s.Headline, s.Tone = rupiah(price)+" · "+label, "good"
		s.Facts = append(s.Facts, Fact{"Harga", rupiah(price)}, Fact{"Dasar", label})
		for _, t := range []string{"A", "B", "C"} {
			if v := p.prices[t]; v > 0 && t != tier {
				s.Facts = append(s.Facts, Fact{"Tier " + t, rupiah(v)})
			}
		}
	} else {
		s.Headline, s.Note = "Harga belum ada", "Produk ditemukan, tetapi harganya belum ada di data Odoo."
	}
	if d.Dealer == nil {
		s.Note = strings.TrimSpace(s.Note + " Percakapan belum tertaut ke dealer — memakai harga list.")
	}
	s.draft(r, map[string]string{"produk": p.name, "harga": rupiahPlain(price)})
}

// tierPrice is a product's price for the dealer: its tier's price, else the list price (tier A).
func tierPrice(list int64, prices map[string]int64, dl *Dealer) (int64, string, string) {
	tier := ""
	if dl != nil {
		tier = strings.ToUpper(strings.TrimSpace(dl.Tier))
	}
	price, label := list, "harga list (tier A)"
	if v, ok := prices[tier]; ok && v > 0 {
		price, label = v, "harga tier "+tier
	}
	if price <= 0 && prices["A"] > 0 {
		price = prices["A"]
	}
	return price, label, tier
}

// stockPrice adds the product's price to a stock card (Sam 21 Oct): the same price as the price card — the dealer's
// tier, else the list price — as facts for the sales and {harga} for the draft.
func stockPrice(s *Suggestion, p *product, d Data, v map[string]string) {
	var x *Product
	for i := range d.Products {
		if (p.sku != "" && strings.EqualFold(d.Products[i].SKU, p.sku)) || strings.EqualFold(d.Products[i].Name, p.name) {
			x = &d.Products[i]
			break
		}
	}
	if x == nil {
		return
	}
	price, label, _ := tierPrice(x.ListPrice, x.Prices, d.Dealer)
	if price <= 0 {
		return
	}
	s.Facts = append(s.Facts, Fact{"Harga", rupiah(price)}, Fact{"Dasar", label})
	v["harga"] = rupiahPlain(price)
}

func creditCard(s *Suggestion, r Rule, d Data) {
	if d.Dealer == nil {
		s.Headline, s.Note = "Dealer belum dikenali", "Percakapan ini belum tertaut ke dealer — limit dan tagihan tidak bisa dibaca."
		return
	}
	c := d.Dealer.Credit
	var open int64
	for _, i := range d.Dealer.Invoices {
		open += i.Residual
	}
	room := c.Limit - c.Exposure
	switch c.State {
	case domain.CreditCash:
		s.Headline = "Dealer cash · tanpa limit kredit"
	case domain.CreditOverLimit:
		s.Headline, s.Tone = "Melewati limit · "+rupiah(-room), "bad"
	case domain.CreditOverdue:
		s.Headline, s.Tone = fmt.Sprintf("Ada tagihan lewat jatuh tempo · %d hari", c.LateDays), "bad"
	case domain.CreditTipis:
		s.Headline, s.Tone = "Sisa limit tipis · "+rupiah(max(room, 0)), "warn"
	default:
		s.Headline, s.Tone = "Sisa limit · "+rupiah(max(room, 0)), "good"
	}
	s.Ready = c.State == domain.CreditAman || c.State == domain.CreditTipis || (c.State == domain.CreditCash && len(d.Dealer.Invoices) == 0)
	if c.Limit > 0 {
		s.Facts = append(s.Facts, Fact{"Limit", rupiah(c.Limit)}, Fact{"Terpakai", rupiah(c.Exposure)}, Fact{"Sisa limit", rupiah(max(room, 0))})
	}
	s.Facts = append(s.Facts, Fact{"Tagihan terbuka", fmt.Sprintf("%d · %s", len(d.Dealer.Invoices), rupiah(open))})
	due := ""
	if len(d.Dealer.Invoices) > 0 {
		first := d.Dealer.Invoices[0]
		due = tanggal(first.DueAt)
		v := first.Number + " · jatuh tempo " + due
		if first.LateDays > 0 {
			v += fmt.Sprintf(" (lewat %d hari)", first.LateDays)
		}
		s.Facts = append(s.Facts, Fact{"Tagihan tertua", v})
	}
	s.Note = "Data kredit internal — periksa dulu sebelum disampaikan ke pelanggan."
	s.draft(r, map[string]string{"limit": rupiahPlain(c.Limit), "sisa_limit": rupiahPlain(max(room, 0)), "tagihan": rupiahPlain(open),
		"jumlah_tagihan": fmt.Sprint(len(d.Dealer.Invoices)), "jatuh_tempo": due})
}

// orderStatus is how a phase is told to the customer.
var orderStatus = [5]string{"sedang diproses", "siap dikirim", "dalam pengiriman", "sudah ditagihkan", "sudah lunas"}

func orderCard(s *Suggestion, r Rule, d Data) {
	if d.Dealer == nil {
		s.Headline, s.Note = "Dealer belum dikenali", "Percakapan ini belum tertaut ke dealer — status order tidak bisa dibaca."
		return
	}
	o := d.Dealer.LastOrder
	if o == nil {
		s.Headline, s.Note = "Belum ada order", "Tidak ada order dealer ini dalam 6 bulan terakhir."
		s.draft(r, map[string]string{})
		return
	}
	ph := min(max(o.Phase, 0), 4)
	s.Ready = true
	s.Headline, s.Tone = fmt.Sprintf("%s · %s", o.Number, domain.Phases[ph]), "good"
	date := ""
	if o.Date != nil {
		date = tanggal(*o.Date)
	}
	s.Facts = append(s.Facts, Fact{"Order terakhir", o.Number}, Fact{"Tanggal", date}, Fact{"Status", domain.Phases[ph]}, Fact{"Total", rupiah(o.Total)})
	s.draft(r, map[string]string{"nomor": o.Number, "tanggal": date, "status": orderStatus[ph], "total": rupiahPlain(o.Total)})
}

// findIn finds the product in the message, else in the earlier messages (newest first): the card then tells which
// message named it (FromHistory).
func (s *Suggestion) findIn(text string, history []Line, cat []*product) (*product, []string) {
	p, cands := find(text, cat)
	if p != nil || len(cands) > 0 {
		return p, cands
	}
	for _, h := range history[:min(len(history), HistoryLines)] {
		if hp, hc := find(h.Text, cat); hp != nil || len(hc) > 0 { // several products fit there: the sales picks
			s.FromHistory = clip(h.Text, 90)
			return hp, hc
		}
	}
	return nil, nil
}

func clip(t string, n int) string {
	t = strings.Join(strings.Fields(t), " ")
	if r := []rune(t); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return t
}

func unknownProduct(s *Suggestion, cands []string, src string) {
	s.Products = cands
	s.Headline = "Produk tidak dikenali"
	s.Note = "Produk tidak dikenali di " + src + ", juga tidak di " + fmt.Sprint(HistoryLines) + " pesan sebelumnya — tanyakan produknya atau cek manual."
	if len(cands) > 0 {
		s.Headline, s.Note = "Beberapa produk cocok", "Beberapa produk cocok — pilih yang dimaksud pelanggan."
	}
}

// stopWords never identify a product.
var stopWords = map[string]bool{"stok": true, "stock": true, "ready": true, "redy": true, "ada": true, "barang": true, "tersedia": true,
	"masih": true, "gak": true, "ga": true, "tidak": true, "kosong": true, "inden": true, "pak": true, "bu": true, "mas": true, "mbak": true,
	"kak": true, "ko": true, "ci": true, "min": true, "nya": true, "yang": true, "untuk": true, "dan": true, "atau": true, "ya": true, "yg": true,
	"berapa": true, "harga": true, "unit": true, "pcs": true, "buah": true, "sama": true, "ini": true, "itu": true, "dong": true, "kah": true,
	"apa": true, "mau": true, "order": true, "pesan": true, "ambil": true, "cek": true, "tolong": true, "di": true, "ke": true,
	"harganya": true, "price": true, "pricelist": true, "brp": true, "skrg": true, "sekarang": true}

type product struct {
	sku, name string
	toks      []string
	byBranch  map[string]int   // stock
	list      int64            // price
	prices    map[string]int64 // price
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

// Placeholders lists the {placeholders} each action fills (shown on the settings page).
var Placeholders = map[string][]string{
	ActionStock:  {"produk", "jumlah", "cabang", "cabang_lain", "harga"},
	ActionPrice:  {"produk", "harga"},
	ActionCredit: {"limit", "sisa_limit", "tagihan", "jumlah_tagihan", "jatuh_tempo"},
	ActionOrder:  {"nomor", "tanggal", "status", "total"},
}

// num formats 1234567 as 1.234.567.
func num(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprint(v)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func rupiah(v int64) string      { return "Rp " + num(v) }
func rupiahPlain(v int64) string { return num(v) } // templates write "Rp {harga}"

var bulan = [12]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func tanggal(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), bulan[t.Month()-1], t.Year())
}
