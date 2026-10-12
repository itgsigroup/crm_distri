package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/worker"
)

// A burst of messages for three dealers (one of them 500 times) flushes as ONE batch of three; the next burst, later,
// as another.
func TestDealerBatcherCoalesces(t *testing.T) {
	var mu sync.Mutex
	var batches [][]uuid.UUID
	b := worker.NewDealerBatcher(30*time.Millisecond, func(_ context.Context, ids []uuid.UUID) {
		mu.Lock()
		batches = append(batches, ids)
		mu.Unlock()
	})
	a, c, d := uuid.New(), uuid.New(), uuid.New()
	for i := 0; i < 500; i++ {
		b.Add(a)
	}
	b.Add(c)
	b.Add(d)
	time.Sleep(120 * time.Millisecond)
	b.Add(a)
	time.Sleep(120 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 2 || len(batches[0]) != 3 || len(batches[1]) != 1 || batches[1][0] != a {
		t.Fatalf("batches %v", batches)
	}
}
