package wa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/events"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Drop reasons (privacy filter, 09-policies-security.md).
const (
	DropInternalDM    = "dm_internal"    // DM between two internal numbers: never stored
	DropGroupDisabled = "group_disabled" // internal group with reading switched off
	DropDuplicate     = "duplicate"      // same WhatsApp message id seen before
	DropEmpty         = "empty"
)

// Result tells what Process did with a message.
type Result struct {
	Stored   bool
	Reason   string
	ThreadID uuid.UUID
	SignalID uuid.UUID
	DealerID *uuid.UUID
}

// Ingestor turns WhatsApp events into chat threads, messages and signals, idempotently.
type Ingestor struct {
	st  *store.Store
	log *slog.Logger
	// OnDealer is called after a dealer's message is stored (the worker enqueues metrics.recompute).
	OnDealer func(ctx context.Context, dealerID uuid.UUID)
}

// NewIngestor builds the pipeline.
func NewIngestor(st *store.Store, log *slog.Logger) *Ingestor { return &Ingestor{st: st, log: log} }

// MaskNumber renders a number the way the UI shows unknown senders: "+62 822-••••-3310".
func MaskNumber(n string) string {
	d := Digits(n)
	if len(d) < 9 {
		return "+" + d
	}
	return fmt.Sprintf("+%s %s-••••-%s", d[:2], d[2:5], d[len(d)-4:])
}

// Process stores one message (or drops it by the privacy rules).
func (in *Ingestor) Process(ctx context.Context, m Message) (Result, error) {
	if strings.TrimSpace(m.Text) == "" {
		return Result{Reason: DropEmpty}, nil
	}
	var res Result
	err := in.st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		account := Digits(m.Account)
		var salesID *uuid.UUID
		if s, err := q.GetSalesByNumber(ctx, &account); err == nil {
			salesID = &s.ID
		}
		internal := func(n string) bool {
			v, _ := q.IsInternalNumber(ctx, Digits(n))
			return v
		}

		kind := "wa"
		var thread gen.ChatThread
		var dealerID, contactID, groupID *uuid.UUID
		threadKind, title, subtitle := "", "", ""
		fromInternal := internal(m.FromNumber)
		if m.IsGroup {
			kind = "wa_group"
			g, err := q.GetWAGroupByJID(ctx, &m.ChatJID)
			if errors.Is(err, pgx.ErrNoRows) {
				name, gk := m.GroupName, "internal"
				g, err = q.UpsertWAGroup(ctx, gen.UpsertWAGroupParams{Jid: &m.ChatJID, Name: &name, Kind: &gk, ReadEnabled: true})
			}
			if err != nil {
				return err
			}
			if deref(g.Kind) == "internal" && !g.ReadEnabled {
				res.Reason = DropGroupDisabled
				return nil
			}
			groupID, threadKind, title = &g.ID, "group", deref(g.Name)
			subtitle = "Grup internal"
			if deref(g.Kind) == "external" {
				subtitle = "Grup eksternal"
			}
		} else {
			remote := NumberOfJID(m.ChatJID)
			if internal(remote) {
				res.Reason = DropInternalDM
				return nil
			}
			if c, err := q.FindContactByNumber(ctx, &remote); err == nil {
				dealerID, contactID, threadKind = c.DealerID, &c.ID, "dealer"
				title = deref(c.Name) + " · " + shortDealer(c.DealerName)
				subtitle = c.DealerName + " · " + deref(c.Role)
			} else if errors.Is(err, pgx.ErrNoRows) {
				threadKind, title, subtitle = "new", MaskNumber(remote), m.FromName
			} else {
				return err
			}
		}

		thread, err := q.GetThreadBySalesJID(ctx, gen.GetThreadBySalesJIDParams{WaJid: &m.ChatJID, SalesID: salesID})
		if errors.Is(err, pgx.ErrNoRows) {
			thread, err = q.InsertThread(ctx, gen.InsertThreadParams{Kind: &threadKind, DealerID: dealerID, GroupID: groupID, ContactID: contactID, WaJid: &m.ChatJID, Title: &title, Subtitle: &subtitle, SalesID: salesID, LastMessageAt: &m.Time})
		}
		if err != nil {
			return fmt.Errorf("thread: %w", err)
		}
		res.ThreadID = thread.ID
		dir := "in"
		if m.FromMe {
			dir = "out"
		}
		id := m.ID
		msgID, err := q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &thread.ID, WaMsgID: &id, Direction: &dir, FromNumber: &m.FromNumber, FromName: &m.FromName, Body: &m.Text, SentAt: m.Time, Status: map[bool]string{true: "sent", false: "received"}[m.FromMe], Internal: fromInternal && m.IsGroup})
		if errors.Is(err, pgx.ErrNoRows) {
			res.Reason = DropDuplicate
			return nil
		}
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"thread": thread.ID, "direction": dir, "text": m.Text, "from_name": m.FromName, "from_number": m.FromNumber, "account": account, "group": m.IsGroup, "internal": fromInternal})
		summary := m.Text
		sp := gen.UpsertSignalParams{Kind: kind, OccurredAt: m.Time, DedupeKey: "wa:" + m.ID, Summary: &summary, Payload: payload, SalesID: salesID}
		if threadKind == "dealer" {
			sp.DealerID, sp.ContactID = dealerID, contactID
		}
		sid, err := q.UpsertSignal(ctx, sp)
		if err != nil {
			return err
		}
		if err := q.SetMessageSignal(ctx, gen.SetMessageSignalParams{ID: msgID, SignalID: &sid}); err != nil {
			return err
		}
		unread := int32(0)
		if !m.FromMe {
			unread = 1
		}
		if err := q.TouchThread(ctx, gen.TouchThreadParams{ID: thread.ID, LastMessageAt: &m.Time, Unread: unread}); err != nil {
			return err
		}
		if contactID != nil && !m.FromMe {
			if err := q.TouchContact(ctx, gen.TouchContactParams{ID: *contactID, LastInteractionAt: &m.Time}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "select pg_notify('chat_message', $1)", fmt.Sprintf(`{"thread_id":%q}`, thread.ID)); err != nil {
			return err
		}
		res.Stored, res.SignalID = true, sid
		if threadKind == "dealer" {
			res.DealerID = dealerID
		}
		return nil
	})
	if err == nil && res.DealerID != nil && in.OnDealer != nil {
		in.OnDealer(ctx, *res.DealerID)
	}
	return res, err
}

// ProcessStatus records a number's connection state and tells the browsers.
func (in *Ingestor) ProcessStatus(ctx context.Context, s Status) error {
	acc := Digits(s.Account)
	var jid *string
	if s.JID != "" {
		jid = &s.JID
	}
	if s.QR != "" {
		if err := in.st.Q.SetWANumberQR(ctx, gen.SetWANumberQRParams{WaNumber: acc, Qr: &s.QR}); err != nil {
			return err
		}
	} else if err := in.st.Q.SetWANumberState(ctx, gen.SetWANumberStateParams{WaNumber: acc, State: s.State, Jid: jid}); err != nil {
		return err
	}
	return events.Notify(ctx, in.st.Pool, "wa_status", map[string]string{"account": acc, "state": s.State})
}

// Run consumes a transport's events until ctx ends.
func (in *Ingestor) Run(ctx context.Context, t Transport) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-t.Events():
			switch {
			case e.Message != nil:
				if r, err := in.Process(ctx, *e.Message); err != nil {
					in.log.Error("wa ingest", "err", err, "msg", e.Message.ID)
				} else if !r.Stored {
					in.log.Debug("wa dropped", "reason", r.Reason, "msg", e.Message.ID)
				}
			case e.Status != nil:
				if err := in.ProcessStatus(ctx, *e.Status); err != nil {
					in.log.Error("wa status", "err", err)
				}
			}
		}
	}
}

func shortDealer(name string) string {
	for _, p := range []string{"PT ", "CV ", "UD ", "Toko "} {
		name = strings.TrimPrefix(name, p)
	}
	f := strings.Fields(name)
	if len(f) > 2 {
		f = f[:2]
	}
	return strings.Join(f, " ")
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
