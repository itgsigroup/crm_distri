package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/httpx"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// readRoutes registers the read endpoints of stage 01 (07-api.md: Dealer, Orbit, Segmen, KPI, stock, credit).
func (s *Server) readRoutes(r chi.Router) {
	r.Get("/sales", s.sales)
	r.Get("/dealers", s.dealers)
	r.Get("/dealers/due", s.dealersDue)
	r.Get("/dealers/drift", s.dealersDrift)
	r.Get("/dealers/credit-tight", s.dealersCreditTight)
	r.Get("/dealers/{id}", s.dealer)
	r.Get("/dealers/{id}/{part}", s.dealerPart)
	r.Get("/orbit", s.orbit)
	r.Get("/orbit/summary", s.orbitSummary)
	r.Get("/orbit/movers", s.orbitMovers)
	r.Get("/segmen", s.segmen)
	r.Get("/segmen/summary", s.segmenSummary)
	r.Get("/segmen/movers", s.segmenMovers)
	r.Get("/kpi", s.kpi)
	r.Get("/agenda", s.agenda)
	r.Get("/stock/aging", s.stockAging)
	r.Get("/stock/critical", s.stockCritical)
	r.Get("/stock/push", s.stockPush)
	r.Get("/stock/sales-by-product", s.stockSales)
	r.Get("/policies", s.policies)
	r.Get("/credit/overview", s.creditPart("overview"))
	r.Get("/credit/dealers", s.creditPart("dealers"))
	r.Get("/credit/exposure", s.creditPart("exposure"))
	r.Get("/credit/forecast", s.creditPart("forecast"))
}

// board is the dealers the user works with: a sales user sees only the dealers they own in every list (07-api ›
// RBAC); other roles see all. Pages of one dealer use fullBoard (a sales user may open a dealer from a group chat).
func (s *Server) board(w http.ResponseWriter, r *http.Request) (*views.Board, bool) {
	b, ok := s.fullBoard(w, r)
	if !ok {
		return nil, false
	}
	if name, ok := salesScope(r); ok {
		own := *b
		own.Items = b.Filter(name)
		b = &own
	}
	// ?type=reseller|si: dealer (reseller) or freelance / system integrator
	if t := r.URL.Query().Get("type"); t == "reseller" || t == "si" {
		typed := *b
		typed.Items = nil
		for _, it := range b.Items {
			if it.Type == t {
				typed.Items = append(typed.Items, it)
			}
		}
		b = &typed
	}
	return b, true
}

// salesScope is whose dealers a request sees: a sales user always their own; anyone else everything, or one sales'
// page with ?sales=<key or name> (Pusat kendali, Orchestrator) — never mixed with another sales' dealers.
func salesScope(r *http.Request) (string, bool) {
	if u, _ := CurrentUser(r.Context()); deref(u.Role) == "sales" || roleOf(u).Scope == "own" { // own data only
		if u.SalesName == nil {
			return "\x00", true // a sales user without a sales profile sees no dealer
		}
		return *u.SalesName, true
	}
	if v := strings.TrimSpace(r.URL.Query().Get("sales")); v != "" && !strings.EqualFold(v, "all") {
		return v, true
	}
	return "", false
}

// scopedDealers is the set of dealer ids of the request's sales scope (ok=false: no scope, everything).
func (s *Server) scopedDealers(w http.ResponseWriter, r *http.Request) (map[uuid.UUID]bool, bool, bool) {
	if _, scoped := salesScope(r); !scoped {
		return nil, false, true
	}
	b, ok := s.board(w, r)
	if !ok {
		return nil, true, false
	}
	set := make(map[uuid.UUID]bool, len(b.Items))
	for _, it := range b.Items {
		set[it.UUID] = true
	}
	return set, true, true
}

// inScope: no scope, or the row's dealer belongs to it (rows without a dealer belong to no sales' page).
func inScope(set map[uuid.UUID]bool, scoped bool, dealer *uuid.UUID) bool {
	return !scoped || (dealer != nil && set[*dealer])
}

func (s *Server) fullBoard(w http.ResponseWriter, r *http.Request) (*views.Board, bool) {
	b, err := s.views.Board(r.Context())
	if err != nil {
		s.log.Error("board", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Gagal memuat data dealer")
		return nil, false
	}
	return b, true
}

func items(v []views.BoardItem) map[string]any {
	if v == nil {
		v = []views.BoardItem{}
	}
	return map[string]any{"items": v}
}

func (s *Server) sales(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListSalesUsers(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	holders, err := s.st.Q.ListMapUsers(r.Context()) // Mapping sales: the Pengguna whose main profile it is
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	holder := map[uuid.UUID]gen.ListMapUsersRow{}
	for _, h := range holders {
		if h.SalesUserID != nil {
			if _, taken := holder[*h.SalesUserID]; !taken {
				holder[*h.SalesUserID] = h
			}
		}
	}
	out := []map[string]any{}
	for _, u := range rows {
		if u.Role != "sales" {
			continue
		}
		v := map[string]any{"id": u.ID, "key": strings.ToLower(u.Name), "name": u.Name, "branch": u.Branch, "initials": views.Initials(u.Name),
			"wa_number": u.WaNumber, "dealers": len(b.Filter(u.Name))}
		if h, ok := holder[u.ID]; ok {
			v["user_id"], v["user_name"] = h.ID, h.Name
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) dealers(w http.ResponseWriter, r *http.Request) {
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	list := b.Filter(q.Get("sales"))
	if term := strings.TrimSpace(q.Get("q")); term != "" {
		ids, err := s.st.Q.SearchDealerIDs(r.Context(), term)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		rank := map[string]int{}
		for i, id := range ids {
			rank[id.String()] = i
		}
		var hit []views.BoardItem
		for _, it := range list {
			if _, ok := rank[it.UUID.String()]; ok {
				hit = append(hit, it)
			}
		}
		list = hit
	}
	var out []views.BoardItem
	for _, it := range list {
		if st := q.Get("status"); st != "" && !strings.EqualFold(views.RingOf(it.Metrics.Status), st) && !strings.EqualFold(it.Metrics.Status, st) {
			continue
		}
		if sg := q.Get("segment"); sg != "" && !strings.EqualFold(it.Metrics.Segment, sg) {
			continue
		}
		out = append(out, it)
	}
	views.SortByCycDesc(out)
	httpx.JSON(w, http.StatusOK, items(out))
}

func (s *Server) dealersDue(w http.ResponseWriter, r *http.Request) {
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	days := atoi(r.URL.Query().Get("days"), 7)
	httpx.JSON(w, http.StatusOK, items(b.Due(days)))
}

func (s *Server) dealersDrift(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		httpx.JSON(w, http.StatusOK, items(b.Drift()))
	}
}

func (s *Server) dealersCreditTight(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		httpx.JSON(w, http.StatusOK, items(b.CreditTight()))
	}
}

func (s *Server) dealerByID(w http.ResponseWriter, r *http.Request) (*views.Board, views.BoardItem, bool) {
	b, ok := s.fullBoard(w, r)
	if !ok {
		return nil, views.BoardItem{}, false
	}
	it, ok := b.Get(chi.URLParam(r, "id"))
	if !ok {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Dealer tidak ditemukan")
		return nil, views.BoardItem{}, false
	}
	return b, it, true
}

func (s *Server) dealer(w http.ResponseWriter, r *http.Request) {
	b, it, ok := s.dealerByID(w, r)
	if !ok {
		return
	}
	d, err := s.views.DealerDetail(r.Context(), b, it)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (s *Server) dealerPart(w http.ResponseWriter, r *http.Request) {
	b, it, ok := s.dealerByID(w, r)
	if !ok {
		return
	}
	h := b.Data.Histories[it.UUID]
	switch chi.URLParam(r, "part") {
	case "next":
		httpx.JSON(w, http.StatusOK, map[string]any{"next": it.Next})
	case "orders":
		httpx.JSON(w, http.StatusOK, views.DealerOrders(h, it.Metrics, b.Today, atoi(r.URL.Query().Get("months"), 6)))
	case "mix":
		httpx.JSON(w, http.StatusOK, map[string]any{"sow": it.Metrics.SOW, "sow_source": it.Metrics.SOWSource, "mix": it.Metrics.Mix,
			"mix_cats": it.Metrics.MixCats, "categories": domain.Categories, "composition": it.Composition})
	case "credit":
		httpx.JSON(w, http.StatusOK, map[string]any{"credit": it.Metrics.Credit, "open_invoices": views.OpenInvoices(h, b.Today)})
	case "contacts":
		httpx.JSON(w, http.StatusOK, map[string]any{"items": views.Contacts(h.Contacts, b.Today), "pic_active": it.Metrics.PICActive})
	case "commitments", "timeline", "memo", "detail":
		d, err := s.views.DealerDetail(r.Context(), b, it)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		switch chi.URLParam(r, "part") {
		case "commitments":
			httpx.JSON(w, http.StatusOK, d.Commitments)
		case "timeline":
			tl := d.Timeline
			if n := atoi(r.URL.Query().Get("limit"), len(tl)); n < len(tl) {
				tl = tl[:n]
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"items": tl})
		case "memo":
			httpx.JSON(w, http.StatusOK, map[string]any{"memo": d.Memo, "updated_at": d.MemoUpdatedAt, "signals": d.MemoSignals})
		default:
			httpx.JSON(w, http.StatusOK, d)
		}
	default:
		httpx.Fail(w, http.StatusNotFound, "not_found", "Bagian tidak dikenal")
	}
}

// customers is the board of one sales (or all) without prospects, plus how many prospects were left out.
func customers(b *views.Board, r *http.Request) ([]views.BoardItem, int) {
	return views.Customers(b.Filter(r.URL.Query().Get("sales")))
}

func (s *Server) orbit(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		list, _ := customers(b, r)
		httpx.JSON(w, http.StatusOK, items(list))
	}
}

func (s *Server) orbitSummary(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		list, prospects := customers(b, r)
		httpx.JSON(w, http.StatusOK, map[string]any{"items": views.OrbitSummary(list), "prospects": prospects})
	}
}

func (s *Server) orbitMovers(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		list, _ := customers(b, r)
		httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(views.OrbitMovers(list))})
	}
}

func (s *Server) segmen(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		list, _ := customers(b, r)
		httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(list),
			"thresholds": map[string]any{"freq_per_month": b.Policies.Segment.FreqPerMonth, "size_idr": b.Policies.Segment.SizeIDR}})
	}
}

func (s *Server) segmenSummary(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		list, prospects := customers(b, r)
		sum, total := views.SegmenSummary(list)
		httpx.JSON(w, http.StatusOK, map[string]any{"items": sum, "total_omzet_bln": total, "prospects": prospects})
	}
}

func (s *Server) segmenMovers(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		list, _ := customers(b, r)
		httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(views.SegmenMovers(list))})
	}
}

func (s *Server) stock(w http.ResponseWriter, r *http.Request) ([]domain.StockItem, bool) {
	rows, err := s.st.Q.ListStockItems(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return nil, false
	}
	return views.StockItems(rows), true
}

func (s *Server) kpi(w http.ResponseWriter, r *http.Request) {
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	st, ok := s.stock(w, r)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, b.KPI(r.URL.Query().Get("branch"), st))
}

func (s *Server) agenda(w http.ResponseWriter, r *http.Request) {
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	sales, err := s.st.Q.ListSalesUsers(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	rows := b.Agenda(sales)
	if name, scoped := salesScope(r); scoped { // one sales' page: only their own agenda
		own := rows[:0]
		for _, a := range rows {
			if strings.EqualFold(a.Sales.Key, name) || strings.EqualFold(a.Sales.Name, name) {
				own = append(own, a)
			}
		}
		rows = own
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

func (s *Server) stockAging(w http.ResponseWriter, r *http.Request) {
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	if st, ok := s.stock(w, r); ok {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(b.StockAging(st, r.URL.Query().Get("branch")))})
	}
}

func (s *Server) stockCritical(w http.ResponseWriter, r *http.Request) {
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	if st, ok := s.stock(w, r); ok {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(b.StockCritical(st))})
	}
}

// stockPush is the compact push list of Pusat kendali: the PushTop aging items (largest value) that have candidates.
func (s *Server) stockPush(w http.ResponseWriter, r *http.Request) {
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	st, ok := s.stock(w, r)
	if !ok {
		return
	}
	var out []views.AgingItem
	for _, a := range b.StockAging(st, "") {
		if a.AgeDays > b.Policies.Stock.AgingDays && a.CandidateCount > 0 {
			out = append(out, a)
			if len(out) == pushTop {
				break
			}
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(out)})
}

// pushTop is the size of Pusat kendali's push list.
const pushTop = 10

func (s *Server) stockSales(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.board(w, r); ok {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": b.SalesByProduct(atoi(r.URL.Query().Get("days"), 30), 5)})
	}
}

func (s *Server) creditPart(part string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, ok := s.board(w, r)
		if !ok {
			return
		}
		ov, ar, ex, fc, total := b.Credit(atoi(r.URL.Query().Get("days"), 30))
		switch part {
		case "overview":
			httpx.JSON(w, http.StatusOK, ov)
		case "dealers":
			httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(ar)})
		case "exposure":
			httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(ex)})
		default:
			httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(fc), "total": total})
		}
	}
}

func clockToday(s *Server) time.Time { return clock.Today(s.clock.Now()) }

func atoi(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v >= 0 {
		return v
	}
	return def
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func (s *Server) briefToday(w http.ResponseWriter, r *http.Request) {
	_, scoped := salesScope(r) // the stored brief covers every sales: one sales' page gets a brief from their dealers
	requireAI := false
	if pol, err := policy.Load(r.Context(), s.st.Q); err == nil {
		requireAI = pol.LLM.RequireAI
	}
	// require_ai: a brief stored before (template sentences) is not shown — only one an AI wrote
	if row, err := s.st.Q.GetBrief(r.Context(), clockToday(s)); err == nil && !scoped && (!requireAI || row.Source == "llm") {
		// written by the last full cycle (text + signal_ids per point); lists normalised for briefs stored by older builds
		var stored views.Brief
		if json.Unmarshal(row.Brief, &stored) == nil {
			for i := range stored.Points {
				if stored.Points[i].Dealers == nil {
					stored.Points[i].Dealers = []views.BriefDealer{}
				}
				if stored.Points[i].SignalIDs == nil {
					stored.Points[i].SignalIDs = []string{}
				}
			}
			if stored.Points == nil {
				stored.Points = []views.BriefPoint{}
			}
			httpx.JSON(w, http.StatusOK, stored)
			return
		}
	}
	b, ok := s.board(w, r)
	if !ok {
		return
	}
	st, ok := s.stock(w, r)
	if !ok {
		return
	}
	since := clockToday(s).AddDate(0, 0, -1)
	c, err := s.st.Q.CountSignalsSince(r.Context(), since)
	if scoped {
		ids := make([]uuid.UUID, 0, len(b.Items))
		for _, it := range b.Items {
			ids = append(ids, it.UUID)
		}
		var sc gen.CountSignalsSinceForDealersRow
		sc, err = s.st.Q.CountSignalsSinceForDealers(r.Context(), gen.CountSignalsSinceForDealersParams{Since: since, Dealers: ids})
		c = gen.CountSignalsSinceRow(sc)
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	branches := map[string]bool{}
	for _, x := range st {
		branches[x.Branch] = true
	}
	brief := b.TemplateBrief(st, views.BriefCounts{WA: c.Wa, SO: c.So, Payments: c.Payments, Branches: len(branches)})
	if requireAI { // no AI summary yet: the counts only, no template sentences
		brief.Points, brief.Source = []views.BriefPoint{}, "none"
	}
	if cyc, err := s.st.Q.LatestFullCycle(r.Context()); err == nil {
		brief.Cycle, brief.GeneratedAt = cyc.Number, cyc.StartedAt // Ringkasan Orchestrator · <time of the last cycle>
	}
	httpx.JSON(w, http.StatusOK, brief)
}

func (s *Server) policies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListPolicies(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := map[string]any{}
	for _, p := range rows {
		out[p.Key] = map[string]any{"value": p.Value, "version": p.Version, "updated_at": p.UpdatedAt}
	}
	httpx.JSON(w, http.StatusOK, out)
}
