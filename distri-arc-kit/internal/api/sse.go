package api

import (
	"fmt"
	"net/http"
	"time"
)

// events streams server-sent events: one connection per browser, heartbeat every 25 seconds (07-api › SSE).
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, cancel := s.hub.Subscribe()
	defer cancel()
	_, _ = fmt.Fprintf(w, "event: hello\ndata: {\"at\":%q}\n\n", s.clock.Now().Format(time.RFC3339))
	fl.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			_, _ = fmt.Fprintf(w, "event: heartbeat\ndata: {\"at\":%q}\n\n", s.clock.Now().Format(time.RFC3339))
			fl.Flush()
		case e := <-ch:
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Name, e.Data)
			fl.Flush()
		}
	}
}
