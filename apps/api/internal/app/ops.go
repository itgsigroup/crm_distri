package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"arc/packages/core/domain"
)

// watchBridgeSessions alerts the CEO once per outage when a bridge session the
// bridge itself reported as disconnected stays down for more than 10 minutes.
// Real wall-clock time is used (not the demo clock): outages are operational.
func (a *App) watchBridgeSessions(ctx context.Context) (any, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT s.id, s.label FROM wa_sessions s
		WHERE s.transport='bridge' AND s.status='disconnected' AND s.updated_at < now() - interval '10 minutes'
		  AND EXISTS (SELECT 1 FROM audit_log l WHERE l.object_type='wa_session' AND l.object_id=s.id AND l.verb='session.disconnected' AND l.at >= s.updated_at - interval '1 minute')
		  AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.subject = 'ARC: sesi WhatsApp putus · ' || s.id AND n.created_at >= s.updated_at)`)
	if err != nil {
		return nil, err
	}
	type down struct{ id, label string }
	var list []down
	for rows.Next() {
		var d down
		if err := rows.Scan(&d.id, &d.label); err == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		subject := "ARC: sesi WhatsApp putus · " + d.id
		if a.Actions.Notify != nil {
			a.Actions.Notify(ctx, domain.RoleCEO, subject, fmt.Sprintf("Sesi %s terputus lebih dari 10 menit. Buka Pengaturan → Nomor WhatsApp untuk menautkan ulang (scan QR).", d.label))
		}
		_, _ = a.DB.Pool.Exec(ctx, `INSERT INTO notifications(channel,recipient,subject) VALUES ('email','ceo',$1)`, subject)
	}
	return len(list), nil
}

// runBridgeWatch checks bridge sessions every 5 minutes.
func (a *App) runBridgeWatch(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := a.watchBridgeSessions(ctx); err != nil {
				a.Log.Warn("bridge watch failed", "err", err)
			}
		}
	}
}

// handleMetrics reports operational metrics: jobs, LLM usage/cost, action queue, WA sessions.
func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if !p.Human() || p.Role != domain.RoleCEO {
		writeErr(w, http.StatusForbidden, "hanya CEO")
		return
	}
	ctx := r.Context()
	out := map[string]any{}

	jobs := []map[string]any{}
	if rows, err := a.DB.Pool.Query(ctx, `SELECT DISTINCT ON (job) job, started_at, finished_at, ok,
			(SELECT count(*) FROM jobs_runs j2 WHERE j2.job=j.job AND j2.ok=false AND j2.started_at > now() - interval '24 hours')
		FROM jobs_runs j ORDER BY job, id DESC`); err == nil {
		for rows.Next() {
			var name string
			var started time.Time
			var finished *time.Time
			var ok *bool
			var fails24 int
			if rows.Scan(&name, &started, &finished, &ok, &fails24) == nil {
				jobs = append(jobs, map[string]any{"job": name, "last_started": started, "last_finished": finished, "ok": ok, "failures_24h": fails24})
			}
		}
		rows.Close()
	}
	out["jobs"] = jobs

	var calls, tokIn, tokOut int64
	var cost float64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(tokens_in),0), COALESCE(sum(tokens_out),0), COALESCE(sum(cost_est),0)::float8
		FROM llm_calls WHERE created_at > now() - interval '24 hours'`).Scan(&calls, &tokIn, &tokOut, &cost)
	out["llm_24h"] = map[string]any{"calls": calls, "tokens_in": tokIn, "tokens_out": tokOut, "cost_usd": cost, "cost_idr": cost * 16300, "alert_idr": a.Cfg.DailyCostAlertIDR}

	queue := map[string]int{}
	if rows, err := a.DB.Pool.Query(ctx, `SELECT status, count(*) FROM actions GROUP BY status`); err == nil {
		for rows.Next() {
			var s string
			var n int
			if rows.Scan(&s, &n) == nil {
				queue[s] = n
			}
		}
		rows.Close()
	}
	out["actions"] = queue

	sessions := map[string]string{}
	if rows, err := a.DB.Pool.Query(ctx, `SELECT id, status FROM wa_sessions ORDER BY id`); err == nil {
		for rows.Next() {
			var id, s string
			if rows.Scan(&id, &s) == nil {
				sessions[id] = s
			}
		}
		rows.Close()
	}
	out["wa_sessions"] = sessions
	bctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if h, err := a.Bridge.Health(bctx); err == nil {
		out["bridge"] = h
	} else {
		out["bridge"] = map[string]any{"ok": false, "error": err.Error()}
	}
	writeJSON(w, http.StatusOK, out)
}
