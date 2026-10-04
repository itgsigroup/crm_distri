package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"arc/packages/connectors/whatsapp"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

// IngestResult tells what happened to one WaEvent.
type IngestResult struct {
	Stored    bool   `json:"stored"`
	Duplicate bool   `json:"duplicate"`
	Skipped   string `json:"skipped,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	InboundID string `json:"inbound_id,omitempty"`
}

func (a *App) rule(ctx context.Context, id string) bool {
	var on bool
	if err := a.DB.Pool.QueryRow(ctx, `SELECT enabled FROM privacy_rules WHERE id=$1`, id).Scan(&on); err != nil {
		return true
	}
	return on
}

// internalPhone reports whether a phone belongs to GSI (registry or a sales number).
func (a *App) internalPhone(ctx context.Context, phone string) (bool, string) {
	n := domain.NormalizePhone(phone)
	if n == "" {
		return false, ""
	}
	var name string
	if a.DB.Pool.QueryRow(ctx, `SELECT name FROM internal_numbers WHERE phone_norm=$1`, n).Scan(&name) == nil {
		return true, name
	}
	if a.DB.Pool.QueryRow(ctx, `SELECT u.name FROM users u WHERE EXISTS (SELECT 1 FROM unnest(u.wa_numbers) x WHERE regexp_replace(x,'[^0-9]','','g')=$1)
		UNION SELECT COALESCE(u.name,w.label) FROM wa_sessions w LEFT JOIN users u ON u.id=w.user_id WHERE regexp_replace(w.phone,'[^0-9]','','g')=$1 LIMIT 1`, n).Scan(&name) == nil {
		return true, name
	}
	return false, ""
}

func jidPhone(jid string) string { return strings.Split(strings.Split(jid, "@")[0], ":")[0] }

// IngestWaEvent stores one WhatsApp event according to the privacy rules (ADR 0002, docs/knowledge/04).
func (a *App) IngestWaEvent(ctx context.Context, ev whatsapp.WaEvent) (IngestResult, error) {
	var res IngestResult
	if ev.Wamid == "" {
		ev.Wamid = whatsapp.ContentWamid(ev.ChatID, ev.SenderName, ev.Timestamp, ev.Text)
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = domain.Now()
	}
	ref := "wa:" + ev.Wamid
	if a.exists(ctx, `SELECT 1 FROM interactions WHERE raw_ref=$1 OR wamid=$2`, ref, ev.Wamid) {
		res.Duplicate = true
		return res, nil
	}
	// Exports and live events collapse on content: same chat + sender + minute + text.
	if cw := whatsapp.ContentWamid(ev.ChatID, ev.SenderName, ev.Timestamp, ev.Text); a.exists(ctx, `SELECT 1 FROM interactions WHERE raw_ref=$1`, "wa:"+cw) && cw != ev.Wamid {
		res.Duplicate = true
		return res, nil
	}
	// Live events carry the transport's message id, so an export imported later (or a
	// live copy of an exported message whose push name differs from the saved contact
	// name) is matched on chat + direction + minute + text against the other kind.
	if a.exists(ctx, `SELECT 1 FROM interactions i JOIN chat_threads t ON t.id=i.thread_id
		WHERE t.session_id=$1 AND t.chat_jid=$2 AND i.direction=$3 AND ($6 OR i.is_history)
		AND date_trunc('minute', i.occurred_at)=date_trunc('minute', $4::timestamptz) AND btrim(i.body_text)=btrim($5)`,
		ev.Session, ev.ChatID, map[bool]string{true: "out", false: "in"}[ev.FromMe], ev.Timestamp, ev.Text, ev.IsHistory) {
		res.Duplicate = true
		return res, nil
	}
	var userID, sessionLabel string
	if err := a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(user_id,''), label FROM wa_sessions WHERE id=$1`, ev.Session).Scan(&userID, &sessionLabel); err != nil {
		return res, fmt.Errorf("sesi WhatsApp %q tidak dikenal", ev.Session)
	}
	var ownerName string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT name FROM users WHERE id=$1`, userID).Scan(&ownerName)
	senderPhone := ev.From
	if senderPhone == "" {
		senderPhone = jidPhone(ev.ChatID)
	}
	senderInternal, internalName := a.internalPhone(ctx, senderPhone)
	if ev.FromMe {
		senderInternal = true
		ev.SenderName = defaultStr(ownerName, ev.SenderName)
	} else if senderInternal && internalName != "" {
		ev.SenderName = internalName
	}
	var threadID, groupID, accountID, personID, threadType string
	var personIDs []string
	if ev.IsGroup {
		gname := ev.ChatID
		members := []map[string]any{}
		external := false
		if ev.GroupMeta != nil {
			gname = defaultStr(ev.GroupMeta.Name, gname)
			for _, m := range ev.GroupMeta.Members {
				ph := defaultStr(m.Phone, jidPhone(m.JID))
				in, nm := a.internalPhone(ctx, ph)
				if !in {
					external = true
				}
				members = append(members, map[string]any{"n": defaultStr(m.Name, defaultStr(nm, ph)), "phone": ph, "int": in})
			}
		} else if !senderInternal {
			external = true
		}
		gtype := map[bool]string{true: "external", false: "internal"}[external]
		gid := "g-" + storage.Hash(ev.ChatID)[:10]
		var readPolicy bool
		err := a.DB.Pool.QueryRow(ctx, `INSERT INTO chat_groups(id,session_id,jid,name,type,read_policy,members) VALUES ($1,$2,$3,$4,$5,false,$6)
			ON CONFLICT (jid) DO UPDATE SET name=EXCLUDED.name, members=CASE WHEN jsonb_array_length(EXCLUDED.members)>0 THEN EXCLUDED.members ELSE chat_groups.members END,
			type=CASE WHEN jsonb_array_length(EXCLUDED.members)>0 THEN EXCLUDED.type ELSE chat_groups.type END, updated_at=now()
			RETURNING id, read_policy, type, COALESCE(account_id,'')`, gid, ev.Session, ev.ChatID, gname, gtype, storage.JSON(members)).Scan(&groupID, &readPolicy, &gtype, &accountID)
		if err != nil {
			return res, err
		}
		if gtype == "internal" && !a.rule(ctx, "internal_groups_schedule") {
			readPolicy = false
		}
		if gtype == "external" && !a.rule(ctx, "external_groups_full") {
			readPolicy = false
		}
		if !readPolicy {
			// Not opted in: count only, never store content.
			if a.firstSkip(ctx, ev) {
				_, _ = a.DB.Pool.Exec(ctx, `UPDATE chat_groups SET skipped_count=skipped_count+1 WHERE id=$1`, groupID)
			}
			res.Skipped = "group_not_opted_in"
			return res, nil
		}
		threadType = map[string]string{"external": "gext", "internal": "gint"}[gtype]
		threadID = groupID
		var memberCount, ext int
		_ = a.DB.Pool.QueryRow(ctx, `SELECT jsonb_array_length(members), (SELECT count(*) FROM jsonb_array_elements(members) m WHERE NOT (m->>'int')::boolean) FROM chat_groups WHERE id=$1`, groupID).Scan(&memberCount, &ext)
		sub := fmt.Sprintf("%d anggota · %d eksternal · %d internal", memberCount, ext, memberCount-ext)
		if threadType == "gint" {
			sub = fmt.Sprintf("Grup internal · %d anggota", memberCount)
		}
		if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO chat_threads(id,session_id,chat_jid,type,name,subtitle,account_id,group_id,last_at) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$1,$8)
			ON CONFLICT (session_id,chat_jid) DO UPDATE SET subtitle=EXCLUDED.subtitle, updated_at=now()`, threadID, ev.Session, ev.ChatID, threadType, gname, sub, accountID, ev.Timestamp); err != nil {
			return res, err
		}
		_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM chat_threads WHERE session_id=$1 AND chat_jid=$2`, ev.Session, ev.ChatID).Scan(&threadID)
		if !senderInternal {
			_ = a.DB.Pool.QueryRow(ctx, `SELECT id, COALESCE(account_id,'') FROM people WHERE EXISTS (SELECT 1 FROM unnest(phones) p WHERE regexp_replace(p,'[^0-9]','','g')=$1) LIMIT 1`,
				domain.NormalizePhone(senderPhone)).Scan(&personID, &accountID)
		} else {
			_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM people WHERE is_internal AND EXISTS (SELECT 1 FROM unnest(phones) p WHERE regexp_replace(p,'[^0-9]','','g')=$1) LIMIT 1`, domain.NormalizePhone(senderPhone)).Scan(&personID)
		}
	} else {
		other := jidPhone(ev.ChatID)
		if ev.FromMe && ev.To != "" {
			other = ev.To
		}
		otherInternal, otherName := a.internalPhone(ctx, other)
		if otherInternal && a.rule(ctx, "skip_internal_private") {
			// Private chat between employees: keep the thread marker, never the content.
			tid := "c-" + storage.Hash(ev.Session, ev.ChatID)[:10]
			inc := 0
			if a.firstSkip(ctx, ev) {
				inc = 1
			}
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO chat_threads(id,session_id,chat_jid,type,name,subtitle,is_private,last_at,skipped_count) VALUES ($1,$2,$3,'internal',$4,'internal · tidak dibaca',true,$5,$6)
				ON CONFLICT (session_id,chat_jid) DO UPDATE SET skipped_count=chat_threads.skipped_count+$6, last_at=GREATEST(chat_threads.last_at, EXCLUDED.last_at)`, tid, ev.Session, ev.ChatID, defaultStr(otherName, other), ev.Timestamp, inc)
			res.Skipped = "internal_private"
			return res, nil
		}
		var private bool
		var pname, prole, accName string
		err := a.DB.Pool.QueryRow(ctx, `SELECT p.id, COALESCE(p.account_id,''), p.is_private, p.name, p.role, COALESCE(a.name,'') FROM people p LEFT JOIN accounts a ON a.id=p.account_id
			WHERE EXISTS (SELECT 1 FROM unnest(p.phones) x WHERE regexp_replace(x,'[^0-9]','','g')=$1) LIMIT 1`, domain.NormalizePhone(other)).Scan(&personID, &accountID, &private, &pname, &prole, &accName)
		if err == nil && private && a.rule(ctx, "skip_marked_private") {
			res.Skipped = "marked_private"
			return res, nil
		}
		if personID == "" {
			if !ev.FromMe && !ev.IsHistory {
				res.InboundID = a.upsertInbound(ctx, other, ev, userID)
			}
			if a.rule(ctx, "odoo_contacts_only") {
				res.Skipped = "unknown_number"
				return res, nil
			}
			pname = defaultStr(ev.SenderName, "+"+other)
		}
		threadType = "cust"
		threadID = "c-" + storage.Hash(ev.Session, ev.ChatID)[:10]
		sub := strings.Trim(accName+" · "+prole, " ·")
		if personID == "" {
			sub = "Nomor baru · belum teridentifikasi"
		}
		if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO chat_threads(id,session_id,chat_jid,type,name,subtitle,account_id,person_id,last_at) VALUES ($1,$2,$3,'cust',$4,$5,NULLIF($6,''),NULLIF($7,''),$8)
			ON CONFLICT (session_id,chat_jid) DO NOTHING`, threadID, ev.Session, ev.ChatID, pname, sub, accountID, personID, ev.Timestamp); err != nil {
			return res, err
		}
		_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM chat_threads WHERE session_id=$1 AND chat_jid=$2`, ev.Session, ev.ChatID).Scan(&threadID)
	}
	if personID != "" {
		personIDs = []string{personID}
	}
	channel := "wa_message"
	if ev.IsGroup {
		channel = "wa_group_message"
	}
	dir := "in"
	if ev.FromMe {
		dir = "out"
	}
	media := []any{}
	if ev.Media != nil {
		media = append(media, ev.Media)
	}
	var acc any
	if accountID != "" {
		acc = accountID
	}
	tag, err := a.DB.Pool.Exec(ctx, `INSERT INTO interactions(channel,direction,occurred_at,person_ids,user_ids,thread_id,group_id,body_text,attachments,raw_ref,wamid,account_id,sender_name,sender_phone,sender_internal,transport,is_history)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT DO NOTHING`,
		channel, dir, ev.Timestamp, nzs(personIDs), []string{userID}, threadID, groupID, ev.Text, storage.JSON(media), ref, ev.Wamid, acc, ev.SenderName,
		domain.NormalizePhone(senderPhone), senderInternal, ev.Transport, ev.IsHistory)
	if err != nil {
		return res, err
	}
	if tag.RowsAffected() == 0 {
		res.Duplicate = true
		return res, nil
	}
	res.Stored, res.ThreadID = true, threadID
	unread := 0
	if dir == "in" && !ev.IsHistory {
		unread = 1
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE chat_threads SET last_at=GREATEST(COALESCE(last_at,$2),$2), unread=unread+$3, updated_at=now() WHERE id=$1`, threadID, ev.Timestamp, unread)
	if accountID != "" && threadType != "gint" {
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE accounts SET last_interaction_at=GREATEST(COALESCE(last_interaction_at,$2),$2), last_via='chat' WHERE id=$1`, accountID, ev.Timestamp)
	}
	if personID != "" {
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE people SET last_contact_at=GREATEST(COALESCE(last_contact_at,$2),$2) WHERE id=$1`, personID, ev.Timestamp)
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE wa_sessions SET last_event_at=now(), messages_30d=messages_30d+1 WHERE id=$1`, ev.Session)
	if !ev.IsHistory && a.debounce != nil {
		a.debounce.trigger(threadID)
	}
	return res, nil
}

func nzs(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// firstSkip records a skipped message by hash(session, wamid) and reports whether
// it is new, so replays never inflate skip counters. No content is stored.
func (a *App) firstSkip(ctx context.Context, ev whatsapp.WaEvent) bool {
	if ev.Wamid == "" {
		return true
	}
	tag, err := a.DB.Pool.Exec(ctx, `INSERT INTO wa_skipped(key) VALUES ($1) ON CONFLICT DO NOTHING`, storage.Hash(ev.Session, ev.Wamid))
	return err != nil || tag.RowsAffected() > 0
}

// upsertInbound records a new inbound number for the Identity agent (inbound only).
func (a *App) upsertInbound(ctx context.Context, phone string, ev whatsapp.WaEvent, userID string) string {
	id := "in-" + storage.Hash(domain.NormalizePhone(phone))[:10]
	tag, err := a.DB.Pool.Exec(ctx, `INSERT INTO inbound_contacts(id,phone,phone_norm,first_message,via_user_id,received_at,identification,status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'unknown') ON CONFLICT (phone_norm) DO NOTHING`,
		id, "+"+domain.NormalizePhone(phone), domain.NormalizePhone(phone), ev.Text, userID, ev.Timestamp,
		storage.JSONObj(map[string]any{"name": defaultStr(ev.SenderName, "Belum teridentifikasi"), "role": "—", "company": "—", "sources": []any{}}))
	if err != nil {
		return ""
	}
	_ = a.DB.Pool.QueryRow(ctx, `SELECT id FROM inbound_contacts WHERE phone_norm=$1`, domain.NormalizePhone(phone)).Scan(&id)
	if tag.RowsAffected() > 0 {
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO funnel_events(inbound_id,subject_key,stage,channel,at) VALUES ($1,$1,'masuk','wa',$2) ON CONFLICT DO NOTHING`, id, ev.Timestamp)
		go func(id string) {
			cctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if _, err := a.Agents.IdentifyInbound(cctx, id); err != nil {
				a.Log.Warn("identify inbound failed", "id", id, "err", err)
			}
		}(id)
	}
	return id
}

// afterCapture refreshes scoring for the account of a thread after new messages were understood.
func (a *App) afterCapture(ctx context.Context, threadID string) {
	var opp string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT o.id FROM chat_threads t JOIN opportunities o ON o.account_id=t.account_id AND o.status='open' AND NOT o.historical WHERE t.id=$1 ORDER BY o.expected_revenue DESC LIMIT 1`, threadID).Scan(&opp)
	if opp != "" {
		if _, err := a.Agents.ScoreOpportunities(ctx, opp); err != nil {
			a.Log.Warn("score after capture failed", "err", err)
		}
	}
}

// handleBridgeWebhook receives WaEvents (single, batch) and session status from wa-bridge.
func (a *App) handleBridgeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "body")
		return
	}
	if !whatsapp.Verify(a.Cfg.BridgeSecret, body, r.Header.Get("X-ARC-Signature")) {
		writeErr(w, http.StatusUnauthorized, "signature tidak valid")
		return
	}
	var env struct {
		Type   string                  `json:"type"`
		Event  *whatsapp.WaEvent       `json:"event"`
		Events []whatsapp.WaEvent      `json:"events"`
		Status *whatsapp.SessionStatus `json:"status"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		writeErr(w, http.StatusBadRequest, "format tidak valid")
		return
	}
	ctx := r.Context()
	if env.Status != nil {
		st := env.Status
		var qrExp any
		if st.QR != "" {
			qrExp = domain.Now().Add(60 * time.Second)
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE wa_sessions SET status=$2, phone=CASE WHEN $3<>'' THEN $3 ELSE phone END, qr_code=$4, qr_expires_at=$5, last_event_at=now(), updated_at=now() WHERE id=$1`,
			st.Session, st.Status, st.Phone, st.QR, qrExp)
		_ = storage.Audit(ctx, a.DB.Pool, storage.Actor{ID: "wa-bridge", Type: "machine"}, "session."+st.Status, "wa_session", st.Session, nil)
	}
	events := env.Events
	if env.Event != nil {
		events = append(events, *env.Event)
	}
	out := []IngestResult{}
	for _, ev := range events {
		ev.Transport = defaultStr(ev.Transport, "bridge")
		res, err := a.IngestWaEvent(ctx, ev)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": out})
}

// handleBridgeActionCheck lets the bridge confirm that an action is approved before sending.
func (a *App) handleBridgeActionCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !whatsapp.Verify(a.Cfg.BridgeSecret, []byte(id), r.Header.Get("X-ARC-Signature")) {
		writeErr(w, http.StatusUnauthorized, "signature tidak valid")
		return
	}
	var status string
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT status FROM actions WHERE id=$1`, id).Scan(&status); err != nil {
		writeErr(w, http.StatusNotFound, "action tidak ditemukan")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": status, "approved": status == "approved" || status == "edited"})
}

// handleCloudWebhook implements Meta's verification (GET) and message webhook (POST).
func (a *App) handleCloudWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		if q.Get("hub.mode") == "subscribe" && q.Get("hub.verify_token") == a.Cfg.WACloudVerifyToken {
			_, _ = w.Write([]byte(q.Get("hub.challenge")))
			return
		}
		writeErr(w, http.StatusForbidden, "verify token salah")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	secret := a.Cfg.WACloudAppSecret
	if secret == "" || !whatsapp.Verify(secret, body, r.Header.Get("X-Hub-Signature-256")) {
		writeErr(w, http.StatusUnauthorized, "signature tidak valid")
		return
	}
	var session string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT id FROM wa_sessions WHERE transport='cloud' ORDER BY created_at LIMIT 1`).Scan(&session)
	evs, err := whatsapp.ParseCloudWebhook(body, session)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, ev := range evs {
		if _, err := a.IngestWaEvent(r.Context(), ev); err != nil {
			a.Log.Warn("cloud ingest failed", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleExportUpload ingests a WhatsApp "Ekspor chat" file as history.
func (a *App) handleExportUpload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	session, chat := q.Get("session"), q.Get("chat_jid")
	if session == "" || chat == "" {
		writeErr(w, http.StatusBadRequest, "parameter session dan chat_jid wajib")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var own string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT COALESCE(u.name,'') FROM wa_sessions s LEFT JOIN users u ON u.id=s.user_id WHERE s.id=$1`, session).Scan(&own)
	evs, err := whatsapp.ParseExport(data, whatsapp.ExportOptions{Session: session, ChatJID: chat, IsGroup: strings.HasSuffix(chat, "@g.us"), OwnName: defaultStr(q.Get("own_name"), own),
		MonthFirst: q.Get("format") == "en", Location: domain.Jakarta})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	stored, dup := 0, 0
	for _, ev := range evs {
		if !ev.FromMe {
			ev.From = jidPhone(chat)
		}
		res, err := a.IngestWaEvent(r.Context(), ev)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if res.Stored {
			stored++
		} else if res.Duplicate {
			dup++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"parsed": len(evs), "stored": stored, "duplicates": dup, "toast": fmt.Sprintf("%d pesan riwayat diimpor · %d duplikat dilewati", stored, dup)})
}

// emitWebhook delivers an outbound event to subscribers with an HMAC signature.
func (a *App) emitWebhook(ctx context.Context, event string, payload any) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, url, secret FROM webhook_subscriptions WHERE active AND $1 = ANY(events)`, event)
	if err != nil {
		return
	}
	type sub struct{ id, url, secret string }
	var subs []sub
	for rows.Next() {
		var s sub
		_ = rows.Scan(&s.id, &s.url, &s.secret)
		subs = append(subs, s)
	}
	rows.Close()
	body, _ := json.Marshal(map[string]any{"event": event, "at": domain.Now().Format(time.RFC3339), "data": payload})
	for _, s := range subs {
		sig := whatsapp.Sign(s.secret, body)
		go func(s sub) {
			cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(cctx, http.MethodPost, s.url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-ARC-Event", event)
			req.Header.Set("X-ARC-Signature", "sha256="+sig)
			code, msg := 0, ""
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				msg = err.Error()
			} else {
				code = resp.StatusCode
				resp.Body.Close()
			}
			_, _ = a.DB.Pool.Exec(context.Background(), `INSERT INTO webhook_deliveries(subscription_id,event,payload,signature,status_code,error) VALUES ($1,$2,$3,$4,$5,$6)`,
				s.id, event, body, sig, code, msg)
		}(s)
	}
}
