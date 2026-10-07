// Package agents holds the six AI agents (05-agents.md). An agent is a function from Input to proposals: it
// computes candidates and numbers in Go first, then asks the LLM router only to phrase the reason and the
// WhatsApp draft. Agents never write to the database or call WhatsApp/Odoo; the runner (and from stage 06 the
// Orchestrator) stores what they return.
package agents

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/views"
)

// Agent is one of the six agents.
type Agent interface {
	Name() string
	Kinds() []string
	Analyze(ctx context.Context, in *Input, r *llm.Router) ([]domain.Proposal, error)
}

// Signal is a signal shown to an agent (summary + payload, already masked by the router when sent to a model).
type Signal struct {
	ID         uuid.UUID
	Kind       string
	At         time.Time
	Text       string
	Conclusion string
	ContactID  *uuid.UUID
	Contact    string
}

// Dealer is everything an agent may read about one dealer.
type Dealer struct {
	views.BoardItem
	Contacts       []domain.Contact
	Memo           string
	Signals        []Signal // newest first, at most 20
	OpenInvoices   []views.OpenInvoice
	SalesWA        string
	MonthlyOrders  []int64 // last 6 calendar months, oldest first
	LastFollowupAt *time.Time
	Commitments    []Commitment // open and late, both sides
}

// Commitment is a two-way commitment of a dealer (Kami = GSI promised, Mereka = the dealer promised).
type Commitment struct {
	ID        uuid.UUID
	Side      string
	Title     string
	Detail    string
	Status    string // open | late
	DueAt     *time.Time
	Invoice   string
	SignalIDs []uuid.UUID
}

// LatePromise is the dealer's broken payment promise, if any.
func (d *Dealer) LatePromise() *Commitment {
	for i := range d.Commitments {
		c := &d.Commitments[i]
		if c.Side == "mereka" && c.Status == "late" && strings.HasPrefix(c.Title, "Bayar") {
			return c
		}
	}
	return nil
}

// Product is a catalog item with tier prices.
type Product struct {
	ID       uuid.UUID
	OdooID   int
	SKU      string
	Name     string
	Category string
	Prices   map[string]int64
	List     int64 // list price (imported data: the last selling price) when there are no tier prices
	Cost     int64
}

// Price returns the tier price (A when the tier is unknown).
func (p Product) Price(tier string) int64 {
	if v, ok := p.Prices[tier]; ok && v > 0 {
		return v
	}
	if v := p.Prices["A"]; v > 0 {
		return v
	}
	return p.List
}

// Input is what the runner (Orchestrator) prepares for the agents.
type Input struct {
	CycleID  *string
	Today    time.Time
	Policies domain.PolicySet
	Dealers  []*Dealer
	Stock    []domain.StockItem
	Products []Product
	WA       []WAMessage // inbound dealer messages of the last days
	Examples []json.RawMessage

	Catalog      map[string]Product   // every product by name (stock SKUs included)
	StockSignals map[string]uuid.UUID // latest stock signal per "name|branch"
	NewNumbers   []NewNumber          // inbound unknown numbers with an identification (AI Prospek)
}

// NewNumber is an inbound number that is not a dealer contact yet, with what identification found.
type NewNumber struct {
	ThreadID  uuid.UUID
	WANumber  string
	Name      string // best name across sources
	Org       string // "Pati · toko CCTV & sound"
	Score     int
	Sources   []IdentSource
	Potential string
	Sales     string // sales number that received it
	SignalID  uuid.UUID
	Text      string
	At        time.Time
}

// IdentSource is one identification source (profil WA Business, Getcontact, Odoo).
type IdentSource struct {
	Source string `json:"source"`
	Value  string `json:"value"`
	OK     string `json:"ok"`
}

// WAMessage is an inbound WhatsApp message from a dealer contact.
type WAMessage struct {
	SignalID uuid.UUID
	DealerID uuid.UUID
	Contact  string
	At       time.Time
	Text     string
}

// Dealer looks a dealer up by uuid.
func (in *Input) Dealer(id uuid.UUID) *Dealer {
	for _, d := range in.Dealers {
		if d.UUID == id {
			return d
		}
	}
	return nil
}

// DealerBySlug looks a dealer up by its board id (slug).
func (in *Input) DealerBySlug(slug string) *Dealer {
	for _, d := range in.Dealers {
		if d.ID == slug {
			return d
		}
	}
	return nil
}

// polish asks the model to rewrite why/preview of a proposal from the facts; on any failure the Go template stays.
func polish(ctx context.Context, r *llm.Router, in *Input, p *domain.Proposal, prompt string, facts map[string]any, names []string) {
	if r == nil {
		return
	}
	type answer struct {
		Why        string  `json:"why"`
		Preview    string  `json:"preview"`
		Confidence float64 `json:"confidence"`
	}
	fallback, _ := json.Marshal(answer{Why: p.Why, Preview: p.Preview, Confidence: p.Confidence})
	facts["template"] = map[string]string{"why": p.Why, "preview": p.Preview}
	input, _ := json.Marshal(facts)
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"why", "preview", "confidence"},
		"properties": map[string]any{"why": map[string]any{"type": "string"}, "preview": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number"}}}
	res := r.Complete(ctx, llm.Request{Purpose: p.Kind + ".draft", Agent: p.Agent, CycleID: in.CycleID, System: llm.Prompt("system") + "\n\n" + llm.Prompt(prompt),
		Input: input, Schema: schema, MaxTokens: 1500, Names: names, Fallback: fallback}, func(b json.RawMessage) error {
		var a answer
		if err := json.Unmarshal(b, &a); err != nil {
			return err
		}
		if a.Why == "" || (p.Preview != "" && a.Preview == "") || a.Confidence < 0 || a.Confidence > 1 {
			return errInvalid
		}
		return nil
	})
	var a answer
	if json.Unmarshal(res.JSON, &a) != nil {
		return
	}
	p.Why = a.Why
	if p.Preview != "" {
		p.Preview = a.Preview
	}
	switch {
	case res.Provider == "template": // the model failed: template text with reduced confidence (05-agents)
		p.Confidence = 0.6
	case res.Provider != "fake":
		p.Confidence = math.Round(a.Confidence*100) / 100
	}
}

type invalidErr struct{}

func (invalidErr) Error() string { return "answer does not satisfy the schema" }

var errInvalid = invalidErr{}

// Latest keeps, per dealer and intent, only the newest WhatsApp message (a request is often both in a chat and
// in the dealer timeline).
func Latest(in *Input) []WAMessage {
	type key struct {
		d      uuid.UUID
		intent string
	}
	best := map[key]WAMessage{}
	var order []key
	for _, m := range in.WA {
		k := key{m.DealerID, Extract(m.Text, in.Products).Intent}
		if prev, ok := best[k]; !ok || m.At.After(prev.At) {
			if !ok {
				order = append(order, k)
			}
			best[k] = m
		}
	}
	out := make([]WAMessage, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}
