package ops

import (
	"context"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Env is what the health check needs to know about the process configuration.
type Env struct {
	Version     string
	WATransport string
	OdooMode    string
	OdooWrite   bool
	LLMProvider string
	LLMModel    string
	LLMKey      bool
}

// EnvFrom reads the process configuration.
func EnvFrom(cfg config.Config) Env {
	return Env{Version: config.Version, WATransport: cfg.WATransport, OdooMode: cfg.OdooMode, OdooWrite: cfg.OdooWrite,
		LLMProvider: cfg.LLMProvider, LLMModel: cfg.LLMModel, LLMKey: cfg.AnthropicKey != "" || cfg.OpenAIKey != ""}
}

// QueueAlertDepth is the queue depth that raises an alert (10-testing-ops).
const QueueAlertDepth = 500

// CycleFailAlert is how many failed cycles in a row raise an alert.
const CycleFailAlert = 2

// WAState is one paired number.
type WAState struct {
	Number    string     `json:"number"`
	Sales     string     `json:"sales"`
	State     string     `json:"state"`
	Transport string     `json:"transport"`
	LastSeen  *time.Time `json:"last_seen_at"`
}

// OdooModel is one synced Odoo model.
type OdooModel struct {
	Model   string     `json:"model"`
	LastRun *time.Time `json:"last_run_at"`
	Records int32      `json:"records"`
	Error   string     `json:"error,omitempty"`
}

// Health is GET /api/health (and the source of /metrics and alerts).
type Health struct {
	Status     string              `json:"status"` // ok | degraded | down
	Now        time.Time           `json:"now"`
	Version    string              `json:"version"`
	DB         string              `json:"db"`
	Queue      string              `json:"queue"`
	QueueDepth int64               `json:"queue_depth"`
	SampleData bool                `json:"sample_data"`
	WA         []WAState           `json:"wa"`
	Odoo       map[string]any      `json:"odoo"`
	LLM        map[string]any      `json:"llm"`
	Cycles     map[string]any      `json:"cycles"`
	Outbox     map[string]int64    `json:"outbox"`
	Counts     gen.HealthCountsRow `json:"counts"`
	Alerts     []gen.SystemAlert   `json:"alerts"`
	Problems   []string            `json:"problems"`

	failedStreak int
	waDown       []WAState
}

// Check gathers the health of every dependency. It never fails: a broken part is reported in the result.
func Check(ctx context.Context, st *store.Store, env Env, now time.Time) Health {
	h := Health{Status: "ok", Now: now, Version: env.Version, DB: "ok", Queue: "ok", Outbox: map[string]int64{}, Problems: []string{}}
	if _, err := st.Q.Ping(ctx); err != nil {
		h.DB, h.Status = "error", "down"
		h.Problems = append(h.Problems, "Database tidak terjangkau")
		return h
	}
	_ = st.Pool.QueryRow(ctx, "select exists(select 1 from signal_keys where dedupe_key like 'seed:%')").Scan(&h.SampleData)
	if d, err := st.Q.QueueDepth(ctx); err != nil {
		h.Queue = "error"
	} else {
		h.QueueDepth = d
		if d > QueueAlertDepth {
			h.Queue = "backlog"
			h.Problems = append(h.Problems, "Antrean job menumpuk")
		}
	}
	if rows, err := st.Q.HealthWANumbers(ctx); err == nil {
		for _, r := range rows {
			w := WAState{Number: r.WaNumber, Sales: deref(r.SalesName), State: r.State, Transport: r.Transport, LastSeen: r.LastSeenAt}
			h.WA = append(h.WA, w)
			if r.State == "disconnected" || r.State == "logged_out" {
				h.waDown = append(h.waDown, w)
			}
		}
	}
	if len(h.waDown) > 0 {
		h.Problems = append(h.Problems, "WhatsApp terputus")
	}
	odooStatus := "ok"
	var models []OdooModel
	if rows, err := st.Q.HealthOdoo(ctx); err == nil {
		for _, r := range rows {
			m := OdooModel{Model: r.Model, LastRun: r.LastRunAt, Records: r.Records, Error: deref(r.Error)}
			if m.Error != "" {
				odooStatus = "error"
			}
			models = append(models, m)
		}
	}
	if env.OdooMode == "off" {
		odooStatus = "off"
	}
	if odooStatus == "error" {
		h.Problems = append(h.Problems, "Sinkron Odoo gagal")
	}
	h.Odoo = map[string]any{"mode": env.OdooMode, "write": env.OdooWrite, "status": odooStatus, "models": models}
	llmStatus := "ok"
	if env.LLMProvider == "fake" || !env.LLMKey {
		llmStatus = "template"
	}
	llm := map[string]any{"provider": env.LLMProvider, "model": env.LLMModel, "status": llmStatus}
	if t, err := st.Q.HealthLLMToday(ctx, clock.Today(now)); err == nil {
		llm["calls_today"], llm["cost_today_idr"] = t.Calls, t.CostIdr
		if t.Calls > 0 {
			llm["last_at"] = t.LastAt
		}
	}
	h.LLM = llm
	if cs, err := st.Q.HealthCycles(ctx); err == nil {
		for _, c := range cs {
			if c.Status != "failed" {
				break
			}
			h.failedStreak++
		}
		c := map[string]any{"failed_streak": h.failedStreak}
		if len(cs) > 0 {
			c["last"] = cs[0]
		}
		h.Cycles = c
	}
	if h.failedStreak >= CycleFailAlert {
		h.Problems = append(h.Problems, "Siklus Orchestrator gagal berturut-turut")
	}
	if rows, err := st.Q.HealthOutbox(ctx); err == nil {
		for _, r := range rows {
			h.Outbox[r.Status] = r.N
		}
	}
	h.Counts, _ = st.Q.HealthCounts(ctx)
	if a, err := st.Q.ListAlerts(ctx); err == nil {
		for _, x := range a {
			if x.ResolvedAt == nil {
				h.Alerts = append(h.Alerts, x)
			}
		}
	}
	if len(h.Problems) > 0 {
		h.Status = "degraded"
	}
	return h
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
