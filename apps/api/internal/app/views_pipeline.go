package app

import (
	"bufio"
	"context"
	"fmt"
	"html"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/storage"
)

func pct0(v float64) string { return fmt.Sprintf("%d%%", int(math.Round(v*100))) }

func dateLabel(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmt.Sprintf("%s %d", domain.ShortDate(*t), t.Year())
}

func (a *App) handlePipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	sc := p.Scope()
	now := domain.Now()
	deals, err := a.Ins.Deals(ctx, sc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	q, _ := domain.Quarter(now)
	qstart := domain.QuarterStart(now)

	// Sync bar.
	var linked, conflicts int
	var lastSync *time.Time
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM opportunities WHERE source_system='odoo' AND NOT historical`).Scan(&linked)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM signals WHERE type='sync_conflict' AND resolved_at IS NULL`).Scan(&conflicts)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT max(finished_at) FROM sync_runs WHERE system='odoo' AND ok`).Scan(&lastSync)
	var proposals, writes int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM actions WHERE type='link_to_odoo' AND status='proposed'`).Scan(&proposals)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM odoo_writes WHERE ok`).Scan(&writes)
	openN, wonQ := 0, 0
	for _, d := range deals {
		if d.IsPipeline() {
			openN++
		}
		if d.Status == "won" && d.WonAt != nil && !d.WonAt.Before(qstart) {
			wonQ++
		}
	}
	sync := map[string]any{}
	db := defaultStr(a.Cfg.OdooDB, "gosyen-solusi-indonesia-2")
	if linked > 0 {
		meta := "Stage, revenue, probability, tag & activity mengikuti Odoo · sinkron 2 arah"
		if lastSync != nil {
			meta += " · " + strings.ReplaceAll(domain.ClockID(*lastSync), ".", ":")
		}
		if a.OdooMock {
			meta += " · mode mock (kredensial Odoo belum diisi)"
		}
		if proposals > 0 {
			meta += fmt.Sprintf(" · %d usulan tautan menunggu", proposals)
		}
		if writes > 0 {
			meta += fmt.Sprintf(" · %d tulis-balik", writes)
		}
		tone := "good"
		if conflicts > 0 {
			tone = "warn"
		}
		sync = map[string]any{"connected": true, "title": "Odoo CRM · " + db, "meta": meta, "source_label": "odoo",
			"pill": map[string]string{"k": tone, "t": fmt.Sprintf("%d opportunity · %d won Q%d · %d konflik", openN, wonQ, q, conflicts), "icon": "i-check"}}
	} else {
		sync = map[string]any{"connected": false, "title": "Odoo belum terhubung — stage ARC", "meta": "Stage disimpan di ARC (Baru / Berkualifikasi / Penawaran / Won / Lost) sampai Odoo ditautkan", "source_label": "arc",
			"pill": map[string]string{"k": "neutral", "t": fmt.Sprintf("%d opportunity · %d won Q%d", openN, wonQ, q)}}
	}

	// Kanban.
	type st struct {
		ID   int
		Name string
		Won  bool
	}
	var stages []st
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, name, is_won FROM stage_definitions WHERE NOT is_lost ORDER BY seq`)
	if err == nil {
		for rows.Next() {
			var s st
			_ = rows.Scan(&s.ID, &s.Name, &s.Won)
			stages = append(stages, s)
		}
		rows.Close()
	}
	initials := map[string]string{}
	urows, err := a.DB.Pool.Query(ctx, `SELECT name, initials FROM users`)
	if err == nil {
		for urows.Next() {
			var n, i string
			_ = urows.Scan(&n, &i)
			initials[n] = i
		}
		urows.Close()
	}
	cols := []map[string]any{}
	for _, s := range stages {
		var ds []insights.Deal
		for _, d := range deals {
			if d.StageID != s.ID {
				continue
			}
			if d.Status == "won" && (d.WonAt == nil || d.WonAt.Before(qstart)) {
				continue
			}
			ds = append(ds, d)
		}
		sort.SliceStable(ds, func(i, j int) bool { return ds[i].Value > ds[j].Value })
		total := 0.0
		cards := []map[string]any{}
		for _, d := range ds {
			total += d.Value
			won := d.Status == "won"
			var acc any
			if d.IsPipeline() {
				acc = d.AccountID
			}
			closing := d.Closing
			if won && d.WonAt != nil {
				closing = "Won " + domain.ShortDate(*d.WonAt)
			} else if closing == "" {
				closing = dateLabel(d.Deadline)
			}
			var health any
			var activity any
			if d.HealthOK && !won {
				health = d.Health
			}
			if d.Activity != "" {
				activity = d.Activity
			}
			var prio any = d.Priority
			if won {
				prio = nil
			}
			cards = append(cards, map[string]any{"id": d.ID, "account_id": acc, "opp": d.Name, "account": d.Account, "value": d.Value,
				"prob": fmt.Sprintf("at %d%%", d.ManualProb), "closing": closing, "tags": nz(d.Tags), "prio": prio, "activity": activity,
				"owner_initials": defaultStr(initials[d.OwnerName], "--"), "health": health, "signal": insights.SignalLabel(d.Signal), "note": d.Note,
				"action": a.actionViewPtr(ctx, d.NextAction), "won": won})
		}
		cols = append(cols, map[string]any{"id": s.ID, "name": s.Name, "total": total, "won": s.Won, "cards": cards})
	}

	// Field (health × value), inference, weighted.
	weighted, open, err := a.Ins.WeightedPipeline(ctx, sc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	field := []map[string]any{}
	inference := []map[string]any{}
	total := 0.0
	for _, d := range open {
		total += d.Value
		next := ""
		if act, err := a.Actions.Get(ctx, d.NextAction); err == nil {
			next = act.Title
		}
		field = append(field, map[string]any{"id": d.AccountID, "account": d.Account, "opp": d.Name, "value": d.Value, "health": d.Health, "trend": d.Trend,
			"stage": d.Stage, "signal": insights.SignalLabel(d.Signal), "manual_prob": d.ManualProb, "next": next})
		if d.Stage == "Penawaran" && d.StageWhy != "" {
			inference = append(inference, map[string]any{"id": d.ID, "account": d.Account, "why": d.StageWhy, "signal": insights.SignalLabel(d.Signal), "stage": d.Stage,
				"manual": d.ManualProb, "arc": d.Health, "can_write": abs(d.ManualProb-d.Health) >= 15})
		}
	}
	wl, _ := a.Ins.ComputeWinLoss(ctx)
	winloss := map[string]any{"meta": fmt.Sprintf("24 bulan · %d deal", wl.Deals), "rows": []map[string]string{
		{"tag": "Menang", "html": fmt.Sprintf("Deal dengan <b>≥ 2 stakeholder</b> aktif menang %s× lebih sering", strings.ReplaceAll(fmt.Sprintf("%.1f", wl.MultiRatio), ".", ","))},
		{"tag": "Kalah", "html": fmt.Sprintf("<b>Sunyi &gt; 14 hari</b> setelah revisi harga: %d%% berakhir kalah atau mati", int(math.Round(wl.SilentLostPct)))},
		{"tag": "Siklus", "html": fmt.Sprintf("Pemerintah rata-rata <b>%d hari</b>, enterprise <b>%d hari</b> dari lead ke PO", int(math.Round(wl.GovCycle)), int(math.Round(wl.EntCycle)))},
	}}

	// Tenders.
	var kws []string
	a.Ins.Setting(ctx, "tender_keywords", &kws)
	tenders := []map[string]any{}
	trows, err := a.DB.Pool.Query(ctx, `SELECT id, title, source, hps::float8, deadline, description, reasons, COALESCE(match_score,0), status FROM tenders ORDER BY match_score DESC NULLS LAST, deadline`)
	if err == nil {
		for trows.Next() {
			var id, title, src, desc, reasons, status string
			var hps float64
			var dl *time.Time
			var score int
			_ = trows.Scan(&id, &title, &src, &hps, &dl, &desc, &reasons, &score, &status)
			tone := "bad"
			switch {
			case score >= 80:
				tone = "good"
			case score >= 60:
				tone = "warn"
			}
			close := ""
			if dl != nil {
				close = "tutup " + domain.ShortDate(*dl)
				if domain.DaysBetween(now, *dl) <= 5 {
					close = `<b style="color:var(--bad)">` + close + `</b>`
				}
			}
			parts := []string{html.EscapeString(src), "HPS " + fmtRp(hps), close}
			if desc != "" {
				d := html.EscapeString(desc)
				if reasons != "" && !strings.Contains(desc, reasons) {
					d += " — " + html.EscapeString(reasons)
				}
				parts = append(parts, d)
			}
			tenders = append(tenders, map[string]any{"id": id, "score": score, "tone": tone, "title": title, "detail_html": strings.Join(parts, " · "), "status": map[string]string{"new": "new", "qualified": "Dikualifikasi", "lead": "Lead dibuat", "skipped": "Dilewati"}[status], "primary": score >= 80})
		}
		trows.Close()
	}
	team, _ := a.teamRows(ctx)
	writeJSON(w, 200, map[string]any{
		"sync": sync, "stages": cols, "field": field, "field_title": fmt.Sprintf("%d deal terbuka · %s", len(open), domain.FormatRp1(total)),
		"inference": inference, "weighted": map[string]any{"sales": weighted.Sales, "arc": weighted.ARC, "note": insights.WeightedNote(weighted, open)},
		"winloss": winloss, "tenders": map[string]any{"meta": fmt.Sprintf("LPSE Jateng · DIY · Jatim + e-katalog · %d kata kunci", len(kws)), "items": tenders}, "team": team,
	})
}

func nz(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func pillPct(v float64, good, warn float64, has bool) map[string]string {
	if !has {
		return map[string]string{"k": "neutral", "t": "—"}
	}
	k := "bad"
	switch {
	case v >= good:
		k = "good"
	case v >= warn:
		k = "warn"
	}
	return map[string]string{"k": k, "t": fmt.Sprintf("%d%%", int(math.Round(v)))}
}

func (a *App) teamRows(ctx context.Context) ([]map[string]any, error) {
	team, err := a.Ins.Team(ctx)
	if err != nil {
		return nil, err
	}
	var coaching map[string]string
	a.Ins.Setting(ctx, "coaching", &coaching)
	out := []map[string]any{}
	for _, t := range team {
		resp := "—"
		if t.ResponseMin > 0 {
			resp = strings.ReplaceAll(fmt.Sprintf("%.1f jam", t.ResponseMin/60), ".", ",")
		}
		out = append(out, map[string]any{"name": t.Name, "branch": t.Branch, "pipeline": t.Pipeline, "response": resp,
			"followup": pillPct(t.FollowUpPct, 85, 70, t.FollowUpPct > 0), "multithread": pillPct(t.MultiPct, 60, 40, t.HasData), "coach": coaching[t.UserID]})
	}
	return out, nil
}

func (a *App) handleForecast(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	ex := []string{}
	if e := r.URL.Query().Get("exclude"); e != "" {
		ex = strings.Split(e, ",")
	}
	fc, err := a.Ins.Forecast(r.Context(), p.Scope(), ex...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, forecastView(fc))
}

func (a *App) handleSyncOdoo(w http.ResponseWriter, r *http.Request) {
	res, err := a.SyncOdoo(r.Context())
	if err != nil {
		writeErr(w, 502, "Sinkron Odoo gagal: "+err.Error())
		return
	}
	msg := fmt.Sprintf("Sinkron selesai · %d perubahan dari Odoo, %d tertaut, %d usulan tautan", res.Updated, res.Linked, res.Proposed)
	if res.Mock {
		msg += " · mode mock"
	}
	toast(w, msg)
}

func (a *App) handleTenderQualify(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	id, err := a.qualifyTender(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "tender.qualify", "tender", r.PathValue("id"), map[string]string{"opportunity": id})
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE actions SET status='executed', executed_at=now(), result_text='Dikualifikasi' WHERE type='qualify_tender' AND payload->>'tender_id'=$1 AND status='proposed'`, r.PathValue("id"))
	dest := "ARC"
	if !a.OdooMock {
		dest = "Odoo"
	}
	toast(w, "Lead dibuat di "+dest+" (Baru) · dokumen kualifikasi disiapkan dari tender serupa")
}

func (a *App) handleTenderSkip(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE tenders SET status='skipped', updated_at=now() WHERE id=$1`, r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "tender.skip", "tender", r.PathValue("id"), nil)
	toast(w, "Ditandai lewat · alasan dicatat untuk kalibrasi radar")
}

// handleTenderImport accepts CSV lines: title;agency;source;hps;deadline(YYYY-MM-DD);description
func (a *App) handleTenderImport(w http.ResponseWriter, r *http.Request) {
	sc := bufio.NewScanner(r.Body)
	n := 0
	for sc.Scan() {
		f := strings.Split(sc.Text(), ";")
		if len(f) < 5 || strings.EqualFold(f[0], "title") {
			continue
		}
		hps, _ := strconv.ParseFloat(strings.TrimSpace(f[3]), 64)
		desc := ""
		if len(f) > 5 {
			desc = f[5]
		}
		id := "tdr-" + storage.Hash(f[0], f[1])[:8]
		tag, err := a.DB.Pool.Exec(r.Context(), `INSERT INTO tenders(id,title,agency,source,hps,deadline,description) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::date,$7) ON CONFLICT DO NOTHING`,
			id, strings.TrimSpace(f[0]), strings.TrimSpace(f[1]), strings.TrimSpace(f[2]), hps, strings.TrimSpace(f[4]), desc)
		if err == nil {
			n += int(tag.RowsAffected())
		}
	}
	if _, err := a.Agents.TenderRadar(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	toast(w, fmt.Sprintf("%d tender diimpor dan dinilai Research agent", n))
}
