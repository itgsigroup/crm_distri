package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
	"arc/packages/mcp"
)

func (a *App) mcpRoutes(mux *http.ServeMux) {
	srv := &mcp.Server{Version: Version, Call: a.callTool, Enabled: a.mcpGroupState}
	h := a.requireAuth("", srv.ServeHTTP)
	mux.HandleFunc("POST /mcp", h)
	mux.HandleFunc("GET /mcp", h)
	mux.HandleFunc("DELETE /mcp", h)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", a.handleResourceMeta)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", a.handleResourceMeta)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", a.handleAuthServerMeta)
	mux.HandleFunc("POST /oauth/register", a.handleOAuthRegister)
	mux.HandleFunc("GET /oauth/authorize", a.handleOAuthAuthorize)
	mux.HandleFunc("POST /oauth/authorize", a.handleOAuthAuthorize)
	mux.HandleFunc("POST /oauth/token", a.handleOAuthToken)
}

func (a *App) base() string { return strings.TrimRight(a.Cfg.PublicURL, "/") }

func (a *App) handleResourceMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"resource": a.base() + "/mcp", "authorization_servers": []string{a.base()}, "bearer_methods_supported": []string{"header"},
		"scopes_supported": []string{"read", "propose"}, "resource_name": "ARC Relationship Core"})
}

func (a *App) handleAuthServerMeta(w http.ResponseWriter, r *http.Request) {
	b := a.base()
	writeJSON(w, 200, map[string]any{"issuer": b, "authorization_endpoint": b + "/oauth/authorize", "token_endpoint": b + "/oauth/token", "registration_endpoint": b + "/oauth/register",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{"read", "propose"}})
}

// handleOAuthRegister implements RFC 7591 dynamic registration for public clients.
func (a *App) handleOAuthRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := decode(r, &req); err != nil || len(req.RedirectURIs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_client_metadata"})
		return
	}
	for _, u := range req.RedirectURIs {
		pu, err := url.Parse(u)
		if err != nil || (pu.Scheme != "https" && pu.Hostname() != "localhost" && pu.Hostname() != "127.0.0.1") {
			writeJSON(w, 400, map[string]string{"error": "invalid_redirect_uri"})
			return
		}
	}
	id := "mcp-" + randomToken(8)
	if _, err := a.DB.Pool.Exec(r.Context(), `INSERT INTO oauth_clients(client_id,client_name,redirect_uris) VALUES ($1,$2,$3)`, id, req.ClientName, req.RedirectURIs); err != nil {
		writeJSON(w, 500, map[string]string{"error": "server_error"})
		return
	}
	writeJSON(w, 201, map[string]any{"client_id": id, "client_name": req.ClientName, "redirect_uris": req.RedirectURIs, "token_endpoint_auth_method": "none",
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
}

var consentTmpl = template.Must(template.New("c").Parse(`<!doctype html><html lang="id"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ARC · Izinkan akses</title>
<style>body{font-family:-apple-system,"Instrument Sans",sans-serif;background:#F5F5F7;color:#1D1D1F;display:grid;place-items:center;min-height:100vh;margin:0;padding:16px}
.c{background:#fff;border-radius:18px;box-shadow:0 10px 30px -12px rgba(20,20,40,.1);padding:24px;max-width:400px;width:100%}h1{font-size:20px;margin:0 0 8px}p{color:#6E6E73;font-size:14px;line-height:1.5}
input{width:100%;box-sizing:border-box;height:38px;border-radius:10px;border:1px solid rgba(20,20,40,.13);padding:0 12px;margin:6px 0 10px;font:inherit}
button{background:#0071E3;color:#fff;border:0;border-radius:999px;height:36px;padding:0 18px;font-weight:600;font:inherit;cursor:pointer}.err{color:#E5342A;font-size:13px}</style></head>
<body><form class="c" method="post">{{range $k,$v := .Hidden}}<input type="hidden" name="{{$k}}" value="{{$v}}">{{end}}
<h1>{{if .User}}Izinkan {{.Client}}?{{else}}Masuk ke ARC{{end}}</h1>
{{if .User}}<p><b>{{.Client}}</b> akan bisa membaca data ARC yang boleh dilihat <b>{{.User}}</b> dan mengusulkan tindakan. Semua tindakan tetap menunggu keputusan manusia di ARC; klien AI tidak bisa menyetujui apa pun.</p><input type="hidden" name="consent" value="yes"><button>Izinkan</button>
{{else}}<p>Login untuk menghubungkan {{.Client}} ke ARC.</p>{{if .Err}}<div class="err">{{.Err}}</div>{{end}}<input name="email" type="email" placeholder="Email" required><input name="password" type="password" placeholder="Kata sandi" required><button>Masuk</button>{{end}}
</form></body></html>`))

// handleOAuthAuthorize: login (if needed) + consent, then issue an authorization code (PKCE S256 required).
func (a *App) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	get := func(k string) string { return r.Form.Get(k) }
	clientID, redirect, challenge, method, state := get("client_id"), get("redirect_uri"), get("code_challenge"), get("code_challenge_method"), get("state")
	var name string
	var uris []string
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT client_name, redirect_uris FROM oauth_clients WHERE client_id=$1`, clientID).Scan(&name, &uris); err != nil {
		http.Error(w, "client tidak dikenal", http.StatusBadRequest)
		return
	}
	okURI := false
	for _, u := range uris {
		if u == redirect {
			okURI = true
		}
	}
	if !okURI || challenge == "" || method != "S256" || get("response_type") != "code" {
		http.Error(w, "permintaan otorisasi tidak valid (redirect_uri / PKCE S256 wajib)", http.StatusBadRequest)
		return
	}
	hidden := map[string]string{"client_id": clientID, "redirect_uri": redirect, "code_challenge": challenge, "code_challenge_method": method, "state": state, "response_type": "code", "scope": get("scope")}
	var uid, uname string
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.DB.Pool.QueryRow(r.Context(), `SELECT u.id, u.name FROM user_sessions s JOIN users u ON u.id=s.user_id WHERE s.token=$1 AND s.expires_at > now()`, hashToken(c.Value)).Scan(&uid, &uname)
	}
	errMsg := ""
	if uid == "" && r.Method == http.MethodPost && get("email") != "" {
		var hash string
		if err := a.DB.Pool.QueryRow(r.Context(), `SELECT id, name, password_hash FROM users WHERE lower(email)=lower($1)`, get("email")).Scan(&uid, &uname, &hash); err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(get("password"))) != nil {
			uid, errMsg = "", "Email atau kata sandi salah"
		} else {
			tok := randomToken(32)
			_, _ = a.DB.Pool.Exec(r.Context(), `INSERT INTO user_sessions(token,user_id,expires_at) VALUES ($1,$2,$3)`, hashToken(tok), uid, time.Now().Add(14*24*time.Hour))
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		}
		r.Form.Del("consent")
	}
	if uid != "" && r.Method == http.MethodPost && get("consent") == "yes" {
		code := randomToken(24)
		_, _ = a.DB.Pool.Exec(r.Context(), `INSERT INTO oauth_codes(code,client_id,user_id,redirect_uri,code_challenge,scope,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			hashToken(code), clientID, uid, redirect, challenge, get("scope"), time.Now().Add(5*time.Minute))
		_ = storage.Audit(r.Context(), a.DB.Pool, storage.Actor{ID: uid, Type: "user"}, "oauth.consent", "oauth_client", clientID, nil)
		u, _ := url.Parse(redirect)
		q := u.Query()
		q.Set("code", code)
		if state != "" {
			q.Set("state", state)
		}
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = consentTmpl.Execute(w, map[string]any{"Hidden": hidden, "User": uname, "Client": defaultStr(name, "Klien AI"), "Err": errMsg})
}

func (a *App) issueTokens(ctx context.Context, clientID, userID, scope string) (map[string]any, error) {
	access, refresh := "arcat_"+randomToken(24), "arcrt_"+randomToken(24)
	if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO oauth_tokens(token_hash,client_id,user_id,scope,kind,expires_at) VALUES ($1,$2,$3,$4,'access',$5),($6,$2,$3,$4,'refresh',$7)`,
		hashToken(access), clientID, userID, scope, time.Now().Add(time.Hour), hashToken(refresh), time.Now().Add(30*24*time.Hour)); err != nil {
		return nil, err
	}
	return map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": 3600, "refresh_token": refresh, "scope": defaultStr(scope, "read propose")}, nil
}

func (a *App) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	ctx := r.Context()
	fail := func(code string) { writeJSON(w, 400, map[string]string{"error": code}) }
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		var client, uid, redirect, challenge, scope string
		var exp time.Time
		var used bool
		err := a.DB.Pool.QueryRow(ctx, `SELECT client_id, user_id, redirect_uri, code_challenge, scope, expires_at, used FROM oauth_codes WHERE code=$1`, hashToken(r.Form.Get("code"))).
			Scan(&client, &uid, &redirect, &challenge, &scope, &exp, &used)
		if err != nil || used || time.Now().After(exp) || client != r.Form.Get("client_id") || redirect != r.Form.Get("redirect_uri") {
			fail("invalid_grant")
			return
		}
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			fail("invalid_grant")
			return
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE oauth_codes SET used=true WHERE code=$1`, hashToken(r.Form.Get("code")))
		t, err := a.issueTokens(ctx, client, uid, scope)
		if err != nil {
			fail("server_error")
			return
		}
		writeJSON(w, 200, t)
	case "refresh_token":
		var client, uid, scope string
		err := a.DB.Pool.QueryRow(ctx, `UPDATE oauth_tokens SET revoked=true WHERE token_hash=$1 AND kind='refresh' AND NOT revoked AND expires_at > now() RETURNING client_id, user_id, scope`,
			hashToken(r.Form.Get("refresh_token"))).Scan(&client, &uid, &scope)
		if err != nil {
			fail("invalid_grant")
			return
		}
		t, err := a.issueTokens(ctx, client, uid, scope)
		if err != nil {
			fail("server_error")
			return
		}
		writeJSON(w, 200, t)
	default:
		fail("unsupported_grant_type")
	}
}

// viewJSON runs an existing UI handler internally (same principal) and decodes its JSON.
func (a *App) viewJSON(ctx context.Context, h http.HandlerFunc, target string, pv map[string]string) (any, error) {
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	for k, v := range pv {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	var out any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code >= 300 {
		if m, ok := out.(map[string]any); ok {
			return nil, fmt.Errorf("%v", m["error"])
		}
		return nil, fmt.Errorf("HTTP %d", rec.Code)
	}
	return out, nil
}

func argStr(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// callTool executes an MCP tool for the principal in ctx (rights are inherited from the user).
func (a *App) callTool(ctx context.Context, tool string, args map[string]any) (any, error) {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("perlu otorisasi")
	}
	need := domain.ScopeRead
	for _, t := range mcp.Tools() {
		if t.Name == tool && t.Kind == "write" {
			need = domain.ScopePropose
		}
	}
	if !p.Can(need) {
		return nil, fmt.Errorf("scope %s diperlukan", need)
	}
	defer func() {
		_ = storage.Audit(context.Background(), a.DB.Pool, p.Actor(), "mcp:"+tool, "mcp", tool, args)
	}()
	q := url.Values{}
	switch tool {
	case "arc_accounts_list":
		return a.viewJSON(ctx, a.handleAccounts, "/api/accounts?q="+url.QueryEscape(argStr(args, "query")), nil)
	case "arc_accounts_brief":
		return a.accountBrief(ctx, argStr(args, "account_id"))
	case "arc_accounts_memory_append":
		id := argStr(args, "account_id")
		if !a.canSeeAccountID(ctx, p, id) {
			return nil, fmt.Errorf("akun di luar hak akses Anda")
		}
		if err := a.Agents.AppendMemory(ctx, id, argStr(args, "note"), p.Actor()); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "arc_commitments_list":
		return a.commitmentsList(ctx, p, argStr(args, "account_id"), argStr(args, "status"))
	case "arc_commitments_create":
		id := argStr(args, "account_id")
		if !a.canSeeAccountID(ctx, p, id) {
			return nil, fmt.Errorf("akun di luar hak akses Anda")
		}
		who, text := argStr(args, "who"), argStr(args, "text")
		if (who != "kami" && who != "mereka") || text == "" || argStr(args, "source") == "" {
			return nil, fmt.Errorf("who (kami|mereka), text, dan source wajib")
		}
		hash := storage.Hash(id, who, strings.ToLower(text), argStr(args, "due"))
		var due any
		if d := argStr(args, "due"); d != "" {
			if t, err := time.ParseInLocation("2006-01-02", d, domain.Jakarta); err == nil {
				due = t.Add(17 * time.Hour)
			}
		}
		_, err := a.DB.Pool.Exec(ctx, `INSERT INTO commitments(id,dedupe_hash,account_id,who,text,detail,due_at,status,evidence,model) VALUES ($1,$2,$3,$4,$5,$6,$7,'open',$8,'mcp') ON CONFLICT (dedupe_hash) DO NOTHING`,
			"cm-"+hash[:16], hash, id, who, text, "via "+p.KeyID, due, storage.JSON([]domain.Evidence{{Source: argStr(args, "source"), Quote: text}}))
		return map[string]any{"ok": err == nil, "id": "cm-" + hash[:16]}, err
	case "arc_deals_list":
		return a.viewJSON(ctx, a.handleOpportunities, "/api/opportunities", nil)
	case "arc_deals_forecast":
		ex := []string{}
		if arr, ok := args["exclude"].([]any); ok {
			for _, x := range arr {
				ex = append(ex, fmt.Sprint(x))
			}
		}
		q.Set("exclude", strings.Join(ex, ","))
		return a.viewJSON(ctx, a.handleForecast, "/api/forecast?"+q.Encode(), nil)
	case "arc_deals_probability_propose":
		return a.viewPost(ctx, a.handleWriteProbability, map[string]string{"id": argStr(args, "opportunity_id")}, nil)
	case "arc_tenders_list":
		pipe, err := a.viewJSON(ctx, a.handlePipeline, "/api/pipeline", nil)
		if err != nil {
			return nil, err
		}
		return pipe.(map[string]any)["tenders"], nil
	case "arc_chat_threads":
		return a.viewJSON(ctx, a.handleChatThreads, "/api/chat/threads?type="+url.QueryEscape(argStr(args, "type")), nil)
	case "arc_chat_read":
		return a.viewJSON(ctx, a.handleChatMessages, "/api/chat/threads/x/messages", map[string]string{"id": argStr(args, "thread_id")})
	case "arc_chat_reply_draft":
		return a.viewPost(ctx, a.handleChatReply, map[string]string{"id": argStr(args, "thread_id")}, map[string]string{"text": argStr(args, "text")})
	case "arc_network_graph":
		q.Set("period", argStr(args, "period"))
		q.Set("sales", argStr(args, "sales"))
		return a.viewJSON(ctx, a.handleNetwork, "/api/network?"+q.Encode(), nil)
	case "arc_cash_l2c", "arc_cash_aging", "arc_cash_forecast":
		if tool == "arc_cash_forecast" && argStr(args, "what_if") != "" {
			return a.viewJSON(ctx, a.handleCashForecast, "/api/cash/forecast?what_if="+url.QueryEscape(argStr(args, "what_if")), nil)
		}
		c, err := a.viewJSON(ctx, a.handleCash, "/api/cash", nil)
		if err != nil {
			return nil, err
		}
		m := c.(map[string]any)
		return map[string]any{"arc_cash_l2c": map[string]any{"l2c": m["l2c"]}, "arc_cash_aging": m["aging"], "arc_cash_forecast": m["forecast"]}[tool], nil
	case "arc_cash_reminder_draft":
		id := argStr(args, "account_id")
		if !a.canSeeAccountID(ctx, p, id) {
			return nil, fmt.Errorf("akun di luar hak akses Anda")
		}
		aid, _, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Collection agent", Type: "payment_reminder", Kind: "send", Icon: "i-mail", ButtonLabel: "Kirim pengingat", AccountID: id,
			Title: "Kirim pengingat pembayaran", Why: "Diminta lewat " + p.KeyID, Prep: "Email ramah dengan salinan invoice.", Steps: []string{"Masuk antrean approval finance"}, InQueue: true,
			Evidence: []domain.Evidence{{Source: "mcp:" + p.KeyID, Quote: "permintaan draf pengingat"}}, Confidence: 0.7}, p.Actor())
		return map[string]any{"action_id": aid, "status": "proposed", "note": "Masuk antrean approval — tidak ada yang terkirim tanpa keputusan manusia."}, err
	case "arc_actions_list":
		return a.viewJSON(ctx, a.handleActions, "/api/actions?status="+url.QueryEscape(defaultStr(argStr(args, "status"), "proposed")), nil)
	case "arc_actions_propose":
		if argStr(args, "source") == "" {
			return nil, fmt.Errorf("source (bukti) wajib — tanpa provenance tidak disimpan")
		}
		acc := argStr(args, "account_id")
		if acc != "" && !a.canSeeAccountID(ctx, p, acc) {
			return nil, fmt.Errorf("akun di luar hak akses Anda")
		}
		aid, _, err := a.Actions.Propose(ctx, actions.Proposal{Agent: "Klien AI (" + p.KeyID + ")", Type: defaultStr(argStr(args, "type"), "task"), Kind: "task", Icon: "i-spark",
			AccountID: acc, Title: argStr(args, "title"), Why: argStr(args, "why"), Preview: argStr(args, "preview"), Prep: "Diusulkan lewat MCP.", Steps: []string{"Masuk antrean approval"}, InQueue: true,
			Evidence: []domain.Evidence{{Source: argStr(args, "source"), Quote: argStr(args, "why")}}, Confidence: 0.7}, p.Actor())
		return map[string]any{"action_id": aid, "status": "proposed"}, err
	case "arc_actions_decide":
		if !p.Human() {
			return nil, fmt.Errorf("ditolak: arc.actions.decide hanya untuk pengguna manusia di ARC — token mesin/klien AI tidak bisa memutuskan")
		}
		act, t, err := a.Actions.Decide(ctx, argStr(args, "action_id"), p.Actor(), actions.Decision{Decision: argStr(args, "decision"), Reason: argStr(args, "reason")})
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": act.Status, "toast": t}, nil
	case "arc_policy_get":
		var v json.RawMessage
		if k := argStr(args, "key"); k != "" {
			if err := a.DB.Pool.QueryRow(ctx, `SELECT value FROM policies WHERE key=$1`, k).Scan(&v); err != nil {
				return nil, fmt.Errorf("policy %s tidak ada", k)
			}
			return map[string]any{"key": k, "value": v}, nil
		}
		return a.viewJSON(ctx, a.handlePolicies, "/api/policies", nil)
	case "arc_policy_set":
		if p.Role != domain.RoleCEO || p.Kind == "apikey" {
			return nil, fmt.Errorf("ditolak: arc.policy.set hanya untuk CEO")
		}
		raw, _ := json.Marshal(args["value"])
		if err := a.SetPolicy(ctx, p, argStr(args, "key"), raw); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "arc_prospects_list":
		pr, err := a.viewJSON(ctx, a.handleProspects, "/api/prospects", nil)
		if err != nil {
			return nil, err
		}
		return pr.(map[string]any)["inbound"], nil
	case "arc_prospects_brief":
		return a.viewJSON(ctx, a.handleProspect, "/api/prospects/x", map[string]string{"id": argStr(args, "prospect_id")})
	}
	return nil, fmt.Errorf("tool tidak dikenal: %s", tool)
}

func (a *App) viewPost(ctx context.Context, h http.HandlerFunc, pv map[string]string, body any) (any, error) {
	raw, _ := json.Marshal(body)
	if body == nil {
		raw = []byte("{}")
	}
	req := httptest.NewRequest(http.MethodPost, "/internal", strings.NewReader(string(raw))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range pv {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	var out any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code >= 300 {
		if m, ok := out.(map[string]any); ok {
			return nil, fmt.Errorf("%v", m["error"])
		}
		return nil, fmt.Errorf("HTTP %d", rec.Code)
	}
	return out, nil
}

func (a *App) canSeeAccountID(ctx context.Context, p Principal, id string) bool {
	sc := p.Scope()
	if sc.All {
		return true
	}
	var ok bool
	_ = a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND (branch=$2 OR owner_user_id=$3))`, id, sc.Branch, sc.UserID).Scan(&ok)
	return ok
}

func (a *App) commitmentsList(ctx context.Context, p Principal, acc, status string) (any, error) {
	sc := p.Scope()
	cond, args := sc.SQL("a", 3)
	rows, err := a.DB.Pool.Query(ctx, `SELECT c.id, c.account_id, a.name, c.who, c.text, c.detail, c.status, c.due_at, c.evidence FROM commitments c JOIN accounts a ON a.id=c.account_id
		WHERE ($1='' OR c.account_id=$1) AND ($2='' OR c.status=$2) AND `+cond+` ORDER BY c.due_at NULLS LAST LIMIT 200`, append([]any{acc, status}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, aid, an, who, text, detail, st string
		var due *time.Time
		var ev json.RawMessage
		_ = rows.Scan(&id, &aid, &an, &who, &text, &detail, &st, &due, &ev)
		out = append(out, map[string]any{"id": id, "account_id": aid, "account": an, "who": who, "text": text, "detail": detail, "status": st, "due_at": due, "evidence": ev})
	}
	return out, nil
}

// accountBrief is the account view plus the evidence behind its open signals, open
// commitments and health components, so MCP/API clients get the provenance inline.
func (a *App) accountBrief(ctx context.Context, id string) (any, error) {
	v, err := a.viewJSON(ctx, a.handleAccount, "/api/accounts/x", map[string]string{"id": id})
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	m["evidence"] = a.accountEvidence(ctx, id)
	return m, nil
}

func (a *App) accountEvidence(ctx context.Context, id string) []map[string]any {
	out := []map[string]any{}
	rows, err := a.DB.Pool.Query(ctx, `SELECT kind, ref, evidence, conf FROM (
		SELECT 1 AS ord, 'signal:' || type AS kind, 'signal:' || id AS ref, evidence, confidence::float8 AS conf FROM signals WHERE account_id=$1 AND resolved_at IS NULL
		UNION ALL SELECT 2, 'commitment:' || who, id, evidence, confidence::float8 FROM commitments WHERE account_id=$1 AND status IN ('open','late')
		UNION ALL SELECT 3, 'health:' || hc.component, hc.opportunity_id, hc.evidence, hc.confidence::float8 FROM health_components hc
			JOIN opportunities o ON o.id=hc.opportunity_id WHERE o.account_id=$1 AND o.status='open' AND NOT o.historical
	) x ORDER BY ord LIMIT 60`, id)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kind, ref string
		var raw []byte
		var conf float64
		if rows.Scan(&kind, &ref, &raw, &conf) != nil {
			continue
		}
		var evs []domain.Evidence
		_ = json.Unmarshal(raw, &evs)
		for _, e := range evs {
			if e.Quote == "" {
				continue
			}
			item := map[string]any{"for": kind, "ref": ref, "quote": e.Quote, "confidence": conf}
			if e.InteractionID != 0 {
				item["interaction_id"] = e.InteractionID
			}
			if e.DocumentID != "" {
				item["document_id"] = e.DocumentID
			}
			if e.Source != "" {
				item["source"] = e.Source
			}
			if e.At != "" {
				item["at"] = e.At
			}
			out = append(out, item)
		}
	}
	return out
}
