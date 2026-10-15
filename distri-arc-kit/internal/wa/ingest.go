package wa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/commitment"
	"distri-arc/internal/events"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Drop reasons (privacy filter, 09-policies-security.md).
const (
	DropInternalDM    = "dm_internal"    // DM between two internal numbers: never stored
	DropGroupDisabled = "group_disabled" // internal group with reading switched off
	DropPersonal      = "personal"       // a number marked personal (pribadi): never kept in the CRM (ADR 0035)
	DropDuplicate     = "duplicate"      // same WhatsApp message id seen before
	DropEmpty         = "empty"
	DropEditUnknown   = "edit_unknown" // an edit of a message GSI Orbit never stored (dropped by the rules above)
)

// Result tells what Process did with a message.
type Result struct {
	Stored    bool
	Reason    string
	ThreadID  uuid.UUID
	SignalID  uuid.UUID
	DealerID  *uuid.UUID
	NewNumber string // an unknown inbound number (identification)
}

// Ingestor turns WhatsApp events into chat threads, messages and signals, idempotently.
type Ingestor struct {
	st  *store.Store
	log *slog.Logger
	// OnDealer is called after a dealer's message is stored (the worker enqueues metrics.recompute).
	OnDealer func(ctx context.Context, dealerID uuid.UUID)
	// OnNewNumber is called after an unknown number wrote to a sales number (the worker enqueues identify.number).
	OnNewNumber func(ctx context.Context, number string)
	// Transport names numbers registered when a new link connects (fake | baileys).
	Transport string
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
	if m.RevokeOf != "" {
		return in.revoke(ctx, m)
	}
	if strings.TrimSpace(m.Text) == "" {
		return Result{Reason: DropEmpty}, nil
	}
	if m.EditOf != "" {
		return in.edit(ctx, m)
	}
	var media json.RawMessage
	if m.Media != nil {
		media, _ = json.Marshal(m.Media)
	}
	var res Result
	err := in.st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		account := Digits(m.Account)
		var salesID *uuid.UUID
		if n, err := q.GetWANumber(ctx, account); err == nil && n.SalesID != nil {
			salesID = n.SalesID // the number's owner (a sales may link several numbers)
		} else if s, err := q.GetSalesByNumber(ctx, &account); err == nil {
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
		if m.IsGroup && !fromInternal && !m.FromMe {
			// in a group, any GSI number (linked, a user's, a sales') is our side
			n, err := q.CountOutsiders(ctx, []string{Digits(m.FromNumber)})
			if err != nil {
				return err
			}
			fromInternal = n == 0 && Digits(m.FromNumber) != ""
		}
		if m.IsGroup {
			kind = "wa_group"
			g, err := q.GetWAGroupByJID(ctx, &m.ChatJID)
			// someone outside GSI in the group — a member, a hidden member (WhatsApp shows a LID, not the number), or
			// the sender — makes it a customer group (ADR 0033)
			outside := func() (bool, error) {
				nums := append([]string{}, m.GroupMembers...)
				if !m.FromMe {
					nums = append(nums, Digits(m.FromNumber))
				}
				n, err := q.CountOutsiders(ctx, nums)
				return n > 0 || m.GroupHidden > 0, err
			}
			if errors.Is(err, pgx.ErrNoRows) {
				name, gk := m.GroupName, "internal"
				if !InternalGroupName(name) {
					if out, err := outside(); err != nil {
						return err
					} else if out {
						gk = "external"
					}
				}
				var members *int32
				if n := len(m.GroupMembers) + m.GroupHidden; n > 0 {
					c := int32(n)
					members = &c
				}
				g, err = q.UpsertWAGroup(ctx, gen.UpsertWAGroupParams{Jid: &m.ChatJID, Name: &name, Kind: &gk, Members: members, ReadEnabled: true})
			} else if err == nil && deref(g.Kind) == "internal" && !g.KindManual && !InternalGroupName(deref(g.Name)) {
				// a group that started internal: someone outside GSI joined or wrote
				if out, err := outside(); err != nil {
					return err
				} else if out {
					if err := q.MarkCustomerGroup(ctx, g.ID); err != nil {
						return err
					}
					g.Kind = strp("external")
				}
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
				subtitle = "Grup pelanggan"
			}
		} else {
			remote := NumberOfJID(m.ChatJID)
			if internal(remote) {
				res.Reason = DropInternalDM
				return nil
			}
			if ct, err := q.GetContactType(ctx, remote); err == nil && ct == "personal" {
				res.Reason = DropPersonal
				return nil
			}
			if c, err := q.FindContactByNumber(ctx, &remote); err == nil {
				dealerID, contactID, threadKind = c.DealerID, &c.ID, "dealer"
				title = deref(c.Name) + " · " + ShortDealer(c.DealerName)
				subtitle = c.DealerName + " · " + deref(c.Role)
			} else if errors.Is(err, pgx.ErrNoRows) {
				threadKind, title, subtitle = "new", MaskNumber(remote), "Nomor baru"
				if name := contactName(m); name != "" {
					title, subtitle = name, MaskNumber(remote)
				}
			} else {
				return err
			}
		}

		thread, err := q.GetThreadByAccountJID(ctx, gen.GetThreadByAccountJIDParams{WaJid: &m.ChatJID, Account: &account})
		if errors.Is(err, pgx.ErrNoRows) {
			thread, err = q.InsertThread(ctx, gen.InsertThreadParams{Kind: &threadKind, DealerID: dealerID, GroupID: groupID, ContactID: contactID, WaJid: &m.ChatJID, Title: &title, Subtitle: &subtitle, SalesID: salesID, LastMessageAt: &m.Time, Account: &account})
		}
		if err != nil {
			return fmt.Errorf("thread: %w", err)
		}
		if name := strings.TrimSpace(m.ChatName); name != "" && !m.IsGroup && thread.PhoneName == nil {
			// what the phone calls this contact, for search (a dealer's title stays its master name)
			if err := q.SetThreadPhoneName(ctx, gen.SetThreadPhoneNameParams{ID: thread.ID, Name: name}); err != nil {
				return err
			}
		}
		if name := contactName(m); threadKind == "new" && name != "" && deref(thread.Title) != name {
			if err := q.NameNewThread(ctx, gen.NameNewThreadParams{ID: thread.ID, Title: &name}); err != nil {
				return err
			}
		}
		res.ThreadID = thread.ID
		dir := "in"
		if m.FromMe {
			dir = "out"
		}
		id := m.ID
		msgID, err := q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &thread.ID, WaMsgID: &id, Direction: &dir, FromNumber: &m.FromNumber, FromName: &m.FromName, Body: &m.Text, Media: media, QuotedWaID: strp(m.QuotedID), SentAt: m.Time, Status: map[bool]string{true: "sent", false: "received"}[m.FromMe], Internal: fromInternal && m.IsGroup})
		if errors.Is(err, pgx.ErrNoRows) {
			res.Reason = DropDuplicate
			if m.Media.Viewable() {
				// stored before media references were kept: a history sync makes its file viewable
				if err := q.SetChatMessageMediaRef(ctx, gen.SetChatMessageMediaRefParams{Media: media, WaMsgID: id}); err != nil {
					return err
				}
			}
			if m.IsGroup {
				// another GSI number in the group stored it first: this number's thread still moves (time, unread),
				// its messages are read from every thread of the group
				unread := int32(0)
				if !m.FromMe && !m.History {
					unread = 1
				}
				return q.TouchThread(ctx, gen.TouchThreadParams{ID: thread.ID, LastMessageAt: &m.Time, Unread: unread})
			}
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
		if !m.FromMe && !m.History { // synced history arrives read: only live messages count as new
			unread = 1
		}
		if err := q.TouchThread(ctx, gen.TouchThreadParams{ID: thread.ID, LastMessageAt: &m.Time, Unread: unread}); err != nil {
			return err
		}
		if m.FromMe && !m.History && !m.IsGroup {
			// answered from the phone: the chat was read there, as WhatsApp shows it
			if err := q.MarkThreadRead(ctx, thread.ID); err != nil {
				return err
			}
		}
		if contactID != nil && !m.FromMe {
			if err := q.TouchContact(ctx, gen.TouchContactParams{ID: *contactID, LastInteractionAt: &m.Time}); err != nil {
				return err
			}
		}
		if threadKind == "dealer" && dir == "in" && dealerID != nil {
			if _, err := commitment.FromInbound(ctx, q, commitment.Inbound{DealerID: *dealerID, ThreadID: thread.ID, MessageID: msgID, SignalID: sid, Text: m.Text, At: m.Time}); err != nil {
				return fmt.Errorf("commitment: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, "select pg_notify('chat_message', $1)", fmt.Sprintf(`{"thread_id":%q}`, thread.ID)); err != nil {
			return err
		}
		res.Stored, res.SignalID = true, sid
		if threadKind == "dealer" {
			res.DealerID = dealerID
		}
		if threadKind == "new" && dir == "in" && !m.History { // no profile lookups for old chats right after linking
			res.NewNumber = Digits(strings.SplitN(m.ChatJID, "@", 2)[0])
		}
		return nil
	})
	if err == nil && res.NewNumber != "" && in.OnNewNumber != nil {
		in.OnNewNumber(ctx, res.NewNumber)
	}
	if err == nil && res.DealerID != nil && in.OnDealer != nil {
		in.OnDealer(ctx, *res.DealerID)
	}
	return res, err
}

// edit applies a WhatsApp edit (the customer's, one made on the phone, or one sent from Chat) to the stored message.
// Idempotent: the same edit twice leaves the same text.
func (in *Ingestor) edit(ctx context.Context, m Message) (Result, error) {
	row, err := in.st.Q.EditChatMessageByWAID(ctx, gen.EditChatMessageByWAIDParams{Body: strings.TrimSpace(m.Text), EditedAt: m.Time, WaMsgID: m.EditOf})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{Reason: DropEditUnknown}, nil
	}
	if err != nil {
		return Result{}, err
	}
	res := Result{Stored: true}
	if row.ThreadID != nil {
		res.ThreadID = *row.ThreadID
		err = events.Notify(ctx, in.st.Pool, "chat_message", map[string]string{"thread_id": row.ThreadID.String()})
	}
	return res, err
}

// revoke applies a WhatsApp delete-for-everyone to the stored message ("Pesan ini dihapus"); the text stays for the
// audit trail. Idempotent.
func (in *Ingestor) revoke(ctx context.Context, m Message) (Result, error) {
	row, err := in.st.Q.DeleteChatMessageByWAID(ctx, gen.DeleteChatMessageByWAIDParams{DeletedAt: m.Time, WaMsgID: m.RevokeOf})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{Reason: DropEditUnknown}, nil
	}
	if err != nil {
		return Result{}, err
	}
	res := Result{Stored: true}
	if row.ThreadID != nil {
		res.ThreadID = *row.ThreadID
		err = events.Notify(ctx, in.st.Pool, "chat_message", map[string]string{"thread_id": row.ThreadID.String()})
	}
	return res, err
}

// ProcessStatus records a number's connection state and tells the browsers. A new link (Chat → + Nomor) is pending
// under its session id until WhatsApp reports the linked phone's number; then the number is registered under that
// session and waits for a person to give it to a user.
func (in *Ingestor) ProcessStatus(ctx context.Context, s Status) error {
	acc := Digits(s.Account)
	sess := s.Session
	if sess == "" {
		sess = acc
	}
	var jid *string
	if s.JID != "" {
		jid = &s.JID
	}
	row, err := in.st.Q.GetWANumberBySession(ctx, sess)
	known := err == nil
	if !known && s.Session == "" && acc != "" { // transports that name sessions after the number
		if r, err := in.st.Q.GetWANumber(ctx, acc); err == nil {
			row, known = r, true
		}
	}
	switch {
	case s.State == "connected" && acc != "" && (!known || row.WaNumber != acc):
		label := ""
		if n, err := in.st.Q.GetWANumber(ctx, acc); err == nil && n.Label != nil {
			label = *n.Label
		}
		transport := in.Transport
		if transport == "" {
			transport = "baileys"
		}
		if err := in.st.Q.LinkWANumber(ctx, gen.LinkWANumberParams{WaNumber: acc, SessionID: sess, Label: &label, Transport: transport, Jid: jid}); err != nil {
			return err
		}
		if err := in.st.Q.SetWALink(ctx, gen.SetWALinkParams{SessionID: sess, State: "connected", WaNumber: &acc}); err != nil {
			return err
		}
	case known:
		acc = row.WaNumber
		if s.QR != "" {
			if row.State == "connected" {
				return nil // a late QR for a device that is already linked
			}
			if err := in.st.Q.SetWANumberQR(ctx, gen.SetWANumberQRParams{WaNumber: acc, Qr: &s.QR}); err != nil {
				return err
			}
		} else if err := in.st.Q.SetWANumberState(ctx, gen.SetWANumberStateParams{WaNumber: acc, State: s.State, Jid: jid}); err != nil {
			return err
		}
	default: // a new link still pairing (or given up)
		l, err := in.st.Q.GetWALink(ctx, sess)
		if err != nil || l.State == "connected" {
			return nil
		}
		state, qr := "pairing", (*string)(nil)
		var msg *string
		if s.QR != "" {
			qr = &s.QR
		} else {
			qr = l.Qr
		}
		if s.State == "disconnected" || s.State == "logged_out" || s.State == "unpaired" {
			state, qr = "failed", nil
			m := "Waktu menautkan habis atau dibatalkan di HP — mulai lagi"
			msg = &m
		}
		if err := in.st.Q.SetWALink(ctx, gen.SetWALinkParams{SessionID: sess, State: state, Qr: qr, Error: msg}); err != nil {
			return err
		}
	}
	return events.Notify(ctx, in.st.Pool, "wa_status", map[string]string{"account": acc, "session": sess, "state": s.State})
}

// ingestLanes: messages are stored in parallel, one lane per chat (FNV of the chat JID), so a chat's messages keep
// their order — an edit or a deletion after its message — while six numbers syncing history do not wait on one
// another (a single lane made history batches time out at the bridge, 14 Oct).
const ingestLanes = 4

// Run consumes a transport's events until ctx ends.
func (in *Ingestor) Run(ctx context.Context, t Transport) {
	var lanes [ingestLanes]chan Event
	for i := range lanes {
		lanes[i] = make(chan Event, 64)
		go in.lane(ctx, lanes[i])
	}
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-t.Events():
			l := lanes[0] // statuses keep their order on one lane
			if e.Message != nil {
				l = lanes[laneOf(e.Message.ChatJID)]
			}
			select {
			case l <- e:
			case <-ctx.Done():
				return
			}
		}
	}
}

func laneOf(chat string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(chat))
	return int(h.Sum32() % ingestLanes)
}

func (in *Ingestor) lane(ctx context.Context, ch <-chan Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
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

// contactName is how the linked phone knows the other side of a direct chat: the saved contact or business name,
// else the sender's own WhatsApp name on an inbound message.
func contactName(m Message) string {
	if m.IsGroup {
		return ""
	}
	if n := strings.TrimSpace(m.ChatName); n != "" {
		return n
	}
	if !m.FromMe {
		return strings.TrimSpace(m.FromName)
	}
	return ""
}

// internalGroupName: a group named for internal work stays internal whoever is in it (the same words as migration 0032).
var internalGroupName = regexp.MustCompile(`(?i)internal|gudang|karyawan|staff|hrd|absensi`)

// InternalGroupName reports whether a group's name says it is for GSI's own work.
func InternalGroupName(name string) bool { return internalGroupName.MatchString(name) }

func ShortDealer(name string) string {
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

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
