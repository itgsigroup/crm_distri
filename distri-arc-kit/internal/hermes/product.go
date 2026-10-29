package hermes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/importer"
	"distri-arc/internal/llm"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store/gen"
)

// Lyra di bawah bubble (ADR 0070): a customer message that asks about products — in words or with a photo — is read
// by the company's Hermes Agent (Lyra), which only names the products (SKU + quantity asked) among GSI Orbit's
// catalog. The stock per city and warehouse and the DPP price are computed here from GSI Orbit's own data when the
// chat is shown (BuildCard), so they are as fresh as the last stock sync and never written by the agent. The sales
// sets a discount, edits the message and sends it; nothing reaches the customer without that click.

// AgentName is how the chat names the company's Hermes Agent.
const AgentName = "Lyra"

// productCandidates is how many catalog products Lyra chooses from (the same as AI Trigger's Claude).
const productCandidates = 40

// maxImage: a larger photo is not sent to Lyra (the card says so).
const maxImage = 4 << 20

// readSlots caps the readings running at once: Hermes runs one agent loop per question on one device.
var readSlots = make(chan struct{}, 2)

// ProductItem is one product Lyra found in a message.
type ProductItem struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"` // asked; 0 = not said
}

// ProductRead is Lyra's stored reading of one message.
type ProductRead struct {
	MessageID  uuid.UUID
	Status     string // pending | done | none (asks no product) | error
	Items      []ProductItem
	Note       string
	Demo       bool
	Manual     bool // asked from the bubble (Tanya Lyra / Baca ulang), not by the trigger
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// Question is a customer message that may ask about products.
type Question struct {
	MessageID uuid.UUID
	ThreadID  uuid.UUID
	Text      string // the text or the photo's caption
	Image     bool   // the message is a photo
	SentAt    time.Time
}

// ImageFunc fetches a message's photo (from WhatsApp).
type ImageFunc func(ctx context.Context, messageID uuid.UUID) (mime string, data []byte, err error)

var reTok = regexp.MustCompile(`[\pL\d]+`)

func toks(s string) []string { return reTok.FindAllString(strings.ToLower(s), -1) }

var reMediaTag = regexp.MustCompile(`^\[[^\]]*\]\s*`)

// Caption is a message's text without the "[Foto]" placeholder a media message is stored with.
func Caption(body string) string { return strings.TrimSpace(reMediaTag.ReplaceAllString(body, "")) }

// ProductRule is the product trigger Lyra adds to AI Trigger's stock and price rules (Sam 28 Oct: "stok, harga,
// produk"): its words also count when those rules are switched off.
var ProductRule = replyrule.Rule{Name: "Tanya produk", Action: "product", Keywords: []string{"produk", "barang", "stok", "harga", "ready", "pricelist"}}

// Asks reports whether a message is read by Lyra without a click: its text (a photo's caption) has a word of the
// AI Trigger stock or price rules, or of ProductRule. A photo without such a word is not read (a screenshot sent to
// a colleague is no product question); the sales can still ask Lyra from the bubble's menu.
func Asks(q Question, rules []replyrule.Rule) bool {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return false
	}
	for _, r := range append(rules, ProductRule) {
		if (r.Action == replyrule.ActionStock || r.Action == replyrule.ActionPrice || r.Action == ProductRule.Action) && replyrule.Matches(r, text) != "" {
			return true
		}
	}
	return false
}

// Reads returns the stored readings of these messages.
func (s Service) Reads(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductRead, error) {
	out := map[uuid.UUID]ProductRead{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.Pool.Query(ctx, `select message_id, status, items, note, demo, manual, created_at, finished_at from hermes_product_reads where message_id = any($1)`, ids)
	if err != nil {
		return nil, err
	}
	reads, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ProductRead, error) {
		var p ProductRead
		var items []byte
		err := r.Scan(&p.MessageID, &p.Status, &items, &p.Note, &p.Demo, &p.Manual, &p.CreatedAt, &p.FinishedAt)
		_ = json.Unmarshal(items, &p.Items)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	for _, p := range reads {
		if p.Status == "pending" && time.Since(p.CreatedAt) > staleAfter {
			p.Status, p.Note = "error", AgentName+" belum menjawab — baca ulang"
		}
		out[p.MessageID] = p
	}
	return out, nil
}

// ReadProducts claims the message and has Lyra read it in the background (the chat polls the card). force reads a
// message again (Baca ulang); a reading still running is not started twice. It reports whether one was started.
func (s Service) ReadProducts(ctx context.Context, q Question, image ImageFunc, force bool) (bool, error) {
	sql := `insert into hermes_product_reads (message_id, thread_id) values ($1, $2) on conflict do nothing`
	if force {
		sql = `insert into hermes_product_reads (message_id, thread_id, manual) values ($1, $2, true) on conflict (message_id) do update
			set status = 'pending', items = '[]', note = '', demo = false, manual = true, created_at = now(), finished_at = null
			where hermes_product_reads.status <> 'pending' or hermes_product_reads.created_at < now() - $3::interval`
	}
	args := []any{q.MessageID, q.ThreadID}
	if force {
		args = append(args, staleAfter.String())
	}
	tag, err := s.Pool.Exec(ctx, sql, args...)
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	go s.readProducts(q, image)
	return true, nil
}

// ReadNow has Lyra read a message and waits for the reading (Balas otomatis, ADR 0066): a reading the chat already
// started is waited for, not started twice. ErrNotConnected when no Hermes is saved (no demo answer goes to a customer).
func (s Service) ReadNow(ctx context.Context, q Question, image ImageFunc) (ProductRead, error) {
	if _, demo := s.agent(ctx); demo {
		return ProductRead{}, ErrNotConnected
	}
	tag, err := s.Pool.Exec(ctx, `insert into hermes_product_reads (message_id, thread_id) values ($1, $2) on conflict do nothing`, q.MessageID, q.ThreadID)
	if err != nil {
		return ProductRead{}, err
	}
	if tag.RowsAffected() == 1 {
		s.read(ctx, q, image)
	}
	for {
		reads, err := s.Reads(ctx, []uuid.UUID{q.MessageID})
		if err != nil {
			return ProductRead{}, err
		}
		if r := reads[q.MessageID]; r.Status != "pending" {
			return r, nil
		}
		select {
		case <-ctx.Done():
			return ProductRead{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (s Service) readProducts(q Question, image ImageFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), staleAfter)
	defer cancel()
	s.read(ctx, q, image)
}

// read runs one claimed reading and stores it.
func (s Service) read(ctx context.Context, q Question, image ImageFunc) {
	select {
	case readSlots <- struct{}{}:
		defer func() { <-readSlots }()
	case <-ctx.Done():
		return
	}
	start := time.Now()
	r, err := s.findProducts(ctx, q, image)
	status := r.Status
	if err != nil {
		status, r.Note = "error", err.Error()
	}
	if s.Log != nil {
		s.Log.Info("hermes product read", "purpose", "hermes.product", "message", q.MessageID, "status", status, "items", len(r.Items), "demo", r.Demo, "ms", time.Since(start).Milliseconds())
	}
	if r.Items == nil {
		r.Items = []ProductItem{}
	}
	items, _ := json.Marshal(r.Items)
	if _, err := s.Pool.Exec(context.Background(), `update hermes_product_reads set status = $2, items = $3, note = $4, demo = $5, finished_at = now()
		where message_id = $1 and status = 'pending'`, q.MessageID, status, items, r.Note, r.Demo); err != nil {
		if s.Log != nil {
			s.Log.Error("hermes product read: store", "err", err)
		}
		return
	}
	_ = events.Notify(context.Background(), s.Pool, "chat_message", map[string]string{"thread_id": q.ThreadID.String()})
}

// lyraAnswer is what Lyra is asked to answer with.
type lyraAnswer struct {
	AsksProduct bool `json:"tanya_produk"`
	Items       []struct {
		SKU  string `json:"sku"`
		Name string `json:"nama"`
		Qty  int    `json:"qty"`
	} `json:"items"`
	Note string `json:"catatan"`
}

// findProducts asks Lyra (or picks by words in demo mode) which catalog products the message asks about.
func (s Service) findProducts(ctx context.Context, q Question, image ImageFunc) (ProductRead, error) {
	out := ProductRead{MessageID: q.MessageID, Status: "done"}
	qs := gen.New(s.Pool)
	st, err := qs.ListStockItems(ctx)
	if err != nil {
		return out, err
	}
	stock := make([]domain.StockItem, 0, len(st))
	for _, x := range st {
		stock = append(stock, domain.StockItem{Branch: x.Branch, SKU: x.Sku, Name: x.Name, Qty: int(x.Qty)})
	}
	known := map[string]string{} // lower SKU → SKU
	for _, x := range stock {
		known[strings.ToLower(x.SKU)] = x.SKU
	}
	if ps, err := qs.ListProducts(ctx); err == nil {
		for _, p := range ps {
			if p.Sku != nil && *p.Sku != "" && known[strings.ToLower(*p.Sku)] == "" {
				known[strings.ToLower(*p.Sku)] = *p.Sku
				stock = append(stock, domain.StockItem{SKU: *p.Sku, Name: p.Name}) // a product without stock: 0 everywhere
			}
		}
	}
	hist, _ := qs.ReplyTriggerHistory(ctx, gen.ReplyTriggerHistoryParams{ThreadID: q.ThreadID, Before: q.SentAt})
	history := make([]replyrule.Line, 0, len(hist))
	for _, h := range hist {
		history = append(history, replyrule.Line{In: h.Direction == "in", Text: Caption(h.Body)})
	}
	text := strings.TrimSpace(q.Text)
	cands := replyrule.Candidates(text, history, stock, productCandidates)

	agent, demo := s.agent(ctx)
	if demo {
		out.Demo = true
		if len(cands) == 0 || text == "" {
			out.Note = "Mode demo — " + AgentName + " belum disambungkan, foto dan pesan tanpa nama barang belum bisa dibaca."
			return out, nil
		}
		out.Items = []ProductItem{{SKU: cands[0].SKU, Qty: askedQty(text)}}
		out.Note = "Mode demo — " + AgentName + " belum disambungkan; barang dipilih dari kata di pesan."
		return out, nil
	}

	var images []string
	if q.Image && image != nil {
		mime, data, err := image(ctx, q.MessageID)
		switch {
		case err != nil:
			out.Note = "Foto tidak bisa diambil dari WhatsApp (" + err.Error() + ") — dibaca dari teks saja. "
		case len(data) > maxImage:
			out.Note = "Foto terlalu besar untuk dibaca — dibaca dari teks saja. "
		default:
			if mime == "" {
				mime = "image/jpeg"
			}
			images = append(images, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
		}
	}
	if len(images) == 0 && text == "" {
		out.Status = "none"
		return out, nil
	}
	m := llm.NewMasker()
	riwayat := make([]map[string]string, 0, len(history))
	for i := min(len(history), replyrule.HistoryLines) - 1; i >= 0; i-- {
		from := "kami"
		if history[i].In {
			from = "pelanggan"
		}
		riwayat = append(riwayat, map[string]string{"dari": from, "teks": m.Mask(history[i].Text, nil)})
	}
	input, _ := json.Marshal(map[string]any{"pesan": m.Mask(text, nil), "ada_foto": len(images) > 0, "riwayat": riwayat, "kandidat": cands})
	answer, err := agent.Chat(ctx, "orbit-lyra-produk", []Msg{
		{Role: "system", Content: productPrompt},
		{Role: "user", Content: string(input), Images: images},
	})
	if err != nil && len(images) > 0 { // a Hermes model without vision: read the text alone
		out.Note = "Foto tidak bisa dibaca " + AgentName + " — dibaca dari teks saja. "
		if text == "" {
			return out, err
		}
		answer, err = agent.Chat(ctx, "orbit-lyra-produk", []Msg{{Role: "system", Content: productPrompt}, {Role: "user", Content: string(input)}})
	}
	if err != nil {
		return out, err
	}
	var a lyraAnswer
	if err := json.Unmarshal([]byte(jsonBlock(answer)), &a); err != nil {
		return out, errors.New("jawaban " + AgentName + " tidak terbaca (bukan JSON)")
	}
	items, missing := resolve(a, known, stock)
	out.Items = items
	note := strings.TrimSpace(a.Note)
	if len(missing) > 0 {
		note = strings.TrimSpace(note + " Tidak ada di katalog: " + strings.Join(missing, ", ") + ".")
	}
	out.Note = strings.TrimSpace(out.Note + note)
	if !a.AsksProduct && len(items) == 0 {
		out.Status = "none"
	}
	return out, nil
}

// resolve keeps the products Lyra named that the catalog knows: by SKU, else by a name whose model code matches one
// product. It returns the names that matched nothing.
func resolve(a lyraAnswer, known map[string]string, stock []domain.StockItem) ([]ProductItem, []string) {
	items := []ProductItem{}
	var missing []string
	seen := map[string]bool{}
	for _, it := range a.Items {
		if len(items) >= 10 {
			break
		}
		sku := known[strings.ToLower(strings.TrimSpace(it.SKU))]
		if sku == "" {
			name := strings.TrimSpace(it.Name + " " + it.SKU)
			if c := replyrule.Candidates(name, nil, stock, 2); len(c) > 0 && sharesCode(name, c[0]) {
				sku = c[0].SKU
			}
		}
		if sku == "" {
			if n := strings.TrimSpace(firstNonEmpty(it.Name, it.SKU)); n != "" {
				missing = append(missing, n)
			}
			continue
		}
		if seen[sku] {
			continue
		}
		seen[sku] = true
		items = append(items, ProductItem{SKU: sku, Qty: min(max(it.Qty, 0), 100000)})
	}
	return items, missing
}

// sharesCode: the name and the candidate share a model code (a word with both letters and digits).
func sharesCode(name string, c replyrule.Candidate) bool {
	have := map[string]bool{}
	for _, t := range append(toks(c.Name), toks(c.SKU)...) {
		have[t] = true
	}
	for _, t := range toks(name) {
		if have[t] && strings.ContainsAny(t, "0123456789") && strings.IndexFunc(t, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

var reQty = regexp.MustCompile(`(?i)(\d{1,5})\s*(unit|pcs|pc|buah|bh|set|roll|box|dus)\b`)

// askedQty is a quantity written as "8 unit" / "2 pcs" (demo mode only).
func askedQty(text string) int {
	if m := reQty.FindStringSubmatch(text); m != nil {
		var n int
		_, _ = fmt.Sscan(m[1], &n)
		return n
	}
	return 0
}

// jsonBlock is the JSON object in an answer that may wrap it in ```json fences or a sentence.
func jsonBlock(s string) string {
	s = strings.TrimSpace(s)
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return s
	}
	return s[i : j+1]
}

const productPrompt = `Anda Lyra, Hermes Agent PT Gosyen Solusi Indonesia (distributor CCTV, jaringan, fire alarm). GSI Orbit meminta Anda membaca SATU pesan WhatsApp pelanggan (teks dan/atau foto) dan menyebut barang yang ditanyakan.

Input JSON: "pesan" (teks pelanggan, bisa kosong bila hanya foto), "ada_foto", "riwayat" (chat sebelumnya, terlama dulu), "kandidat" (barang di katalog GSI: sku, nama, stok total).

Aturan:
- Pilih barang HANYA dari "kandidat" bila ada yang cocok; tulis sku persis. Bila foto/pesan menunjukkan barang yang tidak ada di kandidat, isi "nama" dengan merek + tipe/model yang terbaca (mis. "Ezviz H8C 2MP") dan kosongkan sku.
- Tipe yang tidak jelas jangan ditebak: biarkan items kosong dan jelaskan di catatan.
- qty = jumlah yang diminta pelanggan; 0 bila tidak disebut.
- tanya_produk = true bila pesan/foto menanyakan, memesan, atau meminta harga/stok/spesifikasi barang; false untuk salam, pembayaran, keluhan non-barang, dll.
- Jangan memanggil tool dan jangan menulis harga atau stok — GSI Orbit menghitungnya sendiri.
- Jawab HANYA JSON tanpa teks lain:
{"tanya_produk": true, "items": [{"sku": "...", "nama": "...", "qty": 0}], "catatan": "singkat, Bahasa Indonesia"}`

// WarehouseQty is one warehouse's stock of a SKU (the staged stock import), with the city (branch) it maps to.
type WarehouseQty struct {
	SKU       string
	Warehouse string
	City      string
	Qty       int
}

// Card is what the chat shows under the bubble.
type Card struct {
	Status string `json:"status"` // pending | done | error
	Items  []Line `json:"items"`
	Note   string `json:"note,omitempty"`
	Demo   bool   `json:"demo,omitempty"`
	// Tier: the dealer tier the prices are for; "" = the list price (tier A, the chat has no dealer)
	Tier   string     `json:"tier"`
	ReadAt *time.Time `json:"read_at,omitempty"`
}

// Line is one product of the card.
type Line struct {
	SKU     string `json:"sku"`
	Product string `json:"product"`
	Asked   int    `json:"asked"` // quantity the customer asked; 0 = not said
	Price   int64  `json:"price"` // DPP (before PPN) per unit, rupiah; 0 = no price yet
	Stock   int    `json:"stock"` // every city
	Cities  []City `json:"cities"`
}

// City is a product's stock in one city (branch), with its warehouses when there are several or they are named
// differently from the city.
type City struct {
	City       string      `json:"city"`
	Qty        int         `json:"qty"`
	Warehouses []Warehouse `json:"warehouses,omitempty"`
}

// Warehouse is a product's stock in one warehouse.
type Warehouse struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

// BuildCard computes the card of a reading: stock per city and warehouse, and the DPP price at the dealer's tier.
func BuildCard(r ProductRead, stock []domain.StockItem, wh []WarehouseQty, products []replyrule.Product, tier string) *Card {
	tier = strings.ToUpper(strings.TrimSpace(tier))
	c := &Card{Status: r.Status, Items: []Line{}, Note: r.Note, Demo: r.Demo, Tier: tier, ReadAt: r.FinishedAt}
	if r.Status == "pending" {
		c.Note = ""
	}
	for _, it := range r.Items {
		l := Line{SKU: it.SKU, Asked: it.Qty, Cities: []City{}}
		byCity := map[string]*City{}
		for _, x := range stock {
			if !strings.EqualFold(x.SKU, it.SKU) {
				continue
			}
			if l.Product == "" {
				l.Product = x.Name
			}
			if x.Qty <= 0 {
				continue
			}
			ct := byCity[x.Branch]
			if ct == nil {
				ct = &City{City: x.Branch}
				byCity[x.Branch] = ct
			}
			ct.Qty += x.Qty
			l.Stock += x.Qty
		}
		for _, w := range wh {
			if !strings.EqualFold(w.SKU, it.SKU) || w.Qty <= 0 || byCity[w.City] == nil {
				continue
			}
			byCity[w.City].Warehouses = append(byCity[w.City].Warehouses, Warehouse{Name: w.Warehouse, Qty: w.Qty})
		}
		for _, ct := range byCity {
			if len(ct.Warehouses) == 1 && strings.EqualFold(ct.Warehouses[0].Name, ct.City) {
				ct.Warehouses = nil
			}
			sort.Slice(ct.Warehouses, func(i, j int) bool { return ct.Warehouses[i].Qty > ct.Warehouses[j].Qty })
			l.Cities = append(l.Cities, *ct)
		}
		sort.Slice(l.Cities, func(i, j int) bool {
			if l.Cities[i].Qty != l.Cities[j].Qty {
				return l.Cities[i].Qty > l.Cities[j].Qty
			}
			return l.Cities[i].City < l.Cities[j].City
		})
		for _, p := range products {
			if !strings.EqualFold(p.SKU, it.SKU) {
				continue
			}
			if l.Product == "" {
				l.Product = p.Name
			}
			l.Price = p.ListPrice
			if v := p.Prices[tier]; tier != "" && v > 0 {
				l.Price = v
			} else if l.Price <= 0 {
				l.Price = p.Prices["A"]
			}
			break
		}
		if l.Product == "" {
			l.Product = it.SKU
		}
		c.Items = append(c.Items, l)
	}
	return c
}

// Warehouses is the staged stock import of these SKUs per warehouse, each mapped to its city (branch) the same way
// the import maps it: the warehouse mapping, else the warehouse name itself.
func (s Service) Warehouses(ctx context.Context, skus []string) ([]WarehouseQty, error) {
	rows, err := s.Pool.Query(ctx, `select r.data->>'sku', coalesce(r.data->>'warehouse', ''), coalesce(nullif(m.target, ''), trim(r.data->>'warehouse')), coalesce(r.data->>'qty', '')
		from import_rows r left join data_mappings m on m.kind = 'warehouse' and m.source_value = trim(r.data->>'warehouse')
		where r.entity = 'stock' and r.data->>'sku' = any($1)`, skus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sum := map[[3]string]int{}
	for rows.Next() {
		var sku, w, city, qty string
		if err := rows.Scan(&sku, &w, &city, &qty); err != nil {
			return nil, err
		}
		sum[[3]string{sku, strings.TrimSpace(w), city}] += int(math.Round(importer.Number(qty)))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]WarehouseQty, 0, len(sum))
	for k, q := range sum {
		out = append(out, WarehouseQty{SKU: k[0], Warehouse: k[1], City: k[2], Qty: q})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Warehouse < out[j].Warehouse })
	return out, nil
}

// QuoteText is a card as the message for the customer without a discount — what the sales would send from the card
// untouched (web/src/features/chat/lyra.ts quoteText) — marked as an automatic message.
func QuoteText(c *Card) string { return QuoteTextOf(c, true, true) }

// QuoteTextOf is QuoteText with only the topics the CEO lets the automatic reply give (ADR 0080): without stock no
// stock line, without price "Harga: kami konfirmasi" and no total.
func QuoteTextOf(c *Card, stock, price bool) string {
	out := []string{"Berikut info produk yang ditanyakan:", ""}
	total := int64(0)
	for i, l := range c.Items {
		qty := int64(max(l.Asked, 1))
		num := ""
		if len(c.Items) > 1 {
			num = fmt.Sprintf("%d. ", i+1)
		}
		out = append(out, "*"+num+l.Product+"*")
		switch {
		case !stock:
		case len(l.Cities) > 0:
			cities := make([]string, len(l.Cities))
			for j, ct := range l.Cities {
				cities[j] = ct.City + " " + thousands(int64(ct.Qty))
			}
			out = append(out, "Stok: "+strings.Join(cities, " · "))
		default:
			out = append(out, "Stok: kosong (bisa inden)")
		}
		if !price {
			out = append(out, "Harga: kami konfirmasi")
		} else if l.Price > 0 {
			out = append(out, "Harga: *"+rupiah(l.Price)+"*/unit", thousands(qty)+" unit × "+rupiah(l.Price)+" = *"+rupiah(l.Price*qty)+"*")
			total += l.Price * qty
		} else {
			out = append(out, "Harga: kami konfirmasi segera")
		}
		out = append(out, "")
	}
	if len(c.Items) > 1 && total > 0 {
		out = append(out, "*Total: "+rupiah(total)+"*")
	}
	if price {
		out = append(out, "Harga DPP, belum termasuk PPN.")
	}
	out = append(out, "(Pesan otomatis dari "+AgentName+" — sales kami akan menindaklanjuti.)")
	return strings.Join(out, "\n")
}

// rupiah: "Rp 1.643.500".
func rupiah(v int64) string { return "Rp " + thousands(v) }

// thousands groups digits with dots, as id-ID does.
func thousands(v int64) string {
	s := strconv.FormatInt(v, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
