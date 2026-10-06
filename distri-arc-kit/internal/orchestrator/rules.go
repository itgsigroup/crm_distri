package orchestrator

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
)

// Cand is a proposal on its way through Sintesis and Keputusan.
type Cand struct {
	P      domain.Proposal
	Dealer *agents.Dealer // nil for multi-dealer proposals

	Dropped    bool   // merged into another proposal (rule dedupe)
	Suppressed string // why it does not enter the queue (suppression, followup_gap)
	NoAuto     []string
	WaitFor    string // "payment:INV/0901" — the step waits until this happened
	AutoAfter  bool   // auto once WaitFor is released (collect_before_followup)
	Blocked    bool   // touched by a contention rule (credit_over_stock, margin_floor, one_owner)
	Anchor     bool   // stored earlier today by an agent outside this run's scope: the rules see it, nothing stores it
}

// Live reports whether the candidate is still to be stored and decided.
func (c *Cand) Live() bool { return !c.Dropped && c.Suppressed == "" }

func (c *Cand) dealerID() uuid.UUID {
	if c.P.DealerID == nil {
		return uuid.Nil
	}
	return *c.P.DealerID
}

// Suppression is an active calibration event (agent, dealer, kind) until a date.
type Suppression struct {
	Agent, Kind string
	DealerID    uuid.UUID
	Until       time.Time
}

// SalesCount is how often one sales number talked with a dealer in 30 days.
type SalesCount struct {
	Sales string
	N     int64
}

// RuleInput is everything the rules read besides the candidates.
type RuleInput struct {
	Policies     domain.PolicySet
	Today        time.Time
	Suppressions []Suppression
	Lessons      []Lesson // learned from repeated rejections (Belajar)
	LastFollowup map[uuid.UUID]time.Time
	Interactions map[uuid.UUID][]SalesCount
	// Dealer looks up a dealer of a multi-dealer proposal.
	Dealer func(id uuid.UUID) *agents.Dealer
	// MakeCollect creates the AI Penagihan proposal credit_over_stock needs when none exists (nil = do not create).
	MakeCollect func(d *agents.Dealer) *domain.Proposal
}

// Rule names (conflicts.rule).
const (
	RuleSuppression     = "suppression"
	RuleFollowupGap     = "followup_gap"
	RuleDedupe          = "dedupe"
	RuleCreditOverStock = "credit_over_stock"
	RuleCollectFirst    = "collect_before_followup"
	RuleMarginFloor     = "margin_floor"
	RuleOneOwner        = "one_owner"
)

// Synthesize applies the conflict rules of 04-orchestrator in order and returns what fired. It mutates the
// candidates (merge, suppress, defer, block auto) and may append a collect proposal.
func Synthesize(cands []*Cand, in RuleInput) ([]*Cand, []domain.Conflict) {
	var out []domain.Conflict
	add := func(c domain.Conflict) { out = append(out, c) }
	cands, cs := suppression(cands, in)
	out = append(out, cs...)
	out = append(out, followupGap(cands, in)...)
	out = append(out, dedupe(cands)...)
	cands, cs = creditOverStock(cands, in)
	out = append(out, cs...)
	for _, c := range collectFirst(cands) {
		add(c)
	}
	for _, c := range marginFloor(cands, in) {
		add(c)
	}
	for _, c := range oneOwner(cands, in) {
		add(c)
	}
	return cands, out
}

func slugOf(c *Cand) *string {
	if c.Dealer == nil {
		return nil
	}
	s := c.Dealer.ID
	return &s
}

func suppression(cands []*Cand, in RuleInput) ([]*Cand, []domain.Conflict) {
	var out []domain.Conflict
	for _, c := range cands {
		if !c.Live() {
			continue
		}
		product := ProductOf(c.P.Payload)
		for _, l := range in.Lessons {
			if c.P.Kind == domain.KindPushStock && in.Dealer != nil && len(c.P.DealerIDs) > 0 {
				var keep []uuid.UUID
				for _, id := range c.P.DealerIDs {
					if !l.Applies(c.P.Agent, c.P.Kind, product, in.Dealer(id)) {
						keep = append(keep, id)
					}
				}
				if len(keep) == len(c.P.DealerIDs) {
					continue
				}
				c.P.DealerIDs = keep
				if ds, ok := c.P.Payload["dealers"].([]map[string]any); ok {
					var kept []map[string]any
					for _, d := range ds {
						if id, ok := d["id"].(uuid.UUID); ok && slices.Contains(keep, id) {
							kept = append(kept, d)
						}
					}
					c.P.Payload["dealers"] = kept
				}
				if len(keep) == 0 {
					c.Suppressed = "Pelajaran kalibrasi: " + l.Text
				}
				out = append(out, domain.Conflict{Rule: RuleSuppression, AgentA: c.P.Agent, AgentB: "Kalibrasi", Title: c.P.Title, Resolution: "Pelajaran: " + l.Text, Tone: "neutral", Keys: []string{c.P.DedupeKey}})
				continue
			}
			if l.Applies(c.P.Agent, c.P.Kind, product, c.Dealer) {
				c.Suppressed = "Pelajaran kalibrasi: " + l.Text
				out = append(out, domain.Conflict{Rule: RuleSuppression, DealerID: slugOf(c), AgentA: c.P.Agent, AgentB: "Kalibrasi", Title: c.P.Title, Resolution: "Pelajaran: " + l.Text, Tone: "neutral", Keys: []string{c.P.DedupeKey}})
				break
			}
		}
	}
	for _, c := range cands {
		if !c.Live() || c.P.DealerID == nil {
			continue
		}
		for _, s := range in.Suppressions {
			if s.Agent == c.P.Agent && s.Kind == c.P.Kind && s.DealerID == *c.P.DealerID {
				c.Suppressed = fmt.Sprintf("Ditahan kalibrasi sampai %s: saran serupa ditolak", clock.DayMonth(s.Until))
				out = append(out, domain.Conflict{Rule: RuleSuppression, DealerID: slugOf(c), AgentA: c.P.Agent, AgentB: "Kalibrasi", Title: c.P.Title,
					Resolution: c.Suppressed, Tone: "neutral", Keys: []string{c.P.DedupeKey}})
				break
			}
		}
	}
	return cands, out
}

func followupGap(cands []*Cand, in RuleInput) []domain.Conflict {
	var out []domain.Conflict
	gap := in.Policies.Followup.GapDays
	for _, c := range cands {
		if !c.Live() || c.P.Kind != domain.KindFollowup || c.P.DealerID == nil {
			continue
		}
		last, ok := in.LastFollowup[*c.P.DealerID]
		if !ok {
			continue
		}
		days := int(in.Today.Sub(last).Hours() / 24)
		if sameDay(last, in.Today) || days >= gap {
			continue // today's follow-up is the same proposal (dedupe key); older than the gap is allowed
		}
		c.Suppressed = fmt.Sprintf("Follow-up terakhir %d hari lalu (< %d hari)", days, gap)
		out = append(out, domain.Conflict{Rule: RuleFollowupGap, DealerID: slugOf(c), AgentA: c.P.Agent, AgentB: "Kebijakan follow-up", Title: c.P.Title,
			Resolution: c.Suppressed, Tone: "neutral", Keys: []string{c.P.DedupeKey}})
	}
	return out
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.In(b.Location()).Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func merge(into, from *Cand) {
	for _, id := range from.P.SignalIDs {
		if !slices.Contains(into.P.SignalIDs, id) {
			into.P.SignalIDs = append(into.P.SignalIDs, id)
		}
	}
	from.Dropped = true
}

// dedupe merges (a) a proposal whose kind another proposal for the same dealer already covers, (b) a follow-up
// offering the same aging SKU a push_stock proposal brings to that dealer, (c) two proposals of the same kind for
// one dealer from different agents (the higher confidence stays).
func dedupe(cands []*Cand) []domain.Conflict {
	var out []domain.Conflict
	rec := func(into, from *Cand, why string) {
		merge(into, from)
		out = append(out, domain.Conflict{Rule: RuleDedupe, DealerID: slugOf(from), AgentA: from.P.Agent, AgentB: into.P.Agent,
			Title: from.P.Title, Resolution: why, Tone: "neutral", Keys: []string{from.P.DedupeKey, into.P.DedupeKey}})
	}
	for _, c := range cands {
		if !c.Live() || c.P.DealerID == nil {
			continue
		}
		for _, o := range cands {
			if o == c || !o.Live() || o.P.DealerID == nil || *o.P.DealerID != *c.P.DealerID {
				continue
			}
			if o.P.Covers(c.P.Kind) {
				rec(o, c, "Digabung: "+o.P.Title)
				break
			}
		}
	}
	for _, c := range cands {
		sku, _ := c.P.Payload["bundle_sku"].(string)
		if !c.Live() || c.P.Kind != domain.KindFollowup || sku == "" || c.P.DealerID == nil {
			continue
		}
		for _, o := range cands {
			if !o.Live() || o.P.Kind != domain.KindPushStock || o.P.Payload["name"] != sku || !slices.Contains(o.P.DealerIDs, *c.P.DealerID) {
				continue
			}
			if ds, ok := o.P.Payload["dealers"].([]map[string]any); ok {
				for _, d := range ds {
					if d["id"] == *c.P.DealerID {
						d["preview"] = c.P.Preview // the dealer-specific follow-up becomes its bundle message
					}
				}
			}
			rec(o, c, "Satu tawaran: follow-up "+c.Dealer.Name+" masuk bundle "+sku)
			break
		}
	}
	for _, c := range cands {
		if !c.Live() || c.P.DealerID == nil {
			continue
		}
		for _, o := range cands {
			if o == c || !o.Live() || o.P.DealerID == nil || *o.P.DealerID != *c.P.DealerID || o.P.Kind != c.P.Kind || o.P.Agent == c.P.Agent {
				continue
			}
			keep, drop := o, c
			if c.P.Confidence > o.P.Confidence {
				keep, drop = c, o
			}
			rec(keep, drop, "Digabung ke saran dengan confidence tertinggi")
		}
	}
	return out
}

func creditBad(d *agents.Dealer) bool {
	if d == nil {
		return false
	}
	s := d.Metrics.Credit.State
	return s == domain.CreditOverLimit || s == domain.CreditOverdue
}

func lateInvoice(d *agents.Dealer) (string, int) {
	num, late := "", 0
	for _, i := range d.OpenInvoices {
		if i.LateDays > late {
			num, late = i.Number, i.LateDays
		}
	}
	return num, late
}

func findCollect(cands []*Cand, dealer uuid.UUID) *Cand {
	for _, o := range cands {
		if o.Live() && o.dealerID() == dealer && (o.P.Kind == domain.KindCollect || o.P.Kind == domain.KindInstallment || o.P.Covers(domain.KindCollect)) {
			return o
		}
	}
	return nil
}

// creditOverStock: an offer (push/follow-up) to a dealer over limit or overdue waits for the payment; AI Penagihan
// goes first. Multi-dealer pushes drop such dealers.
func creditOverStock(cands []*Cand, in RuleInput) ([]*Cand, []domain.Conflict) {
	var out []domain.Conflict
	for _, c := range cands {
		if !c.Live() {
			continue
		}
		if c.P.Kind == domain.KindPushStock && in.Dealer != nil {
			// metrics.PushCandidates already leaves such dealers out; a push built elsewhere (MCP) may not
			var keep []uuid.UUID
			for _, id := range c.P.DealerIDs {
				d := in.Dealer(id)
				if !creditBad(d) {
					keep = append(keep, id)
					continue
				}
				inv, _ := lateInvoice(d)
				if findCollect(cands, id) == nil && in.MakeCollect != nil {
					if p := in.MakeCollect(d); p != nil {
						cands = append(cands, &Cand{P: *p, Dealer: d})
					}
				}
				slug := d.ID
				out = append(out, domain.Conflict{Rule: RuleCreditOverStock, DealerID: &slug, AgentA: c.P.Agent, AgentB: "AI Kredit",
					Title:      fmt.Sprintf("Push %s ditahan — dealer %s", c.P.Payload["name"], d.Metrics.Credit.State),
					Resolution: fmt.Sprintf("Urutan: AI Penagihan menagih %s dulu → push stok setelah pembayaran masuk. Tidak ada tawaran ke dealer yang tidak bisa bayar.", inv),
					Tone:       "warn", Visible: true, Keys: []string{c.P.DedupeKey}})
			}
			if len(keep) < len(c.P.DealerIDs) {
				c.P.DealerIDs = keep
				c.P.Steps = append(c.P.Steps, "Dealer over limit / overdue menyusul setelah pembayaran masuk")
				if len(keep) == 0 {
					c.Suppressed = "Semua kandidat menunggu pembayaran"
				}
			}
			continue
		}
		offer := c.P.Kind == domain.KindFollowup || (c.P.Kind == domain.KindInstallment && c.P.Covers(domain.KindCollect)) // follow-up with a scheme
		if !offer || !creditBad(c.Dealer) {
			continue
		}
		d := c.Dealer
		inv, late := lateInvoice(d)
		stateText := d.Metrics.Credit.State
		var res string
		if c.P.Covers(domain.KindCollect) {
			res = fmt.Sprintf("Urutan: skema cicilan %s dulu (AI Penagihan) → order cash kecil; push stok setelah %s masuk. Tidak ada tawaran kredit ke dealer yang tidak bisa bayar.", inv, inv)
		} else {
			c.WaitFor = "payment:" + inv
			c.NoAuto = append(c.NoAuto, RuleCreditOverStock)
			c.P.Steps = append([]string{"Ditunda: dikirim setelah pembayaran " + inv + " masuk"}, c.P.Steps...)
			col := findCollect(cands, d.UUID)
			if col == nil && in.MakeCollect != nil {
				if p := in.MakeCollect(d); p != nil {
					col = &Cand{P: *p, Dealer: d}
					cands = append(cands, col)
				}
			}
			res = fmt.Sprintf("Urutan: AI Penagihan menagih %s dulu → follow-up dikirim setelah pembayaran masuk. Tidak ada tawaran ke dealer yang tidak bisa bayar.", inv)
		}
		c.Blocked = true
		title := fmt.Sprintf("Follow-up ditahan — dealer %s", stateText)
		if late > 0 {
			title += fmt.Sprintf(" (%s lewat %d hari)", inv, late)
		}
		out = append(out, domain.Conflict{Rule: RuleCreditOverStock, DealerID: slugOf(c), AgentA: "AI Follow-up", AgentB: "AI Kredit", Title: title,
			Resolution: res, Tone: "warn", Visible: true, Keys: []string{c.P.DedupeKey}})
	}
	return cands, out
}

// collectFirst: a follow-up for an order due within 7 days and a collect for the same dealer whose limit is not
// safe → the reminder goes first, the follow-up waits for the payment and then runs as an auto step.
func collectFirst(cands []*Cand) []domain.Conflict {
	var out []domain.Conflict
	for _, c := range cands {
		if !c.Live() || c.P.Kind != domain.KindFollowup || c.Dealer == nil || c.WaitFor != "" {
			continue
		}
		m := c.Dealer.Metrics
		if m.DueIn == nil || *m.DueIn < 0 || *m.DueIn > 7 || m.Credit.State == domain.CreditAman || m.Credit.State == domain.CreditCash {
			continue
		}
		col := findCollect(cands, c.Dealer.UUID)
		if col == nil || col.P.Kind != domain.KindCollect {
			continue
		}
		inv := ""
		if xs, ok := col.P.Payload["invoices"].([]string); ok && len(xs) > 0 {
			inv = xs[0]
		}
		c.WaitFor, c.AutoAfter = "payment:"+inv, true
		c.P.Steps = append([]string{"Menunggu pembayaran " + inv + " — lalu dijadwalkan"}, c.P.Steps...)
		room := 0
		if m.Credit.Room != nil {
			room = int(math.Round(*m.Credit.Room * 100))
		}
		out = append(out, domain.Conflict{Rule: RuleCollectFirst, DealerID: slugOf(c), AgentA: "AI Follow-up", AgentB: "AI Penagihan",
			Title:      fmt.Sprintf("Jadwal order H-%d, tapi sisa limit %d%%", *m.DueIn, room),
			Resolution: fmt.Sprintf("Pengingat %s (nada ramah) dikirim dulu; rekomendasi order dijadwalkan begitu pembayaran masuk. Dua pesan terpisah, satu tujuan.", inv),
			Tone:       "accent", Visible: true, Keys: []string{col.P.DedupeKey, c.P.DedupeKey}})
	}
	return out
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// marginFloor: a price below the floor never runs on its own; it is escalated with a counter above the floor.
func marginFloor(cands []*Cand, in RuleInput) []domain.Conflict {
	var out []domain.Conflict
	floor := in.Policies.Margin.Pct
	for _, c := range cands {
		if !c.Live() || (c.P.Kind != domain.KindPriceCounter && c.P.Kind != domain.KindSODraft) {
			continue
		}
		key := "margin_asked"
		if c.P.Kind == domain.KindSODraft {
			key = "margin_pct"
		}
		m, ok := num(c.P.Payload[key])
		if !ok || m >= floor {
			continue
		}
		c.NoAuto = append(c.NoAuto, RuleMarginFloor)
		c.Blocked = true
		title := fmt.Sprintf("Margin %s < floor %s", agents.Pct(m), floorText(floor))
		res := "Tidak diproses otomatis. Dieskalasi ke Anda."
		if d, ok := num(c.P.Payload["discount_pct"]); ok && d > 0 {
			title = fmt.Sprintf("Harga khusus %s di bawah tier %s → margin %s < floor %s", floorText(d), tierOf(c), agents.Pct(m), floorText(floor))
		}
		if cp, ok := num(c.P.Payload["counter_pct"]); ok {
			res = fmt.Sprintf("Tidak diproses otomatis. Dieskalasi ke Anda dengan counter %s yang masih di atas floor.", floorText(cp))
			if strings.Contains(c.P.Preview, "NVR 32") {
				res = strings.TrimSuffix(res, ".") + " + pemanis NVR 32ch yang menua."
			}
		}
		out = append(out, domain.Conflict{Rule: RuleMarginFloor, DealerID: slugOf(c), AgentA: "AI Order", AgentB: "Kebijakan margin", Title: title,
			Resolution: res, Tone: "bad", Visible: true, Keys: []string{c.P.DedupeKey}})
	}
	return out
}

func tierOf(c *Cand) string {
	if c.Dealer == nil || c.Dealer.Tier == "" {
		return "dealer"
	}
	return c.Dealer.Tier
}

func floorText(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d%%", int(f))
	}
	return agents.Pct(f)
}

// oneOwner: two sales numbers active on one dealer (≥ 10 interactions in 30 days each) → the price owner is the
// one with the most interactions; proposals from the second sales are prepared with a coordination note.
func oneOwner(cands []*Cand, in RuleInput) []domain.Conflict {
	var out []domain.Conflict
	done := map[uuid.UUID]bool{}
	for _, c := range cands {
		if !c.Live() || c.Dealer == nil || done[c.Dealer.UUID] {
			continue
		}
		xs := in.Interactions[c.Dealer.UUID]
		if len(xs) < 2 || xs[0].N < 10 || xs[1].N < 10 {
			continue
		}
		done[c.Dealer.UUID] = true
		owner, other := xs[0], xs[1]
		for _, o := range cands {
			if o.Live() && o.Dealer != nil && o.Dealer.UUID == c.Dealer.UUID && o.Dealer.Owner.Name != owner.Sales {
				o.P.Prep = strings.TrimSpace(o.P.Prep + " Koordinasi dengan " + owner.Sales + " (pemilik harga).")
				o.Blocked = true
			}
		}
		out = append(out, domain.Conflict{Rule: RuleOneOwner, DealerID: slugOf(c), AgentA: owner.Sales, AgentB: other.Sales, Title: "Dua sales menyentuh satu dealer",
			Resolution: fmt.Sprintf("Pemilik harga: %s (%d interaksi/bln). %s mendukung. Ditulis ke memori dealer.", owner.Sales, owner.N, other.Sales),
			Tone:       "indigo", Visible: true})
	}
	return out
}
