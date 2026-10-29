package hermes

import (
	"encoding/json"
	"strings"
	"testing"

	"distri-arc/internal/domain"
	"distri-arc/internal/replyrule"
)

var stock = []domain.StockItem{
	{Branch: "Semarang", SKU: "X5S_8W", Name: "Ezviz NVR 8Ch X5S 8W", Qty: 12},
	{Branch: "Surabaya", SKU: "X5S_8W", Name: "Ezviz NVR 8Ch X5S 8W", Qty: 30},
	{Branch: "Jakarta", SKU: "X5S_8W", Name: "Ezviz NVR 8Ch X5S 8W", Qty: 0},
	{Branch: "Semarang", SKU: "H8c 2MP", Name: "Ezviz Camera H8c 2mp", Qty: 5},
}

// Lyra follows the stock, price and product trigger (Sam 28 Oct): a text or a photo's caption with one of their
// words; a photo without one (a screenshot to a colleague) and a text without one are not read.
func TestAsks(t *testing.T) {
	rules := []replyrule.Rule{
		{Action: replyrule.ActionStock, Keywords: []string{"stok", "ada barang"}},
		{Action: replyrule.ActionPrice, Keywords: []string{"hrg"}},
		{Action: replyrule.ActionCredit, Keywords: []string{"tagihan"}},
	}
	for _, c := range []struct {
		q    Question
		want bool
	}{
		{Question{Text: "stoknya nvr ezviz ada?"}, true},
		{Question{Text: "hrg H8c 2mp brp"}, true},                // the price rule's own word
		{Question{Text: "produk ini ready?", Image: true}, true}, // ProductRule, on a photo's caption
		{Question{Text: "harga kamera 4MP"}, true},               // ProductRule
		{Question{Image: true}, false},                           // a photo without words
		{Question{Text: "disini bisa mbak gran", Image: true}, false},
		{Question{Text: "H8c 2mp : 8"}, false},
		{Question{Text: "tagihan bulan ini berapa"}, false}, // credit, not a product
		{Question{Text: "terima kasih pak"}, false},
	} {
		if got := Asks(c.q, rules); got != c.want {
			t.Errorf("Asks(%+v) = %v, want %v", c.q, got, c.want)
		}
	}
}

// Lyra's SKUs are kept when the catalog knows them (any case); a name is matched by its model code; the rest is
// reported as missing; duplicates count once; quantities stay in range.
func TestResolve(t *testing.T) {
	var a lyraAnswer
	_ = json.Unmarshal([]byte(jsonBlock("Berikut:\n```json\n"+`{"tanya_produk":true,"items":[{"sku":"x5s_8w","qty":2},{"sku":"","nama":"Ezviz H8C 2MP","qty":-3},{"sku":"X5S_8W","qty":1},{"sku":"","nama":"Hikvision ColorVu 5MP","qty":1}],"catatan":"ok"}`+"\n```")), &a)
	known := map[string]string{"x5s_8w": "X5S_8W", "h8c 2mp": "H8c 2MP"}
	items, missing := resolve(a, known, stock)
	if len(items) != 2 || items[0] != (ProductItem{SKU: "X5S_8W", Qty: 2}) || items[1] != (ProductItem{SKU: "H8c 2MP", Qty: 0}) {
		t.Fatalf("items: %+v", items)
	}
	if len(missing) != 1 || missing[0] != "Hikvision ColorVu 5MP" {
		t.Fatalf("missing: %v", missing)
	}
}

// The card: stock per city (none-stocked cities left out, most first) with warehouses when a city has several,
// and the DPP price at the dealer's tier, else the list price.
func TestBuildCard(t *testing.T) {
	r := ProductRead{Status: "done", Items: []ProductItem{{SKU: "X5S_8W", Qty: 3}, {SKU: "NEW-1", Qty: 0}}}
	wh := []WarehouseQty{
		{SKU: "X5S_8W", Warehouse: "Gudang Rungkut", City: "Surabaya", Qty: 20},
		{SKU: "X5S_8W", Warehouse: "Gudang Margomulyo", City: "Surabaya", Qty: 10},
		{SKU: "X5S_8W", Warehouse: "Semarang", City: "Semarang", Qty: 12},
	}
	products := []replyrule.Product{
		{SKU: "X5S_8W", Name: "Ezviz NVR 8Ch X5S 8W", ListPrice: 865000, Prices: map[string]int64{"A": 865000, "B": 900000}},
		{SKU: "NEW-1", Name: "Produk baru", ListPrice: 0, Prices: map[string]int64{"A": 120000}},
	}
	c := BuildCard(r, stock, wh, products, "b")
	if c.Tier != "B" || len(c.Items) != 2 {
		t.Fatalf("card: %+v", c)
	}
	l := c.Items[0]
	if l.Product != "Ezviz NVR 8Ch X5S 8W" || l.Price != 900000 || l.Asked != 3 || l.Stock != 42 || len(l.Cities) != 2 {
		t.Fatalf("line: %+v", l)
	}
	if l.Cities[0].City != "Surabaya" || l.Cities[0].Qty != 30 || len(l.Cities[0].Warehouses) != 2 || l.Cities[0].Warehouses[0].Name != "Gudang Rungkut" {
		t.Fatalf("surabaya: %+v", l.Cities[0])
	}
	if l.Cities[1].City != "Semarang" || l.Cities[1].Warehouses != nil { // one warehouse named as the city: not repeated
		t.Fatalf("semarang: %+v", l.Cities[1])
	}
	if n := c.Items[1]; n.Product != "Produk baru" || n.Price != 120000 || n.Stock != 0 || len(n.Cities) != 0 {
		t.Fatalf("no stock: %+v", n)
	}
	if c := BuildCard(r, stock, nil, products, ""); c.Items[0].Price != 865000 {
		t.Fatalf("list price: %+v", c.Items[0])
	}
}

// A turn with a photo goes out as OpenAI content parts; without one as plain text.
func TestMsgJSON(t *testing.T) {
	b, _ := json.Marshal(Msg{Role: "user", Content: "hi"})
	if string(b) != `{"role":"user","content":"hi"}` {
		t.Fatalf("plain: %s", b)
	}
	b, _ = json.Marshal(Msg{Role: "user", Content: "foto", Images: []string{"data:image/jpeg;base64,AAA"}})
	if !strings.Contains(string(b), `"type":"image_url"`) || !strings.Contains(string(b), `"url":"data:image/jpeg;base64,AAA"`) {
		t.Fatalf("parts: %s", b)
	}
}

func TestCaption(t *testing.T) {
	if Caption("[Foto] ready gak?") != "ready gak?" || Caption("[Foto]") != "" || Caption("stok ada") != "stok ada" {
		t.Fatal("caption")
	}
}

// Chat otomatis with price or stock turned off (ADR 0080): the quote leaves them out.
func TestQuoteTextOf(t *testing.T) {
	c := &Card{Items: []Line{{SKU: "NVR-32", Product: "NVR 32ch", Price: 2500000, Stock: 9, Cities: []City{{City: "Semarang", Qty: 9}}}}}
	full := QuoteText(c)
	if !strings.Contains(full, "Rp 2.500.000") || !strings.Contains(full, "Semarang 9") {
		t.Fatalf("full quote: %s", full)
	}
	noPrice := QuoteTextOf(c, true, false)
	if strings.Contains(noPrice, "Rp") || strings.Contains(noPrice, "2.500.000") || strings.Contains(noPrice, "DPP") || !strings.Contains(noPrice, "Semarang 9") {
		t.Fatalf("without price: %s", noPrice)
	}
	if noStock := QuoteTextOf(c, false, true); strings.Contains(noStock, "Stok") || !strings.Contains(noStock, "Rp 2.500.000") {
		t.Fatalf("without stock: %s", noStock)
	}
}
