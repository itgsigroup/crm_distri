package analyst

import (
	"context"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/cron"
	"distri-arc/internal/store/gen"
)

// MaxCatchUp: a slot missed while the worker was down runs once if it is at most this old; older ones are skipped.
const MaxCatchUp = 3 * time.Hour

// Slot is one scheduled run to start.
type Slot struct {
	ScheduleID uuid.UUID
	At         time.Time
}

// Due moves every due schedule to its next time and returns the slots that should run now. A schedule without
// next_run_at (new, or its cron changed) only gets its next time. The move is a compare-and-set, so two ticks
// never fire the same slot.
func Due(ctx context.Context, q *gen.Queries, now time.Time) ([]Slot, error) {
	rows, err := q.DueMCPSchedules(ctx, &now)
	if err != nil {
		return nil, err
	}
	var out []Slot
	for _, s := range rows {
		spec, err := cron.Parse(s.Cron)
		if err != nil {
			continue // the API validates cron; a bad row waits for an edit
		}
		next := spec.Next(now)
		var nextPtr *time.Time
		if !next.IsZero() {
			nextPtr = &next
		}
		n, err := q.AdvanceMCPSchedule(ctx, gen.AdvanceMCPScheduleParams{Next: nextPtr, ID: s.ID, Prev: s.NextRunAt})
		if err != nil {
			return out, err
		}
		if n == 0 || s.NextRunAt == nil || now.Sub(*s.NextRunAt) > MaxCatchUp {
			continue
		}
		out = append(out, Slot{ScheduleID: s.ID, At: *s.NextRunAt})
	}
	return out, nil
}

// NextRun is the first run of expr after now (nil when the expression is invalid or never runs).
func NextRun(expr string, now time.Time) *time.Time {
	s, err := cron.Parse(expr)
	if err != nil {
		return nil
	}
	if t := s.Next(now); !t.IsZero() {
		return &t
	}
	return nil
}
