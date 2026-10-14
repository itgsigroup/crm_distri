package replyrule

import (
	"testing"

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

func TestSuggest(t *testing.T) {
	if s := Suggest([]Rule{rule}, "Pak kamera 4MP ready?", "Semarang", stock); s == nil || !s.Ready || s.Product != "Kamera IP 4MP" || s.Qty != 20 ||
		s.Draft != "Ready 20 pcs untuk Kamera IP 4MP di Semarang." || len(s.Others) != 1 || s.Others[0].Branch != "Jakarta" {
		t.Fatalf("ready: %+v", s)
	}
	s := Suggest([]Rule{rule}, "stok NVR 16ch ada?", "Semarang", stock)
	if s == nil || s.Ready || s.Draft != "Mohon maaf, NVR 16ch di Semarang sedang kosong." || s.Others[0].Qty != 7 || s.Note == "" {
		t.Fatalf("empty with another branch: %+v", s)
	}
	if s := Suggest([]Rule{rule}, "harga kamera 4mp berapa?", "Semarang", stock); s != nil {
		t.Fatalf("no keyword: %+v", s)
	}
	if s := Suggest([]Rule{rule}, "stok kamera ada?", "Semarang", stock); s == nil || s.Product != "" || s.Note == "" {
		t.Fatalf("product not named: %+v", s)
	}
	if s := Suggest([]Rule{rule}, "masih tersedia barang tsb?", "Semarang", stock); s == nil || s.Product != "" {
		t.Fatalf("keyword without product: %+v", s)
	}
	noQty := rule
	noQty.ShowQty = false
	if s := Suggest([]Rule{noQty}, "HDD 2TB ready?", "Semarang", stock); s == nil || s.Draft != "Ready untuk HDD 2TB di Semarang." {
		t.Fatalf("without quantity: %+v", s)
	}
	if s := Suggest([]Rule{rule}, "kamera 4mp ready?", "", stock); s == nil || s.Qty != 25 || s.Draft != "Ready 25 pcs untuk Kamera IP 4MP di gudang kami." {
		t.Fatalf("no branch: %+v", s)
	}
	if Matches(rule, "readymix beton") != "" || Matches(rule, "Ada barang nya?") != "ada barang" {
		t.Fatal("whole-word keywords")
	}
}
