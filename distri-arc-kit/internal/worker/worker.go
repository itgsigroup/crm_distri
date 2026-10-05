// Package worker runs the background jobs on river (ADR 0005).
package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
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

// RecomputeArgs recomputes metrics_current for some dealers (all when empty). Enqueued on new signals and
// every 10 minutes as a safety net.
type RecomputeArgs struct {
	DealerIDs []string `json:"dealer_ids"`
}

// Kind implements river.JobArgs.
func (RecomputeArgs) Kind() string { return "metrics.recompute" }

// RecomputeWorker handles RecomputeArgs.
type RecomputeWorker struct {
	river.WorkerDefaults[RecomputeArgs]
	svc *dealersvc.Service
}

// Work recomputes.
func (w *RecomputeWorker) Work(ctx context.Context, job *river.Job[RecomputeArgs]) error {
	var ids []uuid.UUID
	for _, s := range job.Args.DealerIDs {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id)
		}
	}
	_, err := w.svc.Recompute(ctx, ids...)
	return err
}

// SnapshotArgs writes the daily metrics snapshot (00:30 WIB).
type SnapshotArgs struct{}

// Kind implements river.JobArgs.
func (SnapshotArgs) Kind() string { return "metrics.snapshot" }

// SnapshotWorker handles SnapshotArgs.
type SnapshotWorker struct {
	river.WorkerDefaults[SnapshotArgs]
	svc *dealersvc.Service
}

// Work snapshots.
func (w *SnapshotWorker) Work(ctx context.Context, _ *river.Job[SnapshotArgs]) error {
	_, err := w.svc.Snapshot(ctx)
	return err
}

// Daily is a periodic schedule firing once a day at hour:minute WIB.
type Daily struct{ Hour, Minute int }

// Next implements river.PeriodicSchedule.
func (d Daily) Next(t time.Time) time.Time {
	t = t.In(clock.WIB)
	n := time.Date(t.Year(), t.Month(), t.Day(), d.Hour, d.Minute, 0, 0, clock.WIB)
	if !n.After(t) {
		n = n.AddDate(0, 0, 1)
	}
	return n
}

// New builds the river client with all workers and periodic jobs registered.
func New(st *store.Store, c clock.Clock, log *slog.Logger) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &HeartbeatWorker{st: st, clock: c})
	svc := dealersvc.New(st, c)
	river.AddWorker(workers, &RecomputeWorker{svc: svc})
	river.AddWorker(workers, &SnapshotWorker{svc: svc})
	return river.NewClient[pgx.Tx](riverpgxv5.New(st.Pool), &river.Config{
		Logger:  log,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 8}},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return HeartbeatArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(10*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return RecomputeArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 10 * time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(Daily{Hour: 0, Minute: 30}, func() (river.JobArgs, *river.InsertOpts) {
				return SnapshotArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
			}, nil),
		},
	})
}
