package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"distri-arc/internal/ask"
	"distri-arc/internal/httpx"
)

// askHandler answers a command-bar question with sources (Tanya, ⌘K).
func (s *Server) askHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Q string `json:"q"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Q) == "" || len(body.Q) > 500 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tulis pertanyaan (maks 500 karakter)")
		return
	}
	svc := s.ask
	if svc == nil {
		svc = &ask.Service{St: s.st, Clock: s.clock}
	}
	a, err := svc.Ask(r.Context(), body.Q)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "ask", "ask", map[string]any{"q": body.Q, "intent": a.Intent, "sources": len(a.Sources)})
	httpx.JSON(w, http.StatusOK, a)
}
