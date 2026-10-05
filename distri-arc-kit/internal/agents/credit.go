package agents

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
)

// Credit is AI Kredit: guards the sisa limit. Releases above the limit always go to the CEO with options
// (DP 50%, hold until paid); SOP-SEC-001 must be verified first; limit increases are proposed when earned.
// It never releases goods nor changes the limit in Odoo itself.
type Credit struct{}

func (Credit) Name() string { return "AI Kredit" }
func (Credit) Kinds() []string {
	return []string{domain.KindCreditRelease, domain.KindCreditLimit, domain.KindCreditHold}
}

// sopSec001 reports whether a manual signal recorded the SOP-SEC-001 checks (PO verified by phone, address).
func sopSec001(d *Dealer) (bool, *uuid.UUID) {
	for _, s := range d.Signals {
		if strings.Contains(s.Conclusion, "SOP-SEC-001 ✓") || strings.Contains(s.Text, "SOP-SEC-001 ✓") {
			id := s.ID
			return true, &id
		}
	}
	return false, nil
}

// Analyze proposes releases and limit changes.
func (a Credit) Analyze(ctx context.Context, in *Input, r *llm.Router) ([]domain.Proposal, error) {
	var out []domain.Proposal
	pol := in.Policies
	// 1. release requests above the limit
	for _, m := range Latest(in) {
		d := in.Dealer(m.DealerID)
		if d == nil || d.CreditLimit == 0 {
			continue
		}
		e := Extract(m.Text, in.Products)
		if e.Intent != IntentRelease || len(e.Items) == 0 {
			continue
		}
		var amount int64
		for _, it := range e.Items {
			amount += it.Product.Price(d.Tier) * it.Qty
		}
		cr := d.Metrics.Credit
		after := cr.Exposure + amount
		if after <= cr.Limit && !cr.Late {
			continue // inside the limit: AI Order drafts the SO
		}
		ok, sopSignal := sopSec001(d)
		dp := int64(math.Round(float64(amount)/2/1e6)) * 1_000_000
		inv := lateInvoice(d)
		it := e.Items[0]
		dealerID := d.UUID
		sigs := []uuid.UUID{m.SignalID}
		if sopSignal != nil {
			sigs = append(sigs, *sopSignal)
		}
		day := e.Day
		if day == "" {
			day = "segera"
		}
		sop := "SOP-SEC-001 belum lengkap: verifikasi PO via telepon ke nomor terdaftar dan alamat kirim dulu — opsi rilis belum bisa dipilih."
		if ok {
			sop = "SOP-SEC-001 lolos: PO diverifikasi via telepon ke nomor kantor, alamat kirim konsisten. Risikonya konsentrasi piutang, bukan penipuan."
		}
		lateText := ""
		if inv.LateDays > 0 {
			lateText = fmt.Sprintf(" dan %s %s lewat %d hari", inv.Number, Rp(inv.Residual), inv.LateDays)
		}
		p := domain.Proposal{
			Agent: a.Name(), DealerID: &dealerID, Kind: domain.KindCreditRelease, Icon: "shield", Button: "Setujui DP 50%", DueLabel: "Hari ini",
			Title:   fmt.Sprintf("Rilis %s untuk %s dengan DP 50%%", Rp(amount), d.Name),
			Summary: fmt.Sprintf("%d unit %s (%s) termin %d hari. Setelah rilis, exposure melewati limit.", it.Qty, shortProduct(it.Product.Name), Rp(amount), 30),
			Why: fmt.Sprintf("Exposure %s + %s = %s, limit %s. Pola bayar %d hari (termin 30)%s. %s",
				Rp(cr.Exposure), Rp(amount), Rp(after), Rp(cr.Limit), cr.PayDays, lateText, sop),
			Prep: fmt.Sprintf("Dua opsi disiapkan: (A) DP 50%% %s sebelum kirim, sisa termin 30 hari; (B) tahan sampai %s lunas. Draft balasan ke %s untuk opsi A, SO Odoo dengan syarat DP, gudang %s diberi tahu menunggu bukti transfer.",
				Rp(dp), firstNonEmpty(inv.Number, "invoice lewat tempo"), m.Contact, d.Branch),
			Preview: fmt.Sprintf("%s, untuk %d unit %s (%s) kami bisa kirim %s dengan DP 50%% (%s) karena exposure saat ini di atas limit kredit. Sisanya termin 30 hari seperti biasa. Bukti transfer bisa dikirim ke sini.",
				m.Contact, it.Qty, shortProduct(it.Product.Name), Rp(amount), day, Rp(dp)),
			Steps: []string{"Balasan masuk antrean " + d.Owner.Name, "SO dibuat di Odoo dengan syarat DP", "Gudang menunggu bukti transfer sebelum surat jalan", "AI Penagihan memantau " + firstNonEmpty(inv.Number, "piutang")},
			Pills: [][2]string{{"bad", a.Name()}, {"neutral", fmt.Sprintf("Diajukan %s · %s", d.Owner.Name, d.Branch)}},
			Impact: []domain.Impact{{Label: "Piutang berjalan", Value: Rp(cr.Exposure)}, {Label: "Limit kredit", Value: Rp(cr.Limit)},
				{Label: "Setelah rilis", Value: Rp(after), Tone: "bad"}, {Label: "Rata-rata bayar", Value: fmt.Sprintf("%d hari", cr.PayDays), Tone: tone(cr.PayDays > pol.Credit.PayMaxDays, "warn")}},
			Options: []domain.Option{
				{Key: "approve", Label: "Setujui DP 50%", Style: "primary", Result: fmt.Sprintf("Disetujui dengan DP 50%% · SO dibuat dengan syarat DP, %s & gudang %s diberi tahu", d.Owner.Name, d.Branch), Sends: true},
				{Key: "hold", Label: "Tahan sampai lunas", Style: "ghost", Result: fmt.Sprintf("Ditahan · %s diminta menagih %s dulu; AI Penagihan memantau", d.Owner.Name, firstNonEmpty(inv.Number, "invoice"))},
				{Key: "reject", Label: "Tolak", Style: "quiet", Result: "Ditolak · alasan dicatat untuk kalibrasi"},
			},
			Confidence: 0.88, SignalIDs: sigs, Autonomy: "approve",
			Payload:   map[string]any{"amount": amount, "dp": dp, "exposure_after": after, "sop_sec_001": ok, "to": m.Contact},
			DedupeKey: "credit:release:" + m.SignalID.String(),
		}
		if !ok {
			p.Options = p.Options[1:] // no release option until SOP-SEC-001 is complete
			p.Button = "Tahan sampai lunas"
		}
		polish(ctx, r, in, &p, "credit", map[string]any{"dealer": d.Name, "pic": m.Contact, "message": m.Text, "amount": Rp(amount), "exposure": Rp(cr.Exposure), "limit": Rp(cr.Limit),
			"after": Rp(after), "pay_days": cr.PayDays, "late": inv, "sop_sec_001": ok, "dp": Rp(dp), "delivery_day": day}, []string{m.Contact})
		out = append(out, p)
	}

	// 2. limit increases: ≥ policy on-time, room tight because orders grow
	for _, d := range in.Dealers {
		m := d.Metrics
		cr := m.Credit
		if d.CreditLimit == 0 || cr.Room == nil || cr.Late || cr.OnTime < pol.Credit.LimitUp.OnTimeMin || *cr.Room >= pol.Credit.RoomMin {
			continue
		}
		growth := monthlyGrowth(in, d)
		if growth <= 0 {
			continue
		}
		newLimit := int64(math.Ceil(float64(d.CreditLimit)*1.4/50e6)) * 50_000_000
		mid := (d.CreditLimit + newLimit) / 2 / 50_000_000 * 50_000_000
		if mid <= d.CreditLimit {
			mid = d.CreditLimit + 50_000_000
		}
		ordersLeft := 0
		if m.AvgOrder > 0 {
			ordersLeft = int(math.Max(1, math.Ceil(float64(d.CreditLimit-cr.Exposure)/float64(m.AvgOrder))))
		}
		expansion := ""
		if strings.Contains(strings.ToLower(d.Memo), "cabang") {
			expansion = " dan membuka cabang baru"
		}
		dealerID := d.UUID
		p := domain.Proposal{
			Agent: a.Name(), DealerID: &dealerID, Kind: domain.KindCreditLimit, Icon: "trend", Button: "Setujui", DueLabel: "Minggu ini",
			Title:   fmt.Sprintf("Naikkan limit kredit %s %s → %s", d.Name, Rp(d.CreditLimit), Rp(newLimit)),
			Summary: fmt.Sprintf("Order naik %d%% dalam 6 bulan%s; limit sekarang tersentuh dalam %d order.", growth, expansion, ordersLeft),
			Why: fmt.Sprintf("12 bulan %d%% tepat waktu, pola %d hari, order naik %d%% dalam 6 bulan%s. Limit sekarang akan tersentuh dalam %d order.",
				cr.OnTime, cr.PayDays, growth, expansion, ordersLeft),
			Prep:  fmt.Sprintf("Perubahan limit di Odoo + pesan ke %s dari %s.", Primary(d).Name, d.Owner.Name),
			Steps: []string{"Limit diperbarui di Odoo", "Pesan masuk antrean " + d.Owner.Name, "AI Kredit memantau exposure 90 hari"},
			Pills: [][2]string{{"good", a.Name()}, {"neutral", fmt.Sprintf("12 bln · %d%% tepat waktu", cr.OnTime)}},
			Impact: []domain.Impact{{Label: "Limit sekarang", Value: Rp(d.CreditLimit)}, {Label: "Usulan", Value: Rp(newLimit), Tone: "good"},
				{Label: "Pola bayar", Value: fmt.Sprintf("%d hari", cr.PayDays)}, {Label: "Exposure", Value: Rp(cr.Exposure)}},
			Options: []domain.Option{
				{Key: "approve", Label: "Setujui " + Rp(newLimit), Style: "primary", Result: fmt.Sprintf("Limit %s · Odoo diperbarui, %s menyampaikan ke %s", Rp(newLimit), d.Owner.Name, Primary(d).Name)},
				{Key: "partial", Label: "Setujui " + Rp(mid), Style: "ghost", Result: fmt.Sprintf("Limit %s · dicatat dengan alasan", Rp(mid))},
				{Key: "skip", Label: "Nanti", Style: "quiet", Result: "Ditunda · diingatkan lagi saat exposure > 80% limit"},
			},
			Confidence: 0.87, SignalIDs: provenance(d, 3), Autonomy: "approve",
			Payload:   map[string]any{"limit": d.CreditLimit, "new_limit": newLimit, "partial_limit": mid},
			DedupeKey: fmt.Sprintf("credit:limit:%s:%s", d.ID, in.Today.Format("2006-01")),
		}
		polish(ctx, r, in, &p, "credit", map[string]any{"dealer": d.Name, "on_time": cr.OnTime, "pay_days": cr.PayDays, "growth_pct": growth, "limit": Rp(d.CreditLimit), "new_limit": Rp(newLimit)}, nil)
		out = append(out, p)
	}
	return out, nil
}

// monthlyGrowth compares the last three full months with the three before, in % (0 when not growing).
func monthlyGrowth(in *Input, d *Dealer) int {
	_ = in
	months := d.MonthlyOrders
	if len(months) < 6 {
		return 0
	}
	var a, b int64
	for _, x := range months[:3] {
		a += x
	}
	for _, x := range months[2:5] { // up to the last complete month
		b += x
	}
	if a == 0 || b <= a {
		return 0
	}
	return int(math.Round((float64(b)/float64(a) - 1) * 100))
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func tone(cond bool, t string) string {
	if cond {
		return t
	}
	return ""
}
