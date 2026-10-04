package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

type threadRow struct {
	ID, Type, Name, Sub, Via, Session, JID, AccountID, GroupID, PersonID string
	Unread                                                               int
	LastAt                                                               *time.Time
	Private                                                              bool
	OwnerBranch                                                          string
}

func (a *App) threads(ctx context.Context, p Principal, typ string) ([]threadRow, error) {
	sc := p.Scope()
	q := `SELECT t.id, t.type, t.name, t.subtitle, COALESCE(u.name, w.label), t.session_id, t.chat_jid, COALESCE(t.account_id,''), COALESCE(t.group_id,''), COALESCE(t.person_id,''),
		t.unread, t.last_at, t.is_private, COALESCE(u.branch,'') FROM chat_threads t JOIN wa_sessions w ON w.id=t.session_id LEFT JOIN users u ON u.id=w.user_id
		LEFT JOIN accounts a ON a.id=t.account_id WHERE ($1='all' OR $1='' OR t.type=$1)`
	args := []any{typ}
	if !sc.All {
		q += ` AND (w.user_id=$2 OR a.branch=$3)`
		args = append(args, sc.UserID, sc.Branch)
	}
	q += ` ORDER BY CASE t.type WHEN 'cust' THEN 0 WHEN 'gext' THEN 1 WHEN 'gint' THEN 2 ELSE 3 END, t.unread DESC, t.last_at DESC NULLS LAST`
	rows, err := a.DB.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []threadRow
	for rows.Next() {
		var t threadRow
		if err := rows.Scan(&t.ID, &t.Type, &t.Name, &t.Sub, &t.Via, &t.Session, &t.JID, &t.AccountID, &t.GroupID, &t.PersonID, &t.Unread, &t.LastAt, &t.Private, &t.OwnerBranch); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func timeLabel(t *time.Time) string {
	if t == nil {
		return ""
	}
	now := domain.Now()
	switch domain.DaysBetween(*t, now) {
	case 0:
		return domain.ClockID(*t)
	case 1:
		return "Kemarin"
	}
	return domain.ShortDate(*t)
}

func dayHeader(t time.Time) string {
	switch domain.DaysBetween(t, domain.Now()) {
	case 0:
		return "Hari ini"
	case 1:
		return "Kemarin"
	}
	return domain.DayLabel(t)
}

func initialsOf(t threadRow) string {
	if t.Type == "gext" || t.Type == "gint" {
		return "#"
	}
	return domain.Initials(t.Name)
}

var signalTag = map[string]string{"competitor_mentioned": "Kompetitor disebut", "payment_on_time": "Pembayaran masuk", "champion_moved": "Champion mutasi"}

// threadTag derives the pill shown in the chat list.
func (a *App) threadTag(ctx context.Context, t threadRow) map[string]string {
	switch t.Type {
	case "internal":
		return map[string]string{"k": "neutral", "t": "Tidak dibaca"}
	case "gint":
		return map[string]string{"k": "neutral", "t": "Internal · koordinasi"}
	case "gext":
		var risk string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT e.text FROM extractions e JOIN interactions i ON i.id=e.interaction_id WHERE i.thread_id=$1 AND e.kind='risk' AND e.tone='bad' ORDER BY i.occurred_at DESC LIMIT 1`, t.ID).Scan(&risk)
		if risk != "" {
			r := strings.TrimPrefix(risk, "Risiko: ")
			if i := strings.Index(r, " ·"); i > 0 {
				r = r[:i]
			}
			r = strings.TrimPrefix(r, "kekurangan ")
			return map[string]string{"k": "bad", "t": "Risiko " + strings.TrimSpace(strings.TrimLeft(r, "0123456789 "))}
		}
		var n int
		_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE group_id=$1 AND status='detected'`, t.GroupID).Scan(&n)
		if n > 0 {
			return map[string]string{"k": "indigo", "t": fmt.Sprintf("%d tugas terdeteksi", n)}
		}
		return map[string]string{"k": "accent", "t": "Grup project"}
	}
	if t.AccountID == "" {
		return map[string]string{"k": "neutral", "t": "Nomor baru"}
	}
	sod := domain.StartOfDay(domain.Now())
	if a.exists(ctx, `SELECT 1 FROM commitments WHERE account_id=$1 AND who='kami' AND status IN ('open','late') AND due_at >= $2 AND due_at < $3`, t.AccountID, sod, sod.AddDate(0, 0, 1)) {
		return map[string]string{"k": "accent", "t": "Komitmen hari ini"}
	}
	var typ, title, sev string
	err := a.DB.Pool.QueryRow(ctx, `SELECT type, title, severity FROM signals WHERE account_id=$1 AND resolved_at IS NULL AND type IN ('competitor_mentioned','po_overdue','payment_on_time','champion_moved','silent')
		ORDER BY CASE type WHEN 'po_overdue' THEN 0 WHEN 'competitor_mentioned' THEN 1 WHEN 'champion_moved' THEN 2 WHEN 'payment_on_time' THEN 3 ELSE 4 END LIMIT 1`, t.AccountID).Scan(&typ, &title, &sev)
	if err == nil {
		if l, ok := signalTag[typ]; ok {
			title = l
		}
		return map[string]string{"k": sev, "t": title}
	}
	return map[string]string{"k": "neutral", "t": "Pelanggan"}
}

func (a *App) handleChatThreads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	list, err := a.threads(ctx, p, r.URL.Query().Get("type"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	items := []map[string]any{}
	unread := 0
	for _, t := range list {
		last := ""
		if t.Private {
			last = "Chat pribadi karyawan · tidak dibaca"
		} else {
			var who, body string
			var dir string
			_ = a.DB.Pool.QueryRow(ctx, `SELECT sender_name, body_text, direction FROM interactions WHERE thread_id=$1 ORDER BY occurred_at DESC LIMIT 1`, t.ID).Scan(&who, &body, &dir)
			last = body
			if (t.Type == "gext" || t.Type == "gint") && who != "" {
				last = strings.Fields(who)[0] + ": " + body
			}
		}
		if !t.Private {
			unread += t.Unread
		}
		items = append(items, map[string]any{"id": t.ID, "type": t.Type, "name": t.Name, "sub": t.Sub, "via": firstName(t.Via), "time": timeLabel(t.LastAt),
			"unread": t.Unread, "tag": a.threadTag(ctx, t), "last": last, "private": t.Private, "initials": initialsOf(t)})
	}
	writeJSON(w, 200, map[string]any{"items": items, "unread": unread})
}

func (a *App) threadByID(ctx context.Context, p Principal, id string) (threadRow, bool) {
	list, err := a.threads(ctx, p, "all")
	if err != nil {
		return threadRow{}, false
	}
	for _, t := range list {
		if t.ID == id {
			return t, true
		}
	}
	return threadRow{}, false
}

type msgRow struct {
	ID       int64
	Dir      string
	Who      string
	Int      bool
	Text     string
	At       time.Time
	Delivery string
	AnnID    int64
	AnnTone  string
	AnnText  string
	AnnAct   string
}

func (a *App) messages(ctx context.Context, threadID string) ([]msgRow, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT i.id, i.direction, i.sender_name, i.sender_internal, i.body_text, i.occurred_at, i.delivery_status,
		COALESCE(e.id,0), COALESCE(e.tone,''), COALESCE(e.text,''), COALESCE(e.action_label,'')
		FROM interactions i LEFT JOIN LATERAL (SELECT id, tone, text, action_label FROM extractions WHERE interaction_id=i.id ORDER BY id LIMIT 1) e ON true
		WHERE i.thread_id=$1 ORDER BY i.occurred_at, i.id`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []msgRow
	for rows.Next() {
		var m msgRow
		if err := rows.Scan(&m.ID, &m.Dir, &m.Who, &m.Int, &m.Text, &m.At, &m.Delivery, &m.AnnID, &m.AnnTone, &m.AnnText, &m.AnnAct); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *App) messageViews(ctx context.Context, t threadRow) []map[string]any {
	msgs, _ := a.messages(ctx, t.ID)
	out := []map[string]any{}
	lastDay := ""
	group := t.Type == "gext" || t.Type == "gint"
	for _, m := range msgs {
		d := dayHeader(m.At)
		if d != lastDay {
			out = append(out, map[string]any{"day": d})
			lastDay = d
		}
		v := map[string]any{"id": m.ID, "f": m.Dir, "t": m.Text, "tm": domain.ClockID(m.At)}
		if group && m.Dir == "in" {
			v["who"], v["int"] = m.Who, m.Int
		}
		if m.Dir == "out" {
			if group && m.Who != "" {
				v["who"] = m.Who
			}
			if m.Delivery != "" && strings.HasPrefix(m.Delivery, "terkirim") && domain.DaysBetween(m.At, domain.Now()) == 0 {
				v["sent"] = m.Delivery
			}
		}
		if m.AnnText != "" {
			ann := map[string]any{"k": m.AnnTone, "t": m.AnnText}
			if m.AnnAct != "" {
				ann["act"], ann["act_id"] = m.AnnAct, fmt.Sprint(m.AnnID)
			}
			v["ann"] = ann
		}
		out = append(out, v)
	}
	// Replies waiting for approval are shown as pending outbound bubbles.
	rows, err := a.DB.Pool.Query(ctx, `SELECT preview, created_at FROM actions WHERE type='send_wa' AND status IN ('proposed','approved','edited') AND payload->>'thread_id'=$1 ORDER BY created_at`, t.ID)
	if err == nil {
		for rows.Next() {
			var text string
			var at time.Time
			_ = rows.Scan(&text, &at)
			if d := dayHeader(at); d != lastDay {
				out = append(out, map[string]any{"day": d})
				lastDay = d
			}
			out = append(out, map[string]any{"id": 0, "f": "out", "t": text, "tm": domain.ClockID(at), "sent": "menunggu persetujuan"})
		}
		rows.Close()
	}
	return out
}

func (a *App) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	t, ok := a.threadByID(r.Context(), p, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "thread tidak ditemukan")
		return
	}
	if t.Private {
		writeJSON(w, 200, []any{})
		return
	}
	writeJSON(w, 200, a.messageViews(r.Context(), t))
}

func (a *App) handleChatThread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	t, ok := a.threadByID(ctx, p, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "thread tidak ditemukan")
		return
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE chat_threads SET unread=0 WHERE id=$1`, t.ID)
	via := firstName(t.Via)
	note := map[string]string{"gext": "grup eksternal · dibaca penuh", "gint": "grup internal · jadwal & tugas saja", "internal": "internal · tidak dibaca", "cust": "pelanggan · dibaca ARC"}[t.Type]
	out := map[string]any{"id": t.ID, "type": t.Type, "name": t.Name, "sub": t.Sub, "via": via, "initials": initialsOf(t), "via_note": note, "private": t.Private,
		"messages": []any{}, "suggestions": []string{}}
	dest := "chatter opportunity Odoo"
	if t.Type == "gext" || t.Type == "gint" {
		dest = "project Odoo"
	}
	out["policy_line"] = fmt.Sprintf("Dikirim dari nomor %s · %s diberi tahu · dicatat ke %s", via, via, dest)
	if t.Private {
		var unit, branch string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT unit, branch FROM internal_numbers WHERE phone_norm=$1`, domain.NormalizePhone(jidPhone(t.JID))).Scan(&unit, &branch)
		out["private_note"] = map[string]string{"title": "Chat pribadi antar karyawan tidak dibaca ARC.",
			"body": fmt.Sprintf("Nomor ini terdaftar sebagai internal (%s · %s). Hanya grup project dan grup internal yang ditandai koordinasi yang dibaca. Aturan ini ada di Koneksi → Batas & privasi.", defaultStr(unit, "Internal"), defaultStr(branch, t.OwnerBranch))}
	} else {
		out["messages"] = a.messageViews(ctx, t)
		if s, err := a.Agents.Suggestions(ctx, t.ID); err == nil && s != nil {
			out["suggestions"] = s
		}
	}
	out["context"] = a.chatContext(ctx, t)
	writeJSON(w, 200, out)
}

func (a *App) chatContext(ctx context.Context, t threadRow) map[string]any {
	if t.Type == "internal" || t.Type == "gint" {
		note := "Grup internal: dibaca untuk jadwal dan koordinasi teknisi. Tidak ada pelanggan di grup ini, jadi tidak memengaruhi health akun mana pun."
		if t.Type == "internal" {
			note = "Chat internal pribadi. ARC hanya tahu nomor ini ada dan siapa pemiliknya; isinya tidak dibaca dan tidak disimpan."
		}
		members := []map[string]string{}
		rows, err := a.DB.Pool.Query(ctx, `SELECT name, unit FROM internal_numbers WHERE branch=$1 ORDER BY id`, t.OwnerBranch)
		if err == nil {
			for rows.Next() {
				var n, u string
				_ = rows.Scan(&n, &u)
				members = append(members, map[string]string{"n": n, "unit": u})
			}
			rows.Close()
		}
		return map[string]any{"kind": "internal", "note": note, "internal_members": members}
	}
	out := map[string]any{"kind": "customer"}
	if t.AccountID != "" {
		var name, opp, stage, oppID string
		var health int
		var value float64
		_ = a.DB.Pool.QueryRow(ctx, `SELECT a.name, COALESCE(a.health,0), COALESCE(o.name,''), COALESCE(o.expected_revenue,0)::float8, COALESCE(sd.name,''), COALESCE(o.id,'')
			FROM accounts a LEFT JOIN LATERAL (SELECT * FROM opportunities WHERE account_id=a.id AND status='open' AND NOT historical ORDER BY expected_revenue DESC LIMIT 1) o ON true
			LEFT JOIN stage_definitions sd ON sd.id=o.stage_id WHERE a.id=$1`, t.AccountID).Scan(&name, &health, &opp, &value, &stage, &oppID)
		acc := map[string]any{"id": t.AccountID, "name": name, "health": health, "line": fmt.Sprintf("%s · %s · Odoo %s", opp, fmtRp(value), stage)}
		var actID string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM actions WHERE account_id=$1 AND status='proposed' AND type <> 'historical' AND (opportunity_id=$2 OR $2='') ORDER BY in_queue DESC, created_at LIMIT 1`, t.AccountID, oppID).Scan(&actID)
		if v := a.actionViewPtr(ctx, actID); v != nil {
			acc["action"] = v
		}
		out["account"] = acc
	}
	if t.Type == "gext" {
		out["kind"] = "group"
		var project, summary, members []byte
		_ = a.DB.Pool.QueryRow(ctx, `SELECT project, summary, members FROM chat_groups WHERE id=$1`, t.GroupID).Scan(&project, &summary, &members)
		var pj map[string]any
		_ = json.Unmarshal(project, &pj)
		if len(pj) > 0 {
			out["project"] = pj
		}
		var sm []string
		_ = json.Unmarshal(summary, &sm)
		if sm == nil {
			sm = []string{}
		}
		out["summary"] = sm
		var mem []map[string]any
		_ = json.Unmarshal(members, &mem)
		ms := []map[string]any{}
		for _, m := range mem {
			ms = append(ms, map[string]any{"n": m["n"], "r": defaultStr(fmt.Sprint(m["r"]), "Anggota"), "int": m["int"]})
		}
		out["members"] = ms
		tasks := []map[string]string{}
		rows, err := a.DB.Pool.Query(ctx, `SELECT id, title, assignee, source_label, status FROM tasks WHERE group_id=$1 ORDER BY created_at, id`, t.GroupID)
		if err == nil {
			for rows.Next() {
				var id, title, who, src, st string
				_ = rows.Scan(&id, &title, &who, &src, &st)
				tasks = append(tasks, map[string]string{"id": id, "t": title, "who": who, "src": src, "status": st})
			}
			rows.Close()
		}
		out["tasks"] = tasks
		return out
	}
	ext := []map[string]string{}
	msgs, _ := a.messages(ctx, t.ID)
	for _, m := range msgs {
		if m.AnnText != "" {
			ext = append(ext, map[string]string{"k": m.AnnTone, "t": m.AnnText, "tm": domain.ClockID(m.At)})
		}
	}
	out["extracted"] = ext
	oc := []map[string]any{}
	if t.AccountID != "" {
		rows, err := a.DB.Pool.Query(ctx, `SELECT text, detail, status, due_at FROM commitments WHERE account_id=$1 AND who='mereka' AND status <> 'done' ORDER BY due_at NULLS LAST`, t.AccountID)
		if err == nil {
			for rows.Next() {
				var text, detail, st string
				var due *time.Time
				_ = rows.Scan(&text, &detail, &st, &due)
				pill := map[string]string{"k": "neutral", "t": "Terbuka"}
				if due != nil && due.Before(domain.Now()) {
					pill = map[string]string{"k": "bad", "t": fmt.Sprintf("Lewat %d hari", domain.DaysBetween(*due, domain.Now()))}
				}
				oc = append(oc, map[string]any{"t": "Mereka: " + text, "s": detail, "pill": pill})
			}
			rows.Close()
		}
	}
	out["open_commitments"] = oc
	return out
}

func fmtRp(v float64) string { return domain.FormatRp(v) }

func (a *App) handleChatSuggestions(w http.ResponseWriter, r *http.Request) {
	s, err := a.Agents.Suggestions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if s == nil {
		s = []string{}
	}
	writeJSON(w, 200, s)
}

// handleChatReply turns a typed reply into a send_wa Action awaiting human approval.
func (a *App) handleChatReply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	t, ok := a.threadByID(ctx, p, r.PathValue("id"))
	if !ok || t.Private {
		writeErr(w, 404, "thread tidak ditemukan")
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeErr(w, 400, "teks balasan kosong")
		return
	}
	via := firstName(t.Via)
	id, _, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Balasan manual", Type: "send_wa", Kind: "send", Icon: "i-chat", ButtonLabel: "Kirim",
		AccountID: t.AccountID, DueLabel: "Sekarang", Title: "Kirim balasan ke " + t.Name, Summary: "Balasan diketik " + p.Name + " di Chat; menunggu persetujuan sebelum dikirim dari nomor " + via + ".",
		Why: "Balasan manual dari layar Chat.", Prep: "Pesan siap dikirim dari nomor " + via + ".", Preview: req.Text, PreviewFrom: "WhatsApp · dari nomor " + via,
		Steps: []string{"Pesan dikirim dari nomor " + via + " lewat transport WhatsApp", "Dicatat ke thread dan chatter Odoo"}, InQueue: true,
		Payload:    map[string]any{"session": t.Session, "chat_jid": t.JID, "thread_id": t.ID, "to": t.Name, "toast": "Terkirim ke " + t.Name + " dari nomor " + via},
		Confidence: 1}, p.Actor())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"action": a.actionViewPtr(ctx, id),
		"toast":   "Draft balasan masuk antrean persetujuan · tidak ada yang terkirim tanpa approve",
		"message": map[string]any{"id": 0, "f": "out", "t": req.Text, "tm": domain.ClockID(domain.Now()), "sent": "menunggu persetujuan"}})
}

// handleTaskSend: the user's click is the approval for an internal L3 action (Basecamp to-do).
func (a *App) handleTaskSend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	var title, who, acc string
	var src *int64
	if err := a.DB.Pool.QueryRow(ctx, `SELECT title, assignee, COALESCE(account_id,''), source_interaction_id FROM tasks WHERE id=$1`, r.PathValue("id")).Scan(&title, &who, &acc, &src); err != nil {
		writeErr(w, 404, "tugas tidak ditemukan")
		return
	}
	act, toastMsg, err := a.proposeAndApprove(ctx, p, actions.Proposal{Agent: "Capture agent", Type: "create_task", Kind: "task", Icon: "i-check", ButtonLabel: "Ke Basecamp",
		AccountID: acc, Title: title, Why: "Tugas terdeteksi dari grup project.", Prep: "To-do Basecamp untuk " + who, Steps: []string{"To-do dibuat di Basecamp"},
		Payload:  map[string]any{"task_id": r.PathValue("id"), "assignee": who, "toast": "Tugas dibuat di Basecamp untuk " + who},
		Evidence: []domain.Evidence{{InteractionID: deref(src), Quote: title}}, Confidence: 0.9})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"action": a.actionView(act), "toast": toastMsg})
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// proposeAndApprove records a proposal and the user's immediate decision (button = approval).
func (a *App) proposeAndApprove(ctx context.Context, p Principal, prop actions.Proposal) (actions.Action, string, error) {
	if prop.ID == "" {
		prop.ID = "act-" + storage.Hash(prop.Type, prop.Title, p.UserID, domain.Now().String())[:16]
	}
	if _, _, err := a.Actions.Propose(ctx, prop, p.Actor()); err != nil {
		return actions.Action{}, "", err
	}
	return a.Actions.Decide(ctx, prop.ID, p.Actor(), actions.Decision{Decision: "approve"})
}

func (a *App) handleAnnotationAct(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	var text, label, acc string
	var iid int64
	if err := a.DB.Pool.QueryRow(ctx, `SELECT e.text, e.action_label, COALESCE(i.account_id,''), i.id FROM extractions e JOIN interactions i ON i.id=e.interaction_id WHERE e.id=$1`, r.PathValue("id")).Scan(&text, &label, &acc, &iid); err != nil {
		writeErr(w, 404, "anotasi tidak ditemukan")
		return
	}
	who := "Purchasing"
	toastMsg := label + " · dibuat, permintaan masuk ke Purchasing"
	if strings.Contains(label, "Basecamp") {
		toastMsg = label + " · dibuat, tugas masuk ke project Basecamp"
		who = "Admin Project"
		if i := strings.Index(text, "→ "); i >= 0 {
			who = strings.TrimSpace(strings.Split(text[i+len("→ "):], ":")[0])
		}
	}
	if _, _, err := a.proposeAndApprove(ctx, p, actions.Proposal{Agent: "Capture agent", Type: "create_task", Kind: "task", Icon: "i-check", AccountID: acc, Title: text,
		Why: "Anotasi ARC pada pesan grup.", Prep: label, Steps: []string{label}, Payload: map[string]any{"assignee": who, "toast": toastMsg},
		Evidence: []domain.Evidence{{InteractionID: iid, Quote: text}}, Confidence: 0.88}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	toast(w, toastMsg)
}

func (a *App) handleGroupPolicy(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Read bool `json:"read"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "format tidak valid")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(), `UPDATE chat_groups SET read_policy=$2, updated_at=now() WHERE id=$1`, r.PathValue("id"), req.Read)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "grup tidak ditemukan")
		return
	}
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "group.read_policy", "chat_group", r.PathValue("id"), req)
	toast(w, "Aturan diperbarui · berlaku di sinkron berikutnya")
}

// ---------------- Ask ----------------

func (a *App) handleAsk(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		Question string `json:"question"`
		Screen   string `json:"screen"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Question) == "" {
		writeErr(w, 400, "pertanyaan kosong")
		return
	}
	ans, err := a.Agents.Ask(r.Context(), p.UserID, req.Question, req.Screen, p.Scope())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ans)
}

func (a *App) handleAskHistory(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	h, err := a.Agents.AskHistory(r.Context(), p.UserID, 20)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if h == nil {
		writeJSON(w, 200, []any{})
		return
	}
	writeJSON(w, 200, h)
}

func (a *App) handleAskSuggestions(w http.ResponseWriter, r *http.Request) {
	s := a.Agents.AskSuggestions(r.URL.Query().Get("screen"))
	if s == nil {
		s = []string{}
	}
	sort.SliceStable(s, func(i, j int) bool { return false })
	writeJSON(w, 200, s)
}
