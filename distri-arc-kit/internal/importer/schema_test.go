package importer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuggestCategory(t *testing.T) {
	for in, want := range map[string]string{
		"IP Camera":                 "Kamera & NVR",
		"KAMERA ANALOG":             "Kamera & NVR",
		"NVR / DVR":                 "Kamera & NVR",
		"Hikvision":                 "Kamera & NVR",
		"Kamera Bullet LED":         "Kamera & NVR",
		"Harddisk":                  "HDD & storage",
		"HDD Hikvision":             "HDD & storage",
		"Micro SD Card":             "HDD & storage",
		"Kabel UTP Cat6":            "Kabel & PoE",
		"Kabel CCTV RG59":           "Kabel & PoE",
		"Switch PoE Hikvision":      "Kabel & PoE",
		"Fire Alarm Conventional":   "Fire alarm",
		"Smoke Detector":            "Fire alarm",
		"Modul LED P10":             "Modul LED",
		"Videotron":                 "Modul LED",
		"Bracket":                   "Aksesoris",
		"Power Supply / Adaptor":    "Aksesoris",
		"Lain-lain":                 "",
		"":                          "",
		"Camcorder bag (Accessory)": "Aksesoris",
	} {
		if got := SuggestCategory(in); got != want {
			t.Errorf("SuggestCategory(%q) = %q, want %q", in, got, want)
		}
	}
	if SuggestTarget("ctype", "System Integrator") != "si" || SuggestTarget("ctype", "Dealer") != "reseller" || SuggestTarget("branch", "Medan") != "" {
		t.Fatal("SuggestTarget")
	}
}

var accurateTables = []Table{
	{Dataset: "accurate", Table: "pelanggan", Columns: []TColumn{{"kode_pelanggan", "STRING"}, {"nama_pelanggan", "STRING"}, {"kota", "STRING"}, {"kategori_pelanggan", "STRING"}, {"limit_kredit", "NUMERIC"}, {"termin", "INTEGER"}, {"telepon", "STRING"}}},
	{Dataset: "accurate", Table: "faktur_penjualan", Columns: []TColumn{{"nomor_faktur", "STRING"}, {"kode_pelanggan", "STRING"}, {"tanggal_faktur", "DATE"}, {"jatuh_tempo", "DATE"}, {"total_faktur", "NUMERIC"}, {"sisa_tagihan", "NUMERIC"}, {"cabang", "STRING"}, {"nama_sales", "STRING"}}},
	{Dataset: "accurate", Table: "faktur_penjualan_detail", Columns: []TColumn{{"nomor_faktur", "STRING"}, {"kode_item", "STRING"}, {"nama_item", "STRING"}, {"kategori", "STRING"}, {"kuantitas", "NUMERIC"}, {"harga_satuan", "NUMERIC"}, {"hpp", "NUMERIC"}, {"gudang", "STRING"}}},
	{Dataset: "accurate", Table: "stok_gudang", Columns: []TColumn{{"kode_item", "STRING"}, {"nama_item", "STRING"}, {"gudang", "STRING"}, {"kuantitas", "NUMERIC"}, {"harga_pokok", "NUMERIC"}, {"tanggal_masuk", "DATE"}}},
	{Dataset: "accurate", Table: "salesman", Columns: []TColumn{{"nama_sales", "STRING"}, {"cabang", "STRING"}, {"no_hp", "STRING"}}},
}

func TestSuggestTables(t *testing.T) {
	got := map[Entity]Suggestion{}
	for _, s := range Suggest("gsi-data", accurateTables) {
		got[s.Entity] = s
	}
	want := map[Entity]string{Sales: "accurate.salesman", Customers: "accurate.pelanggan", Invoices: "accurate.faktur_penjualan", InvoiceLines: "accurate.faktur_penjualan_detail", Stock: "accurate.stok_gudang"}
	for e, tbl := range want {
		if got[e].Table != tbl {
			t.Errorf("%s: table %q, want %q", e, got[e].Table, tbl)
		}
		if len(got[e].Missing) != 0 {
			t.Errorf("%s: missing %v", e, got[e].Missing)
		}
	}
	if c := got[Stock].Columns; c["received_date"] != "tanggal_masuk" || c["unit_cost"] != "harga_pokok" {
		t.Fatalf("stock columns %v", c)
	}
	if c := got[InvoiceLines].Columns; c["cost"] != "hpp" {
		t.Fatalf("line columns %v", c)
	}
	inv := got[Invoices]
	if inv.Columns["number"] != "nomor_faktur" || inv.Columns["residual"] != "sisa_tagihan" || inv.Columns["sales"] != "nama_sales" {
		t.Fatalf("invoice columns %v", inv.Columns)
	}
	if got[Customers].Columns["customer_type"] != "kategori_pelanggan" || got[Sales].Columns["code"] != "nama_sales" || got[Sales].Columns["name"] != "nama_sales" {
		t.Fatalf("customers %v sales %v", got[Customers].Columns, got[Sales].Columns)
	}
	if !strings.Contains(inv.SQL, "CAST(`nomor_faktur` AS STRING) AS number") || !strings.Contains(inv.SQL, "FROM `gsi-data.accurate.faktur_penjualan`") {
		t.Fatalf("sql %s", inv.SQL)
	}
	if !strings.HasPrefix(got[Customers].SQL, "SELECT DISTINCT ") {
		t.Fatalf("customers sql %s", got[Customers].SQL)
	}
	// expressions pass through untouched
	q := BuildSQL("p", Stock, "d.t", map[string]string{"sku": "kode_item", "product": "nama_item", "warehouse": "gudang", "qty": "SUM(qty)"}, "aktif = true")
	if !strings.Contains(q, "SUM(qty) AS qty") || !strings.HasSuffix(q, "WHERE aktif = true") {
		t.Fatalf("sql %s", q)
	}
}

func TestBigQuerySchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, "/projects/gsi-data/datasets"):
			_, _ = w.Write([]byte(`{"datasets":[{"datasetReference":{"datasetId":"accurate"},"location":"asia-southeast2"}]}`))
		case strings.HasSuffix(r.URL.Path, "/datasets/accurate/tables"):
			_, _ = w.Write([]byte(`{"tables":[{"tableReference":{"tableId":"pelanggan"},"type":"TABLE"},{"tableReference":{"tableId":"faktur"},"type":"VIEW"}]}`))
		case strings.HasSuffix(r.URL.Path, "/tables/pelanggan"):
			_, _ = w.Write([]byte(`{"numRows":"1234","schema":{"fields":[{"name":"kode_pelanggan","type":"STRING"}]}}`))
		case strings.HasSuffix(r.URL.Path, "/tables/faktur"):
			_, _ = w.Write([]byte(`{"schema":{"fields":[{"name":"nomor_faktur","type":"STRING"},{"name":"total","type":"NUMERIC"}]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	sa, err := ParseServiceAccount(testKey(t, srv.URL+"/token"))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := (&BigQuery{SA: sa, BaseURL: srv.URL}).Schema(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 || tables[0].Table != "faktur" || tables[0].Type != "VIEW" || len(tables[0].Columns) != 2 || tables[1].Rows != 1234 || tables[1].Location != "asia-southeast2" {
		t.Fatalf("tables %+v", tables)
	}
}
