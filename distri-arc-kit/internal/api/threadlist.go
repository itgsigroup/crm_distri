package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"distri-arc/internal/events"
	"distri-arc/internal/store/gen"
)

// threadList shares one ListThreads query among the chat lists asked for while it runs. The query costs ~1.7 s
// (3,900 threads, each with its waiting-since and last message); every open Chat tab refetches the list on each
// new message, and during a history sync those refetches piled up into 10–18 s responses on a 2-CPU server
// (15 Oct). A request never joins a query that started before the latest chat change, so the list it gets is
// never older than the event that made it ask; nothing is kept once the query is done.
type threadList struct {
	mu   sync.Mutex
	gen  uint64 // bumped by every chat change the hub relays
	call *threadCall
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
	if l.call == nil {
		return 0
	}
	return l.call.waiters
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
	l.mu.Unlock()
}

func (l *threadList) get(ctx context.Context, q func(context.Context) ([]gen.ListThreadsRow, error)) ([]gen.ListThreadsRow, error) {
	l.mu.Lock()
	c := l.call
	if c == nil || c.gen != l.gen {
		c = &threadCall{gen: l.gen, done: make(chan struct{})}
		l.call = c
		go func() {
			// not the asker's context: one tab closing must not fail the others waiting on the same query
			qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			c.rows, c.err = q(qctx)
			l.mu.Lock()
			if l.call == c {
				l.call = nil
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
