// Package worker runs the background jobs on river (ADR 0005).
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/identify"
	"distri-arc/internal/jobs"
	"distri-arc/internal/odoo"
	"distri-arc/internal/ops"
	"distri-arc/internal/orchestrator"
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

// Weekly runs once a week at a WIB weekday and time.
type Weekly struct {
	Weekday      time.Weekday
	Hour, Minute int
}

// Next implements river.PeriodicSchedule.
func (w Weekly) Next(t time.Time) time.Time {
	n := Daily{Hour: w.Hour, Minute: w.Minute}.Next(t)
	for n.Weekday() != w.Weekday {
		n = n.AddDate(0, 0, 1)
	}
	return n
}

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
	if errors.Is(err, outbox.ErrRefused) {
		return river.JobCancel(err) // final: the guard refused this message, retrying cannot change that
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

// WAUnpairWorker logs a linked number out; a team number is removed, a sales' main number stays unpaired.
type WAUnpairWorker struct {
	river.WorkerDefaults[jobs.WAUnpairArgs]
	t  wa.Transport
	st *store.Store
}

// Work unlinks the device (when the transport can) and records the state.
func (w *WAUnpairWorker) Work(ctx context.Context, job *river.Job[jobs.WAUnpairArgs]) error {
	n := job.Args.WANumber
	if u, ok := w.t.(wa.Unpairer); ok {
		if err := u.Unpair(ctx, n); err != nil {
			return err
		}
	}
	if err := w.st.Q.SetWANumberUnpaired(ctx, n); err != nil {
		return err
	}
	if err := w.st.Q.DeleteWANumber(ctx, n); err != nil {
		return err
	}
	return events.Notify(ctx, w.st.Pool, "wa_status", map[string]string{"account": n, "state": "unpaired"})
}

// OdooSyncWorker runs the read-only Odoo sync and recomputes the dealers it touched.
type OdooSyncWorker struct {
	river.WorkerDefaults[jobs.OdooSyncArgs]
	syncer *odoo.Syncer
	svc    *dealersvc.Service
}

// Work syncs.
func (w *OdooSyncWorker) Work(ctx context.Context, job *river.Job[jobs.OdooSyncArgs]) error {
	rep, err := w.syncer.Run(ctx, job.Args.Full)
	if err != nil {
		return err
	}
	if len(rep.Dealers) > 0 {
		_, err = w.svc.Recompute(ctx, rep.Dealers...)
	}
	return err
}

// Timeout allows a full sync to take a while.
func (w *OdooSyncWorker) Timeout(*river.Job[jobs.OdooSyncArgs]) time.Duration {
	return 10 * time.Minute
}

// CycleWorker runs Orchestrator cycles (cycle.run).
type CycleWorker struct {
	river.WorkerDefaults[jobs.CycleRunArgs]
	o *orchestrator.Orchestrator
}

// Work executes a queued cycle, or queues and runs the scheduled one; a running cycle makes the scheduled run a no-op.
func (w *CycleWorker) Work(ctx context.Context, job *river.Job[jobs.CycleRunArgs]) error {
	if job.Args.CycleID != "" {
		id, err := uuid.Parse(job.Args.CycleID)
		if err != nil {
			return river.JobCancel(err)
		}
		_, err = w.o.Execute(ctx, id)
		if errors.Is(err, orchestrator.ErrRunning) {
			return river.JobSnooze(15 * time.Second)
		}
		if err != nil {
			return river.JobCancel(err) // the cycle is recorded as failed; retrying would run it twice
		}
		return nil
	}
	_, err := w.o.Run(ctx, domain.Scope{Kind: "all"}, domain.Trigger{Source: "schedule", By: "scheduler", Via: "api"})
	if errors.Is(err, orchestrator.ErrRunning) {
		return nil
	}
	if err != nil {
		return river.JobCancel(err)
	}
	return nil
}

// Timeout gives a full cycle with a real LLM room.
func (w *CycleWorker) Timeout(*river.Job[jobs.CycleRunArgs]) time.Duration { return 15 * time.Minute }

// Hourly fires on the hour between From and To (WIB), e.g. 06.00–20.00 (policy llm.routing.batch_hours).
type Hourly struct{ From, To int }

// Next implements river.PeriodicSchedule.
func (h Hourly) Next(t time.Time) time.Time {
	t = t.In(clock.WIB)
	n := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, clock.WIB).Add(time.Hour)
	switch {
	case n.Hour() < h.From:
		n = time.Date(n.Year(), n.Month(), n.Day(), h.From, 0, 0, 0, clock.WIB)
	case n.Hour() > h.To:
		n = time.Date(n.Year(), n.Month(), n.Day()+1, h.From, 0, 0, 0, clock.WIB)
	}
	return n
}

// IdentifyWorker runs identify.number with the transport's profile reader.
type IdentifyWorker struct {
	river.WorkerDefaults[jobs.IdentifyArgs]
	svc *identify.Service
}

// Work identifies; numbers that did not write first are cancelled (privacy), not retried.
func (w *IdentifyWorker) Work(ctx context.Context, job *river.Job[jobs.IdentifyArgs]) error {
	_, err := w.svc.Identify(ctx, job.Args.WANumber)
	if errors.Is(err, identify.ErrNotInbound) {
		return river.JobCancel(err)
	}
	return err
}

// Deps are the long-lived connections the worker owns.
type Deps struct {
	Transport    wa.Transport
	Ingest       *wa.Ingestor
	Rules        outbox.Rules
	Odoo         odoo.Source
	Orchestrator *orchestrator.Orchestrator
	Identify     *identify.Service
	Ops          ops.Env
	AlertFrom    string // ALERT_WA_FROM
	SessionKey   []byte // opens sealed secrets (BigQuery key)
	AlertGroup   string // ALERT_WA_GROUP
}

// New builds the river client with all workers and periodic jobs registered.
func New(st *store.Store, c clock.Clock, log *slog.Logger, deps Deps) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &HeartbeatWorker{st: st, clock: c})
	svc := dealersvc.New(st, c)
	river.AddWorker(workers, &RecomputeWorker{svc: svc})
	river.AddWorker(workers, &SnapshotWorker{svc: svc})
	river.AddWorker(workers, &PartitionsWorker{st: st, clock: c})
	river.AddWorker(workers, &RetentionWorker{st: st, clock: c, log: log})
	river.AddWorker(workers, &PilotWorker{st: st, clock: c})
	river.AddWorker(workers, &DataSyncWorker{st: st, clock: c, log: log, secret: deps.SessionKey})
	river.AddWorker(workers, &DataApplyWorker{st: st, clock: c, log: log})
	river.AddWorker(workers, &AlertsWorker{st: st, clock: c, log: log, env: deps.Ops, from: deps.AlertFrom, group: deps.AlertGroup})
	var periodic []*river.PeriodicJob
	if deps.Odoo != nil {
		river.AddWorker(workers, &OdooSyncWorker{syncer: odoo.NewSyncer(st, deps.Odoo, c, log), svc: svc})
		periodic = append(periodic, river.NewPeriodicJob(river.PeriodicInterval(10*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
			return jobs.OdooSyncArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 10 * time.Minute}}
		}, nil))
	}
	if deps.Identify != nil {
		river.AddWorker(workers, &IdentifyWorker{svc: deps.Identify})
	}
	if deps.Orchestrator != nil {
		river.AddWorker(workers, &CycleWorker{o: deps.Orchestrator})
		periodic = append(periodic, river.NewPeriodicJob(Hourly{From: 6, To: 20}, func() (river.JobArgs, *river.InsertOpts) {
			return jobs.CycleRunArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Hour}}
		}, nil))
	}
	if deps.Transport != nil {
		river.AddWorker(workers, &OutboxWorker{sender: outbox.NewSender(st, deps.Transport, c, deps.Rules).WithOdoo(deps.Odoo)})
		river.AddWorker(workers, &WAPairWorker{t: deps.Transport, ingest: deps.Ingest})
		river.AddWorker(workers, &WAUnpairWorker{t: deps.Transport, st: st})
	}
	return river.NewClient[pgx.Tx](riverpgxv5.New(st.Pool), &river.Config{
		Logger:  log,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 8}},
		Workers: workers,
		PeriodicJobs: append(periodic, []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return jobs.HeartbeatArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(10*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return jobs.RecomputeArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 10 * time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(Daily{Hour: 0, Minute: 30}, func() (river.JobArgs, *river.InsertOpts) {
				return jobs.SnapshotArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
			}, nil),
			river.NewPeriodicJob(Daily{Hour: 0, Minute: 10}, func() (river.JobArgs, *river.InsertOpts) {
				return jobs.PartitionsEnsureArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Hour}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(Weekly{Weekday: time.Monday, Hour: 0, Minute: 45}, func() (river.JobArgs, *river.InsertOpts) {
				return jobs.PilotSnapshotArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
			}, nil),
			river.NewPeriodicJob(Daily{Hour: 2, Minute: 30}, func() (river.JobArgs, *river.InsertOpts) {
				return jobs.RetentionPurgeArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
			}, nil),
			river.NewPeriodicJob(river.PeriodicInterval(15*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return jobs.DataSyncArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 15 * time.Minute}}
			}, nil),
			river.NewPeriodicJob(river.PeriodicInterval(5*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
				return jobs.AlertsCheckArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 5 * time.Minute}}
			}, &river.PeriodicJobOpts{RunOnStart: true}),
		}...),
	})
}
