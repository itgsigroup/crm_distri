package agents

import (
	"context"
	"fmt"
	"math"
	"sort"
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
	// largest aging value first; at most MaxPushPerCycle bundles per cycle — a team reviews a short list, not hundreds
	aging := make([]domain.StockItem, 0, len(in.Stock))
	for _, s := range in.Stock {
		if metrics.IsAging(s, pol) {
			aging = append(aging, s)
		}
	}
	sort.SliceStable(aging, func(i, j int) bool { return aging[i].Value > aging[j].Value })
	pushes := 0
	for _, s := range aging {
		if pushes >= MaxPushPerCycle {
			break
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
		if price <= 0 {
			continue // no selling price known: no bundle price can be offered
		}
		disc := BundleDiscount(price, s.UnitCost, pol)
		bundle := int64(math.Round(float64(price)*(1-disc/100)/1000)) * 1000
		if bundle <= 0 {
			continue
		}
		margin := float64(bundle-s.UnitCost) / float64(bundle) * 100
		var ids []uuid.UUID
		var names, list []string
		var dealers []map[string]any
		due := 0
		sigs := []uuid.UUID{}
		if id, ok := in.StockSignals[s.Name+"|"+s.Branch]; ok {
			sigs = append(sigs, id)
		}
		matched := 0
		for _, c := range cands {
			d := bySlug[c.DealerID]
			if d == nil || d.Metrics.Status == domain.StatusChurn {
				continue // churn: low priority, no proactive offers (same as AI Follow-up)
			}
			matched++
			if len(ids) >= MaxPushDealers {
				continue // candidates are sorted (jadwal order, then omzet): the first ones get the offer
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
			Title:   fmt.Sprintf("Bundle %s %s ke %d dealer product mix %s%s%s", shortName(s.Name), discText, len(ids), cat, dueText, moreText(matched, len(ids))),
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
		pushes++
	}
	return append(out, a.critical(in, day)...), nil
}

// MaxPushPerCycle caps the bundle proposals of one cycle (largest aging value first).
const MaxPushPerCycle = 20

// MaxPushDealers caps the dealers of one bundle: a sales team follows up a few dozen dealers, not hundreds.
const MaxPushDealers = 30

func moreText(matched, offered int) string {
	if matched <= offered {
		return ""
	}
	return fmt.Sprintf(" · %d teratas dari %d yang cocok", offered, matched)
}

// Stock rules for critical SKUs (ADR 0011): refill a branch to TransferCoverWeeks of sales from another branch that
// keeps at least SourceKeepWeeks; without such a branch ask purchasing for POCoverWeeks of sales.
const (
	TransferCoverWeeks = 2.5
	SourceKeepWeeks    = 4.0
	POCoverWeeks       = 5.0
)

// round10 rounds to the nearest 10 units (a carton line in the warehouse).
func round10(v float64) int { return int(math.Round(v/10)) * 10 }

// critical proposes a transfer or a purchase order for each SKU that runs out before the next cycle.
func (a Stock) critical(in *Input, day string) []domain.Proposal {
	var out []domain.Proposal
	for _, s := range in.Stock {
		if !metrics.IsCritical(s, in.Policies) {
			continue
		}
		dependents := 0
		if ci := domain.CategoryIndex(s.Category); ci >= 0 {
			for _, d := range in.Dealers {
				if d.Branch == s.Branch && d.Metrics.MixCats[ci] && d.Metrics.Status != domain.StatusChurn {
					dependents++
				}
			}
		}
		days := *s.DaysLeft()
		sigs := []uuid.UUID{}
		if id, ok := in.StockSignals[s.Name+"|"+s.Branch]; ok {
			sigs = append(sigs, id)
		}
		var src *domain.StockItem
		need := round10(TransferCoverWeeks*s.WeeklyVelocity - float64(s.Qty))
		if need < 10 {
			need = 10
		}
		for i := range in.Stock {
			o := &in.Stock[i]
			if o.SKU == s.SKU && o.Branch != s.Branch && float64(o.Qty-need) >= SourceKeepWeeks*o.WeeklyVelocity && (src == nil || o.Qty > src.Qty) {
				src = o
			}
		}
		if src != nil {
			if id, ok := in.StockSignals[src.Name+"|"+src.Branch]; ok {
				sigs = append(sigs, id)
			}
		}
		if len(sigs) == 0 {
			continue
		}
		short := shortName(s.Name)
		depText := ""
		if dependents > 0 {
			depText = fmt.Sprintf(", %d dealer bergantung", dependents)
		}
		p := domain.Proposal{Agent: a.Name(), Icon: "refresh", DueLabel: "Hari ini", Confidence: 0.86, SignalIDs: sigs, Autonomy: "approve",
			Pills: [][2]string{{"warn", a.Name()}, {"neutral", fmt.Sprintf("habis ±%.0f hari", math.Ceil(days))}}}
		if src != nil {
			after := float64(s.Qty+need) / s.WeeklyVelocity
			p.Kind, p.Button = domain.KindTransfer, "Usulkan transfer"
			p.Title = fmt.Sprintf("Transfer %d %s %s → %s", need, short, src.Branch, s.Branch)
			p.Summary = fmt.Sprintf("%s sisa %d, %s/minggu%s · %s stok %d", s.Branch, s.Qty, trimFloat(s.WeeklyVelocity), depText, src.Branch, src.Qty)
			p.Why = fmt.Sprintf("%s sisa %d unit, terjual %s/minggu — habis ±%.0f hari%s. %s stok %d. Transfer internal tanpa SO/PO sesuai aturan multi-company.",
				s.Branch, s.Qty, trimFloat(s.WeeklyVelocity), math.Ceil(days), depText, src.Branch, src.Qty)
			p.Prep = "Transfer internal di Odoo (internal transfer), ekspedisi 2 hari."
			p.Steps = []string{"Transfer dibuat di Odoo", fmt.Sprintf("Gudang %s & %s diberi tahu di grup", src.Branch, s.Branch), fmt.Sprintf("Stok %s aman ±%s minggu", s.Branch, trimFloat(math.Round(after*10)/10))}
			p.Impact = []domain.Impact{{Label: "Sisa " + s.Branch, Value: fmt.Sprintf("%d", s.Qty), Tone: "bad"}, {Label: "Transfer", Value: fmt.Sprintf("%d", need)}, {Label: "Sisa " + src.Branch, Value: fmt.Sprintf("%d", src.Qty-need)}}
			p.Payload = map[string]any{"sku": s.SKU, "name": s.Name, "from": src.Branch, "to": s.Branch, "qty": need}
			p.DedupeKey = fmt.Sprintf("transfer:%s:%s:%s", s.SKU, s.Branch, day)
		} else {
			qty := round10(POCoverWeeks*s.WeeklyVelocity - float64(s.Qty))
			p.Kind, p.Button, p.Icon = domain.KindPORequest, "Ajukan PO", "doc"
			p.Title = fmt.Sprintf("PO %d %s untuk %s", qty, short, s.Branch)
			p.Summary = fmt.Sprintf("%s sisa %d, %s/minggu%s · tidak ada cabang lain yang bisa transfer", s.Branch, s.Qty, trimFloat(s.WeeklyVelocity), depText)
			p.Why = fmt.Sprintf("%s sisa %d unit, terjual %s/minggu — habis ±%.0f hari%s. Tidak ada cabang yang bisa transfer tanpa ikut kritis.", s.Branch, s.Qty, trimFloat(s.WeeklyVelocity), math.Ceil(days), depText)
			p.Prep = fmt.Sprintf("Permintaan PO %d unit ke Purchasing (± %s minggu penjualan).", qty, trimFloat(POCoverWeeks))
			p.Steps = []string{"Permintaan PO masuk ke Purchasing", "Purchasing konfirmasi lead time supplier", "AI Stok memantau stok " + s.Branch}
			p.Impact = []domain.Impact{{Label: "Sisa", Value: fmt.Sprintf("%d", s.Qty), Tone: "bad"}, {Label: "PO", Value: fmt.Sprintf("%d", qty)}}
			p.Payload = map[string]any{"sku": s.SKU, "name": s.Name, "branch": s.Branch, "qty": qty}
			p.DedupeKey = fmt.Sprintf("po:%s:%s:%s", s.SKU, s.Branch, day)
		}
		out = append(out, p)
	}
	return out
}

func trimFloat(f float64) string {
	return strings.Replace(strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), "."), ".", ",", 1)
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
