package orchestrator

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/agents"
	"distri-arc/internal/domain"
)

// PlanCand is a stored proposal of today as the plan sees it.
type PlanCand struct {
	ID       uuid.UUID
	Key      string
	Agent    string
	Kind     string
	Autonomy string
	Status   string
	Title    string
	Payload  map[string]any
	Dealer   *agents.Dealer
	Order    int // position on the board (Odoo order); groups list dealers in this order
}

// MaxPlanItems caps Rencana hari ini; the rest stays in the lists and the queue (04-orchestrator).
const MaxPlanItems = 10

// planLink renders a dealer link for the UI ("[[dealer:mitra|Mitra Jaya Teknik]]").
func planLink(d *agents.Dealer) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("[[dealer:%s|%s]]", d.ID, agents.Short(d.Name))
}

func links(cs []PlanCand) string {
	var xs []string
	for _, c := range cs {
		xs = append(xs, planLink(c.Dealer))
	}
	return agents.Join(xs)
}

func open(c PlanCand) bool {
	return c.Status != "rejected" && c.Status != "suppressed" && c.Status != "expired"
}

func planStatus(cs []PlanCand) string {
	done, skipped := true, true
	for _, c := range cs {
		done = done && (c.Status == "executed" || c.Status == "approved" || c.Status == "edited")
		skipped = skipped && (c.Status == "rejected" || c.Status == "expired" || c.Status == "suppressed")
	}
	switch {
	case done:
		return domain.PlanDone
	case skipped:
		return domain.PlanSkipped
	}
	return domain.PlanScheduled
}

func approvePriority(c PlanCand) int {
	switch {
	case c.Kind == domain.KindCreditRelease:
		return 0
	case strings.Contains(c.Agent, " + "):
		return 3 // follow-up with an installment scheme: after the release and the bundle (mockup order)
	case c.Kind == domain.KindInstallment || c.Kind == domain.KindCollect:
		return 1
	case c.Kind == domain.KindPushStock:
		return 2
	case c.Kind == domain.KindFollowup && strings.Contains(c.Agent, " + "):
		return 3
	case c.Kind == domain.KindFollowup:
		return 4
	case c.Kind == domain.KindNewDealer:
		return 5
	}
	return 9
}

func omzet(c PlanCand) int64 {
	if c.Dealer == nil {
		return 0
	}
	return c.Dealer.Metrics.OmzetBln
}

func stockValue(c PlanCand) float64 {
	v, _ := num(c.Payload["stock_value"])
	return v
}

// Build composes Rencana hari ini from today's proposals: auto steps first (grouped: H-1 follow-ups, H-3
// reminders, then follow-ups waiting for a payment), then the steps that wait for a human — a release that holds
// a shipment, reminders that need a firm tone, the most valuable aging-stock bundle, follow-ups of dealers past
// their cycle, and new dealers. Decisions that react to a request (price, return, limit) stay in Keputusan;
// follow-ups due in 2–7 days stay in Jadwal order. Times start at the next half hour (≥ 07.30), 30 minutes apart.
func Build(cands []PlanCand, now time.Time) []domain.PlanItem {
	var h1, h3, waiting, approve []PlanCand
	pushes := []PlanCand{}
	for _, c := range cands {
		if !open(c) && c.Status != "rejected" {
			continue
		}
		wait, _ := c.Payload["wait_for"].(string)
		switch {
		case c.Autonomy == "auto" && c.Kind == domain.KindFollowup && wait != "":
			waiting = append(waiting, c)
		case c.Autonomy == "auto" && c.Kind == domain.KindFollowup:
			h1 = append(h1, c)
		case c.Autonomy == "auto" && c.Kind == domain.KindCollect:
			h3 = append(h3, c)
		case c.Autonomy == "auto":
			// SO drafts and credit holds already ran when they were proposed: not a step of the day
		case c.Kind == domain.KindPushStock:
			pushes = append(pushes, c)
		case c.Kind == domain.KindCreditRelease, c.Kind == domain.KindInstallment, c.Kind == domain.KindCollect, c.Kind == domain.KindNewDealer:
			approve = append(approve, c)
		case c.Kind == domain.KindFollowup && c.Dealer != nil && (c.Dealer.Metrics.Status == domain.StatusAtRisk || strings.Contains(c.Agent, " + ")):
			approve = append(approve, c)
		}
	}
	sort.SliceStable(pushes, func(i, j int) bool { return stockValue(pushes[i]) > stockValue(pushes[j]) })
	if len(pushes) > 0 {
		approve = append(approve, pushes[0]) // one bundle campaign a day; the others stay in Push stok
	}
	sort.SliceStable(approve, func(i, j int) bool {
		pi, pj := approvePriority(approve[i]), approvePriority(approve[j])
		if pi != pj {
			return pi < pj
		}
		return omzet(approve[i]) > omzet(approve[j])
	})
	byBoard := func(xs []PlanCand) {
		sort.SliceStable(xs, func(i, j int) bool { return xs[i].Order < xs[j].Order })
	}
	byBoard(h1)
	byBoard(h3)

	var items []domain.PlanItem
	slot := firstSlot(now)
	next := func() string {
		t := slot.Format("15.04")
		slot = slot.Add(30 * time.Minute)
		return t
	}
	keys := func(cs []PlanCand) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Key)
		}
		return out
	}
	if len(h1) > 0 {
		when := "besok"
		if m := h1[0].Dealer.Metrics; m.DueIn != nil && *m.DueIn == 0 {
			when = "hari ini"
		}
		text := fmt.Sprintf("Kirim rekomendasi order ke %d dealer jadwal order %s — %s", len(h1), when, links(h1))
		if len(h1) == 1 {
			text = fmt.Sprintf("Kirim rekomendasi order ke %s — jadwal order %s", links(h1), when)
		}
		items = append(items, domain.PlanItem{Time: next(), Agent: "AI Follow-up", Autonomy: "auto", Text: text, Keys: keys(h1), Status: planStatus(h1)})
	}
	if len(h3) > 0 {
		var parts []string
		for _, c := range h3 {
			inv := firstInvoice(c)
			why := ""
			if c.Dealer != nil && c.Dealer.Metrics.Credit.State == domain.CreditTipis {
				why = " — supaya order berikutnya tidak tertahan limit"
			}
			parts = append(parts, fmt.Sprintf("%s %s%s", inv, planLink(c.Dealer), why))
		}
		text := "Pengingat H-3 " + strings.Join(parts, "; ")
		items = append(items, domain.PlanItem{Time: next(), Agent: "AI Penagihan", Autonomy: "auto", Text: text, Keys: keys(h3), Status: planStatus(h3)})
	}
	for _, c := range waiting {
		wait, _ := c.Payload["wait_for"].(string)
		inv := strings.TrimPrefix(wait, "payment:")
		text := fmt.Sprintf("Rekomendasi order %s setelah %s dibayar%s", planLink(c.Dealer), inv, dueText(c))
		st := planStatus([]PlanCand{c})
		if st == domain.PlanScheduled {
			st = domain.PlanWaiting
		}
		items = append(items, domain.PlanItem{Time: "nanti", Agent: "AI Follow-up", Autonomy: "auto", Text: text, Keys: []string{c.Key}, Status: st, WaitFor: wait})
	}
	if slot.Before(atClock(now, 8, 30)) {
		slot = atClock(now, 8, 30) // steps that need a human start once the working day has begun
	}
	for _, c := range approve {
		if len(items) >= MaxPlanItems {
			break
		}
		it := domain.PlanItem{Time: next(), Agent: c.Agent, Autonomy: "approve", Text: approveText(c), Keys: []string{c.Key}, Status: planStatus([]PlanCand{c})}
		if c.Kind == domain.KindNewDealer {
			it.Link = "chat"
		}
		items = append(items, it)
	}
	if len(items) > MaxPlanItems {
		items = items[:MaxPlanItems]
	}
	for i := range items {
		items[i].Seq = i + 1
	}
	return items
}

func atClock(now time.Time, h, m int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
}

// firstSlot is the next half hour, not before 07.30.
func firstSlot(now time.Time) time.Time {
	t := now.Truncate(30 * time.Minute).Add(30 * time.Minute)
	if min := atClock(now, 7, 30); t.Before(min) {
		return min
	}
	return t
}

func firstInvoice(c PlanCand) string {
	switch xs := c.Payload["invoices"].(type) {
	case []string:
		if len(xs) > 0 {
			return xs[0]
		}
	case []any:
		if len(xs) > 0 {
			s, _ := xs[0].(string)
			return s
		}
	}
	return "invoice"
}

func dueText(c PlanCand) string {
	if c.Dealer == nil {
		return ""
	}
	m := c.Dealer.Metrics
	var parts []string
	if m.DueIn != nil {
		parts = append(parts, fmt.Sprintf("jadwal order %d hari", *m.DueIn))
	}
	if m.Credit.Room != nil {
		parts = append(parts, fmt.Sprintf("sisa limit %d%%", int(*m.Credit.Room*100+0.5)))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func approveText(c PlanCand) string {
	d := c.Dealer
	switch c.Kind {
	case domain.KindCreditRelease:
		amount, _ := num(c.Payload["amount"])
		return fmt.Sprintf("Rilis kredit %s %s — usul DP 50%%", planLink(d), agents.Rp(int64(amount)))
	case domain.KindPushStock:
		return c.Title
	case domain.KindNewDealer:
		name, _ := c.Payload["name"].(string)
		city, _ := c.Payload["city"].(string)
		sales, _ := c.Payload["sales"].(string)
		return fmt.Sprintf("Nomor baru %s (%s) → usul dealer tier C, harga tier C dari %s", name, city, sales)
	case domain.KindInstallment, domain.KindCollect:
		if d != nil && strings.Contains(c.Agent, " + ") {
			late := 0
			if m := d.Metrics; m.Last != nil && m.Rhythm != nil {
				late = *m.Last - *m.Rhythm
			}
			return fmt.Sprintf("Skema cicilan 2× + order cash kecil untuk %s (lewat jadwal %d hr, %s)", planLink(d), late, d.Metrics.Credit.State)
		}
		return fmt.Sprintf("%s %s — %s", map[string]string{domain.KindInstallment: "Skema cicilan", domain.KindCollect: "Pengingat"}[c.Kind], planLink(d), firstInvoice(c))
	case domain.KindFollowup:
		if d == nil {
			return c.Title
		}
		m := d.Metrics
		late := 0
		if m.Last != nil && m.Rhythm != nil {
			late = *m.Last - *m.Rhythm
		}
		if strings.Contains(c.Agent, "Penagihan") {
			return fmt.Sprintf("Skema cicilan 2× + order cash kecil untuk %s (lewat jadwal %d hr, %s)", planLink(d), late, m.Credit.State)
		}
		if c.Payload["second"] == true && m.Last != nil {
			return fmt.Sprintf("Follow-up ke-2 %s — %d hari tanpa order, sebelum churn", planLink(d), *m.Last)
		}
		return fmt.Sprintf("Follow-up %s — lewat jadwal %d hr", planLink(d), late)
	}
	return c.Title
}
