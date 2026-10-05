package api

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/httpx"
	"distri-arc/internal/proposals"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

func (s *Server) proposalRoutes(r chi.Router) {
	r.Get("/proposals", s.listProposals)
	r.Get("/proposals/{id}", s.getProposal)
	r.Post("/proposals/{id}/decide", s.decideProposal)
	r.Get("/calibration", s.calibration)
}

// ProposalView is a proposal as the UI shows it.
type ProposalView struct {
	gen.GetProposalRow
	Signals []views.TimelineEntry `json:"signals"`
}

func toView(r gen.ListProposalsRow) gen.GetProposalRow { return gen.GetProposalRow(r) }

func (s *Server) listProposals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := gen.ListProposalsParams{Lim: 100}
	if v := q.Get("status"); v != "" {
		p.Status = &v
	}
	if v := q.Get("agent"); v != "" {
		p.Agent = &v
	}
	if v := q.Get("dealer_id"); v != "" {
		if b, ok := s.board(w, r); ok {
			if it, ok := b.Get(v); ok {
				p.Dealer = &it.UUID
			}
		} else {
			return
		}
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		p.Lim = int32(n)
	}
	rows, err := s.st.Q.ListProposals(r.Context(), p)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	today := clockToday(s)
	out := []gen.GetProposalRow{}
	for _, row := range rows {
		if q.Get("queue") == "1" && !row.Queue {
			continue
		}
		if q.Get("today") == "1" && row.CreatedAt.Before(today) {
			continue
		}
		out = append(out, toView(row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) getProposal(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Proposal tidak ditemukan")
		return
	}
	p, err := s.st.Q.GetProposal(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Proposal tidak ditemukan")
		return
	}
	sigs, err := s.st.Q.ListSignalsByIDs(r.Context(), p.SignalIds)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, ProposalView{GetProposalRow: p, Signals: views.Timeline(sigs)})
}

// decideProposal is for humans only (07-api): MCP clients never reach this handler (stage 07 has no decide tool).
func (s *Server) decideProposal(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Proposal tidak ditemukan")
		return
	}
	var d proposals.Decision
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format keputusan tidak valid")
		return
	}
	u, _ := CurrentUser(r.Context())
	if u.SalesUserID == nil {
		httpx.Fail(w, http.StatusForbidden, "no_sales_user", "Pengguna tidak terhubung ke data sales")
		return
	}
	out, err := proposals.Decide(r.Context(), s.st, s.jobs, s.clock, s.cfg.OdooWrite, id, proposals.Decider{SalesUserID: *u.SalesUserID, Name: deref(u.Name), Role: deref(u.Role), Email: deref(u.Email)}, d)
	switch {
	case errors.Is(err, proposals.ErrNotOpen):
		httpx.Fail(w, http.StatusConflict, "not_open", "Proposal sudah diputuskan")
	case errors.Is(err, proposals.ErrForbidden):
		httpx.Fail(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, proposals.ErrReason):
		httpx.Fail(w, http.StatusBadRequest, "reason_required", "Pilih alasan penolakan")
	case err != nil:
		s.log.Error("decide", "err", err)
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
	default:
		httpx.JSON(w, http.StatusOK, out)
	}
}

// calibrationAgent is one bar of Pengaturan → Kalibrasi agen: share of human decisions that accepted the agent's
// proposals over the last 30 days (nil until something was decided).
type calibrationAgent struct {
	Agent      string `json:"agent"`
	Confidence *int   `json:"confidence"`
	Accepted   int64  `json:"accepted"`
	Rejected   int64  `json:"rejected"`
}

func (s *Server) calibration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.st.Q.ListCalibration(ctx, 10)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	stats, err := s.st.Q.AgentDecisionStats(ctx, s.clock.Now().AddDate(0, 0, -30))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	by := map[string]gen.AgentDecisionStatsRow{}
	for _, st := range stats {
		by[st.Agent] = st
	}
	agents := []calibrationAgent{}
	for _, name := range domain.AgentNames {
		st := by[name]
		a := calibrationAgent{Agent: name, Accepted: st.Accepted, Rejected: st.Rejected}
		if n := st.Accepted + st.Rejected; n > 0 {
			c := int(math.Round(float64(st.Accepted) * 100 / float64(n)))
			a.Confidence = &c
		}
		agents = append(agents, a)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"agents": agents, "items": rows, "as_of": s.clock.Now()})
}
