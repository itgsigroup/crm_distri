package replyrule

import (
	"testing"

	"distri-arc/internal/domain"
)

var ezviz = []domain.StockItem{
	{SKU: "H8c 2MP", Name: "Ezviz Camera H8c 2mp", Branch: "Semarang", Qty: 5},
	{SKU: "H8c 2MP", Name: "Ezviz Camera H8c 2mp", Branch: "Surabaya", Qty: 202},
	{SKU: "H8C_3MP_Poe", Name: "Ezviz Camera H8c 3MP PoE", Branch: "Semarang", Qty: 110},
	{SKU: "X5S_8W", Name: "Ezviz NVR 8Ch X5S 8W", Branch: "Semarang", Qty: 42},
	{SKU: "X5S_4W", Name: "Ezviz NVR 4Ch X5S 4W", Branch: "Semarang", Qty: 47},
	{SKU: "HDD-1TB", Name: "HDD 1TB surveillance", Branch: "Semarang", Qty: 9},
}

// Candidates offer Claude the products sharing words with the message (model codes and SKU words count more).
func TestCandidates(t *testing.T) {
	c := Candidates("H8c 2mp : 8 @580rb, sekalian nvr ezviz X5S-8W 1 unit", nil, ezviz, 40)
	if len(c) != 4 || c[0].SKU != "X5S_8W" || c[1].SKU != "H8c 2MP" {
		t.Fatalf("candidates: %+v", c)
	}
	for _, x := range c {
		if x.SKU == "HDD-1TB" {
			t.Fatal("an unrelated product offered")
		}
		if x.SKU == "H8c 2MP" && x.Qty != 207 {
			t.Fatalf("total stock: %+v", x)
		}
	}
	// a question without a product: the chat before it gives the words
	if c := Candidates("stoknya ada?", []Line{{In: true, Text: "x5s 4w harganya?"}}, ezviz, 40); len(c) == 0 || c[0].SKU != "X5S_4W" {
		t.Fatalf("from history: %+v", c)
	}
}

// The products Claude found: several make one card with a line each, the quantity asked compared with the stock.
func TestSuggestMatched(t *testing.T) {
	d := Data{Branch: "Semarang", Stock: ezviz, Products: []Product{{SKU: "X5S_8W", Name: "Ezviz NVR 8Ch X5S 8W", ListPrice: 865000}}}
	text := "Ok aku pesen H8c 2mp : 8 @580rb sekalian nvr ezviz X5S-8W ready? 1 unit"
	m := &Match{Items: []MatchItem{{SKU: "H8c 2MP", Qty: 8}, {SKU: "x5s_8w", Qty: 1}}, Model: "claude-haiku-5-5"}
	cs := SuggestMatched([]Rule{rule}, text, nil, d, m)
	if len(cs) != 1 {
		t.Fatalf("cards: %d", len(cs))
	}
	s := cs[0]
	if len(s.Items) != 2 || s.MatchedBy != "claude-haiku-5-5" || s.Ready || s.Tone != "warn" || s.Headline != "2 barang · 1 ready, 1 kurang/kosong" {
		t.Fatalf("card: %+v", s)
	}
	h8, nvr := s.Items[0], s.Items[1]
	if h8.Qty != 5 || h8.Asked != 8 || h8.Ready || h8.Headline != "Kurang · 5 dari 8 diminta" || h8.Note == "" {
		t.Fatalf("H8c (5 in Semarang, 8 asked): %+v", h8)
	}
	if nvr.Qty != 42 || !nvr.Ready || nvr.Price != "Rp 865.000" || nvr.Headline != "Ready · 42 di Semarang · diminta 1, cukup" {
		t.Fatalf("NVR: %+v", nvr)
	}
	v := s.Vars()
	if v["produk"] != "Ezviz Camera H8c 2mp" || v["diminta"] != "8" || v["produk_2"] != "Ezviz NVR 8Ch X5S 8W" || v["harga_2"] != "865.000" ||
		s.Draft != "Ready 5 pcs untuk Ezviz Camera H8c 2mp di Semarang. Ready 42 pcs untuk Ezviz NVR 8Ch X5S 8W di Semarang." {
		t.Fatalf("vars %v draft %q", v, s.Draft)
	}
	// one product: the card as before, with what was asked
	one := SuggestMatched([]Rule{rule}, text, nil, d, &Match{Items: []MatchItem{{SKU: "X5S_8W", Qty: 100}}, Model: "c"})[0]
	if len(one.Items) != 0 || one.Product != "Ezviz NVR 8Ch X5S 8W" || one.Ready || one.Headline != "Kurang · 42 dari 100 diminta" {
		t.Fatalf("one product: %+v", one)
	}
	// Claude found nothing (the type was unclear): no product guessed by words
	none := SuggestMatched([]Rule{rule}, "stok nvr ezviz ready?", nil, d, &Match{Items: []MatchItem{}, Note: "X5S 4W atau X5S 8W?", Model: "c"})[0]
	if none.Product != "" || none.Headline != "Produk tidak dikenali" || none.Draft != "" {
		t.Fatalf("nothing found: %+v", none)
	}
	s = SuggestMatched([]Rule{rule}, "stok x5s 8w?", nil, d, nil)[0]
	Searching(s)
	if s.Product != "" || !s.AIPending || s.Draft != "" {
		t.Fatalf("searching: %+v", s)
	}
}
