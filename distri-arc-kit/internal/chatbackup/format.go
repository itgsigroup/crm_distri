// Package chatbackup keeps chats the way WhatsApp's own "Ekspor chat" does (ADR 0061): plain text, one line per
// message ("23/10/26 14.05 - Nama: pesan", the Indonesian Android export), media left out ("<Media tidak
// disertakan>"). One chat downloads as a .txt; every night the month's chats go into one zip of .txt files that
// outlives the 90-day chat retention. Messages are streamed row by row: memory stays flat however long the chat.
package chatbackup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/clock"
	"distri-arc/internal/llm"
)

// DB is what the backup reads with (a *pgxpool.Pool).
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Conv is one conversation: a one-to-one chat, or a group (one conversation for every GSI number in it).
type Conv struct {
	ID      uuid.UUID
	Kind    string // dealer | new | group | internal
	JID     string // the other side (a group: the group); a backup entry carries it so it can be forgotten later
	Title   string // as Chat shows it: "nickname - name"
	Account string // the GSI number's label ("Elisa")
}

// convSelect reads a conversation as Chat shows it (GetThread: account_label; web displayTitle).
const convSelect = `select t.id, coalesce(t.kind, ''), coalesce(t.wa_jid, ''), coalesce(t.title, ''), coalesce(t.phone_name, ''),
  coalesce(nullif(n.label, ''), s.name, t.account, '')
from chat_threads t left join wa_numbers n on n.wa_number = t.account left join sales_users s on s.id = t.sales_id`

func scanConv(row pgx.Row) (Conv, error) {
	var c Conv
	var title, nick string
	if err := row.Scan(&c.ID, &c.Kind, &c.JID, &title, &nick, &c.Account); err != nil {
		return c, err
	}
	c.Title = displayTitle(title, nick, c.Kind, c.JID)
	return c, nil
}

// LoadConv reads one conversation.
func LoadConv(ctx context.Context, db DB, id uuid.UUID) (Conv, error) {
	return scanConv(db.QueryRow(ctx, convSelect+` where t.id = $1`, id))
}

// displayTitle is web/src/features/chat/inbox.ts displayTitle: the nickname saved here before the name.
func displayTitle(title, nick, kind, jid string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		if kind == "group" {
			title = "Grup"
		} else if p := phoneOf(jid); p != "" {
			title = p
		} else {
			title = "Tanpa nama"
		}
	}
	nick = strings.TrimSpace(nick)
	if nick == "" || strings.Contains(strings.ToLower(title), strings.ToLower(nick)) {
		return title
	}
	return nick + " - " + title
}

// phoneOf is "+62812…" of a one-to-one JID ("" for a group or a hidden @lid number).
func phoneOf(jid string) string {
	user, server, ok := strings.Cut(jid, "@")
	if !ok || server != "s.whatsapp.net" || user == "" {
		return ""
	}
	return "+" + user
}

// FileName is WhatsApp's own export name: "Chat WhatsApp dengan Nama.txt".
func FileName(c Conv) string { return "Chat WhatsApp dengan " + safeName(c.Title) + ".txt" }

// safeName keeps a name usable as a file name on Windows, macOS and Android.
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		s = strings.TrimSpace(string(r[:80]))
	}
	s = strings.Trim(s, ". ")
	if s == "" {
		return "Tanpa nama"
	}
	return s
}

// stamp is WhatsApp's Indonesian Android time: "23/10/26 14.05".
func stamp(t time.Time) string { return t.In(clock.WIB).Format("02/01/06 15.04") }

// msgCols and msgFrom read messages with the name of the GSI number whose thread holds each (alias x). Queued and
// failed messages never reached WhatsApp and are left out, as WhatsApp's export would.
const msgCols = `m.sent_at, coalesce(m.direction, ''), coalesce(m.from_name, ''), coalesce(m.from_number, ''),
  coalesce(m.body, ''), coalesce(m.media->>'kind', ''), coalesce(m.media->>'file_name', ''), m.deleted_at is not null,
  m.edited_at is not null, coalesce(nullif(n.label, ''), s.name, x.account, '')`

const msgFrom = `from chat_messages m
join chat_threads x on x.id = m.thread_id
left join wa_numbers n on n.wa_number = x.account
left join sales_users s on s.id = x.sales_id
where m.sent_at >= $1 and m.sent_at < $2 and m.status not in ('pending', 'failed')`

// msgSelect: one conversation's messages, a group's from every GSI number's thread of it.
const msgSelect = `select ` + msgCols + ` ` + msgFrom + `
  and (x.id = $3 or ($4 <> '' and x.kind = 'group' and x.wa_jid = $4))
order by m.sent_at, m.id`

// dests are the scan targets of msgCols.
func (m *Msg) dests() []any {
	return []any{&m.At, &m.Direction, &m.FromName, &m.FromNum, &m.Body, &m.Media, &m.FileName, &m.Deleted, &m.Edited, &m.Account}
}

// Msg is one message as the export writes it.
type Msg struct {
	At        time.Time
	Direction string // in | out
	FromName  string
	FromNum   string
	Body      string
	Media     string // image | video | audio | document | sticker | location | contact | ""
	FileName  string
	Deleted   bool
	Edited    bool
	Account   string // label of the GSI number whose thread holds it (the "me" of an out message)
}

// Writer streams one conversation as WhatsApp export text.
type Writer struct {
	w     *bufio.Writer
	c     Conv
	Count int
}

// NewWriter starts a conversation's text with WhatsApp's first system line (here: where the copy comes from).
func NewWriter(w io.Writer, c Conv, now time.Time) *Writer {
	x := &Writer{w: bufio.NewWriterSize(w, 32<<10), c: c}
	_, _ = fmt.Fprintf(x.w, "%s - Salinan chat dari GSI Orbit, format ekspor WhatsApp (media tidak disertakan). Chat dengan %s lewat nomor GSI %s.\n",
		stamp(now), c.Title, nonEmpty(c.Account, "-"))
	return x
}

// Write adds one message (a write error is kept by the buffer and returned by Flush).
func (x *Writer) Write(m Msg) {
	x.Count++
	_, _ = fmt.Fprintf(x.w, "%s - %s: %s\n", stamp(m.At), x.sender(m), text(m))
}

// Flush ends the text.
func (x *Writer) Flush() error { return x.w.Flush() }

// sender is the name WhatsApp would show: our own number's name for what it sent, the chat's name in a one-to-one
// chat, the member's name (else their number) in a group.
func (x *Writer) sender(m Msg) string {
	switch {
	case m.Direction == "out":
		return oneLine(nonEmpty(m.Account, x.c.Account, "GSI"))
	case x.c.Kind != "group":
		return oneLine(nonEmpty(x.c.Title, m.FromName, plus(m.FromNum)))
	}
	return oneLine(nonEmpty(m.FromName, plus(m.FromNum), "Anggota grup"))
}

// text is the message as WhatsApp's export writes it without media. Credentials stay masked, as everywhere in the CRM.
func text(m Msg) string {
	if m.Deleted {
		return "Pesan ini telah dihapus"
	}
	body := strings.ReplaceAll(llm.MaskCredentials(m.Body), "\r\n", "\n")
	switch m.Media {
	case "":
	case "location":
		body = "lokasi: " + nonEmpty(body, "(tanpa nama)")
	case "contact":
		body = "kartu kontak: " + nonEmpty(body, "(tanpa nama)")
	case "call": // "Panggilan suara tak terjawab", as WhatsApp words it
	default:
		media := "<Media tidak disertakan>"
		if m.Media == "document" && m.FileName != "" {
			media = m.FileName + " " + media
		}
		if body != "" {
			body = media + "\n" + body
		} else {
			body = media
		}
	}
	if m.Edited {
		body += " <Pesan ini diedit>"
	}
	return body
}

// WriteConv streams a conversation's messages sent in [from, to) into w. It returns how many were written.
func WriteConv(ctx context.Context, db DB, w io.Writer, c Conv, from, to, now time.Time) (int, error) {
	group := ""
	if c.Kind == "group" {
		group = c.JID
	}
	rows, err := db.Query(ctx, msgSelect, from, to, c.ID, group)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	x := NewWriter(w, c, now)
	var m Msg
	for rows.Next() {
		if err := rows.Scan(m.dests()...); err != nil {
			return x.Count, err
		}
		x.Write(m)
	}
	if err := rows.Err(); err != nil {
		return x.Count, err
	}
	return x.Count, x.Flush()
}

func plus(n string) string {
	if n = strings.TrimPrefix(strings.TrimSpace(n), "+"); n == "" {
		return ""
	}
	return "+" + n
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func nonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
