package insights

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"arc/packages/core/domain"
)

// PulseRow is one "Denyut bisnis" metric.
type PulseRow struct {
	Key   string
	Label string
	Value string
	Delta string
	Tone  string // good | bad | n
	Raw   float64
}

func (s *Service) arcStart(ctx context.Context) time.Time {
	var v string
	if s.Setting(ctx, "arc_active_since", &v) {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return domain.Now().AddDate(0, -3, 0)
}

func medianF(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

func meanF(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	t := 0.0
	for _, x := range v {
		t += x
	}
	return t / float64(len(v))
}

func (s *Service) daysBetweenCols(ctx context.Context, from, to, periodCol string, a, b time.Time) ([]float64, error) {
	rows, err := s.DB.Pool.Query(ctx, fmt.Sprintf(`SELECT EXTRACT(EPOCH FROM (%s - %s))/86400 FROM opportunities
		WHERE %s IS NOT NULL AND %s IS NOT NULL AND %s >= $1 AND %s < $2`, to, from, from, to, periodCol, periodCol), a, b)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []float64
	for rows.Next() {
		var d float64
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, math.Round(d))
	}
	return out, rows.Err()
}

func delta(after, before float64, unit string, lowerIsBetter bool) (string, string) {
	d := after - before
	tone := "good"
	if (d > 0 && lowerIsBetter) || (d < 0 && !lowerIsBetter) {
		tone = "bad"
	}
	sign := "+"
	if d < 0 {
		sign = "−"
	}
	return fmt.Sprintf("%s%s %s", sign, trimNum(math.Abs(d)), unit), tone
}

func trimNum(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%d", int(v))
	}
	return strings.ReplaceAll(fmt.Sprintf("%.1f", v), ".", ",")
}

// Pulse computes the business pulse since ARC went live vs the 6 months before.
func (s *Service) Pulse(ctx context.Context) ([]PulseRow, error) {
	start := s.arcStart(ctx)
	now := domain.Now()
	before := start.AddDate(0, -6, 0)
	var rows []PulseRow

	l2cA, err := s.daysBetweenCols(ctx, "quotation_at", "paid_at", "paid_at", start, now.Add(time.Hour))
	if err != nil {
		return nil, err
	}
	l2cB, _ := s.daysBetweenCols(ctx, "quotation_at", "paid_at", "paid_at", before, start)
	a, b := medianF(l2cA), medianF(l2cB)
	d, t := delta(a, b, "hr", true)
	rows = append(rows, PulseRow{"lead_to_cash", "Lead → cash (median)", trimNum(a) + " hr", d, t, a})

	dsoA, _ := s.daysBetweenCols(ctx, "invoice_at", "paid_at", "paid_at", start, now.Add(time.Hour))
	dsoB, _ := s.daysBetweenCols(ctx, "invoice_at", "paid_at", "paid_at", before, start)
	a, b = math.Round(meanF(dsoA)), math.Round(meanF(dsoB))
	d, t = delta(a, b, "hr", true)
	rows = append(rows, PulseRow{"dso", "DSO piutang", trimNum(a) + " hr", d, t, a})

	w2iA, _ := s.daysBetweenCols(ctx, "won_at", "invoice_at", "paid_at", start, now.Add(time.Hour))
	w2iB, _ := s.daysBetweenCols(ctx, "won_at", "invoice_at", "paid_at", before, start)
	a, b = medianF(w2iA), medianF(w2iB)
	d, t = delta(a, b, "hr", true)
	rows = append(rows, PulseRow{"won_to_invoice", "Won → invoice", trimNum(a) + " hr", d, t, a})

	var respA, respB float64
	_ = s.DB.Pool.QueryRow(ctx, `SELECT COALESCE(avg(first_response_minutes),0) FROM opportunities WHERE first_response_minutes IS NOT NULL AND lead_at >= $1 AND lead_at < $2`, start, now.Add(time.Hour)).Scan(&respA)
	_ = s.DB.Pool.QueryRow(ctx, `SELECT COALESCE(avg(first_response_minutes),0) FROM opportunities WHERE first_response_minutes IS NOT NULL AND lead_at >= $1 AND lead_at < $2`, before, start).Scan(&respB)
	ha, hb := math.Round(respA/6)/10, math.Round(respB/6)/10
	d, t = delta(ha, hb, "jam", true)
	rows = append(rows, PulseRow{"response", "Respons lead baru", trimNum(ha) + " jam", d, t, ha})

	cov, label, target, err := s.Coverage(ctx)
	if err != nil {
		return nil, err
	}
	tone := "good"
	if cov < target {
		tone = "bad"
	}
	rows = append(rows, PulseRow{"coverage", "Pipeline coverage " + label, strings.ReplaceAll(fmt.Sprintf("%.1f×", cov), ".", ","), fmt.Sprintf("target %s×", trimNum(target)), tone, cov})

	var acc []struct {
		Q         string  `json:"q"`
		Committed float64 `json:"committed"`
		Actual    float64 `json:"actual"`
	}
	if s.Setting(ctx, "commit_accuracy", &acc) && len(acc) > 0 {
		last := acc[len(acc)-1]
		v := math.Round(last.Actual / last.Committed * 100)
		dl := "—"
		if len(acc) > 1 {
			p := acc[len(acc)-2]
			dl = fmt.Sprintf("%s: %d%%", strings.Fields(p.Q)[0], int(math.Round(p.Actual/p.Committed*100)))
		}
		rows = append(rows, PulseRow{"commit_accuracy", "Akurasi commit", fmt.Sprintf("%d%%", int(v)), dl, "n", v})
	}
	return rows, nil
}

// Coverage is next-quarter pipeline divided by the next-quarter target.
func (s *Service) Coverage(ctx context.Context) (float64, string, float64, error) {
	now := domain.Now()
	q, qend := domain.Quarter(now)
	nstart := qend.AddDate(0, 0, 1)
	_, nend := domain.Quarter(nstart)
	nq := q%4 + 1
	var pipe float64
	err := s.DB.Pool.QueryRow(ctx, `SELECT COALESCE(sum(o.expected_revenue),0)::float8 FROM opportunities o JOIN stage_definitions sd ON sd.id=o.stage_id
		WHERE NOT o.historical AND o.status='open' AND sd.seq >= 2 AND o.date_deadline >= $1 AND o.date_deadline <= $2`, nstart, nend.AddDate(0, 0, 1)).Scan(&pipe)
	target := s.PolicyFloat(ctx, "next_quarter_target", 6e9)
	return pipe / target, fmt.Sprintf("Q%d", nq), s.PolicyFloat(ctx, "pipeline_coverage_target", 3), err
}

// FunnelStage is one funnel row.
type FunnelStage struct {
	Key, Label, Sub string
	N               int
}

var funnelDefs = []FunnelStage{
	{"masuk", "Kontak masuk", "WA, form, telepon, tender, referral", 0},
	{"teridentifikasi", "Teridentifikasi", "siapa & dari mana", 0},
	{"relevan", "Prospek relevan", "bukan vendor/spam/di luar segmen", 0},
	{"pain_point", "Pain point tergali", "≥ 2 pertanyaan terjawab", 0},
	{"lead", "Lead di Odoo", "Baru / Berkualifikasi", 0},
	{"penawaran", "Penawaran", "proposal terkirim", 0},
	{"won", "Won", "bulan ini", 0},
}

// Funnel counts subjects per stage in a month (YYYY-MM; "" = current).
func (s *Service) Funnel(ctx context.Context, month string) ([]FunnelStage, time.Time, error) {
	now := domain.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, domain.Jakarta)
	if month != "" {
		if t, err := time.ParseInLocation("2006-01", month, domain.Jakarta); err == nil {
			start = t
		}
	}
	end := start.AddDate(0, 1, 0)
	out := make([]FunnelStage, len(funnelDefs))
	copy(out, funnelDefs)
	rows, err := s.DB.Pool.Query(ctx, `SELECT stage, count(DISTINCT subject_key) FROM funnel_events WHERE at >= $1 AND at < $2 GROUP BY stage`, start, end)
	if err != nil {
		return nil, start, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, start, err
		}
		counts[st] = n
	}
	for i := range out {
		out[i].N = counts[out[i].Key]
	}
	return out, start, rows.Err()
}

// FunnelTiming returns median minutes masuk→teridentifikasi and days teridentifikasi→lead in the month.
func (s *Service) FunnelTiming(ctx context.Context, start time.Time) (float64, float64) {
	end := start.AddDate(0, 1, 0)
	q := func(a, b string) float64 {
		rows, err := s.DB.Pool.Query(ctx, `SELECT EXTRACT(EPOCH FROM (y.at - x.at)) FROM funnel_events x JOIN funnel_events y ON y.subject_key=x.subject_key
			WHERE x.stage=$1 AND y.stage=$2 AND x.at >= $3 AND x.at < $4`, a, b, start, end)
		if err != nil {
			return 0
		}
		defer rows.Close()
		var v []float64
		for rows.Next() {
			var sec float64
			if rows.Scan(&sec) == nil {
				v = append(v, sec)
			}
		}
		return medianF(v)
	}
	return q("masuk", "teridentifikasi") / 60, q("teridentifikasi", "lead") / 86400
}

// SourceRow is win rate per deal source over 24 months.
type SourceRow struct {
	Source, Label string
	N, Won, Lost  int
	WinRate       float64
}

var sourceLabels = map[string]string{"ekspansi": "Ekspansi", "referral": "Referral", "inbound": "Inbound (WA/web)", "tender": "Tender", "cold": "Cold outreach"}

// Sources computes deals and win rate per source in the last 24 months.
func (s *Service) Sources(ctx context.Context) ([]SourceRow, int, error) {
	since := domain.Now().AddDate(-2, 0, 0)
	rows, err := s.DB.Pool.Query(ctx, `SELECT source, count(*), count(*) FILTER (WHERE status='won'), count(*) FILTER (WHERE status='lost')
		FROM opportunities WHERE source <> '' AND status IN ('won','lost') AND lead_at >= $1 GROUP BY source`, since)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []SourceRow
	total := 0
	for rows.Next() {
		var r SourceRow
		if err := rows.Scan(&r.Source, &r.N, &r.Won, &r.Lost); err != nil {
			return nil, 0, err
		}
		r.Label = sourceLabels[r.Source]
		if r.Won+r.Lost > 0 {
			r.WinRate = float64(r.Won) / float64(r.Won+r.Lost)
		}
		total += r.N
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WinRate > out[j].WinRate })
	return out, total, rows.Err()
}

// Flywheel is the computed answer to "funnel atau flywheel?".
type Flywheel struct {
	ExistingShare float64
	WinExisting   float64
	WinOthers     float64
	Multiplier    [2]float64
	ReferralRate  float64
	ExpansionRate float64
	TimeToDelight float64
	TTDTarget     float64
	UseFlywheel   bool
	Total         int
}

// ComputeFlywheel derives the verdict from source win rates (existing customers =
// ekspansi + referral) and three flywheel metrics.
func (s *Service) ComputeFlywheel(ctx context.Context) (Flywheel, error) {
	src, total, err := s.Sources(ctx)
	if err != nil {
		return Flywheel{}, err
	}
	fw := Flywheel{Total: total, TTDTarget: 21}
	var exN, exW, exD, otW, otD int
	minM, maxM := math.Inf(1), 0.0
	exRates := []float64{}
	otRates := []float64{}
	for _, r := range src {
		if r.Source == "ekspansi" || r.Source == "referral" {
			exN += r.N
			exW += r.Won
			exD += r.Won + r.Lost
			exRates = append(exRates, r.WinRate)
		} else {
			otW += r.Won
			otD += r.Won + r.Lost
			otRates = append(otRates, r.WinRate)
		}
	}
	for _, e := range exRates {
		for _, o := range otRates {
			if o > 0 {
				m := e / o
				minM = math.Min(minM, m)
				maxM = math.Max(maxM, m)
			}
		}
	}
	if total > 0 {
		fw.ExistingShare = float64(exN) / float64(total)
	}
	if exD > 0 {
		fw.WinExisting = float64(exW) / float64(exD)
	}
	if otD > 0 {
		fw.WinOthers = float64(otW) / float64(otD)
	}
	if !math.IsInf(minM, 1) {
		fw.Multiplier = [2]float64{minM, maxM}
	}
	fw.UseFlywheel = fw.WinOthers > 0 && fw.WinExisting >= 1.5*fw.WinOthers
	var customers, referrers, expansion int
	_ = s.DB.Pool.QueryRow(ctx, `SELECT count(DISTINCT account_id) FROM opportunities WHERE status='won'`).Scan(&customers)
	_ = s.DB.Pool.QueryRow(ctx, `SELECT count(DISTINCT referrer_account_id) FROM opportunities WHERE referrer_account_id IS NOT NULL AND lead_at >= $1`, domain.Now().AddDate(-1, 0, 0)).Scan(&referrers)
	_ = s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT account_id FROM opportunities WHERE status='won' AND product_line <> '' GROUP BY account_id HAVING count(DISTINCT product_line) >= 2) x`).Scan(&expansion)
	if customers > 0 {
		fw.ReferralRate = float64(referrers) / float64(customers)
		fw.ExpansionRate = float64(expansion) / float64(customers)
	}
	ttd, _ := s.daysBetweenCols(ctx, "won_at", "bast_at", "bast_at", domain.Now().AddDate(0, -6, 0), domain.Now().Add(time.Hour))
	fw.TimeToDelight = medianF(ttd)
	return fw, nil
}

// WinLoss returns the three learned patterns from closed deals (24 months).
type WinLoss struct {
	Deals         int
	MultiRatio    float64
	SilentLostPct float64
	GovCycle      float64
	EntCycle      float64
}

// ComputeWinLoss analyses closed deals.
func (s *Service) ComputeWinLoss(ctx context.Context) (WinLoss, error) {
	since := domain.Now().AddDate(-2, 0, 0)
	var w WinLoss
	var mw, md, sw, sd, silentLost, silent int
	err := s.DB.Pool.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER (WHERE active_stakeholders >= 2 AND status='won'), count(*) FILTER (WHERE active_stakeholders >= 2),
		count(*) FILTER (WHERE active_stakeholders < 2 AND status='won'), count(*) FILTER (WHERE active_stakeholders < 2),
		count(*) FILTER (WHERE silent_after_revision AND status='lost'), count(*) FILTER (WHERE silent_after_revision)
		FROM opportunities WHERE status IN ('won','lost') AND lead_at >= $1`, since).Scan(&w.Deals, &mw, &md, &sw, &sd, &silentLost, &silent)
	if err != nil {
		return w, err
	}
	if md > 0 && sd > 0 && sw > 0 {
		w.MultiRatio = (float64(mw) / float64(md)) / (float64(sw) / float64(sd))
	}
	if silent > 0 {
		w.SilentLostPct = float64(silentLost) / float64(silent) * 100
	}
	_ = s.DB.Pool.QueryRow(ctx, `SELECT COALESCE(avg(EXTRACT(EPOCH FROM (o.won_at-o.lead_at))/86400) FILTER (WHERE a.is_government),0),
		COALESCE(avg(EXTRACT(EPOCH FROM (o.won_at-o.lead_at))/86400) FILTER (WHERE NOT a.is_government),0)
		FROM opportunities o JOIN accounts a ON a.id=o.account_id WHERE o.status='won' AND o.historical AND o.lead_at >= $1`, since).Scan(&w.GovCycle, &w.EntCycle)
	return w, nil
}

// TeamRow is execution discipline per sales.
type TeamRow struct {
	UserID, Name, Branch string
	Pipeline             float64
	ResponseMin          float64
	FollowUpPct          float64
	MultiPct             float64
	HasData              bool
	WAMessages30d        int
	NoOppContacts        int
	TopNoOppContact      string
}

// Team computes the 30-day scorecard per sales user.
func (s *Service) Team(ctx context.Context) ([]TeamRow, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT id, name, branch FROM users WHERE role='sales' ORDER BY CASE branch WHEN 'Semarang' THEN 1 WHEN 'Yogyakarta' THEN 2 WHEN 'Surabaya' THEN 3 ELSE 4 END`)
	if err != nil {
		return nil, err
	}
	var team []TeamRow
	for rows.Next() {
		var t TeamRow
		if err := rows.Scan(&t.UserID, &t.Name, &t.Branch); err != nil {
			rows.Close()
			return nil, err
		}
		team = append(team, t)
	}
	rows.Close()
	deals, err := s.Deals(ctx, Scope{All: true})
	if err != nil {
		return nil, err
	}
	shares, err := s.topContactShare(ctx, 30)
	if err != nil {
		return nil, err
	}
	single := s.PolicyFloat(ctx, "single_thread_share", 80) / 100
	for i := range team {
		t := &team[i]
		open, multi := 0, 0
		for _, d := range deals {
			if d.Owner != t.UserID || !d.IsPipeline() {
				continue
			}
			t.Pipeline += d.Value
			open++
			if sh, ok := shares[d.AccountID]; ok && sh < single {
				multi++
			}
		}
		if open > 0 {
			t.MultiPct = float64(multi) / float64(open) * 100
			t.HasData = true
		}
		var resp, fu float64
		if s.DB.Pool.QueryRow(ctx, `SELECT value::float8 FROM metric_snapshots WHERE key='response_min' AND subject=$1`, t.UserID).Scan(&resp) == nil {
			t.ResponseMin = resp
		}
		if s.DB.Pool.QueryRow(ctx, `SELECT value::float8 FROM metric_snapshots WHERE key='followup_on_time' AND subject=$1`, t.UserID).Scan(&fu) == nil {
			t.FollowUpPct = fu
		}
		// WhatsApp contacts without any open opportunity (coaching input).
		_ = s.DB.Pool.QueryRow(ctx, `SELECT COALESCE(sum(i.message_count),0), count(DISTINCT p.id)
			FROM interactions i JOIN people p ON p.id = ANY(i.person_ids)
			WHERE $1 = ANY(i.user_ids) AND i.channel IN ('wa_aggregate','wa_message','wa_group_message') AND i.occurred_at >= $2 AND NOT p.is_internal
			  AND NOT EXISTS (SELECT 1 FROM opportunities o WHERE o.account_id=p.account_id AND o.status='open' AND NOT o.historical)`,
			t.UserID, domain.Now().AddDate(0, 0, -30)).Scan(&t.WAMessages30d, &t.NoOppContacts)
		_ = s.DB.Pool.QueryRow(ctx, `SELECT p.name FROM interactions i JOIN people p ON p.id = ANY(i.person_ids)
			WHERE $1 = ANY(i.user_ids) AND i.channel IN ('wa_aggregate','wa_message') AND i.occurred_at >= $2 AND NOT p.is_internal
			  AND NOT EXISTS (SELECT 1 FROM opportunities o WHERE o.account_id=p.account_id AND o.status='open' AND NOT o.historical)
			GROUP BY p.name ORDER BY sum(i.message_count) DESC LIMIT 1`, t.UserID, domain.Now().AddDate(0, 0, -30)).Scan(&t.TopNoOppContact)
	}
	return team, nil
}

// topContactShare returns, per account, the share of WhatsApp messages (last n days)
// going through the single most active contact.
func (s *Service) topContactShare(ctx context.Context, days int) (map[string]float64, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT p.account_id, p.id, sum(i.message_count)
		FROM interactions i JOIN people p ON p.id = ANY(i.person_ids)
		WHERE i.channel IN ('wa_aggregate','wa_message') AND i.occurred_at >= $1 AND p.account_id IS NOT NULL AND NOT p.is_internal
		GROUP BY p.account_id, p.id`, domain.Now().AddDate(0, 0, -days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tot := map[string]float64{}
	top := map[string]float64{}
	for rows.Next() {
		var acc, pid string
		var n float64
		if err := rows.Scan(&acc, &pid, &n); err != nil {
			return nil, err
		}
		tot[acc] += n
		if n > top[acc] {
			top[acc] = n
		}
	}
	out := map[string]float64{}
	for acc, t := range tot {
		if t > 0 {
			out[acc] = top[acc] / t
		}
	}
	return out, rows.Err()
}

// CalibrationRow is the 30-day acceptance rate of an agent.
type CalibrationRow struct {
	Agent    string
	Accepted int
	Total    int
	Rate     float64
}

// Calibration computes acceptance rate per agent over 30 days (reject counts against, snooze ignored).
func (s *Service) Calibration(ctx context.Context) ([]CalibrationRow, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT agent, count(*) FILTER (WHERE decision IN ('approve','edit','option')), count(*) FILTER (WHERE decision <> 'snooze')
		FROM action_decisions WHERE decided_at >= $1 GROUP BY agent`, domain.Now().AddDate(0, 0, -30))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CalibrationRow
	for rows.Next() {
		var r CalibrationRow
		if err := rows.Scan(&r.Agent, &r.Accepted, &r.Total); err != nil {
			return nil, err
		}
		if r.Total > 0 {
			r.Rate = float64(r.Accepted) / float64(r.Total) * 100
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rate > out[j].Rate })
	return out, rows.Err()
}
