// Package clock provides the application's notion of "now". In development ARC_NOW pins the start time to the
// sample data's anchor (Senin, 5 Oktober 2026) while still ticking, so metrics match the approved mockup.
package clock

import (
	"fmt"
	"time"
)

// WIB is the business time zone; all day boundaries (jadwal order, jatuh tempo) are computed in WIB.
var WIB = time.FixedZone("WIB", 7*3600)

// Clock returns the current time.
type Clock interface{ Now() time.Time }

type system struct{ offset time.Duration }

func (s system) Now() time.Time { return time.Now().Add(s.offset).In(WIB) }

// New returns the system clock, shifted so that it starts at pinned (RFC 3339) when pinned is non-empty.
func New(pinned string) (Clock, error) {
	if pinned == "" {
		return system{}, nil
	}
	t, err := time.Parse(time.RFC3339, pinned)
	if err != nil {
		return nil, fmt.Errorf("ARC_NOW: %w", err)
	}
	return system{offset: time.Until(t)}, nil
}

// Fixed is a clock that never moves (tests).
type Fixed time.Time

func (f Fixed) Now() time.Time { return time.Time(f).In(WIB) }

// Today returns midnight WIB of t.
func Today(t time.Time) time.Time {
	t = t.In(WIB)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, WIB)
}
