package llm

import (
	"fmt"
	"regexp"
	"strings"
)

// PII patterns masked before any payload leaves the server (docs/knowledge/04).
var (
	// The country code may be followed by a separator ("+62 812-…", the format used in Odoo and WhatsApp).
	rePhone   = regexp.MustCompile(`(?:\+62|62|0)[ .\-]?8[0-9][0-9\- .•]{6,14}[0-9]`)
	reEmail   = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reAccount = regexp.MustCompile(`(?i)(?:rek(?:ening)?\.?|no\.? ?rek\.?|account)\s*[:#]?\s*([0-9][0-9\- ]{7,20}[0-9])`)
)

// Masker replaces PII with stable tokens ([PHONE_1], [EMAIL_1], [REK_1]) and
// restores them in the response.
type Masker struct {
	forward map[string]string
	reverse map[string]string
	counts  map[string]int
}

// NewMasker returns an empty masker for one request.
func NewMasker() *Masker {
	return &Masker{forward: map[string]string{}, reverse: map[string]string{}, counts: map[string]int{}}
}

func (m *Masker) token(kind, value string) string {
	if t, ok := m.forward[value]; ok {
		return t
	}
	m.counts[kind]++
	t := fmt.Sprintf("[%s_%d]", kind, m.counts[kind])
	m.forward[value] = t
	m.reverse[t] = value
	return t
}

// Mask replaces phone numbers, e-mail addresses and bank account numbers.
func (m *Masker) Mask(s string) string {
	if s == "" {
		return s
	}
	s = reAccount.ReplaceAllStringFunc(s, func(match string) string {
		sub := reAccount.FindStringSubmatch(match)
		return strings.Replace(match, sub[1], m.token("REK", sub[1]), 1)
	})
	s = reEmail.ReplaceAllStringFunc(s, func(v string) string { return m.token("EMAIL", v) })
	s = rePhone.ReplaceAllStringFunc(s, func(v string) string { return m.token("PHONE", v) })
	return s
}

// Unmask restores the original values.
func (m *Masker) Unmask(s string) string {
	for t, v := range m.reverse {
		s = strings.ReplaceAll(s, t, v)
	}
	return s
}

// ContainsRawPhone reports whether s still holds an unmasked phone number.
func ContainsRawPhone(s string) bool { return rePhone.MatchString(s) }
