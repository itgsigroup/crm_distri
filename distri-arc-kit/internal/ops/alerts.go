package ops

import (
	"context"
	"fmt"
	"time"

	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Alert is one open condition.
type Alert struct {
	Key     string
	Message string
}

// Alerts turns a health check into the open conditions shown in GSI Orbit: a WhatsApp number disconnected, two
// failed cycles in a row, a queue deeper than 500. They are never sent to WhatsApp.
func Alerts(h Health) []Alert {
	var out []Alert
	for _, w := range h.waDown {
		who := w.Sales
		if who == "" {
			who = w.Number
		}
		why := w.State
		if why == "pairing" || why == "unpaired" {
			why = "perangkat tertaut dihapus dari HP"
		}
		out = append(out, Alert{Key: "wa_disconnected:" + w.Number, Message: fmt.Sprintf("WhatsApp %s (%s) terputus — chat tidak sinkron; pindai QR ulang di Pengaturan → WhatsApp", who, why)})
	}
	if h.failedStreak >= CycleFailAlert {
		out = append(out, Alert{Key: "cycle_failed", Message: fmt.Sprintf("Siklus Orchestrator gagal %d× berturut-turut — lihat log worker (RUNBOOK §3)", h.failedStreak)})
	}
	if h.QueueDepth > QueueAlertDepth {
		out = append(out, Alert{Key: "queue_depth", Message: fmt.Sprintf("Antrean job %d (> %d) — worker tertinggal (RUNBOOK §4)", h.QueueDepth, QueueAlertDepth)})
	}
	return out
}

// Notifier opens and resolves alerts (system_alerts). It never sends anything.
type Notifier struct {
	St *store.Store
}

// Run opens new alerts, resolves the ones that cleared and returns the lines (new and recovered) for the log.
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
	// Alerts stay in GSI Orbit (Status sistem, the WhatsApp banner): nothing is ever sent to WhatsApp on its own —
	// Sam, 12 Oct: "JANGAN KIRIM PESAN OTOMATIS". The lines are returned for the log only.
	return lines, nil
}
