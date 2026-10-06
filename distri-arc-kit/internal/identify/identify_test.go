package identify_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/identify"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
	"distri-arc/internal/wa"
)

var now = clock.Fixed(time.Date(2026, 10, 5, 12, 0, 0, 0, clock.WIB))

func TestScore(t *testing.T) {
	two := []identify.Source{
		{Source: "getcontact", OK: "true", Name: "Mandiri Elektronik Pati"},
		{Source: "wa_business", OK: "true", Name: "Toko Mandiri – CCTV & Sound"},
		{Source: "odoo", OK: "false"},
	}
	if s := identify.Score(two); s != 74 {
		t.Fatalf("two agreeing sources: %d, want 74 (sample Toko Mandiri)", s)
	}
	if s := identify.Score([]identify.Source{{Source: "getcontact", OK: "true", Name: "Budi"}, {Source: "truecaller", OK: "true", Name: "Toko Sejahtera"}}); s != 60 {
		t.Fatalf("disagreeing: %d", s)
	}
	if s := identify.Score([]identify.Source{{Source: "odoo", OK: "true", Name: "CV Bina"}}); s != 100 {
		t.Fatalf("odoo: %d", s)
	}
}

// CSV import + identification of the sample new number; non-inbound numbers are refused (privacy).
func TestIdentifyAndImport(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	csv := "nomor,nama,tag\n+62 822-1234-3310,Mandiri Elektronik Pati,2\n0812-xx,Salah,1\n6281111111111,Toko Lain,3\n"
	res, err := identify.ImportGetcontact(ctx, st.Q, strings.NewReader(csv), nil, now.Now())
	if err != nil || res.Imported != 2 || len(res.Skipped) != 1 {
		t.Fatalf("import %+v %v", res, err)
	}
	fake := wa.NewFake()
	fake.SetProfile("6282212343310", wa.Profile{Name: "Toko Mandiri – CCTV & Sound", Category: "Electronics", Business: true})
	svc := &identify.Service{St: st, Clock: now, Profiles: fake, Truecaller: identify.FakeTruecaller{}}
	r, err := svc.Identify(ctx, "6282212343310")
	if err != nil {
		t.Fatal(err)
	}
	if r.Score != 74 || r.BestName != "Toko Mandiri Elektronik" || len(r.Sources) != 3 {
		t.Fatalf("identification %+v", r)
	}
	var thread string
	if err := st.Pool.QueryRow(ctx, "select identification->>'score' from chat_threads where kind = 'new'").Scan(&thread); err != nil || thread != "74" {
		t.Fatalf("thread context %q %v", thread, err)
	}
	if _, err := svc.Identify(ctx, "6281111111111"); !errors.Is(err, identify.ErrNotInbound) {
		t.Fatalf("non-inbound number identified: %v", err)
	}
}
