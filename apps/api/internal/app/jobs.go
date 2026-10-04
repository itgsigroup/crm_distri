package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

// job is a named unit of scheduled work.
type job struct {
	name string
	fn   func(ctx context.Context) (any, error)
}

func (a *App) jobs() map[string]job {
	list := []job{
		{"extract", func(ctx context.Context) (any, error) { return a.Agents.ExtractPending(ctx, 500) }},
		{"hygiene", func(ctx context.Context) (any, error) { return a.Agents.Hygiene(ctx) }},
		{"score_opportunities", func(ctx context.Context) (any, error) { return a.Agents.ScoreOpportunities(ctx) }},
		{"meeting_prep", func(ctx context.Context) (any, error) { return a.Agents.MeetingPrep(ctx) }},
		{"collection", func(ctx context.Context) (any, error) { return a.Agents.Collection(ctx) }},
		{"renewal", func(ctx context.Context) (any, error) { return a.Agents.Renewal(ctx) }},
		{"tender_radar", func(ctx context.Context) (any, error) { return a.Agents.TenderRadar(ctx) }},
		{"coaching", func(ctx context.Context) (any, error) { return a.Agents.Coaching(ctx) }},
		{"forecast", func(ctx context.Context) (any, error) { return a.Agents.ForecastJob(ctx) }},
		{"memory", a.refreshMemories},
		{"sync_odoo", func(ctx context.Context) (any, error) { return a.SyncOdoo(ctx) }},
		{"odoo_activities", func(ctx context.Context) (any, error) { return a.SyncCommitmentActivities(ctx) }},
		{"capture_gmail", func(ctx context.Context) (any, error) { return a.CaptureGmail(ctx) }},
		{"capture_calendar", func(ctx context.Context) (any, error) { return a.CaptureCalendar(ctx) }},
		{"brief", func(ctx context.Context) (any, error) { return a.Agents.GenerateBrief(ctx, briefSlot(), true) }},
		{"identify_inbound", a.identifyPending},
		{"bridge_watch", a.watchBridgeSessions},
	}
	out := map[string]job{}
	for _, j := range list {
		out[j.name] = j
	}
	return out
}

func briefSlot() string {
	if domain.Now().Hour() < 12 {
		return "pagi"
	}
	return "sore"
}

// RunJob runs one job now and records the run.
func (a *App) RunJob(ctx context.Context, name string) (string, error) {
	j, ok := a.jobs()[name]
	if !ok {
		names := []string{}
		for k := range a.jobs() {
			names = append(names, k)
		}
		sort.Strings(names)
		return "", fmt.Errorf("job %q tidak dikenal (pilihan: %s)", name, strings.Join(names, ", "))
	}
	var runID int64
	_ = a.DB.Pool.QueryRow(ctx, `INSERT INTO jobs_runs(job) VALUES ($1) RETURNING id`, name).Scan(&runID)
	res, err := j.fn(ctx)
	detail := map[string]any{"result": res}
	if err != nil {
		detail["error"] = err.Error()
	}
	_, _ = a.DB.Pool.Exec(ctx, `UPDATE jobs_runs SET finished_at=now(), ok=$2, detail=$3 WHERE id=$1`, runID, err == nil, storage.JSONObj(detail))
	if err != nil {
		a.alertOnRepeatedFailure(ctx, name)
	}
	b, _ := json.Marshal(res)
	a.Log.Info("job", "name", name, "ok", err == nil, "result", string(b))
	return string(b), err
}

// alertOnRepeatedFailure notifies the CEO when a job fails twice in a row.
func (a *App) alertOnRepeatedFailure(ctx context.Context, name string) {
	var fails int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT ok FROM jobs_runs WHERE job=$1 AND finished_at IS NOT NULL ORDER BY id DESC LIMIT 2) x WHERE ok = false`, name).Scan(&fails)
	if fails >= 2 && a.Actions.Notify != nil {
		a.Actions.Notify(ctx, domain.RoleCEO, "ARC: job "+name+" gagal 2×", "Job "+name+" gagal dua kali berturut-turut. Lihat log ARC.")
	}
}

// Brief slots (Asia/Jakarta): morning 06:45, afternoon 16:00.
var briefSlots = [2][2]int{{6, 45}, {16, 0}}

// schedule decides which jobs are due: hourly batch (capture, hygiene, follow-up,
// meeting prep, sync), daily analytics at 06:30, briefs at 06:45 and 16:00.
type schedule struct {
	hourly, daily                     []string
	lastHour, lastDay, lastAM, lastPM int
}

func newSchedule() *schedule {
	return &schedule{
		hourly:   []string{"extract", "identify_inbound", "hygiene", "capture_gmail", "capture_calendar", "sync_odoo", "odoo_activities", "meeting_prep"},
		daily:    []string{"score_opportunities", "forecast", "collection", "renewal", "tender_radar", "coaching", "memory"},
		lastHour: -1, lastDay: -1, lastAM: -1, lastPM: -1,
	}
}

func atOrAfter(now time.Time, h, m int) bool {
	return now.Hour() > h || (now.Hour() == h && now.Minute() >= m)
}

// due returns the jobs to run at now ("cost_alert" is the hourly cost check).
func (s *schedule) due(now time.Time) []string {
	var out []string
	if now.Hour() != s.lastHour {
		s.lastHour = now.Hour()
		out = append(out, s.hourly...)
		out = append(out, "cost_alert")
	}
	if now.YearDay() != s.lastDay && atOrAfter(now, 6, 30) {
		s.lastDay = now.YearDay()
		out = append(out, s.daily...)
	}
	am, pm := briefSlots[0], briefSlots[1]
	if now.YearDay() != s.lastAM && atOrAfter(now, am[0], am[1]) && !atOrAfter(now, pm[0], pm[1]) {
		s.lastAM = now.YearDay()
		out = append(out, "brief")
	}
	if now.YearDay() != s.lastPM && atOrAfter(now, pm[0], pm[1]) {
		s.lastPM = now.YearDay()
		out = append(out, "brief")
	}
	return out
}

// runScheduler ticks every 30 seconds and runs the due jobs.
func (a *App) runScheduler(ctx context.Context) {
	sched := newSchedule()
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		for _, j := range sched.due(domain.Now()) {
			if j == "cost_alert" {
				a.checkCostAlert(ctx)
				continue
			}
			if _, err := a.RunJob(ctx, j); err != nil {
				a.Log.Warn("scheduled job failed", "job", j, "err", err)
			}
		}
	}
}

// checkCostAlert emails the CEO when today's LLM cost exceeds the threshold.
func (a *App) checkCostAlert(ctx context.Context) {
	var usd float64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(sum(cost_est),0)::float8 FROM llm_calls WHERE created_at >= $1 AND purpose NOT LIKE '%agregat%'`, domain.StartOfDay(domain.Now())).Scan(&usd)
	idr := usd * 16300
	if idr > a.Cfg.DailyCostAlertIDR && a.Actions.Notify != nil {
		var sent bool
		_ = a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notifications WHERE subject LIKE 'ARC: biaya AI%' AND created_at >= $1)`, domain.StartOfDay(domain.Now())).Scan(&sent)
		if !sent {
			a.Actions.Notify(ctx, domain.RoleCEO, "ARC: biaya AI harian melewati ambang", fmt.Sprintf("Biaya AI hari ini ±Rp %.0f (ambang Rp %.0f).", idr, a.Cfg.DailyCostAlertIDR))
			_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO notifications(channel,recipient,subject) VALUES ('email','ceo','ARC: biaya AI harian')`)
		}
	}
}

func (a *App) refreshMemories(ctx context.Context) (any, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT DISTINCT a.id FROM accounts a JOIN interactions i ON i.account_id=a.id
		WHERE i.occurred_at > COALESCE(a.memory_updated_at, '-infinity') AND i.channel <> 'wa_aggregate' AND i.raw_ref NOT LIKE 'fixture:%'`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if err := a.Agents.UpdateMemory(ctx, id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (a *App) identifyPending(ctx context.Context) (any, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT id FROM inbound_contacts WHERE status='unknown' AND (identification->>'model') IS NULL AND created_at > now() - interval '7 days'`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := a.Agents.IdentifyInbound(ctx, id); err != nil {
			return nil, err
		}
	}
	return len(ids), nil
}
