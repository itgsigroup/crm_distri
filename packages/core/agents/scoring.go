package agents

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"arc/packages/core/actions"
	"arc/packages/core/domain"
	"arc/packages/core/health"
	"arc/packages/core/storage"
)

//go:embed rules/nba.json
var nbaJSON []byte

// NBARule is one row of rules/nba.json.
type NBARule struct {
	ID   string `json:"id"`
	When struct {
		SignalsAny         []string `json:"signals_any"`
		SignalsAll         []string `json:"signals_all"`
		MeetingTomorrow    bool     `json:"meeting_tomorrow"`
		CommitmentDueToday bool     `json:"commitment_due_today"`
		WarrantyDaysLTE    int      `json:"warranty_days_lte"`
		Stage              string   `json:"stage"`
		TagAny             []string `json:"tag_any"`
	} `json:"when"`
	Then struct {
		Agent  string   `json:"agent"`
		Type   string   `json:"type"`
		Kind   string   `json:"kind"`
		Icon   string   `json:"icon"`
		Button string   `json:"button"`
		Due    string   `json:"due"`
		Draft  string   `json:"draft"`
		Title  string   `json:"title"`
		Why    string   `json:"why"`
		Prep   string   `json:"prep"`
		Steps  []string `json:"steps"`
	} `json:"then"`
}

// NBAState is the evidence about one opportunity the rules read.
type NBAState struct {
	Signals            map[string]bool
	MeetingTomorrow    bool
	MeetingTime        string
	CommitmentDueToday string
	WarrantyDays       *int
	Stage              string
	Tags               []string
	Vars               map[string]string
}

// LoadNBARules parses the embedded rule table.
func LoadNBARules() []NBARule {
	var doc struct {
		Rules []NBARule `json:"rules"`
	}
	_ = json.Unmarshal(nbaJSON, &doc)
	return doc.Rules
}

// SelectNBA returns the first matching rule.
func SelectNBA(rules []NBARule, st NBAState) (NBARule, bool) {
	for _, r := range rules {
		w := r.When
		if len(w.SignalsAny) > 0 {
			ok := false
			for _, s := range w.SignalsAny {
				if st.Signals[s] {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		okAll := true
		for _, s := range w.SignalsAll {
			if !st.Signals[s] {
				okAll = false
			}
		}
		if !okAll {
			continue
		}
		if w.MeetingTomorrow && !st.MeetingTomorrow {
			continue
		}
		if w.CommitmentDueToday && st.CommitmentDueToday == "" {
			continue
		}
		if w.WarrantyDaysLTE > 0 && (st.WarrantyDays == nil || *st.WarrantyDays > w.WarrantyDaysLTE) {
			continue
		}
		if w.Stage != "" && w.Stage != st.Stage {
			continue
		}
		if len(w.TagAny) > 0 {
			ok := false
			for _, t := range w.TagAny {
				for _, x := range st.Tags {
					if strings.EqualFold(t, x) {
						ok = true
					}
				}
			}
			if !ok {
				continue
			}
		}
		if len(w.SignalsAny) == 0 && len(w.SignalsAll) == 0 && !w.CommitmentDueToday && w.Stage == "" {
			continue
		}
		return r, true
	}
	return NBARule{}, false
}

func fill(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// ScoreResult summarises a scoring run.
type ScoreResult struct {
	Scored, Signals, Actions int
}

// ScoreOpportunities recomputes health (components with evidence), snapshots,
// deterministic signals and next-best-actions for open opportunities.
// ids limits the run (empty = all).
func (a *Agents) ScoreOpportunities(ctx context.Context, ids ...string) (ScoreResult, error) {
	var res ScoreResult
	rows, err := a.DB.Pool.Query(ctx, `SELECT o.id, o.account_id, a.normal_rhythm_days, sd.name, o.tags, COALESCE(u.name,''), a.name, o.signal
		FROM opportunities o JOIN accounts a ON a.id=o.account_id JOIN stage_definitions sd ON sd.id=o.stage_id LEFT JOIN users u ON u.id=o.owner_user_id
		WHERE o.status='open' AND NOT o.historical AND (cardinality($1::text[]) = 0 OR o.id = ANY($1))`, ids)
	if err != nil {
		return res, err
	}
	type opp struct {
		id, acc, stage, owner, account, signal string
		rhythm                                 int
		tags                                   []string
	}
	var opps []opp
	for rows.Next() {
		var o opp
		if err := rows.Scan(&o.id, &o.acc, &o.rhythm, &o.stage, &o.tags, &o.owner, &o.account, &o.signal); err != nil {
			rows.Close()
			return res, err
		}
		opps = append(opps, o)
	}
	rows.Close()
	now := domain.Now()
	quiet := int(a.Ins.PolicyFloat(ctx, "quiet_threshold_days", 14))
	single := a.Ins.PolicyFloat(ctx, "single_thread_share", 80) / 100
	rules := LoadNBARules()
	for _, o := range opps {
		// ---- evidence ----
		var live, n30, n14, nPrev14 int
		var lastAt *time.Time
		_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE raw_ref NOT LIKE 'fixture%' AND raw_ref NOT LIKE 'wa:fixture%' AND raw_ref NOT LIKE 'agg:%' AND raw_ref NOT LIKE 'email:arc-fixture%' AND occurred_at >= $2 - interval '90 days'),
			count(*) FILTER (WHERE occurred_at >= $2 - interval '30 days'), count(*) FILTER (WHERE occurred_at >= $2 - interval '14 days'),
			count(*) FILTER (WHERE occurred_at < $2 - interval '14 days' AND occurred_at >= $2 - interval '28 days'), max(occurred_at)
			FROM interactions WHERE account_id=$1 AND channel <> 'wa_aggregate' AND occurred_at <= $2 + interval '1 hour'`, o.acc, now).Scan(&live, &n30, &n14, &nPrev14, &lastAt)
		daysQuiet := 0
		if lastAt != nil {
			daysQuiet = domain.DaysBetween(*lastAt, now)
		}
		var comps health.Components
		stored := map[string]int{}
		r2, err := a.DB.Pool.Query(ctx, `SELECT component, value FROM health_components WHERE opportunity_id=$1`, o.id)
		if err != nil {
			return res, err
		}
		for r2.Next() {
			var k string
			var v int
			_ = r2.Scan(&k, &v)
			stored[k] = v
		}
		r2.Close()
		for k, v := range stored {
			comps.Set(k, v)
		}
		if _, ok := stored["fit"]; !ok {
			comps.Fit = 60
		}
		if live >= 5 {
			// Enough live evidence: compute engagement, multithreading, momentum, sentiment.
			comps.Engagement = health.Engagement(health.EngagementInput{Interactions30d: n30, NormalRhythmDays: o.rhythm, DaysSinceLast: daysQuiet})
			var contacts []health.Contact
			r3, err := a.DB.Pool.Query(ctx, `SELECT p.stakeholder_tag, p.strength, EXISTS(SELECT 1 FROM interactions i WHERE p.id = ANY(i.person_ids) AND i.occurred_at >= $2 - interval '30 days')
				FROM people p WHERE p.account_id=$1 AND NOT p.is_internal`, o.acc, now)
			if err != nil {
				return res, err
			}
			for r3.Next() {
				var c health.Contact
				_ = r3.Scan(&c.Tag, &c.Strength, &c.Active)
				contacts = append(contacts, c)
			}
			r3.Close()
			comps.Multithreading = health.Multithreading(contacts)
			var tk, tl, ok, ol int
			_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE who='mereka' AND status='done'), count(*) FILTER (WHERE who='mereka' AND (status='late' OR (status='open' AND due_at < $2))),
				count(*) FILTER (WHERE who='kami' AND status='done'), count(*) FILTER (WHERE who='kami' AND (status='late' OR (status='open' AND due_at < $2)))
				FROM commitments WHERE account_id=$1`, o.acc, now).Scan(&tk, &tl, &ok, &ol)
			trend := 0.0
			if nPrev14 > 0 {
				trend = float64(n14-nPrev14) / float64(nPrev14)
			} else if n14 > 0 {
				trend = 1
			}
			comps.Momentum = health.Momentum(health.MomentumInput{Trend14: trend, TheirCommitsKept: tk, TheirCommitsLate: tl, OurCommitsKept: ok, OurCommitsLate: ol})
			var sents []float64
			r4, err := a.DB.Pool.Query(ctx, `SELECT sentiment::float8 FROM interactions WHERE account_id=$1 AND sentiment IS NOT NULL ORDER BY occurred_at DESC LIMIT 5`, o.acc)
			if err == nil {
				for r4.Next() {
					var s float64
					_ = r4.Scan(&s)
					sents = append(sents, s)
				}
				r4.Close()
			}
			comps.Sentiment = health.Sentiment(sents)
			evidence := map[string]any{"interactions_30d": n30, "days_quiet": daysQuiet, "rhythm": o.rhythm, "commitments": map[string]int{"their_kept": tk, "their_late": tl, "our_kept": ok, "our_late": ol}}
			for _, k := range []string{"engagement", "multithreading", "momentum", "sentiment"} {
				if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO health_components(opportunity_id,component,value,origin,evidence,confidence,model,prompt_version)
					VALUES ($1,$2,$3,'computed',$4,0.8,'rules','health/v1') ON CONFLICT (opportunity_id,component) DO UPDATE SET value=EXCLUDED.value, origin='computed', evidence=EXCLUDED.evidence, updated_at=now()`,
					o.id, k, comps.Get(k), storage.JSON([]any{evidence})); err != nil {
					return res, err
				}
			}
		}
		h := health.Compute(comps)
		var past *int
		_ = a.DB.Pool.QueryRow(ctx, `SELECT health FROM health_snapshots WHERE opportunity_id=$1 AND taken_on <= $2 ORDER BY taken_on DESC LIMIT 1`, o.id, now.AddDate(0, 0, -30).Format("2006-01-02")).Scan(&past)
		if past == nil {
			_ = a.DB.Pool.QueryRow(ctx, `SELECT health FROM health_snapshots WHERE opportunity_id=$1 ORDER BY taken_on LIMIT 1`, o.id).Scan(&past)
		}
		trend := 0
		if past != nil {
			trend = h.Health - *past
		}
		bd := storage.JSONObj(h.Components)
		if _, err := a.DB.Pool.Exec(ctx, `UPDATE opportunities SET health=$2, health_breakdown=$3, health_trend_30d=$4, updated_at=now() WHERE id=$1`, o.id, h.Health, bd, trend); err != nil {
			return res, err
		}
		if _, err := a.DB.Pool.Exec(ctx, `UPDATE accounts SET health=$2, health_trend_30d=$3, updated_at=now() WHERE id=$1`, o.acc, h.Health, trend); err != nil {
			return res, err
		}
		if _, err := a.DB.Pool.Exec(ctx, `INSERT INTO health_snapshots(opportunity_id,health,breakdown,taken_on) VALUES ($1,$2,$3,$4) ON CONFLICT (opportunity_id,taken_on) DO UPDATE SET health=EXCLUDED.health, breakdown=EXCLUDED.breakdown`,
			o.id, h.Health, bd, now.Format("2006-01-02")); err != nil {
			return res, err
		}
		res.Scored++

		// ---- deterministic signals ----
		detect := func(typ, sev, title, detail, action string, on bool) error {
			key := typ + ":" + o.id
			if on {
				tag, err := a.DB.Pool.Exec(ctx, `INSERT INTO signals(type,severity,account_id,opportunity_id,title,detail,suggested_action,evidence,confidence,dedupe_key,detected_at)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0.9,$9,$10) ON CONFLICT (dedupe_key) DO UPDATE SET resolved_at=NULL, detected_at=EXCLUDED.detected_at, updated_at=now() WHERE signals.resolved_at IS NOT NULL`,
					typ, sev, o.acc, o.id, title, detail, action, storage.JSON([]domain.Evidence{{Source: "detector", Quote: detail}}), key, now)
				if err == nil && tag.RowsAffected() > 0 {
					res.Signals++
				}
				return err
			}
			_, err := a.DB.Pool.Exec(ctx, `UPDATE signals SET resolved_at=$2, updated_at=now() WHERE dedupe_key=$1 AND resolved_at IS NULL AND evidence->0->>'source'='detector'`, key, now)
			return err
		}
		silentLimit := quiet
		if 2*o.rhythm < silentLimit {
			silentLimit = 2 * o.rhythm
		}
		sev := "warn"
		if daysQuiet > 20 {
			sev = "bad"
		}
		if err := detect("silent", sev, fmt.Sprintf("Sunyi %d hari", daysQuiet), fmt.Sprintf("Ritme normal akun ini %d hari", o.rhythm), "Kirim sapaan (draf siap)", lastAt != nil && daysQuiet > silentLimit); err != nil {
			return res, err
		}
		var topName string
		var topN, total float64
		_ = a.DB.Pool.QueryRow(ctx, `WITH m AS (SELECT p.name, sum(i.message_count)::float8 n FROM interactions i JOIN people p ON p.id = ANY(i.person_ids)
			WHERE i.account_id=$1 AND i.channel IN ('wa_aggregate','wa_message') AND i.occurred_at >= $2 - interval '30 days' AND NOT p.is_internal GROUP BY p.name)
			SELECT COALESCE((SELECT name FROM m ORDER BY n DESC LIMIT 1),''), COALESCE((SELECT max(n) FROM m),0), COALESCE((SELECT sum(n) FROM m),0)`, o.acc, now).Scan(&topName, &topN, &total)
		share := 0.0
		if total > 0 {
			share = topN / total
		}
		if err := detect("single_threaded", "warn", "Single-threaded", fmt.Sprintf("%d%% interaksi lewat %s", int(math.Round(share*100)), topName), "Draf surat ke pengambil keputusan", total >= 20 && share >= single); err != nil {
			return res, err
		}
		var poDays int
		_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(max(EXTRACT(DAY FROM ($2 - due_at)))::int, 0) FROM commitments WHERE account_id=$1 AND who='mereka' AND status IN ('open','late') AND text ILIKE '%PO%' AND due_at < $2`, o.acc, now).Scan(&poDays)
		if err := detect("po_overdue", "warn", fmt.Sprintf("PO lewat %d hari", poDays), "Komitmen PO dari pelanggan melewati tenggat", "Angkat di meeting berikutnya", poDays > 0); err != nil {
			return res, err
		}
		_, _ = a.DB.Pool.Exec(ctx, `UPDATE commitments SET status='late', updated_at=now() WHERE account_id=$1 AND status='open' AND due_at < $2`, o.acc, now.Add(-12*time.Hour))

		// ---- next best action (one active per opportunity) ----
		var active bool
		_ = a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM actions WHERE opportunity_id=$1 AND status IN ('proposed','snoozed'))`, o.id).Scan(&active)
		if active {
			continue
		}
		st, err := a.nbaState(ctx, o.id, o.acc, o.stage, o.tags, daysQuiet, o.rhythm, o.owner, o.account)
		if err != nil {
			return res, err
		}
		rule, ok := SelectNBA(rules, st)
		if !ok {
			continue
		}
		p := actions.Proposal{Agent: rule.Then.Agent, Type: rule.Then.Type, Kind: rule.Then.Kind, Icon: rule.Then.Icon, ButtonLabel: rule.Then.Button,
			AccountID: o.acc, OpportunityID: o.id, DueLabel: fill(rule.Then.Due, st.Vars), Title: fill(rule.Then.Title, st.Vars),
			Why: fill(rule.Then.Why, st.Vars), Prep: fill(rule.Then.Prep, st.Vars), Confidence: 0.82, Model: "rules/nba",
			Evidence: []domain.Evidence{{Source: "nba:" + rule.ID, Quote: fill(rule.Then.Why, st.Vars)}}}
		for _, s := range rule.Then.Steps {
			p.Steps = append(p.Steps, fill(s, st.Vars))
		}
		if rule.Then.Draft != "" {
			d, model, err := a.Draft(ctx, DraftInput{Kind: rule.Then.Draft, Account: o.account, Contact: st.Vars["contact"], Owner: o.owner, Context: st.Vars["commitment"],
				Channel: map[string]string{"send_wa": "WhatsApp", "send_email": "email"}[rule.Then.Type], DaysQuiet: daysQuiet, Rhythm: o.rhythm})
			if err == nil {
				p.Preview, p.Model = d.Preview, model
				if rule.Then.Type == "send_wa" {
					p.PreviewFrom = "WhatsApp · dari nomor " + o.owner
				} else {
					p.PreviewFrom = "Dari: " + strings.ToLower(o.owner) + "@gsi.co.id"
				}
			}
			if sess, jid := a.contactChat(ctx, o.acc, st.Vars["contact"]); sess != "" {
				p.Payload = map[string]any{"session": sess, "chat_jid": jid, "to": st.Vars["contact"]}
			}
		}
		if _, created, err := a.Actions.Propose(ctx, p, storage.Actor{ID: "nba", Type: "agent"}); err != nil {
			return res, err
		} else if created {
			res.Actions++
		}
	}
	return res, nil
}

func (a *Agents) contactChat(ctx context.Context, accountID, contact string) (string, string) {
	var sess, jid string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT t.session_id, t.chat_jid FROM chat_threads t JOIN people p ON p.id=t.person_id WHERE t.account_id=$1 AND (p.name=$2 OR $2='') AND t.type='cust' LIMIT 1`,
		accountID, contact).Scan(&sess, &jid)
	return sess, jid
}

func (a *Agents) nbaState(ctx context.Context, oppID, accID, stage string, tags []string, daysQuiet, rhythm int, owner, account string) (NBAState, error) {
	now := domain.Now()
	st := NBAState{Signals: map[string]bool{}, Stage: stage, Tags: tags, Vars: map[string]string{"account": account, "owner": owner, "days": fmt.Sprint(daysQuiet), "rhythm": fmt.Sprint(rhythm)}}
	rows, err := a.DB.Pool.Query(ctx, `SELECT type FROM signals WHERE (opportunity_id=$1 OR (account_id=$2 AND opportunity_id IS NULL)) AND resolved_at IS NULL`, oppID, accID)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var t string
		_ = rows.Scan(&t)
		st.Signals[t] = true
	}
	rows.Close()
	var champion, decision, candidate, contact string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT name FROM people WHERE account_id=$1 AND stakeholder_tag IN ('champion','ghost') ORDER BY strength DESC LIMIT 1),''),
		COALESCE((SELECT name FROM people WHERE account_id=$1 AND stakeholder_tag='decision' ORDER BY strength LIMIT 1),''),
		COALESCE((SELECT name FROM people WHERE account_id=$1 AND stakeholder_tag IN ('influencer','user') ORDER BY strength DESC LIMIT 1),''),
		COALESCE((SELECT name FROM people WHERE account_id=$1 AND NOT is_internal ORDER BY strength DESC LIMIT 1),'')`, accID).Scan(&champion, &decision, &candidate, &contact)
	st.Vars["champion"], st.Vars["decision"], st.Vars["candidate"], st.Vars["contact"] = champion, defaultStr(decision, "pengambil keputusan"), candidate, contact
	var mt *time.Time
	_ = a.DB.Pool.QueryRow(ctx, `SELECT min(starts_at) FROM calendar_events WHERE account_id=$1 AND starts_at >= $2 AND starts_at < $3`, accID,
		domain.StartOfDay(now).AddDate(0, 0, 1), domain.StartOfDay(now).AddDate(0, 0, 2)).Scan(&mt)
	if mt != nil {
		st.MeetingTomorrow = true
		st.MeetingTime = domain.ClockID(*mt)
		st.Vars["meeting_time"] = st.MeetingTime
	}
	var due string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT text FROM commitments WHERE account_id=$1 AND who='kami' AND status='open' AND due_at >= $2 AND due_at < $3 LIMIT 1`, accID,
		domain.StartOfDay(now), domain.StartOfDay(now).AddDate(0, 0, 1)).Scan(&due)
	st.CommitmentDueToday = due
	st.Vars["commitment"] = due
	var we *time.Time
	_ = a.DB.Pool.QueryRow(ctx, `SELECT min(warranty_end) FROM installed_systems WHERE account_id=$1 AND warranty_end >= $2`, accID, now).Scan(&we)
	if we != nil {
		d := domain.DaysBetween(now, *we)
		st.WarrantyDays = &d
		st.Vars["warranty"] = domain.MonthLong(we.Month())
	}
	var poDays int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(max(EXTRACT(DAY FROM ($2 - due_at)))::int,0) FROM commitments WHERE account_id=$1 AND who='mereka' AND text ILIKE '%PO%' AND status IN ('open','late') AND due_at < $2`, accID, now).Scan(&poDays)
	if poDays > 0 {
		st.Vars["days"] = fmt.Sprint(poDays)
	}
	return st, nil
}
