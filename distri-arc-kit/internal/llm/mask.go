package llm

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	rePhone = regexp.MustCompile(`(?:\+?62|0)8\d{7,12}|\+?62[\s-]?8\d{2}[\s-]?\d{3,4}[\s-]?\d{3,5}`)
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	reNIK   = regexp.MustCompile(`\b\d{16}\b`)
	reAcct  = regexp.MustCompile(`\b\d{10,15}\b`)
)

// Masker replaces PII with placeholders before a request leaves the server and restores them in the answer.
// The mapping lives only in memory for the duration of one call.
type Masker struct {
	toPlaceholder map[string]string
	fromPh        map[string]string
	n             map[string]int
}

// NewMasker builds an empty masker.
func NewMasker() *Masker {
	return &Masker{toPlaceholder: map[string]string{}, fromPh: map[string]string{}, n: map[string]int{}}
}

func (m *Masker) ph(kind, v string) string {
	if p, ok := m.toPlaceholder[v]; ok {
		return p
	}
	m.n[kind]++
	p := fmt.Sprintf("<%s_%d>", kind, m.n[kind])
	m.toPlaceholder[v], m.fromPh[p] = p, v
	return p
}

// Mask replaces phone numbers, e-mails, NIK, bank accounts and the given person names.
func (m *Masker) Mask(s string, names []string) string {
	// longest names first so "Pak Budi Santoso" wins over "Budi"
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			continue
		}
		s = strings.ReplaceAll(s, n, m.ph("PIC", n))
	}
	s = reEmail.ReplaceAllStringFunc(s, func(v string) string { return m.ph("EMAIL", v) })
	s = rePhone.ReplaceAllStringFunc(s, func(v string) string { return m.ph("NO", v) })
	s = reNIK.ReplaceAllStringFunc(s, func(v string) string { return m.ph("NIK", v) })
	s = reAcct.ReplaceAllStringFunc(s, func(v string) string { return m.ph("REK", v) })
	return s
}

// Unmask restores the placeholders.
func (m *Masker) Unmask(s string) string {
	for p, v := range m.fromPh {
		s = strings.ReplaceAll(s, p, v)
	}
	return s
}
