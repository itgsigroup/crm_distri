package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strings"

	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/llm"
	"arc/packages/core/prompts"
	"arc/packages/core/storage"
)

// EvidenceCard backs an answer with one account's evidence.
type EvidenceCard struct {
	AccountID string  `json:"account_id"`
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Band      string  `json:"band"`
	Items     []struct {
		Icon string `json:"icon"`
		Text string `json:"text"`
	} `json:"items"`
	Provs []string `json:"provs"`
}

// ScenarioRow is a what-if bar.
type ScenarioRow struct {
	Label     string   `json:"label"`
	Width     float64  `json:"width"`
	Was       bool     `json:"was"`
	TargetPos *float64 `json:"target_pos,omitempty"`
	Value     string   `json:"value"`
	Delta     string   `json:"delta,omitempty"`
}

// AskRec is a follow-up the user can trigger.
type AskRec struct {
	Label   string `json:"label"`
	Icon    string `json:"icon,omitempty"`
	Primary bool   `json:"primary,omitempty"`
	Action  string `json:"action,omitempty"`
	Toast   string `json:"toast,omitempty"`
}

// AskAnswer is the response shown on the Ask screen and via /api/v1/ask.
type AskAnswer struct {
	ID          string         `json:"id"`
	Question    string         `json:"question"`
	WhoLabel    string         `json:"who_label"`
	Paragraphs  []string       `json:"paragraphs"`
	Evidence    []EvidenceCard `json:"evidence"`
	Scenario    []ScenarioRow  `json:"scenario"`
	Followup    string         `json:"followup"`
	Recs        []AskRec       `json:"recs"`
	Sources     []string       `json:"sources"`
	Confidence  float64        `json:"confidence"`
	Mode        string         `json:"mode"`
	EvidenceIDs []string       `json:"evidence_ids"`
}

// AskContext is the retrieved graph slice given to the model.
type AskContext struct {
	Question     string            `json:"question"`
	Screen       string            `json:"screen"`
	Intent       string            `json:"intent"`
	Deals        []map[string]any  `json:"deals"`
	Signals      []string          `json:"signals"`
	Commitments  []string          `json:"commitments"`
	Interactions []string          `json:"interactions"`
	Forecast     map[string]any    `json:"forecast"`
	Vars         map[string]string `json:"vars"`
}

var (
	reRisk   = regexp.MustCompile(`(?i)berisiko|risiko|bahaya|terancam`)
	reWhatIf = regexp.MustCompile(`(?i)(kalau|jika|bagaimana kalau).*(tidak masuk|batal|gagal|mundur|slip)`)
)

// Ask answers a question from the graph, restricted to the user's scope.
func (a *Agents) Ask(ctx context.Context, userID, question, screen string, sc insights.Scope) (AskAnswer, error) {
	question = strings.TrimSpace(question)
	now := domain.Now()
	ans := AskAnswer{ID: "ask-" + storage.Hash(userID, question, now.String())[:12], Question: question, Mode: "graph"}
	deals, err := a.Ins.Deals(ctx, sc)
	if err != nil {
		return ans, err
	}
	actx := AskContext{Question: question, Screen: screen, Vars: map[string]string{}}
	var focus []insights.Deal
	switch {
	case reWhatIf.MatchString(question):
		actx.Intent = "what_if"
		for _, d := range deals {
			if d.IsPipeline() && mentions(question, d.Account) {
				focus = append(focus, d)
			}
		}
	case reRisk.MatchString(question):
		actx.Intent = "risk"
		for _, d := range deals {
			if d.IsPipeline() && d.Trend < 0 && d.Health < 65 {
				focus = append(focus, d)
			}
		}
		sort.SliceStable(focus, func(i, j int) bool { return focus[i].Value > focus[j].Value })
	default:
		actx.Intent = "general"
		for _, d := range deals {
			if d.IsPipeline() && mentions(question, d.Account) {
				focus = append(focus, d)
			}
		}
	}
	if len(focus) > 3 {
		focus = focus[:3]
	}
	total := 0.0
	for _, d := range focus {
		total += d.Value
		card, ids := a.evidenceCard(ctx, d)
		ans.Evidence = append(ans.Evidence, card)
		ans.EvidenceIDs = append(ans.EvidenceIDs, ids...)
		actx.Deals = append(actx.Deals, map[string]any{"account": d.Account, "value": d.Value, "health": d.Health, "trend": d.Trend, "stage": d.Stage, "evidence": card.Items})
	}
	actx.Vars["total"] = domain.FormatRp1(total)
	fc, _ := a.Ins.Forecast(ctx, sc)
	actx.Forecast = map[string]any{"commit": fc.Commit, "best": fc.Best, "pipeline": fc.Pipeline, "target": fc.Target}
	actx.Vars["commit"] = domain.FormatRp1(fc.Commit)
	if actx.Intent == "what_if" && len(focus) > 0 {
		ids := []string{}
		for _, d := range focus {
			ids = append(ids, d.ID)
		}
		without, _ := a.Ins.Forecast(ctx, sc, ids...)
		actx.Vars["without"] = domain.FormatRp1(without.Commit)
		actx.Vars["gap"] = domain.FormatRp1(without.Target - without.Commit)
		for _, d := range deals {
			if d.ID == "pelindo" || strings.Contains(strings.ToLower(d.Account), "pelabuhan") {
				actx.Vars["pelindo_health"] = fmt.Sprint(d.Health)
			}
		}
		nextTarget := a.Ins.PolicyFloat(ctx, "next_quarter_target", 6e9)
		scale := math.Max(fc.Target, nextTarget) * 1.2
		tp := math.Round(fc.Target/scale*100*10) / 10
		ans.Scenario = []ScenarioRow{
			{Label: "Commit sekarang", Width: pct(fc.Commit, scale), Was: true, TargetPos: &tp, Value: domain.FormatRp1(fc.Commit)},
			{Label: "Tanpa PO " + abbrev(focus[0].Account), Width: pct(without.Commit, scale), TargetPos: &tp, Value: domain.FormatRp1(without.Commit), Delta: "−" + strings.TrimSuffix(strings.TrimPrefix(domain.FormatM1(fc.Commit-without.Commit), "Rp "), " M")},
			{Label: fmt.Sprintf("Q%d best case", (fc.Quarter%4)+1), Width: 100, Was: true, Value: domain.FormatRp1(nextTarget * 1.3), Delta: "+" + strings.TrimSuffix(strings.TrimPrefix(domain.FormatM1(fc.Commit-without.Commit), "Rp "), " M")},
		}
		ans.Recs = []AskRec{{Label: "Ya, masukkan ke brief", Primary: true, Toast: "Ditambahkan ke brief kick-off besok"}, {Label: "Draf pengingat ke " + a.championOf(ctx, focus[0].AccountID), Toast: "Draf pengingat masuk antrean approval"}}
	}
	// Supporting retrieval.
	rows, err := a.DB.Pool.Query(ctx, `SELECT s.id, s.title || ' · ' || COALESCE(a.name,'') FROM signals s LEFT JOIN accounts a ON a.id=s.account_id WHERE s.resolved_at IS NULL ORDER BY s.detected_at DESC LIMIT 10`)
	if err == nil {
		for rows.Next() {
			var id int64
			var t string
			_ = rows.Scan(&id, &t)
			actx.Signals = append(actx.Signals, t)
			if len(ans.EvidenceIDs) < 2 {
				ans.EvidenceIDs = append(ans.EvidenceIDs, fmt.Sprintf("signal:%d", id))
			}
		}
		rows.Close()
	}
	rows, err = a.DB.Pool.Query(ctx, `SELECT i.id, to_char(i.occurred_at,'DD Mon') || ' ' || COALESCE(a.name,'') || ': ' || left(i.body_text, 200) FROM interactions i LEFT JOIN accounts a ON a.id=i.account_id
		WHERE i.channel <> 'wa_aggregate' AND i.body_text <> '' ORDER BY i.occurred_at DESC LIMIT 20`)
	if err == nil {
		for rows.Next() {
			var id int64
			var t string
			_ = rows.Scan(&id, &t)
			actx.Interactions = append(actx.Interactions, t)
		}
		rows.Close()
	}
	for _, d := range focus {
		if d.NextAction != "" {
			if act, err := a.Actions.Get(ctx, d.NextAction); err == nil && act.Status == domain.ActionProposed && actx.Intent == "risk" {
				ans.Recs = append(ans.Recs, AskRec{Label: act.Title, Icon: act.Icon, Action: act.ID})
			}
		}
	}
	raw, _ := json.Marshal(actx)
	var out struct {
		Answer      string   `json:"answer"`
		EvidenceIDs []string `json:"evidence_ids"`
		Confidence  float64  `json:"confidence"`
	}
	resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Interactive, Purpose: "ask", System: prompts.Get("ask/v1"),
		Messages: []llm.Message{{Role: "user", Content: string(raw)}}, Schema: obj([]string{"answer", "evidence_ids", "confidence"}, map[string]any{"answer": str(), "evidence_ids": arr(str()), "confidence": map[string]any{"type": "number"}}),
		MaxTokens: 3000, FakeInput: actx}, &out)
	if err != nil {
		out.Answer = "Saya belum bisa menjawab sekarang — provider AI tidak tersedia. Data yang relevan sudah saya kumpulkan di kartu bukti di bawah."
		out.Confidence = 0.5
		ans.Mode = "fallback"
	}
	if ex, ok := a.Fx.AskExtra[question]; ok && a.LLM.IsFake(llm.Interactive) {
		for _, p := range ex.Paragraphs {
			ans.Paragraphs = append(ans.Paragraphs, fillVars(p, actx.Vars))
		}
		ans.Followup = ex.Followup
	} else {
		for _, p := range strings.Split(out.Answer, "\n\n") {
			if strings.TrimSpace(p) != "" {
				ans.Paragraphs = append(ans.Paragraphs, html.EscapeString(strings.TrimSpace(p)))
			}
		}
		if actx.Intent == "risk" && len(ans.Recs) > 0 {
			ans.Followup = "Yang bisa saya kerjakan sekarang:"
		}
	}
	ans.Confidence = out.Confidence
	if ans.Confidence == 0 {
		ans.Confidence = 0.86
	}
	ans.EvidenceIDs = append(ans.EvidenceIDs, out.EvidenceIDs...)
	switch actx.Intent {
	case "risk":
		ans.WhoLabel = fmt.Sprintf("menilai %d deal terbuka · %s", countPipeline(deals), domain.ClockID(now))
	case "what_if":
		ans.WhoLabel = "skenario · " + domain.ClockID(now)
	default:
		ans.WhoLabel = "dari graph · " + domain.ClockID(now)
		ans.Sources = []string{fmt.Sprintf("sumber: %d interaksi", min(20, len(actx.Interactions))), fmt.Sprintf("%d sinyal", len(actx.Signals)), fmt.Sprintf("conf %.2f", ans.Confidence)}
	}
	if ans.Mode != "fallback" && !a.LLM.IsFake(llm.Interactive) {
		ans.Mode = "llm"
	}
	_ = resp
	ans.normalize()
	if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO ask_history(id,user_id,question,answer) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, ans.ID, userID, question, storage.JSONObj(ans)); err != nil {
		return ans, err
	}
	return ans, nil
}

// normalize replaces nil slices with empty ones so clients always get arrays.
func (ans *AskAnswer) normalize() {
	if ans.Paragraphs == nil {
		ans.Paragraphs = []string{}
	}
	if ans.Evidence == nil {
		ans.Evidence = []EvidenceCard{}
	}
	for i := range ans.Evidence {
		if ans.Evidence[i].Items == nil {
			ans.Evidence[i].Items = []struct {
				Icon string `json:"icon"`
				Text string `json:"text"`
			}{}
		}
	}
	if ans.Scenario == nil {
		ans.Scenario = []ScenarioRow{}
	}
	if ans.Recs == nil {
		ans.Recs = []AskRec{}
	}
	if ans.Sources == nil {
		ans.Sources = []string{}
	}
	if ans.EvidenceIDs == nil {
		ans.EvidenceIDs = []string{}
	}
}

func countPipeline(ds []insights.Deal) int {
	n := 0
	for _, d := range ds {
		if d.IsPipeline() {
			n++
		}
	}
	return n
}

func pct(v, scale float64) float64 { return math.Round(v/scale*100*10) / 10 }

func abbrev(name string) string {
	words := strings.Fields(name)
	if len(words) >= 3 {
		out := ""
		for _, w := range words {
			out += strings.ToUpper(w[:1])
		}
		return out
	}
	return name
}

func fillVars(s string, v map[string]string) string {
	for k, x := range v {
		s = strings.ReplaceAll(s, "{"+k+"}", x)
	}
	return s
}

func mentions(q, account string) bool {
	ql := strings.ToLower(q)
	for _, w := range strings.Fields(strings.ToLower(account)) {
		if len(w) >= 5 && strings.Contains(ql, w) && w != "daerah" && w != "kota" {
			return true
		}
	}
	return false
}

func (a *Agents) championOf(ctx context.Context, acc string) string {
	var n string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT name FROM people WHERE account_id=$1 AND stakeholder_tag='champion' ORDER BY strength DESC LIMIT 1`, acc).Scan(&n)
	return defaultStr(n, "kontak utama")
}

var signalIcon = map[string]string{"competitor_mentioned": "i-mail", "silent": "i-cal", "single_threaded": "i-people", "champion_moved": "i-chat", "po_overdue": "i-doc",
	"meeting_without_champion": "i-cal", "commitment_due": "i-doc", "spec_pending": "i-doc", "legal_cycle": "i-doc"}

func (a *Agents) evidenceCard(ctx context.Context, d insights.Deal) (EvidenceCard, []string) {
	c := EvidenceCard{AccountID: d.AccountID, Name: d.Account, Value: d.Value, Band: domain.Band(d.Health)}
	var ids []string
	rows, err := a.DB.Pool.Query(ctx, `SELECT id, type, title, detail, confidence::float8 FROM signals WHERE opportunity_id=$1 AND resolved_at IS NULL ORDER BY CASE severity WHEN 'bad' THEN 0 WHEN 'warn' THEN 1 ELSE 2 END, detected_at DESC LIMIT 3`, d.ID)
	conf := 0.0
	if err == nil {
		for rows.Next() {
			var id int64
			var typ, title, detail string
			var cf float64
			_ = rows.Scan(&id, &typ, &title, &detail, &cf)
			icon := signalIcon[typ]
			if icon == "" {
				icon = "i-flag"
			}
			c.Items = append(c.Items, struct {
				Icon string `json:"icon"`
				Text string `json:"text"`
			}{icon, title + ": " + detail})
			ids = append(ids, fmt.Sprintf("signal:%d", id))
			conf = math.Max(conf, cf)
		}
		rows.Close()
	}
	var decision string
	var strength int
	if a.DB.Pool.QueryRow(ctx, `SELECT name || ' (' || role || ')', strength FROM people WHERE account_id=$1 AND stakeholder_tag='decision' ORDER BY strength LIMIT 1`, d.AccountID).Scan(&decision, &strength) == nil && strength == 0 && len(c.Items) < 3 {
		c.Items = append(c.Items, struct {
			Icon string `json:"icon"`
			Text string `json:"text"`
		}{"i-people", decision + " belum pernah kita temui"})
	}
	arrow := "↑"
	if d.Trend < 0 {
		arrow = "↓"
	}
	if conf == 0 {
		conf = 0.8
	}
	c.Provs = []string{fmt.Sprintf("health %d %s%d", d.Health, arrow, int(math.Abs(float64(d.Trend)))), fmt.Sprintf("conf %.2f", conf)}
	return c, ids
}

func (a *Agents) fakeAsk(req llm.Request) (any, error) {
	in, _ := req.FakeInput.(AskContext)
	if ans, ok := a.Fx.AskCanned[in.Question]; ok {
		return map[string]any{"answer": ans, "evidence_ids": []string{}, "confidence": 0.86}, nil
	}
	var b strings.Builder
	switch in.Intent {
	case "risk":
		fmt.Fprintf(&b, "%d deal berisiko, total %s. ", len(in.Deals), in.Vars["total"])
		for _, d := range in.Deals {
			fmt.Fprintf(&b, "%v (health %v, tren %v). ", d["account"], d["health"], d["trend"])
		}
	case "what_if":
		fmt.Fprintf(&b, "Commit turun dari %s ke %s; gap ke target melebar jadi %s.", in.Vars["commit"], in.Vars["without"], in.Vars["gap"])
	default:
		if len(in.Deals) > 0 {
			d := in.Deals[0]
			fmt.Fprintf(&b, "%v: nilai %s, health %v, stage %v.", d["account"], domain.FormatRp(d["value"].(float64)), d["health"], d["stage"])
		} else {
			b.WriteString("Berdasarkan graph saat ini: ")
			if len(in.Signals) > 0 {
				b.WriteString("sinyal terbaru — " + strings.Join(in.Signals[:min(3, len(in.Signals))], "; ") + ". ")
			}
			fmt.Fprintf(&b, "Commit kuartal %s. Untuk jawaban yang lebih spesifik, sebutkan nama akun.", in.Vars["commit"])
		}
	}
	return map[string]any{"answer": b.String(), "evidence_ids": []string{}, "confidence": 0.8}, nil
}

// AskSuggestions returns contextual questions per screen.
func (a *Agents) AskSuggestions(screen string) []string {
	if s, ok := a.Fx.AskSugg[screen]; ok {
		return s
	}
	if screen == "ask" {
		return []string{"Siapa yang biasanya memutuskan di RSUD Kota Yogyakarta?", "Akun mana yang paling sering telat bayar 12 bulan terakhir?", "Objection apa yang paling sering muncul di proyek videotron pemerintah?"}
	}
	return a.Fx.AskSugg["today"]
}

// AskHistory returns the user's conversation (oldest first).
func (a *Agents) AskHistory(ctx context.Context, userID string, limit int) ([]json.RawMessage, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT answer FROM (SELECT answer, created_at FROM ask_history WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2) x ORDER BY created_at`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}
