package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/clock"
	"distri-arc/internal/events"
	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// What the AI is analysing right now (Pusat kendali, Sam 17 Oct): per stage its line, per agent whether it waits,
// is being read or is done and how much it covers, and the AI's last tool calls on this cycle.

type progressStage struct {
	Stage      string     `json:"stage"`
	Status     string     `json:"status"`
	Text       string     `json:"text,omitempty"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type progressAgent struct {
	Agent   string     `json:"agent"`
	State   string     `json:"state"` // waiting | reading | done | failed
	Covers  string     `json:"covers,omitempty"`
	DoneAt  *time.Time `json:"done_at,omitempty"`
	Summary string     `json:"summary,omitempty"`
}

type progressStep struct {
	At     time.Time `json:"at"`
	Tool   string    `json:"tool"`
	Agent  string    `json:"agent,omitempty"`
	Text   string    `json:"text"`
	Client string    `json:"client,omitempty"`
}

// covers: "12 dealer · 30 pesan · 4 stok" from the arrays of an agent's Input.
func covers(input []byte) string {
	var in map[string]json.RawMessage
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	var parts []string
	for _, f := range [][2]string{{"dealers", "dealer"}, {"threads", "percakapan"}, {"messages", "pesan"}, {"candidates", "kandidat"}, {"stock", "stok"}, {"new_numbers", "nomor baru"}} {
		var xs []json.RawMessage
		if json.Unmarshal(in[f[0]], &xs) == nil && len(xs) > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", len(xs), f[1]))
		}
	}
	var since string
	if json.Unmarshal(in["since"], &since) == nil {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			parts = append(parts, "baru sejak "+t.In(clock.WIB).Format("02/01 15.04"))
		}
	}
	return strings.Join(parts, " · ")
}

// stepText says a tool call in words.
func stepText(tool, agent, result string) string {
	switch tool {
	case "orchestrator_input_get":
		return "membaca data " + agent
	case "orchestrator_submit":
		if result != "" {
			return "mengirim analisis " + result
		}
		return "mengirim analisis " + agent
	case "orchestrator_status":
		return "memeriksa status siklus"
	case "orchestrator_reanalyze", "orchestrator_run":
		return "memulai analisis"
	}
	if result != "" {
		return tool + " · " + result
	}
	return tool
}

func (s *Server) cycleProgress(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Siklus tidak ditemukan")
		return
	}
	ctx := r.Context()
	c, err := s.st.Q.GetCycle(ctx, id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Siklus tidak ditemukan")
		return
	}
	stages := []progressStage{}
	if rows, err := s.st.Q.ListCycleStages(ctx, id); err == nil {
		for _, x := range rows {
			var d struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(x.Detail, &d)
			stages = append(stages, progressStage{Stage: x.Stage, Status: x.Status, Text: d.Text, StartedAt: x.StartedAt, FinishedAt: x.FinishedAt})
		}
	}

	// agents: the cycle's Input rows (MCP), then the agent runs (finished, either engine), then what the AI opened
	agents := []progressAgent{}
	idx := map[string]int{}
	add := func(name string) *progressAgent {
		if i, ok := idx[name]; ok {
			return &agents[i]
		}
		idx[name] = len(agents)
		agents = append(agents, progressAgent{Agent: name, State: "waiting"})
		return &agents[len(agents)-1]
	}
	if rows, err := s.st.Q.ListCycleInputs(ctx, id); err == nil {
		for _, x := range rows {
			a := add(x.Agent)
			a.Covers = covers(x.Input)
			if x.SubmittedAt != nil {
				a.State, a.DoneAt = "done", x.SubmittedAt
			}
		}
	}
	if rows, err := s.st.Q.ListAgentRuns(ctx, &id); err == nil {
		for _, x := range rows {
			a := add(x.Agent)
			switch deref(x.Status) {
			case "failed":
				a.State, a.Summary = "failed", deref(x.Error)
			case "done", "template":
				a.State = "done"
				if x.ProposalsCount != nil {
					a.Summary = fmt.Sprintf("%d saran", *x.ProposalsCount)
				}
			}
		}
	}
	steps := []progressStep{}
	rows, err := s.st.Pool.Query(ctx, `select m.created_at, coalesce(m.tool, ''), coalesce(m.args->>'agent', ''), coalesce(m.result_summary, ''), coalesce(k.name, '')
		from mcp_calls m left join mcp_clients k on k.id = m.client_id
		where m.cycle_id = $1 or m.args->>'cycle_id' = $2 order by m.created_at desc limit 8`, id, id.String())
	if err == nil {
		for rows.Next() {
			var p progressStep
			var res string
			if rows.Scan(&p.At, &p.Tool, &p.Agent, &res, &p.Client) == nil {
				p.Text = stepText(p.Tool, p.Agent, res)
				steps = append(steps, p)
			}
		}
		rows.Close()
	}
	for i := len(steps) - 1; i >= 0; i-- { // oldest first: the last read of an agent not yet sent is "reading"
		if st := steps[i]; st.Tool == "orchestrator_input_get" && st.Agent != "" {
			if a := add(st.Agent); a.State == "waiting" {
				a.State = "reading"
			}
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"cycle_id": id, "number": c.Number, "status": c.Status, "stage": c.Stage, "via": c.Via,
		"stages": stages, "agents": agents, "steps": steps})
}

// cancelCycle is "Batalkan analisis": a queued cycle ends at once, a running one within a second (the worker checks
// at each stage and while it waits for Claude). 409 when the cycle already finished.
func (s *Server) cancelCycle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Siklus tidak ditemukan")
		return
	}
	u, _ := CurrentUser(r.Context())
	by := deref(u.Name)
	if by == "" {
		by = deref(u.Email)
	}
	c, err := s.st.Q.CancelCycle(r.Context(), gen.CancelCycleParams{ID: id, By: by})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusConflict, "not_running", "Analisis ini sudah selesai")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if c.Status == "failed" { // it was still queued: closed here, nobody else announces it
		_ = events.Notify(r.Context(), s.st.Pool, "cycle_done", map[string]any{"id": c.ID, "number": c.Number, "status": "failed", "note": deref(c.Note)})
	}
	s.auditUser(r, "cycle.cancel", "cycle:"+id.String(), map[string]any{"number": c.Number, "by": by})
	httpx.JSON(w, http.StatusOK, map[string]any{"id": c.ID, "status": c.Status, "cancelled_by": by})
}
