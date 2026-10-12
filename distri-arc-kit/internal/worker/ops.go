package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/riverqueue/river"

	"distri-arc/internal/clock"
	"distri-arc/internal/jobs"
	"distri-arc/internal/ops"
	"distri-arc/internal/pilot"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// PartitionsWorker handles partitions.ensure.
type PartitionsWorker struct {
	river.WorkerDefaults[jobs.PartitionsEnsureArgs]
	st    *store.Store
	clock clock.Clock
}

// Work creates missing monthly partitions.
func (w *PartitionsWorker) Work(ctx context.Context, _ *river.Job[jobs.PartitionsEnsureArgs]) error {
	_, err := ops.EnsurePartitions(ctx, w.st, w.clock.Now())
	return err
}

// RetentionWorker handles retention.purge.
type RetentionWorker struct {
	river.WorkerDefaults[jobs.RetentionPurgeArgs]
	st    *store.Store
	clock clock.Clock
	log   *slog.Logger
}

// Work purges what retention allows and audits the counts.
func (w *RetentionWorker) Work(ctx context.Context, _ *river.Job[jobs.RetentionPurgeArgs]) error {
	r, err := ops.Purge(ctx, w.st, w.clock.Now())
	if err != nil {
		return err
	}
	w.log.Info("retention purge", "chat_messages", r.ChatMessages, "signals", r.Signals, "llm_calls", r.LLMCalls, "partitions_dropped", r.PartitionsDropped)
	after, _ := json.Marshal(r)
	actor, kind, action, entity := "worker", "system", "retention.purge", "retention"
	return w.st.Q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, After: after})
}

// AlertsWorker handles alerts.check: WA disconnected, cycles failing, queue backlog → alerts inside GSI Orbit.
type AlertsWorker struct {
	river.WorkerDefaults[jobs.AlertsCheckArgs]
	st    *store.Store
	clock clock.Clock
	log   *slog.Logger
	env   ops.Env
}

// Work checks health and opens/resolves alerts inside GSI Orbit. Nothing is sent to WhatsApp.
func (w *AlertsWorker) Work(ctx context.Context, _ *river.Job[jobs.AlertsCheckArgs]) error {
	now := w.clock.Now()
	lines, err := ops.Notifier{St: w.st}.Run(ctx, ops.Check(ctx, w.st, w.env, now), now)
	if len(lines) > 0 {
		w.log.Warn("ops alert", "lines", lines)
	}
	return err
}

// PilotWorker handles pilot.snapshot: the week that just ended, while a pilot runs.
type PilotWorker struct {
	river.WorkerDefaults[jobs.PilotSnapshotArgs]
	st    *store.Store
	clock clock.Clock
}

// Work snapshots last week (nothing when the pilot is off).
func (w *PilotWorker) Work(ctx context.Context, _ *river.Job[jobs.PilotSnapshotArgs]) error {
	pol, err := policy.Load(ctx, w.st.Q)
	if err != nil || pol.Pilot.Mode == "off" {
		return err
	}
	_, err = pilot.Service{St: w.st, Clock: w.clock}.Snapshot(ctx, w.clock.Now().AddDate(0, 0, -7))
	return err
}
