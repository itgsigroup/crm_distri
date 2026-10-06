package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/metrics"
)

// Followup is AI Follow-up: keeps every dealer on its cycle — H-1 reminders with a recommended order, and a
// way back for dealers that drift past 1,2× their cycle. It never offers discounts below tier.
type Followup struct{}

func (Followup) Name() string { return "AI Follow-up" }

// Kinds: a follow-up to a dealer that asked for more time is an installment scheme (with AI Penagihan).
func (Followup) Kinds() []string { return []string{domain.KindFollowup, domain.KindInstallment} }

func provenance(d *Dealer, n int) []uuid.UUID {
	var out []uuid.UUID
	for _, s := range d.Signals {
		if len(out) == n {
			break
		}
		out = append(out, s.ID)
	}
	return out
}

// recommendation is the rekomendasi order with an aging sweetener and critical-stock warnings (glossary).
func recommendation(in *Input, d *Dealer) metrics.Recommendation {
	return metrics.OrderRecommendation(metrics.DealerView{ID: d.ID, Name: d.Name, Metrics: d.Metrics, Composition: d.Composition}, d.Branch, in.Stock, in.Policies)
}

func lowerList(xs []string) string {
	var out []string
	for _, x := range xs {
		out = append(out, shortProduct(x))
	}
	return Join(out)
}

// Analyze proposes follow-ups.
func (a Followup) Analyze(ctx context.Context, in *Input, r *llm.Router) ([]domain.Proposal, error) {
	var out []domain.Proposal
	pol := in.Policies
	day := in.Today.Format("2006-01-02")
	for _, d := range in.Dealers {
		m := d.Metrics
		if m.Rhythm == nil || m.DueIn == nil || m.Last == nil {
			continue
		}
		due, rhythm, last := *m.DueIn, *m.Rhythm, *m.Last
		bad := metrics.CreditTone(m.Credit.State) == "bad"
		drifting := m.Cyc > pol.Orbit.Drift
		if m.Status == domain.StatusChurn {
			continue // churn: prioritas rendah, no proactive follow-up
		}
		if !drifting && (due < 0 || due > 7) {
			continue
		}
		if !drifting && bad {
			continue
		}
		pic := Primary(d)
		rec := recommendation(in, d)
		names := []string{pic.Name, d.Owner.Name}
		dealerID := d.UUID
		p := domain.Proposal{Agent: a.Name(), DealerID: &dealerID, Kind: domain.KindFollowup, Icon: "chat", SignalIDs: provenance(d, 3),
			Autonomy: "approve", Payload: map[string]any{"to": pic.Name, "recommendation": rec}, DedupeKey: fmt.Sprintf("followup:%s:%s", d.ID, day)}
		basket := lowerList(rec.Products)
		switch {
		case drifting:
			p.Button, p.DueLabel, p.Confidence = "Kirim", "Minggu ini", 0.8
			late := last - rhythm
			p.Steps = []string{"Masuk antrean " + d.Owner.Name, "Kalau dibalas, AI Order menyiapkan SO", "Kalau sunyi 14 hari, usul kunjungan"}
			switch d.RootCause {
			case domain.RootProjectUnpaid:
				inv := lateInvoice(d)
				half := inv.Residual / 2
				p.Agent, p.Kind, p.Icon = "AI Follow-up + AI Penagihan", domain.KindInstallment, "cash"
				p.DedupeKey = fmt.Sprintf("installment:%s:%s", d.ID, day)
				p.DueLabel = "Hari ini"
				p.Title = fmt.Sprintf("Follow-up %s dengan skema cicilan 2× + order kecil cash", d.Name)
				p.Why = fmt.Sprintf("Lewat siklus order %d hari dan invoice lewat %d hari — dua masalah yang sama akarnya (proyek belum cair). Cicilan 2× + order cash kecil menjaga hubungan dan mengembalikan piutang.", late, inv.LateDays)
				p.Prep = fmt.Sprintf("Draft WA dari nomor %s: tawaran cicilan 2× (%s + %s jt) dan harga khusus %s untuk order cash.", d.Owner.Name, Jt(half), Jt(inv.Residual-half), firstOr(rec.Products, "produk favorit"))
				p.Preview = fmt.Sprintf("%s, untuk invoice %s kami bisa bagi dua: %s minggu ini dan %s 2 minggu lagi. Kalau butuh %s untuk proyek yang jalan, ada harga khusus untuk order cash minggu ini.", pic.Name, Rp(inv.Residual), Rp(half), Rp(inv.Residual-half), shortProduct(firstOr(rec.Products, "kamera")))
				p.Steps = []string{"Masuk antrean " + d.Owner.Name, "Komitmen cicilan dicatat dengan tanggal", "Limit sementara dibekukan sampai cicilan 1 masuk"}
				p.Payload["installments"] = []int64{half, inv.Residual - half}
				p.Payload["covers"] = []string{domain.KindCollect, domain.KindInstallment}
			case domain.RootMarketplaceModule, domain.RootMarketplace:
				p.Agent = "AI Follow-up + AI Stok"
				sweet := agingFor(in, d)
				if sweet != nil {
					price := int64(float64(sweet.UnitCost) / (1 - (pol.Margin.Pct+5)/100))
					p.Title = fmt.Sprintf("Follow-up %s dengan bundle %s harga marketplace", d.Name, shortName(sweet.Name))
					p.Why = fmt.Sprintf("%d hari tanpa order (siklus order %d). Produk favorit mereka = %s yang sedang menua %d hari di %s. Bundle di atas floor margin %s.", last, rhythm, shortName(sweet.Name), sweet.AgeDays, sweet.Branch, pctWhole(pol.Margin.Pct))
					p.Prep = fmt.Sprintf("Draft WA dari %s dengan harga bundle dan stok siap kirim.", d.Owner.Name)
					p.Preview = fmt.Sprintf("%s, lama tidak order. %s sedang ada harga khusus bundling — %s/pcs, kirim besok dari %s. Mau saya kirim daftar lengkapnya?", pic.Name, shortName(sweet.Name), Rb(price), sweet.Branch)
					p.Payload["bundle_price"] = price
					p.Payload["bundle_sku"] = sweet.Name
				} else {
					p.Title = fmt.Sprintf("Follow-up %s — lewat jadwal %d hari", d.Name, last)
					p.Why = fmt.Sprintf("%d hari tanpa order (siklus order %d); dugaan harga vs marketplace.", last, rhythm)
					p.Preview = fmt.Sprintf("%s, lama tidak order. %s siap kirim besok dengan harga tier. Mau saya siapkan?", pic.Name, capitalize(basket))
				}
			default:
				p.Title = fmt.Sprintf("Follow-up %s — lewat jadwal %d hari", d.Name, last)
				if second(d, in) {
					p.Title = fmt.Sprintf("Follow-up ke-2 %s — %d hari tanpa order, sebelum churn", d.Name, last)
					p.Payload["second"] = true
				}
				p.Why = fmt.Sprintf("%s× siklus order tanpa order%s. Share of wallet %d%%%s.", strings.Replace(fmt.Sprintf("%.1f", m.Cyc), ".", ",", 1), unansweredText(d), m.SOW, rootText(d.RootCause))
				p.Prep = fmt.Sprintf("Draft WA dari %s dengan rekomendasi order (%s).", d.Owner.Name, basket)
				p.Preview = fmt.Sprintf("%s, lama tidak order. %s siap kirim besok dari %s. Mau saya kirim daftarnya?", pic.Name, capitalize(basket), d.Branch)
				p.Steps = []string{"Masuk antrean " + d.Owner.Name, "Kalau sunyi 14 hari → Churn, tidak di-follow-up lagi 60 hari"}
			}
		default:
			p.Button, p.Confidence = "Kirim rekomendasi", 0.9
			when := map[int]string{0: "hari ini", 1: "besok"}[due]
			if when == "" {
				when = fmt.Sprintf("%d hari", due)
				p.Button, p.Confidence = "Kirim", 0.85
			}
			p.DueLabel = capitalize(when)
			p.Title = fmt.Sprintf("Follow-up %s — jadwal order %s", d.Name, when)
			p.Why = fmt.Sprintf("Siklus order %d hari, order terakhir %d hari lalu. Product mix biasa: %s. Stok %s %s.", rhythm, last, basket, d.Branch, stockState(rec))
			p.Prep = fmt.Sprintf("Rekomendasi order + harga tier %s dari %s%s.", d.Tier, d.Owner.Name, criticalText(rec, d.Branch))
			ready := lowerList(rec.Products[:min(2, len(rec.Products))])
			p.Preview = fmt.Sprintf("%s, biasanya minggu ini %s order ya. Stok %s siap%s. Mau saya siapkan seperti biasa?", pic.Name, Bapak(pic.Name), ready, criticalPreview(rec))
			p.Steps = []string{"Masuk antrean " + d.Owner.Name, "Balasan → AI Order membuat SO"}
		}
		if m.PICActive <= 1 { // Hanya 1 PIC: one person leaves, the dealer leaves with them (glossary)
			p.Prep = strings.TrimSpace(p.Prep + " Hanya 1 PIC aktif — draft meminta nomor admin/kasir.")
			p.Preview = strings.TrimSpace(p.Preview + " Boleh minta juga nomor admin atau kasir toko untuk konfirmasi pengiriman?")
			p.Steps = append(p.Steps, "Simpan nomor admin/kasir sebagai PIC kedua")
			p.Payload["ask_second_pic"] = true
		}
		polish(ctx, r, in, &p, "followup", map[string]any{"dealer": d.Name, "pic": pic.Name, "sales": d.Owner.Name, "rhythm_days": rhythm, "days_since_order": last,
			"due_in": due, "recommendation": rec, "root_cause": d.RootCause, "credit_state": m.Credit.State}, names)
		out = append(out, p)
	}
	return out, nil
}

func firstOr(xs []string, def string) string {
	if len(xs) == 0 {
		return def
	}
	return xs[0]
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func shortName(n string) string { return strings.TrimSuffix(n, " outdoor") }

func stockState(rec metrics.Recommendation) string {
	if len(rec.Critical) > 0 {
		return "cukup kecuali " + strings.Join(rec.Critical, ", ")
	}
	return "cukup"
}

func criticalText(rec metrics.Recommendation, branch string) string {
	if len(rec.Critical) == 0 {
		return ""
	}
	return fmt.Sprintf("; %s sedang kritis di %s", strings.Join(rec.Critical, ", "), branch)
}

func criticalPreview(rec metrics.Recommendation) string {
	if len(rec.Critical) == 0 {
		return ""
	}
	return fmt.Sprintf("; %s sedang terbatas", strings.Join(rec.Critical, ", "))
}

func unansweredText(d *Dealer) string {
	if strings.Contains(strings.ToLower(d.Memo), "tanpa balasan") {
		return " dan WA terakhir tanpa balasan"
	}
	return ""
}

func rootText(root string) string {
	switch root {
	case domain.RootWholesaler:
		return " — dugaan beralih ke grosir lokal. Sapaan terakhir sebelum diturunkan ke prioritas rendah"
	case domain.RootSmallShare:
		return " — porsi kecil sejak awal"
	}
	return ""
}

// lateInvoice returns the oldest overdue invoice.
func lateInvoice(d *Dealer) (out struct {
	Number   string
	Residual int64
	LateDays int
}) {
	for _, i := range d.OpenInvoices {
		if i.LateDays > out.LateDays {
			out.Number, out.Residual, out.LateDays = i.Number, i.Residual, i.LateDays
		}
	}
	return
}

// second reports whether this would be the second follow-up: an earlier one went out within 60 days, or the
// last WhatsApp stayed unanswered (memo). Second follow-ups always wait for a human (autonomy matrix).
func second(d *Dealer, in *Input) bool {
	if d.LastFollowupAt != nil && in.Today.Sub(*d.LastFollowupAt) <= 60*24*time.Hour {
		return true
	}
	return strings.Contains(strings.ToLower(d.Memo), "tanpa balasan")
}

// agingFor finds aging stock in the dealer's branch for a category it buys.
func agingFor(in *Input, d *Dealer) *domain.StockItem {
	for i := range in.Stock {
		s := &in.Stock[i]
		ci := domain.CategoryIndex(s.Category)
		if s.Branch == d.Branch && metrics.IsAging(*s, in.Policies) && ci >= 0 && d.Metrics.MixCats[ci] {
			return s
		}
	}
	return nil
}
