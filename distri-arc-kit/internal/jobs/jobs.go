// Package jobs declares the river job arguments shared by the API (which only inserts jobs) and the worker
// (which runs them), plus an insert-only river client for the API process.
package jobs

import (
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// HeartbeatArgs is the periodic proof-of-life job.
type HeartbeatArgs struct{}

func (HeartbeatArgs) Kind() string { return "heartbeat" }

// RecomputeArgs recomputes metrics_current for some dealers (all when empty).
type RecomputeArgs struct {
	DealerIDs []string `json:"dealer_ids"`
}

func (RecomputeArgs) Kind() string { return "metrics.recompute" }

// SnapshotArgs writes the daily metrics snapshot.
type SnapshotArgs struct{}

func (SnapshotArgs) Kind() string { return "metrics.snapshot" }

// OutboxSendArgs delivers one approved outbox row. Unique per outbox id.
type OutboxSendArgs struct {
	OutboxID string `json:"outbox_id"`
}

func (OutboxSendArgs) Kind() string { return "outbox.send" }

// InsertOpts makes outbox.send unique per outbox row.
func (OutboxSendArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}, MaxAttempts: 3}
}

// WAPairArgs starts pairing a sales number (QR) in the worker, which owns the WhatsApp connections.
type WAPairArgs struct {
	WANumber string `json:"wa_number"`
}

func (WAPairArgs) Kind() string { return "wa.pair" }

// Inserter is an insert-only river client (no workers) for the API and ctl processes.
func Inserter(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	return river.NewClient[pgx.Tx](riverpgxv5.New(pool), &river.Config{})
}

// OdooSyncArgs pulls Odoo changes (every 10 minutes; Full from Pengaturan or ctl).
type OdooSyncArgs struct {
	Full bool `json:"full"`
}

func (OdooSyncArgs) Kind() string { return "odoo.sync" }

// CycleRunArgs runs an Orchestrator cycle: a queued one (CycleID, from POST /cycles) or, without CycleID, the
// hourly scheduled cycle (scope all) — unique per hour.
type CycleRunArgs struct {
	CycleID string `json:"cycle_id,omitempty"`
}

func (CycleRunArgs) Kind() string { return "cycle.run" }

// IdentifyArgs identifies an inbound unknown number (AI Prospek): the worker can read its WhatsApp Business profile.
type IdentifyArgs struct {
	WANumber string `json:"wa_number"`
}

func (IdentifyArgs) Kind() string { return "identify.number" }

// InsertOpts: one identification per number per hour.
func (IdentifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: time.Hour}}
}

// PartitionsEnsureArgs creates next months' partitions of signals and chat_messages (daily, on start).
type PartitionsEnsureArgs struct{}

func (PartitionsEnsureArgs) Kind() string { return "partitions.ensure" }

// RetentionPurgeArgs applies policy retention (daily 02.30 WIB).
type RetentionPurgeArgs struct{}

func (RetentionPurgeArgs) Kind() string { return "retention.purge" }

// AlertsCheckArgs evaluates ops alerts (every 5 minutes).
type AlertsCheckArgs struct{}

func (AlertsCheckArgs) Kind() string { return "alerts.check" }

// PilotSnapshotArgs stores the pilot week (Monday 00.45 WIB, for the week that ended).
type PilotSnapshotArgs struct{}

func (PilotSnapshotArgs) Kind() string { return "pilot.snapshot" }

// WAUnpairArgs logs a linked number out (Pengaturan → WhatsApp → Lepas); the worker owns the connections.
type WAUnpairArgs struct {
	WANumber string `json:"wa_number"`
}

func (WAUnpairArgs) Kind() string { return "wa.unpair" }
