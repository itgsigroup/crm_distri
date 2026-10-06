// Package commitment keeps two-way commitments (Komitmen dua arah): what GSI promised (Kami, from approved
// proposals) and what the dealer promised (Mereka, read from their WhatsApp messages). It also links an inbound
// message to the proposal it answers (reply tracking).
package commitment

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/clock"
	"distri-arc/internal/store/gen"
)

var weekdays = map[string]time.Weekday{"minggu": time.Sunday, "senin": time.Monday, "selasa": time.Tuesday, "rabu": time.Wednesday, "kamis": time.Thursday,
	"jumat": time.Friday, "sabtu": time.Saturday}

// NextWeekday is the next date after today falling on an Indonesian weekday name ("Senin").
func NextWeekday(today time.Time, day string) time.Time {
	wd, ok := weekdays[strings.ToLower(day)]
	if !ok {
		return today.AddDate(0, 0, 1)
	}
	d := today.AddDate(0, 0, 1)
	for d.Weekday() != wd {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

var (
	rePay     = regexp.MustCompile(`(?i)\b(bayar|dibayar|transfer|ditransfer|lunas|lunasi|cicil|pelunasan)\b`)
	reInvoice = regexp.MustCompile(`(?i)\bINV/\d+`)
	reDate    = regexp.MustCompile(`(?i)\b(?:tanggal|tgl)\s*(\d{1,2})\b`)
	reWeekday = regexp.MustCompile(`(?i)\b(senin|selasa|rabu|kamis|jumat|sabtu)\b`)
	reYes     = regexp.MustCompile(`(?i)^\s*(ya|iya|oke|ok|siap|boleh|setuju|jadi|lanjut|gas)\b|kirim aja|order ya|sesuai rekomendasi`)
	reTempo   = regexp.MustCompile(`(?i)minggu depan|bulan depan|nanti|belum cair|tunggu`)
)

// When reads the date a promise names; ok is false when the promise has no date ("minggu depan ya" counts as a
// date one week ahead, "nanti" does not).
func When(text string, today time.Time) (time.Time, bool) {
	low := strings.ToLower(text)
	switch {
	case strings.Contains(low, "hari ini"):
		return today, true
	case strings.Contains(low, "besok"):
		return today.AddDate(0, 0, 1), true
	case strings.Contains(low, "lusa"):
		return today.AddDate(0, 0, 2), true
	case strings.Contains(low, "minggu depan"):
		return today.AddDate(0, 0, 7), true
	case strings.Contains(low, "bulan depan"):
		return today.AddDate(0, 1, 0), true
	}
	if m := reDate.FindStringSubmatch(text); m != nil {
		d, _ := strconv.Atoi(m[1])
		if d >= 1 && d <= 31 {
			t := time.Date(today.Year(), today.Month(), d, 0, 0, 0, 0, today.Location())
			if t.Before(today) {
				t = t.AddDate(0, 1, 0)
			}
			return t, true
		}
	}
	if m := reWeekday.FindString(text); m != "" {
		return NextWeekday(today, m), true
	}
	return time.Time{}, false
}

// Inbound is one stored inbound dealer message.
type Inbound struct {
	DealerID  uuid.UUID
	ThreadID  uuid.UUID
	MessageID uuid.UUID
	SignalID  uuid.UUID
	Text      string
	At        time.Time
}

// Result tells what an inbound message produced.
type Result struct {
	ReplyTo    *uuid.UUID // the proposal this message answers
	Commitment *uuid.UUID
	Title      string
}

func rp(v int64) string { return fmt.Sprintf("Rp %d jt", (v+500_000)/1_000_000) }

// FromInbound links a reply to the proposal sent in the thread in the last 72 hours and turns a payment promise or
// an order confirmation into a "Mereka" commitment (idempotent per message).
func FromInbound(ctx context.Context, q *gen.Queries, m Inbound) (Result, error) {
	var res Result
	today := clock.Today(m.At)
	var replyTitle, replyKind string
	var replyPayload json.RawMessage
	if last, err := q.LastSentProposalInThread(ctx, gen.LastSentProposalInThreadParams{ThreadID: &m.ThreadID, Before: m.At}); err == nil && last.ProposalID != nil {
		res.ReplyTo = last.ProposalID
		replyTitle, replyKind, replyPayload = last.Title, last.Kind, last.Payload
		if err := q.SetMessageReplyTo(ctx, gen.SetMessageReplyToParams{ID: m.MessageID, ProposalID: last.ProposalID}); err != nil {
			return res, err
		}
		if err := q.SetSignalReply(ctx, gen.SetSignalReplyParams{ID: m.SignalID, Column2: last.ProposalID.String()}); err != nil {
			return res, err
		}
	}
	key := "wa:" + m.MessageID.String()
	dealer := m.DealerID
	switch {
	case rePay.MatchString(m.Text) || (res.ReplyTo != nil && (replyKind == "collect" || replyKind == "installment") && reTempo.MatchString(m.Text)):
		invs, err := q.DealerOpenInvoiceList(ctx, &dealer)
		if err != nil {
			return res, err
		}
		var inv *gen.DealerOpenInvoiceListRow
		if num := reInvoice.FindString(m.Text); num != "" {
			for i := range invs {
				if strings.EqualFold(deref(invs[i].Number), num) {
					inv = &invs[i]
				}
			}
		}
		if inv == nil && len(replyPayload) > 0 {
			var pl struct {
				Invoices []string `json:"invoices"`
			}
			_ = json.Unmarshal(replyPayload, &pl)
			for i := range invs {
				if len(pl.Invoices) > 0 && deref(invs[i].Number) == pl.Invoices[0] {
					inv = &invs[i]
				}
			}
		}
		if inv == nil && len(invs) > 0 {
			inv = &invs[0] // the oldest open invoice
		}
		title := "Bayar"
		var invID *uuid.UUID
		if inv != nil {
			title = fmt.Sprintf("Bayar %s %s", deref(inv.Number), rp(inv.Residual))
			invID = &inv.ID
		}
		var due *time.Time
		detail := "Janji via WhatsApp tanpa tanggal"
		if d, ok := When(m.Text, today); ok {
			due = &d
			detail = fmt.Sprintf("Janji via WhatsApp · %s", clock.DayMonth(d))
		}
		id, err := q.InsertCommitment(ctx, gen.InsertCommitmentParams{DealerID: &dealer, Side: "mereka", Title: title, Detail: &detail, DueAt: due, InvoiceID: invID,
			ProposalID: res.ReplyTo, SignalIds: []uuid.UUID{m.SignalID}, SourceKey: &key, CreatedAt: m.At})
		if err != nil {
			return res, err
		}
		res.Commitment, res.Title = &id, title
	case res.ReplyTo != nil && reYes.MatchString(m.Text) && (replyKind == "followup" || replyKind == "price_list" || replyKind == "push_stock" || replyKind == "price_counter"):
		due := today.AddDate(0, 0, 2)
		detail := "Balasan untuk: " + replyTitle
		title := "Order sesuai rekomendasi"
		id, err := q.InsertCommitment(ctx, gen.InsertCommitmentParams{DealerID: &dealer, Side: "mereka", Title: title, Detail: &detail, DueAt: &due,
			ProposalID: res.ReplyTo, SignalIds: []uuid.UUID{m.SignalID}, SourceKey: &key, CreatedAt: m.At})
		if err != nil {
			return res, err
		}
		res.Commitment, res.Title = &id, title
	}
	return res, nil
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
