package domain

import (
	"sync"
	"time"
)

// The ARC clock is real time by default. When ARC_CLOCK_ANCHOR is set (demo
// mode), "now" starts at the anchor and advances with the wall clock, so the
// seeded sample data keeps the dates of the approved mockup.
var (
	clockMu     sync.RWMutex
	clockAnchor time.Time
	clockStart  time.Time
)

// SetClockAnchor switches to demo mode. A zero time restores real time.
func SetClockAnchor(anchor time.Time) {
	clockMu.Lock()
	defer clockMu.Unlock()
	clockAnchor = anchor
	clockStart = time.Now()
}

// Now returns the current ARC time.
func Now() time.Time {
	clockMu.RLock()
	defer clockMu.RUnlock()
	if clockAnchor.IsZero() {
		return time.Now().In(Jakarta)
	}
	return clockAnchor.Add(time.Since(clockStart)).In(Jakarta)
}

// ClockIsDemo reports whether the demo anchor is active.
func ClockIsDemo() bool {
	clockMu.RLock()
	defer clockMu.RUnlock()
	return !clockAnchor.IsZero()
}

// StartOfDay returns local midnight of t.
func StartOfDay(t time.Time) time.Time {
	t = t.In(Jakarta)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Jakarta)
}
