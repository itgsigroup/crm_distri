// Package importer brings real data into Distri ARC from BigQuery or CSV: rows in a fixed column contract per entity
// are staged as delivered (import_rows), then transformed with the master-data mappings (data_mappings: branch,
// warehouse, category, sales, customer type) into sales profiles, dealers, contacts, orders, invoices, payments,
// stock, products and the signals the agents cite as provenance. A mapping change re-runs the transform from
// staging. Everything is idempotent (source_system 'import' + the source key).
package importer

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"distri-arc/internal/clock"
)

// Entity is one kind of imported row.
type Entity string

const (
	Sales        Entity = "sales"
	Customers    Entity = "customers"
	Invoices     Entity = "invoices"
	InvoiceLines Entity = "invoice_lines"
	Stock        Entity = "stock"
)

// Entities in transform order.
var Entities = []Entity{Sales, Customers, Invoices, InvoiceLines, Stock}

// Column is one field of the contract.
type Column struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Desc     string `json:"desc"`
}

// Contract is the column contract per entity: BigQuery queries must SELECT these names (… AS code), CSV files use
// them as the header. Extra columns are ignored.
var Contract = map[Entity][]Column{
	Sales: {
		{"code", true, "Kode / nama sales di sistem sumber (unik), mis. nama di Accurate"},
		{"name", true, "Nama tampil"},
		{"branch", false, "Cabang (dipetakan lewat mapping Cabang)"},
		{"wa_number", false, "Nomor WhatsApp sales (62…)"},
		{"email", false, "Email login"},
		{"title", false, "Jabatan, mis. Sales Distri"},
	},
	Customers: {
		{"code", true, "Kode pelanggan (unik)"},
		{"name", true, "Nama pelanggan / toko"},
		{"customer_type", false, "reseller (dealer) atau si (freelance / system integrator); nilai lain dipetakan"},
		{"branch", false, "Cabang"},
		{"city", false, "Kota"},
		{"sales", false, "Kode/nama sales pemegang (dipetakan lewat mapping Sales)"},
		{"phone", false, "Telepon / WhatsApp PIC (62… atau 08…)"},
		{"pic_name", false, "Nama PIC"},
		{"tier", false, "A, B atau C"},
		{"credit_limit", false, "Limit kredit (Rp)"},
		{"payment_terms_days", false, "Termin bayar (hari)"},
		{"segment", false, "Jenis usaha, mis. Toko CCTV & jaringan"},
	},
	Invoices: {
		{"number", true, "Nomor faktur (unik)"},
		{"customer_code", true, "Kode pelanggan"},
		{"date", true, "Tanggal faktur"},
		{"due_date", false, "Jatuh tempo (default: tanggal + termin pelanggan)"},
		{"total", true, "Total faktur (Rp)"},
		{"residual", false, "Sisa tagihan (Rp); 0 = lunas"},
		{"paid_date", false, "Tanggal lunas / bayar terakhir"},
		{"branch", false, "Cabang"},
		{"sales", false, "Sales"},
	},
	InvoiceLines: {
		{"invoice_number", true, "Nomor faktur"},
		{"product", true, "Nama barang"},
		{"sku", false, "Kode barang"},
		{"category", false, "Kategori sumber (dipetakan ke 6 kategori product mix)"},
		{"brand", false, "Merek"},
		{"qty", false, "Kuantitas"},
		{"price", false, "Harga satuan (Rp)"},
		{"amount", false, "Nilai baris (Rp); default qty × harga"},
		{"warehouse", false, "Gudang"},
	},
	Stock: {
		{"sku", true, "Kode barang"},
		{"product", true, "Nama barang"},
		{"category", false, "Kategori sumber"},
		{"warehouse", true, "Gudang (dipetakan ke cabang)"},
		{"qty", true, "Stok"},
		{"unit_cost", false, "HPP satuan (Rp)"},
		{"value", false, "Nilai stok (Rp); default qty × HPP"},
		{"age_days", false, "Umur stok tertua (hari)"},
		{"sold_90d", false, "Terjual 90 hari (unit)"},
	},
}

// Row is one source row: column → raw value.
type Row map[string]string

// Get is a trimmed value.
func (r Row) Get(k string) string { return strings.TrimSpace(r[k]) }

// Missing lists required columns without a value.
func Missing(e Entity, r Row) []string {
	var out []string
	for _, c := range Contract[e] {
		if c.Required && r.Get(c.Name) == "" {
			out = append(out, c.Name)
		}
	}
	return out
}

var numClean = regexp.MustCompile(`[^0-9,.\-]`)

// Number parses amounts as sources write them: "1.234.567,50" (id), "1,234,567.50" (en), "1234567.5", "Rp 12.000".
func Number(s string) float64 {
	s = numClean.ReplaceAllString(strings.TrimSpace(s), "")
	if s == "" {
		return 0
	}
	dots, commas := strings.Count(s, "."), strings.Count(s, ",")
	switch {
	case dots > 0 && commas > 0:
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") { // 1.234,5
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else { // 1,234.5
			s = strings.ReplaceAll(s, ",", "")
		}
	case commas > 1:
		s = strings.ReplaceAll(s, ",", "")
	case commas == 1:
		if i := strings.Index(s, ","); len(s)-i-1 == 3 { // 12,000 → thousands
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.Replace(s, ",", ".", 1)
		}
	case dots > 1:
		s = strings.ReplaceAll(s, ".", "")
	case dots == 1:
		if i := strings.Index(s, "."); len(s)-i-1 == 3 && !strings.HasPrefix(s, "0.") { // 12.000 → thousands (id)
			s = strings.ReplaceAll(s, ".", "")
		}
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// Rupiah is a whole-rupiah amount.
func Rupiah(s string) int64 { return int64(math.Round(Number(s))) }

var dateLayouts = []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05", "02/01/2006", "2/1/2006", "02-01-2006", "02/01/2006 15:04", "2006/01/02"}

// Date parses a calendar date (WIB); nil when empty or unreadable.
func Date(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 1e9 { // BigQuery TIMESTAMP as epoch seconds
		t := time.Unix(int64(f), 0).In(clock.WIB)
		d := clock.Today(t)
		return &d
	}
	for _, l := range dateLayouts {
		if t, err := time.ParseInLocation(l, s, clock.WIB); err == nil {
			d := clock.Today(t)
			return &d
		}
	}
	return nil
}

// CustomerType normalises the usual spellings; "" when the value needs a mapping.
func CustomerType(s string) string {
	switch v := strings.ToLower(strings.TrimSpace(s)); {
	case v == "":
		return "reseller"
	case v == "reseller" || v == "dealer" || strings.Contains(v, "reseller") || strings.Contains(v, "toko"):
		return "reseller"
	case v == "si" || strings.Contains(v, "integrator") || strings.Contains(v, "freelance") || strings.Contains(v, "installer") || strings.Contains(v, "teknisi"):
		return "si"
	}
	return ""
}

var slugClean = regexp.MustCompile(`[^a-z0-9]+`)

// Slug is a URL key from a name ("CV. Mitra Jaya Teknik" → "cv-mitra-jaya-teknik").
func Slug(s string) string {
	s = strings.Trim(slugClean.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "-")
	}
	if s == "" {
		s = "pelanggan"
	}
	return s
}
