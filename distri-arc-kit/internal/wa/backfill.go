package wa

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// HistoryAnchor is the oldest message kept of a chat: the phone is asked for the messages before it.
type HistoryAnchor struct {
	ID     string
	FromMe bool
	At     time.Time
}

// HistoryFetcher is implemented by transports that can ask the linked phone for older messages of one chat (an
// on-demand history sync, ADR 0028). The messages arrive later as ordinary history events through Events().
type HistoryFetcher interface {
	FetchHistory(ctx context.Context, account, chatJID string, before HistoryAnchor, count int) error
}

// ErrHistoryUnavailable: the number cannot be asked right now (not connected, throttled); try again next run.
var ErrHistoryUnavailable = errors.New("riwayat belum bisa diminta")

const (
	// HistoryBatch is how many messages one request asks for (WhatsApp Web asks 50 when scrolling up).
	HistoryBatch = 50
	// historyWait is how long a request has to come back before the chat is looked at again.
	historyWait = 3 * time.Minute
	// historyMaxTries: requests in a row that brought nothing older before the chat counts as complete.
	historyMaxTries = 3
	// historyPerRun caps the requests of one run (the bridge also spaces them per number).
	historyPerRun = 20
)

// BackfillReport counts one wa.backfill run.
type BackfillReport struct {
	Requested int `json:"requested"`
	Done      int `json:"done"`
	Skipped   int `json:"skipped"`
}

// Backfill asks the linked phones for older messages until every conversation reaches `days` back (a year) or the
// phone has nothing older. Idempotent: the messages are deduplicated on their WhatsApp id when they arrive, and a
// chat whose oldest message did not move after historyMaxTries requests is marked done.
func Backfill(ctx context.Context, st *store.Store, f HistoryFetcher, now time.Time, days int, log *slog.Logger) (BackfillReport, error) {
	var r BackfillReport
	if days <= 0 {
		return r, nil
	}
	if log == nil {
		log = slog.Default()
	}
	rows, err := st.Q.HistoryBackfillCandidates(ctx, gen.HistoryBackfillCandidatesParams{RetryBefore: now.Add(-historyWait), Lim: historyPerRun})
	if err != nil {
		return r, err
	}
	cutoff := now.AddDate(0, 0, -days)
	for _, c := range rows {
		if !c.OldestAt.After(cutoff) {
			if err := st.Q.SetHistoryDone(ctx, gen.SetHistoryDoneParams{At: now, ID: c.ID}); err != nil {
				return r, err
			}
			r.Done++
			continue
		}
		tries := int32(0)
		if c.HistoryReqOldest != nil && !c.OldestAt.Before(*c.HistoryReqOldest) {
			tries = c.HistoryTries + 1 // the last request brought nothing older
		}
		if tries >= historyMaxTries {
			if err := st.Q.SetHistoryDone(ctx, gen.SetHistoryDoneParams{At: now, ID: c.ID}); err != nil {
				return r, err
			}
			r.Done++
			continue
		}
		anchor := HistoryAnchor{ID: deref(c.OldestWaID), FromMe: deref(c.OldestDirection) == "out", At: c.OldestAt}
		if err := f.FetchHistory(ctx, deref(c.Account), deref(c.WaJid), anchor, HistoryBatch); err != nil {
			r.Skipped++
			if !errors.Is(err, ErrHistoryUnavailable) {
				log.Warn("wa backfill request", "thread", c.ID, "err", err)
			}
			continue
		}
		if err := st.Q.SetHistoryRequested(ctx, gen.SetHistoryRequestedParams{Oldest: c.OldestAt, At: now, Tries: tries, ID: c.ID}); err != nil {
			return r, err
		}
		r.Requested++
	}
	return r, nil
}
