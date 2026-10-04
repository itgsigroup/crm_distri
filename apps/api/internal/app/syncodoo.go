package app

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"arc/packages/connectors/odoo"
	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

var odooModels = []struct {
	model  string
	fields []string
}{
	{"res.company", []string{"id", "name", "write_date"}},
	{"crm.stage", []string{"id", "name", "sequence", "is_won", "write_date"}},
	{"res.partner", []string{"id", "name", "is_company", "parent_id", "phone", "mobile", "email", "category_id", "user_id", "company_id", "write_date"}},
	{"crm.lead", []string{"id", "name", "type", "partner_id", "stage_id", "expected_revenue", "probability", "date_deadline", "tag_ids", "priority", "activity_ids", "user_id", "team_id", "company_id", "write_date"}},
	{"sale.order", []string{"id", "name", "state", "date_order", "partner_id", "opportunity_id", "amount_total", "write_date"}},
	{"project.task", []string{"id", "name", "stage_id", "sale_order_id", "project_id", "write_date"}},
	{"account.move", []string{"id", "name", "partner_id", "invoice_date", "invoice_date_due", "amount_residual", "amount_total", "payment_state", "move_type", "write_date"}},
}

// SyncResult reports one Odoo sync.
type SyncResult struct {
	Updated  int            `json:"updated"`
	PerModel map[string]int `json:"per_model"`
	Linked   int            `json:"linked"`
	Proposed int            `json:"proposed"`
	CreateIn int            `json:"create_in_odoo"`
	L2CMoved int            `json:"l2c_moved"`
	Mock     bool           `json:"mock"`
}

// SyncOdoo pulls Odoo incrementally (write_date watermark, batch 200) into
// odoo_records, then links ARC records and maps stages/L2C. It never writes to Odoo.
func (a *App) SyncOdoo(ctx context.Context) (SyncResult, error) {
	res := SyncResult{PerModel: map[string]int{}, Mock: a.OdooMock}
	var runID int64
	_ = a.DB.Pool.QueryRow(ctx, `INSERT INTO sync_runs(system, started_at) VALUES ('odoo', $1) RETURNING id`, domain.Now()).Scan(&runID)
	finish := func(err error) (SyncResult, error) {
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE sync_runs SET finished_at=$5, ok=$2, counts=$3, error=$4 WHERE id=$1`, runID, err == nil, storage.JSONObj(res), msg, domain.Now())
		return res, err
	}
	for _, m := range odooModels {
		var wm string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT watermark FROM sync_watermarks WHERE system='odoo' AND model=$1 AND company=0`, m.model).Scan(&wm)
		dom := []any{}
		if wm != "" {
			dom = append(dom, []any{"write_date", ">", wm})
		}
		maxWD := wm
		for offset := 0; ; offset += 200 {
			recs, err := a.OdooReader.SearchRead(ctx, m.model, dom, m.fields, odoo.SearchOpts{Limit: 200, Offset: offset, Order: "write_date asc, id asc"})
			if err != nil {
				return finish(fmt.Errorf("odoo %s: %w", m.model, err))
			}
			for _, r := range recs {
				wd := fmt.Sprint(r["write_date"])
				id := toInt(r["id"])
				comp := int64(0)
				if c, ok := r["company_id"].([]any); ok && len(c) > 0 {
					comp = toInt(c[0])
				}
				tag, err := a.DB.Pool.Exec(ctx, `INSERT INTO odoo_records(model,odoo_id,company_id,write_date,data) VALUES ($1,$2,$3,$4,$5)
					ON CONFLICT (model,odoo_id) DO UPDATE SET write_date=EXCLUDED.write_date, data=EXCLUDED.data, synced_at=now() WHERE odoo_records.write_date <> EXCLUDED.write_date`,
					m.model, id, comp, wd, storage.JSONObj(r))
				if err != nil {
					return finish(err)
				}
				if tag.RowsAffected() > 0 {
					res.Updated++
					res.PerModel[m.model]++
				}
				if wd > maxWD {
					maxWD = wd
				}
			}
			if len(recs) < 200 {
				break
			}
		}
		if maxWD != "" && maxWD != wm {
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO sync_watermarks(system,model,company,watermark) VALUES ('odoo',$1,0,$2) ON CONFLICT (system,model,company) DO UPDATE SET watermark=EXCLUDED.watermark, updated_at=now()`, m.model, maxWD)
		}
	}
	if err := a.mapOdoo(ctx, &res); err != nil {
		return finish(err)
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE connectors SET last_sync_at=$1 WHERE id='odoo'`, domain.Now())
	return finish(nil)
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

func m2oID(v any) int64 {
	if a, ok := v.([]any); ok && len(a) > 0 {
		return toInt(a[0])
	}
	return 0
}

func m2oName(v any) string {
	if a, ok := v.([]any); ok && len(a) > 1 {
		return fmt.Sprint(a[1])
	}
	return ""
}

// normAccount canonicalises Indonesian organisation names for matching.
func normAccount(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer("pemerintah kabupaten", "pemkab", "pemerintah kota", "pemkot", "kabupaten", "kab.", "pt ", "", "cv ", "", ".", "", ",", "")
	return strings.Join(strings.Fields(r.Replace(s)), " ")
}

func tokenSim(a, b string) float64 {
	ta := map[string]bool{}
	for _, t := range strings.Fields(strings.ToLower(a)) {
		ta[strings.Trim(t, "+×,.")] = true
	}
	inter, union := 0, len(ta)
	seen := map[string]bool{}
	for _, t := range strings.Fields(strings.ToLower(b)) {
		t = strings.Trim(t, "+×,.")
		if seen[t] {
			continue
		}
		seen[t] = true
		if ta[t] {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func (a *App) mapOdoo(ctx context.Context, res *SyncResult) error {
	recs := func(model string) ([]map[string]any, error) {
		rows, err := a.DB.Pool.Query(ctx, `SELECT data FROM odoo_records WHERE model=$1 ORDER BY odoo_id`, model)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []map[string]any
		for rows.Next() {
			var m map[string]any
			if err := rows.Scan(&m); err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, rows.Err()
	}
	// Stages: Odoo crm.stage names become the stage definitions (never hardcoded).
	stages, err := recs("crm.stage")
	if err != nil {
		return err
	}
	stageByOdoo := map[int64]string{}
	for _, s := range stages {
		name := fmt.Sprint(s["name"])
		stageByOdoo[toInt(s["id"])] = name
		won, _ := s["is_won"].(bool)
		if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO stage_definitions(name,seq,is_won,source_system,source_id) VALUES ($1,$2,$3,'odoo',$4)
			ON CONFLICT (name) DO UPDATE SET source_system='odoo', source_id=EXCLUDED.source_id, seq=EXCLUDED.seq`, name, toInt(s["sequence"]), won, fmt.Sprint(s["id"])); err != nil {
			return err
		}
	}
	// Accounts.
	partners, err := recs("res.partner")
	if err != nil {
		return err
	}
	type acc struct{ id, name string }
	var accts []acc
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, name FROM accounts`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var x acc
		_ = rows.Scan(&x.id, &x.name)
		accts = append(accts, x)
	}
	rows.Close()
	accByPartner := map[int64]string{}
	for _, p := range partners {
		if c, _ := p["is_company"].(bool); !c {
			continue
		}
		pid := toInt(p["id"])
		pn := normAccount(fmt.Sprint(p["name"]))
		for _, x := range accts {
			if normAccount(x.name) == pn {
				accByPartner[pid] = x.id
				if _, err := a.DB.Pool.Exec(ctx, `UPDATE accounts SET odoo_partner_id=$2, odoo_company_id=$3, source_system='odoo', source_id=$4 WHERE id=$1 AND (odoo_partner_id IS DISTINCT FROM $2)`,
					x.id, pid, m2oID(p["company_id"]), fmt.Sprint(pid)); err != nil {
					return err
				}
				break
			}
		}
	}
	// Opportunities.
	leads, err := recs("crm.lead")
	if err != nil {
		return err
	}
	type opp struct {
		id, acc, name, src string
		value              float64
		deadline, won      *time.Time
	}
	var opps []opp
	rows, err = a.DB.Pool.Query(ctx, `SELECT id, account_id, name, COALESCE(source_id,''), expected_revenue::float8, date_deadline, won_at FROM opportunities WHERE NOT historical AND status IN ('open','won')`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var o opp
		_ = rows.Scan(&o.id, &o.acc, &o.name, &o.src, &o.value, &o.deadline, &o.won)
		opps = append(opps, o)
	}
	rows.Close()
	linkedOdoo := map[string]bool{}
	for _, o := range opps {
		if o.src != "" {
			linkedOdoo[o.src] = true
		}
	}
	matchedARC := map[string]bool{}
	for _, l := range leads {
		lid := fmt.Sprint(toInt(l["id"]))
		accID := accByPartner[m2oID(l["partner_id"])]
		stageName := stageByOdoo[m2oID(l["stage_id"])]
		rev, _ := l["expected_revenue"].(float64)
		prob, _ := l["probability"].(float64)
		var dl *time.Time
		if s, ok := l["date_deadline"].(string); ok && s != "" {
			if t, err := time.ParseInLocation("2006-01-02", s, domain.Jakarta); err == nil {
				dl = &t
			}
		}
		if linkedOdoo[lid] {
			// Odoo is the source of truth: refresh mirrored fields and stage.
			for _, o := range opps {
				if o.src == lid {
					matchedARC[o.id] = true
				}
			}
			if _, err := a.DB.Pool.Exec(ctx, `UPDATE opportunities o SET name=$2, expected_revenue=$3, probability=$4, date_deadline=COALESCE($5,o.date_deadline),
				stage_id=COALESCE((SELECT id FROM stage_definitions WHERE name=$6), o.stage_id), source_write_date=now()
				WHERE source_system='odoo' AND source_id=$1 AND (o.name<>$2 OR o.expected_revenue<>$3 OR o.probability<>$4 OR o.stage_id <> COALESCE((SELECT id FROM stage_definitions WHERE name=$6), o.stage_id))`,
				lid, fmt.Sprint(l["name"]), rev, int(prob), dl, stageName); err != nil {
				return err
			}
			continue
		}
		if accID == "" {
			continue
		}
		best, bestConf := "", 0.0
		for _, o := range opps {
			if o.acc != accID || o.src != "" || matchedARC[o.id] {
				continue
			}
			name := tokenSim(o.name, fmt.Sprint(l["name"]))
			val := 0.0
			if o.value > 0 {
				d := math.Abs(rev-o.value) / o.value
				switch {
				case d <= 0.01:
					val = 1
				case d <= 0.2:
					val = 1 - d/0.2
				}
			}
			date := 0.0
			ref := o.deadline
			if o.won != nil {
				ref = o.won
			}
			if ref != nil && dl != nil {
				dd := math.Abs(ref.Sub(*dl).Hours() / 24)
				switch {
				case dd < 1.5:
					date = 1
				case dd <= 30:
					date = 0.5
				}
			}
			conf := 0.5*name + 0.3*val + 0.2*date
			if conf > bestConf {
				best, bestConf = o.id, conf
			}
		}
		if best == "" || bestConf < 0.4 {
			continue
		}
		matchedARC[best] = true
		if bestConf >= 0.95 {
			if _, err := a.DB.Pool.Exec(ctx, `UPDATE opportunities SET source_system='odoo', source_id=$2, locked_to_source=true, source_write_date=now(),
				stage_id=COALESCE((SELECT id FROM stage_definitions WHERE name=$3), stage_id), probability=$4 WHERE id=$1`, best, lid, stageName, int(prob)); err != nil {
				return err
			}
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO odoo_links(arc_type,arc_id,odoo_model,odoo_id,confidence,status) VALUES ('opportunity',$1,'crm.lead',$2,$3,'linked') ON CONFLICT (arc_type,arc_id) DO UPDATE SET status='linked'`, best, toInt(l["id"]), bestConf)
			res.Linked++
			continue
		}
		if a.exists(ctx, `SELECT 1 FROM odoo_links WHERE arc_type='opportunity' AND arc_id=$1`, best) {
			continue
		}
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO odoo_links(arc_type,arc_id,odoo_model,odoo_id,confidence,status) VALUES ('opportunity',$1,'crm.lead',$2,$3,'proposed')`, best, toInt(l["id"]), bestConf)
		var arcName string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT name FROM opportunities WHERE id=$1`, best).Scan(&arcName)
		if _, created, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Sync agent", Type: "link_to_odoo", Kind: "internal", Icon: "i-refresh", ButtonLabel: "Tautkan",
			AccountID: accID, Title: fmt.Sprintf("Tautkan “%s” ke opportunity Odoo “%v”", arcName, l["name"]),
			Why:  fmt.Sprintf("Akun sama, nama & nilai mirip (confidence %.2f) — di bawah ambang auto-link 0,95.", bestConf),
			Prep: "Setelah ditautkan, field Odoo menjadi read-only di ARC dan stage dikunci ke Odoo.", Steps: []string{"Opportunity ARC ditautkan ke Odoo", "Stage & nilai mengikuti Odoo pada sinkron berikutnya"},
			Payload: map[string]any{"opportunity_id": best, "odoo_id": toInt(l["id"]), "stage": stageName, "probability": prob}, Confidence: bestConf,
			Evidence: []domain.Evidence{{DocumentID: "crm.lead:" + lid, Quote: fmt.Sprint(l["name"])}}}, storage.Actor{ID: "sync", Type: "agent"}); err != nil {
			return err
		} else if created {
			res.Proposed++
		}
	}
	// ARC opportunities without an Odoo counterpart → create_in_odoo (executed by the guarded writer, Stage 10).
	for _, o := range opps {
		if o.src != "" || matchedARC[o.id] || a.exists(ctx, `SELECT 1 FROM odoo_links WHERE arc_type='opportunity' AND arc_id=$1`, o.id) {
			continue
		}
		if a.exists(ctx, `SELECT 1 FROM actions WHERE type='create_in_odoo' AND payload->>'opportunity_id'=$1`, o.id) {
			continue
		}
		if _, created, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Sync agent", Type: "create_in_odoo", Kind: "internal", Icon: "i-plug", ButtonLabel: "Buat di Odoo",
			AccountID: o.acc, Title: fmt.Sprintf("Buat “%s” sebagai lead di Odoo", o.name), Why: "Opportunity ini dibuat di ARC dan belum ada padanannya di Odoo.",
			Prep: "Lead dibuat di stage pertama Odoo dengan catatan sumber “via ARC”.", Steps: []string{"crm.lead dibuat via ARC (daftar putih)", "Opportunity ARC ditautkan dan stage dikunci ke Odoo"},
			Payload: map[string]any{"opportunity_id": o.id}, Confidence: 0.9, Evidence: []domain.Evidence{{Source: "arc:opportunity", Quote: o.name}}}, storage.Actor{ID: "sync", Type: "agent"}); err != nil {
			return err
		} else if created {
			res.CreateIn++
		}
	}
	// L2C from sale orders + project task stage + invoices (Sam's segment definitions).
	orders, _ := recs("sale.order")
	tasks, _ := recs("project.task")
	moves, _ := recs("account.move")
	taskStage := map[string]string{}
	for _, t := range tasks {
		taskStage[m2oName(t["sale_order_id"])] = m2oName(t["stage_id"])
	}
	unpaid := map[int64]bool{}
	for _, mv := range moves {
		if fmt.Sprint(mv["payment_state"]) != "paid" {
			unpaid[m2oID(mv["partner_id"])] = true
		}
	}
	for _, so := range orders {
		name := fmt.Sprint(so["name"])
		st := strings.ToLower(taskStage[name])
		stage := ""
		switch {
		case strings.Contains(st, "persiapan"):
			stage = "persiapan"
		case strings.Contains(st, "pemasangan"), strings.Contains(st, "berjalan"):
			stage = "pemasangan"
		case strings.Contains(st, "bast"), strings.Contains(st, "selesai"):
			stage = "bast"
		case strings.Contains(st, "invoice"):
			stage = "invoice"
			if unpaid[m2oID(so["partner_id"])] {
				stage = "menunggu_bayar"
			}
		case strings.Contains(st, "lunas"):
			stage = "lunas"
		}
		if stage == "" {
			continue
		}
		tag, err := a.DB.Pool.Exec(ctx, `UPDATE cash_items SET stage=$2, stage_entered_at=CASE WHEN stage<>$2 THEN $3 ELSE stage_entered_at END, source_system='odoo', source_id=$4
			WHERE so_id=$1 AND (stage<>$2 OR source_id IS DISTINCT FROM $4)`, name, stage, domain.Now(), fmt.Sprint(toInt(so["id"])))
		if err != nil {
			return err
		}
		res.L2CMoved += int(tag.RowsAffected())
	}
	return nil
}

func (a *App) exists(ctx context.Context, q string, args ...any) bool {
	var b bool
	_ = a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(`+q+`)`, args...).Scan(&b)
	return b
}
