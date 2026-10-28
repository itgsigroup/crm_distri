// Package wa connects the sales WhatsApp numbers (ADR 0002): a Transport interface with three implementations
// (fake for development and tests, whatsmeow linked devices, Cloud API) and the ingest pipeline that turns
// events into chat threads, messages and signals. Sending is only ever triggered by the outbox job for an
// approved proposal.
package wa

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Message is one WhatsApp message seen by a paired sales number (Account).
type Message struct {
	ID         string `json:"id"`
	Account    string `json:"account"`     // sales number that saw the message (E.164 digits)
	ChatJID    string `json:"chat_jid"`    // remote chat: a person or a group
	FromNumber string `json:"from_number"` // sender digits
	FromName   string `json:"from_name"`   // push name / profile
	IsGroup    bool   `json:"is_group"`
	GroupName  string `json:"group_name"`
	// GroupMembers: the group's members' numbers (digits) when the transport knows them
	GroupMembers []string `json:"group_members,omitempty"`
	// GroupHidden: members whose number WhatsApp hides (LID only) — not GSI\'s numbers as far as anyone can tell
	GroupHidden int       `json:"group_hidden,omitempty"`
	FromMe      bool      `json:"from_me"`
	Text        string    `json:"text"`
	Time        time.Time `json:"time"`
	ChatName    string    `json:"chat_name,omitempty"` // contact name on the linked phone (address book / push / business)
	History     bool      `json:"history,omitempty"`   // synced history after linking, not a live message
	Media       *Media    `json:"media,omitempty"`
	// EditOf: this message is a WhatsApp edit of the message with that id; Text replaces its text.
	EditOf string `json:"edit_of,omitempty"`
	// QuotedID: the message this one replies to (WhatsApp "Balas").
	QuotedID string `json:"quoted_id,omitempty"`
	// RevokeOf: this message deletes the message with that id for everyone ("Pesan ini dihapus").
	RevokeOf string `json:"revoke_of,omitempty"`
	// Mentions: the group members the text tags ("@Budi").
	Mentions []Mention `json:"mentions,omitempty"`
	// mirror: the other linked GSI number's copy of a 1:1 message, made by GSI Orbit (never mirrored again)
	mirror bool
}

// Quote is the message a reply sent from Chat quotes: its WhatsApp key and text.
type Quote struct {
	ID          string `json:"id"`
	FromMe      bool   `json:"from_me"`
	Participant string `json:"participant,omitempty"` // a group member's JID (groups only)
	Text        string `json:"text"`
}

// Mention is a member tagged in a message ("@Budi"): the body carries "@<Token>" (a number, or a LID WhatsApp shows
// in groups that hide numbers), Number is the member's phone number when known, Name how Chat shows them.
type Mention struct {
	Token  string `json:"token"`
	Number string `json:"number,omitempty"`
	Name   string `json:"name,omitempty"`
}

type mentionsKey struct{}

// WithMentions makes the send tag these numbers (the text carries "@<number>" for each, as WhatsApp sends tags).
func WithMentions(ctx context.Context, numbers []string) context.Context {
	if len(numbers) == 0 {
		return ctx
	}
	return context.WithValue(ctx, mentionsKey{}, numbers)
}

// MentionsFrom is the numbers set by WithMentions.
func MentionsFrom(ctx context.Context) []string {
	ms, _ := ctx.Value(mentionsKey{}).([]string)
	return ms
}

var mentionToken = regexp.MustCompile(`@(\d{5,20})`)

// NamedMentions is a text with its tags as people read them: "@6281…" becomes "@Budi" (mentions as stored).
func NamedMentions(text string, mentions []byte) string {
	if len(mentions) == 0 || !strings.Contains(text, "@") {
		return text
	}
	var ms []Mention
	if json.Unmarshal(mentions, &ms) != nil {
		return text
	}
	return mentionToken.ReplaceAllStringFunc(text, func(tag string) string {
		for _, m := range ms {
			if m.Token == tag[1:] && m.Name != "" {
				return "@" + m.Name
			}
		}
		return tag
	})
}

// MentionTokens lists the "@<digits>" tags in a text, once each, in order.
func MentionTokens(text string) []string {
	var out []string
	for _, m := range mentionToken.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

type liveKey struct{}

// WithLive marks the send as a person typing in Chat right now (ADR 0064): the bridge counts it against its live
// ceiling, not the volume caps meant for follow-ups.
func WithLive(ctx context.Context) context.Context { return context.WithValue(ctx, liveKey{}, true) }

// LiveFrom reports WithLive.
func LiveFrom(ctx context.Context) bool {
	v, _ := ctx.Value(liveKey{}).(bool)
	return v
}

type quoteKey struct{}

// WithQuote makes the send a reply to q (the transport shows it quoted, like WhatsApp "Balas").
func WithQuote(ctx context.Context, q *Quote) context.Context {
	if q == nil || q.ID == "" {
		return ctx
	}
	return context.WithValue(ctx, quoteKey{}, q)
}

// QuoteFrom is the quote set by WithQuote (nil when none).
func QuoteFrom(ctx context.Context) *Quote {
	q, _ := ctx.Value(quoteKey{}).(*Quote)
	return q
}

// Media describes a message's photo, video, voice note, document or sticker. The content is never stored: Ref is
// the transport's reference to fetch it from WhatsApp when a person opens it (ADR 0029).
type Media struct {
	Kind     string `json:"kind"` // image | video | gif | audio | document | sticker | location | contact | call | system
	Mime     string `json:"mime,omitempty"`
	FileName string `json:"file_name,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Seconds  int    `json:"seconds,omitempty"`
	PTT      bool   `json:"ptt,omitempty"` // a voice note recorded in WhatsApp (not an audio file)
	Ref      string `json:"ref,omitempty"`
	// Call: how a WhatsApp call (kind "call", ADR 0062) ended — missed | answered | rejected; Seconds is the talk
	Call  string `json:"call,omitempty"`
	Video bool   `json:"video,omitempty"`
	// System: a group's own line (kind "system", ADR 0063) — create | subject | icon | description | add | join |
	// remove | leave | promote | demote; the Text says it as WhatsApp does ("Anda menambahkan Budi")
	System string `json:"system,omitempty"`
}

// IsCall reports a WhatsApp call record (no file; the phone carried the call).
func (m *Media) IsCall() bool { return m != nil && m.Kind == "call" }

// IsSystem reports a group's own line (ADR 0063): nobody wrote it, so it is no message to answer, read or analyse.
func (m *Media) IsSystem() bool { return m != nil && m.Kind == "system" }

// Viewable reports whether the file can be fetched to show in Chat.
func (m *Media) Viewable() bool {
	return m != nil && m.Ref != "" && m.Kind != "location" && m.Kind != "contact"
}

// Status is the connection state of one paired number.
type Status struct {
	Account string `json:"account"`           // the number (empty while a new link is still pairing)
	Session string `json:"session,omitempty"` // the transport's session id (a new link gets one before its number is known)
	State   string `json:"state"`             // unpaired | pairing | connected | disconnected | logged_out
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
	About    string `json:"about"` // the WhatsApp "Info" line
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

// Linker is implemented by transports that link a new device before its number is known (Chat → + Nomor: scan, then
// pick the user). The session id is ours; the number arrives with the "connected" status. code: link with a code
// typed on the phone, which needs the phone number up front.
type Linker interface {
	Link(ctx context.Context, session, phone string, code bool) (string, error)
}

// CodePairer is implemented by transports that can link with an 8-character code typed on the phone
// (WhatsApp → Perangkat tertaut → Tautkan dengan nomor telepon) instead of scanning a QR code.
type CodePairer interface {
	PairCode(ctx context.Context, account string) (string, error)
}

// PairCodePrefix marks a pairing code stored where the QR code normally is (wa_numbers.qr).
const PairCodePrefix = "code:"

// LinkingPrefix marks a device whose QR was just scanned (or code typed): the phone is logging in and the first
// sync is starting; the number follows with the "connected" status.
const LinkingPrefix = "linking:"

// Editor is implemented by transports that can edit a text the number sent (WhatsApp: within 15 minutes). Like Send,
// the outbox id of an approved edit travels in ctx (WithAction).
type Editor interface {
	Edit(ctx context.Context, account, chatJID, waMsgID, text string) error
}

// Content is what a send from Chat carries besides text: a file (photo, video, document), a voice note, a poll, or a
// contact card.
type Content struct {
	Kind     string       `json:"kind"` // image | video | document | audio | poll | contact
	Mime     string       `json:"mime,omitempty"`
	FileName string       `json:"file_name,omitempty"`
	Data     []byte       `json:"data,omitempty"` // base64 in JSON
	PTT      bool         `json:"ptt,omitempty"`  // a voice note (converted to ogg/opus by the bridge)
	Seconds  int          `json:"seconds,omitempty"`
	Poll     *Poll        `json:"poll,omitempty"`
	Contact  *ContactCard `json:"contact,omitempty"`
}

// ContactCard is a WhatsApp contact (vCard) sent in a chat: a name and a number (digits, 62…).
type ContactCard struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// ContactSaver is implemented by transports that can save a number with a name in a WhatsApp account's contacts
// (synced to its phone): a name given in GSI Orbit then also shows on the holder's phone (Sam 17 Oct).
type ContactSaver interface {
	SaveContact(ctx context.Context, account, phone, name string) error
}

// Poll is a WhatsApp poll (vote): a question and 2–12 options, one or several answers.
type Poll struct {
	Name    string   `json:"name"`
	Options []string `json:"options"`
	Multi   bool     `json:"multi,omitempty"`
}

// ContentSender is implemented by transports that can send files, voice notes and polls. Like Send, the outbox id
// travels in ctx (WithAction) and a quote in WithQuote; text is the caption. It returns the WhatsApp id and, for
// media, the download reference of the sent file (shown in Chat like received media).
type ContentSender interface {
	SendContent(ctx context.Context, account, chatJID, text string, c Content) (waMsgID, ref string, err error)
}

// Deleter is implemented by transports that can delete a message the number sent, for everyone. The outbox id of
// the approved delete travels in ctx (WithAction).
type Deleter interface {
	Delete(ctx context.Context, account, chatJID, waMsgID string) error
}

// DeleteWindow is how long after sending GSI Orbit offers "Hapus untuk semua orang" (WhatsApp allows about 2 days).
const DeleteWindow = 48 * time.Hour

// EditWindow is how long after sending WhatsApp accepts an edit.
const EditWindow = 15 * time.Minute

// MediaFetcher is implemented by transports that can fetch a message's media from WhatsApp on demand. The caller
// closes the body.
type MediaFetcher interface {
	Media(ctx context.Context, account string, m Media) (io.ReadCloser, error)
}

// PhotoFetcher reads a contact's or group's WhatsApp profile photo through a GSI number (full: the large one for the
// profile sheet, else the small one for the chat list).
type PhotoFetcher interface {
	Photo(ctx context.Context, account, jid string, full bool) (data []byte, mime string, err error)
}

// Group is a WhatsApp group a GSI number is in, with its members.
type Group struct {
	JID     string
	Name    string
	Members []RosterMember
	// AdminOnly: only admins may write and this number is no admin (WhatsApp refuses its messages there)
	AdminOnly bool
}

// GroupLister lists the groups a GSI number is in (the profile sheet's "grup bersama").
type GroupLister interface {
	Groups(ctx context.Context, account string) ([]Group, error)
}

// ErrNoPhoto: the contact has no profile photo or hides it from this number.
var ErrNoPhoto = errors.New("tidak ada foto profil")

// ErrPhotoBusy: the number is not connected or reached its hourly photo cap (asked again later).
var ErrPhotoBusy = errors.New("foto profil belum bisa diambil sekarang")

// RosterMember is a member of a WhatsApp group as the linked phone sees it. Number is empty for a member whose number
// WhatsApp hides (only a LID).
type RosterMember struct {
	Number string
	Name   string
	Admin  bool
}

// GroupRoster lists a group's members through a GSI number that is in it.
type GroupRoster interface {
	GroupMembers(ctx context.Context, account, groupJID string) ([]RosterMember, error)
}

// ErrMediaGone: WhatsApp no longer has the file and the phone could not upload it again.
var ErrMediaGone = errors.New("media sudah tidak tersedia di WhatsApp")

// ErrMediaUnavailable: the number holding the message is not connected, or too many files are being opened at once.
var ErrMediaUnavailable = errors.New("media belum bisa dibuka")

// Unpairer is implemented by transports that can log a linked device out (Pengaturan → WhatsApp → Lepas).
type Unpairer interface {
	Unpair(ctx context.Context, account string) error
}

type actionKey struct{}

// WithAction carries the outbox row id of an approved proposal to the transport, which the Baileys bridge
// confirms with GSI Orbit before it sends (no row, no send).
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
