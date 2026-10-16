package replyai

import "testing"

func TestCheckDraft(t *testing.T) {
	tpl := "Harga Kamera IP 4MP Rp 472.500 per unit."
	allowed := "harga kamera 4mp? " + tpl + " 472.500 Kamera IP 4MP"
	for _, c := range []struct {
		draft string
		ok    bool
	}{
		{"Untuk Kamera IP 4MP harganya Rp 472.500 per unit, Pak.", true},
		{"Kamera IP 4MP Rp472500 per unit Pak, minimal 1 unit.", true}, // separators and a small count are free
		{"Kamera IP 4MP harganya terjangkau, Pak.", false},             // the price was dropped
		{"Kamera IP 4MP Rp 472.500, diskon 10% untuk 5 unit.", false},  // invented numbers
		{"", false},
	} {
		if err := CheckDraft(tpl, allowed, c.draft); (err == nil) != c.ok {
			t.Errorf("%q: %v", c.draft, err)
		}
	}
}
