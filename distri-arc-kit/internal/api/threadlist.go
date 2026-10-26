package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"distri-arc/internal/events"
	"distri-arc/internal/store/gen"
)

// threadList shares each filtered ListThreads query while it runs. A request never joins a query that started
// before the latest chat change, so the list it gets is never older than the event that made it ask; nothing is
// kept once the query is done.
type threadList struct {
	mu    sync.Mutex
	gen   uint64 // bumped by every chat change the hub relays
	calls map[string]*threadCall
}

type threadCall struct {
	gen     uint64
	waiters int // requests sharing it
	done    chan struct{}
	rows    []gen.ListThreadsRow
	err     error
}

// changed is the hub's OnPublish: a chat change makes the next list a fresh query.
func (l *threadList) changed(e events.Event) {
	if e.Name != "chat_message" && e.Name != "proposal_changed" && e.Name != "wa_status" {
		return
	}
	l.bump()
}

// waiting is how many requests share the running query (0 when none runs).
func (l *threadList) waiting() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, c := range l.calls {
		n += c.waiters
	}
	return n
}

// afterWrite: a change made through the API (read, send, contact type, …) also ends sharing, once it is stored —
// the list asked for right after it must show it (a read chat still counted as unread, 15 Oct).
func (l *threadList) afterWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Method != http.MethodGet {
			l.bump()
		}
	})
}

func (l *threadList) bump() {
	l.mu.Lock()
	l.gen++
	l.calls = nil
	l.mu.Unlock()
}

func (l *threadList) get(ctx context.Context, q func(context.Context) ([]gen.ListThreadsRow, error)) ([]gen.ListThreadsRow, error) {
	return l.getKey(ctx, "", q)
}

// getKey shares a query only with requests for the same account/team filter. The full list used
// to be loaded before those filters were applied, making one person's chat page pay for every thread.
func (l *threadList) getKey(ctx context.Context, key string, q func(context.Context) ([]gen.ListThreadsRow, error)) ([]gen.ListThreadsRow, error) {
	l.mu.Lock()
	c := l.calls[key]
	if c == nil || c.gen != l.gen {
		c = &threadCall{gen: l.gen, done: make(chan struct{})}
		if l.calls == nil {
			l.calls = make(map[string]*threadCall)
		}
		l.calls[key] = c
		go func() {
			// not the asker's context: one tab closing must not fail the others waiting on the same query
			qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			c.rows, c.err = q(qctx)
			l.mu.Lock()
			if l.calls[key] == c {
				delete(l.calls, key)
			}
			l.mu.Unlock()
			close(c.done)
		}()
	}
	c.waiters++
	l.mu.Unlock()
	select {
	case <-c.done:
		return c.rows, c.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
