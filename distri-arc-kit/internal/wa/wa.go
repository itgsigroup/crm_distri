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
