package agents

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/llm"
	"distri-arc/internal/metrics"
	"distri-arc/internal/views"
)

// Collect is AI Penagihan: invoice reminders whose tone follows the dealer's payment pattern, in step with the
// order schedule. H-3 friendly reminders may run on their own; firm reminders and installment plans wait for a
// human. It never threatens, never holds shipments and never mentions penalties outside the terms.
type Collect struct{}

func (Collect) Name() string    { return "AI Penagihan" }
func (Collect) Kinds() []string { return []string{domain.KindCollect, domain.KindInstallment} }

// Analyze proposes at most one reminder per dealer.
func (a Collect) Analyze(ctx context.Context, in *Input, r *llm.Router) ([]domain.Proposal, error) {
	var out []domain.Proposal
	day := in.Today.Format("2006-01-02")
	for _, d := range in.Dealers {
		if d.CreditLimit == 0 || len(d.OpenInvoices) == 0 {
			continue
		}
		var late, soon []views.OpenInvoice
		for _, i := range d.OpenInvoices {
			switch due := metrics.DaysBetween(in.Today, i.DueAt); {
			case i.LateDays > 0:
				late = append(late, i)
			case due >= 0 && due <= 3:
				soon = append(soon, i)
			}
		}
		if len(late) == 0 && len(soon) == 0 {
			continue
		}
		m := d.Metrics
		pic := Primary(d)
		dealerID := d.UUID
		dueSoon := m.DueIn != nil && *m.DueIn >= 0 && *m.DueIn <= 7
		orderLine := ""
		if dueSoon {
			orderLine = " Order berikutnya bisa langsung kami proses setelah pembayaran masuk."
		}
		p := domain.Proposal{Agent: a.Name(), DealerID: &dealerID, Icon: "cash", SignalIDs: invoiceSignals(d, 3), Autonomy: "approve",
			Payload: map[string]any{"to": pic.Name}, DedupeKey: fmt.Sprintf("collect:%s:%s", d.ID, day)}
		switch {
		case len(late) > 0:
			sort.Slice(late, func(i, j int) bool { return late[i].LateDays > late[j].LateDays })
			inv := late[0]
			var total int64
			for _, i := range late {
				total += i.Residual
			}
			p.Payload["invoices"] = numbers(late)
			if inv.LateDays > 14 || d.RootCause == domain.RootProjectUnpaid { // asked for time → a scheme, not a third reminder
				half := total / 2
				p.Kind, p.Button, p.DueLabel, p.Confidence = domain.KindInstallment, "Kirim skema cicilan", "Hari ini", 0.82
				p.Title = fmt.Sprintf("Skema cicilan 2× untuk %s — %s lewat %d hari", d.Name, inv.Number, inv.LateDays)
				p.Why = fmt.Sprintf("%s %s lewat %d hari (pola bayar %d hari, %d%% tepat waktu). Cicilan 2× lebih mungkin dibayar daripada pengingat ketiga.", inv.Number, Rp(inv.Residual), inv.LateDays, m.Credit.PayDays, m.Credit.OnTime)
				p.Prep = fmt.Sprintf("Draft WA dari %s: %s minggu ini dan %s dua minggu lagi.", d.Owner.Name, Rp(half), Rp(total-half))
				p.Preview = fmt.Sprintf("%s, untuk %s (%s) kami bisa bagi dua: %s minggu ini dan %s dua minggu lagi.%s", pic.Name, inv.Number, Rp(total), Rp(half), Rp(total-half), orderLine)
				p.Payload["installments"] = []int64{half, total - half}
				p.Payload["tone"] = "cicilan"
			} else {
				p.Kind, p.Button, p.DueLabel, p.Confidence = domain.KindCollect, "Kirim pengingat", "Hari ini", 0.84
				p.Title = fmt.Sprintf("Pengingat %s %s — lewat %d hari", d.Name, inv.Number, inv.LateDays)
				p.Why = fmt.Sprintf("%s %s lewat %d hari; pola bayar %d hari. Nada tegas tapi sopan — tidak menyebut denda.", inv.Number, Rp(inv.Residual), inv.LateDays, m.Credit.PayDays)
				p.Prep = fmt.Sprintf("Draft WA dari %s dengan nomor invoice dan jumlah.", d.Owner.Name)
				p.Preview = fmt.Sprintf("%s, mengingatkan %s %s sudah lewat jatuh tempo %d hari. Mohon info jadwal pembayarannya ya.%s", pic.Name, inv.Number, Rp(inv.Residual), inv.LateDays, orderLine)
				p.Payload["tone"] = "tegas"
			}
			p.Steps = []string{"Masuk antrean " + d.Owner.Name, "Komitmen bayar dicatat dengan tanggal", "Pembayaran masuk → limit terbuka lagi"}
		default:
			inv := soon[0]
			due := metrics.DaysBetween(in.Today, inv.DueAt)
			when := map[int]string{0: "hari ini", 1: "besok"}[due]
			if when == "" {
				when = fmt.Sprintf("%d hari lagi", due)
			}
			p.Kind, p.Button, p.DueLabel, p.Confidence = domain.KindCollect, "Kirim pengingat", "Hari ini", 0.9
			p.Title = fmt.Sprintf("Pengingat H-3 %s %s — jatuh tempo %s", d.Name, inv.Number, when)
			p.Why = fmt.Sprintf("%s %s jatuh tempo %s; pola bayar %d hari, %d%% tepat waktu — nada ramah.%s", inv.Number, Rp(inv.Residual), when, m.Credit.PayDays, m.Credit.OnTime, tightText(d))
			p.Prep = fmt.Sprintf("Pengingat ramah dari %s.", d.Owner.Name)
			p.Preview = fmt.Sprintf("%s, sekadar mengingatkan %s %s jatuh tempo %s. Terima kasih.%s", pic.Name, inv.Number, Rp(inv.Residual), when, orderLine)
			p.Steps = []string{"Dikirim dari nomor " + d.Owner.Name, "Pembayaran masuk → order berikutnya tidak tertahan limit"}
			p.Payload["invoices"] = numbers(soon)
			p.Payload["tone"] = "ramah"
		}
		polish(ctx, r, in, &p, "collect", map[string]any{"dealer": d.Name, "pic": pic.Name, "sales": d.Owner.Name, "invoices": d.OpenInvoices,
			"pay_days": m.Credit.PayDays, "on_time": m.Credit.OnTime, "tone": p.Payload["tone"], "order_due_in": m.DueIn}, []string{pic.Name, d.Owner.Name})
		out = append(out, p)
	}
	return out, nil
}

func numbers(xs []views.OpenInvoice) []string {
	out := make([]string, 0, len(xs))
	for _, i := range xs {
		out = append(out, i.Number)
	}
	return out
}

func tightText(d *Dealer) string {
	if d.Metrics.Credit.State == domain.CreditTipis {
		return " Sisa limit tipis: tanpa pembayaran ini order berikutnya tertahan."
	}
	return ""
}

// invoiceSignals prefers invoice/payment signals as provenance, then the newest others.
func invoiceSignals(d *Dealer, n int) []uuid.UUID {
	var out []uuid.UUID
	for _, s := range d.Signals {
		if (s.Kind == "invoice" || s.Kind == "payment") && len(out) < n {
			out = append(out, s.ID)
		}
	}
	if len(out) == 0 {
		return provenance(d, n)
	}
	return out
}
