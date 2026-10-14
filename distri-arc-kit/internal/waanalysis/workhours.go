package waanalysis

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"distri-arc/internal/domain"
)

// WorkHours are the working hours response time is measured in (ADR 0035, policy chat.rules), in WIB.
type WorkHours struct {
	Days       map[time.Weekday]bool
	Start, End int // minutes after midnight
}

var wib = time.FixedZone("WIB", 7*3600)

// DefaultWorkHours: Monday–Saturday 08.00–17.00 WIB.
func DefaultWorkHours() WorkHours { return HoursOf(domain.DefaultPolicies().Chat) }

// HoursOf reads the policy; an unreadable one falls back to 08.00–17.00 on its days (or Monday–Saturday).
func HoursOf(p domain.ChatPolicy) WorkHours {
	h := WorkHours{Days: map[time.Weekday]bool{}, Start: hhmm(p.WorkStart, 8*60), End: hhmm(p.WorkEnd, 17*60)}
	for _, d := range p.WorkDays {
		if d >= 0 && d <= 6 {
			h.Days[time.Weekday(d)] = true
		}
	}
	if len(h.Days) == 0 {
		for d := time.Monday; d <= time.Saturday; d++ {
			h.Days[d] = true
		}
	}
	if h.End <= h.Start {
		h.Start, h.End = 8*60, 17*60
	}
	return h
}

func hhmm(s string, def int) int {
	hs, ms, ok := strings.Cut(strings.TrimSpace(s), ":")
	h, err1 := strconv.Atoi(hs)
	m, err2 := strconv.Atoi(ms)
	if !ok || err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return def
	}
	return h*60 + m
}

func (h WorkHours) zero() bool { return len(h.Days) == 0 }

// In reports whether t falls inside working hours.
func (h WorkHours) In(t time.Time) bool {
	t = t.In(wib)
	m := t.Hour()*60 + t.Minute()
	return h.Days[t.Weekday()] && m >= h.Start && m < h.End
}

// Minutes counts the working minutes between from and to.
func (h WorkHours) Minutes(from, to time.Time) int {
	if !to.After(from) {
		return 0
	}
	from, to = from.In(wib), to.In(wib)
	total := 0.0
	day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, wib)
	for !day.After(to) {
		if h.Days[day.Weekday()] {
			lo, hi := day.Add(time.Duration(h.Start)*time.Minute), day.Add(time.Duration(h.End)*time.Minute)
			if lo.Before(from) {
				lo = from
			}
			if hi.After(to) {
				hi = to
			}
			if hi.After(lo) {
				total += hi.Sub(lo).Minutes()
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return int(total + 0.5)
}

// percentile is the p-th percentile (nearest rank) of xs; nil when empty.
func percentile(xs []int, p float64) *int {
	if len(xs) == 0 {
		return nil
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	i := int(float64(len(s))*p+0.999999) - 1
	i = max(0, min(i, len(s)-1))
	return &s[i]
}

// Ints converts stored minutes (an int[] column).
func Ints(xs []int32) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = int(x)
	}
	return out
}
