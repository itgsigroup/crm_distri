package agents

import (
	"regexp"
	"strconv"
	"strings"
)

// Intent of an inbound WhatsApp message (AI Order extraction schema: order | ask_price | ask_stock | other,
// plus release and return which AI Kredit and AI Order handle).
const (
	IntentOrder   = "order"
	IntentPrice   = "ask_price"
	IntentStock   = "ask_stock"
	IntentRelease = "release"
	IntentReturn  = "return"
	IntentOther   = "other"
)

// Item is a requested product matched to the catalog.
type Item struct {
	Product Product
	Qty     int64
}

// Extraction is what the Go extractor reads from a message.
type Extraction struct {
	Intent      string
	Items       []Item
	DiscountPct float64 // "3% di bawah"
	Day         string  // requested delivery day
}

var (
	reQty   = regexp.MustCompile(`(?i)\b(\d{1,4})\s*(?:unit|pcs|buah|x|×)?\s*(?:\b|$)`)
	rePct   = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*%`)
	reSplit = regexp.MustCompile(`(?i),|;|\+|\bdan\b|\bsama\b`)
	reTime  = regexp.MustCompile(`(?i)^\s*(minggu|hari|bulan|jam|tahun)`)
)

func tokens(s string) []string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("(", " ", ")", " ", "/", " ", "-", " ", ".", " ", "&", " ").Replace(s)
	return strings.Fields(s)
}

// Extract parses intent, items and terms from a WA message against the catalog (deterministic; the LLM may
// improve it later but the numbers always come from here).
func Extract(text string, catalog []Product) Extraction {
	low := strings.ToLower(text)
	e := Extraction{Intent: IntentOther, Day: DayIn(text)}
	switch {
	case strings.Contains(low, "rilis"):
		e.Intent = IntentRelease
	case strings.Contains(low, "tukar") || strings.Contains(low, "retur") || strings.Contains(low, "rusak") || strings.Contains(low, "mati total"):
		e.Intent = IntentReturn
	case strings.Contains(low, "%") || strings.Contains(low, "samain") || strings.Contains(low, "lebih murah") || strings.Contains(low, "harga khusus"):
		e.Intent = IntentPrice
	case strings.Contains(low, "order") || strings.Contains(low, "pesan") || strings.Contains(low, "ambil"):
		e.Intent = IntentOrder
	case strings.Contains(low, "stok") || strings.Contains(low, "ready"):
		e.Intent = IntentStock
	case strings.Contains(low, "harga"):
		e.Intent = IntentPrice
	}
	if m := rePct.FindStringSubmatch(text); m != nil {
		e.DiscountPct, _ = strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	}
	for _, clause := range reSplit.Split(text, -1) {
		if it, ok := matchClause(clause, catalog); ok {
			e.Items = append(e.Items, it)
		}
	}
	return e
}

func matchClause(clause string, catalog []Product) (Item, bool) {
	toks := tokens(clause)
	set := map[string]bool{}
	for _, t := range toks {
		set[t] = true
	}
	var best Product
	bestScore, tie := 0, false
	for _, p := range catalog {
		if p.Name == best.Name {
			continue
		}
		score := 0
		for _, t := range tokens(p.Name) {
			if set[t] {
				score++
			}
		}
		if score > bestScore {
			best, bestScore, tie = p, score, false
		} else if score == bestScore && score > 0 && len(p.Name) < len(best.Name) {
			best = p // prefer the plainer product ("Kamera IP 4MP" over "Kamera IP 4MP dome")
		} else if score == bestScore && score > 0 {
			tie = tie || len(p.Name) == len(best.Name)
		}
	}
	if bestScore == 0 || tie {
		return Item{}, false
	}
	// quantity: the first standalone number that is not a duration ("2 minggu") or part of a model ("4MP")
	for _, m := range reQty.FindAllStringSubmatchIndex(clause, -1) {
		numEnd := m[3]
		if numEnd < len(clause) {
			next := clause[numEnd]
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') {
				continue // "4MP", "16ch"
			}
		}
		if reTime.MatchString(clause[numEnd:]) {
			continue
		}
		n, _ := strconv.ParseInt(clause[m[2]:m[3]], 10, 64)
		if n > 0 {
			return Item{Product: best, Qty: n}, true
		}
	}
	return Item{}, false
}
