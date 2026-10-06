package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"distri-arc/internal/clock"
	"distri-arc/internal/events"
	"distri-arc/internal/importer"
	"distri-arc/internal/jobs"
	"distri-arc/internal/store"
)

// dataLock serialises syncs and applies (one transform at a time).
const dataLock = 7_420_001

func withDataLock(ctx context.Context, st *store.Store, fn func() error) error {
	conn, err := st.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var ok bool
	if err := conn.QueryRow(ctx, "select pg_try_advisory_lock($1)", dataLock).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return river.JobSnooze(time.Minute) // another sync/apply is running
	}
	defer func() { _, _ = conn.Exec(context.Background(), "select pg_advisory_unlock($1)", dataLock) }()
	return fn()
}

// DataSyncWorker pulls BigQuery when data.source.mode is bigquery and the interval has passed (or Force).
type DataSyncWorker struct {
	river.WorkerDefaults[jobs.DataSyncArgs]
	st     *store.Store
	clock  clock.Clock
	log    *slog.Logger
	secret []byte
}

// Work runs one sync.
func (w *DataSyncWorker) Work(ctx context.Context, job *river.Job[jobs.DataSyncArgs]) error {
	im := importer.Importer{St: w.st, Clock: w.clock, Log: w.log}
	cfg, err := im.LoadConfig(ctx)
	if err != nil || cfg.Mode != "bigquery" {
		return err
	}
	if !job.Args.Force {
		every := time.Duration(max(cfg.BigQuery.SyncMinutes, 15)) * time.Minute
		if last, err := w.st.Q.LastImportRun(ctx, "bigquery"); err == nil && time.Since(last.StartedAt) < every {
			return nil
		}
	}
	by := job.Args.By
	if by == "" {
		by = "jadwal"
	}
	return withDataLock(ctx, w.st, func() error {
		bq, err := im.BigQueryClient(ctx, cfg, w.secret)
		if errors.Is(err, importer.ErrNoCredentials) {
			return nil
		}
		if err != nil {
			return err
		}
		rep, err := im.SyncBigQuery(ctx, bq, cfg, job.Args.Full, by)
		if err != nil {
			w.log.Error("bigquery sync", "err", err)
		} else {
			w.log.Info("bigquery sync", "dealers", rep.Apply.Dealers, "invoices", rep.Apply.Invoices, "stock", rep.Apply.Stock)
		}
		_ = events.Notify(ctx, w.st.Pool, "data_synced", map[string]any{"ok": err == nil})
		return err
	})
}

// DataApplyWorker re-runs the transform from staging (CSV import, mapping change).
type DataApplyWorker struct {
	river.WorkerDefaults[jobs.DataApplyArgs]
	st    *store.Store
	clock clock.Clock
	log   *slog.Logger
}

// Work applies staged rows.
func (w *DataApplyWorker) Work(ctx context.Context, job *river.Job[jobs.DataApplyArgs]) error {
	return withDataLock(ctx, w.st, func() error {
		rep, err := importer.Importer{St: w.st, Clock: w.clock, Log: w.log}.Apply(ctx, job.Args.Full, job.Args.By)
		if err == nil {
			w.log.Info("data apply", "dealers", rep.Dealers, "invoices", rep.Invoices, "stock", rep.Stock, "s", rep.DurationS)
		}
		_ = events.Notify(ctx, w.st.Pool, "data_synced", map[string]any{"ok": err == nil})
		return err
	})
}
