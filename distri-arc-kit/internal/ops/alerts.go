package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/outbox"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Alert is one open condition.
type Alert struct {
	Key     string
	Message string
}

// Alerts turns a health check into the conditions that page the internal group: a WhatsApp number disconnected,
// two failed cycles in a row, a queue deeper than 500.
func Alerts(h Health) []Alert {
	var out []Alert
	for _, w := range h.waDown {
		who := w.Sales
		if who == "" {
			who = w.Number
		}
		out = append(out, Alert{Key: "wa_disconnected:" + w.Number, Message: fmt.Sprintf("WhatsApp %s (%s) terputus — pasangkan ulang di Pengaturan → WhatsApp", who, w.State)})
	}
	if h.failedStreak >= CycleFailAlert {
		out = append(out, Alert{Key: "cycle_failed", Message: fmt.Sprintf("Siklus Orchestrator gagal %d× berturut-turut — lihat log worker (RUNBOOK §3)", h.failedStreak)})
	}
	if h.QueueDepth > QueueAlertDepth {
		out = append(out, Alert{Key: "queue_depth", Message: fmt.Sprintf("Antrean job %d (> %d) — worker tertinggal (RUNBOOK §4)", h.QueueDepth, QueueAlertDepth)})
	}
	return out
}

// Notifier writes system outbox rows for the internal alert group.
type Notifier struct {
	St       *store.Store
	From     string // sending number (a connected sales/ops number)
	GroupJID string // ALERT_WA_GROUP; empty = the first internal group
	Enqueue  func(ctx context.Context, outboxID uuid.UUID) error
}

// ErrNoAlertGroup means no internal group is known yet; alerts stay visible in Status sistem only.
var ErrNoAlertGroup = errors.New("belum ada grup WhatsApp internal untuk alert")

// Run opens new alerts (notifying each once), resolves the ones that cleared (with a recovery line) and returns the
// alerts it notified.
func (n Notifier) Run(ctx context.Context, h Health, now time.Time) ([]string, error) {
	active := Alerts(h)
	keys := make([]string, 0, len(active))
	var lines []string
	for _, a := range active {
		keys = append(keys, a.Key)
		row, err := n.St.Q.OpenAlert(ctx, gen.OpenAlertParams{Key: a.Key, Message: a.Message, OpenedAt: now})
		if err != nil {
			return nil, err
		}
		if row.NotifiedAt == nil {
			lines = append(lines, "⚠️ "+a.Message)
			if err := n.St.Q.MarkAlertNotified(ctx, gen.MarkAlertNotifiedParams{Key: a.Key, NotifiedAt: &now}); err != nil {
				return nil, err
			}
		}
	}
	resolved, err := n.St.Q.ResolveAlerts(ctx, gen.ResolveAlertsParams{At: now, Active: keys})
	if err != nil {
		return nil, err
	}
	for _, r := range resolved {
		lines = append(lines, "✅ Pulih: "+r.Message)
	}
	if len(lines) == 0 {
		return nil, nil
	}
	return lines, n.send(ctx, lines)
}

func (n Notifier) send(ctx context.Context, lines []string) error {
	g, err := n.St.Q.InternalAlertGroup(ctx, n.GroupJID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoAlertGroup
	}
	if err != nil {
		return err
	}
	text := "Distri ARC · status sistem\n"
	for _, l := range lines {
		text += "\n" + l
	}
	payload, _ := json.Marshal(outbox.SystemPayload{From: n.From, To: deref(g.Jid), Text: text})
	id, err := n.St.Q.InsertSystemOutbox(ctx, gen.InsertSystemOutboxParams{ToRef: g.Jid, Payload: payload})
	if err != nil {
		return err
	}
	if n.Enqueue != nil {
		return n.Enqueue(ctx, id)
	}
	return nil
}
