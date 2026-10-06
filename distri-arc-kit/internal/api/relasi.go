package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

func (s *Server) relasiRoutes(r chi.Router) {
	r.Get("/relasi", s.relasi)
	r.Get("/relasi/insights", s.relasiInsights)
}

// relasiGraph loads the board, the sales numbers and the monthly interactions for a period.
func (s *Server) relasiGraph(w http.ResponseWriter, r *http.Request) (*views.Board, views.Relasi, string, bool) {
	b, ok := s.board(w, r)
	if !ok {
		return nil, views.Relasi{}, "", false
	}
	ctx := r.Context()
	asOf, err := s.st.Q.InteractionsAsOf(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return nil, views.Relasi{}, "", false
	}
	rows, err := s.st.Q.RelasiMonthly(ctx, gen.RelasiMonthlyParams{AsOf: asOf, Since: views.RelasiSince(b.Today)})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return nil, views.Relasi{}, "", false
	}
	sales, err := s.st.Q.ListSalesUsers(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return nil, views.Relasi{}, "", false
	}
	var rs []views.RelasiSales
	for _, u := range sales {
		if u.Role != "sales" {
			continue
		}
		rs = append(rs, views.RelasiSales{ID: u.ID, Key: strings.ToLower(u.Name), Name: u.Name, Branch: u.Branch, WANumber: deref(u.WaNumber)})
	}
	var mc []views.MonthCount
	for _, x := range rows {
		mc = append(mc, views.MonthCount{SalesID: x.SalesID, DealerID: x.DealerID, Month: x.Month, N: x.N})
	}
	period, _ := strconv.Atoi(r.URL.Query().Get("period"))
	key := r.URL.Query().Get("sales")
	return b, views.BuildRelasi(b, rs, mc, period, key), key, true
}

func (s *Server) relasi(w http.ResponseWriter, r *http.Request) {
	if _, g, _, ok := s.relasiGraph(w, r); ok {
		httpx.JSON(w, http.StatusOK, g)
	}
}

func (s *Server) relasiInsights(w http.ResponseWriter, r *http.Request) {
	if b, g, key, ok := s.relasiGraph(w, r); ok {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(views.RelasiInsights(b, g, key))})
	}
}
