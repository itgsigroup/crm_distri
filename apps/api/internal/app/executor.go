package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"arc/packages/connectors/notify"
	"arc/packages/connectors/odoo"
	"arc/packages/connectors/whatsapp"
	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

// executor carries out approved actions. Nothing here runs without a recorded human decision.
type executor struct{ a *App }

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// transportFor picks the transport for a WhatsApp session: the bridge when the
// session is connected, the Cloud API for cloud sessions, otherwise the fake (mock mode).
func (a *App) transportFor(ctx context.Context, session string) (whatsapp.Transport, bool) {
	var transport, status string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT transport, status FROM wa_sessions WHERE id=$1`, session).Scan(&transport, &status)
	if a.Cfg.Env == "test" {
		return a.FakeWA, true
	}
	if transport == "cloud" && a.Cloud != nil {
		return a.Cloud, false
	}
	if status == "connected" {
		hctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if h, err := a.Bridge.Health(hctx); err == nil {
			if sessions, ok := h["sessions"].(map[string]any); ok {
				if st, ok := sessions[session].(string); ok && st == "connected" {
					return a.Bridge, false
				}
			}
		}
	}
	return a.FakeWA, true
}

func (e *executor) Execute(ctx context.Context, act actions.Action) (actions.ExecResult, error) {
	a := e.a
	res := actions.ExecResult{Log: map[string]any{"type": act.Type}}
	p := act.Payload
	switch act.Type {
	case "send_wa", "ask_spm_documents":
		session, jid := str(p, "session"), str(p, "chat_jid")
		if session == "" || jid == "" {
			_ = a.DB.Pool.QueryRow(ctx, `SELECT session_id, chat_jid FROM chat_threads WHERE account_id=$1 AND type='cust' ORDER BY last_at DESC NULLS LAST LIMIT 1`, act.AccountID).Scan(&session, &jid)
		}
		if session == "" {
			return res, errors.New("tidak ada nomor WhatsApp tertaut untuk akun ini")
		}
		text := act.Preview
		if strings.TrimSpace(text) == "" {
			return res, errors.New("draf pesan kosong")
		}
		if !a.allowSend(ctx, session) {
			return res, errors.New("batas kirim WhatsApp per jam untuk nomor ini tercapai (≤ 20/jam)")
		}
		tr, mock := a.transportFor(ctx, session)
		wamid, err := tr.Send(ctx, session, jid, text, act.ID)
		if err != nil {
			return res, err
		}
		res.Log["transport"], res.Log["wamid"], res.Log["mock"] = tr.Name(), wamid, mock
		var threadID string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM chat_threads WHERE session_id=$1 AND chat_jid=$2`, session, jid).Scan(&threadID)
		var owner string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(u.name,'') FROM wa_sessions w LEFT JOIN users u ON u.id=w.user_id WHERE w.id=$1`, session).Scan(&owner)
		var uid string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(user_id,'') FROM wa_sessions WHERE id=$1`, session).Scan(&uid)
		if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO interactions(channel,direction,occurred_at,user_ids,thread_id,body_text,raw_ref,wamid,account_id,sender_name,sender_internal,transport,extracted,delivery_status)
			VALUES ('wa_message','out',$1,$2,NULLIF($3,''),$4,$5,$5,NULLIF($6,''),$7,true,$8,true,'terkirim') ON CONFLICT (raw_ref) DO NOTHING`,
			domain.Now(), []string{uid}, threadID, text, "out:"+act.ID, act.AccountID, owner, tr.Name()); err != nil {
			return res, err
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE chat_threads SET last_at=$2, updated_at=now() WHERE id=$1`, threadID, domain.Now())
		a.chatterNote(ctx, act, "WhatsApp terkirim: "+trunc(text, 300))
		a.fulfilOwnCommitment(ctx, act)
		if mock {
			res.Toast = strings.TrimSpace(str(p, "toast"))
			if res.Toast == "" {
				res.Toast = "Terkirim ke " + defaultStr(str(p, "to"), "kontak")
			}
			res.Toast += " · mode mock (nomor belum tertaut ke bridge)"
		}
	case "send_email", "payment_reminder", "request_referral":
		mailbox := defaultStr(str(p, "mailbox"), a.ownerOf(ctx, act.AccountID))
		to := str(p, "to")
		if to == "" {
			_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(emails[1],'') FROM people WHERE account_id=$1 AND cardinality(emails)>0 ORDER BY strength DESC LIMIT 1`, act.AccountID).Scan(&to)
		}
		id, err := a.Drafts.CreateDraft(ctx, mailbox, to, act.Title, act.Preview)
		if err != nil {
			return res, err
		}
		res.Log["gmail_draft"], res.Log["mailbox"] = id, mailbox
		a.chatterNote(ctx, act, "Email draft disiapkan: "+act.Title)
		a.fulfilOwnCommitment(ctx, act)
		if act.ResultText == "" {
			res.Result = "Draft Gmail dibuat di kotak " + mailbox + " · " + domain.ClockID(domain.Now())
		}
	case "create_task", "schedule_bast", "schedule_meeting", "meeting_brief", "meeting_brief_point", "call_prep", "prepare_document",
		"create_quotation", "offer_maintenance", "create_invoice":
		assignee := defaultStr(str(p, "assignee"), a.ownerOf(ctx, act.AccountID))
		if act.Type == "create_invoice" {
			assignee = "Finance"
		}
		todo, err := a.Todos.CreateTodo(ctx, act.Title, assignee)
		if err != nil {
			return res, err
		}
		res.Log["todo"] = todo
		if tid := str(p, "task_id"); tid != "" {
			_, _ = a.DB.Pool.Exec(ctx, `UPDATE tasks SET status='open', basecamp_id=$2, updated_at=now() WHERE id=$1`, tid, todo)
		}
		if act.Type == "meeting_brief" || act.Type == "meeting_brief_point" {
			_, _ = a.DB.Pool.Exec(ctx, `UPDATE calendar_events SET prep_status='ready' WHERE account_id=$1 AND starts_at > $2`, act.AccountID, domain.Now())
		}
		if act.Type == "create_invoice" {
			a.notifyRole(ctx, domain.RoleFinance, "Draft invoice diminta: "+act.Title, act.Why)
		}
		a.fulfilOwnCommitment(ctx, act)
	case "create_opportunity", "expansion_opportunity", "create_lead":
		id, err := a.createOpportunity(ctx, act)
		if err != nil {
			return res, err
		}
		res.Log["opportunity"] = id
	case "create_in_odoo":
		if err := a.createInOdoo(ctx, act); err != nil {
			return res, err
		}
	case "link_to_odoo":
		oid := str(p, "opportunity_id")
		if _, err := a.DB.Pool.Exec(ctx, `UPDATE opportunities SET source_system='odoo', source_id=$2, locked_to_source=true, stage_id=COALESCE((SELECT id FROM stage_definitions WHERE name=$3), stage_id) WHERE id=$1`,
			oid, str(p, "odoo_id"), str(p, "stage")); err != nil {
			return res, err
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE odoo_links SET status='linked' WHERE arc_type='opportunity' AND arc_id=$1`, oid)
	case "write_probability":
		if err := a.writeProbability(ctx, act); err != nil {
			return res, err
		}
	case "merge_person":
		if err := a.Agents.MergePeople(ctx, str(p, "keep"), str(p, "drop")); err != nil {
			return res, err
		}
	case "qualify_tender":
		if _, err := a.qualifyTender(ctx, str(p, "tender_id")); err != nil {
			return res, err
		}
	case "discount_exception", "credit_release":
		chosen := str(p, "chosen_option")
		res.Log["option"] = chosen
		a.notifyUser(ctx, act.ProposedBy, act.Title, "Keputusan: "+chosen)
		if act.Type == "credit_release" {
			res.Log["sop"] = "SOP-SEC-001 checklist tercatat"
		}
	default:
		res.Log["note"] = "dijalankan sebagai tugas internal"
	}
	return res, nil
}

// allowSend enforces ≤ wa_send_rate_per_hour outbound messages per session.
func (a *App) allowSend(ctx context.Context, session string) bool {
	limit := int(a.Ins.PolicyFloat(ctx, "wa_send_rate_per_hour", 20))
	var n int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM interactions i JOIN chat_threads t ON t.id=i.thread_id WHERE t.session_id=$1 AND i.direction='out' AND i.raw_ref LIKE 'out:%' AND i.created_at > now() - interval '1 hour'`, session).Scan(&n)
	return n < limit
}

func (a *App) ownerOf(ctx context.Context, accountID string) string {
	var u string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(owner_user_id,'sam') FROM accounts WHERE id=$1`, accountID).Scan(&u)
	return defaultStr(u, "sam")
}

func (a *App) notifyRole(ctx context.Context, role, subject, body string) {
	for _, n := range a.Notifiers {
		_ = n.Send(ctx, notify.Message{To: a.recipients(ctx, role), Subject: subject, Text: body})
	}
}

func (a *App) notifyUser(ctx context.Context, userID, subject, body string) {
	var email string
	if a.DB.Pool.QueryRow(ctx, `SELECT email FROM users WHERE id=$1`, userID).Scan(&email) != nil {
		return
	}
	for _, n := range a.Notifiers {
		_ = n.Send(ctx, notify.Message{To: []string{email}, Subject: subject, Text: body})
	}
}

// fulfilOwnCommitment marks today's "kami" commitment of the opportunity as done once the action is executed.
func (a *App) fulfilOwnCommitment(ctx context.Context, act actions.Action) {
	if act.AccountID == "" {
		return
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE commitments SET status='done', updated_at=now() WHERE account_id=$1 AND who='kami' AND status IN ('open','late') AND (draft_ready OR (due_at IS NOT NULL AND due_at < $2))`,
		act.AccountID, domain.StartOfDay(domain.Now()).AddDate(0, 0, 1))
}

// chatterNote posts a note on the linked Odoo opportunity (Stage 10), marked "via ARC".
func (a *App) chatterNote(ctx context.Context, act actions.Action, body string) {
	if act.OpportunityID == "" {
		return
	}
	var src string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(source_id,'') FROM opportunities WHERE id=$1 AND source_system='odoo'`, act.OpportunityID).Scan(&src)
	if src == "" {
		return
	}
	var id int64
	fmt.Sscanf(src, "%d", &id)
	op := odoo.WriteOp{Model: "mail.message", Method: "message_post", ID: id, Values: map[string]any{"model": "crm.lead", "res_id": id, "body": body + " — " + odoo.Marker("ARC action "+act.ID)}}
	_, err := a.OdooWriter.Apply(ctx, op)
	a.recordOdooWrite(ctx, act.ID, op, err)
}

func (a *App) recordOdooWrite(ctx context.Context, actionID string, op odoo.WriteOp, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO odoo_writes(action_id,model,method,odoo_id,payload,dry_run,ok,error) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		actionID, op.Model, op.Method, op.ID, storage.JSONObj(op.Values), a.OdooMock, err == nil, msg)
	_ = storage.Audit(ctx, a.DB.Pool, storage.Actor{ID: "odoo_writer", Type: "system"}, "odoo:"+op.Method, op.Model, fmt.Sprint(op.ID), op.Values)
}

func (a *App) writeProbability(ctx context.Context, act actions.Action) error {
	var src string
	var manual int
	if err := a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(source_id,''), probability FROM opportunities WHERE id=$1`, act.OpportunityID).Scan(&src, &manual); err != nil {
		return err
	}
	if src == "" {
		return errors.New("opportunity belum tertaut ke Odoo")
	}
	var id int64
	fmt.Sscanf(src, "%d", &id)
	var wd string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT write_date FROM odoo_records WHERE model='crm.lead' AND odoo_id=$1`, id).Scan(&wd)
	prob := int(toFloat(act.Payload["probability"]))
	op := odoo.WriteOp{Model: "crm.lead", Method: "write", ID: id, Values: map[string]any{"probability": prob}, ExpectedWriteDate: wd}
	_, err := a.OdooWriter.Apply(ctx, op)
	a.recordOdooWrite(ctx, act.ID, op, err)
	var conflict *odoo.ErrConflict
	if errors.As(err, &conflict) {
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO signals(type,severity,account_id,opportunity_id,title,detail,evidence,dedupe_key) VALUES ('sync_conflict','warn',$1,$2,'Konflik sinkron Odoo',$3,$4,$5)
			ON CONFLICT (dedupe_key) DO UPDATE SET detected_at=now(), resolved_at=NULL`, act.AccountID, act.OpportunityID, conflict.Error(),
			storage.JSON([]domain.Evidence{{Source: "odoo", Quote: conflict.Error()}}), "sync_conflict:"+act.OpportunityID)
		return fmt.Errorf("record Odoo berubah sejak sinkron terakhir — penulisan dibatalkan dan dicatat sebagai konflik")
	}
	if err != nil {
		return err
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE opportunities SET probability=$2 WHERE id=$1`, act.OpportunityID, prob)
	note := odoo.WriteOp{Model: "mail.message", Method: "message_post", ID: id, Values: map[string]any{"model": "crm.lead", "res_id": id,
		"body": fmt.Sprintf("Probabilitas %d%% → %d%% berdasarkan bukti: %s — %s", manual, prob, act.Why, odoo.Marker("health & sinyal ARC"))}}
	_, err = a.OdooWriter.Apply(ctx, note)
	a.recordOdooWrite(ctx, act.ID, note, err)
	return err
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		var f float64
		fmt.Sscanf(x, "%f", &f)
		return f
	}
	return 0
}

func (a *App) createInOdoo(ctx context.Context, act actions.Action) error {
	oid := str(act.Payload, "opportunity_id")
	var name, acc string
	var rev float64
	var partner *int64
	if err := a.DB.Pool.QueryRow(ctx, `SELECT o.name, o.account_id, o.expected_revenue::float8, a.odoo_partner_id FROM opportunities o JOIN accounts a ON a.id=o.account_id WHERE o.id=$1`, oid).Scan(&name, &acc, &rev, &partner); err != nil {
		return err
	}
	vals := map[string]any{"name": name, "type": "opportunity", "expected_revenue": rev, "description": odoo.Marker("ARC opportunity " + oid)}
	if partner != nil {
		vals["partner_id"] = *partner
	}
	op := odoo.WriteOp{Model: "crm.lead", Method: "create", Values: vals}
	id, err := a.OdooWriter.Apply(ctx, op)
	op.ID = id
	a.recordOdooWrite(ctx, act.ID, op, err)
	if err != nil {
		return err
	}
	_, err = a.DB.Pool.Exec(ctx, `UPDATE opportunities SET source_system='odoo', source_id=$2, locked_to_source=true WHERE id=$1`, oid, fmt.Sprint(id))
	return err
}

// createOpportunity creates an ARC opportunity (stage Baru) from an approved proposal.
func (a *App) createOpportunity(ctx context.Context, act actions.Action) (string, error) {
	p := act.Payload
	acc := defaultStr(str(p, "account_id"), act.AccountID)
	name := defaultStr(str(p, "name"), act.Title)
	value := toFloat(p["value"])
	source := defaultStr(str(p, "source"), "ekspansi")
	id := "opp-" + storage.Hash(acc, name)[:10]
	var owner any
	_ = a.DB.Pool.QueryRow(ctx, `SELECT owner_user_id FROM accounts WHERE id=$1`, acc).Scan(&owner)
	_, err := a.DB.Pool.Exec(ctx, `INSERT INTO opportunities(id,account_id,name,expected_revenue,probability,stage_id,owner_user_id,source,status,lead_at,note,product_line,closing_label)
		VALUES ($1,$2,$3,$4,10,(SELECT id FROM stage_definitions WHERE name='Baru'),$5,$6,'open',$7,$8,$9,$10) ON CONFLICT (id) DO NOTHING`,
		id, acc, name, value, owner, source, domain.Now(), str(p, "note"), str(p, "line"), str(p, "closing"))
	if err != nil {
		return "", err
	}
	if inb := str(p, "inbound_id"); inb != "" {
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE inbound_contacts SET status='lead', opportunity_id=$2, updated_at=now() WHERE id=$1`, inb, id)
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO funnel_events(inbound_id,subject_key,stage) VALUES ($1,$1,'lead') ON CONFLICT DO NOTHING`, inb)
	}
	_ = storage.Audit(ctx, a.DB.Pool, storage.Actor{ID: "executor", Type: "system"}, "create", "opportunity", id, map[string]any{"from_action": act.ID})
	return id, nil
}

func (a *App) qualifyTender(ctx context.Context, tenderID string) (string, error) {
	var title, agency string
	var hps float64
	if err := a.DB.Pool.QueryRow(ctx, `SELECT title, agency, hps::float8 FROM tenders WHERE id=$1`, tenderID).Scan(&title, &agency, &hps); err != nil {
		return "", err
	}
	accID := "tender-" + storage.Hash(agency)[:8]
	if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO accounts(id,name,sector,branch,is_government) VALUES ($1,$2,'Pemerintah · Tender','Semarang',true) ON CONFLICT (id) DO NOTHING`, accID, agency); err != nil {
		return "", err
	}
	id := "opp-" + storage.Hash(tenderID)[:10]
	if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO opportunities(id,account_id,name,expected_revenue,probability,stage_id,source,status,lead_at,tags,closing_label)
		VALUES ($1,$2,$3,$4,20,(SELECT id FROM stage_definitions WHERE name='Baru'),'tender','open',$5,'{Tender}',(SELECT 'Tutup ' || to_char(deadline,'DD Mon') FROM tenders WHERE id=$6)) ON CONFLICT (id) DO NOTHING`,
		id, accID, title, hps, domain.Now(), tenderID); err != nil {
		return "", err
	}
	_, err := a.DB.Pool.Exec(ctx, `UPDATE tenders SET status='lead', updated_at=now() WHERE id=$1`, tenderID)
	return id, err
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
