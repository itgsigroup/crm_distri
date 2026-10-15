package api

import "testing"

// Searching Chat forgives the spellings of one name and finds a conversation by its number (15 Oct: "yohanes" did
// not find the dealer stored as "JOHANNES YUDHAYANTO").
func TestThreadMatcher(t *testing.T) {
	const number = "6281901337568"
	for _, c := range []struct {
		term, text string
		want       bool
	}{
		{"yohanes", "JOHANNES YUDHAYANTO · JOHANNES YUDHAYANTO", true},
		{"Johanes", "Yohanes Adi Prasetyo", true},
		{"yusuf", "Toko Joesoef Elektrik", true},
		{"budi", "Toko Sinar Jaya", false},
		{"0819-0133", "Toko Lain", true},
		{"7568", "+62 819-••••-7568", true},
		{"62811", "Toko Lain", false},
		{"pak yo", "JOHANNES YUDHAYANTO", true},
		{"Bu Sinta", "SINTA DEWI · Toko Sinar", true},
		{"pak", "Pak Priyo", true},
	} {
		if got := threadMatcher(c.term)(c.text, number); got != c.want {
			t.Errorf("%q in %q: %v, want %v", c.term, c.text, got, c.want)
		}
	}
	if threadMatcher("  ") != nil {
		t.Error("an empty search must not filter")
	}
}
