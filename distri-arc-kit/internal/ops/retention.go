// Package ops keeps the installation healthy: monthly partitions, retention (UU PDP), subject rights (export and
// delete), health checks, Prometheus metrics and ops alerts to the internal WhatsApp group.
package ops

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Partitioned are the tables partitioned by month and their partition column.
var Partitioned = [][2]string{{"signals", "occurred_at"}, {"chat_messages", "sent_at"}}

// MonthsAhead is how many future months always have a partition.
const MonthsAhead = 3

// EnsurePartitions creates the partitions of this month through MonthsAhead (job partitions.ensure, daily).
func EnsurePartitions(ctx context.Context, st *store.Store, now time.Time) (int, error) {
	n := 0
	for _, t := range Partitioned {
		c, err := st.Q.EnsurePartitions(ctx, gen.EnsurePartitionsParams{Tbl: t[0], Col: t[1], FromMonth: now, NowAt: now, MonthsAhead: MonthsAhead})
		if err != nil {
			return n, err
		}
		n += int(c)
	}
	return n, nil
}

// PurgeReport counts what retention removed.
type PurgeReport struct {
	ChatMessages      int64 `json:"chat_messages"`
	Signals           int64 `json:"signals"`
	LLMCalls          int64 `json:"llm_calls"`
	Identifications   int64 `json:"identifications"`
	PartitionsDropped int   `json:"partitions_dropped"`
}

// Purge applies policy retention: chat messages after chat_days, signals after signals_months, llm_calls after
// llm_calls_days, identifications of numbers that never became contacts after 90 days. Dealers, transactions,
// metrics and snapshots are never touched. Whole months are dropped as partitions; the rest is deleted.
func Purge(ctx context.Context, st *store.Store, now time.Time) (PurgeReport, error) {
	var r PurgeReport
	pol, err := policy.Load(ctx, st.Q)
	if err != nil {
		return r, err
	}
	ret := pol.Retention
	if ret.ChatDays <= 0 {
		ret.ChatDays = 90
	}
	if ret.SignalsMonths <= 0 {
		ret.SignalsMonths = 24
	}
	if ret.LLMCallsDays <= 0 {
		ret.LLMCallsDays = 180
	}
	err = st.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		chatCut := now.AddDate(0, 0, -ret.ChatDays)
		n, err := q.PurgeChatMessages(ctx, chatCut)
		if err != nil {
			return err
		}
		r.ChatMessages = n
		d, err := q.DropPartitionsBefore(ctx, gen.DropPartitionsBeforeParams{Tbl: "chat_messages", Cutoff: chatCut})
		if err != nil {
			return err
		}
		r.PartitionsDropped += int(d)
		if _, err := q.PurgeChatMessageKeys(ctx, chatCut); err != nil {
			return err
		}
		sigCut := now.AddDate(0, -ret.SignalsMonths, 0)
		if r.Signals, err = q.PurgeSignals(ctx, sigCut); err != nil {
			return err
		}
		if d, err = q.DropPartitionsBefore(ctx, gen.DropPartitionsBeforeParams{Tbl: "signals", Cutoff: sigCut}); err != nil {
			return err
		}
		r.PartitionsDropped += int(d)
		if _, err := q.PurgeSignalKeys(ctx, sigCut); err != nil {
			return err
		}
		if r.LLMCalls, err = q.PurgeLLMCalls(ctx, now.AddDate(0, 0, -ret.LLMCallsDays)); err != nil {
			return err
		}
		r.Identifications, err = q.PurgeIdentifications(ctx, now.AddDate(0, 0, -90))
		return err
	})
	return r, err
}
