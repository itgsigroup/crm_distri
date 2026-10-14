package importer

import (
	"math"
	"strings"
)

// stockCost is the HPP of the stock snapshot per SKU × branch (and per SKU over every branch), weighted by quantity.
type stockCost struct {
	at, all map[string][2]float64 // key → (Σ cost×qty, Σ qty)
}

func newStockCost() *stockCost {
	return &stockCost{at: map[string][2]float64{}, all: map[string][2]float64{}}
}

func (s *stockCost) add(sku, branch string, cost int64, qty float64) {
	w := math.Max(qty, 1) // an empty shelf still tells its HPP
	for m, k := range map[*map[string][2]float64]string{&s.at: sku + "\x00" + branch, &s.all: sku} {
		v := (*m)[k]
		(*m)[k] = [2]float64{v[0] + float64(cost)*w, v[1] + w}
	}
}

// get is the HPP of a SKU in a branch, else over every branch; 0 when the stock does not know it.
func (s *stockCost) get(sku, branch string) int64 {
	if v, ok := s.at[sku+"\x00"+branch]; ok && v[1] > 0 {
		return int64(math.Round(v[0] / v[1]))
	}
	if v, ok := s.all[sku]; ok && v[1] > 0 {
		return int64(math.Round(v[0] / v[1]))
	}
	return 0
}

// lineCost is the unit HPP of an invoice line. Some sources send the line's total HPP in the cost column: a cost far
// above the unit price that is a plausible unit HPP once divided by the quantity is read as that total (a margin of
// −100 % or worse is never real; 14 Oct review, task 13).
func lineCost(cost, qty, price, amount int64) int64 {
	if cost <= 0 || qty <= 1 {
		return max(cost, 0)
	}
	unit := price
	if unit <= 0 && amount > 0 {
		unit = amount / qty
	}
	if unit > 0 && cost > unit*3/2 {
		if per := cost / qty; per <= unit*3/2 {
			return per
		}
	}
	return cost
}

// voided: an invoice status that means it does not count (batal, void, cancel, dibatalkan).
func voided(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	for _, w := range []string{"batal", "void", "cancel"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// returnDoc: a sales return document (retur penjualan, credit note).
func returnDoc(kind string) bool {
	s := strings.ToLower(strings.TrimSpace(kind))
	return strings.Contains(s, "retur") || strings.Contains(s, "return") || strings.Contains(s, "credit")
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
