package worker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/analyst"
	"distri-arc/internal/clock"
	"distri-arc/internal/jobs"
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
