package api

import (
	"context"
	"time"

	"distri-arc/internal/store/gen"
)

// replyWindow is WhatsApp's customer-service window: a business number may write to a person only within 24 hours
// of that person's last message. Starting or reviving a chat from GSI Orbit outside it is what gets numbers banned,
// so Chat refuses it and the person starts the chat from WhatsApp Web or the phone (ADR 0081).
const replyWindow = 24 * time.Hour

// replyClosed reports whether a one-to-one chat with a customer is outside the window, with the contact's last
// message (nil: they never wrote). Groups and chats with colleagues are not bound by it.
func (s *Server) replyClosed(ctx context.Context, t gen.GetThreadRow) (bool, *time.Time) {
	if k := deref(t.Kind); k != "dealer" && k != "new" {
		return false, nil
	}
	var last *time.Time
	_ = s.st.Pool.QueryRow(ctx, "select max(sent_at) from chat_messages where thread_id = $1 and direction = 'in'", t.ID).Scan(&last)
	return last == nil || s.clock.Now().Sub(*last) >= replyWindow, last
}
