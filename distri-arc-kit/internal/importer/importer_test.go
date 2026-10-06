package importer_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/importer"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/testdb"
)

var now = clock.Fixed(time.Date(2026, 10, 6, 9, 0, 0, 0, clock.WIB))

func newImporter(t *testing.T) (importer.Importer, *store.Store) {
	t.Helper()
	st := testdb.New(t) // empty database: real data only
	return importer.Importer{St: st, Clock: now, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, st
}

func parse(t *testing.T, s string) []importer.Row {
	t.Helper()
	rows, err := importer.ParseCSV(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func count(t *testing.T, st *store.Store, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func TestNumbersDatesTypes(t *testing.T) {
	for in, want := range map[string]float64{"1.234.567,50": 1234567.5, "1,234,567.50": 1234567.5, "Rp 12.000": 12000, "12,000": 12000, "1234567.5": 1234567.5, "0.5": 0.5, "": 0, "-2.500": -2500} {
		if got := importer.Number(in); got != want {
			t.Errorf("Number(%q) = %v, want %v", in, got, want)
		}
	}
	for in, want := range map[string]string{"2026-10-05": "2026-10-05", "05/10/2026": "2026-10-05", "2026-10-05T14:02:00+07:00": "2026-10-05", "1791262800": "2026-10-06"} {
		if d := importer.Date(in); d == nil || d.Format("2006-01-02") != want {
			t.Errorf("Date(%q) = %v, want %s", in, d, want)
		}
	}
	for in, want := range map[string]string{"Reseller": "reseller", "DEALER": "reseller", "System Integrator": "si", "freelance": "si", "SI": "si", "Proyek": ""} {
		if got := importer.CustomerType(in); got != want {
			t.Errorf("CustomerType(%q) = %q, want %q", in, got, want)
		}
	}
}

// Accurate-like rows: two customer types, invoices with lines, a paid and an open invoice, stock in two warehouses
// of one branch. Unmapped values are listed; mapping them and re-applying fixes branch and product mix.
func TestImportAndMapping(t *testing.T) {
	im, st := newImporter(t)
	ctx := context.Background()
	stage := func(e importer.Entity, csv string) importer.StageReport {
		rep, err := im.Stage(ctx, e, parse(t, csv), "csv", "test")
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	stage(importer.Sales, "code,name,branch,wa_number,title\nANDI W,Andi Wibowo,Kantor Pusat,0812-3450-4471,Sales Distri\n")
	stage(importer.Customers, `code;name;customer_type;branch;city;sales;phone;pic_name;tier;credit_limit
C001;Toko Sinar Elektronik;Reseller;Kantor Pusat;Semarang;ANDI W;0812-1111-2222;Mbak Rina;A;250.000.000
C002;Budi Teknik (SI);System Integrator;Kantor Pusat;Kudus;ANDI W;;;;
`)
	rep := stage(importer.Invoices, `number,customer_code,date,due_date,total,residual,paid_date
INV/001,C001,2026-08-01,2026-08-31,"12.000.000",0,2026-08-25
INV/002,C001,2026-09-01,2026-10-01,"8.500.000","8.500.000",
INV/003,C002,2026-09-10,,"3.000.000",0,2026-09-12
INV/004,C999,2026-09-10,,"1.000.000",0,
INV/005,,2026-09-10,,1,0,
`)
	if rep.Skipped != 1 || rep.Staged != 4 {
		t.Fatalf("stage invoices %+v", rep)
	}
	stage(importer.InvoiceLines, `invoice_number,sku,product,category,qty,price
INV/001,CAM-4,Kamera IP 4MP,Hikvision Camera IP,10,"1.000.000"
INV/001,NVR-16,NVR 16ch,Hikvision NVR,1,"2.000.000"
INV/002,HDD-4,HDD 4TB,Harddisk,5,"1.700.000"
INV/003,CBL,Kabel UTP,Hikvision Kabel,10,"300.000"
`)
	stage(importer.Stock, `sku,product,category,warehouse,qty,unit_cost,age_days,sold_90d
CAM-4,Kamera IP 4MP,Hikvision Camera IP,01. LAMPER,30,"800.000",40,26
CAM-4,Kamera IP 4MP,Hikvision Camera IP,Lamper Baru,10,"800.000",120,0
HDD-4,HDD 4TB,Harddisk,04. Jakarta,8,"1.400.000",20,13
`)
	ar, err := im.Apply(ctx, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	if ar.Dealers != 2 || ar.Invoices != 3 || ar.Sales != 1 || ar.Skipped != 1 || ar.Unmapped["branch"] != 1 || ar.Unmapped["category"] != 4 || ar.Unmapped["warehouse"] != 3 {
		t.Fatalf("first apply %+v", ar)
	}
	var ctype, branch, owner string
	var limit int64
	_ = st.Pool.QueryRow(ctx, "select d.customer_type, d.branch, s.name, d.credit_limit from dealers d join sales_users s on s.id = d.owner_id where d.name = 'Toko Sinar Elektronik'").Scan(&ctype, &branch, &owner, &limit)
	if ctype != "reseller" || branch != "Kantor Pusat" || owner != "Andi Wibowo" || limit != 250_000_000 {
		t.Fatalf("dealer %s %s %s %d", ctype, branch, owner, limit)
	}
	if count(t, st, "select count(*) from dealers where customer_type = 'si'") != 1 || count(t, st, "select count(*) from contacts where wa_number = '6281211112222'") != 1 {
		t.Fatal("si customer / PIC contact")
	}
	if count(t, st, "select count(*) from invoices where state = 'paid'") != 2 || count(t, st, "select count(*) from payments") != 2 {
		t.Fatal("invoices / payments")
	}
	if count(t, st, "select count(*) from signals where dedupe_key like 'import:%'") == 0 {
		t.Fatal("no signals for provenance")
	}

	// map: Kantor Pusat → Semarang, categories → KAT, warehouses → branches
	for _, m := range [][3]string{{"branch", "Kantor Pusat", "Semarang"}, {"category", "Hikvision Camera IP", "Kamera & NVR"}, {"category", "Hikvision NVR", "Kamera & NVR"},
		{"category", "Harddisk", "HDD & storage"}, {"category", "Hikvision Kabel", "Kabel & PoE"}, {"warehouse", "01. LAMPER", "Semarang"}, {"warehouse", "Lamper Baru", "Semarang"}, {"warehouse", "04. Jakarta", "Jakarta"}} {
		tgt := m[2]
		if err := st.Q.SetMapping(ctx, gen.SetMappingParams{Kind: m[0], SourceValue: m[1], Target: &tgt}); err != nil {
			t.Fatal(err)
		}
	}
	// a person sets the tier and type by hand: the next import keeps it
	if _, err := st.Pool.Exec(ctx, "update dealers set customer_type = 'si', master_locked = '{customer_type}' where name = 'Toko Sinar Elektronik'"); err != nil {
		t.Fatal(err)
	}
	ar, err = im.Apply(ctx, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	if ar.Unmapped["branch"]+ar.Unmapped["category"]+ar.Unmapped["warehouse"] != 0 {
		t.Fatalf("still unmapped %+v", ar.Unmapped)
	}
	_ = st.Pool.QueryRow(ctx, "select customer_type, branch from dealers where name = 'Toko Sinar Elektronik'").Scan(&ctype, &branch)
	if ctype != "si" || branch != "Semarang" {
		t.Fatalf("after mapping: %s %s", ctype, branch)
	}
	var qty, mix int
	_ = st.Pool.QueryRow(ctx, "select qty from stock_items where sku = 'CAM-4' and branch = 'Semarang'").Scan(&qty)
	_ = st.Pool.QueryRow(ctx, "select (metrics_current->>'mix')::int from dealers where name = 'Toko Sinar Elektronik'").Scan(&mix)
	if qty != 40 || mix != 2 {
		t.Fatalf("stock %d (two warehouses of Semarang), mix %d", qty, mix)
	}
	if count(t, st, "select count(*) from orders") != 3 || count(t, st, "select count(*) from products where category = 'Kamera & NVR'") != 2 {
		t.Fatal("idempotent re-apply / products")
	}
	// incremental apply touches nothing new
	if ar, err = im.Apply(ctx, false, "test"); err != nil || ar.Invoices != 0 {
		t.Fatalf("incremental %+v %v", ar, err)
	}
}
