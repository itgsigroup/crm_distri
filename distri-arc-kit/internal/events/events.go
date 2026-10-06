// Package events fans Postgres NOTIFY messages out to SSE subscribers (02-architecture › Realtime).
// Writers call Notify (pg_notify) inside or outside a transaction; every API process LISTENs and relays.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Channels relayed to the browser.
var Channels = []string{"cycle_stage", "cycle_done", "proposal_changed", "chat_message", "wa_status", "mcp_call", "policy_changed", "outbox_failed", "data_synced"}

// Event is one server-sent event.
type Event struct {
	Name string          `json:"event"`
	Data json.RawMessage `json:"data"`
}

// Hub keeps the SSE subscribers of one process.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

// Subscribe returns a channel of events and its cancel function.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// Publish delivers an event to every subscriber (dropping it for slow ones).
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribers counts live subscribers.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Listen relays NOTIFY on Channels into the hub until ctx ends, reconnecting on errors.
func Listen(ctx context.Context, pool *pgxpool.Pool, hub *Hub, log *slog.Logger) {
	for ctx.Err() == nil {
		if err := listenOnce(ctx, pool, hub); err != nil && ctx.Err() == nil {
			log.Warn("events listen", "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, hub *Hub) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	for _, c := range Channels {
		if _, err := conn.Exec(ctx, "listen "+c); err != nil {
			return err
		}
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		data := json.RawMessage(n.Payload)
		if !json.Valid(data) {
			data, _ = json.Marshal(n.Payload)
		}
		hub.Publish(Event{Name: n.Channel, Data: data})
	}
}

// Notify sends payload (marshalled to JSON) on channel.
func Notify(ctx context.Context, pool *pgxpool.Pool, channel string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, "select pg_notify($1, $2)", channel, string(b))
	return err
}
