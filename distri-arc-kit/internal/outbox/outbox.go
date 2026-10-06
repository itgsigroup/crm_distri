// Package outbox is the only way anything leaves Distri ARC: rows are written for approved proposals and the
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
	Kind      string `json:"kind"`       // proposal kind; "reply" = a human answering in a conversation
	MessageID string `json:"message_id"` // pending chat_messages row shown in the thread
	ThreadID  string `json:"thread_id"`
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
}

// WithOdoo lets the sender carry out Odoo rows (SO drafts, notes); nil keeps them pending.
func (s *Sender) WithOdoo(src odoo.Source) *Sender { s.odoo = src; return s }

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
	if ob.Status == "sent" || ob.Status == "shadow" {
		return 0, nil // idempotent; shadow rows are never delivered
	}
	if pol, err := policy.Load(ctx, s.st.Q); err == nil && pol.Pilot.Shadow() && ob.Channel != "wa_system" {
		// a row queued before the pilot switched to shadow mode: keep it, do not deliver
		_, err := s.st.Q.ShadowOutbox(ctx, ob.ProposalID)
		return 0, err
	}
	if ob.Channel == "wa_system" {
		return 0, s.sendSystem(ctx, ob)
	}
	if ob.Channel != "wa" {
		return 0, s.sendOdoo(ctx, ob)
	}
	var p WAPayload
	if err := json.Unmarshal(ob.Payload, &p); err != nil {
		return 0, err
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
	msgID, sendErr := s.t.Send(wa.WithAction(ctx, id.String()), p.From, p.To, p.Text)
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
			_ = s.st.Q.UpdateChatMessageSent(ctx, gen.UpdateChatMessageSentParams{ID: mid, WaMsgID: &msgID, Status: "sent", SentAt: sent})
		}
	}
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

// sendSystem delivers an ops alert to an internal group. No proposal, no send window: the group is GSI staff.
func (s *Sender) sendSystem(ctx context.Context, ob gen.Outbox) error {
	var p SystemPayload
	if err := json.Unmarshal(ob.Payload, &p); err != nil {
		return err
	}
	g, err := s.st.Q.GetWAGroupByJID(ctx, &p.To)
	if err != nil || g.Kind == nil || *g.Kind != "internal" {
		e := ErrNotInternal.Error()
		_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: ob.ID, Status: "failed", Error: &e})
		return ErrNotInternal
	}
	if _, err := s.t.Send(ctx, p.From, p.To, p.Text); err != nil {
		e := err.Error()
		_ = s.st.Q.SetOutboxError(ctx, gen.SetOutboxErrorParams{ID: ob.ID, Status: "failed", Error: &e})
		return err
	}
	now := s.clock.Now()
	return s.st.Q.SetOutboxResult(ctx, gen.SetOutboxResultParams{ID: ob.ID, Status: "sent", SentAt: &now})
}
