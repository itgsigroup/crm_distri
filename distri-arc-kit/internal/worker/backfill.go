package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"distri-arc/internal/clock"
	"distri-arc/internal/jobs"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/wa"
)

// WABackfillWorker handles wa.backfill: older chat history from the linked phones, up to WA_BACKFILL_DAYS but never
// past retention.chat_days (what would be purged the next night is not fetched).
type WABackfillWorker struct {
	river.WorkerDefaults[jobs.WABackfillArgs]
	st    *store.Store
	clock clock.Clock
	log   *slog.Logger
	f     wa.HistoryFetcher
	days  int
}

// Timeout: a round spaces requests per number (wa.HistoryGap) and takes about a minute — river's default.
func (w *WABackfillWorker) Timeout(*river.Job[jobs.WABackfillArgs]) time.Duration {
	return 5 * time.Minute
}

// Work sends one round of history requests.
func (w *WABackfillWorker) Work(ctx context.Context, _ *river.Job[jobs.WABackfillArgs]) error {
	days := w.days
	if pol, err := policy.Load(ctx, w.st.Q); err == nil && pol.Retention.ChatDays > 0 && pol.Retention.ChatDays < days {
		days = pol.Retention.ChatDays
	}
	r, err := wa.Backfill(ctx, w.st, w.f, w.clock.Now(), days, w.log)
	if r.Requested+r.Done > 0 {
		w.log.Info("wa backfill", "requested", r.Requested, "done", r.Done, "skipped", r.Skipped, "days", days)
	}
	return err
}
