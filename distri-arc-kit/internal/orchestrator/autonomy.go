package orchestrator

import (
	"slices"
	"strings"

	"distri-arc/internal/domain"
)

// Verdict is the Keputusan stage's answer for one proposal.
type Verdict struct {
	Auto   bool
	Reason string // why not auto (empty when auto)
}

// primaryAgent maps a combined agent ("AI Follow-up + AI Penagihan") to the agent whose matrix row applies.
func primaryAgent(name string) string {
	first, _, _ := strings.Cut(name, " + ")
	return first
}

// creditSensitive kinds invite new exposure, so they need a safe sisa limit to run on their own. An SO draft is
// checked against the order itself instead (payload complete = stock ∧ exposure + order ≤ limit).
func creditSensitive(kind string) bool {
	return kind == domain.KindFollowup || kind == domain.KindPushStock
}

// Evaluate applies policy.autonomy.matrix and the guard (04-orchestrator › Keputusan): auto only when the matrix
// allows the kind, confidence ≥ min, the dealer is not At risk/Churn, the limit is safe for kinds that add
// exposure, no contention rule touched it, and the kind-specific condition holds.
func Evaluate(c *Cand, p domain.PolicySet) Verdict {
	row, ok := p.Autonomy[primaryAgent(c.P.Agent)]
	if !ok || !slices.Contains(row.Auto, c.P.Kind) {
		return Verdict{Reason: "matriks: butuh approve"}
	}
	if strings.Contains(c.P.Agent, " + ") {
		return Verdict{Reason: "gabungan dua agen: butuh approve"}
	}
	minConf := p.Guard.MinConfidence
	if minConf == 0 {
		minConf = 0.8
	}
	if c.P.Confidence < minConf {
		return Verdict{Reason: "confidence di bawah batas"}
	}
	if len(c.NoAuto) > 0 || c.Blocked {
		return Verdict{Reason: "konflik: " + strings.Join(c.NoAuto, ", ")}
	}
	if d := c.Dealer; d != nil {
		if s := d.Metrics.Status; s == domain.StatusAtRisk || s == domain.StatusChurn {
			return Verdict{Reason: "dealer " + s}
		}
		if creditSensitive(c.P.Kind) && !c.AutoAfter {
			if st := d.Metrics.Credit.State; st != domain.CreditAman && st != domain.CreditCash {
				return Verdict{Reason: "sisa limit " + st}
			}
		}
	}
	switch c.P.Kind {
	case domain.KindFollowup:
		if c.AutoAfter {
			return Verdict{Auto: true}
		}
		if c.P.Payload["second"] == true {
			return Verdict{Reason: "follow-up ke-2"}
		}
		m := c.Dealer.Metrics
		if m.DueIn == nil || *m.DueIn < 0 || *m.DueIn > max(p.Followup.HMinus, 1) || m.Cyc > p.Orbit.Drift {
			return Verdict{Reason: "bukan H-1"}
		}
	case domain.KindCollect:
		if c.P.Payload["tone"] != "ramah" {
			return Verdict{Reason: "nada tegas"}
		}
	case domain.KindSODraft:
		if c.P.Payload["complete"] != true {
			return Verdict{Reason: "SO belum lengkap (stok/limit)"}
		}
	case domain.KindCreditHold:
	default:
		return Verdict{Reason: "jenis tidak otonom"}
	}
	return Verdict{Auto: true}
}

// MessagesDealer reports whether carrying out the proposal sends something to the dealer (ADR 0008: such auto
// steps wait for "Jalankan sekarang" unless autonomy.guard.dealer_messages is "auto").
func MessagesDealer(kind string) bool { return domain.SendsWA(kind) }
