package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/httpx"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/waanalysis"
	"distri-arc/internal/wareport"
)

// Report WhatsApp (ADR 0050): GET /wa/report?days=1|7|30&sales= — per sales response speed, SLA, hanging chats,
// broken promises, new numbers, order requests won, score and rank. Measured from the stored chats; a sales sees only
// their own conversations (as in Chat), others pick a sales with ?sales=.

// reportDays are the periods the page offers.
var reportDays = map[int]bool{1: true, 7: true, 30: true}

func (s *Server) waReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if !reportDays[days] {
		days = 7
	}
	now := s.clock.Now()
	since := clock.Today(now).AddDate(0, 0, -(days - 1)) // "1" is today from 00.00 WIB
	pol, err := policy.Load(ctx, s.st.Q)
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
	rows, err := s.st.Q.WAWindowMessages(ctx, gen.WAWindowMessagesParams{Since: since, Until: now})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	hours := waanalysis.HoursOf(pol.Chat)
	var threads []waanalysis.Thread
	at := map[uuid.UUID]int{}
	skip := map[uuid.UUID]bool{}
	dealers := map[uuid.UUID]bool{}
	for _, m := range rows {
		if skip[m.ThreadID] {
			continue
		}
		i, seen := at[m.ThreadID]
		if !seen {
			// the same visibility as Chat and Analisa WhatsApp: the sales' page, and a user who holds numbers sees only theirs
			if !viewer.sees(m.SalesID, m.Account) || !onSalesPage(set, scoped, m.DealerID, deref(m.SalesName), scopeName) {
				skip[m.ThreadID] = true
				continue
			}
			i = len(threads)
			at[m.ThreadID] = i
			threads = append(threads, waanalysis.Thread{ID: m.ThreadID, Kind: deref(m.ThreadKind), Title: deref(m.ThreadTitle), DealerID: m.DealerID,
				DealerSlug: deref(m.DealerSlug), DealerName: deref(m.DealerName), SalesID: m.SalesID, SalesName: deref(m.SalesName), Hours: hours})
			if m.DealerID != nil {
				dealers[*m.DealerID] = true
			}
		}
		threads[i].Messages = append(threads[i].Messages, waanalysis.Message{ID: m.ID, In: deref(m.Direction) == "in" && !m.Internal, Text: deref(m.Body),
			At: m.SentAt, Holding: deref(m.Holding) || deref(m.AutoReply), Closing: deref(m.Closing) || deref(m.AutoReply)})
	}
	var orders []wareport.Order
	if len(dealers) > 0 {
		ids := make([]uuid.UUID, 0, len(dealers))
		for id := range dealers {
			ids = append(ids, id)
		}
		os, err := s.st.Q.WAReportOrders(ctx, gen.WAReportOrdersParams{DealerIds: ids, Since: &since})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		for _, o := range os {
			if o.DealerID != nil {
				orders = append(orders, wareport.Order{DealerID: *o.DealerID, At: o.At, Total: o.Total})
			}
		}
	}
	rep := wareport.Build(threads, orders, wareport.Config{Now: now, Since: since, Days: days, Hours: hours, SLAMin: pol.Chat.ReplySLAMin})
	httpx.JSON(w, http.StatusOK, rep)
}
