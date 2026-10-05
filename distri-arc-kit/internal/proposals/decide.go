package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/jobs"
	"distri-arc/internal/outbox"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// RejectReasons are the reasons a human gives when rejecting (07-api › decide).
var RejectReasons = map[string]string{
	"tidak_tepat_waktu":      "Tidak tepat waktu",
	"salah_dealer":           "Salah dealer / kontak",
	"sudah_dilakukan":        "Sudah dilakukan",
	"tidak_sesuai_kebijakan": "Tidak sesuai kebijakan",
	"konteks_kurang":         "Konteks agen kurang",
}

// Decision is a human decision.
type Decision struct {
	Decision   string `json:"decision"` // approve | edit | reject | option
	Option     string `json:"option"`   // option key for "option" (hold, partial, override, check, skip)
	Reason     string `json:"reason"`
	ReasonText string `json:"reason_text"`
	Preview    string `json:"preview"` // edited WhatsApp draft
}

// Decider is the human who decides.
type Decider struct {
	SalesUserID uuid.UUID
	Name, Role  string
	Email       string
}

// Outcome is what deciding did.
type Outcome struct {
	Status   string     `json:"status"`
	Result   string     `json:"result"`
	OutboxID *uuid.UUID `json:"outbox_id,omitempty"`
}

// Errors returned by Decide.
var (
	ErrNotOpen   = errors.New("proposal is no longer open")
	ErrForbidden = errors.New("role not allowed to decide this proposal")
	ErrReason    = errors.New("rejecting needs a reason")
)

// Decide records the decision; approvals that reach a dealer become outbox rows (sent by the worker).
func Decide(ctx context.Context, st *store.Store, ins *river.Client[pgx.Tx], c clock.Clock, odooWrite bool, id uuid.UUID, who Decider, d Decision) (Outcome, error) {
	p, err := st.Q.GetProposal(ctx, id)
	if err != nil {
		return Outcome{}, err
	}
	if p.Status != "proposed" {
		return Outcome{}, ErrNotOpen
	}
	switch p.Kind {
	case domain.KindCreditRelease:
		if who.Role != "ceo" {
			return Outcome{}, fmt.Errorf("%w: rilis di atas limit butuh approve CEO", ErrForbidden)
		}
	case domain.KindCreditLimit:
		if who.Role != "ceo" && who.Role != "finance" {
			return Outcome{}, fmt.Errorf("%w: perubahan limit oleh CEO atau finance", ErrForbidden)
		}
	}
	var options []domain.Option
	_ = json.Unmarshal(p.Options, &options)
	var opt *domain.Option
	key := d.Option
	if d.Decision == "approve" && key == "" {
		key = "approve"
	}
	for i := range options {
		if options[i].Key == key {
			opt = &options[i]
		}
	}
	if d.Decision == "option" && opt == nil {
		return Outcome{}, fmt.Errorf("unknown option %q", d.Option)
	}
	if d.Decision == "option" && key == "reject" {
		d.Decision = "reject"
	}
	now := c.Now()
	out := Outcome{}
	err = st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		status, reason := "approved", ""
		var edited json.RawMessage
		switch d.Decision {
		case "reject":
			label, ok := RejectReasons[d.Reason]
			if !ok {
				return ErrReason
			}
			status, reason = "rejected", label
			if d.ReasonText != "" {
				reason += " — " + d.ReasonText
			}
		case "edit":
			if strings.TrimSpace(d.Preview) == "" {
				return errors.New("edited draft is empty")
			}
			status = "edited"
			edited, _ = json.Marshal(map[string]string{"preview": d.Preview})
		case "approve", "option":
			if key == "skip" {
				status, reason = "expired", "Ditunda oleh "+who.Name
			}
		default:
			return fmt.Errorf("unknown decision %q", d.Decision)
		}
		var chosen *string
		if key != "" {
			chosen = &key
		}
		uid := who.SalesUserID
		row, err := q.DecideProposal(ctx, gen.DecideProposalParams{ID: id, Status: status, DecidedBy: &uid, DecidedAt: &now, DecisionReason: strp(reason), EditedPayload: edited, ChosenOption: chosen})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotOpen
		}
		if err != nil {
			return err
		}
		out.Status = status
		if opt != nil {
			out.Result = opt.Result
		}
		switch status {
		case "rejected":
			until := clock.Today(now).AddDate(0, 0, 14)
			agent, kind, decision := row.Agent, row.Kind, "rejected"
			if err := q.InsertCalibration(ctx, gen.InsertCalibrationParams{ProposalID: &id, Agent: &agent, DealerID: row.DealerID, Kind: &kind, Decision: &decision, Reason: &reason, SuppressUntil: &until}); err != nil {
				return err
			}
			if out.Result == "" {
				out.Result = "Ditolak · dicatat untuk kalibrasi"
			}
		case "approved", "edited":
			preview := deref(row.Preview)
			if opt != nil && opt.Preview != "" {
				preview = opt.Preview
			}
			if status == "edited" {
				preview = d.Preview
			}
			sends := domain.SendsWA(row.Kind) && preview != "" && (opt == nil || opt.Sends || key == "approve")
			var obID *uuid.UUID
			switch {
			case row.Kind == domain.KindPushStock:
				n, err := spawnPerDealer(ctx, q, tx, ins, row, who, now)
				if err != nil {
					return err
				}
				out.Status = "executed"
				if out.Result == "" {
					out.Result = fmt.Sprintf("Bundle disetujui · %d draft masuk antrean sales", n)
				}
			case row.Kind == domain.KindNewDealer:
				var pl map[string]any
				_ = json.Unmarshal(row.Payload, &pl)
				payload, _ := json.Marshal(map[string]any{"create_partner": pl, "approved_by": who.Name, "note": "Dealer baru dari Distri ARC · proposal " + id.String()})
				ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: id, Channel: "odoo_note", Payload: payload})
				if err != nil {
					return err
				}
				obID = &ob.ID
				if out.Result == "" {
					out.Result = fmt.Sprintf("%v dicatat sebagai dealer tier C · dibuat di Odoo saat tulis-balik aktif", pl["name"])
				}
			case sends:
				obID, err = queueWA(ctx, q, tx, ins, row, preview, now)
				if err != nil {
					return err
				}
			case row.Kind == domain.KindSODraft && odooWrite:
				payload, _ := json.Marshal(map[string]any{"proposal": row.Payload, "approved_by": who.Name})
				ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: id, Channel: "odoo_so_draft", Payload: payload})
				if err != nil {
					return err
				}
				obID = &ob.ID
			case row.Kind == domain.KindCreditLimit:
				var pl map[string]any
				_ = json.Unmarshal(row.Payload, &pl)
				limit := pl["new_limit"]
				if key == "partial" {
					limit = pl["partial_limit"]
				}
				payload, _ := json.Marshal(map[string]any{"note": fmt.Sprintf("Limit kredit diubah ke %v · disetujui %s · proposal %s", limit, who.Name, id), "new_limit": limit})
				ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: id, Channel: "odoo_note", Payload: payload})
				if err != nil {
					return err
				}
				obID = &ob.ID
			}
			out.OutboxID = obID
			if obID == nil { // nothing leaves the system: the decision itself completes the proposal
				if err := q.SetProposalStatus(ctx, gen.SetProposalStatusParams{ID: id, Status: "executed"}); err != nil {
					return err
				}
				out.Status = "executed"
			}
			if out.Result == "" {
				out.Result = row.Title + " · dijalankan"
			}
		}
		if err := q.SyncPlanStatus(ctx, clock.Today(now)); err != nil {
			return err
		}
		actor, actorKind, action, entity := who.Email, "user", "proposal.decide", "proposal"
		after, _ := json.Marshal(map[string]any{"decision": d.Decision, "option": key, "status": out.Status, "reason": reason, "outbox_id": out.OutboxID})
		if err := q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &actorKind, Action: &action, Entity: &entity, EntityID: &id, After: after}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "select pg_notify('proposal_changed', $1)", fmt.Sprintf(`{"id":%q,"status":%q}`, id, out.Status))
		return err
	})
	return out, err
}

// queueWA writes the outbox row for a WhatsApp draft: from the dealer owner's number to the contact the
// proposal addresses (else the main contact), plus a pending bubble in the chat thread.
func queueWA(ctx context.Context, q *gen.Queries, tx pgx.Tx, ins *river.Client[pgx.Tx], p gen.Proposal, text string, now time.Time) (*uuid.UUID, error) {
	if p.DealerID == nil {
		return nil, errors.New("proposal without dealer cannot be sent")
	}
	d, err := q.GetDealer(ctx, ptrStr(p.DealerID.String()))
	if err != nil {
		return nil, err
	}
	if d.OwnerID == nil || d.OwnerWa == nil {
		return nil, errors.New("dealer has no sales number")
	}
	var pl map[string]any
	_ = json.Unmarshal(p.Payload, &pl)
	var contact gen.Contact
	if name, _ := pl["to"].(string); name != "" {
		contact, _ = q.FindContactByName(ctx, gen.FindContactByNameParams{DealerID: p.DealerID, Name: &name})
	}
	if contact.WaNumber == nil {
		cs, err := q.ListDealerContacts(ctx, p.DealerID)
		if err != nil || len(cs) == 0 {
			return nil, errors.New("dealer has no contact number")
		}
		contact = cs[0]
		for _, c := range cs {
			if c.IsPrimary {
				contact = c
			}
		}
	}
	jid := wa.UserJID(deref(contact.WaNumber))
	payload := outbox.WAPayload{From: *d.OwnerWa, To: jid, Text: text, Kind: p.Kind}
	if tid, err := q.FindThreadBySalesJID(ctx, gen.FindThreadBySalesJIDParams{SalesID: d.OwnerID, WaJid: &jid}); err == nil {
		dir, name, pending := "out", deref(d.OwnerName), "outbox:"+p.ID.String()
		mid, err := q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &tid, WaMsgID: &pending, Direction: &dir, FromNumber: d.OwnerWa, FromName: &name, Body: &text, SentAt: now, Status: "pending", ProposalID: &p.ID})
		if err == nil {
			payload.MessageID, payload.ThreadID = mid.String(), tid.String()
			if err := q.TouchThread(ctx, gen.TouchThreadParams{ID: tid, LastMessageAt: &now}); err != nil {
				return nil, err
			}
		}
	}
	b, _ := json.Marshal(payload)
	ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: p.ID, Channel: "wa", ToRef: &jid, Payload: b})
	if err != nil {
		return nil, err
	}
	if ins != nil {
		if _, err := ins.InsertTx(ctx, tx, jobs.OutboxSendArgs{OutboxID: ob.ID.String()}, nil); err != nil {
			return nil, err
		}
	}
	return &ob.ID, nil
}

func ptrStr(s string) *string { return &s }

// ApproveBySystem carries out an auto step the Orchestrator may send on its own (autonomy.guard.dealer_messages =
// "auto", ADR 0008): approved without a human, recorded as such, queued through the outbox like any approval.
func ApproveBySystem(ctx context.Context, st *store.Store, ins *river.Client[pgx.Tx], c clock.Clock, id uuid.UUID) (Outcome, error) {
	now := c.Now()
	out := Outcome{}
	err := st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		reason := "otonom · dikirim Orchestrator dalam batas kebijakan"
		row, err := q.DecideProposal(ctx, gen.DecideProposalParams{ID: id, Status: "approved", DecidedAt: &now, DecisionReason: &reason})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotOpen
		}
		if err != nil {
			return err
		}
		out.Status = "approved"
		if domain.SendsWA(row.Kind) && deref(row.Preview) != "" {
			obID, err := queueWA(ctx, q, tx, ins, row, deref(row.Preview), now)
			if err != nil {
				return err
			}
			out.OutboxID = obID
		}
		actor, actorKind, action, entity := "orchestrator", "system", "proposal.auto", "proposal"
		after, _ := json.Marshal(map[string]any{"status": out.Status, "outbox_id": out.OutboxID})
		return q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &actorKind, Action: &action, Entity: &entity, EntityID: &id, After: after})
	})
	return out, err
}

// spawnPerDealer turns an approved bundle into one WhatsApp draft per dealer, each from the dealer owner's number,
// each carrying the same human decision (and its own outbox row).
func spawnPerDealer(ctx context.Context, q *gen.Queries, tx pgx.Tx, ins *river.Client[pgx.Tx], parent gen.Proposal, who Decider, now time.Time) (int, error) {
	var pl struct {
		Name    string `json:"name"`
		Dealers []struct {
			ID      uuid.UUID `json:"id"`
			Name    string    `json:"name"`
			To      string    `json:"to"`
			Preview string    `json:"preview"`
		} `json:"dealers"`
	}
	_ = json.Unmarshal(parent.Payload, &pl)
	n := 0
	reason := "bagian dari bundle yang disetujui"
	for _, d := range pl.Dealers {
		if d.Preview == "" {
			continue
		}
		dealerID := d.ID
		payload, _ := json.Marshal(map[string]any{"parent": parent.ID, "to": d.To, "sku": pl.Name})
		key := fmt.Sprintf("push-child:%s:%s", parent.ID, d.ID)
		empty, _ := json.Marshal([]any{})
		child, err := q.InsertAgentProposal(ctx, gen.InsertAgentProposalParams{
			CycleID: parent.CycleID, Agent: parent.Agent, DealerID: &dealerID, Kind: domain.KindPushStock, Title: fmt.Sprintf("Bundle %s · %s", pl.Name, d.Name),
			Why: parent.Why, Preview: &d.Preview, Steps: empty, Impact: empty, Confidence: parent.Confidence, SignalIds: parent.SignalIds,
			Autonomy: "approve", Status: "approved", Pills: empty, Options: empty, Payload: payload, DedupeKey: &key, Icon: parent.Icon, CreatedAt: now,
		})
		if err != nil {
			return n, err
		}
		uid := who.SalesUserID
		row, err := q.MarkDecided(ctx, gen.MarkDecidedParams{ID: child, DecidedBy: &uid, DecidedAt: &now, DecisionReason: &reason})
		if err != nil {
			return n, err
		}
		if _, err := queueWA(ctx, q, tx, ins, row, d.Preview, now); err != nil {
			return n, err
		}
		n++
	}
	return n, q.SetProposalStatus(ctx, gen.SetProposalStatusParams{ID: parent.ID, Status: "executed"})
}
