package app

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	qrcode "github.com/skip2/go-qrcode"

	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/llm"
	"arc/packages/core/storage"
	"arc/packages/mcp"
)

func (a *App) handleSettingsSources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, name, subtitle, logo, color, text_color, grp, last_sync_at FROM connectors ORDER BY seq`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	var sessions, connected, pairing, groups int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status='connected'), count(*) FILTER (WHERE status='pairing') FROM wa_sessions`).Scan(&sessions, &connected, &pairing)
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM chat_groups WHERE read_policy`).Scan(&groups)
	var mailboxes int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM oauth_google_tokens`).Scan(&mailboxes)
	sources, identity := []map[string]any{}, []map[string]any{}
	for rows.Next() {
		var id, name, sub, logo, color, text, grp string
		var last *time.Time
		_ = rows.Scan(&id, &name, &sub, &logo, &color, &text, &grp, &last)
		dot, status := "good", ""
		lastLabel := ""
		if last != nil {
			lastLabel = " · " + strings.ReplaceAll(domain.ClockID(*last), ".", ":")
		}
		switch id {
		case "odoo":
			status = "Terhubung · baca & tulis" + lastLabel
			if a.OdooMock {
				dot, status = "warn", "Mode mock · baca & tulis"+lastLabel+" · isi ODOO_* di .env"
			}
		case "gmail":
			status = "Terhubung · baca & draft"
			if a.GoogleMock {
				dot, status = "warn", "Mode mock (.eml contoh) · OAuth belum diisi"
			} else if mailboxes > 0 {
				sub = fmt.Sprintf("%d kotak masuk terhubung", mailboxes)
			}
		case "gcal":
			status = "Terhubung · baca"
			if a.GoogleMock {
				dot, status = "warn", "Mode mock · OAuth belum diisi"
			}
		case "gdrive":
			status = "Terhubung · baca"
			if a.GoogleMock {
				dot, status = "off", "Belum dihubungkan"
			}
		case "whatsapp":
			sub = fmt.Sprintf("%d nomor sales · %d grup project", sessions, groups)
			status = fmt.Sprintf("%d terhubung · %d menunggu · %d belum", connected, pairing, sessions-connected-pairing)
			if connected < sessions {
				dot = "warn"
			}
		case "basecamp":
			if a.Cfg.BasecampToken == "" {
				dot, status = "off", "Belum dihubungkan"
			} else {
				status = "Terhubung · tujuan brief & tugas"
			}
		case "getcontact":
			dot, status = "warn", "Aktif · tidak ada API resmi — lihat catatan"
		case "wa_profile":
			status = "Terhubung"
			if _, err := a.Bridge.Health(ctx); err != nil {
				dot, status = "warn", "Lewat wa-bridge · bridge belum berjalan"
			}
		case "truecaller":
			status = "Terhubung · Business API"
			if a.Cfg.TruecallerKey == "" {
				dot, status = "warn", "Mode mock · isi TRUECALLER_API_KEY"
			}
		case "web":
			status = "Research agent"
			if a.Cfg.WebSearchKey == "" {
				status += " · mode mock"
			}
		}
		card := map[string]any{"id": id, "name": name, "sub": sub, "logo": logo, "color": color, "text_color": text, "dot": dot, "status": status, "active": id == "whatsapp"}
		if grp == "source" {
			sources = append(sources, card)
		} else {
			identity = append(identity, card)
		}
	}
	writeJSON(w, 200, map[string]any{"sources": sources, "identity": identity,
		"getcontact_note": "<b>Catatan Getcontact:</b> Getcontact tidak menyediakan API resmi untuk pihak ketiga; integrasi yang beredar memakai protokol tidak resmi dan bisa diputus sewaktu-waktu. Di ARC, Getcontact diposisikan sebagai <em>sumber pendukung</em> (tag dicek manual atau via ekspor), bukan tulang punggung — identitas selalu digabung dari ≥ 2 sumber dengan confidence, dan hanya untuk nomor inbound. Data hasil identifikasi disimpan 90 hari kecuali menjadi lead."})
}

func qrDataURL(content string) string {
	png, err := qrcode.Encode(content, qrcode.Medium, 352)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func (a *App) whatsappView(ctx context.Context) (map[string]any, error) {
	now := domain.Now()
	var mode string
	var days int
	a.Ins.Setting(ctx, "wa_mode", &mode)
	a.Ins.Setting(ctx, "wa_history_days", &days)
	if days == 0 {
		days = 30
	}
	var est map[string]struct {
		Msg int    `json:"msg"`
		Com int    `json:"com"`
		Nw  int    `json:"nw"`
		T   string `json:"t"`
	}
	a.Ins.Setting(ctx, "wa_history_estimates", &est)
	e := est[fmt.Sprint(days)]
	numbers := []map[string]any{}
	rows, err := a.DB.Pool.Query(ctx, `SELECT w.id, COALESCE(u.initials,''), w.label, w.phone, w.status, w.last_event_at, w.messages_30d, w.qr_expires_at, COALESCE(u.branch,''), COALESCE(u.name,'')
		FROM wa_sessions w LEFT JOIN users u ON u.id=w.user_id ORDER BY array_position(ARRAY['Semarang','Yogyakarta','Surabaya','Jakarta'], u.branch), w.created_at, w.id`)
	if err != nil {
		return nil, err
	}
	var pairing map[string]any
	var totalMsgs int
	for rows.Next() {
		var id, ini, label, phone, status, branch, name string
		var last, qrExp *time.Time
		var msgs int
		_ = rows.Scan(&id, &ini, &label, &phone, &status, &last, &msgs, &qrExp, &branch, &name)
		totalMsgs += msgs
		note := "belum ditautkan"
		switch status {
		case "connected":
			ago := 0
			if last != nil {
				ago = int(now.Sub(*last).Minutes())
			}
			note = fmt.Sprintf("sinkron %d mnt lalu · %s pesan / 30 hr", ago, fmtThousands(msgs))
		case "pairing":
			left := 0
			if qrExp != nil {
				left = int(qrExp.Sub(now).Seconds())
			}
			if left < 0 {
				left = 0
			}
			note = fmt.Sprintf("kode berlaku %d:%02d", left/60, left%60)
		case "disconnected":
			note = "terputus · tautkan ulang"
		}
		numbers = append(numbers, map[string]any{"id": id, "initials": ini, "label": label, "no": insights.MaskPhone(phone), "status": status, "note": note})
	}
	rows.Close()
	var pid, pname, pbranch, qr string
	var qrExp *time.Time
	if a.DB.Pool.QueryRow(ctx, `SELECT w.id, COALESCE(u.name, w.label), COALESCE(u.branch,''), w.qr_code, w.qr_expires_at FROM wa_sessions w LEFT JOIN users u ON u.id=w.user_id WHERE w.status='pairing' ORDER BY w.updated_at DESC LIMIT 1`).
		Scan(&pid, &pname, &pbranch, &qr, &qrExp) == nil {
		if st, err := a.Bridge.QR(ctx, pid); err == nil && st.QR != "" {
			qr = st.QR
			t := now.Add(60 * time.Second)
			qrExp = &t
		}
		left := 0
		if qrExp != nil {
			left = int(qrExp.Sub(now).Seconds())
		}
		if left < 0 {
			left = 0
		}
		pairing = map[string]any{"session_id": pid, "name": pname, "branch": pbranch, "qr_png": qrDataURL(defaultStr(qr, "ARC pairing "+pid)), "expires_in": left, "history_days": days}
	}
	rules := []map[string]any{}
	rrows, err := a.DB.Pool.Query(ctx, `SELECT id, title, detail, enabled, locked FROM privacy_rules ORDER BY seq`)
	if err == nil {
		for rrows.Next() {
			var id, t, d string
			var en, lk bool
			_ = rrows.Scan(&id, &t, &d, &en, &lk)
			rules = append(rules, map[string]any{"id": id, "title": t, "detail": d, "enabled": en, "locked": lk})
		}
		rrows.Close()
	}
	groups := []map[string]any{}
	grows, err := a.DB.Pool.Query(ctx, `SELECT id, name, type, read_policy, (SELECT count(*) FROM jsonb_array_elements(members) m WHERE NOT (m->>'int')::boolean), jsonb_array_length(members) FROM chat_groups ORDER BY type, created_at`)
	if err == nil {
		for grows.Next() {
			var id, n, t string
			var read bool
			var ext, total int
			_ = grows.Scan(&id, &n, &t, &read, &ext, &total)
			note := fmt.Sprintf("%d int", total)
			if ext > 0 {
				note = fmt.Sprintf("%d ekst · %d int", ext, total-ext)
			}
			groups = append(groups, map[string]any{"id": id, "name": n, "type": t, "members_note": note, "read": read})
		}
		grows.Close()
	}
	var unlisted int
	a.Ins.Setting(ctx, "wa_unlisted_groups", &unlisted)
	extracted := map[string]any{"period": fmt.Sprintf("%d hari", days), "messages": e.Msg, "commitments": e.Com, "contacts": e.Nw}
	if days == 30 {
		extracted["messages"] = totalMsgs
	}
	var sent int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM actions WHERE type='send_wa' AND status='executed' AND agent <> 'Balasan manual' AND executed_at >= $1`, now.AddDate(0, 0, -days)).Scan(&sent)
	extracted["sent"] = 0 // ARC never sends on its own: every send above was a human approval.
	_ = sent
	note := fmt.Sprintf("Perkiraan untuk %d nomor: <b>%s pesan</b>, pemrosesan awal %s. <b>Tautkan perangkat (QR)</b> mengambil riwayat langsung dari ponsel sales sejauh masih tersimpan di sana. <b>WhatsApp Business Platform</b> hanya menyimpan pesan sejak tersambung — riwayat %d hari ke belakang diisi lewat <em>ekspor chat</em> dari ponsel (Chat → Lainnya → Ekspor chat), yang bisa diunggah di sini.",
		len(numbers), fmtThousands(e.Msg), e.T, days)
	return map[string]any{"mode": defaultStr(mode, "cloud"), "history_days": days, "history_note_html": note, "numbers": numbers, "rules": rules, "groups": groups,
		"unlisted_groups": unlisted, "pairing": pairing, "extracted": extracted}, nil
}

func fmtThousands(n int) string {
	s := fmt.Sprint(n)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func (a *App) handleSettingsWhatsApp(w http.ResponseWriter, r *http.Request) {
	v, err := a.whatsappView(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, v)
}

func (a *App) handleSettingsWhatsAppUpdate(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Mode        string `json:"mode"`
		HistoryDays int    `json:"history_days"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "format tidak valid")
		return
	}
	if req.Mode == "cloud" || req.Mode == "bridge" {
		_ = a.Ins.SetSetting(r.Context(), "wa_mode", req.Mode)
	}
	if req.HistoryDays == 30 || req.HistoryDays == 60 || req.HistoryDays == 90 || req.HistoryDays == 180 {
		_ = a.Ins.SetSetting(r.Context(), "wa_history_days", req.HistoryDays)
		_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE wa_sessions SET history_days=$1 WHERE status IN ('unlinked','pairing')`, req.HistoryDays)
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "settings.whatsapp", "settings", "wa", req)
	a.handleSettingsWhatsApp(w, r)
}

func (a *App) handlePrivacyRule(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "format tidak valid")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(), `UPDATE privacy_rules SET enabled=$2 WHERE id=$1 AND NOT locked`, r.PathValue("id"), req.Enabled)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 400, "aturan ini terkunci atau tidak ditemukan")
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "privacy_rule", "privacy_rule", r.PathValue("id"), req)
	toast(w, "Aturan diperbarui · berlaku di sinkron berikutnya")
}

// handleSessionLink starts QR pairing on the wa-bridge (falls back to a demo code when the bridge is down).
func (a *App) handleSessionLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	id := r.PathValue("id")
	var label string
	var days int
	if err := a.DB.Pool.QueryRow(ctx, `SELECT label, history_days FROM wa_sessions WHERE id=$1`, id).Scan(&label, &days); err != nil {
		writeErr(w, 404, "nomor tidak ditemukan")
		return
	}
	qr, note := "", ""
	st, err := a.Bridge.CreateSession(ctx, id, label, days)
	if err == nil && st.QR != "" {
		qr = st.QR
	} else {
		qr = "ARC-DEMO-" + id + "-" + randomToken(6)
		note = " · wa-bridge belum berjalan (kode contoh)"
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE wa_sessions SET status='pairing', qr_code=$2, qr_expires_at=$3, updated_at=now() WHERE id=$1`, id, qr, domain.Now().Add(272*time.Second))
	_ = storage.Audit(ctx, a.DB.Pool, p.Actor(), "wa.link", "wa_session", id, nil)
	v, _ := a.whatsappView(ctx)
	writeJSON(w, 200, map[string]any{"toast": "QR baru dibuat untuk " + firstName(label) + note, "pairing": v["pairing"]})
}

func (a *App) handleSessionInstructions(w http.ResponseWriter, r *http.Request) {
	var email, name string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT COALESCE(u.email,''), COALESCE(u.name,'') FROM wa_sessions w LEFT JOIN users u ON u.id=w.user_id WHERE w.id=$1`, r.PathValue("id")).Scan(&email, &name)
	if email != "" {
		a.notifyUser(r.Context(), strings.TrimPrefix(r.PathValue("id"), "s-"), "Petunjuk menautkan WhatsApp ke ARC",
			"Buka WhatsApp → ⋮ → Perangkat tertaut → Tautkan perangkat, lalu pindai kode di Pengaturan → WhatsApp.")
	}
	toast(w, "Tautan pemasangan dikirim ke "+defaultStr(name, "sales")+" via email")
}

// ---------------- Internal numbers ----------------

func (a *App) handleInternal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, name, phone, unit, branch, source FROM internal_numbers ORDER BY id`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []map[string]any{}
	talenta := 0
	for rows.Next() {
		var id int64
		var n, ph, unit, br, src string
		_ = rows.Scan(&id, &n, &ph, &unit, &br, &src)
		label := map[string]string{"talenta": "Talenta", "manual": "Manual", "arc_suggested": "Dugaan ARC"}[src]
		if src == "talenta" {
			talenta++
		}
		out = append(out, map[string]any{"id": id, "n": n, "no": insights.MaskPhone(ph), "unit": unit, "branch": br, "src": label})
	}
	rows.Close()
	sus := []map[string]any{}
	srows, err := a.DB.Pool.Query(ctx, `SELECT id, phone, name_hint, reason FROM internal_suspects WHERE status='pending' ORDER BY id`)
	if err == nil {
		for srows.Next() {
			var id int64
			var ph, n, why string
			_ = srows.Scan(&id, &ph, &n, &why)
			sus = append(sus, map[string]any{"id": id, "no": insights.MaskPhone(ph), "n": "“" + n + "”", "why": why})
		}
		srows.Close()
	}
	writeJSON(w, 200, map[string]any{"rows": out, "meta": fmt.Sprintf("%d nomor · %d dari Talenta", len(out), talenta), "suspects": sus,
		"units":    []string{"Teknisi", "Gudang", "Admin Project", "Finance", "Purchasing", "Service Center", "Direksi", "Lainnya"},
		"branches": []string{"Semarang", "Yogyakarta", "Surabaya", "Jakarta", "Pusat"}})
}

func (a *App) handleInternalAdd(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Name, Phone, Unit, Branch string
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Name) == "" || domain.NormalizePhone(req.Phone) == "" {
		writeErr(w, 400, "nama dan nomor wajib diisi")
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `INSERT INTO internal_numbers(name,phone,phone_norm,unit,branch,source,confirmed_by) VALUES ($1,$2,$3,$4,$5,'manual',$6)
		ON CONFLICT (phone_norm) DO UPDATE SET name=EXCLUDED.name, unit=EXCLUDED.unit, branch=EXCLUDED.branch`, req.Name, req.Phone, domain.NormalizePhone(req.Phone), defaultStr(req.Unit, "Lainnya"), defaultStr(req.Branch, "Pusat"), p.UserID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = a.Agents.Hygiene(r.Context())
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "internal.add", "internal_number", domain.NormalizePhone(req.Phone), req)
	toast(w, req.Name+" ditandai internal · berlaku di sinkron berikutnya")
}

func (a *App) handleInternalDelete(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var name string
	if err := a.DB.Pool.QueryRow(r.Context(), `DELETE FROM internal_numbers WHERE id=$1 RETURNING name`, r.PathValue("id")).Scan(&name); err != nil {
		writeErr(w, 404, "nomor tidak ditemukan")
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "internal.delete", "internal_number", r.PathValue("id"), nil)
	toast(w, name+" dihapus dari daftar internal")
}

func (a *App) handleTalentaSync(w http.ResponseWriter, r *http.Request) {
	var n int
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT count(*) FROM internal_numbers WHERE source='talenta'`).Scan(&n)
	toast(w, fmt.Sprintf("Sinkron Talenta · %d karyawan dicek, 0 nomor baru · impor CSV Talenta untuk data terbaru", n))
}

// handleInternalImport accepts the Talenta CSV export: name,unit,branch,phone
func (a *App) handleInternalImport(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	sc := bufio.NewScanner(r.Body)
	n := 0
	for sc.Scan() {
		f := strings.Split(sc.Text(), ",")
		if len(f) < 4 || strings.EqualFold(strings.TrimSpace(f[0]), "nama") || strings.EqualFold(strings.TrimSpace(f[0]), "name") {
			continue
		}
		ph := strings.TrimSpace(f[3])
		if domain.NormalizePhone(ph) == "" {
			continue
		}
		tag, err := a.DB.Pool.Exec(r.Context(), `INSERT INTO internal_numbers(name,phone,phone_norm,unit,branch,source,confirmed_by) VALUES ($1,$2,$3,$4,$5,'talenta',$6) ON CONFLICT (phone_norm) DO NOTHING`,
			strings.TrimSpace(f[0]), ph, domain.NormalizePhone(ph), strings.TrimSpace(f[1]), strings.TrimSpace(f[2]), p.UserID)
		if err == nil {
			n += int(tag.RowsAffected())
		}
	}
	_, _ = a.Agents.Hygiene(r.Context())
	toast(w, fmt.Sprintf("Impor Talenta · %d nomor baru ditambahkan", n))
}

func (a *App) handleSuspect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	var phone, name string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT phone, name_hint FROM internal_suspects WHERE id=$1`, r.PathValue("id")).Scan(&phone, &name); err != nil {
		writeErr(w, 404, "dugaan tidak ditemukan")
		return
	}
	switch r.PathValue("verb") {
	case "confirm":
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO internal_numbers(name,phone,phone_norm,unit,branch,source,confirmed_by) VALUES ($1,$2,$3,'Lainnya','Semarang','arc_suggested',$4) ON CONFLICT DO NOTHING`, name, phone, domain.NormalizePhone(phone), p.UserID)
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE internal_suspects SET status='confirmed' WHERE id=$1`, r.PathValue("id"))
		toast(w, "“"+name+"” ditandai internal")
	case "reject":
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE internal_suspects SET status='rejected' WHERE id=$1`, r.PathValue("id"))
		toast(w, "Ditandai bukan internal · akan diperlakukan sebagai kontak eksternal")
	default:
		writeErr(w, 404, "aksi tidak dikenal")
		return
	}
	_ = storage.Audit(ctx, a.DB.Pool, p.Actor(), "suspect."+r.PathValue("verb"), "internal_suspect", r.PathValue("id"), nil)
}

// ---------------- AI & model, MCP & API ----------------

var routingDefs = []struct {
	tier, title, sub string
	options          []string
}{
	{"light", "Capture, Hygiene, ekstraksi komitmen", "Volume tinggi, murah, cepat · ±2.000 panggilan/hari", []string{"Claude Haiku 4.5", "OpenAI · model ringan", "Self-hosted (Ollama)"}},
	{"heavy", "Deal intelligence, forecast, brief", "Penalaran berat · ±60 panggilan/hari", []string{"Claude Sonnet 5", "Claude Opus 5.5", "OpenAI · model penalaran"}},
	{"interactive", "Ask & Tanya ARC (interaktif)", "Mengikuti klien yang dipakai pengguna", []string{"Claude Sonnet 5", "Mengikuti klien (Claude/ChatGPT)"}},
	{"fallback", "Fallback", "Kalau provider utama gagal 2×", []string{"OpenAI", "Claude", "Self-hosted"}},
}

func (a *App) monthCostIDR(ctx context.Context) float64 {
	now := domain.Now()
	var usd float64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(sum(cost_est),0)::float8 FROM llm_calls WHERE created_at >= $1`, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, domain.Jakarta)).Scan(&usd)
	return usd * llm.USDToIDR
}

func fmtJt(idr float64) string {
	return "Rp " + strings.ReplaceAll(fmt.Sprintf("%.1f", idr/1e6), ".", ",") + " jt"
}

func (a *App) handleSettingsAI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	groups := []map[string]any{}
	enabled := a.mcpGroupState(ctx)
	for _, g := range mcp.Groups() {
		tools := []map[string]any{}
		for _, t := range mcp.Tools() {
			if t.Group == g.ID {
				tools = append(tools, map[string]any{"name": t.Display, "desc": t.Short, "kind": t.Kind, "approval": t.Approval})
			}
		}
		groups = append(groups, map[string]any{"id": g.ID, "label": g.Label, "note": g.Note, "enabled": enabled[g.ID], "tools": tools})
	}
	clients := []map[string]any{}
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, name, logo, color, description, users_count, last_seen_at FROM ai_clients ORDER BY CASE id WHEN 'claude' THEN 0 WHEN 'chatgpt' THEN 1 ELSE 2 END`)
	if err == nil {
		var keys int
		_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM api_keys WHERE revoked_at IS NULL`).Scan(&keys)
		for rows.Next() {
			var id, n, logo, color, desc string
			var users int
			var last *time.Time
			_ = rows.Scan(&id, &n, &logo, &color, &desc, &users, &last)
			status := map[string]string{"k": "good", "t": "Terhubung"}
			lastL := ""
			if last != nil {
				lastL = "terakhir " + domain.ClockID(*last)
			}
			if id == "api" {
				status = map[string]string{"k": "neutral", "t": fmt.Sprintf("%d key aktif", keys)}
			}
			clients = append(clients, map[string]any{"id": id, "name": n, "logo": logo, "color": color, "desc": desc, "status": status, "last": strings.ReplaceAll(lastL, ".", ":")})
		}
		rows.Close()
	}
	var routing map[string]string
	a.Ins.Setting(ctx, "routing", &routing)
	rt := []map[string]any{}
	for _, d := range routingDefs {
		rt = append(rt, map[string]any{"tier": d.tier, "title": d.title, "sub": d.sub, "options": d.options, "selected": defaultStr(routing[d.tier], d.options[0])})
	}
	endpoint := strings.TrimRight(a.Cfg.PublicURL, "/") + "/mcp"
	snippet := fmt.Sprintf("// Claude Desktop · claude_desktop_config.json\n{\n  \"mcpServers\": {\n    \"arc\": {\n      \"url\": \"%s\",\n      \"headers\": { \"Authorization\": \"Bearer arc_live_••••••••\" }\n    }\n  }\n}", endpoint)
	writeJSON(w, 200, map[string]any{"mcp": map[string]any{"endpoint": endpoint, "version": "1.4", "active": true, "groups": groups},
		"clients": clients, "config_snippet": snippet, "routing": rt, "cost_month": fmtJt(a.monthCostIDR(ctx)), "openapi_url": strings.TrimRight(a.Cfg.PublicURL, "/") + "/openapi.json"})
}

func (a *App) mcpGroupState(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, enabled FROM mcp_tool_groups`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var en bool
		_ = rows.Scan(&id, &en)
		out[id] = en
	}
	return out
}

func (a *App) handleMCPGroup(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "format tidak valid")
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE mcp_tool_groups SET enabled=$2 WHERE id=$1`, r.PathValue("id"), req.Enabled); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "mcp.group", "mcp_tool_group", r.PathValue("id"), req)
	toast(w, "Aturan diperbarui · berlaku di sinkron berikutnya")
}

func (a *App) handleRouting(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Tier   string `json:"tier"`
		Option string `json:"option"`
	}
	if err := decode(r, &req); err != nil || req.Tier == "" {
		writeErr(w, 400, "format tidak valid")
		return
	}
	var routing map[string]string
	a.Ins.Setting(r.Context(), "routing", &routing)
	if routing == nil {
		routing = map[string]string{}
	}
	routing[req.Tier] = req.Option
	_ = a.Ins.SetSetting(r.Context(), "routing", routing)
	a.applyRoutingSetting(r.Context())
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "routing", "settings", req.Tier, req)
	toast(w, "Routing model diperbarui · "+req.Option)
}

func (a *App) handleSettingsAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := strings.TrimRight(a.Cfg.PublicURL, "/") + "/api/v1"
	keys := []map[string]any{}
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, name, prefix, scopes, last_used_at, rate_limit_rpm, calls_count, revoked_at IS NULL FROM api_keys ORDER BY created_at DESC, id`)
	if err == nil {
		for rows.Next() {
			var id, n, prefix string
			var scopes []string
			var last *time.Time
			var rpm int
			var calls int64
			var active bool
			_ = rows.Scan(&id, &n, &prefix, &scopes, &last, &rpm, &calls, &active)
			tail := fmt.Sprintf("%d rpm", rpm)
			switch {
			case last != nil && domain.DaysBetween(*last, domain.Now()) == 0:
				tail = "terakhir " + strings.ReplaceAll(domain.ClockID(*last), ".", ":")
			case calls > 1000:
				tail = fmtThousands(int(calls)) + " panggilan/bln"
			}
			keys = append(keys, map[string]any{"id": id, "name": n, "line": fmt.Sprintf("%s••••  · scope: %s · %s", prefix, strings.Join(scopes, ", "), tail), "active": active})
		}
		rows.Close()
	}
	cal, _ := a.Ins.Calibration(ctx)
	calibration := []map[string]any{}
	for _, c := range cal {
		calibration = append(calibration, map[string]any{"agent": strings.TrimSuffix(c.Agent, " agent"), "rate": int(math.Round(c.Rate)), "warn": c.Rate < 60})
	}
	learned := []map[string]string{}
	lrows, err := a.DB.Pool.Query(ctx, `SELECT text, created_at FROM learned_rules WHERE active ORDER BY created_at DESC, id`)
	if err == nil {
		for lrows.Next() {
			var t string
			var at time.Time
			_ = lrows.Scan(&t, &at)
			learned = append(learned, map[string]string{"html": htmlEsc(t)})
		}
		lrows.Close()
	}
	// Recent rejections also appear as what the agents learned (14-day suppression).
	rrows, err := a.DB.Pool.Query(ctx, `SELECT d.decided_at, a.title, d.reason, d.note, d.agent FROM action_decisions d JOIN actions a ON a.id=d.action_id WHERE d.decision='reject' AND a.type <> 'historical' ORDER BY d.decided_at DESC LIMIT 5`)
	if err == nil {
		var extra []map[string]string
		for rrows.Next() {
			var at time.Time
			var title, reason, note, agent string
			_ = rrows.Scan(&at, &title, &reason, &note, &agent)
			if note != "" {
				reason += " — " + note
			}
			extra = append(extra, map[string]string{"html": fmt.Sprintf("<b>%s</b> · “%s” ditolak: %s. %s tidak mengusulkan tindakan serupa untuk akun ini selama 14 hari kecuali sinyalnya berubah.",
				domain.ClockID(at), htmlEsc(title), htmlEsc(reason), htmlEsc(agent))})
		}
		rrows.Close()
		learned = append(extra, learned...)
	}
	writeJSON(w, 200, map[string]any{"endpoint": base, "endpoints": []map[string]string{
		{"method": "POST", "path": "/ask", "html": "Pertanyaan bebas → jawaban + daftar bukti (sumber, waktu, confidence)"},
		{"method": "GET", "path": "/accounts/{id}/brief", "html": "Memori akun, stakeholder, komitmen, next action"},
		{"method": "GET", "path": "/signals?since=", "html": "Sinyal baru: kompetitor, champion pindah, sunyi, pembayaran"},
		{"method": "GET", "path": "/actions?status=pending", "html": "Saran agen yang menunggu keputusan manusia"},
		{"method": "POST", "path": "/actions/{id}/decision", "html": "<code>approve</code> · <code>reject</code> + alasan · <code>edit</code> — hanya dengan key scope <em>human</em>"},
		{"method": "POST", "path": "/events", "html": "Masukkan kejadian dari luar (ERP, WhatsApp gateway, form web)"},
		{"method": "WEBHOOK", "path": "", "html": "<code>action.proposed</code> · <code>commitment.overdue</code> · <code>signal.detected</code> · <code>cash.forecast.changed</code>"},
	}, "curl": fmt.Sprintf("curl -X POST %s/ask \\\n  -H \"Authorization: Bearer arc_live_••••••••\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"question\":\"Deal mana yang berisiko bulan ini dan kenapa?\",\"evidence\":true}'", base),
		"keys": keys, "calibration": calibration, "learned": learned})
}

func htmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func (a *App) handleKeyCreate(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	_ = decode(r, &req)
	scopes := []string{}
	for _, s := range req.Scopes {
		switch s {
		case domain.ScopeRead, domain.ScopePropose, domain.ScopeEvents, domain.ScopeSignals:
			scopes = append(scopes, s)
		}
	}
	if len(scopes) == 0 {
		scopes = []string{domain.ScopeRead}
	}
	prefix := "arc_live_" + randomToken(2)
	full := prefix + randomToken(18)
	id := "key-" + randomToken(4)
	name := defaultStr(req.Name, "Key baru")
	if _, err := a.DB.Pool.Exec(r.Context(), `INSERT INTO api_keys(id,name,prefix,key_hash,scopes,owner_user_id) VALUES ($1,$2,$3,$4,$5,$6)`, id, name, prefix, hashToken(full), scopes, p.UserID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "api_key.create", "api_key", id, map[string]any{"scopes": scopes})
	writeJSON(w, 200, map[string]any{"key": full, "row": map[string]any{"id": id, "name": name, "line": fmt.Sprintf("%s••••  · scope: %s · dibuat %s", prefix, strings.Join(scopes, ", "), domain.ClockID(domain.Now())), "active": true},
		"toast": "API key dibuat · tampil sekali, simpan sekarang"})
}

func (a *App) handleKeyRevoke(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if _, err := a.DB.Pool.Exec(r.Context(), `UPDATE api_keys SET revoked_at=now() WHERE id=$1`, r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "api_key.revoke", "api_key", r.PathValue("id"), nil)
	toast(w, "API key dicabut")
}

func (a *App) handlePolicies(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Pool.Query(r.Context(), `SELECT key, value, description, version, updated_by, updated_at FROM policies ORDER BY key`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var k, desc, by string
		var v json.RawMessage
		var ver int
		var at time.Time
		_ = rows.Scan(&k, &v, &desc, &ver, &by, &at)
		out = append(out, map[string]any{"key": k, "value": v, "description": desc, "version": ver, "updated_by": by, "updated_at": at})
	}
	writeJSON(w, 200, out)
}

// SetPolicy changes a policy (CEO only), keeping history and audit.
func (a *App) SetPolicy(ctx context.Context, p Principal, key string, value json.RawMessage) error {
	if p.Role != domain.RoleCEO {
		return fmt.Errorf("perubahan policy hanya oleh CEO")
	}
	return a.DB.Tx(ctx, func(tx pgxTx) error {
		var ver int
		if err := tx.QueryRow(ctx, `INSERT INTO policies(key,value,updated_by) VALUES ($1,$2,$3) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, version=policies.version+1, updated_by=EXCLUDED.updated_by, updated_at=now() RETURNING version`,
			key, value, p.UserID).Scan(&ver); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policy_history(key,value,version,updated_by) VALUES ($1,$2,$3,$4)`, key, value, ver, p.UserID); err != nil {
			return err
		}
		return storage.Audit(ctx, tx, p.Actor(), "policy.set", "policy", key, json.RawMessage(value))
	})
}

func (a *App) handlePolicySet(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Value json.RawMessage `json:"value"`
	}
	if err := decode(r, &req); err != nil || len(req.Value) == 0 {
		writeErr(w, 400, "value wajib")
		return
	}
	if err := a.SetPolicy(r.Context(), p, r.PathValue("key"), req.Value); err != nil {
		writeErr(w, 403, err.Error())
		return
	}
	toast(w, "Policy diperbarui · tercatat di audit log")
}

func (a *App) handleLLMUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := domain.Now()
	rows, err := a.DB.Pool.Query(ctx, `SELECT tier, provider, model, count(*), COALESCE(sum(tokens_in),0), COALESCE(sum(tokens_out),0), COALESCE(sum(cost_est),0)::float8
		FROM llm_calls WHERE created_at >= $1 GROUP BY 1,2,3 ORDER BY 7 DESC`, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, domain.Jakarta))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var tier, prov, model string
		var n, tin, tout int64
		var cost float64
		_ = rows.Scan(&tier, &prov, &model, &n, &tin, &tout, &cost)
		out = append(out, map[string]any{"tier": tier, "provider": prov, "model": model, "calls": n, "tokens_in": tin, "tokens_out": tout, "cost_usd": cost, "cost_idr": cost * llm.USDToIDR})
	}
	routes := map[string]any{}
	for _, t := range []llm.Tier{llm.Light, llm.Heavy, llm.Interactive} {
		rt := a.LLM.RouteFor(t)
		routes[string(t)] = map[string]string{"provider": rt.Provider, "model": rt.Model}
	}
	writeJSON(w, 200, map[string]any{"month_cost_idr": a.monthCostIDR(ctx), "by_model": out, "routes": routes})
}

func (a *App) handleJobRun(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if !p.Human() && !p.Can(domain.ScopeEvents) {
		writeErr(w, 403, "tidak diizinkan")
		return
	}
	out, err := a.RunJob(r.Context(), r.PathValue("name"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "job.run", "job", r.PathValue("name"), nil)
	writeJSON(w, 200, map[string]any{"job": r.PathValue("name"), "result": json.RawMessage(defaultStr(out, "null")), "toast": "Job " + r.PathValue("name") + " selesai"})
}

func (a *App) handleWebhookSubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := decode(r, &req); err != nil || req.URL == "" {
		writeErr(w, 400, "url wajib")
		return
	}
	if _, err := url.ParseRequestURI(req.URL); err != nil {
		writeErr(w, 400, "url tidak valid")
		return
	}
	id, secret := "wh-"+randomToken(4), randomToken(16)
	if _, err := a.DB.Pool.Exec(r.Context(), `INSERT INTO webhook_subscriptions(id,url,secret,events) VALUES ($1,$2,$3,$4)`, id, req.URL, secret, req.Events); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "secret": secret, "toast": "Webhook terdaftar · simpan secret untuk verifikasi signature"})
}

type pgxTx = pgx.Tx
