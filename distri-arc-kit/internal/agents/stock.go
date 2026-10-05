package agents

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/metrics"
)

// Stock is AI Stok: matches aging stock with dealers whose product mix fits and whose order is due (glossary
// "Push stok"). The bundle price never goes below the margin floor; sending the offers needs a human.
type Stock struct{}

func (Stock) Name() string { return "AI Stok" }
func (Stock) Kinds() []string {
	return []string{domain.KindPushStock, domain.KindTransfer, domain.KindPORequest}
}

// BundleDiscount is the largest discount (0,5% steps, ≤ policy max) that keeps the margin at or above the floor.
func BundleDiscount(price, cost int64, p domain.PolicySet) float64 {
	if price <= 0 {
		return 0
	}
	floorPrice := float64(cost) / (1 - p.Margin.Pct/100)
	maxPct := (1 - floorPrice/float64(price)) * 100
	d := math.Floor(math.Min(maxPct, p.Stock.BundleMaxDiscountPct)*2) / 2
	return math.Max(0, d)
}

// Analyze proposes one bundle per aging SKU that has candidates.
func (a Stock) Analyze(ctx context.Context, in *Input, r *llm.Router) ([]domain.Proposal, error) {
	var out []domain.Proposal
	pol := in.Policies
	views := make([]metrics.DealerView, 0, len(in.Dealers))
	bySlug := map[string]*Dealer{}
	for _, d := range in.Dealers {
		views = append(views, metrics.DealerView{ID: d.ID, Name: d.Name, Metrics: d.Metrics, Composition: d.Composition})
		bySlug[d.ID] = d
	}
	day := in.Today.Format("2006-01-02")
	for _, s := range in.Stock {
		if !metrics.IsAging(s, pol) {
			continue
		}
		cands := metrics.PushCandidates(s, views, pol)
		if len(cands) == 0 {
			continue
		}
		prod, ok := in.Catalog[s.Name]
		if !ok {
			continue
		}
		price := prod.Price("A")
		disc := BundleDiscount(price, s.UnitCost, pol)
		bundle := int64(math.Round(float64(price)*(1-disc/100)/1000)) * 1000
		margin := float64(bundle-s.UnitCost) / float64(bundle) * 100
		var ids []uuid.UUID
		var names, list []string
		var dealers []map[string]any
		due := 0
		sigs := []uuid.UUID{}
		if id, ok := in.StockSignals[s.Name+"|"+s.Branch]; ok {
			sigs = append(sigs, id)
		}
		for _, c := range cands {
			d := bySlug[c.DealerID]
			if d == nil || d.Metrics.Status == domain.StatusChurn {
				continue // churn: low priority, no proactive offers (same as AI Follow-up)
			}
			if c.DueIn != nil && *c.DueIn >= 0 && *c.DueIn <= 7 {
				due++
			}
			pic := Primary(d)
			ids = append(ids, d.UUID)
			names = append(names, Short(d.Name))
			list = append(list, d.Name)
			preview := fmt.Sprintf("%s, %s sedang ada harga bundle %s/pcs (stok %s, kirim dari %s). Cocok untuk %s yang biasa %s ambil — mau saya siapkan?",
				pic.Name, shortName(s.Name), Unit(bundle), s.Branch, s.Branch, s.Category, Bapak(pic.Name))
			dealers = append(dealers, map[string]any{"id": d.UUID, "slug": d.ID, "name": d.Name, "owner": d.Owner.Name, "to": pic.Name, "reason": c.Reason, "preview": preview})
			if len(sigs) < 4 {
				sigs = append(sigs, provenance(d, 1)...)
			}
		}
		if len(ids) == 0 || len(sigs) == 0 {
			continue
		}
		discText := "harga tier"
		if disc > 0 {
			discText = "−" + pctWhole(disc)
		}
		cat := s.Category
		dueText := ""
		if due > 0 {
			dueText = fmt.Sprintf(" (%d jadwal order minggu ini)", due)
		}
		p := domain.Proposal{
			Agent: a.Name(), DealerIDs: ids, Kind: domain.KindPushStock, Icon: "box", Button: "Buat bundle", DueLabel: "Minggu ini",
			Title:   fmt.Sprintf("Bundle %s %s ke %d dealer product mix %s%s", shortName(s.Name), discText, len(ids), cat, dueText),
			Summary: fmt.Sprintf("%d pcs · %d hari · %s di %s → %s", s.Qty, s.AgeDays, Rp(s.Value), s.Branch, Join(names)),
			Why: fmt.Sprintf("%s menua %d hari di %s (%s). %d dealer cocok product mix-nya%s dan limitnya aman. Harga bundle %s, margin %s — di atas floor %s.",
				s.Name, s.AgeDays, s.Branch, Rp(s.Value), len(ids), dueText, Unit(bundle), Pct(margin), pctWhole(pol.Margin.Pct)),
			Prep:  fmt.Sprintf("Draft WA per dealer dari nomor sales pemiliknya (%s), harga bundle %s berlaku 7 hari.", strings.Join(uniq(ownersOf(dealers)), ", "), Unit(bundle)),
			Steps: []string{"Draft per dealer masuk antrean sales pemiliknya", "Harga bundle berlaku 7 hari", "Balasan → AI Order menyiapkan SO dari stok " + s.Branch},
			Pills: [][2]string{{"warn", a.Name()}, {"neutral", fmt.Sprintf("%d hari · %s", s.AgeDays, s.Branch)}},
			Impact: []domain.Impact{{Label: "Nilai stok", Value: Rp(s.Value)}, {Label: "Harga bundle", Value: Unit(bundle)},
				{Label: "Margin", Value: Pct(margin), Tone: tone(margin < pol.Margin.Pct+1, "warn")}, {Label: "Dealer", Value: fmt.Sprintf("%d", len(ids))}},
			Confidence: 0.8, SignalIDs: sigs, Autonomy: "approve",
			Payload: map[string]any{"sku": s.SKU, "name": s.Name, "branch": s.Branch, "discount_pct": disc, "bundle_price": bundle, "margin_pct": math.Round(margin*10) / 10,
				"dealers": dealers, "stock_value": s.Value},
			DedupeKey: fmt.Sprintf("push:%s:%s:%s", s.Name, s.Branch, day),
		}
		if len(dealers) > 0 {
			p.Preview = dealers[0]["preview"].(string)
		}
		polish(ctx, r, in, &p, "stock", map[string]any{"sku": s.Name, "age_days": s.AgeDays, "branch": s.Branch, "bundle_price": Unit(bundle), "discount_pct": disc,
			"dealers": list}, nil)
		out = append(out, p)
	}
	return out, nil
}

func ownersOf(ds []map[string]any) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d["owner"].(string))
	}
	return out
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
