package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"distri-arc/internal/llm"
)

// Balas otomatis seperti sales (ADR 0078): during the hours the CEO approved, Lyra answers any customer message the
// way a sales answers on WhatsApp — a greeting, the answer to what was asked, a next step. Every number comes from the
// facts GSI Orbit computed (the product card, the AI Trigger answers); the caller checks the reply's numbers against
// them. The customer's name, numbers and e-mails are masked on the way to Lyra and put back in the reply.

// SalesInput is one customer message with what GSI Orbit knows for the answer.
type SalesInput struct {
	Name      string     // how the customer is greeted: the contact's or dealer's name, "Kak" without one
	Message   string     // the customer's text
	History   []ChatLine // the conversation before it, oldest first
	Facts     []string   // what the system computed for it (stock, price, order, credit), ready to quote
	Back      string     // when the sales is back, "13.00"
	PartOfDay string     // pagi | siang | sore
}

// ChatLine is one earlier message of the conversation.
type ChatLine struct {
	In   bool
	Text string
}

// SalesAnswer is Lyra's reply, or why a person should answer instead.
type SalesAnswer struct {
	Reply     string
	NeedSales bool   // an order, a negotiation, a complaint, a question the facts do not answer
	Note      string // why, for the log
}

// salesAnswer is what Lyra is asked to answer with.
type salesAnswer struct {
	Reply     string `json:"balasan"`
	NeedSales bool   `json:"perlu_sales"`
	Note      string `json:"catatan"`
}

// SalesReply has Lyra write the reply. ErrNotConnected when no Hermes is saved (nothing is made up in demo mode).
func (s Service) SalesReply(ctx context.Context, in SalesInput) (SalesAnswer, error) {
	agent, demo := s.agent(ctx)
	if demo {
		return SalesAnswer{}, ErrNotConnected
	}
	m := llm.NewMasker()
	names := []string{}
	if in.Name != "" && in.Name != "Kak" {
		names = append(names, in.Name)
	}
	hist := make([]map[string]string, 0, len(in.History))
	for _, h := range in.History {
		from := "kami"
		if h.In {
			from = "pelanggan"
		}
		hist = append(hist, map[string]string{"dari": from, "teks": m.Mask(h.Text, names)})
	}
	facts := make([]string, len(in.Facts))
	for i, f := range in.Facts {
		facts[i] = m.Mask(f, names)
	}
	input, _ := json.Marshal(map[string]any{"pesan": m.Mask(in.Message, names), "riwayat": hist, "fakta": facts,
		"sales_kembali": in.Back, "waktu": in.PartOfDay})
	answer, err := agent.Chat(ctx, "orbit-lyra-sales", []Msg{{Role: "system", Content: salesPrompt}, {Role: "user", Content: string(input)}})
	if err != nil {
		return SalesAnswer{}, err
	}
	var a salesAnswer
	if err := json.Unmarshal([]byte(jsonBlock(answer)), &a); err != nil {
		return SalesAnswer{}, errors.New("jawaban " + AgentName + " tidak terbaca (bukan JSON)")
	}
	reply := strings.TrimSpace(strings.ReplaceAll(m.Unmask(a.Reply), "{nama}", in.Name))
	return SalesAnswer{Reply: reply, NeedSales: a.NeedSales, Note: strings.TrimSpace(a.Note)}, nil
}

// SalesFooter marks the reply as automatic (chat_messages.auto_reply), so the chat stays "Belum dibalas" for the sales.
const SalesFooter = "_— " + AgentName + ", asisten GSI (pesan otomatis)_"

const salesPrompt = `Anda Lyra, asisten sales PT Gosyen Solusi Indonesia (GSI), distributor CCTV, jaringan, dan fire alarm. Sales sedang istirahat; Anda membalas SATU pesan WhatsApp pelanggan seperti sales GSI yang ramah dan sigap.

Input JSON: "pesan" (pesan pelanggan), "riwayat" (chat sebelumnya, terlama dulu; "kami" = GSI), "fakta" (data dari sistem GSI untuk pesan ini: stok, harga DPP, status order, limit/tagihan — sudah benar), "sales_kembali" (jam sales kembali, WIB), "waktu" (pagi/siang/sore).

Cara membalas:
- Bahasa Indonesia santai-sopan seperti chat WhatsApp sales: sapa dengan "{nama}" (tulis persis {nama}, sistem mengganti dengan nama pelanggan), panggil "Bapak/Ibu" atau "Kak", jawab langsung yang ditanya, singkat (2–6 kalimat), boleh pakai *tebal* WhatsApp untuk nama barang dan harga. Tanpa emoji berlebihan.
- Angka (stok, harga, jumlah, tanggal, nomor order, limit, tagihan) HANYA dari "fakta" atau dari pesan pelanggan. Jangan menghitung atau menulis angka lain. Harga selalu "harga DPP, belum termasuk PPN".
- Bila "fakta" kosong untuk yang ditanya (mis. barang tidak dikenal, spesifikasi, ongkir, jadwal kirim), jangan menebak: katakan akan dicek dan sales akan mengabari setelah jam {sales_kembali}.
- Jangan pernah menjanjikan diskon, harga khusus, kredit/tempo, tanggal kirim, atau stok yang ditahan; jangan menerima pesanan atas nama sales. Untuk itu katakan sales akan menindaklanjuti setelah jam sales_kembali.
- Salam, terima kasih, atau basa-basi: balas singkat dan ramah.
- Jangan mengaku manusia; jangan menyebut sistem, AI, atau data internal.
- perlu_sales = true bila pelanggan ingin memesan, menawar, komplain, menagih janji, atau bertanya hal yang tidak ada di fakta.
- Jawab HANYA JSON tanpa teks lain:
{"balasan": "teks WhatsApp untuk pelanggan", "perlu_sales": false, "catatan": "singkat, untuk sales"}`
