package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/storage"
)

func whenLabel(t time.Time) string {
	switch domain.DaysBetween(t, domain.Now()) {
	case 0:
		return "Hari ini " + domain.ClockID(t)
	case 1:
		return "Kemarin " + domain.ClockID(t)
	}
	return domain.ShortDate(t) + " " + domain.ClockID(t)
}

func scoreTone(status string, score *int) string {
	switch {
	case status == "not_prospect":
		return "bad"
	case score == nil:
		return "neutral"
	case *score >= 60:
		return "good"
	case *score >= 30:
		return "warn"
	}
	return "bad"
}

type inbound struct {
	ID, Phone, First, Via, Status, Overview, Role, Name, Company string
	Score                                                        *int
	Received                                                     time.Time
	Sources, Solutions, Questions                                []map[string]any
}

func (a *App) loadInbound(ctx context.Context, id string) ([]inbound, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT i.id, i.phone, i.first_message, COALESCE(u.name,''), i.status, i.overview, i.identification, i.fit_score, i.received_at, i.solutions, i.pain_questions
		FROM inbound_contacts i LEFT JOIN users u ON u.id=i.via_user_id WHERE ($1='' OR i.id=$1) AND ($1<>'' OR i.received_at >= $2) ORDER BY i.received_at DESC`, id, domain.Now().AddDate(0, 0, -7))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []inbound
	for rows.Next() {
		var x inbound
		var ident, sol, qs []byte
		if err := rows.Scan(&x.ID, &x.Phone, &x.First, &x.Via, &x.Status, &x.Overview, &ident, &x.Score, &x.Received, &sol, &qs); err != nil {
			return nil, err
		}
		var idm map[string]any
		_ = json.Unmarshal(ident, &idm)
		x.Name, _ = idm["name"].(string)
		x.Role, _ = idm["role"].(string)
		x.Company, _ = idm["company"].(string)
		if b, err := json.Marshal(idm["sources"]); err == nil {
			_ = json.Unmarshal(b, &x.Sources)
		}
		_ = json.Unmarshal(sol, &x.Solutions)
		_ = json.Unmarshal(qs, &x.Questions)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (x inbound) listView() map[string]any {
	return map[string]any{"id": x.ID, "score": x.Score, "status": x.Status, "tone": scoreTone(x.Status, x.Score), "name": defaultStr(x.Name, "Belum teridentifikasi"),
		"company": defaultStr(x.Company, "—"), "no": insights.MaskPhone(x.Phone), "via": firstName(x.Via), "first": x.First, "when": whenLabel(x.Received), "lead": x.Status == "lead"}
}

func (x inbound) detailView() map[string]any {
	v := x.listView()
	v["role"] = defaultStr(x.Role, "—")
	sources := []map[string]any{}
	for _, s := range x.Sources {
		sources = append(sources, map[string]any{"s": s["s"], "c": s["c"], "v": s["v"]})
	}
	v["sources"] = sources
	v["overview"] = x.Overview
	sols := []map[string]any{}
	req, extra := 0.0, 0.0
	for i, s := range x.Solutions {
		val := toFloat(s["v"])
		k := fmt.Sprint(s["k"])
		if i == 0 {
			req = val
		}
		if k == "peluang" {
			extra += val
		}
		sols = append(sols, map[string]any{"t": s["t"], "v": val, "k": k, "why": s["why"]})
	}
	qs := []map[string]any{}
	for _, q := range x.Questions {
		qs = append(qs, map[string]any{"q": q["q"], "u": q["u"]})
	}
	v["solutions"], v["questions"], v["requested"], v["extra"] = sols, qs, req, extra
	return v
}

func (a *App) handleProspects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stages, start, err := a.Ins.Funnel(ctx, r.URL.Query().Get("month"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	fun := []map[string]any{}
	first := 1
	if len(stages) > 0 && stages[0].N > 0 {
		first = stages[0].N
	}
	for i, s := range stages {
		conv, tone := "", ""
		if i > 0 && stages[i-1].N > 0 {
			c := float64(s.N) / float64(stages[i-1].N)
			conv = pct0(c)
			tone = "good"
			if c >= 0.4 && c < 0.6 {
				tone = "warn"
			}
		}
		width := math.Round(float64(s.N)/float64(first)*1000) / 10
		fun = append(fun, map[string]any{"label": s.Label, "sub": s.Sub, "n": s.N, "width": width, "conv": conv, "conv_tone": tone, "soft": i == 0, "won": i == len(stages)-1})
	}
	identMin, leadDays := a.Ins.FunnelTiming(ctx, start)
	var base map[string]float64
	a.Ins.Setting(ctx, "funnel_baseline", &base)
	var avgLead float64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(avg(expected_revenue),0)::float8 FROM opportunities WHERE NOT historical AND lead_at >= $1 AND lead_at < $2`, start, start.AddDate(0, 1, 0)).Scan(&avgLead)
	mult := 0.0
	if base["avg_lead_value"] > 0 {
		mult = avgLead / base["avg_lead_value"]
	}
	metaCards := []map[string]string{
		{"label": "Masuk → teridentifikasi", "value": fmt.Sprintf("%d menit", int(math.Round(identMin))), "sub": "sebelumnya " + humanMinutes(base["identify_minutes"]) + " (manual)"},
		{"label": "Teridentifikasi → lead", "value": strings.ReplaceAll(fmt.Sprintf("%.1f hari", leadDays), ".", ","), "sub": fmt.Sprintf("sebelumnya %d hari", int(base["ident_to_lead_days"]))},
		{"label": "Nilai rata-rata lead", "value": fmtRp(avgLead), "sub": strings.ReplaceAll(fmt.Sprintf("+%.1f× karena solusi lengkap", mult), ".", ",")},
	}
	src, total, _ := a.Ins.Sources(ctx)
	srows := []map[string]any{}
	for _, s := range src {
		tone := ""
		switch {
		case s.WinRate >= 0.5:
			tone = "good"
		case s.WinRate < 0.15:
			tone = "bad"
		}
		srows = append(srows, map[string]any{"label": s.Label, "n": s.N, "win": int(math.Round(s.WinRate * 100)), "tone": tone})
	}
	fw, _ := a.Ins.ComputeFlywheel(ctx)
	note := fmt.Sprintf("%d%% deal datang dari pelanggan yang sudah ada, dengan win rate %s–%s× lebih tinggi dari sumber lain. Ini alasan utama pertanyaan flywheel di bawah.",
		int(math.Round(fw.ExistingShare*100)), trimF(fw.Multiplier[0]), trimF(fw.Multiplier[1]))
	list, _ := a.loadInbound(ctx, "")
	inb := []map[string]any{}
	for _, x := range list {
		inb = append(inb, x.listView())
	}
	writeJSON(w, 200, map[string]any{
		"funnel":  map[string]any{"meta": fmt.Sprintf("%s · dari nomor/kontak masuk sampai Won", domain.MonthLong(start.Month())), "stages": fun, "meta_cards": metaCards},
		"sources": map[string]any{"meta": fmt.Sprintf("24 bulan · %d deal", total), "rows": srows, "note": note},
		"inbound": inb,
		"flywheel": map[string]any{"meta": fmt.Sprintf("Dari %d deal, 24 bulan", fw.Total), "center": fmt.Sprintf("%d%%", int(math.Round(fw.ExistingShare*100))),
			"metrics": []map[string]string{
				{"label": "Referral rate", "value": pct0(fw.ReferralRate), "sub": "pelanggan yang mereferensikan dalam 12 bln"},
				{"label": "Expansion rate", "value": pct0(fw.ExpansionRate), "sub": "akun yang beli lini kedua"},
				{"label": "Time-to-delight", "value": fmt.Sprintf("%d hr", int(fw.TimeToDelight)), "sub": fmt.Sprintf("Won → BAST · target %d", int(fw.TTDTarget))},
			}, "verdict_html": verdictHTML(fw, src)},
	})
}

func trimF(v float64) string {
	if v >= 1.95 {
		return fmt.Sprintf("%d", int(math.Round(v)))
	}
	return strings.ReplaceAll(fmt.Sprintf("%.1f", v), ".", ",")
}

func humanMinutes(m float64) string {
	switch {
	case m >= 1440:
		return fmt.Sprintf("%d hari", int(math.Round(m/1440)))
	case m >= 60:
		return fmt.Sprintf("%d jam", int(math.Round(m/60)))
	}
	return fmt.Sprintf("%d menit", int(m))
}

// verdictHTML is computed from the data: existing-customer win rate vs others.
func verdictHTML(fw insights.Flywheel, src []insights.SourceRow) string {
	rate := map[string]int{}
	for _, s := range src {
		rate[s.Source] = int(math.Round(s.WinRate * 100))
	}
	if !fw.UseFlywheel {
		return fmt.Sprintf("<b>Belum — fokus pada funnel dulu.</b> Win rate pelanggan lama (%d%%) belum jauh di atas sumber lain (%d%%), jadi energi pertumbuhan belum datang dari pelanggan yang puas. Perkuat kualifikasi prospek baru dan ukur ulang setiap kuartal.",
			int(math.Round(fw.WinExisting*100)), int(math.Round(fw.WinOthers*100)))
	}
	return fmt.Sprintf("<b>Ya, pakai flywheel — tapi sebagai model operasi, bukan pengganti funnel.</b> Funnel di atas tetap cara mengukur prospek baru. Flywheel menjelaskan dari mana energinya: sumber dengan win rate tertinggi (ekspansi %d%%, referral %d%%) hanya muncul kalau tahap <b>Puaskan</b> dikerjakan — BAST cepat, maintenance ditawarkan, pembayaran mudah. Friksi terbesar saat ini ada di situ: Won → BAST %d hari, dan hanya %d%% pelanggan yang pernah kita minta referensi. Tiga tindakan yang memutar roda: minta referensi otomatis H+3 setelah BAST (sudah ada di Kas), tawarkan maintenance saat garansi habis (Renewal radar), dan jadikan setiap nomor masuk pintu ke <em>solusi lengkap</em>, bukan satu produk.",
		rate["ekspansi"], rate["referral"], int(fw.TimeToDelight), int(math.Round(fw.ReferralRate*100)))
}

func (a *App) handleFunnel(w http.ResponseWriter, r *http.Request) {
	stages, start, err := a.Ins.Funnel(r.Context(), r.URL.Query().Get("month"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []map[string]any{}
	for _, s := range stages {
		out = append(out, map[string]any{"stage": s.Key, "label": s.Label, "n": s.N})
	}
	writeJSON(w, 200, map[string]any{"month": start.Format("2006-01"), "stages": out})
}

func (a *App) handleProspect(w http.ResponseWriter, r *http.Request) {
	list, err := a.loadInbound(r.Context(), r.PathValue("id"))
	if err != nil || len(list) == 0 {
		writeErr(w, 404, "kontak tidak ditemukan")
		return
	}
	writeJSON(w, 200, list[0].detailView())
}

func (a *App) inboundDraft(ctx context.Context, p Principal, x inbound, text, title string) (string, error) {
	var session, jid, threadID string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT w.id FROM wa_sessions w JOIN users u ON u.id=w.user_id WHERE u.name=$1 LIMIT 1`, x.Via).Scan(&session)
	jid = domain.NormalizePhone(x.Phone) + "@s.whatsapp.net"
	_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM chat_threads WHERE session_id=$1 AND chat_jid=$2`, session, jid).Scan(&threadID)
	id, _, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Identity agent", Type: "send_wa", Kind: "send", Icon: "i-chat", ButtonLabel: "Kirim", Title: title,
		DueLabel: "Hari ini", Summary: "Balasan pertama ke nomor baru " + insights.MaskPhone(x.Phone) + " dari nomor " + firstName(x.Via) + ".", Why: "Nomor ini menghubungi kita lebih dulu: “" + x.First + "”",
		Prep: "Satu pertanyaan sopan untuk menggali kebutuhan.", Preview: text, PreviewFrom: "WhatsApp · dari nomor " + firstName(x.Via),
		Steps: []string{"Pesan dikirim dari nomor " + firstName(x.Via), "Jawaban diekstrak; identifikasi & solusi diperbarui otomatis"}, InQueue: true,
		Payload:  map[string]any{"session": session, "chat_jid": jid, "thread_id": threadID, "to": defaultStr(x.Name, x.Phone), "inbound_id": x.ID},
		Evidence: []domain.Evidence{{Source: "inbound:" + x.ID, Quote: x.First}}, Confidence: 0.85}, p.Actor())
	return id, err
}

func (a *App) handleProspectAct(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	list, err := a.loadInbound(ctx, r.PathValue("id"))
	if err != nil || len(list) == 0 {
		writeErr(w, 404, "kontak tidak ditemukan")
		return
	}
	x := list[0]
	var msg string
	switch r.PathValue("verb") {
	case "lead":
		company := defaultStr(strings.Trim(x.Company, "— "), x.Name)
		accID := "acc-" + storage.Hash(strings.ToLower(company))[:8]
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO accounts(id,name,sector,branch,owner_user_id) SELECT $1,$2,'Enterprise · Prospek',u.branch,u.id FROM users u WHERE u.name=$3 ON CONFLICT (id) DO NOTHING`, accID, company, x.Via)
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO people(id,name,role,account_id,phones,stakeholder_tag,strength,stakeholder_note) VALUES ($1,$2,$3,$4,$5,'champion',1,'kontak inbound') ON CONFLICT (id) DO NOTHING`,
			"p-"+storage.Hash(x.Phone)[:12], x.Name, x.Role, accID, []string{x.Phone})
		req := 0.0
		if len(x.Solutions) > 0 {
			req = toFloat(x.Solutions[0]["v"])
		}
		qs := []string{}
		for _, q := range x.Questions {
			qs = append(qs, fmt.Sprint(q["q"]))
		}
		name := "Permintaan " + company
		if len(x.Solutions) > 0 {
			name = fmt.Sprint(x.Solutions[0]["t"])
		}
		act, _, err := a.proposeAndApprove(ctx, p, actions.Proposal{Agent: "Identity agent", Type: "create_lead", Kind: "task", Icon: "i-check", AccountID: accID, Title: "Buat lead " + company,
			Why: x.Overview, Prep: "Konteks identifikasi dan pertanyaan pain point dilampirkan.", Steps: []string{"Lead dibuat (Baru)", "Sales ditugaskan"},
			Payload:  map[string]any{"account_id": accID, "name": name, "value": req, "source": "inbound", "inbound_id": x.ID, "note": "Pertanyaan: " + strings.Join(qs, " | ")},
			Evidence: []domain.Evidence{{Source: "inbound:" + x.ID, Quote: x.First}}, Confidence: 0.9})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		dest := "ARC"
		if oppID, ok := act.Payload["opportunity_id"]; ok {
			_ = oppID
		}
		var oppID string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(opportunity_id,'') FROM inbound_contacts WHERE id=$1`, x.ID).Scan(&oppID)
		if oppID != "" {
			if err := a.createInOdoo(ctx, actions.Action{ID: act.ID, Payload: map[string]any{"opportunity_id": oppID}}); err == nil {
				dest = "Odoo"
			}
		}
		msg = fmt.Sprintf("Lead “%s” dibuat di %s (Baru) · konteks & pertanyaan dilampirkan", company, dest)
	case "reply-first":
		q := "Terima kasih sudah menghubungi kami. Boleh tahu kebutuhannya untuk gedung apa?"
		if len(x.Questions) > 0 {
			q = fmt.Sprint(x.Questions[0]["q"])
		}
		if _, err := a.inboundDraft(ctx, p, x, q, "Balas pertanyaan pertama ke "+defaultStr(x.Name, "nomor baru")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		msg = "Pertanyaan pertama masuk draft WhatsApp " + firstName(x.Via) + " · menunggu approve"
	case "question":
		var req struct {
			Index int `json:"index"`
		}
		_ = decode(r, &req)
		if req.Index < 0 || req.Index >= len(x.Questions) {
			writeErr(w, 400, "indeks pertanyaan tidak valid")
			return
		}
		if _, err := a.inboundDraft(ctx, p, x, fmt.Sprint(x.Questions[req.Index]["q"]), fmt.Sprintf("Tanyakan pain point %d ke %s", req.Index+1, defaultStr(x.Name, "nomor baru"))); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		msg = fmt.Sprintf("Pertanyaan %d dikirim ke draft WhatsApp %s", req.Index+1, firstName(x.Via))
	case "talking-points":
		var email string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT email FROM users WHERE name=$1`, x.Via).Scan(&email)
		body := x.Overview + "\n\nPertanyaan:\n"
		for i, q := range x.Questions {
			body += fmt.Sprintf("%d. %v\n", i+1, q["q"])
		}
		a.notifyUser(ctx, strings.ToLower(x.Via), "Talking points: "+defaultStr(x.Name, x.Phone), body)
		msg = "Talking points dikirim ke " + firstName(x.Via) + " · jawaban akan diekstrak otomatis dari chat"
	case "not-prospect":
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE inbound_contacts SET status='not_prospect', fit_score=5, updated_at=now() WHERE id=$1`, x.ID)
		_, _ = a.DB.Pool.Exec(ctx, `DELETE FROM funnel_events WHERE inbound_id=$1 AND stage NOT IN ('masuk','teridentifikasi')`, x.ID)
		msg = "Ditandai bukan prospek · tidak masuk funnel"
	case "undo":
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE inbound_contacts SET status='unknown', fit_score=NULL, updated_at=now() WHERE id=$1`, x.ID)
		go func() {
			cctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_, _ = a.Agents.IdentifyInbound(cctx, x.ID)
		}()
		msg = "Penandaan dibatalkan · identifikasi diulang"
	case "getcontact":
		var req struct {
			Text string `json:"text"`
		}
		if err := decode(r, &req); err != nil {
			writeErr(w, 400, "format tidak valid")
			return
		}
		if err := a.Agents.ImportGetcontact(ctx, x.ID, req.Text); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		msg = "Tag Getcontact diimpor manual · identifikasi diulang"
	default:
		writeErr(w, 404, "aksi tidak dikenal")
		return
	}
	_ = storage.Audit(ctx, a.DB.Pool, p.Actor(), "prospect."+r.PathValue("verb"), "inbound_contact", x.ID, nil)
	list, _ = a.loadInbound(ctx, x.ID)
	var item any
	if len(list) > 0 {
		item = list[0].detailView()
	}
	writeJSON(w, 200, map[string]any{"toast": msg, "item": item})
}

// ---------------- Kas ----------------

var stageLabel = domain.L2CStageLabels

func (a *App) handleCash(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := domain.Now()
	pulse, _ := a.Ins.Pulse(ctx)
	pv := map[string]insights.PulseRow{}
	for _, p := range pulse {
		pv[p.Key] = p
	}
	items, total, err := a.Ins.CashForecast(ctx, "")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	buckets, agingTotal, nInv, _ := a.Ins.Aging(ctx)
	overdue := buckets[1].Amount + buckets[2].Amount + buckets[3].Amount
	overN := buckets[1].Count + buckets[2].Count + buckets[3].Count
	l2cTarget := a.Ins.PolicyFloat(ctx, "lead_to_cash_target_days", 60)
	kpis := []map[string]any{
		{"label": "Lead → cash (median)", "value": strings.TrimSuffix(pv["lead_to_cash"].Value, " hr"), "unit": "hari", "delta": fmt.Sprintf("%s vs H1 · target %d", arrowDays(pv["lead_to_cash"].Delta), int(l2cTarget)), "tone": pv["lead_to_cash"].Tone},
		{"label": "DSO piutang", "value": strings.TrimSuffix(pv["dso"].Value, " hr"), "unit": "hari", "delta": arrowDays(pv["dso"].Delta) + " · termin rata-rata 30", "tone": pv["dso"].Tone},
		{"label": "Piutang jatuh tempo", "value": insights.FormatM(overdue), "delta": fmt.Sprintf("%d invoice · %d di atas 60 hari", overN, buckets[3].Count), "tone": "bad"},
		{"label": "Kas masuk 30 hari (prediksi)", "value": domain.FormatRp1(total), "delta": "tertimbang pola bayar tiap akun", "tone": "n"},
	}
	l2c, _ := a.Ins.L2C(ctx)
	rows := []map[string]any{}
	for _, it := range l2c {
		idx := domain.L2CIndex(it.Stage)
		rows = append(rows, map[string]any{"id": it.ID, "account": it.Account, "sub": fmt.Sprintf("%s · %s · %s · %s", it.Project, it.SO, fmtRp(it.Value), it.Owner),
			"stage": idx, "stage_label": stageLabel[it.Stage], "days": it.Days, "bench": it.Bench, "tone": insights.L2CTone(it), "note": it.Note, "action": a.actionViewPtr(ctx, it.ActionID)})
	}
	colors := []string{"var(--accent-line)", "var(--warn)", "var(--warn)", "var(--bad)"}
	aging := []map[string]any{}
	for i, b := range buckets {
		sub := "—"
		if b.Count > 0 {
			sub = fmt.Sprintf("%d invoice", b.Count)
			if b.Gov == b.Count && i == 3 {
				sub += " · pemerintah"
			}
		}
		width := 0.0
		if agingTotal > 0 {
			width = math.Round(b.Amount / agingTotal * 100)
		}
		aging = append(aging, map[string]any{"label": b.Label, "width": width, "color": colors[i], "value": insights.FormatM(b.Amount), "sub": sub})
	}
	patterns, _ := a.Ins.PayPatterns(ctx)
	collection := []map[string]any{}
	list, _ := a.Actions.List(ctx, actions.Filter{Status: "proposed", Agent: "Collection agent"})
	invs, _ := a.Ins.OpenInvoices(ctx, "project")
	for _, act := range list {
		if act.Type != "payment_reminder" && act.Type != "ask_spm_documents" {
			continue
		}
		var inv *insights.Invoice
		for i := range invs {
			if invs[i].AccountID == act.AccountID && (inv == nil || invs[i].DueDate.Before(inv.DueDate)) {
				inv = &invs[i]
			}
		}
		if inv == nil {
			continue
		}
		late := domain.DaysBetween(inv.DueDate, now)
		badge := map[string]string{"k": "warn", "t": fmt.Sprintf("H+%d", late)}
		if late > 30 {
			badge["k"] = "bad"
		}
		pat := patterns[inv.AccountID]
		pl := fmt.Sprintf("pola bayar %d hari", pat.MedianDays)
		switch pat.Kind {
		case "tepat":
			pl = fmt.Sprintf("pelanggan baik (pola %d hari)", pat.MedianDays)
		case "termin_anggaran":
			pl = "termin anggaran"
		}
		collection = append(collection, map[string]any{"late": late, "badge": badge, "t": fmt.Sprintf("%s · %s · %s", inv.Account, insights.FormatM(inv.Residual), pl), "s": act.Summary, "action": a.actionView(act)})
	}
	sort.SliceStable(collection, func(i, j int) bool {
		return collection[i]["late"].(int) < collection[j]["late"].(int)
	})
	for _, c := range collection {
		delete(c, "late")
	}
	fitems := []map[string]any{}
	bestInvoice, bestDelta := "", 0.0
	for _, it := range items {
		fitems = append(fitems, map[string]any{"t": it.Label, "s": it.Detail, "p": it.P, "v": it.Value})
		if it.CashItem != "" && it.P < 0.5 {
			if _, t2, err := a.Ins.CashForecast(ctx, "create_invoice:"+it.CashItem); err == nil && t2-total > bestDelta {
				bestInvoice, bestDelta = it.Label, t2-total
			}
		}
	}
	reminder := ""
	for _, c := range collection {
		if strings.Contains(fmt.Sprint(c["s"]), "ramah") {
			reminder = strings.Split(fmt.Sprint(c["t"]), " · ")[0]
		}
	}
	note := "Angka ini yang dipakai <b>Purchasing</b> untuk budget pembelian mingguan — bukan saldo hari ini."
	if bestInvoice != "" {
		note += fmt.Sprintf(" Dua tindakan di kiri menaikkan prediksi paling besar: invoice %s (+%s)", strings.TrimPrefix(bestInvoice, "RS "), fmtRp(bestDelta))
		if reminder != "" {
			note += " dan pengingat " + strings.TrimPrefix(strings.TrimSuffix(reminder, " Tuban"), "PT ")
		}
		note += "."
	}
	writeJSON(w, 200, map[string]any{"kpis": kpis, "l2c": rows,
		"aging":      map[string]any{"meta": fmt.Sprintf("Total %s · %d invoice", domain.FormatRp1(agingTotal), nInv), "rows": aging},
		"collection": collection, "forecast": map[string]any{"total": total, "items": fitems, "note_html": note}})
}

func arrowDays(delta string) string {
	if strings.HasPrefix(delta, "−") {
		return "↓ " + strings.TrimSuffix(strings.TrimPrefix(delta, "−"), " hr") + " hari"
	}
	return "↑ " + strings.TrimSuffix(strings.TrimPrefix(delta, "+"), " hr") + " hari"
}

func (a *App) handleCashForecast(w http.ResponseWriter, r *http.Request) {
	whatIf := r.URL.Query().Get("what_if")
	_, base, err := a.Ins.CashForecast(r.Context(), "")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, alt, _ := a.Ins.CashForecast(r.Context(), whatIf)
	writeJSON(w, 200, map[string]any{"total": alt, "base": base, "delta": alt - base})
}

func (a *App) handleCashForecastCSV(w http.ResponseWriter, r *http.Request) {
	items, total, err := a.Ins.CashForecast(r.Context(), "")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=prediksi-kas-30-hari.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"sumber", "keterangan", "nilai", "probabilitas", "tertimbang"})
	for _, it := range items {
		_ = cw.Write([]string{it.Label, it.Detail, fmt.Sprintf("%.0f", it.Amount), fmt.Sprintf("%.2f", it.P), fmt.Sprintf("%.0f", it.Value)})
	}
	_ = cw.Write([]string{"TOTAL", "", "", "", fmt.Sprintf("%.0f", total)})
	cw.Flush()
}
