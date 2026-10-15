package api

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgconn"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// Pinned messages and labels in Chat, like WhatsApp Business (migration 0039). A pin or a label belongs to the
// conversation — for a group, the same for every GSI number in it. Pins stay in GSI Orbit: nothing goes to WhatsApp.

// maxPins: WhatsApp keeps three pinned messages; a fourth replaces the oldest.
const maxPins = 3

var labelColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func (s *Server) chatLabelRoutes(r chi.Router) {
	r.Get("/chat/labels", s.listChatLabels)
	r.Post("/chat/labels", s.createChatLabel)
	r.Put("/chat/labels/{lid}", s.updateChatLabel)
	r.Delete("/chat/labels/{lid}", s.deleteChatLabel)
	r.Put("/chat/threads/{id}/labels", s.setThreadLabels)
	r.Put("/chat/threads/{id}/messages/{mid}/labels", s.setMessageLabels)
	r.Put("/chat/threads/{id}/messages/{mid}/pin", s.pinMessage)
}

// convKey is the conversation a thread shows: a group's jid (every GSI number's thread of it), else the thread.
func convKey(kind, jid *string, id uuid.UUID) string {
	if deref(kind) == "group" && deref(jid) != "" {
		return "g:" + *jid
	}
	return id.String()
}

// ChatLabel is one of the team's labels.
type ChatLabel struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
	Chats int32     `json:"chats"`
}

// PinView is a pinned message in the bar above the chat.
type PinView struct {
	MessageID uuid.UUID  `json:"message_id"`
	SentAt    time.Time  `json:"sent_at"`
	Direction string     `json:"direction"`
	FromName  string     `json:"from_name"`
	Body      string     `json:"body"`
	Media     *MediaView `json:"media,omitempty"`
	PinnedBy  string     `json:"pinned_by"`
	PinnedAt  time.Time  `json:"pinned_at"`
}

func (s *Server) listChatLabels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListChatLabels(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]ChatLabel, 0, len(rows))
	for _, l := range rows {
		out = append(out, ChatLabel{ID: l.ID, Name: l.Name, Color: l.Color, Chats: l.Chats})
	}
	u, _ := CurrentUser(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "can_manage": isAdmin(u)})
}

func labelBody(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	var b struct{ Name, Color string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&b); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return "", "", false
	}
	name := strings.Join(strings.Fields(b.Name), " ")
	if name == "" || utf8.RuneCountInString(name) > 40 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nama label 1–40 karakter")
		return "", "", false
	}
	if !labelColor.MatchString(b.Color) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Warna label tidak valid")
		return "", "", false
	}
	return name, strings.ToUpper(b.Color), true
}

func uniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// createChatLabel: anyone in Chat may add a label (as on WhatsApp Business); renaming and deleting are for admins.
func (s *Server) createChatLabel(w http.ResponseWriter, r *http.Request) {
	name, color, ok := labelBody(w, r)
	if !ok {
		return
	}
	u, _ := CurrentUser(r.Context())
	l, err := s.st.Q.InsertChatLabel(r.Context(), gen.InsertChatLabelParams{Name: name, Color: color, CreatedBy: u.Email})
	if uniqueViolation(err) {
		httpx.Fail(w, http.StatusConflict, "exists", "Label "+name+" sudah ada")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "chat.label_create", "chat_label", &l.ID, map[string]any{"name": name, "color": color})
	httpx.JSON(w, http.StatusCreated, ChatLabel{ID: l.ID, Name: l.Name, Color: l.Color})
}

func (s *Server) labelAdmin(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya admin yang bisa mengubah atau menghapus label")
		return uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "lid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Label tidak ditemukan")
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) updateChatLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := s.labelAdmin(w, r)
	if !ok {
		return
	}
	name, color, ok := labelBody(w, r)
	if !ok {
		return
	}
	n, err := s.st.Q.UpdateChatLabel(r.Context(), gen.UpdateChatLabelParams{ID: id, Name: name, Color: color})
	switch {
	case uniqueViolation(err):
		httpx.Fail(w, http.StatusConflict, "exists", "Label "+name+" sudah ada")
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
	case n == 0:
		httpx.Fail(w, http.StatusNotFound, "not_found", "Label tidak ditemukan")
	default:
		s.audit(r, "chat.label_update", "chat_label", &id, map[string]any{"name": name, "color": color})
		httpx.JSON(w, http.StatusOK, ChatLabel{ID: id, Name: name, Color: color})
	}
}

func (s *Server) deleteChatLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := s.labelAdmin(w, r)
	if !ok {
		return
	}
	if n, err := s.st.Q.DeleteChatLabel(r.Context(), id); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	} else if n == 0 {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Label tidak ditemukan")
		return
	}
	s.audit(r, "chat.label_delete", "chat_label", &id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func labelIDs(w http.ResponseWriter, r *http.Request) ([]uuid.UUID, bool) {
	var b struct {
		LabelIDs []uuid.UUID `json:"label_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&b); err != nil || len(b.LabelIDs) > 20 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return nil, false
	}
	if b.LabelIDs == nil {
		b.LabelIDs = []uuid.UUID{}
	}
	return b.LabelIDs, true
}

// setThreadLabels replaces a conversation's labels.
func (s *Server) setThreadLabels(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	ids, ok := labelIDs(w, r)
	if !ok {
		return
	}
	u, _ := CurrentUser(r.Context())
	conv := convKey(t.Kind, t.WaJid, t.ID)
	err := s.st.Tx(r.Context(), func(q *gen.Queries, _ pgx.Tx) error {
		if err := q.DeleteConvLabelsExcept(r.Context(), gen.DeleteConvLabelsExceptParams{Conv: conv, Keep: ids}); err != nil {
			return err
		}
		for _, id := range ids {
			if err := q.AddConvLabel(r.Context(), gen.AddConvLabelParams{Conv: conv, LabelID: id, AddedBy: u.Email}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Label tidak ditemukan")
		return
	}
	s.audit(r, "chat.labels", "chat_thread", &t.ID, map[string]any{"labels": ids})
	httpx.JSON(w, http.StatusOK, map[string]any{"labels": ids})
}

// threadMessage is a message of the conversation (a group: of any GSI number's thread of it).
func (s *Server) threadMessage(w http.ResponseWriter, r *http.Request, t gen.GetThreadRow) (gen.GetThreadMessageRow, bool) {
	mid, err := uuid.Parse(chi.URLParam(r, "mid"))
	if err == nil {
		m, err := s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ID: mid, ThreadID: t.ID})
		if err == nil {
			return m, true
		}
	}
	httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
	return gen.GetThreadMessageRow{}, false
}

// setMessageLabels replaces a message's labels.
func (s *Server) setMessageLabels(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	m, ok := s.threadMessage(w, r, t)
	if !ok {
		return
	}
	ids, ok := labelIDs(w, r)
	if !ok {
		return
	}
	u, _ := CurrentUser(r.Context())
	conv := convKey(t.Kind, t.WaJid, t.ID)
	err := s.st.Tx(r.Context(), func(q *gen.Queries, _ pgx.Tx) error {
		if err := q.DeleteMessageLabelsExcept(r.Context(), gen.DeleteMessageLabelsExceptParams{MessageID: m.ID, Keep: ids}); err != nil {
			return err
		}
		for _, id := range ids {
			if err := q.AddMessageLabel(r.Context(), gen.AddMessageLabelParams{MessageID: m.ID, SentAt: m.SentAt, Conv: conv, LabelID: id, AddedBy: u.Email}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Label tidak ditemukan")
		return
	}
	s.audit(r, "chat.message_labels", "chat_message", &m.ID, map[string]any{"labels": ids})
	httpx.JSON(w, http.StatusOK, map[string]any{"labels": ids})
}

// pinMessage pins or unpins a message of the conversation ({"pinned": true|false}).
func (s *Server) pinMessage(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	m, ok := s.threadMessage(w, r, t)
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
	if b.Pinned && m.DeletedAt != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pesan yang dihapus tidak bisa disematkan")
		return
	}
	if b.Pinned && (m.Status == "pending" || m.Status == "failed") { // its time changes when it goes out
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pesan yang belum terkirim tidak bisa disematkan")
		return
	}
	u, _ := CurrentUser(r.Context())
	conv := convKey(t.Kind, t.WaJid, t.ID)
	err := s.st.Tx(r.Context(), func(q *gen.Queries, _ pgx.Tx) error {
		if !b.Pinned {
			return q.DeletePin(r.Context(), gen.DeletePinParams{Conv: conv, MessageID: m.ID})
		}
		if err := q.InsertPin(r.Context(), gen.InsertPinParams{Conv: conv, MessageID: m.ID, SentAt: m.SentAt, PinnedBy: u.Name}); err != nil {
			return err
		}
		return q.TrimPins(r.Context(), gen.TrimPinsParams{Conv: conv, Keep: maxPins})
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, map[bool]string{true: "chat.pin", false: "chat.unpin"}[b.Pinned], "chat_message", &m.ID, map[string]any{"thread": t.ID})
	pins, err := s.pinsOf(r, conv)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"pins": pins})
}

// pinsOf is a conversation's pinned messages, last pinned first.
func (s *Server) pinsOf(r *http.Request, conv string) ([]PinView, error) {
	rows, err := s.st.Q.ListPins(r.Context(), conv)
	if err != nil {
		return nil, err
	}
	out := make([]PinView, 0, len(rows))
	for _, p := range rows {
		out = append(out, PinView{MessageID: p.MessageID, SentAt: p.SentAt, Direction: p.Direction, FromName: p.FromName, Body: p.Body,
			Media: mediaView(p.Media), PinnedBy: deref(p.PinnedBy), PinnedAt: p.PinnedAt})
	}
	return out, nil
}

// labelsByConv maps every conversation to its labels (the chat list).
func (s *Server) labelsByConv(r *http.Request) map[string][]uuid.UUID {
	rows, err := s.st.Q.AllConvLabels(r.Context())
	out := map[string][]uuid.UUID{}
	if err != nil {
		return out
	}
	for _, x := range rows {
		out[x.Conv] = append(out[x.Conv], x.LabelID)
	}
	return out
}

// attachLabelsPins marks each bubble with its labels and whether it is pinned.
func (s *Server) attachLabelsPins(ctx context.Context, msgs []MessageView, pins []PinView) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	rows, err := s.st.Q.MessageLabels(ctx, ids)
	if err != nil {
		return err
	}
	by := map[uuid.UUID][]uuid.UUID{}
	for _, x := range rows {
		by[x.MessageID] = append(by[x.MessageID], x.LabelID)
	}
	pinned := map[uuid.UUID]bool{}
	for _, p := range pins {
		pinned[p.MessageID] = true
	}
	for i := range msgs {
		msgs[i].Labels, msgs[i].Pinned = by[msgs[i].ID], pinned[msgs[i].ID]
	}
	return nil
}
