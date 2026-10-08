package cron

import (
	"testing"
	"time"

	"distri-arc/internal/clock"
)

// Wednesday 7 Oct 2026, 16.12 WIB
var now = time.Date(2026, 10, 7, 16, 12, 30, 0, clock.WIB)

func TestNext(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"0 7 * * 1-6", "2026-10-08 07:00"},    // tomorrow (Thursday)
		{"0 7 * * 1", "2026-10-12 07:00"},      // next Monday
		{"30 16 * * *", "2026-10-07 16:30"},    // later today
		{"0 */4 * * *", "2026-10-07 20:00"},    // 0,4,8,…,20
		{"0 7-19/3 * * *", "2026-10-07 19:00"}, // 7,10,13,16,19
		{"0 8 1 * *", "2026-11-01 08:00"},      // first of the month
		{"0 7 * * 0", "2026-10-11 07:00"},      // Sunday as 0
		{"0 7 * * 7", "2026-10-11 07:00"},      // Sunday as 7
		{"@daily", "2026-10-08 00:00"},
		{"0 9 15 * 1", "2026-10-12 09:00"}, // day or weekday: Monday 12 comes before the 15th
	}
	for _, c := range cases {
		s, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got := s.Next(now).Format("2006-01-02 15:04"); got != c.want {
			t.Errorf("%s: next %s, want %s", c.expr, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, e := range []string{"", "0 7 * *", "60 7 * * *", "0 24 * * *", "0 7 * * 8", "0 7 0 * *", "a 7 * * *", "0 9-7 * * *", "*/0 * * * *"} {
		if _, err := Parse(e); err == nil {
			t.Errorf("%q: want error", e)
		}
	}
}

func TestCheckGap(t *testing.T) {
	ok := []string{"0 7 * * 1-6", "*/15 * * * *", "0 * * * *"}
	bad := []string{"*/5 * * * *", "* * * * *", "0,10 7 * * *"}
	for _, e := range ok {
		if s, _ := Parse(e); s.CheckGap(now) != nil {
			t.Errorf("%s: unexpected gap error", e)
		}
	}
	for _, e := range bad {
		if s, _ := Parse(e); s.CheckGap(now) == nil {
			t.Errorf("%s: want gap error", e)
		}
	}
	if s, _ := Parse("0 7 31 2 *"); s.CheckGap(now) == nil {
		t.Error("31 Feb never runs: want error")
	}
}

func TestDescribe(t *testing.T) {
	cases := map[string]string{
		"0 7 * * 1-6":     "Senin–Sabtu pukul 07.00",
		"30 6 * * *":      "Setiap hari pukul 06.30",
		"0 8 * * 1":       "Setiap Senin pukul 08.00",
		"0 7-19 * * 1-5":  "Senin–Jumat setiap jam, 07.00–19.00",
		"0 */4 * * *":     "Setiap 4 jam",
		"0 7,12,17 * * *": "Setiap hari pukul 07.00, 12.00, 17.00",
		"0 8 1 * *":       "Tanggal 1 tiap bulan pukul 08.00",
		"*/30 * * * *":    "Setiap 30 menit",
		"0 9 * * 1,3,5":   "Senin, Rabu, Jumat pukul 09.00",
	}
	for e, want := range cases {
		if got := Describe(e); got != want {
			t.Errorf("%s: %q, want %q", e, got, want)
		}
	}
}
