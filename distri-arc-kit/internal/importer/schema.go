package importer

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Table is one BigQuery table with its columns (Pengaturan → Data & master → Jelajahi BigQuery).
type Table struct {
	Dataset  string    `json:"dataset"`
	Table    string    `json:"table"`
	Type     string    `json:"type"` // TABLE | VIEW | EXTERNAL …
	Rows     int64     `json:"rows"`
	Location string    `json:"location"`
	Columns  []TColumn `json:"columns"`
}

// TColumn is one column of a table.
type TColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Schema lists the datasets, tables and columns of the project (read-only REST: datasets.list, tables.list,
// tables.get). Large projects are capped at maxTables.
func (b *BigQuery) Schema(ctx context.Context, maxTables int) ([]Table, error) {
	project := b.Project
	if project == "" {
		project = b.SA.ProjectID
	}
	p := "/bigquery/v2/projects/" + url.PathEscape(project)
	var ds struct {
		Datasets []struct {
			Ref struct {
				DatasetID string `json:"datasetId"`
			} `json:"datasetReference"`
			Location string `json:"location"`
		} `json:"datasets"`
	}
	if err := b.call(ctx, http.MethodGet, p+"/datasets?all=false&maxResults=1000", nil, &ds); err != nil {
		return nil, err
	}
	var out []Table
	for _, d := range ds.Datasets {
		var tl struct {
			Tables []struct {
				Ref struct {
					TableID string `json:"tableId"`
				} `json:"tableReference"`
				Type string `json:"type"`
			} `json:"tables"`
		}
		dp := p + "/datasets/" + url.PathEscape(d.Ref.DatasetID)
		if err := b.call(ctx, http.MethodGet, dp+"/tables?maxResults=1000", nil, &tl); err != nil {
			return nil, err
		}
		for _, t := range tl.Tables {
			if len(out) >= maxTables {
				break
			}
			var meta struct {
				NumRows  string `json:"numRows"`
				Location string `json:"location"`
				Schema   struct {
					Fields []struct {
						Name string `json:"name"`
						Type string `json:"type"`
					} `json:"fields"`
				} `json:"schema"`
			}
			if err := b.call(ctx, http.MethodGet, dp+"/tables/"+url.PathEscape(t.Ref.TableID), nil, &meta); err != nil {
				return nil, err
			}
			n, _ := strconv.ParseInt(meta.NumRows, 10, 64)
			tb := Table{Dataset: d.Ref.DatasetID, Table: t.Ref.TableID, Type: t.Type, Rows: n, Location: d.Location}
			for _, f := range meta.Schema.Fields {
				tb.Columns = append(tb.Columns, TColumn{Name: f.Name, Type: f.Type})
			}
			out = append(out, tb)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dataset+"."+out[i].Table < out[j].Dataset+"."+out[j].Table })
	return out, nil
}

// synonyms are the column names Accurate exports (and common Indonesian / English names) per contract column,
// best first. Used to pre-fill the column mapper; a person confirms.
var synonyms = map[Entity]map[string][]string{
	Sales: {
		"code":      {"kode_sales", "sales_code", "salesman_code", "nama_sales", "sales_user", "salesman", "sales", "nama"},
		"name":      {"nama_sales", "salesman_name", "sales_name", "nama", "name", "sales_user", "salesman"},
		"branch":    {"cabang", "branch", "lokasi", "kantor"},
		"wa_number": {"no_hp", "hp", "telepon", "phone", "whatsapp", "no_wa"},
		"email":     {"email", "surel"},
		"title":     {"jabatan", "title", "posisi"},
	},
	Customers: {
		"code":               {"kode_pelanggan", "customer_no", "customer_code", "kode_customer", "id_pelanggan", "customer_id", "no_pelanggan", "nama_customer", "nama_pelanggan"},
		"name":               {"nama_pelanggan", "nama_customer", "customer_name", "nama", "name"},
		"customer_type":      {"tipe_pelanggan", "jenis_pelanggan", "kategori_pelanggan", "customer_type", "customer_category", "tipe_customer", "kategori"},
		"branch":             {"cabang", "branch", "kantor"},
		"city":               {"kota", "city", "kabupaten"},
		"sales":              {"nama_sales", "sales_user", "salesman", "sales"},
		"phone":              {"telepon_customer", "telepon", "no_hp", "hp", "phone", "mobile", "whatsapp", "telepon_pic"},
		"pic_name":           {"nama_pic", "pic", "contact_name", "kontak"},
		"tier":               {"tier", "level", "grade"},
		"credit_limit":       {"limit_kredit", "credit_limit", "plafon", "batas_kredit"},
		"payment_terms_days": {"termin", "term_days", "payment_terms", "top", "jangka_waktu"},
		"segment":            {"segment", "segmen", "jenis_usaha", "bidang_usaha"},
	},
	Invoices: {
		"number":        {"nomor_faktur", "no_faktur", "invoice_no", "number", "nomor", "no_invoice"},
		"customer_code": {"kode_pelanggan", "customer_no", "customer_code", "kode_customer", "customer_id", "nama_customer", "nama_pelanggan"},
		"date":          {"tanggal_faktur", "tgl_faktur", "trans_date", "invoice_date", "tanggal", "date"},
		"due_date":      {"tanggal_jatuh_tempo", "jatuh_tempo", "due_date", "tgl_jatuh_tempo"},
		"total":         {"total_faktur", "total_amount", "total", "grand_total", "nilai_faktur"},
		"residual":      {"sisa_tagihan", "outstanding", "residual", "sisa", "piutang", "balance"},
		"paid_date":     {"tanggal_bayar", "tgl_bayar", "paid_date", "tanggal_lunas", "payment_date"},
		"branch":        {"cabang", "branch"},
		"sales":         {"sales_user", "nama_sales", "salesman", "sales"},
	},
	InvoiceLines: {
		"invoice_number": {"nomor_faktur", "no_faktur", "invoice_no", "nomor"},
		"product":        {"nama_item", "nama_barang", "item_name", "product_name", "nama_produk", "deskripsi"},
		"sku":            {"kode_item", "kode_barang", "item_no", "sku", "item_code", "kode_produk"},
		"category":       {"kategori", "kategori_item", "item_category", "category", "nama_kategori"},
		"brand":          {"merek", "brand", "merk"},
		"qty":            {"kuantitas", "qty", "quantity", "jumlah"},
		"price":          {"harga_satuan", "unit_price", "price", "harga"},
		"amount":         {"nilai_penjualan", "total_harga", "amount", "subtotal", "total"},
		"warehouse":      {"gudang", "warehouse", "lokasi_gudang"},
	},
	Stock: {
		"sku":       {"kode_item", "kode_barang", "item_no", "sku", "item_code"},
		"product":   {"nama_item", "nama_barang", "item_name", "product_name"},
		"category":  {"kategori", "item_category", "category", "nama_kategori"},
		"warehouse": {"gudang", "warehouse", "lokasi_gudang"},
		"qty":       {"kuantitas", "qty", "quantity", "stok", "jumlah", "on_hand"},
		"unit_cost": {"harga_pokok", "hpp", "unit_cost", "cost", "harga_satuan"},
		"value":     {"nilai_stok", "value", "stock_value", "nilai"},
		"age_days":  {"umur_hari", "umur", "age_days", "aging"},
		"sold_90d":  {"terjual_90_hari", "sold_90d", "terjual_90"},
	},
}

// tableHints are words in table names that suggest an entity.
var tableHints = map[Entity][]string{
	Sales:        {"sales", "salesman", "pengguna", "karyawan", "employee"},
	Customers:    {"pelanggan", "customer", "kontak", "partner"},
	Invoices:     {"faktur", "invoice", "penjualan", "sales_invoice"},
	InvoiceLines: {"faktur_item", "invoice_item", "invoice_detail", "detail", "item_faktur", "penjualan_item"},
	Stock:        {"stok", "stock", "persediaan", "inventory", "gudang"},
}

// Suggestion is a proposed table and column mapping for one entity.
type Suggestion struct {
	Entity  Entity            `json:"entity"`
	Table   string            `json:"table"`   // dataset.table
	Columns map[string]string `json:"columns"` // contract column → source column
	Missing []string          `json:"missing"` // required contract columns without a source column
	SQL     string            `json:"sql"`
	// Candidates are other tables that fit, best first, each with its suggested column mapping.
	Candidates []Candidate `json:"candidates"`
}

// Candidate is one table that may hold an entity.
type Candidate struct {
	Table   string            `json:"table"`
	Score   int               `json:"score"`
	Columns map[string]string `json:"columns"`
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func scoreTable(e Entity, t Table) int {
	name := norm(t.Table)
	s := 0
	for i, h := range tableHints[e] {
		if strings.Contains(name, h) {
			s += 10 - i
		}
	}
	if e == Invoices && (strings.Contains(name, "item") || strings.Contains(name, "detail")) {
		s -= 15 // the header table, not the lines
	}
	cols := map[string]bool{}
	for _, c := range t.Columns {
		cols[norm(c.Name)] = true
	}
	for _, c := range Contract[e] {
		for _, syn := range synonyms[e][c.Name] {
			if cols[syn] {
				if c.Required {
					s += 4
				} else {
					s++
				}
				break
			}
		}
	}
	return s
}

// SuggestColumns maps contract columns to a table's columns by name.
func SuggestColumns(e Entity, t Table) (map[string]string, []string) {
	cols := map[string]string{}
	for _, c := range t.Columns {
		cols[norm(c.Name)] = c.Name
	}
	out := map[string]string{}
	var missing []string
	used := map[string]bool{}
	for _, c := range Contract[e] {
		for _, syn := range synonyms[e][c.Name] {
			src, ok := cols[syn]
			// a source column feeds one contract column, except code and name (a sales name is both)
			if ok && (!used[src] || c.Name == "name" || c.Name == "code") {
				out[c.Name] = src
				used[src] = true
				break
			}
		}
		if _, ok := out[c.Name]; !ok && c.Required {
			missing = append(missing, c.Name)
		}
	}
	return out, missing
}

// BuildSQL writes the import query for a table and column mapping (contract column → source column or
// expression). Customers taken from an invoice table are made distinct.
func BuildSQL(project string, e Entity, table string, cols map[string]string, where string) string {
	var sel []string
	for _, c := range Contract[e] {
		src, ok := cols[c.Name]
		if !ok || strings.TrimSpace(src) == "" {
			continue
		}
		expr := src
		if isIdent(src) {
			expr = "`" + src + "`"
		}
		switch c.Name {
		case "code", "customer_code", "invoice_number", "number", "sku":
			expr = "CAST(" + expr + " AS STRING)"
		}
		sel = append(sel, expr+" AS "+c.Name)
	}
	distinct := ""
	if e == Customers || e == Sales {
		distinct = "DISTINCT "
	}
	q := "SELECT " + distinct + strings.Join(sel, ",\n       ") + "\nFROM `" + project + "." + table + "`"
	if w := strings.TrimSpace(where); w != "" {
		q += "\nWHERE " + w
	}
	return q
}

func isAlnum(r rune) bool { return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' }

func isIdent(s string) bool {
	for _, r := range s {
		if !isAlnum(r) && (r < 'A' || r > 'Z') && r != '_' {
			return false
		}
	}
	return s != ""
}

// Suggest proposes, per entity, the best-matching table with its column mapping and SQL, plus the other
// candidate tables.
func Suggest(project string, tables []Table) []Suggestion {
	var out []Suggestion
	for _, e := range Entities {
		var cands []Candidate
		for _, t := range tables {
			if s := scoreTable(e, t); s > 0 {
				cols, _ := SuggestColumns(e, t)
				cands = append(cands, Candidate{Table: t.Dataset + "." + t.Table, Score: s, Columns: cols})
			}
		}
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
		if len(cands) > 8 {
			cands = cands[:8]
		}
		sg := Suggestion{Entity: e, Columns: map[string]string{}, Missing: required(e), Candidates: cands}
		if len(cands) > 0 {
			best := cands[0]
			sg.Table, sg.Columns, sg.Missing = best.Table, best.Columns, missingOf(e, best.Columns)
			sg.SQL = BuildSQL(project, e, best.Table, best.Columns, "")
		} else {
			sg.Candidates = []Candidate{}
		}
		out = append(out, sg)
	}
	return out
}

func missingOf(e Entity, cols map[string]string) []string {
	out := []string{}
	for _, c := range Contract[e] {
		if _, ok := cols[c.Name]; c.Required && !ok {
			out = append(out, c.Name)
		}
	}
	return out
}

func required(e Entity) []string {
	var out []string
	for _, c := range Contract[e] {
		if c.Required {
			out = append(out, c.Name)
		}
	}
	return out
}

// categoryRules map words in a source category or product name to the 6 KAT, checked in order (strong words
// first, brands last). The result is only a suggestion shown in Pengaturan; a person confirms it.
var categoryRules = []struct {
	kat   string
	words []string
}{
	{"Kabel & PoE", []string{"kabel", "cable", "utp", "cat5", "cat6", "coaxial", "rg59", "rg6", "fiber", "rj45"}},
	{"HDD & storage", []string{"harddisk", "hard disk", "hdd", "ssd", "storage", "memory", "micro sd", "microsd", "sd card", "hiksemi", "skyhawk", "flashdisk"}},
	{"Kamera & NVR", []string{"kamera", "camera", "cctv", "cam", "ipc", "nvr", "dvr", "xvr", "ptz", "dome", "bullet", "turret", "ip cam"}},
	{"Kabel & PoE", []string{"poe", "switch", "network", "jaringan", "router", "access point", "konektor", "connector"}},
	{"Fire alarm", []string{"fire", "smoke", "alarm", "detector", "detektor", "sprinkler", "mcfa", "kebakaran", "hydrant", "apar"}},
	{"Modul LED", []string{"led", "videotron", "modul", "module", "running text", "p10", "p5", "p3", "signage", "novastar"}},
	{"Kamera & NVR", []string{"ezviz", "hilook", "hikvision", "dahua", "imou", "tiandy"}},
}

// SuggestCategory proposes one of the 6 KAT for a source category; "" when nothing matches (falls to a person).
func SuggestCategory(raw string) string {
	v := " " + strings.Join(strings.FieldsFunc(norm(raw), func(r rune) bool {
		return !isAlnum(r)
	}), " ") + " "
	if strings.TrimSpace(v) == "" {
		return ""
	}
	for _, r := range categoryRules {
		for _, w := range r.words {
			// short words must match whole; longer ones may be part of a word (harddisk, kabelutp)
			if len(w) <= 3 && strings.Contains(v, " "+w+" ") || len(w) > 3 && strings.Contains(v, w) {
				return r.kat
			}
		}
	}
	for _, w := range []string{"aksesoris", "accessories", "accessory", "bracket", "adaptor", "adapter", "power supply", "box", "jack", "bnc", "balun", "ups", "mounting"} {
		if strings.Contains(v, w) {
			return "Aksesoris"
		}
	}
	return ""
}

// SuggestTarget proposes a target for an unmapped master value (category → KAT, ctype → reseller|si).
func SuggestTarget(kind, raw string) string {
	switch kind {
	case "category":
		return SuggestCategory(raw)
	case "ctype":
		return CustomerType(raw)
	}
	return ""
}
