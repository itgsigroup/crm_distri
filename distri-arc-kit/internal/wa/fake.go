package wa

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Fake is an in-memory transport: Inject simulates an incoming message, Send records and acknowledges.
type Fake struct {
	mu       sync.Mutex
	events   chan Event
	profiles map[string]Profile
	sent     []Message
	seq      atomic.Int64
	states   map[string]string
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
func (f *Fake) Send(_ context.Context, account, chatJID, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty message")
	}
	id := fmt.Sprintf("FAKE%06d", f.seq.Add(1))
	f.mu.Lock()
	f.sent = append(f.sent, Message{ID: id, Account: Digits(account), ChatJID: chatJID, FromNumber: Digits(account), FromMe: true, Text: text})
	f.mu.Unlock()
	return id, nil
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
