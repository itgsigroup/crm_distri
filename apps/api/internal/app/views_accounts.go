package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/storage"
)

var viaIcon = map[string]string{"mail": "i-mail", "chat": "i-chat", "people": "i-people", "doc": "i-doc", "form": "i-form", "box": "i-box", "phone": "i-phone"}
var channelVia = map[string]string{"email": "mail", "wa_message": "chat", "wa_group_message": "chat", "meeting": "people", "document": "doc", "form": "form", "erp_event": "box", "call": "phone", "note": "doc"}

func (a *App) canSeeAccount(r *http.Request, id string) bool {
	p, _ := PrincipalFrom(r.Context())
	sc := p.Scope()
	if sc.All {
		return true
	}
	var ok bool
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND (branch=$2 OR owner_user_id=$3))`, id, sc.Branch, sc.UserID).Scan(&ok)
	return ok
}

func (a *App) handleAccounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	deals, err := a.Ins.Deals(ctx, p.Scope())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	seen := map[string]bool{}
	items := []map[string]any{}
	now := domain.Now()
	for _, d := range deals {
		if !d.IsPipeline() || seen[d.AccountID] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(d.Account+d.Name+d.OwnerName+d.Branch), q) {
			continue
		}
		seen[d.AccountID] = true
		var last *time.Time
		_ = a.DB.Pool.QueryRow(ctx, `SELECT last_interaction_at FROM accounts WHERE id=$1`, d.AccountID).Scan(&last)
		lastLabel := ""
		if last != nil {
			lastLabel = domain.RelativeDay(*last, now)
		}
		items = append(items, map[string]any{"id": d.AccountID, "name": d.Account, "opp": d.Name, "last": lastLabel, "owner": d.OwnerName, "value": d.Value, "health": d.Health, "band": domain.Band(d.Health), "branch": d.Branch})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total_active": len(items)})
}

func (a *App) handleAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if !a.canSeeAccount(r, id) {
		writeErr(w, 403, "akun ini di luar cabang Anda")
		return
	}
	var name, sector, branch, owner, memory, lastVia string
	var health, trend int
	var memProv []byte
	var memUpdated, lastAt *time.Time
	err := a.DB.Pool.QueryRow(ctx, `SELECT a.name, a.sector, a.branch, COALESCE(u.name,''), COALESCE(a.health,0), a.health_trend_30d, a.memory, a.memory_provenance, a.memory_updated_at, a.last_interaction_at, a.last_via
		FROM accounts a LEFT JOIN users u ON u.id=a.owner_user_id WHERE a.id=$1`, id).Scan(&name, &sector, &branch, &owner, &health, &trend, &memory, &memProv, &memUpdated, &lastAt, &lastVia)
	if err != nil {
		writeErr(w, 404, "akun tidak ditemukan")
		return
	}
	now := domain.Now()
	out := map[string]any{"id": id, "name": name, "sector": sector, "branch": branch, "owner": owner, "health": health, "trend": trend, "last_icon": viaIcon[lastVia]}
	if out["last_icon"] == "" {
		out["last_icon"] = "i-mail"
	}
	if lastAt != nil {
		out["last"] = domain.RelativeDay(*lastAt, now)
	}
	// Main open opportunity.
	var oppID, oppName, stage, signal, why string
	var value float64
	var locked bool
	var bd []byte
	_ = a.DB.Pool.QueryRow(ctx, `SELECT o.id, o.name, sd.name, o.signal, o.stage_evidence, o.expected_revenue::float8, o.locked_to_source, o.health_breakdown, COALESCE(o.next_action_id,'')
		FROM opportunities o JOIN stage_definitions sd ON sd.id=o.stage_id WHERE o.account_id=$1 AND o.status='open' AND NOT o.historical ORDER BY o.expected_revenue DESC LIMIT 1`, id).
		Scan(&oppID, &oppName, &stage, &signal, &why, &value, &locked, &bd, new(string))
	out["value"] = value
	// Next action: proposed/executed/rejected action of the opportunity.
	var actID string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT next_action_id FROM opportunities WHERE id=$1), '')`, oppID).Scan(&actID)
	if actID == "" {
		_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM actions WHERE account_id=$1 AND status='proposed' AND type <> 'historical' ORDER BY created_at LIMIT 1`, id).Scan(&actID)
	}
	out["next_action"] = a.actionViewPtr(ctx, actID)
	out["next_meta"] = ""
	if actID != "" {
		if act, err := a.Actions.Get(ctx, actID); err == nil {
			if act.Status == domain.ActionExecuted && act.ExecutedAt != nil {
				out["next_meta"] = "Dijalankan · " + domain.ClockID(*act.ExecutedAt)
			} else {
				out["next_meta"] = "Tenggat: " + act.DueLabel
			}
		}
	}
	// Flags (open signals of the opportunity).
	flags := []map[string]any{}
	alts := []string{}
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, title, detail, severity, suggested_action FROM signals WHERE opportunity_id=$1 AND resolved_at IS NULL ORDER BY id`, oppID)
	if err == nil {
		for rows.Next() {
			var sid int64
			var t, s, k, act string
			_ = rows.Scan(&sid, &t, &s, &k, &act)
			flags = append(flags, map[string]any{"id": sid, "t": t, "s": s, "k": k, "act": act})
			if act != "" {
				alts = append(alts, act)
			}
		}
		rows.Close()
	}
	out["alternatives"] = alts
	// Installed systems & whitespace.
	inst := []map[string]any{}
	rows, err = a.DB.Pool.Query(ctx, `SELECT system, installed_year, warranty_label, service_contract FROM installed_systems WHERE account_id=$1 ORDER BY id`, id)
	if err == nil {
		for rows.Next() {
			var s, y, wl, c string
			_ = rows.Scan(&s, &y, &wl, &c)
			inst = append(inst, map[string]any{"s": s, "y": y, "w": wl, "c": c, "warn": regexp.MustCompile(`habis|Rusak`).MatchString(wl)})
		}
		rows.Close()
	}
	ws := []map[string]any{}
	var pot float64
	lines := 0
	rows, err = a.DB.Pool.Query(ctx, `SELECT product_line, status, value::float8, why FROM whitespace WHERE account_id=$1 ORDER BY seq`, id)
	if err == nil {
		for rows.Next() {
			var pl, st, why string
			var v float64
			_ = rows.Scan(&pl, &st, &v, &why)
			x := map[string]any{"p": pl, "st": st}
			if v > 0 {
				x["v"] = v
			}
			if why != "" {
				x["why"] = why
			}
			if st == "peluang" {
				pot += v
				lines++
			}
			ws = append(ws, x)
		}
		rows.Close()
	}
	if len(ws) > 0 {
		out["expansion"] = map[string]any{"installed": inst, "whitespace": ws, "potential": pot, "lines": lines}
	} else {
		out["expansion"] = nil
	}
	// Memory.
	var prov []map[string]string
	_ = json.Unmarshal(memProv, &prov)
	if prov == nil {
		prov = []map[string]string{}
	}
	updated := ""
	if memUpdated != nil {
		updated = "diperbarui " + relAgo(*memUpdated, now)
	}
	out["memory"] = map[string]any{"text": memory, "updated": updated, "provenance": prov}
	// Stakeholders.
	sh := []map[string]any{}
	strong := 0
	rows, err = a.DB.Pool.Query(ctx, `SELECT name, role, stakeholder_tag, strength, stakeholder_note FROM people WHERE account_id=$1 AND NOT is_internal AND (stakeholder_note <> '' OR stakeholder_tag IN ('decision','champion','ghost')) ORDER BY created_at`, id)
	if err == nil {
		for rows.Next() {
			var n, role, tag, note string
			var s int
			_ = rows.Scan(&n, &role, &tag, &s, &note)
			if s >= 2 {
				strong++
			}
			sh = append(sh, map[string]any{"n": n, "role": role, "tag": tag, "s": s, "note": note, "initials": domain.Initials(n)})
		}
		rows.Close()
	}
	out["stakeholders"] = sh
	out["single_threaded"] = strong <= 1 && len(sh) > 0
	// Deal intelligence.
	if oppID != "" {
		var comps map[string]int
		_ = json.Unmarshal(bd, &comps)
		breakdown := []map[string]any{}
		for _, c := range domain.HealthComponents {
			breakdown = append(breakdown, map[string]any{"label": c.Label, "value": comps[c.Key]})
		}
		out["deal"] = map[string]any{"opportunity_id": oppID, "opp": oppName, "odoo_stage": stage, "signal": insights.SignalLabel(signal), "stage_why": why, "breakdown": breakdown, "flags": flags, "locked": locked}
	} else {
		out["deal"] = nil
	}
	// Commitment ledger.
	ledger := map[string][]map[string]any{"kami": {}, "mereka": {}}
	rows, err = a.DB.Pool.Query(ctx, `SELECT who, text, detail, status, due_at FROM commitments WHERE account_id=$1 AND status <> 'cancelled' ORDER BY created_at, id`, id)
	if err == nil {
		for rows.Next() {
			var who, text, detail, st string
			var due *time.Time
			_ = rows.Scan(&who, &text, &detail, &st, &due)
			row := map[string]any{"t": text, "s": detail, "st": st}
			if st != "done" && due != nil && due.Before(domain.StartOfDay(now)) {
				row["st"] = "late"
				days := domain.DaysBetween(*due, now)
				if days >= 30 {
					row["d"] = "Lewat 30+ hari"
				} else {
					row["d"] = fmt.Sprintf("Lewat %d hari", days)
				}
			} else if st == "late" {
				row["st"] = "open"
			}
			ledger[who] = append(ledger[who], row)
		}
		rows.Close()
	}
	out["commitments"] = ledger
	// Timeline with what ARC inferred.
	tl := []map[string]any{}
	rows, err = a.DB.Pool.Query(ctx, `SELECT i.channel, i.occurred_at, COALESCE(NULLIF(i.participants_label,''), i.sender_name), i.body_text, i.inference, i.hot,
		COALESCE((SELECT string_agg(e.text, ' · ') FROM extractions e WHERE e.interaction_id=i.id), '')
		FROM interactions i WHERE i.account_id=$1 AND i.channel NOT IN ('wa_aggregate') AND (i.inference <> '' OR (i.thread_id IS NULL AND NOT i.raw_ref LIKE 'email:arc-fixture%') OR (i.raw_ref NOT LIKE 'fixture:%' AND EXISTS (SELECT 1 FROM extractions e WHERE e.interaction_id=i.id)))
		ORDER BY i.occurred_at DESC LIMIT 12`, id)
	if err == nil {
		for rows.Next() {
			var ch, who, body, inf, ext string
			var at time.Time
			var hot bool
			_ = rows.Scan(&ch, &at, &who, &body, &inf, &hot, &ext)
			via := channelVia[ch]
			if inf == "" {
				inf = ext
			}
			if inf == "" {
				continue
			}
			tl = append(tl, map[string]any{"d": domain.ShortDate(at), "via": via, "icon": viaIcon[via], "who": who, "t": body, "x": inf, "hot": hot})
		}
		rows.Close()
	}
	out["timeline"] = tl
	out["has_network"] = a.exists(ctx, `SELECT 1 FROM people p JOIN interactions i ON p.id = ANY(i.person_ids) WHERE p.account_id=$1 AND i.channel IN ('wa_aggregate','wa_message') LIMIT 1`, id)
	writeJSON(w, 200, out)
}

func relAgo(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Hour:
		return "baru saja"
	case d < 20*time.Hour:
		return fmt.Sprintf("%d jam lalu", int(d.Hours()))
	case d < 40*time.Hour:
		return "kemarin"
	}
	return fmt.Sprintf("%d hari lalu", int(d.Hours()/24))
}

// handleSignalAct prepares the flag's suggested action as a proposal (goes to the approval queue).
func (a *App) handleSignalAct(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	var acc, opp, title, detail, act string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(account_id,''), COALESCE(opportunity_id,''), title, detail, suggested_action FROM signals WHERE id=$1`, r.PathValue("id")).Scan(&acc, &opp, &title, &detail, &act); err != nil {
		writeErr(w, 404, "sinyal tidak ditemukan")
		return
	}
	id, _, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Follow-up agent", Type: "prepare_document", Kind: "task", Icon: "i-doc", ButtonLabel: "Setujui",
		AccountID: acc, Title: act, DueLabel: "Minggu ini", Why: title + ": " + detail, Prep: "Disiapkan agen dari sinyal ini; masuk antrean approval.",
		Steps: []string{"Agen menyiapkan bahan", "Masuk antrean approval Anda"}, InQueue: true, Confidence: 0.8,
		Evidence: []domain.Evidence{{Source: "signal:" + r.PathValue("id"), Quote: detail}}}, p.Actor())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"action": a.actionViewPtr(ctx, id), "toast": act + " → disiapkan agen, masuk antrean approval"})
}

func (a *App) handleAlternative(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	var req struct {
		Label string `json:"label"`
	}
	if err := decode(r, &req); err != nil || req.Label == "" {
		writeErr(w, 400, "label wajib")
		return
	}
	id, _, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Follow-up agent", Type: "prepare_document", Kind: "task", Icon: "i-doc", AccountID: r.PathValue("id"), Title: req.Label,
		DueLabel: "Minggu ini", Why: "Alternatif langkah dipilih dari halaman akun.", Prep: "Disiapkan agen; masuk antrean approval.", Steps: []string{"Masuk antrean approval"}, InQueue: true, Confidence: 0.75}, p.Actor())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"action": a.actionViewPtr(ctx, id), "toast": req.Label + " → disiapkan agen, masuk antrean approval"})
}

// handleWhitespace creates an expansion opportunity (the click is the human decision).
func (a *App) handleWhitespace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	var req struct {
		Line string `json:"line"`
	}
	if err := decode(r, &req); err != nil || req.Line == "" {
		writeErr(w, 400, "lini wajib")
		return
	}
	acc := r.PathValue("id")
	var value float64
	var why string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT value::float8, why FROM whitespace WHERE account_id=$1 AND product_line=$2`, acc, req.Line).Scan(&value, &why); err != nil {
		writeErr(w, 404, "lini tidak ditemukan")
		return
	}
	act, _, err := a.proposeAndApprove(ctx, p, actions.Proposal{Agent: "Research agent", Type: "expansion_opportunity", Kind: "task", Icon: "i-trend", AccountID: acc,
		Title: "Opportunity " + req.Line, Why: why, Prep: "Research agent menyiapkan konteks.", Steps: []string{"Opportunity dibuat di stage Baru", "Konteks ruang ekspansi dilampirkan"},
		Payload: map[string]any{"name": req.Line, "value": value, "source": "ekspansi", "line": req.Line, "note": why}, Evidence: []domain.Evidence{{Source: "whitespace", Quote: why}}, Confidence: 0.8})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE whitespace SET status='proses' WHERE account_id=$1 AND product_line=$2`, acc, req.Line)
	dest := "ARC"
	if !a.OdooMock {
		dest = "Odoo"
	}
	writeJSON(w, 200, map[string]any{"action": a.actionView(act), "toast": fmt.Sprintf("Opportunity “%s” dibuat di %s (Baru) · %s · Research agent menyiapkan konteks", req.Line, dest, fmtRp(value))})
}

func (a *App) handleMemoryAppend(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Note string `json:"note"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "format tidak valid")
		return
	}
	if err := a.Agents.AppendMemory(r.Context(), r.PathValue("id"), req.Note, p.Actor()); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	toast(w, "Catatan ditambahkan ke memori akun")
}

func (a *App) handleOverride(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if !p.Human() {
		writeErr(w, 403, "override hanya untuk pengguna manusia")
		return
	}
	var req struct {
		Stage       string `json:"stage"`
		Probability *int   `json:"probability"`
		Reason      string `json:"reason"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Reason) == "" {
		writeErr(w, 400, "alasan override wajib diisi")
		return
	}
	id := r.PathValue("id")
	var locked bool
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT locked_to_source FROM opportunities WHERE id=$1`, id).Scan(&locked); err != nil {
		writeErr(w, 404, "opportunity tidak ditemukan")
		return
	}
	if req.Stage != "" {
		if locked {
			writeErr(w, 409, "stage milik Odoo — ubah di Odoo; ARC hanya mencatat override sebagai catatan")
			return
		}
		if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE opportunities SET stage_id=(SELECT id FROM stage_definitions WHERE name=$2) WHERE id=$1`, id, req.Stage); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if req.Probability != nil {
		_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE opportunities SET arc_probability=$2 WHERE id=$1`, id, *req.Probability)
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "override", "opportunity", id, req)
	toast(w, "Override stage tercatat di audit log dengan alasan Anda")
}

func (a *App) handleStageMove(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		StageID int `json:"stage_id"`
	}
	if err := decode(r, &req); err != nil || req.StageID == 0 {
		writeErr(w, 400, "stage_id wajib")
		return
	}
	id := r.PathValue("id")
	var locked bool
	var won bool
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT o.locked_to_source, sd.is_won FROM opportunities o, stage_definitions sd WHERE o.id=$1 AND sd.id=$2`, id, req.StageID).Scan(&locked, &won); err != nil {
		writeErr(w, 404, "opportunity atau stage tidak ditemukan")
		return
	}
	if locked {
		writeErr(w, 409, "Stage milik Odoo · opportunity ini tertaut ke Odoo, pindahkan stage di Odoo")
		return
	}
	status := "open"
	if won {
		status = "won"
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE opportunities SET stage_id=$2, status=$3, won_at=CASE WHEN $3='won' THEN $4 ELSE won_at END, updated_at=now() WHERE id=$1`, id, req.StageID, status, domain.Now()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "stage.move", "opportunity", id, req)
	var name string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT name FROM stage_definitions WHERE id=$1`, req.StageID).Scan(&name)
	toast(w, "Stage dipindah ke "+name+" · tercatat di audit log")
}

func (a *App) handleWriteProbability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	id := r.PathValue("id")
	deals, _ := a.Ins.Deals(ctx, p.Scope())
	var d *insights.Deal
	for i := range deals {
		if deals[i].ID == id {
			d = &deals[i]
		}
	}
	if d == nil {
		writeErr(w, 404, "opportunity tidak ditemukan")
		return
	}
	if diff := d.Health - d.ManualProb; diff > -15 && diff < 15 {
		writeErr(w, 400, "selisih probabilitas < 15 poin — tidak perlu ditulis")
		return
	}
	actID, _, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Forecast agent", Type: "write_probability", Kind: "internal", Icon: "i-target", ButtonLabel: fmt.Sprintf("Tulis %d%% ke Odoo", d.Health),
		AccountID: d.AccountID, OpportunityID: d.ID, Title: fmt.Sprintf("Tulis probabilitas %d%% ke Odoo · %s", d.Health, d.Account),
		Why:     fmt.Sprintf("Sales menulis %d%%, bukti ARC menunjukkan %d (selisih %d poin). %s", d.ManualProb, d.Health, abs(d.Health-d.ManualProb), d.StageWhy),
		Prep:    "crm.lead.probability ditulis dengan catatan bukti di chatter (via ARC). Penulisan dibatalkan bila record berubah di Odoo sejak sinkron terakhir.",
		Steps:   []string{"Probabilitas ditulis ke Odoo", "Catatan bukti ditambahkan ke chatter", "Konflik write_date → dibatalkan + sinyal"},
		Payload: map[string]any{"probability": d.Health}, Confidence: 0.85, InQueue: false,
		Evidence: []domain.Evidence{{Source: "health", Quote: fmt.Sprintf("health %d, sales %d%%", d.Health, d.ManualProb)}}}, p.Actor())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"action": a.actionViewPtr(ctx, actID), "toast": fmt.Sprintf("Usulan “Tulis %d%% ke Odoo” dibuat · setujui di lembar tindakan", d.Health)})
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (a *App) handlePeople(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Pool.Query(r.Context(), `SELECT id, name, role, COALESCE(account_id,''), stakeholder_tag, strength, is_internal FROM people ORDER BY created_at LIMIT 500`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n, role, acc, tag string
		var s int
		var in bool
		_ = rows.Scan(&id, &n, &role, &acc, &tag, &s, &in)
		out = append(out, map[string]any{"id": id, "name": n, "role": role, "account_id": acc, "tag": tag, "strength": s, "internal": in})
	}
	writeJSON(w, 200, out)
}

func (a *App) handleOpportunities(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	deals, err := a.Ins.Deals(r.Context(), p.Scope())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []map[string]any{}
	for _, d := range deals {
		out = append(out, map[string]any{"id": d.ID, "account_id": d.AccountID, "account": d.Account, "name": d.Name, "value": d.Value, "stage": d.Stage, "status": d.Status,
			"health": d.Health, "trend": d.Trend, "signal": d.Signal, "manual_probability": d.ManualProb, "arc_probability": a.Ins.ArcProbability(r.Context(), d), "owner": d.OwnerName, "locked_to_odoo": d.Locked})
	}
	writeJSON(w, 200, out)
}

func (a *App) handleStages(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Pool.Query(r.Context(), `SELECT id, name, seq, is_won, is_lost, COALESCE(source_system,'') FROM stage_definitions ORDER BY seq`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, seq int
		var n, src string
		var won, lost bool
		_ = rows.Scan(&id, &n, &seq, &won, &lost, &src)
		out = append(out, map[string]any{"id": id, "name": n, "seq": seq, "is_won": won, "is_lost": lost, "source": src})
	}
	writeJSON(w, 200, out)
}

func (a *App) handleNetwork(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	period, _ := strconv.Atoi(q.Get("period"))
	acc := q.Get("account")
	if acc != "" && !a.canSeeAccount(r, acc) {
		writeErr(w, 403, "akun ini di luar cabang Anda")
		return
	}
	g, err := a.Ins.Network(r.Context(), period, q.Get("sales"), acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	sales := []map[string]string{}
	for _, s := range g.Sales {
		sales = append(sales, map[string]string{"id": s.ID, "n": s.Name, "branch": s.Branch, "no": s.No})
	}
	contacts := []map[string]any{}
	for _, c := range g.Contacts {
		var acc any
		if c.Account != "" {
			acc = c.Account
		}
		contacts = append(contacts, map[string]any{"id": c.ID, "n": c.Name, "role": c.Role, "acc": acc, "account_name": c.AccountName, "health": c.Health, "decision": c.Decision})
	}
	edges := [][]any{}
	for _, e := range g.Edges {
		edges = append(edges, []any{e[0], e[1], e[2]})
	}
	pairs := []map[string]any{}
	for _, pr := range g.Pairs {
		pairs = append(pairs, map[string]any{"a": pr.A, "b": pr.B, "account": pr.Account, "w": pr.W})
	}
	ins := []map[string]string{}
	for _, i := range g.Insights {
		ins = append(ins, map[string]string{"k": i.K, "icon": i.Icon, "t": i.T, "s": i.S})
	}
	writeJSON(w, 200, map[string]any{"months": g.Months, "period": g.Period, "period_label": insights.PeriodLabel(g.Period), "sales": sales, "contacts": contacts,
		"edges": edges, "monthly": g.Monthly, "pairs": pairs, "insights": ins, "count": map[string]int{"connections": g.Count[0], "messages": g.Count[1]}})
}
