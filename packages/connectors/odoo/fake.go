package odoo

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Fake is an in-memory Odoo with three [NEW] companies, real crm.stage names and
// a dataset mirroring the ARC fixture (8 opportunities: 6 match exactly, 2 differ
// enough to need a human-approved link). It implements Reader and Writer.
type Fake struct {
	mu     sync.Mutex
	data   map[string][]Record
	Writes []WriteOp
	nextID int64
	// ConflictOn forces a write_date conflict for model#id (tests).
	ConflictOn map[string]bool
}

// NewFake builds the dataset.
func NewFake() *Fake {
	f := &Fake{data: map[string][]Record{}, nextID: 9000, ConflictOn: map[string]bool{}}
	wd := "2026-09-28 13:31:00"
	f.data["res.company"] = []Record{
		{"id": int64(11), "name": "[NEW] GSI Semarang", "write_date": wd},
		{"id": int64(12), "name": "[NEW] GSI Yogyakarta", "write_date": wd},
		{"id": int64(13), "name": "[NEW] GSI Surabaya", "write_date": wd},
		{"id": int64(3), "name": "GSI Semarang (arsip)", "write_date": wd},
	}
	f.data["crm.stage"] = []Record{
		{"id": int64(1), "name": "Baru", "sequence": int64(1), "is_won": false, "write_date": wd},
		{"id": int64(2), "name": "Berkualifikasi", "sequence": int64(2), "is_won": false, "write_date": wd},
		{"id": int64(3), "name": "Penawaran", "sequence": int64(3), "is_won": false, "write_date": wd},
		{"id": int64(4), "name": "Won", "sequence": int64(4), "is_won": true, "write_date": wd},
	}
	partners := []struct {
		id      int64
		name    string
		phone   string
		company int64
	}{
		{101, "RSUD Kota Yogyakarta", "+62 274 515 865", 12}, {102, "PT Nusantara Baja Tuban", "+62 356 711 200", 13},
		{103, "Diskominfo Kota Semarang", "+62 24 354 9446", 11}, {104, "Bank Sejahtera Daerah", "+62 274 561 000", 12},
		{105, "Pemerintah Kabupaten Sleman", "+62 274 868 405", 12}, {106, "PT Pelabuhan Jaya Surabaya", "+62 31 329 1092", 13},
		{107, "Universitas Merdeka Semarang", "+62 24 850 1111", 11}, {108, "Hotel Amarta Semarang", "+62 24 845 0000", 11},
		{109, "PT Cakra Logistik Semarang", "+62 24 658 2200", 11}, {110, "Pemkot Salatiga", "+62 298 326 767", 11},
		{111, "RS Panti Rapih", "+62 274 514 845", 12}, {112, "Dinas PU Kab. Magelang", "+62 293 788 181", 12},
		{113, "PT Semen Mitra Tuban", "+62 356 322 100", 13},
	}
	for _, p := range partners {
		f.data["res.partner"] = append(f.data["res.partner"], Record{"id": p.id, "name": p.name, "is_company": true, "phone": p.phone, "company_id": []any{p.company, ""}, "write_date": wd})
	}
	leads := []struct {
		id       int64
		name     string
		partner  int64
		rev      float64
		prob     float64
		stage    int64
		user     string
		deadline string
		company  int64
	}{
		{201, "CCTV 240 titik + VMS", 101, 2.4e9, 70, 3, "Dewi", "2026-10-15", 12},
		{202, "Command center videowall 3×3", 102, 3.1e9, 40, 2, "Rizky", "2026-11-28", 13},
		{203, "Videotron Simpang Lima 6×4 m", 103, 1.85e9, 60, 3, "Andi", "2026-10-10", 11},
		{204, "Fire alarm addressable 4 gedung", 104, 0.86e9, 90, 3, "Dewi", "2026-09-30", 12},
		{205, "Dashboard ruang pimpinan Bupati", 105, 1.02e9, 70, 3, "Dewi", "2026-11-05", 12},
		{206, "CCTV pelabuhan 90 titik", 106, 1.6e9, 75, 3, "Rizky", "2026-10-08", 13},
		{207, "LED auditorium kampus", 107, 0.62e9, 30, 2, "Andi", "2026-12-20", 11},
		{208, "Digital signage lobby & ballroom", 108, 0.32e9, 85, 3, "Andi", "2026-10-03", 11},
		{209, "CCTV gudang 60 titik", 109, 1.9e9, 100, 4, "Andi", "2026-09-15", 11},
		{210, "Videotron alun-alun 4×3 m", 110, 1.2e9, 100, 4, "Andi", "2026-09-02", 11},
	}
	for _, l := range leads {
		f.data["crm.lead"] = append(f.data["crm.lead"], Record{"id": l.id, "name": l.name, "type": "opportunity", "partner_id": []any{l.partner, ""},
			"expected_revenue": l.rev, "probability": l.prob, "stage_id": []any{l.stage, f.stageName(l.stage)}, "user_id": []any{int64(1), l.user},
			"date_deadline": l.deadline, "company_id": []any{l.company, ""}, "write_date": wd, "priority": "1"})
	}
	orders := []struct {
		id      int64
		name    string
		partner int64
		amount  float64
		state   string
		lead    int64
	}{
		{301, "SO-2026-0745", 111, 0.72e9, "sale", 0}, {302, "SO-2026-0688", 112, 0.98e9, "sale", 0}, {303, "SO-2026-0702", 113, 1.45e9, "sale", 0},
		{304, "SO-2026-0799", 110, 1.2e9, "sale", 210}, {305, "SO-2026-0812", 109, 1.9e9, "sale", 209}, {306, "SO-2026-0771", 108, 0.412e9, "sale", 0},
	}
	for _, o := range orders {
		f.data["sale.order"] = append(f.data["sale.order"], Record{"id": o.id, "name": o.name, "partner_id": []any{o.partner, ""}, "amount_total": o.amount,
			"state": o.state, "opportunity_id": []any{o.lead, ""}, "write_date": wd})
	}
	tasks := []struct {
		id    int64
		so    string
		stage string
	}{
		{401, "SO-2026-0745", "SO-SPK-BAST"}, {402, "SO-2026-0688", "Invoice"}, {403, "SO-2026-0702", "Invoice"},
		{404, "SO-2026-0799", "Persiapan"}, {405, "SO-2026-0812", "Pemasangan"}, {406, "SO-2026-0771", "Lunas"},
	}
	for _, t := range tasks {
		f.data["project.task"] = append(f.data["project.task"], Record{"id": t.id, "name": "Project " + t.so, "sale_order_id": []any{int64(0), t.so}, "stage_id": []any{int64(0), t.stage}, "write_date": wd})
	}
	moves := []struct {
		id       int64
		name     string
		partner  int64
		residual float64
		total    float64
		state    string
		due      string
	}{
		{501, "INV/2026/0702", 113, 1.45e9, 1.45e9, "not_paid", "2026-09-20"},
		{502, "INV/2026/0688", 112, 0.98e9, 0.98e9, "not_paid", "2026-07-19"},
		{503, "INV/2026/0771", 108, 0, 0.412e9, "paid", "2026-09-27"},
	}
	for _, m := range moves {
		f.data["account.move"] = append(f.data["account.move"], Record{"id": m.id, "name": m.name, "partner_id": []any{m.partner, ""}, "amount_residual": m.residual,
			"amount_total": m.total, "payment_state": m.state, "invoice_date_due": m.due, "move_type": "out_invoice", "write_date": wd})
	}
	return f
}

func (f *Fake) stageName(id int64) string {
	for _, s := range f.data["crm.stage"] {
		if s["id"] == id {
			return s["name"].(string)
		}
	}
	return ""
}

func matches(r Record, domain []any) bool {
	for _, d := range domain {
		c, ok := d.([]any)
		if !ok || len(c) != 3 {
			continue
		}
		field, _ := c[0].(string)
		op, _ := c[1].(string)
		v := r[field]
		switch op {
		case ">":
			if fmt.Sprint(v) <= fmt.Sprint(c[2]) {
				return false
			}
		case "=":
			if fmt.Sprint(v) != fmt.Sprint(c[2]) {
				return false
			}
		case "ilike":
			if !strings.Contains(strings.ToLower(fmt.Sprint(v)), strings.ToLower(fmt.Sprint(c[2]))) {
				return false
			}
		}
	}
	return true
}

// SearchRead filters the in-memory records.
func (f *Fake) SearchRead(_ context.Context, model string, domain []any, _ []string, opt SearchOpts) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Record
	for _, r := range f.data[model] {
		if matches(r, domain) {
			cp := Record{}
			for k, v := range r {
				cp[k] = v
			}
			out = append(out, cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["id"]) < fmt.Sprint(out[j]["id"]) })
	if opt.Offset > 0 {
		if opt.Offset >= len(out) {
			return nil, nil
		}
		out = out[opt.Offset:]
	}
	if opt.Limit > 0 && len(out) > opt.Limit {
		out = out[:opt.Limit]
	}
	return out, nil
}

// FieldsGet returns no custom fields (chatter fallback is used).
func (f *Fake) FieldsGet(_ context.Context, _ string) (map[string]any, error) {
	return map[string]any{}, nil
}

// Companies returns [NEW] companies.
func (f *Fake) Companies(ctx context.Context) ([]Record, error) {
	return f.SearchRead(ctx, "res.company", []any{[]any{"name", "ilike", "[NEW]"}}, nil, SearchOpts{})
}

// Touch changes write_date of a record (tests: incremental sync).
func (f *Fake) Touch(model string, id int64, writeDate string, changes Record) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.data[model] {
		if r["id"] == id {
			r["write_date"] = writeDate
			for k, v := range changes {
				r[k] = v
			}
		}
	}
}

// CurrentWriteDate returns write_date of a record.
func (f *Fake) CurrentWriteDate(_ context.Context, model string, id int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ConflictOn[fmt.Sprintf("%s#%d", model, id)] {
		return "2099-01-01 00:00:00", nil
	}
	for _, r := range f.data[model] {
		if r["id"] == id {
			return fmt.Sprint(r["write_date"]), nil
		}
	}
	return "", fmt.Errorf("fake odoo: %s#%d tidak ada", model, id)
}

// Apply records a validated write.
func (f *Fake) Apply(ctx context.Context, op WriteOp) (int64, error) {
	if err := Validate(op); err != nil {
		return 0, err
	}
	if op.ExpectedWriteDate != "" && op.ID != 0 {
		have, err := f.CurrentWriteDate(ctx, op.Model, op.ID)
		if err != nil {
			return 0, err
		}
		if have != op.ExpectedWriteDate {
			return 0, &ErrConflict{op.Model, op.ID, have, op.ExpectedWriteDate}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Writes = append(f.Writes, op)
	if op.Method == "create" {
		f.nextID++
		return f.nextID, nil
	}
	return op.ID, nil
}
