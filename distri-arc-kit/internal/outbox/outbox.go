// Package outbox is the only way anything leaves GSI Orbit: rows are written for approved proposals and the
// worker delivers them through the WhatsApp transport (or Odoo, stage 12) under the anti-ban rules.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/events"
	"distri-arc/internal/odoo"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// WAPayload is the payload of a channel=wa outbox row.
type WAPayload struct {
	From      string `json:"from"` // sales number (digits)
	To        string `json:"to"`   // chat JID
	Text      string `json:"text"`
	Kind      string `json:"kind"`       // proposal kind; "reply" = a human answering in a conversation, "edit" = a human editing a sent text
	MessageID string `json:"message_id"` // pending chat_messages row shown in the thread (edit: the message edited)
	ThreadID  string `json:"thread_id"`
	// an edit: the WhatsApp id of the message edited and when it was sent (WhatsApp accepts edits for 15 minutes)
	EditOf     string     `json:"edit_of,omitempty"`
	EditSentAt *time.Time `json:"edit_sent_at,omitempty"`
	// a reply to one bubble (WhatsApp "Balas"): the quoted message
	Quote *wa.Quote `json:"quote,omitempty"`
	// the members a group send tags ("@<number>" in Text)
	Mentions []string `json:"mentions,omitempty"`
	// a file, voice note or poll sent from Chat (Text is the caption); the file waits in chat_outgoing_files
	Content *OutContent `json:"content,omitempty"`
}

// OutContent is the non-text part of a send from Chat.
type OutContent struct {
	Kind     string          `json:"kind"` // image | video | document | audio | poll
	Mime     string          `json:"mime,omitempty"`
	FileName string          `json:"file_name,omitempty"`
	Size     int64           `json:"size,omitempty"`
	PTT      bool            `json:"ptt,omitempty"`
	Seconds  int             `json:"seconds,omitempty"`
	FileID   string          `json:"file_id,omitempty"`
	Poll     *wa.Poll        `json:"poll,omitempty"`
	Contact  *wa.ContactCard `json:"contact,omitempty"`
}

// Rules are the anti-ban limits (09-policies-security): proactive messages are capped per sales per day and
// spaced by a random gap; human replies inside a conversation only wait a short typing delay.
type Rules struct {
	GapMin, GapMax     time.Duration // between proactive sends of one number (default 20–90 s)
	ReplyMin, ReplyMax time.Duration // typing delay for replies (default 2–6 s)
	DailyCapOverride   int           // 0 = policy followup.rules.max_per_day_per_sales
	SendFrom, SendTo   int           // proactive sends only between these WIB hours (0, 0 = any time)
}

// DefaultRules are the production limits.
func DefaultRules() Rules {
	return Rules{GapMin: 20 * time.Second, GapMax: 90 * time.Second, ReplyMin: 2 * time.Second, ReplyMax: 6 * time.Second, SendFrom: 8, SendTo: 18}
}

// Sender delivers outbox rows.
type Sender struct {
	st    *store.Store
	t     wa.Transport
	clock clock.Clock
	rules Rules
	sleep func(time.Duration)
	odoo  odoo.Source
	// onSent: a chat message went out with its WhatsApp id (another linked GSI number gets its copy)
	onSent func(ctx context.Context, messageID uuid.UUID)
}

// WithOdoo lets the sender carry out Odoo rows (SO drafts, notes); nil keeps them pending.
func (s *Sender) WithOdoo(src odoo.Source) *Sender { s.odoo = src; return s }

// WithOnSent runs f for each chat message once WhatsApp accepted it.
func (s *Sender) WithOnSent(f func(ctx context.Context, messageID uuid.UUID)) *Sender {
	s.onSent = f
	return s
}

// windowWait is how long a proactive message waits for the send window (08–18 WIB by default).
func (r Rules) windowWait(now time.Time) time.Duration {
	if r.SendFrom == 0 && r.SendTo == 0 {
		return 0
	}
	t := now.In(clock.WIB)
	open := time.Date(t.Year(), t.Month(), t.Day(), r.SendFrom, 0, 0, 0, clock.WIB)
	switch {
	case t.Before(open):
		return open.Sub(t)
	case t.Hour() >= r.SendTo:
		return open.AddDate(0, 0, 1).Sub(t)
	}
	return 0
}

// NewSender builds a sender.
func NewSender(st *store.Store, t wa.Transport, c clock.Clock, r Rules) *Sender {
	return &Sender{st: st, t: t, clock: c, rules: r, sleep: time.Sleep}
}

// ErrRefused is a final no from the anti-ban guard (contact never wrote first, opted out, broadcast pattern):
// retrying would not help; the row stays failed with the reason for a person to see.
var ErrRefused = errors.New("ditolak penjaga anti-blokir")

// ErrCapReached tells the caller to retry tomorrow.
var ErrCapReached = errors.New("daily send cap reached")

// Send delivers one outbox row. It returns a positive duration when the job must be snoozed (gap or cap).
func (s *Sender) Send(ctx context.Context, id uuid.UUID) (time.Duration, error) {
	ob, err := s.st.Q.GetOutbox(ctx, id)
	if err != nil {
		return 0, err
	}
	if ob.Status == "sent" || ob.Status == "shadow" || ob.Status == "cancelled" {
		return 0, nil // idempotent; shadow rows and replies deleted before sending are never delivered
	}
	if pol, err := policy.Load(ctx, s.st.Q); err == nil && pol.Pilot.Shadow() && ob.Channel != "wa_system" {
		// a row queued before the pilot switched to shadow mode: keep it, do not deliver
		_, err := s.st.Q.ShadowOutbox(ctx, ob.ProposalID)
		return 0, err
	}
	if ob.Channel == "wa_system" {
		return s.sendSystem(ctx, ob)
	}
	if ob.Channel != "wa" {
		return 0, s.sendOdoo(ctx, ob)
	}
	var p WAPayload
	if err := json.Unmarshal(ob.Payload, &p); err != nil {
		return 0, err
	}
	if p.Kind == "edit" || p.Kind == "delete" {
		return s.sendEdit(ctx, ob, p)
	}
	now := s.clock.Now()
	if p.Kind == "reply" {
		s.sleep(jitter(s.rules.ReplyMin, s.rules.ReplyMax))
	} else {
		if w := s.rules.windowWait(now); w > 0 {
			return w, nil // outside 08–18 WIB: wait for the window
		}
		pol, err := policy.Load(ctx, s.st.Q)
		if err != nil {
			return 0, err
		}
		cap := pol.Followup.MaxPerDayPerSales
		if s.rules.DailyCapOverride > 0 {
			cap = s.rules.DailyCapOverride
		}
		day := clock.Today(now)
		n, err := s.st.Q.CountSentToday(ctx, gen.CountSentTodayParams{FromNumber: p.From, Since: &day})
		if err != nil {
			return 0, err
		}
		if int(n) >= cap {
			return day.AddDate(0, 0, 1).Add(8 * time.Hour).Sub(now), ErrCapReached
		}
		last, err := s.st.Q.LastSentAt(ctx, p.From)
		if err != nil {
			return 0, err
		}
		if gap := jitter(s.rules.GapMin, s.rules.GapMax); now.Sub(last) < gap {
			return gap - now.Sub(last), nil
		}
	}
	sctx := wa.WithMentions(wa.WithQuote(wa.WithAction(ctx, id.String()), p.Quote), p.Mentions)
	var msgID, ref string
	var sendErr error
	if p.Content != nil {
		msgID, ref, sendErr = s.sendContent(sctx, p)
	} else {
		msgID, sendErr = s.t.Send(sctx, p.From, p.To, p.Text)
	}
	sent := s.clock.Now()
	var refused *wa.SendRefused
	if errors.As(sendErr, &refused) {
		e := "Penjaga anti-blokir: " + refused.Reason
		if refused.Temporary() { // pacing: the same message goes out later
			_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: id, Status: "pending", Error: &e})
			return refused.RetryAfter, nil
		}
		_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: id, Status: "failed", Error: &e})
		if mid, err := uuid.Parse(p.MessageID); err == nil {
			_ = s.st.Q.SetChatMessageStatus(ctx, gen.SetChatMessageStatusParams{ID: mid, Status: "failed"})
		}
		s.notifyFailed(ctx, ob, errors.New(e))
		return 0, fmt.Errorf("%w: %s", ErrRefused, refused.Reason)
	}
	if sendErr != nil {
		e := sendErr.Error()
		_ = s.st.Q.SetOutboxResult(ctx, gen.SetOutboxResultParams{ID: id, Status: "failed", Error: &e})
		return 0, sendErr
	}
	err = s.st.Q.SetOutboxResult(ctx, gen.SetOutboxResultParams{ID: id, Status: "sent", SentAt: &sent})
	if err != nil {
		return 0, err
	}
	if p.MessageID != "" {
		if mid, err := uuid.Parse(p.MessageID); err == nil {
			body := &p.Text
			if p.Content != nil {
				body = nil // the bubble keeps its "[gambar] caption" text
				if ref != "" {
					media, _ := json.Marshal(wa.Media{Kind: p.Content.Kind, Mime: p.Content.Mime, FileName: p.Content.FileName, Size: p.Content.Size, Seconds: p.Content.Seconds, PTT: p.Content.PTT, Ref: ref})
					_ = s.st.Q.SetChatMessageMedia(ctx, gen.SetChatMessageMediaParams{ID: mid, Media: media})
				}
			}
			// the phone's echo (or another GSI number's copy) may arrive before, or between, these two steps: the
			// copies are dropped before and after the bubble takes the id, so the queued bubble stays the only one
			drop := gen.DropOtherCopiesParams{ID: mid, WaMsgID: msgID}
			_ = s.st.Q.DropOtherCopies(ctx, drop)
			err := s.st.Q.UpdateChatMessageSent(ctx, gen.UpdateChatMessageSentParams{ID: mid, WaMsgID: &msgID, Status: "sent", SentAt: sent, Body: body})
			_ = s.st.Q.DropOtherCopies(ctx, drop)
			if err == nil && s.onSent != nil {
				s.onSent(ctx, mid)
			}
		}
	}
	// the sent file stays OutgoingKeepDays so Chat shows it as it was sent (retention purges it); after that the
	// bubble is fetched from WhatsApp through its reference
	if err := s.st.Q.SetProposalExecuted(ctx, pidOf(ob)); err != nil {
		return 0, err
	}
	if err := s.trail(ctx, ob, p, sent); err != nil {
		return 0, err
	}
	actor, kind, action, entity := "worker", "system", "outbox.sent", "outbox"
	after, _ := json.Marshal(map[string]any{"wa_msg_id": msgID, "from": p.From, "to": p.To, "proposal_id": ob.ProposalID})
	_ = s.st.Q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &id, After: after})
	_ = events.Notify(ctx, s.st.Pool, "chat_message", map[string]string{"thread_id": p.ThreadID})
	_ = events.Notify(ctx, s.st.Pool, "proposal_changed", map[string]string{"id": pidOf(ob).String(), "status": "executed"})
	return 0, nil
}

func (s *Sender) sendOdoo(ctx context.Context, ob gen.Outbox) error {
	err := s.deliverOdoo(ctx, ob)
	switch {
	case errors.Is(err, ErrManual):
		e := err.Error()
		return s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: ob.ID, Status: "manual", Error: &e})
	case err != nil:
		e := err.Error()
		_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: ob.ID, Status: "failed", Error: &e})
		s.notifyFailed(ctx, ob, err)
		return err
	}
	now := s.clock.Now()
	if err := s.st.Q.SetOutboxResult(ctx, gen.SetOutboxResultParams{ID: ob.ID, Status: "sent", SentAt: &now}); err != nil {
		return err
	}
	actor, kind, action, entity := "worker", "system", "outbox.odoo", "outbox"
	after, _ := json.Marshal(map[string]any{"channel": ob.Channel, "proposal_id": ob.ProposalID})
	_ = s.st.Q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &ob.ID, After: after})
	_ = events.Notify(ctx, s.st.Pool, "proposal_changed", map[string]string{"id": pidOf(ob).String(), "channel": ob.Channel})
	return nil
}

func jitter(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(rand.Int64N(int64(hi-lo)))
}

// pidOf is the proposal of a row (zero for system rows).
func pidOf(ob gen.Outbox) uuid.UUID {
	if ob.ProposalID == nil {
		return uuid.Nil
	}
	return *ob.ProposalID
}

// ErrNotInternal refuses a system message whose target is not an internal WhatsApp group: system alerts never reach
// a dealer (CLAUDE.md §2).
var ErrNotInternal = errors.New("pesan sistem hanya ke grup WhatsApp internal")

// SystemPayload is the payload of a channel=wa_system row (ops alerts).
type SystemPayload struct {
	From string `json:"from"`
	To   string `json:"to"` // internal group JID
	Text string `json:"text"`
}

// ErrNoAutoMessage: GSI Orbit never sends a message on its own — not even an alert to the internal group.
var ErrNoAutoMessage = errors.New("pesan otomatis dimatikan — GSI Orbit tidak mengirim pesan tanpa keputusan manusia")

// sendSystem refuses every system row (an older one still queued): alerts stay inside GSI Orbit.
func (s *Sender) sendSystem(ctx context.Context, ob gen.Outbox) (time.Duration, error) {
	e := ErrNoAutoMessage.Error()
	_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: ob.ID, Status: "failed", Error: &e})
	return 0, ErrNoAutoMessage
}

// sendEdit carries out a human's edit (or delete for everyone) of a message the number sent: no pacing (nothing new
// lands in the chat), but the same approval check by the bridge, and never after WhatsApp's window.
func (s *Sender) sendEdit(ctx context.Context, ob gen.Outbox, p WAPayload) (time.Duration, error) {
	fail := func(e string) (time.Duration, error) {
		_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: ob.ID, Status: "failed", Error: &e})
		s.notifyFailed(ctx, ob, errors.New(e))
		return 0, fmt.Errorf("%w: %s", ErrRefused, e)
	}
	now := s.clock.Now()
	var err error
	if p.Kind == "delete" {
		del, ok := s.t.(wa.Deleter)
		if !ok {
			return fail("Transport WhatsApp ini tidak bisa menghapus pesan")
		}
		if p.EditSentAt == nil || now.Sub(*p.EditSentAt) > wa.DeleteWindow {
			return fail("Lewat 48 jam sejak dikirim — hapus untuk semua orang tidak tersedia lagi")
		}
		err = del.Delete(wa.WithAction(ctx, ob.ID.String()), p.From, p.To, p.EditOf)
	} else {
		ed, ok := s.t.(wa.Editor)
		if !ok {
			return fail("Transport WhatsApp ini tidak bisa mengedit pesan")
		}
		if p.EditSentAt == nil || now.Sub(*p.EditSentAt) > wa.EditWindow {
			return fail("Lewat 15 menit sejak dikirim — WhatsApp tidak menerima edit lagi")
		}
		err = ed.Edit(wa.WithAction(ctx, ob.ID.String()), p.From, p.To, p.EditOf, p.Text)
	}
	var refused *wa.SendRefused
	if errors.As(err, &refused) {
		if refused.Temporary() {
			e := "Penjaga anti-blokir: " + refused.Reason
			_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: ob.ID, Status: "pending", Error: &e})
			return refused.RetryAfter, nil
		}
		return fail("Edit ditolak: " + refused.Reason)
	}
	if err != nil {
		e := err.Error()
		_ = s.st.Q.SetOutboxResult(ctx, gen.SetOutboxResultParams{ID: ob.ID, Status: "failed", Error: &e})
		return 0, err
	}
	at := s.clock.Now()
	if err := s.st.Q.SetOutboxResult(ctx, gen.SetOutboxResultParams{ID: ob.ID, Status: "sent", SentAt: &at}); err != nil {
		return 0, err
	}
	if mid, err := uuid.Parse(p.MessageID); err == nil {
		if p.Kind == "delete" {
			err = s.st.Q.SetChatMessageDeleted(ctx, gen.SetChatMessageDeletedParams{ID: mid, DeletedAt: at})
		} else {
			err = s.st.Q.SetChatMessageBody(ctx, gen.SetChatMessageBodyParams{ID: mid, Body: p.Text, EditedAt: &at})
		}
		if err != nil {
			return 0, err
		}
	}
	if err := s.st.Q.SetProposalExecuted(ctx, pidOf(ob)); err != nil {
		return 0, err
	}
	actor, kind, action, entity := "worker", "system", "outbox."+map[bool]string{true: "deleted", false: "edited"}[p.Kind == "delete"], "outbox"
	after, _ := json.Marshal(map[string]any{"edit_of": p.EditOf, "from": p.From, "to": p.To, "proposal_id": ob.ProposalID})
	_ = s.st.Q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, EntityID: &ob.ID, After: after})
	_ = events.Notify(ctx, s.st.Pool, "chat_message", map[string]string{"thread_id": p.ThreadID})
	_ = events.Notify(ctx, s.st.Pool, "proposal_changed", map[string]string{"id": pidOf(ob).String(), "status": "executed"})
	return 0, nil
}

// sendContent sends a file, voice note or poll through a transport that can (the file comes from
// chat_outgoing_files). A transport without it, or a file gone, is a final refusal.
func (s *Sender) sendContent(ctx context.Context, p WAPayload) (string, string, error) {
	cs, ok := s.t.(wa.ContentSender)
	if !ok {
		return "", "", &wa.SendRefused{Code: "bad_content", Reason: "transport WhatsApp ini tidak bisa mengirim file, pesan suara, atau polling"}
	}
	c := wa.Content{Kind: p.Content.Kind, Mime: p.Content.Mime, FileName: p.Content.FileName, PTT: p.Content.PTT, Seconds: p.Content.Seconds, Poll: p.Content.Poll, Contact: p.Content.Contact}
	if p.Content.FileID != "" {
		fid, err := uuid.Parse(p.Content.FileID)
		if err != nil {
			return "", "", &wa.SendRefused{Code: "bad_content", Reason: "file tidak ditemukan"}
		}
		f, err := s.st.Q.GetOutgoingFile(ctx, fid)
		if err != nil {
			return "", "", &wa.SendRefused{Code: "bad_content", Reason: "file sudah tidak tersedia — kirim ulang"}
		}
		c.Data = f.Data
	}
	return cs.SendContent(ctx, p.From, p.To, p.Text, c)
}
