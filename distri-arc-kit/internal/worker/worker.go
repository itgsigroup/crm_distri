// Package worker runs the background jobs on river (ADR 0005).
package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"distri-arc/internal/clock"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// HeartbeatArgs is the periodic proof-of-life job: it writes one audit_log row per minute.
type HeartbeatArgs struct{}

// Kind implements river.JobArgs.
func (HeartbeatArgs) Kind() string { return "heartbeat" }

// HeartbeatWorker handles HeartbeatArgs.
type HeartbeatWorker struct {
	river.WorkerDefaults[HeartbeatArgs]
	st    *store.Store
	clock clock.Clock
}

// Work writes the heartbeat.
func (w *HeartbeatWorker) Work(ctx context.Context, job *river.Job[HeartbeatArgs]) error {
	after, _ := json.Marshal(map[string]any{"job_id": job.ID, "at": w.clock.Now()})
	actor, kind, action, entity := "worker", "system", "worker.heartbeat", "worker"
	return w.st.Q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, After: after})
}

// New builds the river client with all workers and periodic jobs registered.
func New(st *store.Store, c clock.Clock, log *slog.Logger) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &HeartbeatWorker{st: st, clock: c})
	return river.NewClient[pgx.Tx](riverpgxv5.New(st.Pool), &river.Config{
		Logger:  log,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 8}},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return HeartbeatArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
		},
	})
}
