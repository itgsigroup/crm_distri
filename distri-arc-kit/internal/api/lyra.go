package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/hermes"
	"distri-arc/internal/httpx"
	"distri-arc/internal/importer"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
)

// Lyra di bawah bubble (ADR 0070): a customer message asking about products gets a card from the company's Hermes
// Agent — stock per city and warehouse and the DPP price, with a discount the sales sets before sending it.

// lyraAutoWindow: only messages this recent are read without a click (opening an old chat does not flood Hermes).
const lyraAutoWindow = 3 * 24 * time.Hour

// lyraAutoPerLoad caps the readings one chat load starts (newest first); the rest start on the next load.
const lyraAutoPerLoad = 3

func (s *Server) lyraRoutes(r chi.Router) {
	r.Post("/chat/threads/{id}/messages/{mid}/lyra", s.lyraRead)
}

// lyraAsks: a message Lyra may read — a customer's or a colleague's (Sam 28 Oct: internal chats and groups too, so a
// stock question between staff gets the card as well).
func lyraAsks(m MessageView) bool {
	return m.Direction == "in" && !m.Deleted && !m.AutoReply && (m.Media == nil || m.Media.Kind == "image")
}

func lyraQuestion(t gen.GetThreadRow, m MessageView) hermes.Question {
	return hermes.Question{MessageID: m.ID, ThreadID: t.ID, Text: hermes.Caption(m.Body), Image: m.Media != nil && m.Media.Kind == "image" && m.Media.Viewable, SentAt: m.SentAt}
}

// lyraImage fetches a message's photo from WhatsApp for Lyra.
func (s *Server) lyraImage(thread uuid.UUID) hermes.ImageFunc {
	return func(ctx context.Context, mid uuid.UUID) (string, []byte, error) {
		m, err := s.st.Q.GetThreadMessage(ctx, gen.GetThreadMessageParams{ThreadID: thread, ID: mid})
		if err != nil {
			return "", nil, err
		}
		var md wa.Media
		if json.Unmarshal(m.Media, &md) != nil || !md.Viewable() {
			return "", nil, errors.New("foto tidak tersimpan")
		}
		f := s.mediaFetcher()
		if f == nil {
			return "", nil, errors.New("WhatsApp Baileys belum tersambung")
		}
		body, err := f.Media(ctx, deref(m.HolderAccount), md)
		if err != nil {
			return "", nil, err
		}
		defer body.Close()
		data, err := io.ReadAll(io.LimitReader(body, 4<<20+1))
		mime, _, _ := strings.Cut(md.Mime, ";")
		return strings.TrimSpace(mime), data, err
	}
}

// attachLyra adds Lyra's cards to a customer chat's messages, and has Lyra read the recent ones that ask about
// products and were not read yet.
func (s *Server) attachLyra(ctx context.Context, t gen.GetThreadRow, msgs []MessageView, now time.Time) error {
	var idx []int
	for i, m := range msgs {
		if lyraAsks(m) && now.Sub(m.SentAt) <= suggestWindow {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(idx))
	for j, i := range idx {
		ids[j] = msgs[i].ID
	}
	h := s.hermes()
	reads, err := h.Reads(ctx, ids)
	if err != nil {
		return err
	}
	rules, err := s.replyAI.Rules(ctx)
	if err != nil {
		return err
	}
	// a reading the trigger started shows only while its message asks about stock, price or a product (a reading
	// of an earlier, wider trigger — any photo — is hidden); one asked from the bubble always shows
	for id, r := range reads {
		if !r.Manual && r.Status != "pending" {
			for _, i := range idx {
				if msgs[i].ID == id && !hermes.Asks(lyraQuestion(t, msgs[i]), rules) {
					delete(reads, id)
				}
			}
		}
	}
	started := 0
	for j := len(idx) - 1; j >= 0 && started < lyraAutoPerLoad; j-- {
		m := msgs[idx[j]]
		if _, ok := reads[m.ID]; ok || now.Sub(m.SentAt) > lyraAutoWindow {
			continue
		}
		q := lyraQuestion(t, m)
		if !hermes.Asks(q, rules) {
			continue
		}
		if ok, err := h.ReadProducts(ctx, q, s.lyraImage(t.ID), false); err != nil {
			return err
		} else if ok {
			started++
			reads[m.ID] = hermes.ProductRead{MessageID: m.ID, Status: "pending", CreatedAt: now}
		}
	}
	var skus []string
	shown := 0
	for _, r := range reads {
		if r.Status == "none" {
			continue
		}
		shown++
		for _, it := range r.Items {
			skus = append(skus, it.SKU)
		}
	}
	if shown == 0 {
		return nil
	}
	var products []replyrule.Product
	var wh []hermes.WarehouseQty
	tier := ""
	var stock []domain.StockItem
	if len(skus) > 0 {
		st, err := s.st.Q.ListStockItems(ctx)
		if err != nil {
			return err
		}
		stock = views.StockItems(st)
		if products, err = s.replyAI.Products(ctx); err != nil {
			return err
		}
		if wh, err = s.lyraWarehouses(ctx, skus); err != nil {
			return err
		}
		if t.DealerID != nil {
			_ = s.st.Pool.QueryRow(ctx, "select coalesce(tier, '') from dealers where id = $1", *t.DealerID).Scan(&tier)
		}
	}
	for _, i := range idx {
		r, ok := reads[msgs[i].ID]
		if !ok || r.Status == "none" {
			continue
		}
		msgs[i].Lyra = hermes.BuildCard(r, stock, wh, products, tier)
	}
	return nil
}

// lyraWarehouses is the staged stock import of these SKUs per warehouse, each mapped to its city (branch) the same
// way the import maps it: the warehouse mapping, else the warehouse name itself.
func (s *Server) lyraWarehouses(ctx context.Context, skus []string) ([]hermes.WarehouseQty, error) {
	rows, err := s.st.Pool.Query(ctx, `select r.data->>'sku', coalesce(r.data->>'warehouse', ''), coalesce(nullif(m.target, ''), trim(r.data->>'warehouse')), coalesce(r.data->>'qty', '')
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
	out := make([]hermes.WarehouseQty, 0, len(sum))
	for k, q := range sum {
		out = append(out, hermes.WarehouseQty{SKU: k[0], Warehouse: k[1], City: k[2], Qty: q})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Warehouse < out[j].Warehouse })
	return out, nil
}

// lyraRead has Lyra read (or read again) one message: the "Tanya Lyra" / "Baca ulang" button.
func (s *Server) lyraRead(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "mid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	m, err := s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ThreadID: t.ID, ID: mid})
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	v := MessageView{ID: m.ID, Direction: deref(m.Direction), Body: deref(m.Body), SentAt: m.SentAt, Media: mediaView(m.Media), Deleted: m.DeletedAt != nil}
	if v.Direction != "in" || v.Deleted || (v.Media != nil && v.Media.Kind != "image") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", hermes.AgentName+" hanya membaca pesan teks atau foto yang masuk")
		return
	}
	started, err := s.hermes().ReadProducts(r.Context(), lyraQuestion(t, v), s.lyraImage(t.ID), true)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if !started {
		httpx.Fail(w, http.StatusConflict, "busy", hermes.AgentName+" masih membaca pesan ini")
		return
	}
	s.auditUser(r, "hermes.product_read", "message:"+mid.String(), nil)
	httpx.JSON(w, http.StatusAccepted, map[string]any{"status": "pending"})
}
