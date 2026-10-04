// Package google provides Gmail and Google Calendar capture (read-only plus
// Gmail drafts). ARC never calls messages.send: replies become drafts in the
// owner's mailbox.
package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Mail is one captured e-mail.
type Mail struct {
	MessageID   string
	ThreadID    string
	From        string
	FromName    string
	To          []string
	Cc          []string
	Subject     string
	Date        time.Time
	Body        string // cleaned: quotes and signature removed
	Attachments []Attachment
	Mailbox     string // ARC user id
	Outbound    bool
}

// Attachment metadata (content downloaded only for proposal/quotation/PO/BAST PDFs/DOCX).
type Attachment struct {
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int    `json:"size"`
	Kept bool   `json:"kept"`
}

// Event is a calendar event.
type Event struct {
	ID        string
	Title     string
	Start     time.Time
	Duration  time.Duration
	Location  string
	Attendees []string
	Organizer string
}

// MailSource lists mail incrementally.
type MailSource interface {
	Fetch(ctx context.Context, mailbox, sinceHistory string) ([]Mail, string, error)
}

// CalendarSource lists events in a window.
type CalendarSource interface {
	Events(ctx context.Context, mailbox string, from, to time.Time) ([]Event, error)
}

// DraftCreator creates Gmail drafts.
type DraftCreator interface {
	CreateDraft(ctx context.Context, mailbox, to, subject, body string) (string, error)
}

var (
	reQuote = regexp.MustCompile(`(?m)^(On .+ wrote:|Pada .+ menulis:|-----Original Message-----|From: .+)$`)
	reSig   = regexp.MustCompile(`(?m)^(--\s*$|Salam,?$|Hormat kami,?$|Regards,?$|Terima kasih,?\s*$)`)
	reTag   = regexp.MustCompile(`<[^>]+>`)
)

// CleanBody strips quoted replies, signatures and HTML.
func CleanBody(body string) string {
	if strings.Contains(body, "<html") || strings.Contains(body, "<div") || strings.Contains(body, "<p") {
		body = reTag.ReplaceAllString(strings.ReplaceAll(strings.ReplaceAll(body, "<br>", "\n"), "</p>", "\n"), "")
	}
	if loc := reQuote.FindStringIndex(body); loc != nil {
		body = body[:loc[0]]
	}
	var lines []string
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), ">") {
			continue
		}
		lines = append(lines, l)
	}
	body = strings.Join(lines, "\n")
	if loc := reSig.FindStringIndex(body); loc != nil && loc[0] > 20 {
		body = body[:loc[0]]
	}
	return strings.TrimSpace(body)
}

var keepAttach = regexp.MustCompile(`(?i)(penawaran|quotation|proposal|po|purchase|bast|kontrak|spk)`)

// KeepAttachment decides whether an attachment's content is downloaded.
func KeepAttachment(name, mimeType string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return (ext == ".pdf" || ext == ".docx") && keepAttach.MatchString(name)
}

// ParseEML parses an RFC 822 message.
func ParseEML(raw []byte, mailbox string) (Mail, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Mail{}, err
	}
	h := msg.Header
	m := Mail{MessageID: strings.Trim(h.Get("Message-Id"), "<> "), ThreadID: strings.Trim(h.Get("Thread-Id"), "<> "), Subject: decodeHeader(h.Get("Subject")), Mailbox: mailbox}
	if from, err := mail.ParseAddress(h.Get("From")); err == nil {
		m.From, m.FromName = strings.ToLower(from.Address), from.Name
	}
	for _, k := range []string{"To", "Cc"} {
		if list, err := mail.ParseAddressList(h.Get(k)); err == nil {
			for _, a := range list {
				if k == "To" {
					m.To = append(m.To, strings.ToLower(a.Address))
				} else {
					m.Cc = append(m.Cc, strings.ToLower(a.Address))
				}
			}
		}
	}
	if d, err := h.Date(); err == nil {
		m.Date = d
	}
	if m.ThreadID == "" {
		m.ThreadID = strings.Trim(firstRef(h.Get("References"), h.Get("In-Reply-To"), m.MessageID), "<> ")
	}
	ct, params, _ := mime.ParseMediaType(h.Get("Content-Type"))
	if strings.HasPrefix(ct, "multipart/") {
		mr := multipart.NewReader(msg.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(io.LimitReader(p, 8<<20))
			pct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
			if name := p.FileName(); name != "" {
				m.Attachments = append(m.Attachments, Attachment{Name: name, Mime: pct, Size: len(data), Kept: KeepAttachment(name, pct)})
				continue
			}
			if strings.HasPrefix(pct, "text/") && m.Body == "" {
				m.Body = decodeBody(data, p.Header.Get("Content-Transfer-Encoding"))
			}
		}
	} else {
		data, _ := io.ReadAll(io.LimitReader(msg.Body, 8<<20))
		m.Body = decodeBody(data, h.Get("Content-Transfer-Encoding"))
	}
	m.Body = CleanBody(m.Body)
	return m, nil
}

func firstRef(vals ...string) string {
	for _, v := range vals {
		if f := strings.Fields(v); len(f) > 0 {
			return f[0]
		}
	}
	return ""
}

func decodeHeader(s string) string {
	d, err := new(mime.WordDecoder).DecodeHeader(s)
	if err != nil {
		return s
	}
	return d
}

func decodeBody(data []byte, enc string) string {
	if strings.EqualFold(enc, "base64") {
		if b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(data), "\r\n", "")); err == nil {
			return string(b)
		}
	}
	return string(data)
}

// FakeMail serves .eml fixtures from a directory (tests and done-with-mocks mode).
type FakeMail struct {
	Dir    string
	Drafts []map[string]string
}

// Fetch returns all fixtures newer than the watermark (fixture file index).
func (f *FakeMail) Fetch(_ context.Context, mailbox, since string) ([]Mail, string, error) {
	files, _ := filepath.Glob(filepath.Join(f.Dir, "*.eml"))
	sort.Strings(files)
	var out []Mail
	last := since
	for _, p := range files {
		name := filepath.Base(p)
		if since != "" && name <= since {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, since, err
		}
		m, err := ParseEML(raw, mailbox)
		if err != nil {
			return nil, since, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, m)
		last = name
	}
	return out, last, nil
}

// CreateDraft records a draft (never sends).
func (f *FakeMail) CreateDraft(_ context.Context, mailbox, to, subject, body string) (string, error) {
	f.Drafts = append(f.Drafts, map[string]string{"mailbox": mailbox, "to": to, "subject": subject, "body": body})
	return fmt.Sprintf("draft-%d", len(f.Drafts)), nil
}

// FakeCalendar returns events from a JSON fixture.
type FakeCalendar struct{ File string }

// Events reads fixture events (offsets are days relative to `from`).
func (f *FakeCalendar) Events(_ context.Context, _ string, from, to time.Time) ([]Event, error) {
	raw, err := os.ReadFile(f.File)
	if err != nil {
		return nil, err
	}
	var items []struct {
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		DayOffset int      `json:"day_offset"`
		Time      string   `json:"time"`
		Minutes   int      `json:"minutes"`
		Location  string   `json:"location"`
		Attendees []string `json:"attendees"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	var out []Event
	// Fixture offsets are relative to "today" = from + 7 days (capture window is H-7..H+14).
	today := from.AddDate(0, 0, 7)
	base := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	for _, it := range items {
		var h, m int
		fmt.Sscanf(it.Time, "%d:%d", &h, &m)
		start := base.AddDate(0, 0, it.DayOffset).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
		if start.Before(from) || start.After(to) {
			continue
		}
		out = append(out, Event{ID: it.ID, Title: it.Title, Start: start, Duration: time.Duration(it.Minutes) * time.Minute, Location: it.Location, Attendees: it.Attendees})
	}
	return out, nil
}

// API is the real Gmail/Calendar client for one user's OAuth access token.
type API struct {
	Token func(ctx context.Context, mailbox string) (string, error)
	HTTP  *http.Client
}

func (a *API) get(ctx context.Context, mailbox, u string, out any) error {
	tok, err := a.Token(ctx, mailbox)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("google: HTTP %d: %s", resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Fetch lists new messages via history (or the last 7 days on first run) and parses raw MIME.
func (a *API) Fetch(ctx context.Context, mailbox, since string) ([]Mail, string, error) {
	var ids []string
	newHistory := since
	if since != "" {
		var h struct {
			HistoryID string `json:"historyId"`
			History   []struct {
				MessagesAdded []struct {
					Message struct{ ID string } `json:"message"`
				} `json:"messagesAdded"`
			} `json:"history"`
		}
		if err := a.get(ctx, mailbox, "https://gmail.googleapis.com/gmail/v1/users/me/history?historyTypes=messageAdded&startHistoryId="+url.QueryEscape(since), &h); err != nil {
			return nil, since, err
		}
		for _, x := range h.History {
			for _, m := range x.MessagesAdded {
				ids = append(ids, m.Message.ID)
			}
		}
		newHistory = h.HistoryID
	} else {
		var l struct {
			Messages []struct{ ID string } `json:"messages"`
		}
		if err := a.get(ctx, mailbox, "https://gmail.googleapis.com/gmail/v1/users/me/messages?q=newer_than:7d&maxResults=200", &l); err != nil {
			return nil, since, err
		}
		for _, m := range l.Messages {
			ids = append(ids, m.ID)
		}
	}
	var out []Mail
	for _, id := range ids {
		var m struct {
			Raw       string `json:"raw"`
			ThreadID  string `json:"threadId"`
			HistoryID string `json:"historyId"`
		}
		if err := a.get(ctx, mailbox, "https://gmail.googleapis.com/gmail/v1/users/me/messages/"+id+"?format=raw", &m); err != nil {
			return out, newHistory, err
		}
		raw, err := base64.URLEncoding.DecodeString(m.Raw)
		if err != nil {
			continue
		}
		parsed, err := ParseEML(raw, mailbox)
		if err != nil {
			continue
		}
		parsed.ThreadID = m.ThreadID
		out = append(out, parsed)
		if m.HistoryID > newHistory {
			newHistory = m.HistoryID
		}
	}
	return out, newHistory, nil
}

// Events lists primary-calendar events in a window.
func (a *API) Events(ctx context.Context, mailbox string, from, to time.Time) ([]Event, error) {
	var r struct {
		Items []struct {
			ID       string `json:"id"`
			Summary  string `json:"summary"`
			Location string `json:"location"`
			Start    struct {
				DateTime time.Time `json:"dateTime"`
			} `json:"start"`
			End struct {
				DateTime time.Time `json:"dateTime"`
			} `json:"end"`
			Attendees []struct {
				Email string `json:"email"`
			} `json:"attendees"`
		} `json:"items"`
	}
	u := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/primary/events?singleEvents=true&timeMin=%s&timeMax=%s",
		url.QueryEscape(from.Format(time.RFC3339)), url.QueryEscape(to.Format(time.RFC3339)))
	if err := a.get(ctx, mailbox, u, &r); err != nil {
		return nil, err
	}
	var out []Event
	for _, it := range r.Items {
		ev := Event{ID: it.ID, Title: it.Summary, Start: it.Start.DateTime, Duration: it.End.DateTime.Sub(it.Start.DateTime), Location: it.Location}
		for _, at := range it.Attendees {
			ev.Attendees = append(ev.Attendees, at.Email)
		}
		out = append(out, ev)
	}
	return out, nil
}

// CreateDraft creates a Gmail draft (scope gmail.compose). Never sends.
func (a *API) CreateDraft(ctx context.Context, mailbox, to, subject, body string) (string, error) {
	tok, err := a.Token(ctx, mailbox)
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", to, mime.QEncoding.Encode("utf-8", subject), body)
	payload, _ := json.Marshal(map[string]any{"message": map[string]string{"raw": base64.URLEncoding.EncodeToString([]byte(msg))}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://gmail.googleapis.com/gmail/v1/users/me/drafts", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ID string `json:"id"`
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gmail draft: HTTP %d: %s", resp.StatusCode, b)
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out.ID, err
}
