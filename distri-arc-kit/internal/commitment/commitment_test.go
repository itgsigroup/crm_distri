package commitment

import (
	"testing"
	"time"

	"distri-arc/internal/clock"
)

func TestWhen(t *testing.T) {
	today := time.Date(2026, 10, 5, 0, 0, 0, 0, clock.WIB) // Monday
	cases := []struct {
		text string
		want string
		ok   bool
	}{
		{"saya transfer hari Kamis", "2026-10-08", true},
		{"besok ya mas", "2026-10-06", true},
		{"lunas minggu depan", "2026-10-12", true},
		{"bayar tgl 20", "2026-10-20", true},
		{"bayar tanggal 2", "2026-11-02", true},
		{"Senin depan saya cicil", "2026-10-12", true},
		{"nanti saya kabari", "", false},
	}
	for _, c := range cases {
		got, ok := When(c.text, today)
		if ok != c.ok || (ok && got.Format("2006-01-02") != c.want) {
			t.Errorf("When(%q) = %s %v, want %s %v", c.text, got.Format("2006-01-02"), ok, c.want, c.ok)
		}
	}
}

func TestPatterns(t *testing.T) {
	for _, s := range []string{"Ya mbak, kirim aja seperti biasa", "oke lanjut", "siap"} {
		if !reYes.MatchString(s) {
			t.Errorf("yes: %q", s)
		}
	}
	for _, s := range []string{"belum dulu", "stok masih banyak"} {
		if reYes.MatchString(s) {
			t.Errorf("not yes: %q", s)
		}
	}
	if !rePay.MatchString("INV/0964 saya transfer hari Kamis") || rePay.MatchString("kirim aja") {
		t.Error("pay pattern")
	}
}
