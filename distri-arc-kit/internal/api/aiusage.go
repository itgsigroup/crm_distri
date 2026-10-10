package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/analyst"
	"distri-arc/internal/clock"
	"distri-arc/internal/httpx"
	"distri-arc/internal/llm"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
)

// AI usage (MCP Claude page): which model each AI analysis uses, what it cost, how often it runs and when it ran.
// Two kinds of analysis spend tokens: the Orchestrator cycle (hourly, its agents call the routing policy's model)
// and Analisis terjadwal (Claude through the MCP tools on a cron). Both write llm_calls, so the totals cover both.

// AIRun is one analysis in the history.
type AIRun struct {
	Kind       string    `json:"kind"` // cycle | schedule
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	Trigger    string    `json:"trigger"` // schedule | manual | mcp
	By         string    `json:"by"`
	Via        string    `json:"via"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs *int64    `json:"duration_ms"`
	Model      string    `json:"model"`
	Calls      int64     `json:"calls"`
	TokensIn   int64     `json:"tokens_in"`
	TokensOut  int64     `json:"tokens_out"`
	CostIDR    int64     `json:"cost_idr"`
	// TemplateSteps: steps answered without a model (LLM_PROVIDER=fake / failed call) — template text, no cost
	TemplateSteps int64 `json:"template_steps"`
}

func (s *Server) aiUsage(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !can(u, "mcp") && !can(u, "conn") {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Pemakaian AI untuk pemegang menu MCP Claude atau Pengaturan")
		return
	}
	ctx := r.Context()
	now := s.clock.Now()
	today := clock.Today(now)
	pol, _ := policy.Load(ctx, s.st.Q)
	cfg := analyst.LoadConfig(ctx, s.st.Q)
	key, _ := analyst.LoadKey(ctx, s.st.Q, s.secret(), s.cfg.AnthropicKey)
	spent, _ := s.st.Q.AnalystCostSince(ctx, today)

	from, to := 6, 20
	if len(pol.LLM.BatchHours) == 2 {
		from, to = pol.LLM.BatchHours[0], pol.LLM.BatchHours[1]
	}
	orchEngine := "template" // without an API key the agents write from templates (no model, no cost)
	if s.cfg.AnthropicKey != "" && s.cfg.LLMProvider != "" && s.cfg.LLMProvider != "fake" {
		orchEngine = "claude"
	}
	anEngine := "template"
	if key != "" {
		anEngine = "claude"
	}

	fail := func(err error) { httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error()) }
	period := map[string]any{}
	for name, since := range map[string]time.Time{"today": today, "d7": today.AddDate(0, 0, -6), "d30": today.AddDate(0, 0, -29)} {
		v, err := s.st.Q.LLMUsageSince(ctx, since)
		if err != nil {
			fail(err)
			return
		}
		period[name] = v
	}
	byModel, err := s.st.Q.LLMUsageByModel(ctx, today.AddDate(0, 0, -29))
	if err != nil {
		fail(err)
		return
	}
	daily, err := s.st.Q.LLMUsageDaily(ctx, today.AddDate(0, 0, -13))
	if err != nil {
		fail(err)
		return
	}
	cycles, err := s.st.Q.CycleAIUsage(ctx, 200)
	if err != nil {
		fail(err)
		return
	}
	runs, err := s.st.Q.ScheduleAIUsage(ctx, 200)
	if err != nil {
		fail(err)
		return
	}
	history := make([]AIRun, 0, len(cycles)+len(runs))
	for _, c := range cycles {
		title := "Siklus Orchestrator"
		if c.Number != nil {
			title += " #" + strconv.FormatInt(*c.Number, 10)
		}
		if c.Scope != "" && c.Scope != "all" {
			title += " · " + cycleScopeLabel(c.Scope)
		}
		var d *int64
		if c.DurationMs != nil {
			v := int64(*c.DurationMs)
			d = &v
		}
		model := c.Models
		if model == "" {
			model = "template"
		}
		history = append(history, AIRun{Kind: "cycle", ID: c.ID, Title: title, Trigger: c.Trigger, By: c.RequestedBy, Via: c.Via, Status: c.Status,
			StartedAt: c.StartedAt, DurationMs: d, Model: model, Calls: c.Calls, TokensIn: c.TokensIn, TokensOut: c.TokensOut, CostIDR: c.CostIdr,
			TemplateSteps: c.TemplateSteps})
	}
	for _, x := range runs {
		var d *int64
		if x.FinishedAt != nil && !x.FinishedAt.Before(x.StartedAt) { // a frozen demo clock can end a run "before" it began
			v := x.FinishedAt.Sub(x.StartedAt).Milliseconds()
			d = &v
		}
		model := x.Model
		if x.Engine == "template" || model == "" {
			model = "template"
		}
		history = append(history, AIRun{Kind: "schedule", ID: x.ID, Title: "Analisis terjadwal · " + x.Name, Trigger: x.Trigger, By: x.TriggeredBy, Via: "mcp",
			Status: x.Status, StartedAt: x.StartedAt, DurationMs: d, Model: model, Calls: int64(x.Steps), TokensIn: int64(x.TokensIn), TokensOut: int64(x.TokensOut), CostIDR: x.CostIdr})
	}
	// Claude's own sessions through MCP (claude.ai / Desktop / Code): paid by the Claude subscription, no API cost here
	calls, err := s.st.Q.MCPCallsSince(ctx, today.AddDate(0, 0, -29))
	if err != nil {
		fail(err)
		return
	}
	sessions := mcpSessions(calls)
	history = append(history, sessions...)
	sort.Slice(history, func(i, j int) bool { return history[i].StartedAt.After(history[j].StartedAt) })
	mcpToday, mcpLast := int64(0), (*time.Time)(nil)
	for _, c := range calls {
		if !c.CreatedAt.Before(today) {
			mcpToday++
		}
		if mcpLast == nil || c.CreatedAt.After(*mcpLast) {
			t := c.CreatedAt
			mcpLast = &t
		}
	}
	clients, err := s.st.Q.ListMCPClients(ctx, today)
	if err != nil {
		fail(err)
		return
	}
	var conns []map[string]any
	for _, c := range clients {
		if c.Active && deref(c.Kind) != "schedule" {
			conns = append(conns, map[string]any{"name": deref(c.Name), "kind": deref(c.Kind), "user": deref(c.UserName), "last_seen_at": c.LastSeenAt, "calls_today": c.CallsToday})
		}
	}

	prices := map[string]any{}
	for m, p := range llm.Prices {
		prices[m] = map[string]float64{"in_usd_per_mtok": p.In, "out_usd_per_mtok": p.Out}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"orchestrator": map[string]any{"engine": orchEngine, "mode": pol.LLM.Mode, "provider": pol.LLM.Provider, "model": pol.LLM.Model, "fallback": pol.LLM.Fallback,
			"from_hour": from, "to_hour": to, "next_run_at": nextHourly(now, from, to)},
		"mcp":     map[string]any{"connections": nonNil(conns), "calls_today": mcpToday, "sessions_30d": len(sessions), "last_at": mcpLast},
		"analyst": map[string]any{"engine": anEngine, "model": cfg.Model, "daily_budget_idr": cfg.DailyBudgetIDR, "spent_today_idr": spent.Cost, "runs_today": spent.Runs},
		"cost":    map[string]any{"today": period["today"], "d7": period["d7"], "d30": period["d30"], "by_model": nonNil(byModel), "daily": nonNil(daily)},
		"prices":  prices, "idr_per_usd": llm.IDRPerUSD,
		"runs": history,
	})
}

// nextHourly is the next Orchestrator cycle: on the hour between from and to (WIB), as the worker schedules it.
func nextHourly(now time.Time, from, to int) time.Time {
	t := now.In(clock.WIB)
	n := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, clock.WIB).Add(time.Hour)
	switch {
	case n.Hour() < from:
		n = time.Date(n.Year(), n.Month(), n.Day(), from, 0, 0, 0, clock.WIB)
	case n.Hour() > to:
		n = time.Date(n.Year(), n.Month(), n.Day()+1, from, 0, 0, 0, clock.WIB)
	}
	return n
}

func cycleScopeLabel(scope string) string {
	k, v, ok := strings.Cut(scope, ":")
	if !ok {
		return scope
	}
	switch k {
	case "screen":
		return "layar " + v
	case "dealer":
		return "dealer " + v
	case "agent":
		return v
	}
	return scope
}

// mcpSession is the gap that closes a Claude session: calls of one connection closer than this belong together.
const mcpSession = 30 * time.Minute

// mcpSessions groups Claude's MCP tool calls (ordered by connection, then time) into sessions for the history.
func mcpSessions(calls []gen.MCPCallsSinceRow) []AIRun {
	var out []AIRun
	var cur *AIRun
	var last time.Time
	var tools map[string]bool
	var client *uuid.UUID
	flush := func() {
		if cur == nil {
			return
		}
		names := make([]string, 0, len(tools))
		for t := range tools {
			names = append(names, t)
		}
		sort.Strings(names)
		if len(names) > 4 {
			names = append(names[:4], "…")
		}
		cur.Title += " · " + strings.Join(names, ", ")
		out = append(out, *cur)
		cur = nil
	}
	for _, c := range calls {
		same := cur != nil && client != nil && c.ClientID != nil && *client == *c.ClientID && c.CreatedAt.Sub(last) <= mcpSession
		if !same {
			flush()
			id := uuid.Nil
			if c.ClientID != nil {
				id = *c.ClientID
			}
			name := c.ClientName
			if name == "" {
				name = "Claude"
			}
			// the session id: connection id xor'd with its start time keeps rows distinct and stable
			start := c.CreatedAt.UnixNano()
			for i := 0; i < 8; i++ {
				id[15-i] ^= byte(start >> (8 * i))
			}
			cur = &AIRun{Kind: "mcp", ID: id, Title: "Claude lewat MCP · " + name, Trigger: "mcp", By: c.UserName, Via: c.ClientKind, Status: "ok",
				StartedAt: c.CreatedAt, Model: "claude.ai"}
			tools = map[string]bool{}
			client = c.ClientID
		}
		cur.Calls++
		tools[c.Tool] = true
		if c.Status != "ok" && c.Status != "" {
			cur.Status = "partial"
		}
		end := c.CreatedAt
		if c.DurationMs != nil {
			end = end.Add(time.Duration(*c.DurationMs) * time.Millisecond)
		}
		d := end.Sub(cur.StartedAt).Milliseconds()
		cur.DurationMs = &d
		last = c.CreatedAt
	}
	flush()
	return out
}
