package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// The chat list's row menu, like WhatsApp: pin a chat to the top of one's own list, mark it unread, mark it
// unreplied or replied (migration 0040).

// maxThreadPins: WhatsApp pins at most three chats.
const maxThreadPins = 3

func (s *Server) chatListRoutes(r chi.Router) {
	r.Put("/chat/threads/{id}/pin-chat", s.pinThread)
	r.Post("/chat/threads/{id}/unread", s.markUnread)
	r.Put("/chat/threads/{id}/reply-state", s.setReplyState)
}

// threadPins maps the conversations the person pinned to when they pinned them.
func (s *Server) threadPins(r *http.Request) map[string]time.Time {
	out := map[string]time.Time{}
	u, ok := CurrentUser(r.Context())
	if !ok {
		return out
	}
	rows, err := s.st.Q.UserThreadPins(r.Context(), u.ID)
	if err != nil {
		return out
	}
	for _, p := range rows {
		out[p.Conv] = p.PinnedAt
	}
	return out
}

func (s *Server) pinThread(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	var b struct {
		Pinned bool `json:"pinned"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&b); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	u, _ := CurrentUser(r.Context())
	conv := convKey(t.Kind, t.WaJid, t.ID)
	if !b.Pinned {
		if err := s.st.Q.UnpinThread(r.Context(), gen.UnpinThreadParams{UserID: u.ID, Conv: conv}); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"pinned": false})
		return
	}
	if _, already := s.threadPins(r)[conv]; !already {
		if n, err := s.st.Q.CountThreadPins(r.Context(), u.ID); err == nil && n >= maxThreadPins {
			httpx.Fail(w, http.StatusConflict, "pin_limit", "Maksimal 3 chat disematkan — lepas sematan salah satu chat dulu")
			return
		}
	}
	if err := s.st.Q.PinThread(r.Context(), gen.PinThreadParams{UserID: u.ID, Conv: conv}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"pinned": true})
}

// markUnread: "Tandai belum dibaca" — the chat shows a count until it is opened again.
func (s *Server) markUnread(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	if err := s.st.Q.MarkThreadUnread(r.Context(), t.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "chat.mark_unread", "chat_thread", &t.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// setReplyState: "Tandai belum dibalas" (waits from now until a reply goes out) or "Tandai sudah dibalas" (what the
// customer wrote so far needs no reply).
func (s *Server) setReplyState(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	var b struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&b); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	if deref(t.Kind) == "group" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Status balasan hanya untuk chat pribadi, bukan grup")
		return
	}
	now := s.clock.Now()
	var err error
	switch b.State {
	case "unreplied":
		err = s.st.Q.MarkThreadUnreplied(r.Context(), gen.MarkThreadUnrepliedParams{ID: t.ID, At: now})
	case "replied":
		err = s.st.Q.MarkThreadReplied(r.Context(), gen.MarkThreadRepliedParams{ID: t.ID, At: now})
	default:
		httpx.Fail(w, http.StatusBadRequest, "invalid", "state: unreplied | replied")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "chat.reply_state", "chat_thread", &t.ID, map[string]any{"state": b.State})
	w.WriteHeader(http.StatusNoContent)
}
