package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/analyst"
	"distri-arc/internal/clock"
	"distri-arc/internal/events"
	"distri-arc/internal/jobs"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/store"
	"distri-arc/internal/wa"
)

// API as the second option for analysis (ADR 0040): Claude through MCP (claude.ai, paid by the subscription) answers
// first; when it has not answered an Orchestrator cycle within MCP_WAIT, or the claude.ai routine of Analisa
// WhatsApp is late, the server runs the chosen API (Claude API or ChatGPT API) through the same MCP tools.

// FallbackWindow is the extra time a cycle's Analisis waits for the API after the MCP wait.
const FallbackWindow = 10 * time.Minute

// waLate: Analisa WhatsApp without a successful run for this long is late (the label in Pusat kendali turns red).
const waLate = 150 * time.Minute

// Schedule rows the API second option runs through (disabled: never on a cron; the run history shows them).
const (
	FallbackSchedule   = analyst.FallbackScheduleCycle
	WAFallbackSchedule = analyst.FallbackScheduleWA
	fallbackBy         = "API cadangan"
)

// Fallback decides whether the API second option may run.
type Fallback struct {
	St        *store.Store
	Clock     clock.Clock
	Secret    []byte
	EnvClaude string
	EnvOpenAI string
}

// Ready reports whether the second option is on, its key is present and today's budget is not spent.
func (f *Fallback) Ready(ctx context.Context) bool {
	ok, _, _ := analyst.FallbackReady(ctx, f.St.Q, f.Clock.Now(), f.Secret, f.EnvClaude, f.EnvOpenAI)
	return ok
}

// Hook is orchestrator.OnMCPWait: when the second option is ready, a job runs the API on the cycle once the MCP
// wait is over, and Analisis waits FallbackWindow longer for it.
func (f *Fallback) Hook(client *river.Client[pgx.Tx], log *slog.Logger) func(ctx context.Context, cycleID uuid.UUID, number int64, agents []string, wait time.Duration) time.Duration {
	return func(ctx context.Context, cycleID uuid.UUID, number int64, agents []string, wait time.Duration) time.Duration {
		// Claude routine on claude.ai first (ADR 0049): Claude runs in the cloud on the subscription — nobody opens
		// Claude Desktop, no API key, no ChatGPT
		if extra, ok := f.fireRoutine(ctx, log, cycleID, number, agents, wait); ok {
			return extra
		}
		if client == nil || !f.Ready(ctx) {
			return 0
		}
		// a cycle asked from Analisis ulang names its MCP client (Claude or ChatGPT): no API takes it over (19 Oct)
		if c, err := f.St.Q.GetCycle(ctx, cycleID); err == nil && c.Trigger == "manual" && c.EngineModel != nil && *c.EngineModel != "" {
			return 0
		}
		if _, err := client.Insert(ctx, jobs.CycleMCPArgs{CycleID: cycleID.String(), By: fallbackBy, Fallback: true},
			&river.InsertOpts{ScheduledAt: time.Now().Add(wait), MaxAttempts: 1}); err != nil {
			log.Warn("api fallback: queue", "cycle", cycleID, "err", err)
			return 0
		}
		return FallbackWindow
	}
}

// RoutineWindow is the extra time a cycle's Analisis waits for the Claude routine: a cloud session starts, then Claude
// reads every agent's input and submits.
const RoutineWindow = 20 * time.Minute

// fireRoutine fires the Claude routine with the cycle's MCP prompt; ok when it started (Analisis then waits
// RoutineWindow longer). A routine that cannot be fired leaves the cycle to Claude through MCP as before.
func (f *Fallback) fireRoutine(ctx context.Context, log *slog.Logger, cycleID uuid.UUID, number int64, agents []string, wait time.Duration) (time.Duration, bool) {
	id, token := analyst.LoadRoutine(ctx, f.St.Q, f.Secret)
	if id == "" {
		return 0, false
	}
	var names []string
	for _, a := range agents {
		if !orchestrator.IsScreenReading(a) && a != orchestrator.AgentWA {
			names = append(names, a)
		}
	}
	prompt := orchestrator.MCPPrompt(number, cycleID.String(), names, wait+RoutineWindow)
	url, err := analyst.FireRoutine(ctx, id, token, prompt)
	if err != nil {
		log.Warn("claude routine: fire", "cycle", cycleID, "err", err)
		_ = events.Notify(ctx, f.St.Pool, "routine_fired", map[string]any{"cycle_id": cycleID, "ok": false, "error": err.Error()})
		return 0, false
	}
	log.Info("claude routine: fired", "cycle", cycleID, "run", url)
	_ = events.Notify(ctx, f.St.Pool, "routine_fired", map[string]any{"cycle_id": cycleID, "ok": true, "url": url})
	return RoutineWindow, true
}

// fallbackCycle runs the API on the agents of a still-waiting cycle that Claude through MCP did not answer.
func (w *CycleMCPWorker) fallbackCycle(ctx context.Context, cid uuid.UUID) error {
	st := w.runner.St
	cyc, err := st.Q.GetCycle(ctx, cid)
	if err != nil {
		return river.JobCancel(err)
	}
	if cyc.Status != "running" {
		return nil
	}
	rows, err := st.Q.ListCycleInputs(ctx, cid)
	if err != nil {
		return err
	}
	var missing []string
	for _, x := range rows {
		if x.SubmittedAt == nil && !orchestrator.IsScreenReading(x.Agent) {
			missing = append(missing, x.Agent)
		}
	}
	if len(missing) == 0 {
		return nil // Claude through MCP answered every agent in time
	}
	prompt := orchestrator.MCPPrompt(deref64(cyc.Number), cid.String(), missing, FallbackWindow)
	id, err := ensureSchedule(ctx, st, FallbackSchedule, prompt)
	if err != nil {
		return err
	}
	if _, err := w.runner.Run(ctx, id, nil, "manual", fallbackBy); err != nil {
		return river.JobCancel(err)
	}
	return nil
}

// WAFallbackWorker writes Analisa WhatsApp with the API when the claude.ai routine is late (every 30 minutes).
type WAFallbackWorker struct {
	river.WorkerDefaults[jobs.WAFallbackArgs]
	runner *analyst.Runner
	fb     *Fallback
}

// Work runs the routine's task once when no Analisa WhatsApp succeeded for waLate and the API did not try in the
// last 2 hours (a failing key or prompt costs at most one run per routine period).
func (w *WAFallbackWorker) Work(ctx context.Context, _ *river.Job[jobs.WAFallbackArgs]) error {
	// Analisa WhatsApp otomatis on: it has its own hours and API, and a failed run is not taken over (ADR 0043)
	if !w.fb.Ready(ctx) || analyst.WAAutoOn(ctx, w.runner.St.Q) {
		return nil
	}
	st := w.runner.St
	now := w.runner.Clock.Now()
	k := analyst.KeyWAAnalysis
	sch, err := st.Q.GetMCPScheduleByKey(ctx, &k)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !sch.Enabled) {
		return nil
	}
	if err != nil {
		return err
	}
	if at, err := st.Q.LastOKMCPScheduleRun(ctx, sch.ID); err == nil && at != nil && now.Sub(*at) < waLate {
		return nil
	}
	var last *time.Time
	if err := st.Pool.QueryRow(ctx, `select last_run_at from mcp_schedules where name = $1`, WAFallbackSchedule).Scan(&last); err == nil && last != nil && now.Sub(*last) < 2*time.Hour {
		return nil
	}
	id, err := ensureSchedule(ctx, st, WAFallbackSchedule, sch.Prompt)
	if err != nil {
		return err
	}
	if _, err := w.runner.Run(ctx, id, nil, "manual", fallbackBy); err != nil {
		return river.JobCancel(err)
	}
	return nil
}

// Timeout: the API second option on a cycle is a multi-turn tool loop over several agents (river's default of one
// minute would cut it off).
func (w *CycleMCPWorker) Timeout(*river.Job[jobs.CycleMCPArgs]) time.Duration { return runTimeout }

// Timeout gives the run room (as a scheduled analysis).
func (w *WAFallbackWorker) Timeout(*river.Job[jobs.WAFallbackArgs]) time.Duration { return runTimeout }

// GroupNamesWorker names the groups whose first message came without a name (the bridge could not read it then).
type GroupNamesWorker struct {
	river.WorkerDefaults[jobs.GroupNamesArgs]
	ingest *wa.Ingestor
	src    wa.GroupNames
	log    *slog.Logger
}

// Work asks each number holding such a group for its group list once.
func (w *GroupNamesWorker) Work(ctx context.Context, _ *river.Job[jobs.GroupNamesArgs]) error {
	n, err := w.ingest.NameGroups(ctx, w.src)
	if n > 0 {
		w.log.Info("wa groups named", "groups", n)
	}
	return err
}
