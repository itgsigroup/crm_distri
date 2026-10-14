package waanalysis

import (
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
)

// Per bubble: every analysis also reads each message of the conversations it covers — what a customer message asks
// for and whether (and how fast) it was answered; for a sales message, how long the customer had waited.

// Message states.
const (
	MsgReplied    = "replied"     // customer message the sales answered
	MsgWaiting    = "waiting"     // customer message without a reply yet (less than ReplyDue)
	MsgNeedsReply = "needs_reply" // customer message waiting longer than ReplyDue
	MsgClosing    = "closing"     // "Ok Bu", "siap", "terima kasih 👍" at the end: nobody waits for a reply
	MsgReply      = "reply"       // sales message answering the customer
	MsgFollowUp   = "follow_up"   // sales message nobody was waiting for (the sales writes first)
	MsgHolding    = "holding"     // "tunggu sebentar", "saya konfirmasi dulu": not an answer, the customer still waits
)

// MessageInsight is what one bubble says.
type MessageInsight struct {
	MessageID uuid.UUID
	ThreadID  uuid.UUID
	In        bool
	SentAt    time.Time
	State     string
	Intent    string // customer messages: the request (IntentNone for ours)
	Items     []Item
	// WaitMin: customer message — minutes until the answer (a holding reply is not one), or (unanswered) since the
	// customer's first unanswered message; sales reply or holding reply — minutes the customer had waited by then.
	// Nil for a follow-up or a closing message.
	WaitMin *int
}

// AnalyzeMessages reads every message of a conversation at now, oldest first.
func AnalyzeMessages(t Thread, catalog []agents.Product, now time.Time) []MessageInsight {
	out := make([]MessageInsight, len(t.Messages))
	var pending []int // indexes of the customer's messages since our last reply
	for i, m := range t.Messages {
		mi := MessageInsight{MessageID: m.ID, ThreadID: t.ID, In: m.In, SentAt: m.At, Intent: IntentNone, Items: []Item{}}
		if m.In {
			mi.Intent = agents.IntentOther
			if !IsAck(m.Text) {
				e := agents.Extract(m.Text, catalog)
				mi.Intent = e.Intent
				for _, it := range e.Items {
					mi.Items = addItem(mi.Items, Item{Name: it.Product.Name, Qty: it.Qty})
				}
			}
			out[i] = mi
			pending = append(pending, i)
			continue
		}
		if m.Holding && len(pending) > 0 {
			// a holding reply: the customer's messages stay unanswered and their time keeps running
			mi.State = MsgHolding
			mi.WaitMin = minutes(m.At.Sub(t.Messages[pending[0]].At))
			out[i] = mi
			continue
		}
		if len(pending) == 0 {
			mi.State = MsgFollowUp
		} else {
			mi.State = MsgReply
			mi.WaitMin = minutes(m.At.Sub(t.Messages[pending[0]].At))
			for _, j := range pending {
				out[j].State = MsgReplied
				out[j].WaitMin = minutes(m.At.Sub(t.Messages[j].At))
			}
		}
		pending = nil
		out[i] = mi
	}
	if len(pending) > 0 {
		closing := allAck(msgsAt(t.Messages, pending))
		runWait := now.Sub(t.Messages[pending[0]].At)
		for _, j := range pending {
			switch {
			case closing:
				out[j].State = MsgClosing
			case runWait >= ReplyDue:
				out[j].State = MsgNeedsReply
			default:
				out[j].State = MsgWaiting
			}
			if !closing {
				out[j].WaitMin = minutes(runWait) // how long the customer has been waiting: since their first unanswered message
			}
		}
	}
	return out
}

func msgsAt(ms []Message, idx []int) []Message {
	out := make([]Message, len(idx))
	for k, i := range idx {
		out[k] = ms[i]
	}
	return out
}

func minutes(d time.Duration) *int {
	m := max(0, int(d.Minutes()+0.5))
	return &m
}
