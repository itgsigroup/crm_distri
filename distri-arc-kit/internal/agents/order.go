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

// Order is AI Order: WhatsApp requests become SO drafts; price negotiations and returns are escalated.
type Order struct{}

func (Order) Name() string { return "AI Order" }
func (Order) Kinds() []string {
	return []string{domain.KindSODraft, domain.KindPriceCounter, domain.KindReturn, domain.KindCreditHold}
}

// stockFor finds the branch stock of a product (same category, most shared name tokens).
func stockFor(in *Input, branch string, p Product) *domain.StockItem {
	var best *domain.StockItem
	bestScore := 0
	pt := tokens(p.Name)
	for i := range in.Stock {
		s := &in.Stock[i]
		if s.Branch != branch || s.Category != p.Category {
			continue
		}
		score := 0
		st := strings.Join(tokens(s.Name), " ")
		for _, t := range pt {
			if strings.Contains(st, t) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = s, score
		}
	}
	if bestScore < 2 {
		return nil
	}
	return best
}

func itemsText(items []Item) string {
	var parts []string
	for _, it := range items {
		parts = append(parts, fmt.Sprintf("%d %s", it.Qty, shortProduct(it.Product.Name)))
	}
	return strings.Join(parts, " + ")
}

// shortProduct writes "kamera 4MP" for "Kamera IP 4MP" the way sales do.
func shortProduct(n string) string {
	n = strings.Replace(n, "Kamera IP ", "kamera ", 1)
	n = strings.Replace(n, "Kamera ", "kamera ", 1)
	return n
}

// marginAfter returns the gross margin (%) of a line at a discount.
func marginAfter(price, cost int64, discountPct float64) float64 {
	p := float64(price) * (1 - discountPct/100)
	if p <= 0 {
		return 0
	}
	return (p - float64(cost)) / p * 100
}

// Analyze reads recent inbound WhatsApp messages.
func (a Order) Analyze(ctx context.Context, in *Input, r *llm.Router) ([]domain.Proposal, error) {
	var out []domain.Proposal
	floor := in.Policies.Margin.Pct
	for _, m := range Latest(in) {
		d := in.Dealer(m.DealerID)
		if d == nil {
			continue
		}
		e := Extract(m.Text, in.Products)
		names := []string{m.Contact}
		dealerID := d.UUID
		switch {
		case e.Intent == IntentOrder && len(e.Items) > 0:
			out = append(out, a.soDraft(ctx, in, r, d, m, e, names))
		case e.Intent == IntentPrice && e.DiscountPct > 0 && len(e.Items) > 0:
			it := e.Items[0]
			price, cost := it.Product.Price(d.Tier), it.Product.Cost
			base := marginAfter(price, cost, 0)
			asked := marginAfter(price, cost, e.DiscountPct)
			value := price * it.Qty
			// the largest discount (in 0,5% steps, 0,5% under the floor limit) that keeps the margin above the floor
			maxD := (1 - float64(cost)/(float64(price)*(1-floor/100))) * 100
			counter := math.Floor((maxD-0.5)*2) / 2
			if counter < 0 {
				counter = 0
			}
			cm := marginAfter(price, cost, counter)
			sweet := ""
			for _, s := range in.Stock {
				if s.Branch == d.Branch && metrics.IsAging(s, in.Policies) && s.Category == it.Product.Category && !strings.Contains(strings.ToLower(s.Name), "kamera") {
					sweet = s.Name
					break
				}
			}
			if asked >= floor {
				continue // within policy: AI Order prices it directly from the tier (stage 06 autonomy)
			}
			contact := m.Contact
			p := domain.Proposal{
				Agent: a.Name(), DealerID: &dealerID, Kind: domain.KindPriceCounter, Icon: "cash", Button: "Setujui counter", DueLabel: "Hari ini",
				Title:   fmt.Sprintf("Jawab permintaan harga %s: counter %s + ongkir gratis", d.Name, pctWhole(counter)),
				Summary: fmt.Sprintf("%s menyebut distributor lain %s lebih murah. Floor margin %s.", contact, pctWhole(e.DiscountPct), pctWhole(floor)),
				Why: fmt.Sprintf("Diminta %s di bawah tier %s untuk %d unit → margin %s, di bawah floor %s. %s bayar %d hari, order tiap %d hari, %d%% tepat waktu. Counter %s + ongkir gratis menjaga margin %s dan menjawab keluhan harga.",
					pctWhole(e.DiscountPct), d.Tier, it.Qty, Pct(asked), pctWhole(floor), Short(d.Name), d.Metrics.Credit.PayDays, deref(d.Metrics.Rhythm), d.Metrics.Credit.OnTime, pctWhole(counter), Pct(cm)),
				Prep:    fmt.Sprintf("Draft balasan ke %s dengan dua opsi: %s + ongkir gratis%s.", contact, pctWhole(counter), bundleText(sweet)),
				Preview: fmt.Sprintf("%s, untuk %d unit kami bisa %s di bawah harga biasa plus ongkir gratis ke gudang %s.%s Mana yang lebih pas?", contact, it.Qty, pctWhole(counter), Bapak(contact), bundlePreview(sweet)),
				Steps:   []string{"Balasan masuk antrean " + d.Owner.Name, "Harga khusus dicatat di SO dengan alasan", "AI Order mencatat kompetitor disebut"},
				Pills:   [][2]string{{"warn", a.Name()}, {"neutral", fmt.Sprintf("%d unit %s", it.Qty, shortProduct(it.Product.Name))}},
				Impact: []domain.Impact{{Label: "Margin tier " + d.Tier, Value: Pct(base)}, {Label: "Jika " + pctWhole(e.DiscountPct), Value: Pct(asked), Tone: "bad"},
					{Label: "Counter " + pctWhole(counter) + " + ongkir", Value: Pct(cm), Tone: "good"}, {Label: "Nilai order", Value: Rp(value)}},
				Options: []domain.Option{
					{Key: "approve", Label: "Setujui counter " + pctWhole(counter), Style: "primary", Result: "Counter disetujui · balasan masuk antrean " + d.Owner.Name, Sends: true},
					{Key: "override", Label: "Setujui " + pctWhole(e.DiscountPct) + " (di bawah floor)", Style: "ghost", Result: "Disetujui dengan pengecualian · dicatat di audit, floor tetap " + pctWhole(floor), Sends: true,
						Preview: fmt.Sprintf("%s, untuk %d unit kami samakan %s di bawah harga biasa. Saya buatkan SO-nya ya?", contact, it.Qty, pctWhole(e.DiscountPct))},
					{Key: "reject", Label: "Tolak", Style: "quiet", Result: "Ditolak · " + d.Owner.Name + " diminta menawarkan harga tier"},
				},
				Confidence: 0.84, SignalIDs: []uuid.UUID{m.SignalID}, Autonomy: "approve",
				Payload:   map[string]any{"product": it.Product.Name, "qty": it.Qty, "discount_pct": e.DiscountPct, "counter_pct": counter, "margin_asked": asked, "margin_counter": cm, "to": contact},
				DedupeKey: "order:price:" + m.SignalID.String(),
			}
			polish(ctx, r, in, &p, "order", map[string]any{"dealer": d.Name, "pic": contact, "message": m.Text, "qty": it.Qty, "product": it.Product.Name, "asked_pct": e.DiscountPct,
				"margin_asked": Pct(asked), "counter_pct": counter, "margin_counter": Pct(cm), "floor": floor, "sweetener": sweet}, names)
			out = append(out, p)
		case e.Intent == IntentReturn && len(e.Items) > 0:
			it := e.Items[0]
			value := it.Product.Price(d.Tier) * it.Qty
			contact := m.Contact
			p := domain.Proposal{
				Agent: a.Name(), DealerID: &dealerID, Kind: domain.KindReturn, Icon: "refresh", Button: "Setujui retur", DueLabel: "Hari ini",
				Title:   fmt.Sprintf("Setujui retur %d %s %s (ganti unit)", it.Qty, it.Product.Name, d.Name),
				Summary: "Unit mati total setelah 2 minggu. RMA supplier bisa diajukan, biaya Rp 0.",
				Why:     fmt.Sprintf("Unit mati total dalam 2 minggu; garansi supplier berlaku (biaya Rp 0). Dealer %s — retur yang cepat menjaga order berikutnya.", dueText(d)),
				Prep:    fmt.Sprintf("RMA ke supplier, surat jalan tukar dari gudang %s, pesan ke %s.", d.Branch, contact),
				Preview: fmt.Sprintf("%s, retur %d %s sudah kami setujui. Unit pengganti kami kirim bersama order berikutnya atau terpisah, mana yang lebih cepat untuk %s.", contact, it.Qty, it.Product.Name, Bapak(contact)),
				Steps:   []string{"RMA dibuat", "Unit pengganti dikirim bersama order berikutnya atau terpisah (pilihan " + d.Owner.Name + ")", "Dicatat di riwayat dealer"},
				Pills:   [][2]string{{"indigo", a.Name()}, {"neutral", "Garansi supplier berlaku"}},
				Impact:  []domain.Impact{{Label: "Biaya GSI", Value: "Rp 0"}, {Label: "Nilai unit", Value: Rp1(value)}, {Label: "Dealer", Value: fmt.Sprintf("skor dealer %d", d.Metrics.Score), Tone: domain.ScoreBand(d.Metrics.Score)}},
				Options: []domain.Option{
					{Key: "approve", Label: "Setujui tukar unit", Style: "primary", Result: "Retur disetujui · RMA dibuat, unit pengganti dikirim dari " + d.Branch, Sends: true},
					{Key: "check", Label: "Minta unit dicek dulu", Style: "ghost", Result: "Unit diminta dikirim ke service center dulu · " + contact + " diberi tahu", Sends: true,
						Preview: fmt.Sprintf("%s, mohon unit %s dikirim ke service center kami dulu untuk dicek, setelah itu langsung kami tukar.", contact, it.Product.Name)},
					{Key: "reject", Label: "Tolak", Style: "quiet", Result: "Ditolak · alasan dicatat"},
				},
				Confidence: 0.86, SignalIDs: []uuid.UUID{m.SignalID}, Autonomy: "approve",
				Payload:   map[string]any{"product": it.Product.Name, "qty": it.Qty, "value": value, "to": contact},
				DedupeKey: "order:return:" + m.SignalID.String(),
			}
			polish(ctx, r, in, &p, "order", map[string]any{"dealer": d.Name, "pic": contact, "message": m.Text, "qty": it.Qty, "product": it.Product.Name}, names)
			out = append(out, p)
		}
	}
	return out, nil
}

func (a Order) soDraft(ctx context.Context, in *Input, r *llm.Router, d *Dealer, m WAMessage, e Extraction, names []string) domain.Proposal {
	var total, cost int64
	var lines []map[string]any
	stockOK, critical := true, []string{}
	for _, it := range e.Items {
		price := it.Product.Price(d.Tier)
		total += price * it.Qty
		cost += it.Product.Cost * it.Qty
		lines = append(lines, map[string]any{"product": it.Product.Name, "product_odoo_id": it.Product.OdooID, "qty": it.Qty, "price": price})
		s := stockFor(in, d.Branch, it.Product)
		if s == nil || int64(s.Qty) < it.Qty {
			stockOK = false
			continue
		}
		left := *s
		left.Qty -= int(it.Qty)
		if metrics.IsCritical(left, in.Policies) {
			critical = append(critical, s.Name)
		}
	}
	cr := d.Metrics.Credit
	after := cr.Exposure + total
	creditOK := cr.Limit == 0 || (after <= cr.Limit && !cr.Late)
	margin := 0.0
	if total > 0 {
		margin = float64(total-cost) / float64(total) * 100
	}
	dealerID := d.UUID
	contact := m.Contact
	day := e.Day
	if day == "" {
		day = "besok"
	}
	kirim := "kirim " + day
	if e.Day != "" {
		kirim = "kirim " + e.Day + " pagi"
	}
	p := domain.Proposal{
		Agent: a.Name(), DealerID: &dealerID, Kind: domain.KindSODraft, Icon: "check", Button: "Buat SO", DueLabel: "Hari ini",
		Title:      fmt.Sprintf("Buat SO %s: %s (%s)", d.Name, itemsText(e.Items), Rp1(total)),
		Summary:    fmt.Sprintf("Permintaan WA %s: %s.", contact, itemsText(e.Items)),
		Prep:       fmt.Sprintf("SO draft di Odoo dengan %d baris; surat jalan %s; konfirmasi ke %s.", len(e.Items), day, contact),
		Preview:    fmt.Sprintf("%s, order %s total %s sudah kami catat, %s. Terima kasih!", contact, itemsText(e.Items), Rp1(total), kirim),
		Steps:      []string{"SO dikonfirmasi di Odoo", "Konfirmasi WA terkirim"},
		Pills:      [][2]string{{"accent", a.Name()}, {"neutral", fmt.Sprintf("tier %s · %s", d.Tier, Pct(margin))}},
		Impact:     []domain.Impact{{Label: "Nilai SO", Value: Rp1(total)}, {Label: "Margin", Value: Pct(margin)}, {Label: "Exposure setelah", Value: Rp(after)}},
		Confidence: 0.92, SignalIDs: []uuid.UUID{m.SignalID}, Autonomy: "approve",
		Payload:   map[string]any{"lines": lines, "total": total, "margin_pct": margin, "complete": stockOK && creditOK, "to": contact},
		DedupeKey: "order:so:" + m.SignalID.String(),
	}
	stockText := fmt.Sprintf("stok %s cukup", d.Branch)
	if len(critical) > 0 {
		stockText = fmt.Sprintf("stok %s cukup, tapi akan kritis", d.Branch)
		p.Steps = append(p.Steps, fmt.Sprintf("Stok %s menjadi kritis → usul transfer antar cabang", d.Branch))
	}
	if !stockOK {
		stockText = fmt.Sprintf("stok %s tidak cukup", d.Branch)
	}
	limitText := "dealer cash"
	if cr.Limit > 0 {
		limitText = fmt.Sprintf("exposure %s + %s = %s jt ≤ %s jt, tidak ada invoice lewat", Jt(cr.Exposure), Jt(total), Jt(after), Jt(cr.Limit))
		if !creditOK {
			limitText = fmt.Sprintf("exposure %s + %s = %s jt melewati limit %s jt", Jt(cr.Exposure), Jt(total), Jt(after), Jt(cr.Limit))
		}
	}
	p.Why = fmt.Sprintf("Permintaan WA jelas; %s, harga tier %s, %s.", stockText, d.Tier, limitText)
	if !creditOK {
		// over the limit: the SO is held for AI Kredit instead of drafted
		p.Kind, p.Button, p.Icon = domain.KindCreditHold, "Tahan SO", "shield"
		p.Title = fmt.Sprintf("Tahan SO %s: %s melewati limit", d.Name, Rp1(total))
		p.Preview = ""
		p.Steps = []string{"SO draft ditahan", "AI Kredit menyiapkan opsi DP / pelunasan"}
		p.DedupeKey = "order:hold:" + m.SignalID.String()
	}
	polish(ctx, r, in, &p, "order", map[string]any{"dealer": d.Name, "pic": contact, "message": m.Text, "lines": lines, "total": Rp1(total), "margin": Pct(margin),
		"stock_ok": stockOK, "credit_ok": creditOK, "delivery_day": day}, names)
	return p
}

func pctWhole(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d%%", int(f))
	}
	return Pct(f)
}

func bundleText(sweet string) string {
	if sweet == "" {
		return ""
	}
	return fmt.Sprintf(", atau harga tier + bundle %s (stok menua) di harga khusus", sweet)
}

func bundlePreview(sweet string) string {
	if sweet == "" {
		return ""
	}
	return fmt.Sprintf(" Atau, tetap harga biasa tapi %s kami beri harga khusus.", sweet)
}

func dueText(d *Dealer) string {
	m := d.Metrics
	if m.Rhythm == nil || m.DueIn == nil {
		return "baru masuk orbit"
	}
	if *m.DueIn < 0 {
		return fmt.Sprintf("lewat jadwal %d hari", -*m.DueIn)
	}
	return fmt.Sprintf("jadwal order %d hari lagi", *m.DueIn)
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
