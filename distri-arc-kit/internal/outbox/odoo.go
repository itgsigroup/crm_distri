package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/clock"
	"distri-arc/internal/commitment"
	"distri-arc/internal/events"
	"distri-arc/internal/odoo"
	"distri-arc/internal/store/gen"
)

// ErrManual marks rows a person carries out in Odoo (internal transfer, purchase request): the row stays visible
// with status "manual" (OPEN-QUESTIONS: the picking types per company are not known yet).
var ErrManual = errors.New("dikerjakan manual di Odoo")

func partnerID(sourceID *string) int {
	if sourceID == nil {
		return 0
	}
	_, after, ok := strings.Cut(*sourceID, ":")
	if !ok {
		return 0
	}
	n, _ := strconv.Atoi(after)
	return n
}

// deliverOdoo carries out odoo_so_draft and odoo_note rows. The only writes GSI Orbit makes to Odoo are a draft
// sale order and internal notes, each with its source (CLAUDE.md §2).
func (s *Sender) deliverOdoo(ctx context.Context, ob gen.Outbox) error {
	if s.odoo == nil {
		return odoo.ErrWriteDisabled
	}
	p, err := s.st.Q.GetProposal(ctx, pidOf(ob))
	if err != nil {
		return err
	}
	var dealer gen.GetDealerRow
	if p.DealerID != nil {
		dealer, err = s.st.Q.GetDealer(ctx, ptr(p.DealerID.String()))
		if err != nil {
			return err
		}
	}
	var pl map[string]json.RawMessage
	_ = json.Unmarshal(ob.Payload, &pl)
	now := s.clock.Now()
	switch ob.Channel {
	case "odoo_so_draft":
		var prop struct {
			Lines []struct {
				Product   string  `json:"product"`
				ProductID int     `json:"product_odoo_id"`
				Qty       float64 `json:"qty"`
				Price     float64 `json:"price"`
			} `json:"lines"`
			Total  int64   `json:"total"`
			Margin float64 `json:"margin_pct"`
			Day    string  `json:"day"`
		}
		_ = json.Unmarshal(pl["proposal"], &prop)
		var by string
		_ = json.Unmarshal(pl["approved_by"], &by)
		pid := partnerID(dealer.SourceID)
		if pid == 0 {
			return fmt.Errorf("dealer %s belum tertaut ke partner Odoo", deref(dealer.Slug))
		}
		var lines []odoo.DraftLine
		var names []string
		for _, l := range prop.Lines {
			lines = append(lines, odoo.DraftLine{ProductID: l.ProductID, Qty: l.Qty, PriceUnit: l.Price})
			names = append(names, fmt.Sprintf("%.0f %s", l.Qty, l.Product))
		}
		soID, err := odoo.CreateSODraft(ctx, s.odoo, pid, lines, p.ID.String(), by)
		if err != nil {
			return err
		}
		src := fmt.Sprintf("sale.order:%d", soID)
		summary := fmt.Sprintf("SO draft #%d dibuat di Odoo dari proposal · disetujui %s", soID, by)
		linesJSON, _ := json.Marshal(prop.Lines)
		margin := prop.Margin
		number := fmt.Sprintf("Draft #%d", soID)
		if _, err := s.st.Q.InsertAIDraftOrder(ctx, gen.InsertAIDraftOrderParams{DealerID: p.DealerID, Number: &number, OrderedAt: &now, Total: prop.Total,
			MarginPct: &margin, Lines: linesJSON, SourceID: &src}); err != nil {
			return err
		}
		sigPayload, _ := json.Marshal(map[string]any{"model": "sale.order", "id": soID, "proposal_id": p.ID, "source": "distri-arc", "via": "doc", "who": "AI Order",
			"text": summary, "conclusion": fmt.Sprintf("SO draft di Odoo dengan catatan sumber · %s", strings.Join(names, " + "))})
		sig, err := s.st.Q.UpsertSignal(ctx, gen.UpsertSignalParams{Kind: "so", DealerID: p.DealerID, OccurredAt: now, DedupeKey: src + ":draft", Summary: &summary, Payload: sigPayload})
		if err != nil {
			return err
		}
		title := "Kirim " + strings.Join(names, " + ")
		var due *time.Time
		if prop.Day != "" {
			title += " " + prop.Day
			d := commitment.NextWeekday(clock.Today(now), prop.Day)
			due = &d
		}
		key := "proposal:" + p.ID.String()
		detail := fmt.Sprintf("SO draft #%d · Odoo", soID)
		if _, err := s.st.Q.AdoptKamiCommitment(ctx, gen.AdoptKamiCommitmentParams{ProposalID: &p.ID, Detail: &detail, DueAt: due, SignalID: sig, DealerID: p.DealerID}); err == nil {
			return s.st.Q.SetProposalExecuted(ctx, p.ID)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := s.st.Q.InsertCommitment(ctx, gen.InsertCommitmentParams{DealerID: p.DealerID, Side: "kami", Title: title, Detail: &detail, DueAt: due,
			ProposalID: &p.ID, SignalIds: []uuid.UUID{sig}, SourceKey: &key, CreatedAt: now}); err != nil {
			return err
		}
		return s.st.Q.SetProposalExecuted(ctx, p.ID)
	case "odoo_note":
		var body string
		_ = json.Unmarshal(pl["note"], &body)
		if raw, ok := pl["create_partner"]; ok {
			var np struct {
				WANumber string `json:"wa_number"`
				Name     string `json:"name"`
				City     string `json:"city"`
			}
			_ = json.Unmarshal(raw, &np)
			id, err := s.odoo.Create(ctx, "res.partner", map[string]any{"name": np.Name, "mobile": "+" + np.WANumber, "city": np.City, "is_company": true,
				"comment": body + " · tier C · cash"})
			if err != nil {
				return err
			}
			_, err = s.odoo.PostNote(ctx, "res.partner", id, body)
			return err
		}
		if _, ok := pl["internal_transfer"]; ok {
			return ErrManual
		}
		if _, ok := pl["purchase_request"]; ok {
			return ErrManual
		}
		pid := partnerID(dealer.SourceID)
		if pid == 0 {
			return fmt.Errorf("dealer belum tertaut ke partner Odoo")
		}
		_, err := s.odoo.PostNote(ctx, "res.partner", pid, body)
		return err
	}
	return fmt.Errorf("channel %s tidak dikenal", ob.Channel)
}

// trail records a sent message as a signal (04-orchestrator › Eksekusi: "jejak kirim") so the dealer's Timeline shows
// "Terkirim 09.12 oleh Andi · dari proposal …".
func (s *Sender) trail(ctx context.Context, ob gen.Outbox, p WAPayload, at time.Time) error {
	if p.Kind == "reply" {
		return nil // a human reply is already a chat message of the thread
	}
	pr, err := s.st.Q.GetProposal(ctx, pidOf(ob))
	if err != nil {
		return err
	}
	who := "Sales"
	if su, err := s.st.Q.GetSalesByNumber(ctx, &p.From); err == nil {
		who = su.Name
	}
	text := p.Text
	if r := []rune(text); len(r) > 140 {
		text = string(r[:140]) + "…"
	}
	payload, _ := json.Marshal(map[string]any{"via": "chat", "who": who, "text": text, "proposal_id": pr.ID, "direction": "out",
		"conclusion": fmt.Sprintf("Terkirim %s oleh %s · dari proposal %s · %s", at.In(clock.WIB).Format("15.04"), who, pr.ID.String()[:8], pr.Title)})
	summary := "Terkirim: " + text
	_, err = s.st.Q.UpsertSignal(ctx, gen.UpsertSignalParams{Kind: "manual", DealerID: pr.DealerID, OccurredAt: at, DedupeKey: "outbox:" + ob.ID.String(), Summary: &summary, Payload: payload})
	return err
}

// notifyFailed tells open screens that a row could not be delivered (toast) — the proposal stays approved.
func (s *Sender) notifyFailed(ctx context.Context, ob gen.Outbox, err error) {
	_ = events.Notify(ctx, s.st.Pool, "outbox_failed", map[string]string{"outbox_id": ob.ID.String(), "proposal_id": pidOf(ob).String(), "channel": ob.Channel, "error": err.Error()})
}

func ptr[T any](v T) *T { return &v }

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
