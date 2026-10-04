package main

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// waEvent mirrors packages/connectors/whatsapp.WaEvent (the API contract).
type waEvent struct {
	Wamid      string     `json:"wamid"`
	Session    string     `json:"session"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	ChatID     string     `json:"chat_id"`
	IsGroup    bool       `json:"is_group"`
	GroupMeta  *groupMeta `json:"group_meta,omitempty"`
	SenderName string     `json:"sender_name"`
	Text       string     `json:"text"`
	Media      *mediaMeta `json:"media_meta,omitempty"`
	Timestamp  time.Time  `json:"timestamp"`
	Quoted     string     `json:"quoted,omitempty"`
	FromMe     bool       `json:"from_me"`
	IsHistory  bool       `json:"is_history"`
	Transport  string     `json:"transport"`
}

type groupMember struct {
	JID   string `json:"jid"`
	Phone string `json:"phone"`
	Name  string `json:"name"`
}

type groupMeta struct {
	Name    string        `json:"name"`
	Members []groupMember `json:"members"`
}

type mediaMeta struct {
	Kind     string `json:"kind"`
	Mime     string `json:"mime"`
	FileName string `json:"file_name"`
	Size     int64  `json:"size"`
}

// messageText extracts the human text and media metadata. Media content is never downloaded.
func messageText(msg *waE2E.Message) (string, *mediaMeta, string) {
	if msg == nil {
		return "", nil, ""
	}
	quoted := ""
	if ci := msg.GetExtendedTextMessage().GetContextInfo(); ci != nil {
		quoted = ci.GetStanzaID()
	}
	switch {
	case msg.GetConversation() != "":
		return msg.GetConversation(), nil, quoted
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetText(), nil, quoted
	case msg.GetImageMessage() != nil:
		m := msg.GetImageMessage()
		return m.GetCaption(), &mediaMeta{Kind: "image", Mime: m.GetMimetype(), Size: int64(m.GetFileLength())}, quoted
	case msg.GetDocumentMessage() != nil:
		m := msg.GetDocumentMessage()
		return m.GetCaption(), &mediaMeta{Kind: "document", Mime: m.GetMimetype(), FileName: m.GetFileName(), Size: int64(m.GetFileLength())}, quoted
	case msg.GetVideoMessage() != nil:
		m := msg.GetVideoMessage()
		return m.GetCaption(), &mediaMeta{Kind: "video", Mime: m.GetMimetype(), Size: int64(m.GetFileLength())}, quoted
	case msg.GetAudioMessage() != nil:
		m := msg.GetAudioMessage()
		return "", &mediaMeta{Kind: "audio", Mime: m.GetMimetype(), Size: int64(m.GetFileLength())}, quoted
	case msg.GetLocationMessage() != nil:
		return msg.GetLocationMessage().GetName(), &mediaMeta{Kind: "location"}, quoted
	case msg.GetContactMessage() != nil:
		return msg.GetContactMessage().GetDisplayName(), &mediaMeta{Kind: "contact"}, quoted
	}
	return "", nil, quoted
}

// phoneOf resolves a JID to a phone number, translating LIDs through the store when possible.
func (m *manager) phoneOf(ctx context.Context, s *session, jid types.JID) string {
	if jid.Server == types.HiddenUserServer && s.client.Store.LIDs != nil {
		if pn, err := s.client.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !pn.IsEmpty() {
			return pn.User
		}
	}
	return jid.User
}

func (m *manager) toWaEvent(ctx context.Context, s *session, e *events.Message, history bool) (waEvent, bool) {
	text, media, quoted := messageText(e.Message)
	if text == "" && media == nil {
		return waEvent{}, false // protocol messages, reactions, receipts
	}
	info := e.Info
	if info.Chat.Server == types.BroadcastServer {
		return waEvent{}, false // status updates are not customer conversations
	}
	own := ""
	if id := s.client.Store.ID; id != nil {
		own = id.User
	}
	ev := waEvent{
		Wamid:      info.ID,
		Session:    s.meta.ID,
		From:       m.phoneOf(ctx, s, info.Sender),
		ChatID:     info.Chat.String(),
		IsGroup:    info.IsGroup,
		SenderName: info.PushName,
		Text:       text,
		Media:      media,
		Timestamp:  info.Timestamp,
		Quoted:     quoted,
		FromMe:     info.IsFromMe,
		IsHistory:  history,
		Transport:  "bridge",
	}
	if info.IsFromMe {
		ev.From = own
		ev.To = m.phoneOf(ctx, s, info.Chat)
	} else {
		ev.To = own
	}
	if ev.SenderName == "" {
		if c, err := s.client.Store.Contacts.GetContact(ctx, info.Sender); err == nil && c.Found {
			ev.SenderName = firstNonEmpty(c.FullName, c.PushName, c.BusinessName)
		}
	}
	if info.IsGroup {
		ev.GroupMeta = m.groupMeta(ctx, s, info.Chat)
	}
	return ev, true
}

// groupMeta returns group name and members, cached per session for an hour.
func (m *manager) groupMeta(ctx context.Context, s *session, chat types.JID) *groupMeta {
	key := chat.String()
	s.mu.Lock()
	if c, ok := s.groups[key]; ok && time.Since(c.at) < time.Hour {
		s.mu.Unlock()
		return c.meta
	}
	s.mu.Unlock()
	gi, err := s.client.GetGroupInfo(ctx, chat)
	if err != nil || gi == nil {
		return &groupMeta{}
	}
	meta := &groupMeta{Name: gi.Name}
	for _, p := range gi.Participants {
		phone := p.PhoneNumber.User
		if phone == "" {
			phone = m.phoneOf(ctx, s, p.JID)
		}
		name := p.DisplayName
		if c, err := s.client.Store.Contacts.GetContact(ctx, p.JID); err == nil && c.Found {
			name = firstNonEmpty(c.FullName, c.PushName, name)
		}
		meta.Members = append(meta.Members, groupMember{JID: p.JID.String(), Phone: phone, Name: name})
	}
	s.mu.Lock()
	if s.groups == nil {
		s.groups = map[string]cachedGroup{}
	}
	s.groups[key] = cachedGroup{meta: meta, at: time.Now()}
	s.mu.Unlock()
	return meta
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
