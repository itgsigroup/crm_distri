package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"distri-arc/internal/events"
	"distri-arc/internal/store/gen"
)

// Chat lists asked for while a query runs share it; a chat change in between starts a fresh one, and a tab that
// closes does not fail the others.
func TestThreadListSharesOnlyCurrentQuery(t *testing.T) {
	var l threadList
	var calls atomic.Int32
	release := make(chan struct{})
	q := func(ctx context.Context) ([]gen.ListThreadsRow, error) {
		n := calls.Add(1)
		<-release
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return make([]gen.ListThreadsRow, n), nil
	}
	var wg sync.WaitGroup
	got := make([]int, 3)
	cancelled, cancel := context.WithCancel(context.Background())
	for i := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			if i == 0 {
				ctx = cancelled
			}
			rows, err := l.get(ctx, q)
			if err == nil {
				got[i] = len(rows)
			}
		}()
	}
	for l.waiting() < 3 || calls.Load() < 1 {
		runtime.Gosched()
	}
	cancel() // the first tab closes while the query runs
	// a new message arrives: the next list must not reuse the query that started before it
	l.changed(events.Event{Name: "chat_message"})
	var fresh int
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, _ := l.get(context.Background(), q)
		fresh = len(rows)
	}()
	for calls.Load() < 2 {
		runtime.Gosched()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 2 || got[1] != 1 || got[2] != 1 || fresh != 2 {
		t.Fatalf("calls %d, shared %v, after change %d", calls.Load(), got, fresh)
	}
	// unrelated events do not end sharing; nothing is kept after a query finishes
	l.changed(events.Event{Name: "mcp_call"})
	if rows, _ := l.get(context.Background(), func(context.Context) ([]gen.ListThreadsRow, error) { calls.Add(1); return nil, nil }); rows != nil || calls.Load() != 3 {
		t.Fatalf("finished query was reused: calls %d", calls.Load())
	}
}

// A change made through the API (here: reading a chat) ends sharing once it is stored, like a relayed chat event.
func TestThreadListEndsSharingAfterWrite(t *testing.T) {
	var l threadList
	release := make(chan struct{})
	var calls atomic.Int32
	q := func(context.Context) ([]gen.ListThreadsRow, error) {
		n := calls.Add(1)
		if n == 1 {
			<-release
		}
		return make([]gen.ListThreadsRow, n), nil
	}
	done := make(chan int)
	go func() { rows, _ := l.get(context.Background(), q); done <- len(rows) }()
	for l.waiting() < 1 || calls.Load() < 1 {
		runtime.Gosched()
	}
	h := l.afterWrite(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/chat/threads", nil))
	if rows, _ := func() ([]gen.ListThreadsRow, error) {
		ch := make(chan []gen.ListThreadsRow)
		go func() { r, _ := l.get(context.Background(), q); ch <- r }()
		for l.waiting() < 2 {
			runtime.Gosched()
		}
		close(release)
		return <-ch, nil
	}(); len(rows) != 1 {
		t.Fatalf("a GET must keep sharing: got the result of call %d", len(rows))
	}
	<-done
	release = make(chan struct{})
	calls.Store(0)
	go func() { rows, _ := l.get(context.Background(), q); done <- len(rows) }()
	for l.waiting() < 1 || calls.Load() < 1 {
		runtime.Gosched()
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/chat/threads/x/read", nil))
	if rows, _ := l.get(context.Background(), q); len(rows) != 2 {
		t.Fatalf("after a read the list reused the older query (%d)", len(rows))
	}
	close(release)
	<-done
}
