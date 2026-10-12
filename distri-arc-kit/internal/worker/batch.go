package worker

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DealerBatcher gathers the dealers a burst of messages touched and flushes them once, after wait: a freshly linked
// number delivers thousands of old messages in minutes, and one recompute job per message starved the database
// (the bridge's forwards timed out, 12 Oct). The periodic full recompute (every 10 minutes) covers a flush lost to a
// restart.
type DealerBatcher struct {
	wait  time.Duration
	flush func(ctx context.Context, ids []uuid.UUID)

	mu      sync.Mutex
	pending map[uuid.UUID]struct{}
	timer   *time.Timer
}

// NewDealerBatcher flushes the collected dealers wait after the first one arrived.
func NewDealerBatcher(wait time.Duration, flush func(ctx context.Context, ids []uuid.UUID)) *DealerBatcher {
	return &DealerBatcher{wait: wait, flush: flush, pending: map[uuid.UUID]struct{}{}}
}

// Add queues one dealer for the next flush (the same dealer many times counts once).
func (b *DealerBatcher) Add(id uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending[id] = struct{}{}
	if b.timer == nil {
		b.timer = time.AfterFunc(b.wait, b.run)
	}
}

func (b *DealerBatcher) run() {
	b.mu.Lock()
	ids := make([]uuid.UUID, 0, len(b.pending))
	for id := range b.pending {
		ids = append(ids, id)
	}
	b.pending, b.timer = map[uuid.UUID]struct{}{}, nil
	b.mu.Unlock()
	if len(ids) > 0 {
		b.flush(context.Background(), ids)
	}
}
