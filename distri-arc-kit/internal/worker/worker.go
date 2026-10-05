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
	"distri-arc/internal/jobs"
	"distri-arc/internal/outbox"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// HeartbeatWorker handles jobs.HeartbeatArgs.
type HeartbeatWorker struct {
	river.WorkerDefaults[jobs.HeartbeatArgs]
	st    *store.Store
	clock clock.Clock
}

// Work writes the heartbeat.
func (w *HeartbeatWorker) Work(ctx context.Context, job *river.Job[jobs.HeartbeatArgs]) error {
	after, _ := json.Marshal(map[string]any{"job_id": job.ID, "at": w.clock.Now()})
	actor, kind, action, entity := "worker", "system", "worker.heartbeat", "worker"
	return w.st.Q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, After: after})
}

// RecomputeWorker handles jobs.RecomputeArgs.
type RecomputeWorker struct {
	river.WorkerDefaults[jobs.RecomputeArgs]
	svc *dealersvc.Service
}

// Work recomputes.
func (w *RecomputeWorker) Work(ctx context.Context, job *river.Job[jobs.RecomputeArgs]) error {
	var ids []uuid.UUID
	for _, s := range job.Args.DealerIDs {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id)
		}
	}
	_, err := w.svc.Recompute(ctx, ids...)
	return err
}

// SnapshotWorker handles jobs.SnapshotArgs.
type SnapshotWorker struct {
	river.WorkerDefaults[jobs.SnapshotArgs]
	svc *dealersvc.Service
}

// Work snapshots.
func (w *SnapshotWorker) Work(ctx context.Context, _ *river.Job[jobs.SnapshotArgs]) error {
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

// OutboxWorker delivers approved outbox rows (outbox.send).
type OutboxWorker struct {
	river.WorkerDefaults[jobs.OutboxSendArgs]
	sender *outbox.Sender
}

// Work sends one row; gaps and the daily cap snooze the job instead of failing it.
func (w *OutboxWorker) Work(ctx context.Context, job *river.Job[jobs.OutboxSendArgs]) error {
	id, err := uuid.Parse(job.Args.OutboxID)
	if err != nil {
		return river.JobCancel(err)
	}
	wait, err := w.sender.Send(ctx, id)
	if wait > 0 {
		return river.JobSnooze(wait)
	}
	return err
}

// WAPairWorker starts QR pairing for a sales number; QR codes flow back as status events.
type WAPairWorker struct {
	river.WorkerDefaults[jobs.WAPairArgs]
	t      wa.Transport
	ingest *wa.Ingestor
}

// Work requests the first QR code.
func (w *WAPairWorker) Work(ctx context.Context, job *river.Job[jobs.WAPairArgs]) error {
	qr, err := w.t.Pair(ctx, job.Args.WANumber)
	if err != nil {
		return err
	}
	if qr != "" {
		return w.ingest.ProcessStatus(ctx, wa.Status{Account: job.Args.WANumber, State: "pairing", QR: qr})
	}
	return nil
}

// Deps are the long-lived connections the worker owns.
type Deps struct {
	Transport wa.Transport
	Ingest    *wa.Ingestor
	Rules     outbox.Rules
}

// New builds the river client with all workers and periodic jobs registered.
func New(st *store.Store, c clock.Clock, log *slog.Logger, deps Deps) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &HeartbeatWorker{st: st, clock: c})
	svc := dealersvc.New(st, c)
	river.AddWorker(workers, &RecomputeWorker{svc: svc})
	river.AddWorker(workers, &SnapshotWorker{svc: svc})
	if deps.Transport != nil {
		river.AddWorker(workers, &OutboxWorker{sender: outbox.NewSender(st, deps.Transport, c, deps.Rules)})
		river.AddWorker(workers, &WAPairWorker{t: deps.Transport, ingest: deps.Ingest})
	}
	return river.NewClient[pgx.Tx](riverpgxv5.New(st.Pool), &river.Config{
		Logger:  log,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 8}},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return jobs.HeartbeatArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(10*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return jobs.RecomputeArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 10 * time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(Daily{Hour: 0, Minute: 30}, func() (river.JobArgs, *river.InsertOpts) {
				return jobs.SnapshotArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
			}, nil),
		},
	})
}
