package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/jobs"
	"distri-arc/internal/outbox"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Reaksi per pesan (Sam 28 Oct, like WhatsApp): everyone's emoji under a bubble, and the team reacting from Chat as
// the conversation's number. A reaction reaches the customer, so it is a human decision like a reply: proposal,
// outbox row kind=react, audit; the worker carries it out through the bridge.

// ReactionView is one emoji under a bubble: how many reacted with it, and whether this number is one of them.
type ReactionView struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine,omitempty"`
}

// attachReactions adds the reactions to the messages; account is the conversation's number (its reaction is "mine").
func (s *Server) attachReactions(ctx context.Context, msgs []MessageView, account string) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(msgs))
	at := make(map[uuid.UUID]int, len(msgs))
	for i, m := range msgs {
		ids[i], at[m.ID] = m.ID, i
	}
	rows, err := s.st.Pool.Query(ctx, `select message_id, emoji, count(*), bool_or(reactor = $2) from chat_message_reactions
		where message_id = any($1) group by message_id, emoji order by message_id, min(reacted_at)`, ids, wa.Digits(account))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var v ReactionView
		if err := rows.Scan(&id, &v.Emoji, &v.Count, &v.Mine); err != nil {
			return err
		}
		msgs[at[id]].Reactions = append(msgs[at[id]].Reactions, v)
	}
	return rows.Err()
}

// validEmoji: a reaction is one emoji (a few code points: skin tone, ZWJ sequence), never words.
func validEmoji(e string) bool {
	if e == "" || len(e) > 32 || !utf8.ValidString(e) {
		return false
	}
	for _, r := range e {
		if r < 0x80 || unicode.IsLetter(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// chatReact sets (or with an empty emoji takes back) this number's reaction to a bubble.
func (s *Server) chatReact(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "mid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	u, ok := s.chatAllowed(w, r, t)
	if !ok {
		return
	}
	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	emoji := strings.TrimSpace(body.Emoji)
	if emoji != "" && !validEmoji(emoji) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Reaksi harus satu emoji")
		return
	}
	if t.SalesWa == nil || t.WaJid == nil {
		httpx.Fail(w, http.StatusConflict, "no_number", "Chat ini belum punya nomor GSI untuk memberi reaksi")
		return
	}
	if closed, _ := s.replyClosed(r.Context(), t); closed { // a reaction revives a chat too (ADR 0081)
		httpx.Fail(w, http.StatusConflict, "reply_window_closed", "Pelanggan belum mengirim pesan dalam 24 jam terakhir — reaksi juga tidak dikirim agar nomor tidak diblokir WhatsApp")
		return
	}
	m, err := s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ThreadID: t.ID, ID: mid})
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	if md := mediaView(m.Media); m.DeletedAt != nil || (md != nil && (md.Kind == "system" || md.Kind == "call")) {
		httpx.Fail(w, http.StatusConflict, "not_reactable", "Pesan ini tidak bisa diberi reaksi")
		return
	}
	target, why := s.replyQuote(r.Context(), t, mid.String())
	if target == nil {
		httpx.Fail(w, http.StatusConflict, "not_reactable", strings.Replace(why, "balas", "beri reaksi", 1))
		return
	}
	target.Text = ""
	var current string
	_ = s.st.Pool.QueryRow(r.Context(), "select emoji from chat_message_reactions where message_id = $1 and reactor = $2", mid, wa.Digits(*t.SalesWa)).Scan(&current)
	if current == emoji {
		httpx.JSON(w, http.StatusOK, map[string]any{"message_id": mid, "emoji": emoji, "status": "unchanged"})
		return
	}
	now := s.clock.Now()
	actor, kind, entity := deref(u.Email), "user", "chat_message"
	var res map[string]any
	err = s.st.Tx(r.Context(), func(q *gen.Queries, tx pgx.Tx) error {
		title, what := "Reaksi ke "+deref(t.Title), "Reaksi "+emoji+" pada: "+deref(m.Body)
		if emoji == "" {
			what = "Batalkan reaksi pada: " + deref(m.Body)
		}
		p, err := s.humanDecision(r.Context(), q, t, u, title, what, now)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(outbox.WAPayload{From: *t.SalesWa, To: *t.WaJid, Kind: "react", MessageID: mid.String(), ThreadID: t.ID.String(), Quote: target, Emoji: emoji})
		ob, err := q.InsertOutbox(r.Context(), gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: t.WaJid, Payload: payload})
		if err != nil {
			return err
		}
		action := "chat.react"
		after, _ := json.Marshal(map[string]any{"thread_id": t.ID, "outbox_id": ob.ID, "message_id": mid, "emoji": emoji, "was": current})
		if err := q.InsertAudit(r.Context(), gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &mid, After: after}); err != nil {
			return err
		}
		if s.jobs != nil {
			if _, err := s.jobs.InsertTx(r.Context(), tx, jobs.OutboxSendArgs{OutboxID: ob.ID.String()}, nil); err != nil {
				return err
			}
		}
		res = map[string]any{"proposal_id": p.ID, "outbox_id": ob.ID, "message_id": mid, "emoji": emoji, "status": "reacting"}
		return nil
	})
	if err != nil {
		s.log.Error("chat react", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Gagal mencatat reaksi")
		return
	}
	httpx.JSON(w, http.StatusAccepted, res)
}
