package agents

import "testing"

var catalog = []Product{
	{Name: "Kamera IP 4MP"}, {Name: "Kamera IP 2MP"}, {Name: "Kamera analog"}, {Name: "NVR 8ch"}, {Name: "NVR 16ch"},
	{Name: "NVR 32ch"}, {Name: "NVR 16/32ch"}, {Name: "HDD 4TB"}, {Name: "Modul LED P5"},
}

func TestExtract(t *testing.T) {
	cases := []struct {
		text   string
		intent string
		items  map[string]int64
		pct    float64
		day    string
	}{
		{"Mas Andi, order ya: kamera 4MP dome 10 unit, NVR 16ch 1 unit. Kirim Senin bisa?", IntentOrder, map[string]int64{"Kamera IP 4MP": 10, "NVR 16ch": 1}, 0, "Senin"},
		{"Mas Fajar, rilis 24 unit kamera 4MP ya, termin 30 hari seperti biasa. Kirim Rabu.", IntentRelease, map[string]int64{"Kamera IP 4MP": 24}, 0, "Rabu"},
		{"Pak Rizky, untuk 60 unit 4MP, distributor lain kasih 3% di bawah harga Bapak. Bisa disamain?", IntentPrice, map[string]int64{"Kamera IP 4MP": 60}, 3, ""},
		{"2 NVR 8ch mati total setelah 2 minggu, minta tukar.", IntentReturn, map[string]int64{"NVR 8ch": 2}, 0, ""},
		{"Invoice kemarin sudah kami transfer ya.", IntentOther, map[string]int64{}, 0, ""},
	}
	for _, c := range cases {
		e := Extract(c.text, catalog)
		if e.Intent != c.intent || e.DiscountPct != c.pct || e.Day != c.day {
			t.Errorf("%q: intent %s pct %v day %q", c.text, e.Intent, e.DiscountPct, e.Day)
		}
		got := map[string]int64{}
		for _, it := range e.Items {
			got[it.Product.Name] += it.Qty
		}
		if len(got) != len(c.items) {
			t.Errorf("%q: items %v want %v", c.text, got, c.items)
		}
		for k, v := range c.items {
			if got[k] != v {
				t.Errorf("%q: %s = %d want %d", c.text, k, got[k], v)
			}
		}
	}
}
