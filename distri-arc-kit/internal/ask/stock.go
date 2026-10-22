package ask

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"distri-arc/internal/domain"
	"distri-arc/internal/stockanalysis"
	"distri-arc/internal/views"
)

// A stock question answered as asked (Sam 22 Oct: "saya tanya stok tapi Orchestrator menampilkan dealer"): a product
// named → its stock per branch; fast / dead / slow moving, reorder, transfers, age → that list; a branch or a category
// → its stock; push / bundle → the aging stock with its candidate dealers (the only stock answer that names dealers);
// anything else → the stock summary. Numbers from stockanalysis, the same as Cek stok's Analisa stok.

var (
	reFast     = regexp.MustCompile(`(?i)fast ?moving|laris|paling laku|cepat laku|cepat terjual`)
	reDead     = regexp.MustCompile(`(?i)dead ?stock|tidak laku|gak laku|ga laku|nggak laku|tidak terjual|stok mati`)
	reSlow     = regexp.MustCompile(`(?i)slow ?moving|lambat|kurang laku`)
	reReorder  = regexp.MustCompile(`(?i)re-?order|restock|habis|kritis|kosong|perlu (di)?pesan|beli lagi|menipis`)
	reTransfer = regexp.MustCompile(`(?i)mutasi|transfer|pindah(kan)? (stok|barang)`)
	rePush     = regexp.MustCompile(`(?i)push|didorong|bundle|dorong`)
	reAge      = regexp.MustCompile(`(?i)umur|menua|aging|lama di gudang`)
	reWord     = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}\-_./]*`)
)

// stopWords are the words of a question that do not name a product.
var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`stok stock barang produk item ada berapa apa apakah yang di ke dari untuk unit pcs
		masih ready tersedia sisa cek tolong mohon dong ya kah gak tidak cabang gudang semua total jumlah tampilkan lihat
		harga nilai berapakah sekarang hari ini kita kami saya info minta mau the and & per dan atau ini itu`) {
		stopWords[w] = true
	}
}

const stockTop = 8

func (s *Service) stockAnswer(q string, b *views.Board, items []domain.StockItem) (string, []views.BoardItem) {
	ql := strings.ToLower(q)
	if rePush.MatchString(ql) {
		return stock(b, items)
	}
	branch := branchIn(ql, items)
	a := stockanalysis.Compute(items, branch, func(string, string) int64 { return 0 }, b.Policies.Stock)
	where := "semua cabang"
	if branch != "" {
		where = "cabang " + branch
	}
	if a.SKUs == 0 {
		return "Belum ada data stok di " + where + ".", nil
	}
	switch {
	case reFast.MatchString(ql):
		return listAnswer("Barang fast moving di "+where, filter(a.Products, func(p stockanalysis.Product) bool { return p.Class == stockanalysis.Fast }), byPace,
			func(p stockanalysis.Product) string {
				return fmt.Sprintf("%s/mgg, stok %s unit (cukup %s)", num(p.PerWeek), qty(p.Qty), cover(p.CoverDays))
			},
			"Belum ada barang fast moving di "+where+"."), nil
	case reDead.MatchString(ql):
		return listAnswer("Dead stock di "+where+" (tidak terjual 90 hari)", filter(a.Products, func(p stockanalysis.Product) bool { return p.Class == stockanalysis.Dead }), byValue,
			func(p stockanalysis.Product) string {
				return fmt.Sprintf("%s unit, %s, umur %d hari", qty(p.Qty), stockanalysis.Rp(p.Value), p.AgeDays)
			},
			"Tidak ada dead stock di "+where+"."), nil
	case reSlow.MatchString(ql):
		return listAnswer("Slow moving di "+where+" (stok cukup lebih dari 90 hari)", filter(a.Products, func(p stockanalysis.Product) bool { return p.Class == stockanalysis.Slow }), byValue,
			func(p stockanalysis.Product) string {
				return fmt.Sprintf("%s unit, %s, cukup %s", qty(p.Qty), stockanalysis.Rp(p.Value), cover(p.CoverDays))
			},
			"Tidak ada slow moving di "+where+"."), nil
	case reTransfer.MatchString(ql):
		if branch != "" {
			a = stockanalysis.Compute(items, "", func(string, string) int64 { return 0 }, b.Policies.Stock)
		}
		if len(a.Transfers) == 0 {
			return "Tidak ada peluang mutasi antar cabang: tidak ada barang yang diam di satu cabang sementara cabang lain kekurangan.", nil
		}
		var parts []string
		for _, t := range a.Transfers[:min(stockTop, len(a.Transfers))] {
			parts = append(parts, fmt.Sprintf("%s (%s) %s unit dari %s ke %s", t.Name, t.SKU, qty(t.Qty), t.From, t.To))
		}
		return fmt.Sprintf("%d peluang mutasi antar cabang: %s.", len(a.Transfers), strings.Join(parts, "; ")), nil
	case reReorder.MatchString(ql):
		return listAnswer(fmt.Sprintf("Perlu reorder di %s (habis kurang dari %.0f hari)", where, a.CriticalDays), filter(a.Products, func(p stockanalysis.Product) bool { return p.Critical }), byCover,
			func(p stockanalysis.Product) string {
				return fmt.Sprintf("stok %s unit, habis %s, saran pesan ±%s unit", qty(p.Qty), cover(p.CoverDays), qty(p.ReorderQty))
			},
			"Tidak ada barang yang akan habis dalam "+fmt.Sprintf("%.0f", a.CriticalDays)+" hari di "+where+"."), nil
	case reAge.MatchString(ql):
		var parts []string
		for _, bk := range a.AgeBuckets {
			if bk.SKUs > 0 {
				parts = append(parts, fmt.Sprintf("%s hari: %d barang (%s)", bk.Key, bk.SKUs, stockanalysis.Rp(bk.Value)))
			}
		}
		return fmt.Sprintf("Umur stok di %s — %s. Stok menua lebih dari %d hari: %d barang senilai %s.", where, strings.Join(parts, "; "), a.AgingDays, a.AgingSKUs, stockanalysis.Rp(a.AgingValue)), nil
	}
	if found := products(ql, branch, a.Products); len(found) > 0 {
		return productAnswer(found, branch), nil
	}
	if cat := categoryIn(ql, a.Categories); cat != nil {
		ps := filter(a.Products, func(p stockanalysis.Product) bool { return strings.EqualFold(p.Category, cat.Key) })
		t := fmt.Sprintf("Kategori %s di %s: %d barang, nilai stok %s (%d%%)", cat.Key, where, cat.SKUs, stockanalysis.Rp(cat.Value), cat.Pct)
		if cat.Turnover != nil {
			t += fmt.Sprintf(", perputaran %d hari", *cat.Turnover)
		}
		return t + ". " + listAnswer("Terbesar", ps, byValue, func(p stockanalysis.Product) string {
			return fmt.Sprintf("%s unit, %s", qty(p.Qty), stockanalysis.Rp(p.Value))
		}, ""), nil
	}
	if words := productWords(ql, items); len(words) > 0 {
		return fmt.Sprintf("Barang \"%s\" tidak ditemukan di data stok %s. Coba nama lain atau kode barangnya, atau cari di menu Cek stok.", strings.Join(words, " "), where), nil
	}
	return summary(a, where), nil
}

func summary(a stockanalysis.Analysis, where string) string {
	t := fmt.Sprintf("Stok %s: %d barang, %s unit, nilai %s.", where, a.SKUs, qty(a.Units), stockanalysis.Rp(a.Value))
	if a.Turnover != nil {
		t += fmt.Sprintf(" Perputaran %d hari (target ≤ %d).", *a.Turnover, a.TurnoverGoal)
	}
	counts := map[string]int{}
	for _, c := range a.Classes {
		counts[c.Key] = c.SKUs
	}
	t += fmt.Sprintf(" Fast moving %d, slow moving %d, dead stock %d barang; %d perlu reorder; %d barang menua lebih dari %d hari.",
		counts["fast"], counts["slow"], counts["dead"], a.CriticalSKUs, a.AgingSKUs, a.AgingDays)
	if len(a.Branches) > 1 {
		var parts []string
		for _, b := range a.Branches {
			parts = append(parts, fmt.Sprintf("%s %s", b.Key, stockanalysis.Rp(b.Value)))
		}
		t += " Per cabang: " + strings.Join(parts, ", ") + "."
	}
	return t + " Rinciannya ada di Cek stok → Analisa stok."
}

func productAnswer(ps []stockanalysis.Product, branch string) string {
	var parts []string
	for _, p := range ps[:min(5, len(ps))] {
		t := fmt.Sprintf("%s (%s): %s unit", p.Name, p.SKU, qty(p.Qty))
		var at []string
		for _, b := range p.Branches {
			if branch == "" || b.Branch == branch {
				at = append(at, fmt.Sprintf("%s %s", b.Branch, qty(b.Qty)))
			}
		}
		if len(at) > 0 && branch == "" {
			t += " — " + strings.Join(at, ", ")
		}
		if p.PerWeek > 0 {
			t += fmt.Sprintf("; terjual %s/mgg, cukup %s", num(p.PerWeek), cover(p.CoverDays))
		} else {
			t += "; tidak terjual 90 hari"
		}
		t += "; " + classLabel[p.Class]
		if p.Critical && p.ReorderQty > 0 {
			t += fmt.Sprintf(", perlu reorder ±%s unit", qty(p.ReorderQty))
		}
		parts = append(parts, t)
	}
	out := strings.Join(parts, ". ") + "."
	if len(ps) > 5 {
		out += fmt.Sprintf(" Dan %d barang lain yang cocok — cari di Cek stok.", len(ps)-5)
	}
	return out
}

var classLabel = map[stockanalysis.Class]string{stockanalysis.Fast: "fast moving", stockanalysis.Normal: "normal", stockanalysis.Slow: "slow moving", stockanalysis.Dead: "dead stock"}

// products: the products whose name or code holds every product word of the question.
func products(ql, branch string, all []stockanalysis.Product) []stockanalysis.Product {
	words := questionWords(ql, branch)
	if len(words) == 0 {
		return nil
	}
	var out []stockanalysis.Product
	for _, p := range all {
		hay := strings.ToLower(p.Name + " " + p.SKU)
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, byValue)
	return out
}

// questionWords: the words that may name a product (no stop word, no branch).
func questionWords(ql, branch string) []string {
	var out []string
	for _, w := range reWord.FindAllString(ql, -1) {
		w = strings.Trim(w, "-_./")
		if w == "" || stopWords[w] || (branch != "" && strings.Contains(strings.ToLower(branch), w)) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// productWords: the question's product words when none is a branch or category word (for "not found").
func productWords(ql string, items []domain.StockItem) []string {
	words := questionWords(ql, branchIn(ql, items))
	var out []string
	for _, w := range words {
		if len([]rune(w)) >= 2 {
			out = append(out, w)
		}
	}
	return out
}

// branchIn: the branch the question names.
func branchIn(ql string, items []domain.StockItem) string {
	seen := map[string]bool{}
	for _, s := range items {
		if seen[s.Branch] || s.Branch == "" {
			continue
		}
		seen[s.Branch] = true
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(strings.ToLower(s.Branch)) + `\b`).MatchString(ql) {
			return s.Branch
		}
	}
	return ""
}

// categoryIn: the category the question names (its first word is enough: "kamera", "kabel", "led").
func categoryIn(ql string, cats []stockanalysis.Share) *stockanalysis.Share {
	for i := range cats {
		k := strings.ToLower(cats[i].Key)
		if strings.Contains(ql, k) {
			return &cats[i]
		}
		for _, w := range strings.FieldsFunc(k, func(r rune) bool { return r == ' ' || r == '&' }) {
			if len([]rune(w)) >= 3 && regexp.MustCompile(`\b`+regexp.QuoteMeta(w)+`\b`).MatchString(ql) {
				return &cats[i]
			}
		}
	}
	return nil
}

func filter(ps []stockanalysis.Product, keep func(stockanalysis.Product) bool) []stockanalysis.Product {
	var out []stockanalysis.Product
	for _, p := range ps {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}

func listAnswer(title string, ps []stockanalysis.Product, order func(a, b stockanalysis.Product) int, line func(stockanalysis.Product) string, empty string) string {
	if len(ps) == 0 {
		return empty
	}
	slices.SortStableFunc(ps, order)
	var parts []string
	for _, p := range ps[:min(stockTop, len(ps))] {
		parts = append(parts, fmt.Sprintf("%s (%s) %s", p.Name, p.SKU, line(p)))
	}
	t := fmt.Sprintf("%s: %d barang. %s.", title, len(ps), strings.Join(parts, "; "))
	if len(ps) > stockTop {
		t += fmt.Sprintf(" Dan %d lainnya di Cek stok → Analisa stok.", len(ps)-stockTop)
	}
	return t
}

func byValue(a, b stockanalysis.Product) int { return cmp64(b.Value, a.Value) }
func byPace(a, b stockanalysis.Product) int  { return cmpF(b.PerWeek, a.PerWeek) }
func byCover(a, b stockanalysis.Product) int {
	if a.CoverDays == nil || b.CoverDays == nil {
		return 0
	}
	return cmpF(*a.CoverDays, *b.CoverDays)
}

func cmp64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// qty is "1.053": thousands with dots.
func qty(n int) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func num(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%d", int(f))
	}
	return strings.Replace(fmt.Sprintf("%.1f", f), ".", ",", 1)
}

func cover(d *float64) string {
	if d == nil {
		return "—"
	}
	if *d < 1 {
		return "kurang dari 1 hari"
	}
	return fmt.Sprintf("%.0f hari", *d)
}
