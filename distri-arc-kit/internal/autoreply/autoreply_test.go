package autoreply

import (
	"slices"
	"testing"

	"distri-arc/internal/domain"
)

// A brand without a type (Sam 29 Oct: "cek stok cctv hilook semarang kak" got "sales akan mengabari"): the brand's
// products in stock in the named city, most stock first; another brand that shares only "camera" is left out.
func TestBrowse(t *testing.T) {
	stock := []domain.StockItem{
		{Branch: "Semarang", SKU: "THC-T120-PC", Name: "Hilook Camera Analog 4in1 DM 2MP THC-T120-PC", Qty: 501},
		{Branch: "Semarang", SKU: "NVR-108MH-D", Name: "Hilook NVR 8ch NVR-108MH-D", Qty: 63},
		{Branch: "Surabaya", SKU: "IPC-D140HA-LUC", Name: "Hilook Camera IP 4MP IPC-D140HA-LUC", Qty: 900},
		{Branch: "Semarang", SKU: "THC-B140-P", Name: "Hilook Camera 4in1 BL 4MP THC-B140-P", Qty: 0},
		{Branch: "Semarang", SKU: "HWT-1", Name: "Holowits Camera IP 4MP HWT-1", Qty: 40},
	}
	got := Browse("cek stok cctv hilook semarang kak", stock)
	if want := []string{"THC-T120-PC", "NVR-108MH-D"}; !slices.Equal(got, want) {
		t.Fatalf("Semarang: %v, want %v", got, want)
	}
	// no city named: every city, most stock first
	if got := Browse("hilook ready?", stock); len(got) != 3 || got[0] != "IPC-D140HA-LUC" {
		t.Fatalf("no city: %v", got)
	}
	// the city has none of them: any city
	if got := Browse("hilook jakarta", append(stock, domain.StockItem{Branch: "Jakarta", SKU: "X", Name: "Ezviz X", Qty: 1})); len(got) != 3 {
		t.Fatalf("Jakarta: %v", got)
	}
	if got := Browse("halo selamat siang", stock); len(got) != 0 {
		t.Fatalf("no product word: %v", got)
	}
}

// Harga dimatikan (Sam 29 Oct: "sales tidak mau memberikan harga"): a reply with a rupiah amount is not sent.
func TestHasPrice(t *testing.T) {
	for _, s := range []string{"Harganya Rp 2.500.000/unit", "rp2500000", "cuma 250rb kak", "2,5 jt saja", "dalam rupiah"} {
		if !HasPrice(s) {
			t.Errorf("%q: no price found", s)
		}
	}
	for _, s := range []string{"Stok di Semarang ada 9 unit", "Harganya akan kami konfirmasi ya Pak", "NVR 32ch ready", "kirim ke jl. rpx 12"} {
		if HasPrice(s) {
			t.Errorf("%q: price found", s)
		}
	}
}
