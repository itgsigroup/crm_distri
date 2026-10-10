package worker

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/analyst"
	"distri-arc/internal/clock"
	"distri-arc/internal/jobs"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/store"
)

// runTimeout bounds one scheduled analysis (model turns + tool calls).
const runTimeout = 15 * time.Minute

// AnalystTickWorker starts due scheduled analyses (analyst.tick, every minute).
type AnalystTickWorker struct {
	river.WorkerDefaults[jobs.AnalystTickArgs]
	st    *store.Store
	clock clock.Clock
}

// Work queues one analyst.run per due slot; a run left "running" past the timeout is marked as failed.
func (w *AnalystTickWorker) Work(ctx context.Context, _ *river.Job[jobs.AnalystTickArgs]) error {
	now := w.clock.Now()
	_ = w.st.Q.FailStaleMCPScheduleRuns(ctx, now.Add(-runTimeout-time.Minute))
	slots, err := analyst.Due(ctx, w.st.Q, now)
	if err != nil {
		return err
	}
	c, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return err
	}
	for _, s := range slots {
		at := s.At
		if _, err := c.Insert(ctx, jobs.AnalystRunArgs{ScheduleID: s.ScheduleID.String(), Slot: &at, By: "jadwal"},
			&river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByArgs: true}}); err != nil {
			return err
		}
	}
	return nil
}

// AnalystRunWorker runs one scheduled analysis (analyst.run).
type AnalystRunWorker struct {
	river.WorkerDefaults[jobs.AnalystRunArgs]
	runner *analyst.Runner
}

// Work runs; a failure is recorded on the run and not retried (every attempt costs model tokens).
func (w *AnalystRunWorker) Work(ctx context.Context, job *river.Job[jobs.AnalystRunArgs]) error {
	id, err := uuid.Parse(job.Args.ScheduleID)
	if err != nil {
		return river.JobCancel(err)
	}
	trigger := "manual"
	if job.Args.Slot != nil {
		trigger = "schedule"
	}
	if _, err := w.runner.Run(ctx, id, job.Args.Slot, trigger, job.Args.By); err != nil {
		return river.JobCancel(err)
	}
	return nil
}

// Timeout gives a run with several model turns room.
func (w *AnalystRunWorker) Timeout(*river.Job[jobs.AnalystRunArgs]) time.Duration { return runTimeout }

// ButtonSchedule is the analyst schedule row the "Analisis ulang" button runs through (never on a cron: disabled).
const ButtonSchedule = "Analisis ulang lewat MCP (tombol)"

// CycleMCPWorker lets the server-side Claude analyse a cycle through the MCP tools: it waits until the cycle has
// published its agents' Input, writes the task into the button's schedule and runs it like "Jalankan sekarang".
type CycleMCPWorker struct {
	river.WorkerDefaults[jobs.CycleMCPArgs]
	runner *analyst.Runner
	wait   time.Duration
}

func (w *CycleMCPWorker) Work(ctx context.Context, job *river.Job[jobs.CycleMCPArgs]) error {
	cid, err := uuid.Parse(job.Args.CycleID)
	if err != nil {
		return river.JobCancel(err)
	}
	st := w.runner.St
	var names []string
	for i := 0; i < 180; i++ { // the cycle publishes its Input at the start of Analisis
		cyc, err := st.Q.GetCycle(ctx, cid)
		if err != nil {
			return river.JobCancel(err)
		}
		if cyc.Status != "queued" && cyc.Status != "running" {
			return nil // finished or failed before Analisis
		}
		rows, err := st.Q.ListCycleInputs(ctx, cid)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			for _, x := range rows {
				names = append(names, x.Agent)
			}
			prompt := orchestrator.MCPPrompt(deref64(cyc.Number), cid.String(), names, w.wait)
			id, err := ensureButtonSchedule(ctx, st, prompt)
			if err != nil {
				return err
			}
			if _, err := w.runner.Run(ctx, id, nil, "manual", job.Args.By); err != nil {
				return river.JobCancel(err)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return river.JobCancel(errors.New("siklus tidak menerbitkan Input dalam 90 detik"))
}

// ensureButtonSchedule keeps one disabled schedule for the button and sets its task to this cycle's prompt.
func ensureButtonSchedule(ctx context.Context, st *store.Store, prompt string) (uuid.UUID, error) {
	var id uuid.UUID
	err := st.Pool.QueryRow(ctx, `update mcp_schedules set prompt = $2, updated_at = now() where name = $1 returning id`, ButtonSchedule, prompt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = st.Pool.QueryRow(ctx, `insert into mcp_schedules (name, prompt, cron, enabled, scopes, max_steps) values ($1, $2, '0 0 1 1 *', false, '{read,analyze,orchestrate}', 30) returning id`,
			ButtonSchedule, prompt).Scan(&id)
	}
	return id, err
}

func deref64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
