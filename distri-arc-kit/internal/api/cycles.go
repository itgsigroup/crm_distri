package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/analyst"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/httpx"
	"distri-arc/internal/jobs"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/policy"
	"distri-arc/internal/proposals"
	"distri-arc/internal/store/gen"
)

func (s *Server) cycleRoutes(r chi.Router) {
	r.Get("/cycles/latest", s.latestCycle)
	r.Get("/cycles", s.listCycles)
	r.Post("/cycles", s.postCycle)
	r.Get("/cycles/{id}", s.getCycle)
	r.Get("/cycles/{id}/inputs", s.cycleInputs)
	r.Get("/conflicts", s.conflicts)
	r.Get("/agents", s.agents)
	r.Post("/agents/{name}/run", s.runAgent)
	r.Get("/plan/today", s.planToday)
	r.Post("/plan/{id}/run", s.runPlanStep)
	r.Get("/policies/autonomy", s.getAutonomy)
	r.Put("/policies/autonomy", s.putAutonomy)
}

// CycleView is a cycle with its stages (the Orchestrator card, dock and pipeline).
type CycleView struct {
	gen.Cycle
	Stages []gen.CycleStage `json:"stages"`
	Label  string           `json:"label"`
}

func (s *Server) cycleView(r *http.Request, c gen.Cycle) CycleView {
	st, _ := s.st.Q.ListCycleStages(r.Context(), c.ID)
	sc, _ := domain.ParseScope(c.Scope)
	return CycleView{Cycle: c, Stages: nonNil(st), Label: sc.Label()}
}

// nextRun is the next hourly slot within 06.00–20.00 WIB.
func nextRun(now time.Time) time.Time {
	t := now.In(clock.WIB)
	n := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, clock.WIB).Add(time.Hour)
	switch {
	case n.Hour() < 6:
		n = time.Date(n.Year(), n.Month(), n.Day(), 6, 0, 0, 0, clock.WIB)
	case n.Hour() > 20:
		n = time.Date(n.Year(), n.Month(), n.Day()+1, 6, 0, 0, 0, clock.WIB)
	}
	return n
}

// latestCycle: the current/last cycle, the last finished one (counters), and the next scheduled run.
func (s *Server) latestCycle(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"cycle": nil, "last_done": nil, "last_full": nil, "running": false, "next_at": nextRun(s.clock.Now())}
	if c, err := s.st.Q.LatestCycle(r.Context()); err == nil {
		out["cycle"] = s.cycleView(r, c)
		out["running"] = c.Status == "running" || c.Status == "queued"
	}
	if c, err := s.st.Q.LatestDoneCycle(r.Context()); err == nil {
		out["last_done"] = s.cycleView(r, c)
	}
	if c, err := s.st.Q.LatestFullCycle(r.Context()); err == nil {
		out["last_full"] = s.cycleView(r, c) // the day's counters (Rencana hari ini, card numbers)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *Server) listCycles(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.st.Q.ListCycles(r.Context(), int32(limit))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	items := make([]CycleView, 0, len(rows))
	for _, c := range rows {
		sc, _ := domain.ParseScope(c.Scope)
		items = append(items, CycleView{Cycle: c, Stages: []gen.CycleStage{}, Label: sc.Label()})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type cycleRequest struct {
	Scope string `json:"scope"`
	Via   string `json:"via"`
}

// postCycle queues a cycle (Analisis ulang) and lets the worker run it: 202, or 409 while one is running.
func (s *Server) postCycle(w http.ResponseWriter, r *http.Request) {
	var req cycleRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.queueCycle(w, r, req.Scope, req.Via)
}

func (s *Server) queueCycle(w http.ResponseWriter, r *http.Request, scope, via string) {
	sc, err := domain.ParseScope(scope)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_scope", err.Error())
		return
	}
	if via != "mcp" {
		via = "api"
	}
	// require_ai without a model on the server: the button's analysis goes to Claude through MCP
	pol, _ := policy.Load(r.Context(), s.st.Q)
	serverModel := s.cfg.LLMProvider == "anthropic" && s.cfg.AnthropicKey != ""
	if pol.LLM.RequireAI && !serverModel {
		via = "mcp"
	}
	anKey, _ := analyst.LoadKey(r.Context(), s.st.Q, s.secret(), s.cfg.AnthropicKey)
	u, _ := CurrentUser(r.Context())
	o := &orchestrator.Orchestrator{St: s.st, Clock: s.clock}
	var cyc gen.Cycle
	err = s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
		c, err := o.Queue(r.Context(), q, sc, domain.Trigger{Source: "manual", By: deref(u.Email), Via: via})
		if err != nil {
			return err
		}
		cyc = c
		if s.jobs != nil {
			_, err = s.jobs.InsertTx(r.Context(), tx, jobs.CycleRunArgs{CycleID: c.ID.String()}, nil)
			if err == nil && via == "mcp" && anKey != "" { // the server's Claude analyses it through the MCP tools
				_, err = s.jobs.InsertTx(r.Context(), tx, jobs.CycleMCPArgs{CycleID: c.ID.String(), By: deref(u.Email)}, nil)
			}
		}
		return err
	})
	switch {
	case errors.Is(err, orchestrator.ErrRunning):
		httpx.Fail(w, http.StatusConflict, "cycle_running", "Orchestrator sedang berjalan")
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
	default:
		out := cycleQueued{CycleView: s.cycleView(r, cyc)}
		if via == "mcp" {
			var names []string
			for _, a := range proposals.All() {
				if n := sc.Agents(); n == nil || slices.Contains(n, a.Name()) {
					names = append(names, a.Name())
				}
			}
			prompt := orchestrator.MCPPrompt(deref64(cyc.Number), cyc.ID.String(), names, s.mcpWait())
			m := &cycleMCP{Engine: "claude_ai", Prompt: prompt, ClaudeURL: "https://claude.ai/new?q=" + url.QueryEscape(prompt), WaitSec: int(s.mcpWait().Seconds()), Agents: names}
			if anKey != "" {
				m.Engine = "claude_api" // Claude on the server (Analisis terjadwal engine) does it; nothing to open
			}
			out.MCP = m
		}
		httpx.JSON(w, http.StatusAccepted, out)
	}
}

// cycleQueued is POST /cycles: the cycle, and how it is analysed when it goes to Claude through MCP.
type cycleQueued struct {
	CycleView
	MCP *cycleMCP `json:"mcp,omitempty"`
}

type cycleMCP struct {
	Engine    string   `json:"engine"` // claude_api: the server's Claude does it · claude_ai: the person opens Claude
	Prompt    string   `json:"prompt"`
	ClaudeURL string   `json:"claude_url"`
	WaitSec   int      `json:"wait_sec"`
	Agents    []string `json:"agents"`
}

// cycleInputs: per agent, whether Claude sent its analysis through MCP yet (the button's progress).
func (s *Server) cycleInputs(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Siklus tidak ditemukan")
		return
	}
	rows, err := s.st.Q.ListCycleInputs(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	type item struct {
		Agent       string     `json:"agent"`
		SubmittedAt *time.Time `json:"submitted_at"`
		SubmittedBy *uuid.UUID `json:"submitted_by"` // the MCP connection that sent it
	}
	out := make([]item, 0, len(rows))
	for _, x := range rows {
		out = append(out, item{Agent: x.Agent, SubmittedAt: x.SubmittedAt, SubmittedBy: x.SubmittedBy})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func deref64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func (s *Server) getCycle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Siklus tidak ditemukan")
		return
	}
	c, err := s.st.Q.GetCycle(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Siklus tidak ditemukan")
		return
	}
	runs, _ := s.st.Q.ListAgentRuns(r.Context(), &id)
	conf, _ := s.st.Q.ListConflicts(r.Context(), gen.ListConflictsParams{CycleID: &id, AllRules: true})
	props, _ := s.st.Q.ProposalsOfCycle(r.Context(), &id)
	httpx.JSON(w, http.StatusOK, map[string]any{"cycle": s.cycleView(r, c), "agent_runs": nonNil(runs), "conflicts": nonNil(conf), "proposals": nonNil(props)})
}

// conflicts of the last full cycle (Resolusi konflik); ?all=1 includes housekeeping rules.
func (s *Server) conflicts(w http.ResponseWriter, r *http.Request) {
	c, err := s.st.Q.LatestFullCycle(r.Context())
	if err != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	rows, err := s.st.Q.ListConflicts(r.Context(), gen.ListConflictsParams{CycleID: &c.ID, AllRules: r.URL.Query().Get("all") == "1"})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	set, scoped, ok := s.scopedDealers(w, r)
	if !ok {
		return
	}
	out := rows[:0]
	for _, x := range rows {
		if inScope(set, scoped, x.DealerID) {
			out = append(out, x)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(out), "cycle_number": c.Number})
}

// agentMeta is the agent card text (mockup AGENTS): role and icon.
var agentMeta = map[string][2]string{
	"AI Order":     {"Mengubah permintaan WA menjadi SO: cek stok, harga tier, sisa limit.", "doc"},
	"AI Follow-up": {"Menghitung siklus order, jadwal order, lewat jadwal; menyusun rekomendasi order.", "refresh"},
	"AI Kredit":    {"Menjaga sisa limit: menahan rilis di atas limit, mengusulkan DP atau kenaikan limit.", "shield"},
	"AI Stok":      {"Mencocokkan stok menua dengan dealer yang product mix-nya cocok dan masuk jadwal order.", "box"},
	"AI Penagihan": {"Mengingatkan invoice dengan nada mengikuti pola bayar; sinkron dengan jadwal order.", "cash"},
	"AI Prospek":   {"Mengenali nomor baru (profil WA, Getcontact manual) dan mengusulkan tier awal.", "people"},
}

// agentOrder is the order of the agent cards and the autonomy matrix (mockup).
var agentOrder = []string{"AI Order", "AI Follow-up", "AI Kredit", "AI Stok", "AI Penagihan", "AI Prospek"}

// AgentView is one agent card.
type AgentView struct {
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	Icon       string     `json:"icon"`
	Auto       string     `json:"auto"`
	Approve    string     `json:"approve"`
	Never      string     `json:"never"`
	Output     string     `json:"output"`
	Confidence *int16     `json:"confidence"`
	LastRunAt  *time.Time `json:"last_run_at"`
	Today      int64      `json:"today"`
}

func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	pol, err := policy.Load(r.Context(), s.st.Q)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	state, _ := s.st.Q.ListAgentState(r.Context())
	byName := map[string]gen.AgentState{}
	for _, a := range state {
		byName[a.Agent] = a
	}
	set, scoped, ok := s.scopedDealers(w, r)
	if !ok {
		return
	}
	today := map[string]int64{}
	if scoped { // one sales' page: today's proposals on their dealers only
		since := clock.Today(s.clock.Now())
		rows, _ := s.st.Q.ListProposals(r.Context(), gen.ListProposalsParams{Lim: 1000})
		for _, p := range rows {
			if !p.CreatedAt.Before(since) && p.Kind != "reply" && inScope(set, true, p.DealerID) {
				today[p.Agent]++
			}
		}
	} else {
		counts, _ := s.st.Q.TodayAgentCounts(r.Context(), clock.Today(s.clock.Now()))
		for _, c := range counts {
			today[c.Agent] += c.N
		}
	}
	out := make([]AgentView, 0, len(agentOrder))
	for _, n := range agentOrder {
		row := pol.Autonomy[n]
		a := AgentView{Name: n, Role: agentMeta[n][0], Icon: agentMeta[n][1], Auto: row.Labels["auto"], Approve: row.Labels["approve"], Never: row.Labels["never"], Today: today[n]}
		if st, ok := byName[n]; ok {
			a.Output, a.Confidence, a.LastRunAt = deref(st.LastOutput), st.Confidence, st.LastRunAt
		}
		out = append(out, a)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "guard": pol.Guard})
}

func (s *Server) runAgent(w http.ResponseWriter, r *http.Request) {
	s.queueCycle(w, r, "agent:"+chi.URLParam(r, "name"), r.URL.Query().Get("via"))
}

// PlanStepView is one step of Rencana hari ini with what its button acts on.
type PlanStepView struct {
	gen.PlanItem
	Proposals []PlanProposal `json:"proposals"`
}

// PlanProposal is the state of one proposal behind a step.
type PlanProposal struct {
	ID       uuid.UUID `json:"id"`
	Kind     string    `json:"kind"`
	Status   string    `json:"status"`
	Button   string    `json:"button"`
	Icon     string    `json:"icon"`
	Autonomy string    `json:"autonomy"`
}

func (s *Server) planToday(w http.ResponseWriter, r *http.Request) {
	today := clock.Today(s.clock.Now())
	rows, err := s.st.Q.ListPlan(r.Context(), today)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	set, scoped, ok := s.scopedDealers(w, r)
	if !ok {
		return
	}
	items := make([]PlanStepView, 0, len(rows))
	auto := 0
	for _, it := range rows {
		v := PlanStepView{PlanItem: it, Proposals: []PlanProposal{}}
		ids := it.ProposalIds
		if len(ids) == 0 && it.ProposalID != nil {
			ids = []uuid.UUID{*it.ProposalID}
		}
		mine := false // one sales' page keeps the steps that touch their dealers, with only their proposals
		for _, id := range ids {
			if p, err := s.st.Q.GetProposal(r.Context(), id); err == nil {
				if !inScope(set, scoped, p.DealerID) {
					continue
				}
				mine = true
				v.Proposals = append(v.Proposals, PlanProposal{ID: p.ID, Kind: p.Kind, Status: p.Status, Button: deref(p.Button), Icon: deref(p.Icon), Autonomy: p.Autonomy})
			}
		}
		if scoped && !mine {
			continue
		}
		if deref(it.Autonomy) == "auto" {
			auto++
		}
		items = append(items, v)
	}
	var cycle *int64
	if c, err := s.st.Q.LatestFullCycle(r.Context()); err == nil {
		cycle = c.Number
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "auto": auto, "approve": len(items) - auto, "cycle_number": cycle})
}

// runPlanStep is "Jalankan sekarang" on an auto step: the human runs it now — each proposal behind it is approved
// by that human (ADR 0008), so nothing reaches a dealer without a recorded decision.
func (s *Server) runPlanStep(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Langkah tidak ditemukan")
		return
	}
	it, err := s.st.Q.GetPlanItem(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Langkah tidak ditemukan")
		return
	}
	if it.Status == "waiting" {
		httpx.Fail(w, http.StatusConflict, "waiting", "Langkah ini menunggu pembayaran masuk")
		return
	}
	u, _ := CurrentUser(r.Context())
	if u.SalesUserID == nil {
		httpx.Fail(w, http.StatusForbidden, "no_sales_user", "Pengguna tidak terhubung ke data sales")
		return
	}
	who := decider(u)
	ids := it.ProposalIds
	if len(ids) == 0 && it.ProposalID != nil {
		ids = []uuid.UUID{*it.ProposalID}
	}
	sent, skipped := 0, 0
	for _, pid := range ids {
		out, err := proposals.Decide(r.Context(), s.st, s.jobs, s.clock, s.cfg.OdooWrite, pid, who, proposals.Decision{Decision: "approve"})
		switch {
		case errors.Is(err, proposals.ErrNotOpen):
			skipped++
		case err != nil:
			httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
			return
		case out.OutboxID != nil:
			sent++
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sent": sent, "skipped": skipped, "result": planResult(sent, skipped)})
}

func planResult(sent, skipped int) string {
	if sent == 0 {
		return "Langkah ini sudah dijalankan"
	}
	msg := strconv.Itoa(sent) + " pesan masuk antrean kirim"
	if skipped > 0 {
		msg += " · " + strconv.Itoa(skipped) + " sudah diputuskan sebelumnya"
	}
	return msg
}

func (s *Server) getAutonomy(w http.ResponseWriter, r *http.Request) {
	pol, err := policy.Load(r.Context(), s.st.Q)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"matrix": pol.Autonomy, "guard": pol.Guard, "order": agentOrder})
}

// putAutonomy writes the matrix and/or guard as a new policy version (CEO only); credit_release stays approve.
func (s *Server) putAutonomy(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if deref(u.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang dapat mengubah matriks otonomi")
		return
	}
	var body struct {
		Matrix map[string]domain.AutonomyRow `json:"matrix"`
		Guard  *domain.AutonomyGuard         `json:"guard"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format kebijakan tidak valid")
		return
	}
	for agent, row := range body.Matrix {
		for _, k := range row.Auto {
			if k == domain.KindCreditRelease || k == domain.KindCreditLimit {
				httpx.Fail(w, http.StatusBadRequest, "invalid", agent+": "+k+" tidak boleh otonom")
				return
			}
		}
	}
	if body.Guard != nil && body.Guard.DealerMessages != "confirm" && body.Guard.DealerMessages != "auto" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "dealer_messages: confirm | auto")
		return
	}
	var err error
	if body.Matrix != nil {
		err = s.writePolicy(r, "autonomy.matrix", body.Matrix)
	}
	if err == nil && body.Guard != nil {
		err = s.writePolicy(r, "autonomy.guard", body.Guard)
	}
	if err != nil {
		policyError(w, err)
		return
	}
	s.getAutonomy(w, r)
}
