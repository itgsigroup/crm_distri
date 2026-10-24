package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// Balasan cepat (ADR 0065), like WhatsApp Business quick replies: the team's saved texts, typed with "/shortcut" in
// Chat. Picking one only fills the box — sending stays the person's Kirim. Anyone in Chat adds one; its maker or an
// admin changes or deletes it.

// quickShortcut is a shortcut without its "/": a letter or digit, then letters, digits, "-" or "_" (≤ 24).
var quickShortcut = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,23}$`)

const (
	maxQuickReplies  = 200  // WhatsApp Business keeps 50; a team shares these
	maxQuickReplyLen = 2000 // characters
)

func (s *Server) quickReplyRoutes(r chi.Router) {
	r.Get("/chat/quick-replies", s.listQuickReplies)
	r.Post("/chat/quick-replies", s.createQuickReply)
	r.Put("/chat/quick-replies/{qid}", s.updateQuickReply)
	r.Delete("/chat/quick-replies/{qid}", s.deleteQuickReply)
	r.Post("/chat/quick-replies/{qid}/use", s.useQuickReply)
}

// QuickReply is one saved text.
type QuickReply struct {
	ID        uuid.UUID `json:"id"`
	Shortcut  string    `json:"shortcut"`
	Body      string    `json:"body"`
	By        string    `json:"by"`
	Uses      int32     `json:"uses"`
	UpdatedAt time.Time `json:"updated_at"`
	CanEdit   bool      `json:"can_edit"`
}

func canEditQuick(u User, owner *uuid.UUID) bool {
	return isAdmin(u) || (owner != nil && *owner == u.ID)
}

func (s *Server) listQuickReplies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListQuickReplies(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	u, _ := CurrentUser(r.Context())
	out := make([]QuickReply, 0, len(rows))
	for _, q := range rows {
		out = append(out, QuickReply{ID: q.ID, Shortcut: q.Shortcut, Body: q.Body, By: q.CreatedByName, Uses: q.Uses, UpdatedAt: q.UpdatedAt, CanEdit: canEditQuick(u, q.CreatedBy)})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

// quickBody reads {shortcut, body}: the shortcut lower-cased without its "/", the text trimmed (lines kept).
func quickBody(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	var b struct{ Shortcut, Body string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&b); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return "", "", false
	}
	sc := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(b.Shortcut), "/"))
	if !quickShortcut.MatchString(sc) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pintasan 1–24 karakter: huruf, angka, - atau _ (tanpa spasi), mis. /rekening")
		return "", "", false
	}
	body := strings.TrimSpace(strings.ReplaceAll(b.Body, "\r\n", "\n"))
	if body == "" || utf8.RuneCountInString(body) > maxQuickReplyLen {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi pesan 1–2000 karakter")
		return "", "", false
	}
	return sc, body, true
}

func (s *Server) createQuickReply(w http.ResponseWriter, r *http.Request) {
	sc, body, ok := quickBody(w, r)
	if !ok {
		return
	}
	if n, err := s.st.Q.CountQuickReplies(r.Context()); err == nil && n >= maxQuickReplies {
		httpx.Fail(w, http.StatusConflict, "full", "Sudah 200 balasan cepat — hapus yang tidak dipakai dulu")
		return
	}
	u, _ := CurrentUser(r.Context())
	q, err := s.st.Q.InsertQuickReply(r.Context(), gen.InsertQuickReplyParams{Shortcut: sc, Body: body, CreatedBy: &u.ID, CreatedByName: deref(u.Name)})
	if uniqueViolation(err) {
		httpx.Fail(w, http.StatusConflict, "exists", "Pintasan /"+sc+" sudah dipakai")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "chat.quick_reply_create", "quick_reply", &q.ID, map[string]any{"shortcut": sc})
	httpx.JSON(w, http.StatusCreated, QuickReply{ID: q.ID, Shortcut: q.Shortcut, Body: q.Body, By: q.CreatedByName, UpdatedAt: q.UpdatedAt, CanEdit: true})
}

// quickOwned: the quick reply of the URL, when the person may change it (its maker or an admin).
func (s *Server) quickOwned(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "qid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Balasan cepat tidak ditemukan")
		return uuid.Nil, false
	}
	owner, err := s.st.Q.GetQuickReplyOwner(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Balasan cepat tidak ditemukan")
		return uuid.Nil, false
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return uuid.Nil, false
	}
	if u, _ := CurrentUser(r.Context()); !canEditQuick(u, owner) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya pembuatnya atau admin yang bisa mengubah balasan cepat ini")
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) updateQuickReply(w http.ResponseWriter, r *http.Request) {
	id, ok := s.quickOwned(w, r)
	if !ok {
		return
	}
	sc, body, ok := quickBody(w, r)
	if !ok {
		return
	}
	_, err := s.st.Q.UpdateQuickReply(r.Context(), gen.UpdateQuickReplyParams{ID: id, Shortcut: sc, Body: body})
	switch {
	case uniqueViolation(err):
		httpx.Fail(w, http.StatusConflict, "exists", "Pintasan /"+sc+" sudah dipakai")
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
	default:
		s.audit(r, "chat.quick_reply_update", "quick_reply", &id, map[string]any{"shortcut": sc})
		httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "shortcut": sc, "body": body})
	}
}

func (s *Server) deleteQuickReply(w http.ResponseWriter, r *http.Request) {
	id, ok := s.quickOwned(w, r)
	if !ok {
		return
	}
	if _, err := s.st.Q.DeleteQuickReply(r.Context(), id); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "chat.quick_reply_delete", "quick_reply", &id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// useQuickReply counts a pick (the most used come first in the "/" list).
func (s *Server) useQuickReply(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "qid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Balasan cepat tidak ditemukan")
		return
	}
	if n, err := s.st.Q.UseQuickReply(r.Context(), id); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	} else if n == 0 {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Balasan cepat tidak ditemukan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
