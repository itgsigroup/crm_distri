package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Proposal kinds.
// AgentNames are the six agents the Orchestrator directs, in the order Pengaturan → Kalibrasi lists them.
var AgentNames = []string{"AI Order", "AI Penagihan", "AI Follow-up", "AI Stok", "AI Kredit", "AI Prospek"}

const (
	KindFollowup      = "followup"
	KindCollect       = "collect"
	KindInstallment   = "installment"
	KindCreditRelease = "credit_release"
	KindCreditLimit   = "credit_limit"
	KindCreditHold    = "credit_hold"
	KindPriceCounter  = "price_counter"
	KindPushStock     = "push_stock"
	KindSODraft       = "so_draft"
	KindReturn        = "return"
	KindTransfer      = "transfer"
	KindPORequest     = "po_request"
	KindNewDealer     = "new_dealer"
	KindPriceList     = "price_list"
	KindReply         = "reply"
)

// QueueKinds are decided in "Keputusan" (outside the agents' autonomy, high stakes).
var QueueKinds = map[string]bool{KindCreditRelease: true, KindCreditLimit: true, KindPriceCounter: true, KindReturn: true}

// SendsWA reports whether approving the kind sends a WhatsApp message to the dealer.
func SendsWA(kind string) bool {
	switch kind {
	case KindFollowup, KindCollect, KindInstallment, KindCreditRelease, KindPriceCounter, KindPushStock, KindReturn, KindPriceList, KindReply:
		return true
	}
	return false
}

// Impact is one figure of the queue card ("Setelah rilis · Rp 400 jt").
type Impact struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"` // bad | warn | good
}

// Option is an alternative decision offered with a proposal.
type Option struct {
	Key     string `json:"key"` // approve | hold | partial | override | check | skip | reject
	Label   string `json:"label"`
	Style   string `json:"style"`             // primary | ghost | quiet
	Result  string `json:"result"`            // toast after deciding
	Sends   bool   `json:"sends"`             // choosing it sends the (option) WhatsApp draft to the dealer
	Preview string `json:"preview,omitempty"` // draft that replaces the proposal's preview for this option
}

// Proposal is what an agent suggests ("Action" in the UI). Every proposal carries provenance.
type Proposal struct {
	Agent      string         `json:"agent"`
	DealerID   *uuid.UUID     `json:"dealer_id"`
	Kind       string         `json:"kind"`
	Title      string         `json:"title"`
	Summary    string         `json:"summary"`
	Why        string         `json:"why"`
	Prep       string         `json:"prep"`
	Preview    string         `json:"preview"`
	Steps      []string       `json:"steps"`
	Impact     []Impact       `json:"impact"`
	Options    []Option       `json:"options"`
	Pills      [][2]string    `json:"pills"`
	Button     string         `json:"button"`
	Icon       string         `json:"icon"`
	DueLabel   string         `json:"due_label"`
	Confidence float64        `json:"confidence"`
	SignalIDs  []uuid.UUID    `json:"signal_ids"`
	Autonomy   string         `json:"autonomy"` // auto | approve
	Payload    map[string]any `json:"payload"`
	DedupeKey  string         `json:"dedupe_key"`
}

// ErrNoProvenance rejects claims without sources (CLAUDE.md §2).
var ErrNoProvenance = errors.New("proposal without provenance (signal_ids)")

// Validate enforces the domain rules every proposal must satisfy before it is stored.
func (p Proposal) Validate(allowed []string) error {
	if len(p.SignalIDs) == 0 {
		return ErrNoProvenance
	}
	if p.Confidence < 0 || p.Confidence > 1 {
		return fmt.Errorf("confidence %.2f outside 0–1", p.Confidence)
	}
	if p.Title == "" || p.Why == "" {
		return errors.New("proposal needs a title and a reason")
	}
	if p.Autonomy != "auto" && p.Autonomy != "approve" {
		return fmt.Errorf("autonomy %q", p.Autonomy)
	}
	for _, k := range allowed {
		if k == p.Kind {
			return nil
		}
	}
	return fmt.Errorf("kind %q not allowed for %s", p.Kind, p.Agent)
}
