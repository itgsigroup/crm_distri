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

// Prospect is AI Prospek: an inbound unknown number → identification → proposed tier C dealer with a first reply.
// Creating the dealer in Odoo and sending prices both wait for a human.
type Prospect struct{}

func (Prospect) Name() string { return "AI Prospek" }
func (Prospect) Kinds() []string {
	return []string{domain.KindNewDealer, domain.KindPriceList}
}

// MinIdentScore is the identification score from which AI Prospek proposes a dealer.
const MinIdentScore = 60

// Analyze proposes new dealers.
func (a Prospect) Analyze(ctx context.Context, in *Input, r *llm.Router) ([]domain.Proposal, error) {
	var out []domain.Proposal
	for _, n := range in.NewNumbers {
		if n.Score < MinIdentScore || n.SignalID == uuid.Nil {
			continue
		}
		city, kind, _ := strings.Cut(n.Org, " · ")
		var srcs []string
		for _, s := range n.Sources {
			if s.OK == "true" {
				srcs = append(srcs, s.Value)
			}
		}
		e := Extract(n.Text, in.Products)
		var quote []string
		var total int64
		for _, it := range e.Items {
			price := it.Product.Price("C")
			total += price * it.Qty
			quote = append(quote, fmt.Sprintf("%d %s @ %s", it.Qty, shortProduct(it.Product.Name), Unit(price)))
		}
		preview := fmt.Sprintf("Selamat siang, terima kasih sudah menghubungi GSI. Untuk toko baru di %s berlaku harga tier C dan ongkir subsidi untuk order ≥ Rp 5 jt. Boleh kami kirim daftar harganya?", city)
		if len(quote) > 0 {
			preview = fmt.Sprintf("Selamat siang, terima kasih sudah menghubungi GSI. Harga tier C: %s, total %s; ongkir ke %s kami subsidi untuk order ≥ Rp 5 jt. Mau kami daftarkan sebagai dealer supaya bisa order langsung?",
				strings.Join(quote, ", "), Rp1(total), city)
		}
		p := domain.Proposal{
			Agent: a.Name(), Kind: domain.KindNewDealer, Icon: "people", Button: "Buat dealer tier C", DueLabel: "Hari ini",
			Title:      fmt.Sprintf("Nomor baru %s (%s) → dealer tier C", n.Name, city),
			Summary:    fmt.Sprintf("Bertanya ke %s: “%s”", n.Sales, n.Text),
			Why:        fmt.Sprintf("Skor identitas %d dari %s. %s", n.Score, Join(srcs), n.Potential),
			Prep:       fmt.Sprintf("Data dealer baru siap dibuat di Odoo (%s, %s, tier C, cash); harga tier C dikirim lewat “Kirim harga”.", n.Name, strings.TrimSpace(city+" · "+kind)),
			Steps:      []string{"Dealer dibuat di Odoo dengan catatan sumber (Stage 12)", "Harga tier C dikirim dari nomor " + n.Sales + " (Kirim harga)", "Order pertama → AI Order menyiapkan SO cash"},
			Pills:      [][2]string{{"accent", a.Name()}, {"neutral", fmt.Sprintf("skor identitas %d", n.Score)}},
			Impact:     []domain.Impact{{Label: "Skor identitas", Value: fmt.Sprintf("%d", n.Score)}, {Label: "Tier usulan", Value: "C · cash"}, {Label: "Kota", Value: city}},
			Confidence: math.Round(float64(n.Score)) / 100, SignalIDs: []uuid.UUID{n.SignalID}, Autonomy: "approve",
			Payload:   map[string]any{"wa_number": n.WANumber, "name": n.Name, "org": n.Org, "city": city, "tier": "C", "sales": n.Sales, "thread_id": n.ThreadID},
			DedupeKey: "new_dealer:" + n.WANumber,
		}
		out = append(out, p)
		// the reply with tier C prices: a message to a number that is not a dealer yet, sent only after approve
		pl := domain.Proposal{
			Agent: a.Name(), Kind: domain.KindPriceList, Icon: "send", Button: "Kirim harga", DueLabel: "Hari ini",
			Title:      fmt.Sprintf("Kirim harga tier C ke %s (%s)", n.Name, city),
			Summary:    fmt.Sprintf("Jawaban untuk: “%s”", n.Text),
			Why:        fmt.Sprintf("Nomor baru bertanya harga ke %s. Harga tier C (cash) untuk toko baru; tidak ada harga di bawah tier C.", n.Sales),
			Prep:       fmt.Sprintf("Draft WA dari nomor %s dengan harga tier C%s.", n.Sales, map[bool]string{true: " dan total", false: ""}[len(quote) > 0]),
			Preview:    preview,
			Steps:      []string{"Dikirim dari nomor " + n.Sales, "Balasan → AI Order menyiapkan SO cash"},
			Pills:      [][2]string{{"accent", a.Name()}, {"neutral", "tier C · cash"}},
			Confidence: math.Round(float64(n.Score)) / 100, SignalIDs: []uuid.UUID{n.SignalID}, Autonomy: "approve",
			Payload:   map[string]any{"wa_number": n.WANumber, "name": n.Name, "sales": n.Sales, "thread_id": n.ThreadID, "total": total},
			DedupeKey: "price_list:" + n.WANumber,
		}
		polish(ctx, r, in, &pl, "prospect", map[string]any{"name": n.Name, "org": n.Org, "score": n.Score, "sources": srcs, "potential": n.Potential, "question": n.Text}, []string{n.Name})
		out = append(out, pl)
	}
	return out, nil
}
