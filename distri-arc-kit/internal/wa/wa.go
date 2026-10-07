// Package wa connects the sales WhatsApp numbers (ADR 0002): a Transport interface with three implementations
// (fake for development and tests, whatsmeow linked devices, Cloud API) and the ingest pipeline that turns
// events into chat threads, messages and signals. Sending is only ever triggered by the outbox job for an
// approved proposal.
package wa

import (
	"context"
	"strings"
	"time"
)

// Message is one WhatsApp message seen by a paired sales number (Account).
type Message struct {
	ID         string    `json:"id"`
	Account    string    `json:"account"`     // sales number that saw the message (E.164 digits)
	ChatJID    string    `json:"chat_jid"`    // remote chat: a person or a group
	FromNumber string    `json:"from_number"` // sender digits
	FromName   string    `json:"from_name"`   // push name / profile
	IsGroup    bool      `json:"is_group"`
	GroupName  string    `json:"group_name"`
	FromMe     bool      `json:"from_me"`
	Text       string    `json:"text"`
	Time       time.Time `json:"time"`
	ChatName   string    `json:"chat_name,omitempty"` // contact name on the linked phone (address book / push / business)
	History    bool      `json:"history,omitempty"`   // synced history after linking, not a live message
}

// Status is the connection state of one paired number.
type Status struct {
	Account string `json:"account"`
	State   string `json:"state"` // unpaired | pairing | connected | disconnected | logged_out
	JID     string `json:"jid,omitempty"`
	QR      string `json:"qr,omitempty"`
}

// Event is either a message or a status change.
type Event struct {
	Message *Message
	Status  *Status
}

// Transport is a WhatsApp connection for one or more sales numbers.
type Transport interface {
	Name() string
	// Start connects every known number and begins emitting Events.
	Start(ctx context.Context) error
	// Pair starts linking a sales number; the QR code (or empty for transports without QR) comes back here and
	// as a Status event while it refreshes.
	Pair(ctx context.Context, account string) (string, error)
	// Status lists the state of every number.
	Status(ctx context.Context) []Status
	// Send delivers a text from account to a chat and returns the WhatsApp message id.
	Send(ctx context.Context, account, chatJID, text string) (string, error)
	Events() <-chan Event
}

// Profile is what WhatsApp shows about a number: the verified WA Business name, category and address.
type Profile struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Address  string `json:"address"`
	Business bool   `json:"business"`
}

// ProfileReader is implemented by transports that can read a number's WhatsApp Business profile (AI Prospek
// identification of inbound numbers; never used for numbers that did not write first).
type ProfileReader interface {
	Profile(ctx context.Context, account, jid string) (Profile, error)
}

// Digits normalises a phone number to E.164 digits (62…): "+62 812-3450-4471" → "6281234504471", "0812…" → "62812…".
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if strings.HasPrefix(d, "0") {
		d = "62" + d[1:]
	}
	return d
}

// UserJID returns the personal chat JID of a number.
func UserJID(number string) string { return Digits(number) + "@s.whatsapp.net" }

// NumberOfJID extracts the number of a personal JID ("62812…@s.whatsapp.net" → "62812…").
func NumberOfJID(jid string) string {
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		jid = jid[:i]
	}
	if i := strings.IndexByte(jid, ':'); i >= 0 {
		jid = jid[:i]
	}
	return Digits(jid)
}

// IsGroupJID reports whether a JID is a group chat.
func IsGroupJID(jid string) bool { return strings.HasSuffix(jid, "@g.us") }

// CodePairer is implemented by transports that can link with an 8-character code typed on the phone
// (WhatsApp → Perangkat tertaut → Tautkan dengan nomor telepon) instead of scanning a QR code.
type CodePairer interface {
	PairCode(ctx context.Context, account string) (string, error)
}

// PairCodePrefix marks a pairing code stored where the QR code normally is (wa_numbers.qr).
const PairCodePrefix = "code:"

// Unpairer is implemented by transports that can log a linked device out (Pengaturan → WhatsApp → Lepas).
type Unpairer interface {
	Unpair(ctx context.Context, account string) error
}

type actionKey struct{}

// WithAction carries the outbox row id of an approved proposal to the transport, which the Baileys bridge
// confirms with Distri ARC before it sends (no row, no send).
func WithAction(ctx context.Context, outboxID string) context.Context {
	return context.WithValue(ctx, actionKey{}, outboxID)
}

// ActionFrom is the outbox id set by WithAction ("" when none).
func ActionFrom(ctx context.Context) string {
	s, _ := ctx.Value(actionKey{}).(string)
	return s
}

// SendRefused is the anti-ban guard saying no. With RetryAfter it is pacing (quiet hours, hourly or daily cap,
// warm-up of a new device, gap to the same chat): the outbox job waits. Without it the refusal is final for this
// message (contact never wrote first, opted out, identical text to many chats): a person must change the plan.
type SendRefused struct {
	Code       string
	Reason     string
	RetryAfter time.Duration
}

func (e *SendRefused) Error() string { return e.Reason }

// Temporary reports whether the message may go out later unchanged.
func (e *SendRefused) Temporary() bool { return e.RetryAfter > 0 }
