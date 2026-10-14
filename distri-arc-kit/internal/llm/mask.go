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
	// credentials typed in a chat (ADR 0035): "password: abc123", "pwd = x", "kata sandi Gsi2024", "PIN 123456"
	reCredSep = regexp.MustCompile(`(?i)\b(password|passwd|pass|pwd|sandi|kata\s*sandi|pin|otp|kode\s+otp|kode\s+verifikasi|token)((?:\s+[\pL\d]+){0,2}\s*[:=]\s*)(\S+)`)
	reCredSp  = regexp.MustCompile(`(?i)\b(password|passwd|pwd|sandi|kata\s*sandi|pin|otp|kode\s+otp|kode\s+verifikasi)(\s+)(\S*\d\S*)`)
	// reUUID: record ids (a conversation, a message) are not PII — the last group of a UUID can be 12 digits, which
	// reAcct would take for a bank account and the answer could no longer name the record
	reUUID = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
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

// MaskCredentials hides passwords, PINs and codes typed in a message ("password: ••••••") — in the CRM and
// before any LLM. The words stay so the reader knows a credential was sent (and should be changed).
func MaskCredentials(s string) string {
	s = reCredSep.ReplaceAllString(s, "$1$2••••••")
	return reCredSp.ReplaceAllString(s, "$1$2••••••")
}

// Mask replaces credentials, phone numbers, e-mails, NIK, bank accounts and the given person names.
func (m *Masker) Mask(s string, names []string) string {
	var ids []string
	s = reUUID.ReplaceAllStringFunc(s, func(v string) string {
		ids = append(ids, v)
		return fmt.Sprintf("\x00%d\x00", len(ids)-1)
	})
	s = MaskCredentials(s)
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
	for i, v := range ids {
		s = strings.Replace(s, fmt.Sprintf("\x00%d\x00", i), v, 1)
	}
	return s
}

// Unmask restores the placeholders.
func (m *Masker) Unmask(s string) string {
	for p, v := range m.fromPh {
		s = strings.ReplaceAll(s, p, v)
	}
	return s
}

// Mapping returns placeholder → original value (kept server-side to unmask a later answer).
func (m *Masker) Mapping() map[string]string {
	out := make(map[string]string, len(m.fromPh))
	for k, v := range m.fromPh {
		out[k] = v
	}
	return out
}

// UnmaskWith restores placeholders from a stored mapping.
func UnmaskWith(s string, mapping map[string]string) string {
	for p, v := range mapping {
		s = strings.ReplaceAll(s, p, v)
	}
	return s
}
