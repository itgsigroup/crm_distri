package replyrule

import (
	"testing"
	"time"

	"distri-arc/internal/domain"
)

var rule = Rule{ID: "r1", Name: "Tanya stok", Action: "stock", ShowQty: true,
	Keywords:      []string{"stok", "ready", "ada barang", "tersedia"},
	TemplateReady: "Ready {jumlah} pcs untuk {produk} di {cabang}.", TemplateEmpty: "Mohon maaf, {produk} di {cabang} sedang kosong."}

var stock = []domain.StockItem{
	{SKU: "K4", Name: "Kamera IP 4MP", Branch: "Semarang", Qty: 20},
	{SKU: "K4", Name: "Kamera IP 4MP", Branch: "Jakarta", Qty: 5},
	{SKU: "K4D", Name: "Kamera IP 4MP Dome", Branch: "Semarang", Qty: 3},
	{SKU: "N16", Name: "NVR 16ch", Branch: "Semarang", Qty: 0},
	{SKU: "N16", Name: "NVR 16ch", Branch: "Jakarta", Qty: 7},
	{SKU: "H2", Name: "HDD 2TB", Branch: "Semarang", Qty: 4},
}

func one(t *testing.T, rules []Rule, text string, d Data) *Suggestion {
	t.Helper()
	cs := Suggest(rules, text, nil, d)
	if len(cs) > 1 {
		t.Fatalf("%q: %d cards", text, len(cs))
	}
	if len(cs) == 0 {
		return nil
	}
	return cs[0]
}

func TestSuggest(t *testing.T) {
	sm := Data{Branch: "Semarang", Stock: stock}
	if s := one(t, []Rule{rule}, "Pak kamera 4MP ready?", sm); s == nil || !s.Ready || s.Product != "Kamera IP 4MP" || s.Qty != 20 ||
		s.Draft != "Ready 20 pcs untuk Kamera IP 4MP di Semarang." || len(s.Others) != 1 || s.Others[0].Branch != "Jakarta" || s.Tone != "good" {
		t.Fatalf("ready: %+v", s)
	}
	s := one(t, []Rule{rule}, "stok NVR 16ch ada?", sm)
	if s == nil || s.Ready || s.Draft != "Mohon maaf, NVR 16ch di Semarang sedang kosong." || s.Others[0].Qty != 7 || s.Note == "" {
		t.Fatalf("empty with another branch: %+v", s)
	}
	if s := one(t, []Rule{rule}, "harga kamera 4mp berapa?", sm); s != nil {
		t.Fatalf("no keyword: %+v", s)
	}
	if s := one(t, []Rule{rule}, "stok kamera ada?", sm); s == nil || s.Product != "" || s.Note == "" || s.Draft != "" {
		t.Fatalf("product not named: %+v", s)
	}
	if s := one(t, []Rule{rule}, "masih tersedia barang tsb?", sm); s == nil || s.Product != "" {
		t.Fatalf("keyword without product: %+v", s)
	}
	noQty := rule
	noQty.ShowQty = false
	if s := one(t, []Rule{noQty}, "HDD 2TB ready?", sm); s == nil || s.Draft != "Ready untuk HDD 2TB di Semarang." || s.Vars()["jumlah"] != "" {
		t.Fatalf("without quantity: %+v", s)
	}
	if s := one(t, []Rule{rule}, "kamera 4mp ready?", Data{Stock: stock}); s == nil || s.Qty != 25 || s.Draft != "Ready 25 pcs untuk Kamera IP 4MP di gudang kami." {
		t.Fatalf("no branch: %+v", s)
	}
	if Matches(rule, "readymix beton") != "" || Matches(rule, "Ada barang nya?") != "ada barang" || Matches(rule, "stoknya ada?") != "stok" ||
		Matches(rule, "stokis") != "" {
		t.Fatal("whole-word keywords")
	}
}

var (
	priceRule = Rule{ID: "p", Name: "Tanya harga", Action: ActionPrice, Keywords: []string{"harga", "price"},
		TemplateReady: "Harga {produk} Rp {harga} per unit.", TemplateEmpty: "Harga {produk} kami cek dulu."}
	creditRule = Rule{ID: "c", Name: "Limit", Action: ActionCredit, Keywords: []string{"limit", "tagihan"},
		TemplateReady: "Sisa limit Rp {sisa_limit} dari Rp {limit}.", TemplateEmpty: "Ada {jumlah_tagihan} tagihan Rp {tagihan}, jatuh tempo {jatuh_tempo}."}
	orderRule = Rule{ID: "o", Name: "Order", Action: ActionOrder, Keywords: []string{"status order", "kapan dikirim"},
		TemplateReady: "Order {nomor} tanggal {tanggal} {status}.", TemplateEmpty: "Pesanan kami cek dulu."}
	products = []Product{
		{SKU: "K4", Name: "Kamera IP 4MP", ListPrice: 450000, Prices: map[string]int64{"A": 450000, "B": 472500, "C": 495000}},
		{SKU: "N16", Name: "NVR 16ch", ListPrice: 0},
	}
)

func TestPrice(t *testing.T) {
	d := Data{Products: products, Dealer: &Dealer{Name: "Mitra", Tier: "B"}}
	s := one(t, []Rule{priceRule}, "harga kamera 4mp brp pak?", d)
	if s == nil || !s.Ready || s.Draft != "Harga Kamera IP 4MP Rp 472.500 per unit." || s.Headline != "Rp 472.500 · harga tier B" {
		t.Fatalf("tier price: %+v", s)
	}
	if s := one(t, []Rule{priceRule}, "harga kamera 4mp?", Data{Products: products}); s == nil || s.Vars()["harga"] != "450.000" || s.Note == "" {
		t.Fatalf("no dealer: list price: %+v", s)
	}
	if s := one(t, []Rule{priceRule}, "price NVR 16ch", d); s == nil || s.Ready || s.Draft != "Harga NVR 16ch kami cek dulu." {
		t.Fatalf("no price: %+v", s)
	}
}

func TestCreditAndOrder(t *testing.T) {
	room := 0.4
	due := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	ok := &Dealer{Name: "Mitra", Credit: domain.Credit{Limit: 50_000_000, Exposure: 30_000_000, Room: &room, State: domain.CreditAman},
		Invoices: []Invoice{{Number: "INV/1", DueAt: due, Residual: 30_000_000}}}
	s := one(t, []Rule{creditRule}, "sisa limit saya berapa?", Data{Dealer: ok})
	if s == nil || !s.Ready || s.Draft != "Sisa limit Rp 20.000.000 dari Rp 50.000.000." || s.Tone != "good" {
		t.Fatalf("aman: %+v", s)
	}
	late := *ok
	late.Credit.State, late.Credit.LateDays = domain.CreditOverdue, 16
	late.Invoices = []Invoice{{Number: "INV/1", DueAt: due, Residual: 30_000_000, LateDays: 16}}
	if s := one(t, []Rule{creditRule}, "tagihan saya?", Data{Dealer: &late}); s == nil || s.Ready || s.Tone != "bad" ||
		s.Draft != "Ada 1 tagihan Rp 30.000.000, jatuh tempo 30 Sep 2026." {
		t.Fatalf("overdue: %+v", s)
	}
	if s := one(t, []Rule{creditRule}, "limit?", Data{}); s == nil || s.Draft != "" || s.Note == "" {
		t.Fatalf("no dealer: %+v", s)
	}
	at := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	withOrder := &Dealer{Name: "Mitra", LastOrder: &Order{Number: "SO123", Date: &at, Phase: 2, Total: 12_000_000}}
	if s := one(t, []Rule{orderRule}, "Status order saya gimana?", Data{Dealer: withOrder}); s == nil || s.Draft != "Order SO123 tanggal 14 Okt 2026 dalam pengiriman." {
		t.Fatalf("order: %+v", s)
	}
	if s := one(t, []Rule{orderRule}, "kapan dikirim ya", Data{Dealer: &Dealer{Name: "Mitra"}}); s == nil || s.Ready || s.Draft != "Pesanan kami cek dulu." {
		t.Fatalf("no order: %+v", s)
	}
}

func TestSeveralCards(t *testing.T) {
	cs := Suggest([]Rule{rule, priceRule, {ID: "r2", Action: ActionStock, Keywords: []string{"ready"}}}, "harga dan stok kamera 4MP ready?", nil,
		Data{Branch: "Semarang", Stock: stock, Products: products})
	if len(cs) != 2 || cs[0].Action != ActionStock || cs[0].RuleID != "r1" || cs[1].Action != ActionPrice {
		t.Fatalf("one card per kind, first rule wins: %+v", cs)
	}
	if n := Needs([]Rule{rule, priceRule, creditRule}, "harga kamera"); !n[ActionPrice] || n[ActionStock] || n[ActionCredit] {
		t.Fatalf("needs: %v", n)
	}
}

// "stoknya ada?" names no product: the card takes it from the newest earlier message that names one.
func TestProductFromHistory(t *testing.T) {
	d := Data{Branch: "Semarang", Stock: stock, Products: products}
	history := []Line{{In: true, Text: "oke pak"}, {In: false, Text: "Untuk NVR 16ch bisa kami bantu"}, {In: true, Text: "kamera 4mp ada?"}}
	cs := Suggest([]Rule{rule}, "stoknya masih ada?", history, d)
	if len(cs) != 1 || cs[0].Product != "NVR 16ch" || cs[0].FromHistory != "Untuk NVR 16ch bisa kami bantu" || cs[0].Ready {
		t.Fatalf("product from history: %+v", cs)
	}
	if cs := Suggest([]Rule{rule}, "stok kamera 4mp ada?", history, d); cs[0].Product != "Kamera IP 4MP" || cs[0].FromHistory != "" {
		t.Fatalf("the message's own product wins: %+v", cs[0])
	}
	if cs := Suggest([]Rule{priceRule}, "harganya berapa?", history[2:], d); cs[0].Product != "Kamera IP 4MP" || cs[0].FromHistory == "" {
		t.Fatalf("price from history: %+v", cs[0])
	}
	if cs := Suggest([]Rule{rule}, "stoknya ada?", []Line{{In: true, Text: "kamera ip 4mp dome atau bullet?"}}, Data{Stock: append(stock[:3:3], domain.StockItem{SKU: "K4X", Name: "Kamera IP 4MP Dome", Qty: 1})}); cs[0].Product != "" || len(cs[0].Products) != 2 || cs[0].FromHistory == "" {
		t.Fatalf("several products in history: %+v", cs[0])
	}
	if cs := Suggest([]Rule{rule}, "stoknya ada?", []Line{{In: true, Text: "halo"}}, d); cs[0].Product != "" || cs[0].Note == "" {
		t.Fatalf("no product anywhere: %+v", cs[0])
	}
}
