package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"arc/packages/core/actions"
	"arc/packages/core/agents"
	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/storage"
)

var numberWords = []string{"Tidak ada", "Satu", "Dua", "Tiga", "Empat", "Lima", "Enam", "Tujuh", "Delapan", "Sembilan", "Sepuluh"}

func numWord(n int) string {
	if n >= 0 && n < len(numberWords) {
		return numberWords[n]
	}
	return fmt.Sprint(n)
}

func firstName(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return s
	}
	if (f[0] == "Pak" || f[0] == "Bu") && len(f) > 1 {
		return f[1]
	}
	return f[0]
}

func (a *App) handleShell(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	now := domain.Now()
	q, qend := domain.Quarter(now)
	var screens map[string][2]string
	a.Ins.Setting(ctx, "screens", &screens)
	out := map[string]map[string]string{}
	for k, v := range screens {
		out[k] = map[string]string{"title": v[0], "sub": v[1]}
	}
	var nums, accts int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM wa_sessions`).Scan(&nums)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(DISTINCT o.account_id) FROM opportunities o JOIN stage_definitions sd ON sd.id=o.stage_id WHERE o.status='open' AND NOT o.historical AND sd.seq >= 2 AND o.health IS NOT NULL`).Scan(&accts)
	out["today"] = map[string]string{"title": "Hari ini", "sub": fmt.Sprintf("%s · Q%d tersisa %d hari kerja", domain.LongDate(now), q, domain.WorkdaysUntil(now.AddDate(0, 0, -1), qend))}
	if c, ok := out["chat"]; ok {
		c["sub"] = fmt.Sprintf("WhatsApp %d nomor sales + grup project · baca, balas, dan biarkan ARC mencatat", nums)
	}
	if c, ok := out["rel"]; ok {
		c["sub"] = fmt.Sprintf("%d akun aktif · ingatan hidup, bukan record statis", accts)
	}
	var today, chat, conn int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM actions WHERE in_queue AND status='proposed'`).Scan(&today)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(sum(unread),0) FROM chat_threads WHERE NOT is_private`).Scan(&chat)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM wa_sessions WHERE status IN ('pairing','disconnected')`).Scan(&conn)
	items, _ := a.Ins.L2C(ctx)
	cash := 0
	for _, it := range items {
		if insights.L2CTone(it) == "bad" {
			cash++
		}
	}
	agentsList := []map[string]any{}
	for _, n := range []string{"Capture", "Research", "Follow-up", "Meeting prep", "Forecast", "Hygiene"} {
		agentsList = append(agentsList, map[string]any{"name": n, "on": true})
	}
	_ = p
	writeJSON(w, http.StatusOK, map[string]any{
		"screens": out, "badges": map[string]int{"today": today, "chat": chat, "cash": cash, "conn": conn},
		"agents": map[string]any{"active": len(agentsList), "list": agentsList}, "sample_data": domain.ClockIsDemo(),
	})
}

// ---------------- Actions ----------------

var modelSuffix = " via ARC MCP"

func (a *App) actionView(act actions.Action) map[string]any {
	model := agents.ModelLabel(act.Model)
	switch {
	case act.Model == "rules/nba":
		model = "aturan NBA (rules/nba.json)"
	case act.Model == "fake-deterministic":
	default:
		model += modelSuffix
	}
	decided := ""
	if t, ok := act.Decision["at"].(string); ok {
		if tt, err := time.Parse(time.RFC3339, t); err == nil {
			decided = domain.ClockID(tt)
		}
	}
	reject := ""
	if act.Status == domain.ActionRejected {
		reject = fmt.Sprint(act.Decision["reason"])
		if n, _ := act.Decision["note"].(string); n != "" {
			reject += " — " + n
		}
	}
	opts := []map[string]any{}
	for _, o := range act.Options {
		opts = append(opts, map[string]any{"key": o.Key, "label": o.Label, "primary": o.Primary})
	}
	tags := []map[string]string{}
	for _, t := range act.Tags {
		tags = append(tags, map[string]string{"k": t.K, "t": t.T})
	}
	if len(tags) == 0 && act.InQueue {
		tags = append(tags, map[string]string{"k": "accent", "t": act.Agent})
		if act.AccountName != "" {
			tags = append(tags, map[string]string{"k": "neutral", "t": act.AccountName})
		}
	}
	impact := []map[string]string{}
	for _, i := range act.Impact {
		impact = append(impact, map[string]string{"label": i.Label, "value": i.Value, "tone": i.Tone})
	}
	steps := act.Steps
	if steps == nil {
		steps = []string{}
	}
	prov := "dianalisis dari email, WhatsApp, meeting & Odoo akun ini"
	if act.AccountID == "" && act.CashItemID == "" {
		prov = "dianalisis dari data internal ARC"
	}
	return map[string]any{
		"id": act.ID, "agent": act.Agent, "type": act.Type, "kind": act.Kind, "title": act.Title, "button_label": act.ButtonLabel, "icon": act.Icon,
		"account_id": act.AccountID, "account_name": act.AccountName, "opportunity_id": act.OpportunityID, "due_label": act.DueLabel,
		"summary": act.Summary, "why": act.Why, "prep": act.Prep, "preview": act.Preview, "preview_from": act.PreviewFrom, "context_note": act.ContextNote,
		"impact": impact, "steps": steps, "options": opts, "tags": tags, "confidence": act.Confidence, "model": model, "provenance_line": prov,
		"status": act.Status, "decided_at_label": decided, "result_text": act.ResultText, "reject_reason": reject, "in_queue": act.InQueue,
	}
}

func (a *App) actionViewPtr(ctx context.Context, id string) any {
	if id == "" {
		return nil
	}
	act, err := a.Actions.Get(ctx, id)
	if err != nil {
		return nil
	}
	return a.actionView(act)
}

func (a *App) handleActions(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	f := actions.Filter{Status: r.URL.Query().Get("status"), AccountID: r.URL.Query().Get("account"), Limit: 200}
	if sc := p.Scope(); !sc.All {
		f.Branch, f.UserID = sc.Branch, sc.UserID
	}
	list, err := a.Actions.List(r.Context(), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []any{}
	for _, x := range list {
		out = append(out, a.actionView(x))
	}
	writeJSON(w, 200, out)
}

func (a *App) handleAction(w http.ResponseWriter, r *http.Request) {
	act, err := a.Actions.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeErr(w, 404, "saran tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a.actionView(act))
}

func (a *App) handleDecision(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if !p.Human() {
		writeErr(w, http.StatusForbidden, actions.ErrHumanOnly.Error())
		return
	}
	var d actions.Decision
	if err := decode(r, &d); err != nil {
		writeErr(w, 400, "format tidak valid")
		return
	}
	act, toastMsg, err := a.Actions.Decide(r.Context(), r.PathValue("id"), p.Actor(), d)
	switch {
	case errors.Is(err, actions.ErrHumanOnly):
		writeErr(w, 403, err.Error())
	case errors.Is(err, actions.ErrInvalid):
		writeErr(w, 400, err.Error())
	case errors.Is(err, storage.ErrNotFound):
		writeErr(w, 404, "saran tidak ditemukan")
	case err != nil:
		writeErr(w, 500, err.Error())
	default:
		writeJSON(w, 200, map[string]any{"action": a.actionView(act), "toast": toastMsg})
	}
}

// ---------------- Today ----------------

var signalIcons = map[string]string{"competitor_mentioned": "i-flag", "champion_moved": "i-user-x", "single_threaded": "i-people", "payment_on_time": "i-box", "inbound_high_value": "i-phone",
	"silent": "i-cal", "po_overdue": "i-doc", "stock_risk": "i-alert", "sync_conflict": "i-refresh"}

func greeting(h int) string {
	switch {
	case h < 11:
		return "Selamat pagi"
	case h < 15:
		return "Selamat siang"
	case h < 19:
		return "Selamat sore"
	}
	return "Selamat malam"
}

func (a *App) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	sc := p.Scope()
	now := domain.Now()
	sod := domain.StartOfDay(now)

	// Queue.
	inq := true
	list, err := a.Actions.List(ctx, actions.Filter{InQueue: &inq})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var queue []actions.Action
	pending := 0
	for _, x := range list {
		if x.Status == domain.ActionProposed {
			pending++
			queue = append(queue, x)
		} else if x.UpdatedAt.After(sod) && x.Status != domain.ActionSnoozed {
			queue = append(queue, x)
		}
	}
	sort.SliceStable(queue, func(i, j int) bool {
		oi, oj := toFloat(queue[i].Payload["queue_order"]), toFloat(queue[j].Payload["queue_order"])
		if oi == 0 {
			oi = 99
		}
		if oj == 0 {
			oj = 99
		}
		if oi != oj {
			return oi < oj
		}
		return queue[i].CreatedAt.Before(queue[j].CreatedAt)
	})
	qitems := []any{}
	for _, x := range queue {
		qitems = append(qitems, a.actionView(x))
	}
	meta := fmt.Sprintf("%d item · di luar batas otonomi agen", pending)
	if pending == 0 {
		meta = "Semua keputusan hari ini selesai"
	}

	// Commitments due today / late.
	type crow struct {
		who, text, detail, owner, status string
		due                              *time.Time
		draft                            bool
	}
	var crows []crow
	cond, args := sc.SQL("a", 3)
	rows, err := a.DB.Pool.Query(ctx, `SELECT c.who, c.text, c.detail, COALESCE(u.name,''), c.status, c.due_at, c.draft_ready FROM commitments c JOIN accounts a ON a.id=c.account_id LEFT JOIN users u ON u.id=c.owner_user_id
		WHERE ((c.due_at >= $1 AND c.due_at < $2) OR (c.status IN ('late','open') AND c.due_at < $1 AND c.due_at >= $1 - interval '7 days'))
		AND `+cond+` ORDER BY CASE c.who WHEN 'kami' THEN 0 ELSE 1 END, c.due_at`, append([]any{sod, sod.AddDate(0, 0, 1)}, args...)...)
	if err == nil {
		for rows.Next() {
			var c crow
			_ = rows.Scan(&c.who, &c.text, &c.detail, &c.owner, &c.status, &c.due, &c.draft)
			crows = append(crows, c)
		}
		rows.Close()
	}
	commitments := []map[string]any{}
	openToday := 0
	for _, c := range crows {
		pill := map[string]string{"k": "neutral", "t": "Terbuka"}
		late := c.due != nil && c.due.Before(sod) && c.status != "done"
		switch {
		case c.status == "done":
			pill = map[string]string{"k": "good", "t": "Selesai", "icon": "i-check"}
		case late || c.status == "late":
			pill = map[string]string{"k": "bad", "t": fmt.Sprintf("Lewat %d hari", domain.DaysBetween(*c.due, now))}
		case c.draft:
			pill = map[string]string{"k": "warn", "t": "Draf siap"}
		}
		if c.status != "done" {
			openToday++
		}
		s := c.detail
		who := "Mereka"
		if c.who == "kami" {
			who = "Kami"
			if c.owner != "" {
				s = c.owner + " · " + c.detail
			}
		}
		commitments = append(commitments, map[string]any{"who": who, "t": c.text, "s": s, "pill": pill})
	}

	// Signals of the last 24 hours.
	signals := []map[string]any{}
	srows, err := a.DB.Pool.Query(ctx, `SELECT s.id, s.type, s.severity, s.title, s.headline, s.detail, COALESCE(s.account_id,''), COALESCE(a.name,'') FROM signals s LEFT JOIN accounts a ON a.id=s.account_id
		WHERE s.resolved_at IS NULL AND s.detected_at >= $1 ORDER BY s.detected_at LIMIT 8`, now.Add(-24*time.Hour))
	if err == nil {
		for srows.Next() {
			var id int64
			var typ, sev, title, head, detail, acc, accName string
			_ = srows.Scan(&id, &typ, &sev, &title, &head, &detail, &acc, &accName)
			g := "rel:" + acc
			if acc == "" {
				g = "pros"
			}
			d := head
			if d == "" {
				d = strings.Trim(accName+" · "+detail, " ·")
			}
			icon := signalIcons[typ]
			if icon == "" {
				icon = "i-flag"
			}
			signals = append(signals, map[string]any{"id": id, "k": sev, "icon": icon, "title": title, "detail": d, "go": g})
		}
		srows.Close()
	}

	// Brief.
	var brief any
	if b, err := a.Agents.LatestBrief(ctx); err == nil {
		pts := []map[string]any{}
		for _, pt := range b.Points {
			pts = append(pts, map[string]any{"k": pt.K, "icon": pt.Icon, "html": pt.HTML})
		}
		c := b.SourceCounts
		brief = map[string]any{
			"written_label": "Ditulis ARC · " + strings.ReplaceAll(domain.ClockID(b.CreatedAt), ".", ":"),
			"meta":          fmt.Sprintf("Dari %d email, %d WhatsApp, %d meeting, %d pembayaran hari ini", c["emails"], c["whatsapp"], c["meetings"], c["payments"]),
			"points":        pts,
			"sources": []map[string]string{{"icon": "i-mail", "label": fmt.Sprintf("%d email", c["emails"])}, {"icon": "i-chat", "label": fmt.Sprintf("%d WhatsApp", c["whatsapp"])},
				{"icon": "i-people", "label": fmt.Sprintf("%d meeting", c["meetings"])}, {"icon": "i-box", "label": fmt.Sprintf("%d pembayaran ERP", c["payments"])}},
			"confidence": b.Confidence,
		}
	}

	fc, err := a.Ins.Forecast(ctx, sc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	forecast := forecastView(fc)
	gap := fc.Target - fc.Commit
	gapNote := ""
	_ = a.DB.Pool.QueryRow(ctx, `SELECT 'PO ' || a.name FROM commitments c JOIN accounts a ON a.id=c.account_id WHERE c.who='mereka' AND c.text ILIKE '%PO%' AND c.status IN ('open','late')
		ORDER BY (SELECT max(expected_revenue) FROM opportunities o WHERE o.account_id=a.id AND o.status='open') DESC NULLS LAST LIMIT 1`).Scan(&gapNote)
	var conns int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT DISTINCT u, p FROM interactions i, unnest(i.user_ids) u, unnest(i.person_ids) p
		WHERE i.channel IN ('wa_aggregate','wa_message') AND i.occurred_at >= $1 AND NOT EXISTS (SELECT 1 FROM people pp WHERE pp.id=p AND pp.is_internal)) x`, now.AddDate(0, 0, -30)).Scan(&conns)
	strip := []map[string]any{
		{"n": fmt.Sprint(pending), "tone": "accent", "label": "Keputusan", "muted": "satu klik per item", "scroll": "queue-card"},
		{"n": fmt.Sprint(openToday), "tone": "warn", "label": "Komitmen hari ini", "scroll": "commit-card"},
		{"n": fmt.Sprint(len(signals)), "tone": "bad", "label": "Sinyal baru", "scroll": "signal-card"},
		{"n": domain.FormatM1(math.Max(gap, 0)), "tone": "muted", "label": fmt.Sprintf("Gap Q%d", fc.Quarter), "muted": strings.Replace(gapNote, "PO Bank Sejahtera Daerah", "PO Bank Sejahtera", 1), "go": "cash"},
		{"n": "3D", "tone": "indigo", "label": "Peta koneksi", "muted": fmt.Sprintf("%d koneksi WA", conns), "go": "net"},
	}

	// Tomorrow.
	tm := sod.AddDate(0, 0, 1)
	agenda := []map[string]any{}
	erows, err := a.DB.Pool.Query(ctx, `SELECT starts_at, duration_min, title, location, attendees, prep_pills FROM calendar_events WHERE starts_at >= $1 AND starts_at < $2 ORDER BY starts_at`, tm, tm.AddDate(0, 0, 1))
	if err == nil {
		for erows.Next() {
			var st time.Time
			var dur int
			var title, loc, att string
			var pills []map[string]string
			_ = erows.Scan(&st, &dur, &title, &loc, &att, &pills)
			if pills == nil {
				pills = []map[string]string{}
			}
			agenda = append(agenda, map[string]any{"time": domain.ClockID(st), "duration": fmt.Sprintf("%d mnt", dur), "title": title, "detail": strings.Trim(loc+" · "+att, " ·"), "pills": pills})
		}
		erows.Close()
	}

	pulse, _ := a.Ins.Pulse(ctx)
	prows := []map[string]string{}
	for _, pr := range pulse {
		prows = append(prows, map[string]string{"label": pr.Label, "value": pr.Value, "delta": pr.Delta, "tone": pr.Tone})
	}
	var since string
	a.Ins.Setting(ctx, "arc_active_since", &since)
	sinceLabel := "Jul"
	if t, err := time.Parse(time.RFC3339, since); err == nil {
		sinceLabel = domain.MonthName(t.Month())
	}

	renewal := a.renewalRows(ctx)

	// Agents line.
	var interToday, commitsToday, confirm int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM interactions WHERE occurred_at >= $1 AND channel <> 'wa_aggregate'`, sod).Scan(&interToday)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM commitments WHERE created_at >= $1`, sod).Scan(&commitsToday)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM actions WHERE type IN ('merge_person','link_to_odoo') AND status='proposed'`).Scan(&confirm)
	var activity map[string]int
	if a.Ins.Setting(ctx, "today_activity", &activity) && domain.ClockIsDemo() {
		if activity["interactions"] > interToday {
			interToday = activity["interactions"]
		}
		if activity["commitments_new"] > commitsToday {
			commitsToday = activity["commitments_new"]
		}
	}
	var feed []map[string]string
	a.Ins.Setting(ctx, "agent_feed", &feed)
	feedOut := []map[string]string{}
	for _, f := range feed {
		h := strings.ReplaceAll(strings.ReplaceAll(f["html"], "{interactions}", fmt.Sprint(interToday)), "{commitments}", fmt.Sprint(commitsToday))
		feedOut = append(feedOut, map[string]string{"agent": f["agent"], "html": h})
	}

	first := firstName(p.Name)
	writeJSON(w, http.StatusOK, map[string]any{
		"greeting": map[string]string{"title": fmt.Sprintf("%s, %s.", greeting(now.Hour()), first),
			"sub": fmt.Sprintf("%s hal menunggu keputusan Anda. Sisanya sudah saya urus.", numWord(pending))},
		"strip": strip, "brief": brief,
		"queue":       map[string]any{"meta": meta, "filters": []map[string]string{{"key": "all", "label": "Semua"}, {"key": "send", "label": "Kirim"}, {"key": "policy", "label": "Kebijakan & kredit"}, {"key": "re", "label": "Re-engagement"}}, "items": qitems},
		"commitments": commitments, "signals": signals,
		"tomorrow":    map[string]any{"label": domain.DayLabel(tm), "items": agenda},
		"forecast":    forecast,
		"pulse":       map[string]any{"meta": fmt.Sprintf("Sejak ARC aktif (%s) vs 6 bln sebelumnya", sinceLabel), "rows": prows},
		"renewal":     map[string]any{"rows": renewal},
		"agents_line": map[string]any{"summary_html": fmt.Sprintf("<b>6 agen</b> memproses %d interaksi hari ini · %d komitmen baru · %d perlu konfirmasi", interToday, commitsToday, confirm), "feed": feedOut},
	})
}

func forecastView(fc insights.ForecastResult) map[string]any {
	scale := math.Max(fc.Pipeline, math.Max(fc.Best, fc.Target))
	if scale == 0 {
		scale = 1
	}
	w := func(v float64) float64 { return math.Min(99, math.Round(v/scale*100)) }
	return map[string]any{
		"title": fmt.Sprintf("Forecast Q%d", fc.Quarter),
		"meta":  fmt.Sprintf("Berbasis bukti · %d hari lagi", fc.WorkdaysLeft),
		"rows": []map[string]any{
			{"label": "Commit", "value": fc.Commit, "width": w(fc.Commit), "variant": ""},
			{"label": "Best case", "value": fc.Best, "width": w(fc.Best), "variant": "soft"},
			{"label": "Pipeline", "value": fc.Pipeline, "width": w(fc.Pipeline), "variant": "faint"},
		},
		"target": fc.Target, "target_pos": math.Round(fc.Target / scale * 100),
		"note":   fmt.Sprintf("Garis hitam = target %s. Commit hanya menghitung deal Won dan verbal dengan health ≥ 80; angka probabilitas manual sales tidak dipakai.", domain.FormatRp1(fc.Target)),
		"commit": fc.Commit, "best": fc.Best, "pipeline": fc.Pipeline,
	}
}

func (a *App) renewalRows(ctx context.Context) []map[string]any {
	now := domain.Now()
	var rows []map[string]any
	type w struct {
		acc, name string
		end       time.Time
	}
	var ws []w
	r, err := a.DB.Pool.Query(ctx, `SELECT s.account_id, a.name, s.warranty_end FROM installed_systems s JOIN accounts a ON a.id=s.account_id
		WHERE s.warranty_end <= $1 AND s.service_contract ILIKE 'tanpa kontrak%' ORDER BY s.warranty_end >= $2 DESC, s.warranty_end`, now.AddDate(0, 0, 120), now)
	if err == nil {
		for r.Next() {
			var x w
			_ = r.Scan(&x.acc, &x.name, &x.end)
			ws = append(ws, x)
		}
		r.Close()
	}
	if len(ws) > 0 {
		var pot float64
		names := []string{}
		ids := []string{}
		for _, x := range ws {
			ids = append(ids, x.acc)
			label := domain.MonthName(x.end.Month())
			if x.end.Before(now) {
				label = "lewat " + label
			}
			names = append(names, fmt.Sprintf("%s (%s)", shortAcc(x.name), label))
		}
		_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(sum(value),0)::float8 FROM whitespace WHERE account_id = ANY($1) AND status='peluang' AND product_line ILIKE 'maintenance%'`, ids).Scan(&pot)
		rows = append(rows, map[string]any{"icon": "i-refresh", "tone": "warn",
			"html": fmt.Sprintf(`<b>%d garansi habis ≤ 90 hari</b> · potensi kontrak maintenance <b class="num">%s/th</b><span hidden data-go="rel:%s"></span>`, len(ws), domain.FormatRp(pot), html.EscapeString(ws[0].acc)),
			"sub":  strings.Join(names, ", ")})
	}
	var accs int
	var total float64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(DISTINCT w.account_id), COALESCE(sum(w.value),0)::float8 FROM whitespace w WHERE w.status='peluang'
		AND EXISTS (SELECT 1 FROM opportunities o WHERE o.account_id=w.account_id AND o.status='open' AND NOT o.historical AND o.health IS NOT NULL)`).Scan(&accs, &total)
	ex := []string{}
	r, err = a.DB.Pool.Query(ctx, `SELECT w.product_line, a.name FROM whitespace w JOIN accounts a ON a.id=w.account_id WHERE w.status='peluang' AND w.product_line NOT ILIKE 'maintenance%'
		AND EXISTS (SELECT 1 FROM opportunities o WHERE o.account_id=w.account_id AND o.status='open' AND NOT o.historical AND o.health IS NOT NULL) ORDER BY w.value DESC LIMIT 3`)
	if err == nil {
		for r.Next() {
			var line, name string
			_ = r.Scan(&line, &name)
			ex = append(ex, line+" di "+shortAcc(name))
		}
		r.Close()
	}
	rows = append(rows, map[string]any{"icon": "i-trend", "tone": "accent", "html": fmt.Sprintf(`<b>Ruang ekspansi di %d akun aktif</b> · <b class="num">%s</b> lini produk yang belum kita isi`, accs, domain.FormatRp1(total)), "sub": strings.Join(ex, ", ")})
	var tn int
	var hps float64
	var nearest string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(hps),0)::float8 FROM tenders WHERE match_score >= 70 AND status='new'`).Scan(&tn, &hps)
	var nd *time.Time
	_ = a.DB.Pool.QueryRow(ctx, `SELECT title, deadline FROM tenders WHERE match_score >= 70 AND status='new' AND deadline >= $1 ORDER BY deadline LIMIT 1`, domain.StartOfDay(now)).Scan(&nearest, &nd)
	sub := ""
	if nd != nil {
		sub = fmt.Sprintf("Terdekat: %s, tutup %s", strings.ReplaceAll(strings.ReplaceAll(nearest, "Addressable — ", ""), "Pengadaan ", ""), domain.ShortDate(*nd))
	}
	rows = append(rows, map[string]any{"icon": "i-radar", "tone": "indigo", "html": fmt.Sprintf(`<b>%d tender cocok</b> ditemukan di LPSE/e-katalog · HPS total <b class="num">%s</b>`, tn, domain.FormatRp1(hps)), "sub": sub})
	return rows
}

func shortAcc(n string) string {
	r := strings.NewReplacer(" Semarang", "", " Daerah", "", " Surabaya", "", "Diskominfo Kota", "Diskominfo")
	return r.Replace(n)
}
