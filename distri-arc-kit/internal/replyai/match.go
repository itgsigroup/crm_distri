package replyai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/analyst"
	"distri-arc/internal/domain"
	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store/gen"
)

// MatchCandidates is how many products of the stock Claude chooses from.
const MatchCandidates = 40

// ErrNoClaude: no Claude key — products are found by words (the card says so).
var ErrNoClaude = errors.New("kunci Claude belum diisi (Pengaturan → API & model AI) — barang dicari dengan kata kunci")

// claude is the Claude model products are found with (Sam 21 Oct: Claude, whatever writes the drafts): the AI
// Trigger engine's model when it is Claude, else the fastest Claude model.
func (s *Service) claude(ctx context.Context) llm.Provider {
	if s.Matcher != nil {
		return s.Matcher
	}
	e := LoadEngine(ctx, s.St.Q)
	if e.Provider != analyst.FallbackClaude {
		e = Engine{Provider: analyst.FallbackClaude, Model: EngineModels[analyst.FallbackClaude][0]}
	}
	return s.providerOf(ctx, e)
}

// HasClaude reports whether Claude can find products (a key is set).
func (s *Service) HasClaude(ctx context.Context) bool { return s.claude(ctx) != nil }

var matchSchema = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"items", "catatan"},
	"properties": map[string]any{
		"items": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"sku", "qty"},
			"properties": map[string]any{"sku": map[string]any{"type": "string"}, "qty": map[string]any{"type": "integer"}}}},
		"catatan": map[string]any{"type": "string"},
	}}

// Find has Claude name the products of a message among the candidates of the stock (nothing stored).
func (s *Service) Find(ctx context.Context, text string, history []replyrule.Line, stock []domain.StockItem) (*replyrule.Match, error) {
	model := s.claude(ctx)
	if model == nil {
		return nil, ErrNoClaude
	}
	cands := replyrule.Candidates(text, history, stock, MatchCandidates)
	if len(cands) == 0 {
		return &replyrule.Match{Items: []replyrule.MatchItem{}, Note: "tidak ada barang di data stok yang mirip", Model: "system"}, nil
	}
	known := map[string]string{}
	for _, c := range cands {
		known[strings.ToLower(c.SKU)] = c.SKU
	}
	riwayat := make([]map[string]string, 0, len(history))
	for i := min(len(history), replyrule.HistoryLines) - 1; i >= 0; i-- {
		from := "kami"
		if history[i].In {
			from = "pelanggan"
		}
		riwayat = append(riwayat, map[string]string{"dari": from, "teks": history[i].Text})
	}
	input, _ := json.Marshal(map[string]any{"pesan": text, "riwayat": riwayat, "kandidat": cands})
	type answer struct {
		Items []struct {
			SKU string `json:"sku"`
			Qty int    `json:"qty"`
		} `json:"items"`
		Catatan string `json:"catatan"`
	}
	check := func(b json.RawMessage) error {
		var a answer
		if err := json.Unmarshal(b, &a); err != nil {
			return err
		}
		if len(a.Items) > 10 {
			return errors.New("terlalu banyak barang")
		}
		for _, it := range a.Items {
			if known[strings.ToLower(it.SKU)] == "" {
				return fmt.Errorf("sku %q bukan kandidat", it.SKU)
			}
			if it.Qty < 0 || it.Qty > 100000 {
				return fmt.Errorf("jumlah %d tidak wajar", it.Qty)
			}
		}
		return nil
	}
	router := &llm.Router{Primary: model, St: s.St, Log: s.Log}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	res := router.Complete(ctx, llm.Request{Purpose: "reply.match", Agent: "AI Trigger", System: llm.Prompt("system") + "\n\n" + llm.Prompt("reply_match"),
		Input: input, Schema: matchSchema, MaxTokens: 400}, check)
	var a answer
	if !llm.FromModel(res) || json.Unmarshal(res.JSON, &a) != nil || check(res.JSON) != nil {
		return nil, errors.New("barang di pesan ini belum berhasil dibaca Claude")
	}
	m := &replyrule.Match{Items: []replyrule.MatchItem{}, Note: strings.TrimSpace(a.Catatan), Model: res.Model}
	seen := map[string]bool{}
	for _, it := range a.Items {
		sku := known[strings.ToLower(it.SKU)]
		if seen[sku] {
			continue
		}
		seen[sku] = true
		m.Items = append(m.Items, replyrule.MatchItem{SKU: sku, Qty: it.Qty})
	}
	return m, nil
}

// Matched is the stored reading of these messages.
func (s *Service) Matched(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*replyrule.Match, error) {
	rows, err := s.St.Q.ReplyAIMatches(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]*replyrule.Match, len(rows))
	for _, r := range rows {
		m := &replyrule.Match{Items: []replyrule.MatchItem{}, Note: r.Note, Model: r.Model}
		_ = json.Unmarshal(r.Items, &m.Items)
		out[r.MessageID] = m
	}
	return out, nil
}

// match is the message's stored reading, else Claude's now (stored): nil when no Claude or it failed — the card
// then finds the products by words.
func (s *Service) match(ctx context.Context, msg uuid.UUID, thread uuid.UUID, text string, history []replyrule.Line, d replyrule.Data) *replyrule.Match {
	if got, err := s.Matched(ctx, []uuid.UUID{msg}); err == nil && got[msg] != nil {
		return got[msg]
	}
	m, err := s.Find(ctx, text, history, d.Stock)
	if err != nil {
		if s.Log != nil && !errors.Is(err, ErrNoClaude) {
			s.Log.Warn("reply match", "message", msg, "err", err)
		}
		return nil
	}
	items, _ := json.Marshal(m.Items)
	provider := "claude"
	if m.Model == "system" {
		provider = "system"
	}
	if err := s.St.Q.UpsertReplyAIMatch(ctx, gen.UpsertReplyAIMatchParams{MessageID: msg, Items: items, Note: m.Note, Provider: provider, Model: m.Model}); err == nil {
		_ = events.Notify(ctx, s.St.Pool, "chat_message", map[string]string{"thread_id": thread.String()})
	}
	return m
}
