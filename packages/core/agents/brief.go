package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"arc/packages/connectors/notify"
	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/llm"
	"arc/packages/core/prompts"
	"arc/packages/core/storage"
)

// BriefFact is one candidate point with its evidence, assembled deterministically.
type BriefFact struct {
	Kind      string         `json:"kind"` // gap | risk | people | opportunity
	AccountID string         `json:"account_id"`
	Account   string         `json:"account"`
	Data      map[string]any `json:"data"`
	Evidence  []string       `json:"evidence"`
}

// BriefPoint is a rendered point.
type BriefPoint struct {
	Kind      string   `json:"kind"`
	K         string   `json:"k"`
	Icon      string   `json:"icon"`
	Text      string   `json:"text"`
	HTML      string   `json:"html"`
	AccountID string   `json:"account_id"`
	Evidence  []string `json:"evidence"`
}

// Brief is a generated brief.
type Brief struct {
	ID           int64          `json:"id"`
	Slot         string         `json:"slot"`
	Points       []BriefPoint   `json:"points"`
	SourceCounts map[string]int `json:"source_counts"`
	Confidence   float64        `json:"confidence"`
	Model        string         `json:"model"`
	CreatedAt    time.Time      `json:"created_at"`
	TextBody     string         `json:"text_body"`
	HTMLBody     string         `json:"html_body"`
}

var pointStyle = map[string][2]string{"gap": {"accent", "i-target"}, "risk": {"bad", "i-alert"}, "people": {"warn", "i-user-x"}, "opportunity": {"good", "i-check"}}

// BriefFacts collects at most one fact per kind, each with evidence ids.
func (a *Agents) BriefFacts(ctx context.Context) ([]BriefFact, error) {
	now := domain.Now()
	var facts []BriefFact
	fc, err := a.Ins.Forecast(ctx, insights.Scope{All: true})
	if err != nil {
		return nil, err
	}
	q, _ := domain.Quarter(now)
	gap := fc.Target - fc.Commit
	var poAcc, poAccName, poDue string
	var poVal float64
	var poLate int
	var poEvidence int64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT o.account_id, a.name, o.expected_revenue::float8, c.due_at::date::text, GREATEST(0, EXTRACT(DAY FROM ($1 - c.due_at)))::int, COALESCE(c.origin_interaction_id,0)
		FROM commitments c JOIN opportunities o ON o.account_id=c.account_id AND o.status='open' AND NOT o.historical JOIN accounts a ON a.id=o.account_id
		WHERE c.who='mereka' AND c.text ILIKE '%PO%' AND c.status IN ('open','late') ORDER BY o.expected_revenue DESC LIMIT 1`, now).Scan(&poAcc, &poAccName, &poVal, &poDue, &poLate, &poEvidence)
	gapFact := BriefFact{Kind: "gap", AccountID: poAcc, Account: poAccName, Data: map[string]any{"quarter": q, "commit": fc.Commit, "target": fc.Target, "gap": gap,
		"po_value": poVal, "po_late_days": poLate, "workdays_left": fc.WorkdaysLeft}, Evidence: []string{"forecast:commit"}}
	if poDue != "" {
		if t, err := time.Parse("2006-01-02", poDue); err == nil {
			gapFact.Data["po_due"] = domain.ShortDate(t)
		}
		gapFact.Evidence = append(gapFact.Evidence, "commitment:po:"+poAcc)
	}
	var nextMeeting *time.Time
	_ = a.DB.Pool.QueryRow(ctx, `SELECT min(starts_at) FROM calendar_events WHERE account_id=$1 AND starts_at > $2`, poAcc, now).Scan(&nextMeeting)
	gapFact.Data["deadline_day"] = "Rabu"
	if nextMeeting != nil {
		gapFact.Data["deadline_day"] = dayName(nextMeeting.AddDate(0, 0, 1))
	}
	facts = append(facts, gapFact)

	// Risk: lowest-health open deal with a competitor/silence signal.
	var rAcc, rName, rOwner, rChamp, rDecision string
	var rDays int
	err = a.DB.Pool.QueryRow(ctx, `SELECT o.account_id, a.name, COALESCE(u.name,''),
		COALESCE((SELECT name FROM people WHERE account_id=a.id AND stakeholder_tag='champion' ORDER BY strength DESC LIMIT 1),''),
		COALESCE((SELECT role FROM people WHERE account_id=a.id AND stakeholder_tag='decision' LIMIT 1),''),
		COALESCE(EXTRACT(DAY FROM ($1 - a.last_interaction_at))::int, 0)
		FROM opportunities o JOIN accounts a ON a.id=o.account_id LEFT JOIN users u ON u.id=o.owner_user_id
		WHERE o.status='open' AND NOT o.historical AND o.health IS NOT NULL AND EXISTS (SELECT 1 FROM signals s WHERE s.opportunity_id=o.id AND s.resolved_at IS NULL AND s.type IN ('competitor_mentioned','silent'))
		ORDER BY o.expected_revenue * (100 - o.health) DESC LIMIT 1`, now).Scan(&rAcc, &rName, &rOwner, &rChamp, &rDecision, &rDays)
	if err == nil {
		types := a.signalTypes(ctx, rAcc)
		facts = append(facts, BriefFact{Kind: "risk", AccountID: rAcc, Account: rName, Data: map[string]any{"days_quiet": rDays, "champion": rChamp, "decision_role": rDecision,
			"owner": rOwner, "competitor": types["competitor_mentioned"], "single": types["single_threaded"]}, Evidence: a.signalEvidence(ctx, rAcc)})
	}
	// People: champion moved.
	var pAcc, pName, pGhost, pCand string
	var pMeetings int
	err = a.DB.Pool.QueryRow(ctx, `SELECT s.account_id, a.name, COALESCE((SELECT name FROM people WHERE account_id=a.id AND stakeholder_tag='ghost' LIMIT 1),''),
		COALESCE((SELECT name FROM people WHERE account_id=a.id AND stakeholder_tag='influencer' ORDER BY strength DESC LIMIT 1),''),
		COALESCE((SELECT strength FROM people WHERE account_id=a.id AND stakeholder_tag='influencer' ORDER BY strength DESC LIMIT 1),0)
		FROM signals s JOIN accounts a ON a.id=s.account_id WHERE s.type='champion_moved' AND s.resolved_at IS NULL ORDER BY s.detected_at DESC LIMIT 1`).Scan(&pAcc, &pName, &pGhost, &pCand, &pMeetings)
	if err == nil {
		var candRole string
		_ = a.DB.Pool.QueryRow(ctx, `SELECT role FROM people WHERE name=$1 AND account_id=$2`, pCand, pAcc).Scan(&candRole)
		var nextEv *time.Time
		_ = a.DB.Pool.QueryRow(ctx, `SELECT min(starts_at) FROM calendar_events WHERE account_id=$1 AND starts_at > $2`, pAcc, now).Scan(&nextEv)
		facts = append(facts, BriefFact{Kind: "people", AccountID: pAcc, Account: pName, Data: map[string]any{"ghost": pGhost, "candidate": pCand, "candidate_role": candRole,
			"meetings": pMeetings + 1}, Evidence: a.signalEvidence(ctx, pAcc)})
	}
	// Opportunity: on-time payment → maintenance.
	var oAcc, oName string
	err = a.DB.Pool.QueryRow(ctx, `SELECT s.account_id, a.name FROM signals s JOIN accounts a ON a.id=s.account_id WHERE s.type='payment_on_time' AND s.resolved_at IS NULL ORDER BY s.detected_at DESC LIMIT 1`).Scan(&oAcc, &oName)
	if err == nil {
		var paid float64
		_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(max(amount)::float8,0) FROM invoices WHERE account_id=$1 AND paid_at IS NOT NULL`, oAcc).Scan(&paid)
		facts = append(facts, BriefFact{Kind: "opportunity", AccountID: oAcc, Account: oName, Data: map[string]any{"paid": paid}, Evidence: []string{"signal:payment_on_time:" + oAcc}})
	}
	return facts, nil
}

func dayName(t time.Time) string {
	return []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}[t.In(domain.Jakarta).Weekday()]
}

func (a *Agents) signalTypes(ctx context.Context, acc string) map[string]bool {
	out := map[string]bool{}
	rows, err := a.DB.Pool.Query(ctx, `SELECT type FROM signals WHERE account_id=$1 AND resolved_at IS NULL`, acc)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		_ = rows.Scan(&t)
		out[t] = true
	}
	return out
}

func (a *Agents) signalEvidence(ctx context.Context, acc string) []string {
	var out []string
	rows, err := a.DB.Pool.Query(ctx, `SELECT id FROM signals WHERE account_id=$1 AND resolved_at IS NULL ORDER BY detected_at DESC LIMIT 3`, acc)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		out = append(out, fmt.Sprintf("signal:%d", id))
	}
	return out
}

// GenerateBrief writes the brief for a slot (pagi 06.45, sore 16.00, manual) and optionally delivers it.
func (a *Agents) GenerateBrief(ctx context.Context, slot string, send bool) (Brief, error) {
	facts, err := a.BriefFacts(ctx)
	if err != nil {
		return Brief{}, err
	}
	raw, _ := json.Marshal(facts)
	var out struct {
		Points []struct {
			Kind      string   `json:"kind"`
			Text      string   `json:"text"`
			AccountID string   `json:"account_id"`
			Evidence  []string `json:"evidence"`
		} `json:"points"`
		Confidence float64 `json:"confidence"`
	}
	resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Heavy, Purpose: "brief", System: prompts.Get("brief/v1"),
		Messages: []llm.Message{{Role: "user", Content: "Fakta hari ini:\n" + string(raw)}},
		Schema: obj([]string{"points", "confidence"}, map[string]any{"points": arr(obj([]string{"kind", "text", "account_id", "evidence"}, map[string]any{
			"kind": map[string]any{"type": "string", "enum": []string{"gap", "risk", "people", "opportunity"}}, "text": str(), "account_id": str(), "evidence": arr(str())})),
			"confidence": map[string]any{"type": "number"}}), MaxTokens: 3000, FakeInput: facts}, &out)
	if err != nil {
		return Brief{}, err
	}
	names := map[string]string{}
	evidence := map[string][]string{}
	for _, f := range facts {
		names[f.AccountID] = f.Account
		evidence[f.Kind] = f.Evidence
	}
	b := Brief{Slot: slot, Confidence: out.Confidence, Model: resp.Model, CreatedAt: domain.Now()}
	for _, p := range out.Points {
		if len(b.Points) == 4 {
			break
		}
		ev := p.Evidence
		if len(ev) == 0 {
			ev = evidence[p.Kind]
		}
		if len(ev) == 0 {
			continue // every point needs evidence
		}
		st := pointStyle[p.Kind]
		b.Points = append(b.Points, BriefPoint{Kind: p.Kind, K: st[0], Icon: st[1], Text: p.Text, HTML: pointHTML(p.Kind, p.Text, p.AccountID, names[p.AccountID]), AccountID: p.AccountID, Evidence: ev})
	}
	b.SourceCounts = a.todayCounts(ctx)
	b.TextBody, b.HTMLBody = renderBrief(b)
	if err := a.DB.Pool.QueryRow(ctx, `INSERT INTO briefs(slot,brief_date,points,source_counts,confidence,model,html,text_body,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (brief_date,slot) DO UPDATE SET points=EXCLUDED.points, source_counts=EXCLUDED.source_counts, confidence=EXCLUDED.confidence,
		model=EXCLUDED.model, html=EXCLUDED.html, text_body=EXCLUDED.text_body, created_at=EXCLUDED.created_at RETURNING id`,
		slot, b.CreatedAt.Format("2006-01-02"), storage.JSON(b.Points), storage.JSONObj(b.SourceCounts), b.Confidence, b.Model, b.HTMLBody, b.TextBody, b.CreatedAt).Scan(&b.ID); err != nil {
		return b, err
	}
	if send {
		var sent []string
		for _, n := range a.Notifiers {
			to := []string{}
			if a.Recipients != nil {
				to = a.Recipients(ctx, domain.RoleCEO)
			}
			m := notify.Message{To: to, Subject: fmt.Sprintf("Brief ARC · %s", domain.LongDate(b.CreatedAt)), Text: b.TextBody, HTML: b.HTMLBody}
			if err := n.Send(ctx, m); err != nil {
				a.logger().Warn("brief delivery failed", "channel", n.Channel(), "err", err)
				continue
			}
			sent = append(sent, n.Channel())
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO notifications(channel,recipient,subject,body) VALUES ($1,$2,$3,$4)`, n.Channel(), strings.Join(to, ","), m.Subject, b.TextBody)
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE briefs SET sent=$2 WHERE id=$1`, b.ID, storage.JSON(sent))
	}
	return b, nil
}

func (a *Agents) todayCounts(ctx context.Context) map[string]int {
	now := domain.Now()
	out := map[string]int{}
	var e, w, m, p int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE channel='email'), count(*) FILTER (WHERE channel IN ('wa_message','wa_group_message')),
		count(*) FILTER (WHERE channel IN ('meeting','call')), count(*) FILTER (WHERE channel='erp_event')
		FROM interactions WHERE occurred_at >= $1 AND occurred_at < $2`, domain.StartOfDay(now), domain.StartOfDay(now).AddDate(0, 0, 1)).Scan(&e, &w, &m, &p)
	out["emails"], out["whatsapp"], out["meetings"], out["payments"] = e, w, m, p
	return out
}

var reMoney = regexp.MustCompile(`Rp\s?[0-9][0-9.,]*\s?(M|jt)`)

// pointHTML escapes the text, links the account name and bolds key amounts.
func pointHTML(kind, text, accountID, accountName string) string {
	h := html.EscapeString(text)
	if accountName != "" {
		esc := html.EscapeString(accountName)
		if i := strings.Index(h, esc); i >= 0 {
			h = h[:i] + `<button class="ev" data-go="rel:` + html.EscapeString(accountID) + `">` + esc + `</button>` + h[i+len(esc):]
		}
	}
	limit := 0
	switch kind {
	case "gap":
		limit = 2
	case "opportunity":
		limit = 1
	}
	n := 0
	h = reMoney.ReplaceAllStringFunc(h, func(m string) string {
		if n >= limit {
			return m
		}
		n++
		return `<b class="num">` + m + `</b>`
	})
	return h
}

func renderBrief(b Brief) (string, string) {
	var t, hb strings.Builder
	fmt.Fprintf(&t, "Brief ARC · %s\n\n", domain.LongDate(b.CreatedAt))
	hb.WriteString(`<div style="font-family:-apple-system,Segoe UI,sans-serif;max-width:640px"><h2 style="margin:0 0 12px">Brief ARC</h2><ol style="padding-left:18px;line-height:1.55">`)
	for i, p := range b.Points {
		fmt.Fprintf(&t, "%d. %s\n", i+1, p.Text)
		fmt.Fprintf(&hb, `<li style="margin-bottom:10px">%s</li>`, html.EscapeString(p.Text))
	}
	c := b.SourceCounts
	fmt.Fprintf(&t, "\nSumber: %d email, %d WhatsApp, %d meeting, %d pembayaran · confidence %.2f\n", c["emails"], c["whatsapp"], c["meetings"], c["payments"], b.Confidence)
	fmt.Fprintf(&hb, `</ol><p style="color:#6E6E73;font-size:12px">Sumber: %d email, %d WhatsApp, %d meeting, %d pembayaran · confidence %.2f · Tidak ada yang terkirim ke pelanggan tanpa keputusan Anda.</p></div>`,
		c["emails"], c["whatsapp"], c["meetings"], c["payments"], b.Confidence)
	return t.String(), hb.String()
}

func num(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

func (a *Agents) fakeBrief(req llm.Request) (any, error) {
	facts, _ := req.FakeInput.([]BriefFact)
	var pts []map[string]any
	for _, f := range facts {
		d := f.Data
		var text string
		switch f.Kind {
		case "gap":
			text = fmt.Sprintf("Commit Q%v %s dari target %s.", d["quarter"], domain.FormatRp1(num(d, "commit")), domain.FormatRp1(num(d, "target")))
			if f.Account != "" && num(d, "gap") > 0 {
				text += fmt.Sprintf(" Gap %s tertutup kalau PO %s (%s) masuk sebelum %v", domain.FormatM1(num(d, "gap")), f.Account, domain.FormatRp(num(d, "po_value")), d["deadline_day"])
				if num(d, "po_late_days") > 0 {
					text += fmt.Sprintf(" — mereka janji %v, sudah lewat %d hari.", d["po_due"], int(num(d, "po_late_days")))
				} else {
					text += "."
				}
			}
		case "risk":
			text = fmt.Sprintf("%s diam %d hari sejak revisi harga.", f.Account, int(num(d, "days_quiet")))
			if b, _ := d["competitor"].(bool); b {
				text += fmt.Sprintf(" Di email terakhir %v menyebut vendor lain.", d["champion"])
			}
			if b, _ := d["single"].(bool); b {
				text += fmt.Sprintf(" Semua komunikasi lewat satu orang; saya sarankan %v menembus %v.", d["owner"], defaultStr(fmt.Sprint(d["decision_role"]), "pengambil keputusan"))
			}
		case "people":
			text = fmt.Sprintf("%v, champion kita di %s, pindah tugas. Presentasi berikutnya butuh champion baru — kandidat terkuat %v (%v), %d kali ikut meeting.",
				d["ghost"], f.Account, d["candidate"], d["candidate_role"], int(num(d, "meetings")))
		case "opportunity":
			text = fmt.Sprintf("Pembayaran %s dari %s masuk tepat waktu — akun ini layak ditawari paket maintenance; saya siapkan drafnya kalau Anda mau.", domain.FormatRp(num(d, "paid")), shortName(f.Account))
		}
		pts = append(pts, map[string]any{"kind": f.Kind, "text": text, "account_id": f.AccountID, "evidence": f.Evidence})
	}
	return map[string]any{"points": pts, "confidence": 0.91}, nil
}

func shortName(n string) string {
	return strings.TrimSuffix(strings.TrimSuffix(n, " Semarang"), " Surabaya")
}

// LatestBrief loads the most recent brief.
func (a *Agents) LatestBrief(ctx context.Context) (*Brief, error) {
	var b Brief
	var pts, counts []byte
	err := a.DB.Pool.QueryRow(ctx, `SELECT id, slot, points, source_counts, confidence::float8, model, created_at, text_body, html FROM briefs ORDER BY created_at DESC LIMIT 1`).
		Scan(&b.ID, &b.Slot, &pts, &counts, &b.Confidence, &b.Model, &b.CreatedAt, &b.TextBody, &b.HTMLBody)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(pts, &b.Points)
	_ = json.Unmarshal(counts, &b.SourceCounts)
	return &b, nil
}
