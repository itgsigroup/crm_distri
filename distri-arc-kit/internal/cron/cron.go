// Package cron parses 5-field cron expressions (minute hour day month weekday) evaluated in WIB, and describes
// them in Bahasa Indonesia for the UI. Supported: *, numbers, ranges a-b, steps */n and a-b/n, lists, weekday 0–7
// (0 and 7 are Sunday) and the macros @hourly, @daily, @weekly, @monthly. Like classic cron, when both day and
// weekday are restricted a time matches either of them.
package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"distri-arc/internal/clock"
)

// MinGap is the shortest allowed time between two runs (each run calls the model).
const MinGap = 15 * time.Minute

// Spec is a parsed expression.
type Spec struct {
	expr                     string
	min, hour, dom, mon, dow [61]bool
	domAny, dowAny           bool
}

var macros = map[string]string{"@hourly": "0 * * * *", "@daily": "0 0 * * *", "@weekly": "0 0 * * 0", "@monthly": "0 0 1 * *"}

// Parse reads an expression; errors are in Bahasa Indonesia (shown in the schedule form).
func Parse(expr string) (Spec, error) {
	expr = strings.Join(strings.Fields(expr), " ")
	if m, ok := macros[expr]; ok {
		expr = m
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return Spec{}, errors.New("cron harus 5 bagian: menit jam tanggal bulan hari (mis. 0 7 * * 1-6)")
	}
	s := Spec{expr: expr, domAny: f[2] == "*", dowAny: f[4] == "*"}
	parts := []struct {
		name   string
		lo, hi int
		dst    *[61]bool
	}{{"menit", 0, 59, &s.min}, {"jam", 0, 23, &s.hour}, {"tanggal", 1, 31, &s.dom}, {"bulan", 1, 12, &s.mon}, {"hari", 0, 7, &s.dow}}
	for i, p := range parts {
		if err := field(f[i], p.lo, p.hi, p.dst); err != nil {
			return Spec{}, fmt.Errorf("bagian %s: %w", p.name, err)
		}
	}
	if s.dow[7] {
		s.dow[0] = true
	}
	return s, nil
}

func field(v string, lo, hi int, dst *[61]bool) error {
	for _, part := range strings.Split(v, ",") {
		rng, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepStr)
			if err != nil || n < 1 {
				return fmt.Errorf("langkah %q tidak valid", stepStr)
			}
			step = n
		}
		a, b := lo, hi
		switch {
		case rng == "*":
		case strings.Contains(rng, "-"):
			x, y, _ := strings.Cut(rng, "-")
			var err1, err2 error
			a, err1 = strconv.Atoi(x)
			b, err2 = strconv.Atoi(y)
			if err1 != nil || err2 != nil || a > b {
				return fmt.Errorf("rentang %q tidak valid", rng)
			}
		default:
			n, err := strconv.Atoi(rng)
			if err != nil {
				return fmt.Errorf("%q bukan angka", rng)
			}
			a = n
			if !hasStep {
				b = n
			}
		}
		if a < lo || b > hi {
			return fmt.Errorf("%q di luar %d–%d", rng, lo, hi)
		}
		for i := a; i <= b; i += step {
			dst[i] = true
		}
	}
	return nil
}

// String returns the normalised expression.
func (s Spec) String() string { return s.expr }

func (s Spec) dayOK(t time.Time) bool {
	if !s.mon[int(t.Month())] {
		return false
	}
	d, w := s.dom[t.Day()], s.dow[int(t.Weekday())]
	switch {
	case s.domAny && s.dowAny:
		return true
	case s.domAny:
		return w
	case s.dowAny:
		return d
	}
	return d || w
}

// Next returns the first matching minute strictly after t (WIB); zero when none within ~5 years (e.g. 31 Feb).
func (s Spec) Next(t time.Time) time.Time {
	t = t.In(clock.WIB).Truncate(time.Minute).Add(time.Minute)
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, clock.WIB)
	for i := 0; i < 366*5; i, day = i+1, day.AddDate(0, 0, 1) {
		if !s.dayOK(day) {
			continue
		}
		for h := 0; h < 24; h++ {
			if !s.hour[h] {
				continue
			}
			for m := 0; m < 60; m++ {
				if !s.min[m] {
					continue
				}
				if c := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, clock.WIB); !c.Before(t) {
					return c
				}
			}
		}
	}
	return time.Time{}
}

// NextN lists the next n run times after t.
func (s Spec) NextN(t time.Time, n int) []time.Time {
	var out []time.Time
	for len(out) < n {
		t = s.Next(t)
		if t.IsZero() {
			break
		}
		out = append(out, t)
	}
	return out
}

// CheckGap rejects schedules that fire more often than MinGap (looking at the next week of runs).
func (s Spec) CheckGap(now time.Time) error {
	runs := s.NextN(now, 200)
	if len(runs) == 0 {
		return errors.New("jadwal ini tidak pernah jalan")
	}
	end := now.Add(7 * 24 * time.Hour)
	for i := 1; i < len(runs) && runs[i].Before(end); i++ {
		if runs[i].Sub(runs[i-1]) < MinGap {
			return fmt.Errorf("terlalu sering: jarak antar-analisis minimal %d menit", int(MinGap.Minutes()))
		}
	}
	return nil
}

var dayNames = []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// Describe says when the expression runs, in Bahasa Indonesia ("Senin–Sabtu pukul 07.00").
func Describe(expr string) string {
	s, err := Parse(expr)
	if err != nil {
		return ""
	}
	f := strings.Fields(s.expr)
	mi, ho, dm, mo, dw := f[0], f[1], f[2], f[3], f[4]
	days := describeDays(s, dw)
	if dm != "*" {
		days = "tanggal " + strings.ReplaceAll(dm, ",", ", ") + " tiap bulan"
		if dw != "*" {
			days += " atau " + describeDays(s, dw)
		}
	}
	if mo != "*" {
		days += " (bulan " + strings.ReplaceAll(mo, ",", ", ") + ")"
	}
	var when string
	switch {
	case isNum(mi) && isNum(ho):
		when = "pukul " + hm(ho, mi)
	case isNum(mi) && strings.Contains(ho, ","):
		hs := strings.Split(ho, ",")
		for i := range hs {
			hs[i] = hm(hs[i], mi)
		}
		when = "pukul " + strings.Join(hs, ", ")
	case isNum(mi):
		rng, step, hasStep := strings.Cut(ho, "/")
		when = "setiap jam"
		if hasStep {
			when = "setiap " + step + " jam"
		}
		if rng != "*" {
			a, b, _ := strings.Cut(rng, "-")
			if b == "" {
				b = "23"
			}
			when += ", " + hm(a, mi) + "–" + hm(b, mi)
		} else if mi != "0" {
			when += " lewat " + mi + " menit"
		}
	case strings.HasPrefix(mi, "*/") && ho == "*":
		when = "setiap " + strings.TrimPrefix(mi, "*/") + " menit"
	default:
		return "Cron " + s.expr
	}
	if days == "setiap hari" && strings.HasPrefix(when, "setiap") {
		return upper(when) // "Setiap 4 jam", not "Setiap hari setiap 4 jam"
	}
	return upper(days) + " " + when
}

func describeDays(s Spec, dw string) string {
	switch dw {
	case "*":
		return "setiap hari"
	case "1-5":
		return "Senin–Jumat"
	case "1-6":
		return "Senin–Sabtu"
	case "0,6", "6,0", "6-7", "6,7":
		return "Sabtu & Minggu"
	}
	var names []string
	for d := 1; d <= 7; d++ {
		if s.dow[d%7] {
			names = append(names, dayNames[d%7])
		}
	}
	if len(names) == 1 {
		return "setiap " + names[0]
	}
	return strings.Join(names, ", ")
}

func isNum(v string) bool { _, err := strconv.Atoi(v); return err == nil }

func hm(h, m string) string {
	hi, _ := strconv.Atoi(h)
	mi, _ := strconv.Atoi(m)
	return fmt.Sprintf("%02d.%02d", hi, mi)
}

func upper(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
