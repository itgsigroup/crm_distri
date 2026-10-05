package agents

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"distri-arc/internal/domain"
)

// Rp formats like the mockup: "Rp 62 jt", "Rp 1,28 M".
func Rp(v int64) string {
	if v >= 1_000_000_000 {
		s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(v)/1e9), "0"), ".")
		return "Rp " + strings.Replace(s, ".", ",", 1) + " M"
	}
	return fmt.Sprintf("Rp %d jt", int64(math.Round(float64(v)/1e6)))
}

// Rp1 keeps one decimal below Rp 100 jt ("Rp 18,4 jt").
func Rp1(v int64) string {
	if v >= 100_000_000 || v%1_000_000 == 0 {
		return Rp(v)
	}
	return "Rp " + strings.Replace(strconv.FormatFloat(math.Round(float64(v)/1e5)/10, 'f', -1, 64), ".", ",", 1) + " jt"
}

// Jt is a bare number of millions ("176").
func Jt(v int64) string {
	f := math.Round(float64(v)/1e5) / 10
	return strings.Replace(strconv.FormatFloat(f, 'f', -1, 64), ".", ",", 1)
}

// Rb formats thousands ("Rp 478 rb").
func Rb(v int64) string { return fmt.Sprintf("Rp %d rb", int64(math.Round(float64(v)/1e3))) }

// Pct formats a percentage with one decimal and a comma ("8,3%").
func Pct(f float64) string {
	return strings.Replace(strconv.FormatFloat(math.Round(f*10)/10, 'f', 1, 64), ".", ",", 1) + "%"
}

// Short drops PT/CV/UD/Toko.
func Short(name string) string {
	for _, p := range []string{"PT ", "CV ", "UD ", "Toko "} {
		name = strings.TrimPrefix(name, p)
	}
	return name
}

// Primary returns the main contact of a dealer (primary flag, else most interactions).
func Primary(d *Dealer) domain.Contact {
	best := domain.Contact{}
	for _, c := range d.Contacts {
		if c.IsPrimary {
			return c
		}
		if c.Interactions90d > best.Interactions90d || best.Name == "" {
			best = c
		}
	}
	return best
}

// Bapak returns "Bapak" or "Ibu" for a contact's honorific (used inside drafts).
func Bapak(name string) string {
	if strings.HasPrefix(name, "Bu ") || strings.HasPrefix(name, "Mbak ") {
		return "Ibu"
	}
	return "Bapak"
}

var dayRe = regexp.MustCompile(`(?i)\b(senin|selasa|rabu|kamis|jumat|sabtu|minggu)\b`)

// DayIn returns the weekday a message asks for ("Senin"), or "". "minggu" counts only as Sunday when it is not
// a duration ("2 minggu") or a relative week ("minggu depan", "minggu ini").
func DayIn(text string) string {
	low := strings.ToLower(text)
	for _, m := range dayRe.FindAllStringIndex(low, -1) {
		w := low[m[0]:m[1]]
		if w == "minggu" {
			before := strings.TrimSpace(low[:m[0]])
			after := strings.TrimSpace(low[m[1]:])
			if (before != "" && before[len(before)-1] >= '0' && before[len(before)-1] <= '9') ||
				strings.HasPrefix(after, "depan") || strings.HasPrefix(after, "ini") || strings.HasPrefix(after, "lalu") {
				continue
			}
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		return string(r)
	}
	return ""
}

// Join lists names with commas and "dan".
func Join(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	default:
		return strings.Join(xs[:len(xs)-1], ", ") + " dan " + xs[len(xs)-1]
	}
}
