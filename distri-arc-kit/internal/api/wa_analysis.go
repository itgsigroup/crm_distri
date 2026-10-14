package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/analyst"
	"distri-arc/internal/httpx"
	"distri-arc/internal/waanalysis"
)

// waAnalysisRoutes: Analisa WhatsApp of the last full cycle (ADR 0026), the second analysis on Pusat kendali and
// Orchestrator next to the BigQuery one. Guarded by the chat page (it quotes customer messages).
func (s *Server) waAnalysisRoutes(r chi.Router) {
	r.Get("/wa/analysis", s.waAnalysis)
}

// WAHistoryRow is one cycle's reading of WhatsApp (Riwayat on the Orchestrator tab).
type WAHistoryRow struct {
	CycleNumber    *int64    `json:"cycle_number"`
	At             time.Time `json:"at"`
	Status         string    `json:"status"`
	Via            string    `json:"via"`
	Threads        int32     `json:"threads"`
	Inbound        int32     `json:"inbound"`
	Outbound       int32     `json:"outbound"`
	Waiting        int32     `json:"waiting"`
	ReplyMedianMin *int32    `json:"reply_median_min"`
	Source         string    `json:"source"`
}

// WAInsightView is a conversation as the tab shows it: the cycle's reading, its waiting state refreshed now.
type WAInsightView struct {
	waanalysis.Insight
	// Answered: the cycle saw the customer waiting, a reply went out since
	Answered bool `json:"answered"`
}

func (s *Server) waAnalysis(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.clock.Now()
	out := map[string]any{"cycle_number": nil, "generated_at": nil, "window_days": waanalysis.WindowDays, "reply_due_min": int(waanalysis.ReplyDue.Minutes()),
		"source": "none", "overview": nil, "items": []WAInsightView{}, "sales": []waanalysis.SalesRow{}, "history": []WAHistoryRow{}}
	tot, _ := waanalysis.Summarize(nil)
	out["totals"] = tot
	hist, err := s.st.Q.ListWAAnalyses(ctx, 30)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	history := make([]WAHistoryRow, 0, len(hist))
	for _, h := range hist {
		history = append(history, WAHistoryRow{CycleNumber: h.CycleNumber, At: h.CreatedAt, Status: h.CycleStatus, Via: deref(h.CycleVia), Threads: h.Threads,
			Inbound: h.Inbound, Outbound: h.Outbound, Waiting: h.Waiting, ReplyMedianMin: h.ReplyMedianMin, Source: h.Source})
	}
	out["history"] = history
	out["claude"] = s.waSchedule(ctx, now)
	a, err := s.st.Q.LatestWAAnalysis(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.JSON(w, http.StatusOK, out) // no cycle has read WhatsApp yet
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	rows, err := s.st.Q.ListWAInsights(ctx, a.CycleID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	set, scoped, ok := s.scopedDealers(w, r)
	if !ok {
		return
	}
	scopeName := ""
	if scoped {
		scopeName = s.scopeSalesName(r)
	}
	u, _ := CurrentUser(ctx)
	viewer, err := s.chatViewerOf(ctx, u)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	// Penagihan reads only the conversations of the number it holds (ADR 0033)
	ownNumbers := viewer.accounts != nil
	ids := make([]uuid.UUID, 0, len(rows))
	for _, x := range rows {
		ids = append(ids, x.ThreadID)
	}
	live, err := s.st.Q.ThreadsWaitingSince(ctx, ids)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	waiting := map[uuid.UUID]*time.Time{}
	for _, l := range live {
		waiting[l.ID] = timeOrNil(l.WaitingSince)
	}
	items := make([]WAInsightView, 0, len(rows))
	plain := make([]waanalysis.Insight, 0, len(rows))
	for _, x := range rows {
		if !onSalesPage(set, scoped, x.DealerID, deref(x.SalesName), scopeName) || (ownNumbers && !viewer.sees(nil, x.Account)) {
			continue
		}
		in := waanalysis.Insight{ThreadID: x.ThreadID, Kind: x.Kind, Title: x.Title, DealerID: x.DealerID, DealerSlug: deref(x.DealerSlug), DealerName: deref(x.DealerName),
			SalesID: x.SalesID, SalesName: deref(x.SalesName), Intent: x.Intent, Items: []waanalysis.Item{}, LastText: deref(x.LastText), Inbound: int(x.Inbound),
			Outbound: int(x.Outbound), LastInAt: x.LastInAt, LastOutAt: x.LastOutAt, WaitingSince: x.WaitingSince, NeedsReply: x.NeedsReply, Priority: int(x.Priority),
			SignalIDs: x.SignalIds, ProposalIDs: x.ProposalIds, Summary: deref(x.Summary), ReplyWorkMin: waanalysis.Ints(x.ReplyWorkMin), ReplyOffMin: waanalysis.Ints(x.ReplyOffMin)}
		_ = json.Unmarshal(x.Items, &in.Items)
		if x.ReplyMedianMin != nil {
			v := int(*x.ReplyMedianMin)
			in.ReplyMedianMin = &v
		}
		if x.Confidence != nil {
			v := int(*x.Confidence)
			in.Confidence = &v
		}
		v := WAInsightView{Insight: in}
		// the waiting state is read now, not at the cycle: a reply sent since clears it
		// (it only clears: a closing "Ok Bu" the cycle saw as needing no reply stays answered)
		if ws, ok := waiting[x.ThreadID]; ok && in.WaitingSince != nil && ws == nil {
			v.Answered, v.WaitingSince = true, nil
		}
		v.WaitMin, v.NeedsReply = 0, false
		if v.WaitingSince != nil {
			v.WaitMin = max(0, int(now.Sub(*v.WaitingSince).Minutes()))
			v.NeedsReply = now.Sub(*v.WaitingSince) >= waanalysis.ReplyDue
		}
		items = append(items, v)
		plain = append(plain, v.Insight)
	}
	tot, sales := waanalysis.Summarize(plain)
	out["cycle_number"], out["cycle_status"], out["generated_at"] = a.CycleNumber, a.CycleStatus, a.CreatedAt
	out["source"], out["items"], out["totals"], out["sales"] = a.Source, items, tot, sales
	if !scoped && !ownNumbers { // the overview covers every sales: one sales' page shows its own numbers only
		out["overview"] = a.Overview
	}
	httpx.JSON(w, http.StatusOK, out)
}

// onSalesPage: a conversation belongs to one sales' page when its dealer does, or — a new number without a dealer —
// when it wrote to that sales' number.
func onSalesPage(set map[uuid.UUID]bool, scoped bool, dealer *uuid.UUID, salesName, scopeName string) bool {
	if inScope(set, scoped, dealer) {
		return true
	}
	return dealer == nil && scopeName != "" && strings.EqualFold(salesName, scopeName)
}

// scopeSalesName resolves the request's sales scope (a key or a name) to the sales' name.
func (s *Server) scopeSalesName(r *http.Request) string {
	name, ok := salesScope(r)
	if !ok {
		return ""
	}
	if b, err := s.views.Board(r.Context()); err == nil {
		for _, it := range b.Items {
			if strings.EqualFold(it.Owner.Key, name) {
				return it.Owner.Name
			}
		}
	}
	return name
}

// chatStepThread is the conversation a "Balas chat" plan step points at ("chat:<thread id>").
func chatStepThread(link *string) (uuid.UUID, bool) {
	v, ok := strings.CutPrefix(deref(link), "chat:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(v)
	return id, err == nil
}

// chatStepState is a "Balas chat" step on one sales' page and whether the reply went out already.
func (s *Server) chatStepState(r *http.Request, id uuid.UUID, set map[uuid.UUID]bool, scoped bool, scopeName string) (mine, answered bool) {
	rows, err := s.st.Q.ThreadsWaitingSince(r.Context(), []uuid.UUID{id})
	if err != nil || len(rows) == 0 {
		return !scoped, false
	}
	t := rows[0]
	return onSalesPage(set, scoped, t.DealerID, deref(t.SalesName), scopeName), timeOrNil(t.WaitingSince) == nil
}

// waSchedule is the state of Claude's Analisa WhatsApp every 2 hours (ADR 0037) for the label in Pusat kendali:
// the last run (berhasil / gagal / berjalan) and the next one. Nil when the schedule was removed.
func (s *Server) waSchedule(ctx context.Context, now time.Time) map[string]any {
	sch, err := s.st.Q.GetMCPScheduleByKey(ctx, strPtr(analyst.KeyWAAnalysis))
	if err != nil {
		return nil
	}
	next := sch.NextRunAt
	if next == nil || next.Before(now) {
		next = analyst.NextRun(sch.Cron, now)
	}
	out := map[string]any{"schedule_id": sch.ID, "name": sch.Name, "enabled": sch.Enabled, "cron": sch.Cron, "next_run_at": next, "last": nil}
	if !sch.Enabled {
		out["next_run_at"] = nil
	}
	if run, err := s.st.Q.LatestMCPScheduleRun(ctx, sch.ID); err == nil {
		out["last"] = map[string]any{"id": run.ID, "status": run.Status, "error": run.Error, "started_at": run.StartedAt, "finished_at": run.FinishedAt}
	}
	return out
}

func strPtr(v string) *string { return &v }
