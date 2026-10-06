package wa

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Whatsmeow links sales phones as WhatsApp linked devices (multi-device, QR pairing). Device keys live in the
// `whatsmeow` schema of the same database; reading groups and history is supported, broadcasting is not.
type Whatsmeow struct {
	container *sqlstore.Container
	log       *slog.Logger
	events    chan Event
	backfill  time.Duration

	mu      sync.Mutex
	clients map[string]*whatsmeow.Client // by account digits
	states  map[string]Status
	groups  map[string]string // group jid → name
}

// WhatsmeowStore opens the device store on db (search_path must point at the whatsmeow schema).
func WhatsmeowStore(db *sql.DB) *sqlstore.Container {
	store.SetOSInfo("Distri ARC", [3]uint32{1, 0, 0})
	return sqlstore.NewWithDB(db, "postgres", waLog.Noop)
}

// NewWhatsmeow builds the transport; accounts are loaded from the device store on Start.
func NewWhatsmeow(container *sqlstore.Container, backfillDays int, log *slog.Logger) *Whatsmeow {
	return &Whatsmeow{container: container, log: log, events: make(chan Event, 512), backfill: time.Duration(backfillDays) * 24 * time.Hour,
		clients: map[string]*whatsmeow.Client{}, states: map[string]Status{}, groups: map[string]string{}}
}

func (w *Whatsmeow) Name() string         { return "whatsmeow" }
func (w *Whatsmeow) Events() <-chan Event { return w.events }

// Start connects every device already paired.
func (w *Whatsmeow) Start(ctx context.Context) error {
	devices, err := w.container.GetAllDevices(ctx)
	if err != nil {
		return err
	}
	for _, d := range devices {
		if d.ID == nil {
			continue
		}
		account := Digits(d.ID.User)
		cli := w.client(account, d)
		if err := cli.ConnectContext(ctx); err != nil {
			w.setState(account, "disconnected", d.ID.String(), "")
			w.log.Warn("whatsmeow connect", "account", account, "err", err)
		}
	}
	return nil
}

func (w *Whatsmeow) client(account string, d *store.Device) *whatsmeow.Client {
	cli := whatsmeow.NewClient(d, waLog.Noop)
	cli.AddEventHandler(func(evt any) { w.handle(account, cli, evt) })
	w.mu.Lock()
	w.clients[account] = cli
	w.mu.Unlock()
	return cli
}

func (w *Whatsmeow) setState(account, state, jid, qr string) {
	s := Status{Account: account, State: state, JID: jid, QR: qr}
	w.mu.Lock()
	w.states[account] = s
	w.mu.Unlock()
	select {
	case w.events <- Event{Status: &s}:
	default:
	}
}

// Pair links a new device for account and returns the first QR code; later codes arrive as Status events.
func (w *Whatsmeow) Pair(ctx context.Context, account string) (string, error) {
	account = Digits(account)
	d := w.container.NewDevice()
	cli := w.client(account, d)
	qrCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	ch, err := cli.GetQRChannel(qrCtx)
	if err != nil {
		cancel()
		return "", err
	}
	if err := cli.ConnectContext(ctx); err != nil {
		cancel()
		return "", err
	}
	first := make(chan string, 1)
	go func() {
		defer cancel()
		sent := false
		for item := range ch {
			switch {
			case item.Event == "code":
				w.setState(account, "pairing", "", item.Code)
				if !sent {
					first <- item.Code
					sent = true
				}
			case item == whatsmeow.QRChannelSuccess:
				jid := ""
				if cli.Store.ID != nil {
					jid = cli.Store.ID.String()
				}
				w.setState(account, "connected", jid, "")
			default:
				w.setState(account, "unpaired", "", "")
				if !sent {
					first <- ""
					sent = true
				}
			}
		}
	}()
	select {
	case code := <-first:
		if code == "" {
			return "", fmt.Errorf("pairing failed")
		}
		return code, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Status lists known numbers.
func (w *Whatsmeow) Status(context.Context) []Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Status, 0, len(w.states))
	for _, s := range w.states {
		out = append(out, s)
	}
	return out
}

// Profile reads the verified business name and the business profile of jid through account's connection.
func (w *Whatsmeow) Profile(ctx context.Context, account, jid string) (Profile, error) {
	w.mu.Lock()
	cli := w.clients[Digits(account)]
	w.mu.Unlock()
	if cli == nil || !cli.IsLoggedIn() {
		return Profile{}, fmt.Errorf("number %s is not connected", account)
	}
	to, err := types.ParseJID(jid)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if info, err := cli.GetUserInfo(ctx, []types.JID{to}); err == nil {
		if u, ok := info[to]; ok && u.VerifiedName != nil && u.VerifiedName.Details != nil {
			p.Name, p.Business = u.VerifiedName.Details.GetVerifiedName(), true
		}
	}
	if bp, err := cli.GetBusinessProfile(ctx, to); err == nil && bp != nil {
		p.Business, p.Address = true, bp.Address
		if len(bp.Categories) > 0 {
			p.Category = bp.Categories[0].Name
		}
	}
	return p, nil
}

// Send delivers a text message from account.
func (w *Whatsmeow) Send(ctx context.Context, account, chatJID, text string) (string, error) {
	w.mu.Lock()
	cli := w.clients[Digits(account)]
	w.mu.Unlock()
	if cli == nil || !cli.IsLoggedIn() {
		return "", fmt.Errorf("number %s is not connected", account)
	}
	to, err := types.ParseJID(chatJID)
	if err != nil {
		return "", err
	}
	resp, err := cli.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(text)})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (w *Whatsmeow) handle(account string, cli *whatsmeow.Client, evt any) {
	switch e := evt.(type) {
	case *events.Message:
		if m := w.convert(account, cli, e); m != nil {
			w.events <- Event{Message: m}
		}
	case *events.HistorySync:
		cutoff := time.Now().Add(-w.backfill)
		for _, conv := range e.Data.GetConversations() {
			chat, err := types.ParseJID(conv.GetID())
			if err != nil {
				continue
			}
			for _, hm := range conv.GetMessages() {
				pm, err := cli.ParseWebMessage(chat, hm.GetMessage())
				if err != nil || pm.Info.Timestamp.Before(cutoff) {
					continue
				}
				if m := w.convert(account, cli, pm); m != nil {
					w.events <- Event{Message: m}
				}
			}
		}
	case *events.Connected:
		jid := ""
		if cli.Store.ID != nil {
			jid = cli.Store.ID.String()
		}
		w.setState(account, "connected", jid, "")
	case *events.Disconnected:
		w.setState(account, "disconnected", "", "")
	case *events.LoggedOut:
		w.setState(account, "logged_out", "", "")
	}
}

func (w *Whatsmeow) convert(account string, cli *whatsmeow.Client, e *events.Message) *Message {
	msg := e.Message
	text := msg.GetConversation()
	if text == "" {
		text = msg.GetExtendedTextMessage().GetText()
	}
	if text == "" {
		text = msg.GetImageMessage().GetCaption()
	}
	if text == "" {
		return nil
	}
	m := &Message{ID: e.Info.ID, Account: account, ChatJID: e.Info.Chat.ToNonAD().String(), FromNumber: Digits(e.Info.Sender.User),
		FromName: e.Info.PushName, IsGroup: e.Info.IsGroup, FromMe: e.Info.IsFromMe, Text: text, Time: e.Info.Timestamp}
	if e.Info.Sender.Server == types.HiddenUserServer && !e.Info.SenderAlt.IsEmpty() {
		m.FromNumber = Digits(e.Info.SenderAlt.User)
	}
	if m.IsGroup {
		m.GroupName = w.groupName(cli, e.Info.Chat)
	}
	return m
}

func (w *Whatsmeow) groupName(cli *whatsmeow.Client, jid types.JID) string {
	w.mu.Lock()
	name, ok := w.groups[jid.String()]
	w.mu.Unlock()
	if ok {
		return name
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if info, err := cli.GetGroupInfo(ctx, jid); err == nil {
		name = info.Name
	}
	w.mu.Lock()
	w.groups[jid.String()] = name
	w.mu.Unlock()
	return name
}
