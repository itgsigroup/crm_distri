package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"arc/packages/connectors/whatsapp"
	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

func (a *App) publicRoutes(mux *http.ServeMux) {
	read := func(h http.HandlerFunc) http.HandlerFunc { return a.requireAuth(domain.ScopeRead, h) }
	mux.HandleFunc("POST /api/v1/ask", read(a.handleAsk))
	mux.HandleFunc("GET /api/v1/accounts/{id}/brief", read(a.handleAccountBriefV1))
	mux.HandleFunc("GET /api/v1/signals", a.requireAuth("", a.handleSignalsV1))
	mux.HandleFunc("GET /api/v1/actions", read(a.handleActions))
	mux.HandleFunc("POST /api/v1/actions", a.requireAuth(domain.ScopePropose, a.handleProposeV1))
	mux.HandleFunc("POST /api/v1/actions/{id}/decision", a.requireAuth(domain.ScopeHuman, a.handleDecision))
	mux.HandleFunc("POST /api/v1/events", a.requireAuth(domain.ScopeEvents, a.handleEventsV1))
	mux.HandleFunc("GET /openapi.json", a.handleOpenAPI)
}

// handleAccountBriefV1 serves the account brief with its evidence list (same as the MCP tool).
func (a *App) handleAccountBriefV1(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.canSeeAccount(r, id) {
		writeErr(w, 403, "akun ini di luar cabang Anda")
		return
	}
	out, err := a.accountBrief(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (a *App) handleSignalsV1(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if !p.Can(domain.ScopeRead) && !p.Can(domain.ScopeSignals) {
		writeErr(w, 403, "scope read atau signals diperlukan")
		return
	}
	since := domain.Now().AddDate(0, 0, -7)
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		} else if t, err := time.Parse("2006-01-02", s); err == nil {
			since = t
		}
	}
	sc := p.Scope()
	cond, args := sc.SQL("a", 2)
	rows, err := a.DB.Pool.Query(r.Context(), `SELECT s.id, s.type, s.severity, s.title, s.detail, COALESCE(s.account_id,''), COALESCE(a.name,''), s.detected_at, s.confidence::float8, s.evidence
		FROM signals s LEFT JOIN accounts a ON a.id=s.account_id WHERE s.resolved_at IS NULL AND s.detected_at >= $1 AND (s.account_id IS NULL OR `+cond+`) ORDER BY s.detected_at DESC`, append([]any{since}, args...)...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var typ, sev, title, detail, acc, an string
		var at time.Time
		var conf float64
		var ev json.RawMessage
		_ = rows.Scan(&id, &typ, &sev, &title, &detail, &acc, &an, &at, &conf, &ev)
		out = append(out, map[string]any{"id": id, "type": typ, "severity": sev, "title": title, "detail": detail, "account_id": acc, "account": an, "detected_at": at, "confidence": conf, "evidence": ev})
	}
	writeJSON(w, 200, out)
}

func (a *App) handleProposeV1(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	var req struct {
		AccountID string `json:"account_id"`
		Title     string `json:"title"`
		Why       string `json:"why"`
		Preview   string `json:"preview"`
		Type      string `json:"type"`
		Source    string `json:"source"`
	}
	if err := decode(r, &req); err != nil || req.Title == "" || req.Source == "" {
		writeErr(w, 400, "title dan source (bukti) wajib")
		return
	}
	if req.AccountID != "" && !a.canSeeAccountID(r.Context(), p, req.AccountID) {
		writeErr(w, 403, "akun di luar hak akses")
		return
	}
	id, created, err := a.Actions.Propose(r.Context(), actions.Proposal{Agent: "Klien API (" + p.KeyID + ")", Type: defaultStr(req.Type, "task"), Kind: "task", Icon: "i-spark", AccountID: req.AccountID,
		Title: req.Title, Why: req.Why, Preview: req.Preview, Prep: "Diusulkan lewat ARC AI API.", Steps: []string{"Masuk antrean approval"}, InQueue: true,
		Evidence: []domain.Evidence{{Source: req.Source, Quote: req.Why}}, Confidence: 0.7}, p.Actor())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "created": created, "status": "proposed"})
}

// handleEventsV1 ingests external events (ERP, WhatsApp gateway, web form, distribution credit requests). Idempotent by (source, external_id).
func (a *App) handleEventsV1(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, _ := PrincipalFrom(ctx)
	var ev struct {
		Source     string         `json:"source"`
		Type       string         `json:"type"`
		ExternalID string         `json:"external_id"`
		Data       map[string]any `json:"data"`
	}
	if err := decode(r, &ev); err != nil || ev.Type == "" || ev.ExternalID == "" {
		writeErr(w, 400, "type dan external_id wajib")
		return
	}
	ev.Source = defaultStr(ev.Source, p.KeyID)
	tag, err := a.DB.Pool.Exec(ctx, `INSERT INTO external_events(source,type,external_id,payload) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, ev.Source, ev.Type, ev.ExternalID, storage.JSONObj(ev.Data))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 200, map[string]any{"duplicate": true})
		return
	}
	d := ev.Data
	result := map[string]any{"accepted": true}
	switch ev.Type {
	case "credit_release_request":
		id, err := a.creditGuardrail(ctx, p, ev.ExternalID, d)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		result["action_id"] = id
	case "payment_received":
		acc := argStr(d, "account_id")
		amount := toFloat(d["amount"])
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO interactions(channel,direction,occurred_at,body_text,raw_ref,account_id,inference,extracted) VALUES ('erp_event','in',$1,$2,$3,NULLIF($4,''),$5,true) ON CONFLICT DO NOTHING`,
			domain.Now(), fmt.Sprintf("Pembayaran %s diterima", fmtRp(amount)), "event:"+ev.Source+":"+ev.ExternalID, acc, "Sinyal: kesehatan pembayaran")
		if inv := argStr(d, "invoice"); inv != "" {
			_, _ = a.DB.Pool.Exec(ctx, `UPDATE invoices SET residual=GREATEST(0, residual-$2), paid_at=CASE WHEN residual-$2 <= 0 THEN $3::date ELSE paid_at END WHERE number=$1`, inv, amount, domain.Now())
		}
		if acc != "" {
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO signals(type,severity,account_id,title,detail,evidence,dedupe_key) VALUES ('payment_on_time','good',$1,'Pembayaran tepat waktu',$2,$3,$4) ON CONFLICT (dedupe_key) DO UPDATE SET detected_at=now(), resolved_at=NULL`,
				acc, fmt.Sprintf("%s diterima", fmtRp(amount)), storage.JSON([]domain.Evidence{{Source: "erp:" + ev.ExternalID, Quote: fmtRp(amount)}}), "payment_on_time:"+acc)
		}
		a.emitWebhook(ctx, "cash.forecast.changed", map[string]any{"account_id": acc, "amount": amount})
	case "web_form":
		phone := argStr(d, "phone")
		if phone == "" {
			writeErr(w, 400, "phone wajib untuk form web")
			return
		}
		res, err := a.IngestWaEvent(ctx, whatsapp.WaEvent{Wamid: "form-" + ev.ExternalID, Session: firstSession(a, r), From: domain.NormalizePhone(phone),
			ChatID: domain.NormalizePhone(phone) + "@s.whatsapp.net", SenderName: argStr(d, "name"), Text: argStr(d, "message"), Timestamp: domain.Now(), Transport: "form"})
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		result["inbound_id"] = res.InboundID
	case "wa_message":
		var wev whatsapp.WaEvent
		raw, _ := json.Marshal(d)
		_ = json.Unmarshal(raw, &wev)
		res, err := a.IngestWaEvent(ctx, wev)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		result["ingest"] = res
	}
	_ = storage.Audit(ctx, a.DB.Pool, p.Actor(), "event:"+ev.Type, "external_event", ev.ExternalID, nil)
	writeJSON(w, 202, result)
}

func firstSession(a *App, r *http.Request) string {
	var id string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT id FROM wa_sessions ORDER BY created_at LIMIT 1`).Scan(&id)
	return id
}

// creditGuardrail turns a distribution credit release request into a policy decision (SOP-SEC-001).
func (a *App) creditGuardrail(ctx interface {
	Done() <-chan struct{}
	Err() error
	Value(any) any
	Deadline() (time.Time, bool)
}, p Principal, ext string, d map[string]any) (string, error) {
	acc := argStr(d, "account_id")
	amount := toFloat(d["amount"])
	if acc == "" || amount <= 0 {
		return "", fmt.Errorf("account_id dan amount wajib")
	}
	var name string
	var limit, open float64
	var avg int
	var overdue, phone string
	err := a.DB.Pool.QueryRow(ctx, `SELECT a.name, COALESCE(c.credit_limit, $2), COALESCE(c.open_receivable,0)::float8, COALESCE(c.avg_days_to_pay,30), COALESCE(c.overdue_note,''), COALESCE(c.registered_phone,'')
		FROM accounts a LEFT JOIN credit_profiles c ON c.account_id=a.id WHERE a.id=$1`, acc, a.Ins.PolicyFloat(ctx, "credit_limit_default", 250e6)).Scan(&name, &limit, &open, &avg, &overdue, &phone)
	if err != nil {
		return "", fmt.Errorf("akun tidak ditemukan")
	}
	after := open + amount
	check := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗ (belum)"
	}
	poOK, _ := d["po_verified_phone"].(bool)
	addrOK, _ := d["address_consistent"].(bool)
	note := fmt.Sprintf("SOP-SEC-001: PO asli terverifikasi lewat telepon ke nomor kantor terdaftar %s, alamat kirim sama dengan pengiriman sebelumnya %s.", check(poOK), check(addrOK))
	if overdue != "" {
		note += " Risiko konsentrasi piutang: " + overdue + "."
	}
	if after > limit {
		note += " Usulan: rilis dengan DP 50% atau setelah invoice tersebut lunas."
	}
	impact := []actions.Impact{{Label: "Piutang berjalan", Value: fmtRp(open)}, {Label: "Limit kredit", Value: fmtRp(limit)}, {Label: "Setelah rilis", Value: fmtRp(after), Tone: map[bool]string{true: "bad", false: "good"}[after > limit]},
		{Label: "Rata-rata bayar", Value: fmt.Sprintf("%d hari", avg), Tone: map[bool]string{true: "warn", false: ""}[avg > 35]}}
	by := defaultStr(argStr(d, "requested_by"), p.Name)
	id, _, err := a.Actions.Propose(ctx, actions.Proposal{ID: "credit-" + storage.Hash(ext)[:12], Agent: domain.AgentCredit, Type: "credit_release", Kind: "policy", Icon: "i-shield",
		ButtonLabel: "Setujui dengan DP 50%", AccountID: acc, DueLabel: "Hari ini", Title: "Pelepasan barang kredit — " + name,
		Summary: fmt.Sprintf("%s meminta rilis %v (%s) dengan termin %v hari. %s", by, defaultStr(argStr(d, "items"), "barang"), fmtRp(amount), defaultStr(argStr(d, "term_days"), "30"),
			map[bool]string{true: "Setelah rilis, exposure akun ini melewati limit kredit.", false: "Exposure masih di bawah limit kredit."}[after > limit]),
		Why: "Permintaan pelepasan barang kredit distribusi.", Prep: "Checklist SOP-SEC-001 dijalankan dan exposure dihitung dari data piutang.", ContextNote: note, Impact: impact,
		Steps: []string{"Keputusan dicatat di audit log", by + " & gudang diberi tahu", "SO dibuat dengan syarat yang dipilih"},
		Options: []actions.Option{{Key: "approve_with_dp", Label: "Setujui dengan DP 50%", Primary: true, Result: "Disetujui dengan DP 50% · gudang menunggu bukti transfer", Toast: "Keputusan dicatat di audit log & Odoo"},
			{Key: "hold", Label: "Tahan sampai invoice lunas", Result: "Ditahan · penagihan invoice berjalan dulu; ARC memantau pembayarannya", Toast: "Keputusan dicatat di audit log & Odoo"},
			{Key: "reject", Label: "Tolak rilis", Result: "Rilis ditolak · " + by + " diberi tahu", Toast: "Keputusan dicatat di audit log"}},
		Tags:    []actions.Pill{{K: "bad", T: "Credit guardrail"}, {K: "neutral", T: "Diajukan " + by + " · distribusi"}},
		Payload: map[string]any{"amount": amount, "registered_phone": phone}, InQueue: true, Confidence: 0.9,
		Evidence: []domain.Evidence{{Source: "event:" + ext, Quote: fmt.Sprintf("rilis %s, limit %s", fmtRp(amount), fmtRp(limit))}}}, p.Actor())
	return id, err
}

func (a *App) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	bearer := map[string]any{"bearerAuth": []string{}}
	op := func(summary string, body bool) map[string]any {
		o := map[string]any{"summary": summary, "security": []any{bearer}, "responses": map[string]any{"200": map[string]any{"description": "OK"}, "401": map[string]any{"description": "Tidak terautentikasi"}, "403": map[string]any{"description": "Scope tidak cukup / human-only"}}}
		if body {
			o["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}}
		}
		return o
	}
	idParam := []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}}}
	brief := op("Memori akun, stakeholder, komitmen, next action", false)
	brief["parameters"] = idParam
	decision := op("approve · reject + alasan · edit — hanya sesi manusia (scope human)", true)
	decision["parameters"] = idParam
	ask := op("Pertanyaan bebas → jawaban + bukti", true)
	ask["operationId"] = "ask"
	ask["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "required": []string{"question"},
		"properties": map[string]any{"question": map[string]string{"type": "string"}, "evidence": map[string]string{"type": "boolean"}}}}}}
	doc := map[string]any{
		"openapi":    "3.1.0",
		"info":       map[string]any{"title": "ARC AI API", "version": Version, "description": "API ARC (Agentic Relationship Core) GSI. AI mengusulkan, manusia memutuskan: endpoint keputusan menolak key mesin."},
		"servers":    []any{map[string]string{"url": strings.TrimRight(a.Cfg.PublicURL, "/") + "/api/v1"}},
		"components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer", "description": "API key arc_live_… atau token OAuth"}}},
		"paths": map[string]any{
			"/ask":                   map[string]any{"post": ask},
			"/accounts/{id}/brief":   map[string]any{"get": brief},
			"/signals":               map[string]any{"get": op("Sinyal baru (since=YYYY-MM-DD)", false)},
			"/actions":               map[string]any{"get": op("Saran agen (status=pending)", false), "post": op("Usulkan tindakan (scope propose, wajib source)", true)},
			"/actions/{id}/decision": map[string]any{"post": decision},
			"/events":                map[string]any{"post": op("Kejadian dari luar: credit_release_request, payment_received, web_form, wa_message (scope events)", true)},
		},
		"webhooks": map[string]any{"action.proposed": map[string]any{}, "commitment.overdue": map[string]any{}, "signal.detected": map[string]any{}, "cash.forecast.changed": map[string]any{}},
	}
	writeJSON(w, 200, doc)
}

// ---------------- Google OAuth (Gmail & Calendar consent per user) ----------------

func (a *App) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	if a.Cfg.GoogleClientID == "" {
		writeErr(w, 400, "Google OAuth belum dikonfigurasi (GOOGLE_CLIENT_ID/SECRET) — capture email memakai mode mock")
		return
	}
	p, _ := PrincipalFrom(r.Context())
	q := url.Values{"client_id": {a.Cfg.GoogleClientID}, "redirect_uri": {a.Cfg.GoogleRedirectURL}, "response_type": {"code"}, "access_type": {"offline"}, "prompt": {"consent"},
		"scope": {"https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/gmail.compose https://www.googleapis.com/auth/calendar.readonly"}, "state": {p.UserID}}
	http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode(), http.StatusFound)
}

func (a *App) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	code := r.URL.Query().Get("code")
	if code == "" || a.Cfg.GoogleClientID == "" {
		writeErr(w, 400, "kode otorisasi tidak ada")
		return
	}
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{"code": {code}, "client_id": {a.Cfg.GoogleClientID}, "client_secret": {a.Cfg.GoogleClientSecret},
		"redirect_uri": {a.Cfg.GoogleRedirectURL}, "grant_type": {"authorization_code"}})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		writeErr(w, 502, "pertukaran token gagal")
		return
	}
	enc, err := encrypt(a.Cfg.EncryptKey, tok.AccessToken)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `INSERT INTO oauth_google_tokens(user_id,enc_token,scopes) VALUES ($1,$2,$3) ON CONFLICT (user_id) DO UPDATE SET enc_token=EXCLUDED.enc_token, consent_at=now()`,
		p.UserID, enc, []string{"gmail.readonly", "gmail.compose", "calendar.readonly"})
	_ = storage.Audit(r.Context(), a.DB.Pool, p.Actor(), "consent.google", "user", p.UserID, nil)
	http.Redirect(w, r, "/#conn/sec-sumber", http.StatusFound)
}
