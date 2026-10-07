package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/jobs"
	"distri-arc/internal/outbox"
	"distri-arc/internal/policy"
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
	// Decide narrows the kinds this person decides (their role in the role master); nil = all their Role allows.
	Decide   []string
	RoleName string
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
	if err := authorize(ctx, st.Q, p.Kind, p.DealerID, p.Payload, who); err != nil {
		return Outcome{}, err
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
	pol, err := policy.Load(ctx, st.Q)
	if err != nil {
		return Outcome{}, err
	}
	shadow := pol.Pilot.Shadow()
	if shadow { // pilot shadow mode: the decision calibrates the agents, nothing leaves Distri ARC
		ins, odooWrite = nil, false
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
			var obID, odooOB *uuid.UUID
			switch {
			case row.Kind == domain.KindPlanChange:
				if err := applyPlanChange(ctx, q, row, now); err != nil {
					return err
				}
			case row.Kind == domain.KindPushStock:
				n, err := spawnPerDealer(ctx, q, tx, ins, row, who, now)
				if err != nil {
					return err
				}
				out.Status = "executed"
				if out.Result == "" {
					out.Result = fmt.Sprintf("Bundle disetujui · %d draft masuk antrean sales", n)
				}
			case row.Kind == domain.KindTransfer || row.Kind == domain.KindPORequest:
				var pl map[string]any
				_ = json.Unmarshal(row.Payload, &pl)
				what := map[string]string{domain.KindTransfer: "internal_transfer", domain.KindPORequest: "purchase_request"}[row.Kind]
				payload, _ := json.Marshal(map[string]any{what: pl, "approved_by": who.Name, "note": row.Title + " · proposal " + id.String()})
				ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &id, Channel: "odoo_note", Payload: payload})
				if err != nil {
					return err
				}
				obID, odooOB = &ob.ID, &ob.ID
				if out.Result == "" {
					out.Result = row.Title + " · dicatat untuk gudang/Purchasing"
				}
			case row.Kind == domain.KindNewDealer:
				var pl map[string]any
				_ = json.Unmarshal(row.Payload, &pl)
				payload, _ := json.Marshal(map[string]any{"create_partner": pl, "approved_by": who.Name, "note": "Dealer baru dari Distri ARC · proposal " + id.String()})
				ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &id, Channel: "odoo_note", Payload: payload})
				if err != nil {
					return err
				}
				obID, odooOB = &ob.ID, &ob.ID
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
				ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &id, Channel: "odoo_so_draft", Payload: payload})
				if err != nil {
					return err
				}
				obID, odooOB = &ob.ID, &ob.ID
			case row.Kind == domain.KindCreditLimit:
				var pl map[string]any
				_ = json.Unmarshal(row.Payload, &pl)
				limit := pl["new_limit"]
				if key == "partial" {
					limit = pl["partial_limit"]
				}
				payload, _ := json.Marshal(map[string]any{"note": fmt.Sprintf("Limit kredit diubah ke %v · disetujui %s · proposal %s", limit, who.Name, id), "new_limit": limit})
				ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &id, Channel: "odoo_note", Payload: payload})
				if err != nil {
					return err
				}
				obID, odooOB = &ob.ID, &ob.ID
			}
			out.OutboxID = obID
			if odooOB != nil && odooWrite && ins != nil { // written by the worker; without ODOO_WRITE the row waits
				if _, err := ins.InsertTx(ctx, tx, jobs.OutboxSendArgs{OutboxID: odooOB.String()}, nil); err != nil {
					return err
				}
			}
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
		if err := decisionNote(ctx, q, tx, ins, odooWrite, row, out.Status, who, reason); err != nil {
			return err
		}
		if shadow {
			n, err := q.ShadowOutbox(ctx, &id)
			if err != nil {
				return err
			}
			if err := q.DropPendingBubble(ctx, &id); err != nil {
				return err
			}
			if n > 0 {
				out.Result = row.Title + " · disetujui · mode bayangan: tidak dikirim"
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

// ErrShadow refuses an automatic send while the pilot runs in shadow mode.
var ErrShadow = errors.New("mode bayangan pilot: tidak ada langkah otomatis")

// queueWA writes the outbox row for a WhatsApp draft: from the dealer owner's number to the contact the
// proposal addresses (else the main contact), plus a pending bubble in the chat thread.
func queueWA(ctx context.Context, q *gen.Queries, tx pgx.Tx, ins *river.Client[pgx.Tx], p gen.Proposal, text string, now time.Time) (*uuid.UUID, error) {
	if p.DealerID == nil {
		return queueThreadWA(ctx, q, tx, ins, p, text, now)
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
	tid, err := q.FindThreadByAccountJID(ctx, gen.FindThreadByAccountJIDParams{Account: d.OwnerWa, WaJid: &jid})
	if errors.Is(err, pgx.ErrNoRows) {
		// first message to this contact: open the thread so the dealer's answer can be linked back (reply tracking)
		kind, title, subtitle := "dealer", deref(contact.Name)+" · "+wa.ShortDealer(d.Name), d.Name+" · "+deref(contact.Role)
		var t gen.ChatThread
		t, err = q.InsertThread(ctx, gen.InsertThreadParams{Kind: &kind, DealerID: p.DealerID, ContactID: &contact.ID, WaJid: &jid, Title: &title, Subtitle: &subtitle, SalesID: d.OwnerID, LastMessageAt: &now, Account: d.OwnerWa})
		tid = t.ID
	}
	if err == nil {
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
	ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: &jid, Payload: b})
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
	if pol, err := policy.Load(ctx, st.Q); err != nil {
		return Outcome{}, err
	} else if pol.Pilot.Shadow() {
		return Outcome{}, ErrShadow
	}
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

// applyPlanChange carries out an approved orchestrator.plan.update (move | skip | add).
func applyPlanChange(ctx context.Context, q *gen.Queries, p gen.Proposal, now time.Time) error {
	var pl struct {
		Action string `json:"action"`
		ItemID string `json:"item_id"`
		Time   string `json:"time"`
		Text   string `json:"text"`
		Client string `json:"client"`
	}
	_ = json.Unmarshal(p.Payload, &pl)
	switch pl.Action {
	case "move", "skip":
		id, err := uuid.Parse(pl.ItemID)
		if err != nil {
			return err
		}
		if pl.Action == "skip" {
			return q.SetPlanStatus(ctx, gen.SetPlanStatusParams{ID: id, Status: domain.PlanSkipped})
		}
		return q.MovePlanItem(ctx, gen.MovePlanItemParams{ID: id, TimeLabel: &pl.Time})
	case "add":
		day := clock.Today(now)
		seq, err := q.NextPlanSeq(ctx, day)
		if err != nil {
			return err
		}
		agent, auto := "MCP · "+pl.Client, "approve"
		return q.InsertPlanItem(ctx, gen.InsertPlanItemParams{PlanDate: day, CycleID: p.CycleID, Seq: seq, TimeLabel: &pl.Time, Agent: &agent, Autonomy: &auto,
			TextHtml: &pl.Text, Status: domain.PlanScheduled}) // a step of its own: the plan_change proposal is already decided
	}
	return fmt.Errorf("unknown plan action %q", pl.Action)
}

// queueThreadWA sends to the thread a proposal is about when there is no dealer yet (a new inbound number):
// from the sales number that received the message, to that number.
func queueThreadWA(ctx context.Context, q *gen.Queries, tx pgx.Tx, ins *river.Client[pgx.Tx], p gen.Proposal, text string, now time.Time) (*uuid.UUID, error) {
	var pl struct {
		ThreadID uuid.UUID `json:"thread_id"`
	}
	_ = json.Unmarshal(p.Payload, &pl)
	if pl.ThreadID == uuid.Nil {
		return nil, errors.New("proposal without dealer or thread cannot be sent")
	}
	t, err := q.GetThread(ctx, pl.ThreadID)
	if err != nil {
		return nil, err
	}
	if t.SalesWa == nil || t.WaJid == nil {
		return nil, errors.New("thread has no sales number")
	}
	jid := *t.WaJid
	payload := outbox.WAPayload{From: *t.SalesWa, To: jid, Text: text, Kind: p.Kind}
	dir, name, pending := "out", deref(t.SalesName), "outbox:"+p.ID.String()
	if mid, err := q.InsertChatMessage(ctx, gen.InsertChatMessageParams{ThreadID: &t.ID, WaMsgID: &pending, Direction: &dir, FromNumber: t.SalesWa, FromName: &name, Body: &text, SentAt: now, Status: "pending", ProposalID: &p.ID}); err == nil {
		payload.MessageID, payload.ThreadID = mid.String(), t.ID.String()
		if err := q.TouchThread(ctx, gen.TouchThreadParams{ID: t.ID, LastMessageAt: &now}); err != nil {
			return nil, err
		}
	}
	b, _ := json.Marshal(payload)
	ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "wa", ToRef: &jid, Payload: b})
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

// Who may decide which proposals (09-policies-security › RBAC). The CEO decides everything; a release above the
// limit only the CEO. Sales decide proposals of the dealers they own (and the new numbers that wrote to them).
var deciders = map[string][]string{
	domain.KindCreditRelease: {"ceo"},
	domain.KindCreditLimit:   {"ceo", "finance"},
	domain.KindCollect:       {"ceo", "admin", "finance", "sales"},
	domain.KindInstallment:   {"ceo", "admin", "finance", "sales"},
	domain.KindTransfer:      {"ceo", "admin", "warehouse"},
	domain.KindPORequest:     {"ceo", "admin", "warehouse"},
	domain.KindPlanChange:    {"ceo", "admin"},
	domain.KindPushStock:     {"ceo", "admin"},
	domain.KindPriceCounter:  {"ceo", "admin"},
}

var roleLabel = map[string]string{"ceo": "CEO", "admin": "admin", "finance": "finance", "sales": "sales pemilik dealer", "warehouse": "gudang"}

func kindLabel(kind string) string {
	l := map[string]string{domain.KindCreditLimit: "Perubahan limit", domain.KindCollect: "Pengingat penagihan", domain.KindInstallment: "Skema cicilan",
		domain.KindTransfer: "Transfer stok", domain.KindPORequest: "Permintaan PO", domain.KindPlanChange: "Perubahan rencana", domain.KindPushStock: "Bundle stok",
		domain.KindPriceCounter: "Harga khusus"}[kind]
	if l == "" {
		l = "Saran ini"
	}
	return l
}

// defaultDeciders applies to the other kinds (follow-up, SO draft, return, new dealer, price list, reply).
var defaultDeciders = []string{"ceo", "admin", "sales"}

// RolesFor lists the roles that may decide a kind.
func RolesFor(kind string) []string {
	if r, ok := deciders[kind]; ok {
		return r
	}
	return defaultDeciders
}

func authorize(ctx context.Context, q *gen.Queries, kind string, dealerID *uuid.UUID, payload json.RawMessage, who Decider) error {
	roles := RolesFor(kind)
	if who.Decide != nil && slices.Contains(roles, who.Role) && !slices.Contains(who.Decide, kind) {
		return fmt.Errorf("%w: peran %s tidak memutuskan %s — atur di Pengaturan → Peran & akses", ErrForbidden, who.RoleName, strings.ToLower(kindLabel(kind)))
	}
	if !slices.Contains(roles, who.Role) {
		if kind == domain.KindCreditRelease {
			return fmt.Errorf("%w: rilis di atas limit butuh approve CEO", ErrForbidden)
		}
		var names []string
		for _, r := range roles {
			names = append(names, roleLabel[r])
		}
		return fmt.Errorf("%w: %s diputuskan oleh %s", ErrForbidden, kindLabel(kind), strings.Join(names, ", "))
	}
	if who.Role != "sales" {
		return nil
	}
	// sales: only their own dealers, or the new number that wrote to their WhatsApp
	if dealerID != nil {
		d, err := q.GetDealer(ctx, ptrStr(dealerID.String()))
		if err != nil {
			return err
		}
		if d.OwnerID == nil || *d.OwnerID != who.SalesUserID {
			return fmt.Errorf("%w: dealer ini milik sales lain", ErrForbidden)
		}
		return nil
	}
	var pl struct {
		ThreadID uuid.UUID `json:"thread_id"`
	}
	_ = json.Unmarshal(payload, &pl)
	if pl.ThreadID != uuid.Nil {
		if t, err := q.GetThread(ctx, pl.ThreadID); err == nil && t.SalesID != nil && *t.SalesID == who.SalesUserID {
			return nil
		}
	}
	return fmt.Errorf("%w: bukan dealer atau chat Anda", ErrForbidden)
}

// decisionNote writes the decision as an internal note on the dealer's Odoo partner (policy odoo.write.notes,
// ODOO_WRITE=true). Kinds that already write to Odoo (SO draft, limit, new dealer, transfer, PO) are skipped.
func decisionNote(ctx context.Context, q *gen.Queries, tx pgx.Tx, ins *river.Client[pgx.Tx], odooWrite bool, p gen.Proposal, status string, who Decider, reason string) error {
	if !odooWrite || p.DealerID == nil {
		return nil
	}
	switch p.Kind {
	case domain.KindSODraft, domain.KindCreditLimit, domain.KindNewDealer, domain.KindTransfer, domain.KindPORequest, domain.KindReply:
		return nil
	}
	pol, err := policy.Load(ctx, q)
	if err != nil || !pol.OdooWrite.Notes {
		return err
	}
	verb := map[string]string{"rejected": "Ditolak", "expired": "Ditunda"}[status]
	if verb == "" {
		verb = "Disetujui"
	}
	note := fmt.Sprintf("Distri ARC · %s oleh %s · %s (%s)", verb, who.Name, p.Title, p.Agent)
	if reason != "" && status == "rejected" {
		note += " · alasan: " + reason
	}
	note += " · proposal " + p.ID.String()
	payload, _ := json.Marshal(map[string]any{"note": note})
	ob, err := q.InsertOutbox(ctx, gen.InsertOutboxParams{ProposalID: &p.ID, Channel: "odoo_note", Payload: payload})
	if err != nil {
		return err
	}
	if ins != nil {
		_, err = ins.InsertTx(ctx, tx, jobs.OutboxSendArgs{OutboxID: ob.ID.String()}, nil)
	}
	return err
}
