package events_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"distri-arc/internal/events"
	"distri-arc/internal/testdb"
)

// The hub fans an event out to every subscriber and forgets cancelled ones.
func TestHubFanOut(t *testing.T) {
	h := events.NewHub()
	a, cancelA := h.Subscribe()
	b, cancelB := h.Subscribe()
	h.Publish(events.Event{Name: "cycle_stage", Data: json.RawMessage(`{"stage":"ingest"}`)})
	for _, ch := range []<-chan events.Event{a, b} {
		if e := <-ch; e.Name != "cycle_stage" {
			t.Fatalf("got %s", e.Name)
		}
	}
	cancelA()
	if h.Subscribers() != 1 {
		t.Fatalf("%d subscribers", h.Subscribers())
	}
	cancelB()
}

// A slow subscriber never blocks the publisher.
func TestHubDropsForSlowSubscribers(t *testing.T) {
	h := events.NewHub()
	_, cancel := h.Subscribe()
	defer cancel()
	done := make(chan struct{})
	go func() {
		for range 100 {
			h.Publish(events.Event{Name: "cycle_stage"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked")
	}
}

// NOTIFY on a relayed channel reaches the hub (cycle_stage from the Orchestrator → SSE).
func TestListenRelaysNotify(t *testing.T) {
	st := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := events.NewHub()
	ch, unsub := h.Subscribe()
	defer unsub()
	go events.Listen(ctx, st.Pool, h, slog.New(slog.NewTextHandler(io.Discard, nil)))
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case e := <-ch:
			if e.Name == "cycle_done" {
				return
			}
		case <-tick.C:
			_ = events.Notify(ctx, st.Pool, "cycle_done", map[string]any{"number": 1})
		case <-deadline:
			t.Fatal("no event relayed")
		}
	}
}
