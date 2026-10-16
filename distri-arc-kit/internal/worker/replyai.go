package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"distri-arc/internal/jobs"
	"distri-arc/internal/replyai"
)

// ReplyAIWorker handles reply.ai: an AI writes the AI Trigger drafts of a new customer message (ADR 0041).
type ReplyAIWorker struct {
	river.WorkerDefaults[jobs.ReplyAIArgs]
	svc *replyai.Service
	log *slog.Logger
}

// Timeout: a draft that takes longer is not worth waiting for (the card keeps the template draft).
func (w *ReplyAIWorker) Timeout(*river.Job[jobs.ReplyAIArgs]) time.Duration { return 2 * time.Minute }

// Work writes the drafts; no API key is not an error (the template draft stays).
func (w *ReplyAIWorker) Work(ctx context.Context, job *river.Job[jobs.ReplyAIArgs]) error {
	id, err := uuid.Parse(job.Args.MessageID)
	if err != nil {
		return river.JobCancel(err)
	}
	n, err := w.svc.Write(ctx, id, nil)
	if errors.Is(err, replyai.ErrNoModel) {
		return nil
	}
	if err != nil {
		return err
	}
	w.log.Info("reply ai", "message", id, "drafts", n)
	return nil
}
