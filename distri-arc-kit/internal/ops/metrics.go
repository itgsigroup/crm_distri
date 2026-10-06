package ops

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// buckets are the request duration histogram bounds in seconds (p95 target 300 ms).
var buckets = []float64{0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1, 2.5, 5}

type histogram struct {
	counts []uint64 // per bucket, cumulative at render time
	sum    float64
	n      uint64
}

// HTTPMetrics records request durations per route pattern (Prometheus text format, no client library).
type HTTPMetrics struct {
	mu sync.Mutex
	h  map[string]*histogram // "GET /api/orbit"
}

// NewHTTPMetrics returns an empty recorder.
func NewHTTPMetrics() *HTTPMetrics { return &HTTPMetrics{h: map[string]*histogram{}} }

// Middleware measures every request under its chi route pattern (unknown routes share one series).
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/events") {
			return // SSE stays open; its duration says nothing
		}
		route := "other"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		m.Observe(r.Method+" "+route, time.Since(start))
	})
}

// Observe adds one duration.
func (m *HTTPMetrics) Observe(key string, d time.Duration) {
	s := d.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.h[key]
	if h == nil {
		h = &histogram{counts: make([]uint64, len(buckets))}
		m.h[key] = h
	}
	for i, b := range buckets {
		if s <= b {
			h.counts[i]++
			break
		}
	}
	h.sum += s
	h.n++
}

// Write renders the request histograms and the health gauges in Prometheus text format.
func (m *HTTPMetrics) Write(w io.Writer, h Health) {
	pf := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) } // a client gone mid-scrape is not an error
	g := func(name, help string, v any, labels ...string) {
		pf("# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
		l := ""
		if len(labels) > 0 {
			l = "{" + strings.Join(labels, ",") + "}"
		}
		pf("%s%s %v\n", name, l, v)
	}
	up := 0
	if h.DB == "ok" {
		up = 1
	}
	g("arc_up", "Database reachable (1) or not (0)", up)
	g("arc_queue_depth", "River jobs waiting (available, scheduled, retryable)", h.QueueDepth)
	g("arc_signals", "Stored signals", h.Counts.Signals)
	g("arc_chat_messages", "Stored chat messages", h.Counts.ChatMessages)
	g("arc_open_proposals", "Proposals waiting for a decision", h.Counts.OpenProposals)
	g("arc_cycles_failed_streak", "Failed Orchestrator cycles in a row", h.failedStreak)
	g("arc_alerts_open", "Open ops alerts", len(h.Alerts))
	pf("# HELP arc_wa_connected WhatsApp number connected (1) or not (0)\n# TYPE arc_wa_connected gauge\n")
	for _, n := range h.WA {
		v := 0
		if n.State == "connected" {
			v = 1
		}
		pf("arc_wa_connected{number=%q,transport=%q} %d\n", n.Number, n.Transport, v)
	}
	pf("# HELP arc_outbox Outbox rows by status\n# TYPE arc_outbox gauge\n")
	statuses := make([]string, 0, len(h.Outbox))
	for s := range h.Outbox {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		pf("arc_outbox{status=%q} %d\n", s, h.Outbox[s])
	}
	if v, ok := h.LLM["cost_today_idr"]; ok {
		g("arc_llm_cost_today_idr", "Estimated LLM cost today (IDR)", v)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.h))
	for k := range m.h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pf("# HELP arc_http_request_duration_seconds API request duration by route\n# TYPE arc_http_request_duration_seconds histogram\n")
	for _, k := range keys {
		x := m.h[k]
		method, route, _ := strings.Cut(k, " ")
		var cum uint64
		for i, b := range buckets {
			cum += x.counts[i]
			pf("arc_http_request_duration_seconds_bucket{method=%q,route=%q,le=\"%g\"} %d\n", method, route, b, cum)
		}
		pf("arc_http_request_duration_seconds_bucket{method=%q,route=%q,le=\"+Inf\"} %d\n", method, route, x.n)
		pf("arc_http_request_duration_seconds_sum{method=%q,route=%q} %g\n", method, route, x.sum)
		pf("arc_http_request_duration_seconds_count{method=%q,route=%q} %d\n", method, route, x.n)
	}
}
