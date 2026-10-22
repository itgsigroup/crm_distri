package api

import (
	"encoding/json"
	"net/http"

	"distri-arc/internal/chatstats"
	"distri-arc/internal/clock"
	"distri-arc/internal/httpx"
	"distri-arc/internal/waanalysis"
)

// chatStats is GET /chat/threads/{id}/stats: the lead panel's Statistik over the whole stored conversation (a group:
// every GSI number's thread of it) — volume, trend, answer speed, when the customer writes, who starts the talks,
// what is asked, who writes in a group — with insight lines computed from the numbers (chatstats).
func (s *Server) chatStats(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := s.st.Q.ThreadStatsMessages(ctx, t.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	group := deref(t.Kind) == "group"
	msgs := make([]chatstats.Msg, 0, len(rows))
	for _, m := range rows {
		from := ""
		if group {
			from = m.FromName
		}
		msgs = append(msgs, chatstats.Msg{At: m.SentAt, Out: m.Direction == "out", Internal: m.Internal, Holding: m.Holding, AutoReply: m.AutoReply,
			Closing: m.Closing, Failed: m.Status == "failed", Media: m.HasMedia, From: from})
	}
	ar, err := s.st.Q.ThreadStatsAsks(ctx, t.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	asks := make([]chatstats.Ask, 0, len(ar))
	for _, a := range ar {
		var items []chatstats.Item
		_ = json.Unmarshal(a.Items, &items)
		asks = append(asks, chatstats.Ask{Intent: a.Intent, Items: items})
	}
	httpx.JSON(w, http.StatusOK, chatstats.Compute(msgs, asks, waanalysis.IntentLabel, s.clock.Now(), clock.WIB))
}
