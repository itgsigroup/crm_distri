package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"distri-arc/internal/analyst"
	"distri-arc/internal/httpx"
)

// Claude routine (ADR 0049): the routine on claude.ai a waiting cycle fires, so Claude analyses in the cloud without
// anyone opening Claude Desktop. The CEO stores the routine id and its API-trigger token; others see whether it is set.

// aiRoutine: GET /ai/routine — {set, routine} (the id shortened, never the token).
func (s *Server) aiRoutine(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !can(u, "conn") && !can(u, "mcp") {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Routine Claude untuk pemegang menu Pengaturan atau MCP Claude")
		return
	}
	id, _ := analyst.LoadRoutine(r.Context(), s.st.Q, s.secret())
	out := map[string]any{"set": id != "", "routine": "", "url": "", "can_configure": deref(u.Role) == "ceo"}
	if id != "" {
		out["routine"], out["url"] = analyst.RoutineHint(id), "https://claude.ai/code/routines/"+id
	}
	httpx.JSON(w, http.StatusOK, out)
}

// aiRoutineSave: PUT /ai/routine {routine_id, token} — CEO only, audited without the token.
func (s *Server) aiRoutineSave(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var body struct {
		RoutineID string `json:"routine_id"`
		Token     string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	id, token := strings.TrimSpace(body.RoutineID), strings.TrimSpace(body.Token)
	if err := analyst.ValidRoutine(id, token); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if err := analyst.SaveRoutine(r.Context(), s.st.Q, id, token, s.secret(), u.Email); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "ai_routine.saved", "secret:"+analyst.SecretRoutineID, map[string]any{"routine": analyst.RoutineHint(id)})
	s.aiRoutine(w, r)
}

// aiRoutineDelete: DELETE /ai/routine — waiting cycles go back to Claude through MCP by hand.
func (s *Server) aiRoutineDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ceoOnly(w, r); !ok {
		return
	}
	analyst.DeleteRoutine(r.Context(), s.st.Q)
	s.auditUser(r, "ai_routine.deleted", "secret:"+analyst.SecretRoutineID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// aiRoutineTest: POST /ai/routine/test — fires one short run that only checks the connector; returns its link.
func (s *Server) aiRoutineTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ceoOnly(w, r); !ok {
		return
	}
	id, token := analyst.LoadRoutine(r.Context(), s.st.Q, s.secret())
	if id == "" {
		httpx.Fail(w, http.StatusConflict, "no_routine", "Routine Claude belum diisi")
		return
	}
	url, err := analyst.FireRoutine(r.Context(), id, token, "TES KONEKSI dari GSI Orbit — tidak ada siklus untuk dianalisis. Panggil hanya orchestrator_status sekali, lalu selesai dengan satu kalimat: status Orchestrator saat ini. Jangan memanggil orchestrator_submit atau tool lain.")
	if err != nil {
		httpx.Fail(w, http.StatusBadGateway, "routine_failed", err.Error())
		return
	}
	s.auditUser(r, "ai_routine.test", "secret:"+analyst.SecretRoutineID, map[string]any{"run": url})
	httpx.JSON(w, http.StatusOK, map[string]any{"url": url})
}
