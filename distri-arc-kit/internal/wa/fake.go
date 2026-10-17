package wa

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Fake is an in-memory transport: Inject simulates an incoming message, Send records and acknowledges.
type Fake struct {
	mu       sync.Mutex
	events   chan Event
	profiles map[string]Profile
	sent     []Message
	seq      atomic.Int64
	states   map[string]string
	saved    []string
}

// NewFake returns a fake transport whose numbers are all "connected".
func NewFake(accounts ...string) *Fake {
	f := &Fake{events: make(chan Event, 256), states: map[string]string{}}
	for _, a := range accounts {
		f.states[Digits(a)] = "connected"
	}
	return f
}

func (f *Fake) Name() string                { return "fake" }
func (f *Fake) Start(context.Context) error { return nil }
func (f *Fake) Events() <-chan Event        { return f.events }

// Pair marks the number connected immediately.
func (f *Fake) Pair(_ context.Context, account string) (string, error) {
	f.mu.Lock()
	f.states[Digits(account)] = "connected"
	f.mu.Unlock()
	f.events <- Event{Status: &Status{Account: Digits(account), State: "connected", JID: UserJID(account)}}
	return "", nil
}

// Link simulates scanning: a QR (or code), then the phone reports its number (phone, or a generated one).
func (f *Fake) Link(_ context.Context, session, phone string, code bool) (string, error) {
	n := Digits(phone)
	if n == "" {
		n = fmt.Sprintf("62899%07d", f.seq.Add(1))
	}
	f.mu.Lock()
	f.states[n] = "connected"
	f.mu.Unlock()
	qr := "2@fake-qr-" + session
	if code {
		qr = PairCodePrefix + "FAKE-0000"
	}
	go func() { // the "phone" scans a moment later, logs in, then reports its number
		time.Sleep(1500 * time.Millisecond)
		f.events <- Event{Status: &Status{Session: session, State: "pairing", QR: LinkingPrefix}}
		time.Sleep(2500 * time.Millisecond)
		f.events <- Event{Status: &Status{Session: session, Account: n, State: "connected", JID: UserJID(n)}}
	}()
	return qr, nil
}

// Status lists fake numbers.
func (f *Fake) Status(context.Context) []Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Status
	for a, s := range f.states {
		out = append(out, Status{Account: a, State: s, JID: UserJID(a)})
	}
	return out
}

// Send records the message.
func (f *Fake) Send(ctx context.Context, account, chatJID, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty message")
	}
	id := fmt.Sprintf("FAKE%06d", f.seq.Add(1))
	m := Message{ID: id, Account: Digits(account), ChatJID: chatJID, FromNumber: Digits(account), FromMe: true, Text: text}
	if q := QuoteFrom(ctx); q != nil {
		m.QuotedID = q.ID
	}
	f.mu.Lock()
	f.sent = append(f.sent, m)
	f.mu.Unlock()
	return id, nil
}

// Edit records an edit as a sent message with EditOf set.
func (f *Fake) Edit(_ context.Context, account, chatJID, waMsgID, text string) error {
	if text == "" || waMsgID == "" {
		return fmt.Errorf("empty edit")
	}
	f.mu.Lock()
	f.sent = append(f.sent, Message{ID: fmt.Sprintf("FAKE%06d", f.seq.Add(1)), Account: Digits(account), ChatJID: chatJID, FromNumber: Digits(account), FromMe: true, Text: text, EditOf: waMsgID})
	f.mu.Unlock()
	return nil
}

// SendContent records a file, voice note or poll as a sent message carrying its media (Kind "poll" for polls); a
// media send gets a fake download reference.
func (f *Fake) SendContent(ctx context.Context, account, chatJID, text string, c Content) (string, string, error) {
	noFile := c.Kind == "poll" || c.Kind == "contact"
	if c.Kind == "" || (!noFile && len(c.Data) == 0) || (c.Kind == "poll" && (c.Poll == nil || len(c.Poll.Options) < 2)) ||
		(c.Kind == "contact" && (c.Contact == nil || c.Contact.Name == "" || len(Digits(c.Contact.Phone)) < 8)) {
		return "", "", fmt.Errorf("empty content")
	}
	id := fmt.Sprintf("FAKE%06d", f.seq.Add(1))
	m := Message{ID: id, Account: Digits(account), ChatJID: chatJID, FromNumber: Digits(account), FromMe: true, Text: text,
		Media: &Media{Kind: c.Kind, Mime: c.Mime, FileName: c.FileName, Size: int64(len(c.Data)), PTT: c.PTT, Seconds: c.Seconds}}
	if q := QuoteFrom(ctx); q != nil {
		m.QuotedID = q.ID
	}
	ref := ""
	if !noFile {
		ref = "FAKEREF-" + id
	}
	f.mu.Lock()
	f.sent = append(f.sent, m)
	f.mu.Unlock()
	return id, ref, nil
}

// Delete records a delete-for-everyone as a sent message with RevokeOf set.
func (f *Fake) Delete(_ context.Context, account, chatJID, waMsgID string) error {
	if waMsgID == "" {
		return fmt.Errorf("empty delete")
	}
	f.mu.Lock()
	f.sent = append(f.sent, Message{ID: fmt.Sprintf("FAKE%06d", f.seq.Add(1)), Account: Digits(account), ChatJID: chatJID, FromNumber: Digits(account), FromMe: true, RevokeOf: waMsgID})
	f.mu.Unlock()
	return nil
}

// SetProfile registers a WhatsApp Business profile the fake returns for a number (tests, seed demo).
func (f *Fake) SetProfile(number string, p Profile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.profiles == nil {
		f.profiles = map[string]Profile{}
	}
	f.profiles[Digits(number)] = p
}

// Profile returns the registered profile (empty when the number is not a business account).
func (f *Fake) Profile(_ context.Context, _ string, jid string) (Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	num, _, _ := strings.Cut(jid, "@")
	return f.profiles[Digits(num)], nil
}

// Inject queues an incoming message as if WhatsApp delivered it.
func (f *Fake) Inject(m Message) { f.events <- Event{Message: &m} }

// Sent returns every message sent so far.
func (f *Fake) Sent() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Message(nil), f.sent...)
}

// SaveContact records a contact saved to an account's WhatsApp contacts (Saved).
func (f *Fake) SaveContact(_ context.Context, account, phone, name string) error {
	if len(Digits(phone)) < 8 || name == "" {
		return fmt.Errorf("bad contact")
	}
	f.mu.Lock()
	f.saved = append(f.saved, Digits(account)+":"+Digits(phone)+"="+name)
	f.mu.Unlock()
	return nil
}

// Saved lists the contacts saved so far ("account:phone=name").
func (f *Fake) Saved() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.saved...)
}
