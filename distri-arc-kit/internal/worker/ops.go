package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// AlertsWorker handles alerts.check: WA disconnected, cycles failing, queue backlog → internal group.
type AlertsWorker struct {
	river.WorkerDefaults[jobs.AlertsCheckArgs]
	st    *store.Store
	clock clock.Clock
	log   *slog.Logger
	env   ops.Env
	from  string
	group string
}

// Work checks health and notifies the internal group through the system outbox.
func (w *AlertsWorker) Work(ctx context.Context, _ *river.Job[jobs.AlertsCheckArgs]) error {
	now := w.clock.Now()
	h := ops.Check(ctx, w.st, w.env, now)
	from := w.from
	for _, n := range h.WA {
		if from == "" && n.State == "connected" {
			from = n.Number
		}
	}
	client := river.ClientFromContext[pgx.Tx](ctx)
	n := ops.Notifier{St: w.st, From: from, GroupJID: w.group, Enqueue: func(ctx context.Context, id uuid.UUID) error {
		_, err := client.Insert(ctx, jobs.OutboxSendArgs{OutboxID: id.String()}, nil)
		return err
	}}
	lines, err := n.Run(ctx, h, now)
	if errors.Is(err, ops.ErrNoAlertGroup) {
		w.log.Warn("ops alert not sent: no internal WhatsApp group", "alerts", lines)
		return nil
	}
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
