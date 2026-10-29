package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"distri-arc/internal/autoreply"
	"distri-arc/internal/jobs"
)

// AutoReplyWorker handles chat.auto_reply: a customer message during a sales' break gets the fact answer or the
// approved template (ADR 0066).
type AutoReplyWorker struct {
	river.WorkerDefaults[jobs.AutoReplyArgs]
	svc *autoreply.Service
	log *slog.Logger
}

// Timeout: Lyra's product reading and her reply (autoreply.LyraWait each), then the AI draft (45 s per model).
func (w *AutoReplyWorker) Timeout(*river.Job[jobs.AutoReplyArgs]) time.Duration {
	return 2*autoreply.LyraWait + 2*time.Minute
}

func (w *AutoReplyWorker) Work(ctx context.Context, job *river.Job[jobs.AutoReplyArgs]) error {
	id, err := uuid.Parse(job.Args.MessageID)
	if err != nil {
		return river.JobCancel(err)
	}
	out, err := w.svc.Handle(ctx, id)
	if err != nil {
		return err
	}
	if out != autoreply.Skipped {
		w.log.Info("auto reply", "message", id, "kind", out)
	}
	return nil
}
