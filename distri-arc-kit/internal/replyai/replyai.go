// Package replyai is the data side of AI Trigger (ADR 0041): it loads the rules and the data they read (stock,
// prices, the conversation's dealer) and asks an AI to write the draft of a card from the facts replyrule computed.
// The AI only phrases: a draft that drops or adds a number is rejected and the template draft stays. Nothing is sent.
package replyai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// Service loads AI Trigger data and writes AI drafts.
type Service struct {
	St    *store.Store
	Views *views.Builder
	Log   *slog.Logger
	// the API & model AI page's key (sealed in secrets) or the env keys
	SessionKey []byte
	EnvClaude  string
	EnvOpenAI  string
	// Env is the LLM_PROVIDER router, used when no key is saved in the app (nil: none)
	Env *llm.Router
	// Provider overrides the model (tests)
	Provider llm.Provider
	// Matcher overrides the Claude model that finds products (tests)
	Matcher llm.Provider

	mu      sync.Mutex
	rules   []replyrule.Rule // cached for Wants
	rulesAt time.Time
}

// Wants reports whether a new customer message matches a rule that asks the AI for its draft (the ingest hook
// enqueues reply.ai only then). The rules are cached for a minute.
func (s *Service) Wants(ctx context.Context, text string) bool {
	s.mu.Lock()
	if s.rules == nil || time.Since(s.rulesAt) > time.Minute {
		rules, err := s.Rules(ctx)
		if err != nil {
			s.mu.Unlock()
			return false
		}
		s.rules, s.rulesAt = rules, time.Now()
	}
	rules := s.rules
	s.mu.Unlock()
	for _, r := range rules {
		if replyrule.Matches(r, text) != "" { // every trigger is written by AI (Sam 21 Oct)
			return true
		}
	}
	return false
}

// ToRule converts a reply_rules row.
func ToRule(x gen.ReplyRule) replyrule.Rule {
	return replyrule.Rule{ID: x.ID.String(), Name: x.Name, Action: x.Action, Keywords: x.Keywords, TemplateReady: x.TemplateReady,
		TemplateEmpty: x.TemplateEmpty, ShowQty: x.ShowQty, AIPhrase: x.AiPhrase, AIInstruction: x.AiInstruction}
}

// Rules are the enabled rules.
func (s *Service) Rules(ctx context.Context) ([]replyrule.Rule, error) {
	rows, err := s.St.Q.EnabledReplyRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]replyrule.Rule, 0, len(rows))
	for _, x := range rows {
		out = append(out, ToRule(x))
	}
	return out, nil
}

// Data loads what the rules matching texts read, for a conversation of branch and dealer (nil: no dealer).
func (s *Service) Data(ctx context.Context, rules []replyrule.Rule, texts []string, branch string, dealerID *uuid.UUID) (replyrule.Data, error) {
	d := replyrule.Data{Branch: branch}
	need := map[string]bool{}
	for _, t := range texts {
		for a := range replyrule.Needs(rules, t) {
			need[a] = true
		}
	}
	if len(need) == 0 {
		return d, nil
	}
	if need[replyrule.ActionStock] {
		st, err := s.St.Q.ListStockItems(ctx)
		if err != nil {
			return d, err
		}
		d.Stock = views.StockItems(st)
	}
	if need[replyrule.ActionPrice] || need[replyrule.ActionStock] { // the stock card shows the price too (Sam 21 Oct)
		ps, err := s.Products(ctx)
		if err != nil {
			return d, err
		}
		d.Products = ps
	}
	if dealerID != nil && (need[replyrule.ActionPrice] || need[replyrule.ActionStock] || need[replyrule.ActionCredit] || need[replyrule.ActionOrder]) && s.Views != nil {
		b, err := s.Views.Metrics(ctx)
		if err != nil {
			return d, err
		}
		if it, ok := b.Get(dealerID.String()); ok {
			h := b.Data.Histories[it.UUID]
			dl := &replyrule.Dealer{Name: it.Name, Tier: it.Tier, Credit: it.Metrics.Credit}
			for _, i := range views.OpenInvoices(h, b.Today) {
				dl.Invoices = append(dl.Invoices, replyrule.Invoice{Number: i.Number, DueAt: i.DueAt, Residual: i.Residual, LateDays: i.LateDays})
			}
			if l := views.DealerOrders(h, it.Metrics, b.Today, 6).Last; l != nil {
				dl.LastOrder = &replyrule.Order{Number: l.Number, Date: l.ConfirmedAt, Phase: l.Phase, Total: l.Total}
			}
			d.Dealer = dl
		}
	}
	return d, nil
}

// Products is the price list the cards read (Odoo / import), list price and tier prices.
func (s *Service) Products(ctx context.Context) ([]replyrule.Product, error) {
	ps, err := s.St.Q.ListProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]replyrule.Product, 0, len(ps))
	for _, p := range ps {
		x := replyrule.Product{Name: p.Name, ListPrice: p.ListPrice, Prices: map[string]int64{}}
		if p.Sku != nil {
			x.SKU = *p.Sku
		}
		_ = json.Unmarshal(p.Prices, &x.Prices)
		out = append(out, x)
	}
	return out, nil
}

// model is the provider AI drafts are written with: the AI Trigger engine (ChatGPT or Claude, ADR 0051) and its
// key, else LLM_PROVIDER.
func (s *Service) model(ctx context.Context) llm.Provider {
	if s.Provider != nil {
		return s.Provider
	}
	if p := s.engineModel(ctx); p != nil {
		return p
	}
	if s.Env.HasModel() {
		return s.Env.Primary
	}
	return nil
}

// HasModel reports whether an AI can write drafts (a key is set).
func (s *Service) HasModel(ctx context.Context) bool { return s.model(ctx) != nil }

// ErrNoModel: no API key — no recommendation can be written (the card shows the facts and says so).
var ErrNoModel = errors.New("AI belum aktif — isi kunci ChatGPT atau Claude di Pengaturan → API & model AI")

// ErrNoDraft: no AI wrote a draft that keeps to the facts (or the message has no card with a draft to rewrite).
var ErrNoDraft = errors.New("ChatGPT dan Claude belum berhasil menulis rekomendasi yang sesuai fakta untuk pesan ini")

// Write has the AI write the drafts of one customer message: the rules asking for it (ruleID nil), or one rule on
// request from the card (ruleID set). Drafts already written from the same facts are kept. Returns how many it wrote.
func (s *Service) Write(ctx context.Context, messageID uuid.UUID, ruleID *string, provider string) (int, error) {
	m, err := s.St.Q.ReplyTriggerMessage(ctx, messageID)
	if err != nil {
		return 0, err
	}
	if m.Direction != "in" || m.Internal || m.DeletedAt != nil || strings.TrimSpace(m.Body) == "" || m.ThreadID == nil {
		return 0, nil
	}
	rules, err := s.Rules(ctx)
	if err != nil {
		return 0, err
	}
	var want []replyrule.Rule
	for _, r := range rules {
		if ruleID == nil || r.ID == *ruleID {
			want = append(want, r)
		}
	}
	if len(want) == 0 {
		return 0, nil
	}
	t, err := s.St.Q.GetThread(ctx, *m.ThreadID)
	if err != nil {
		return 0, err
	}
	data, err := s.Data(ctx, rules, []string{m.Body}, t.ThreadBranch, t.DealerID)
	if err != nil {
		return 0, err
	}
	history, err := s.History(ctx, *m.ThreadID, m.SentAt)
	if err != nil {
		return 0, err
	}
	var found *replyrule.Match
	if replyrule.Needs(rules, m.Body)[replyrule.ActionStock] { // Claude finds the products first (Sam 21 Oct)
		found = s.match(ctx, messageID, *m.ThreadID, m.Body, history, data)
	}
	cards := replyrule.SuggestMatched(rules, m.Body, history, data, found)
	existing, err := s.St.Q.ReplyAIDrafts(ctx, []uuid.UUID{messageID})
	if err != nil {
		return 0, err
	}
	have := map[uuid.UUID]string{}
	for _, x := range existing {
		have[x.RuleID] = x.FactsHash
	}
	var models []llm.Provider
	n, failed := 0, 0
	for _, c := range cards {
		if c.Draft == "" || !wanted(want, c.RuleID) {
			continue
		}
		rid, err := uuid.Parse(c.RuleID)
		if err != nil {
			continue
		}
		hash := FactsHash(c)
		if have[rid] == hash && ruleID == nil {
			continue
		}
		if models == nil {
			if models = s.modelsFor(ctx, provider); len(models) == 0 {
				return n, ErrNoModel
			}
		}
		var (
			draft string
			res   llm.Response
			ok    bool
		)
		for _, model := range models { // the other AI when one fails
			if draft, res, ok = s.phrase(ctx, model, ruleOf(want, c.RuleID), c, m.Body, history, data); ok {
				break
			}
		}
		if !ok {
			failed++
			continue
		}
		if err := s.St.Q.UpsertReplyAIDraft(ctx, gen.UpsertReplyAIDraftParams{MessageID: messageID, RuleID: rid, Draft: draft,
			Provider: res.Provider, Model: res.Model, FactsHash: hash}); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		_ = events.Notify(ctx, s.St.Pool, "chat_message", map[string]string{"thread_id": m.ThreadID.String()})
	}
	if n == 0 && (ruleID != nil || failed > 0) {
		return 0, ErrNoDraft
	}
	return n, nil
}

// History is the conversation before a message (newest first) that the cards and the AI read.
func (s *Service) History(ctx context.Context, thread uuid.UUID, before time.Time) ([]replyrule.Line, error) {
	rows, err := s.St.Q.ReplyTriggerHistory(ctx, gen.ReplyTriggerHistoryParams{ThreadID: thread, Before: before})
	if err != nil {
		return nil, err
	}
	out := make([]replyrule.Line, 0, len(rows))
	for _, x := range rows {
		out = append(out, replyrule.Line{In: x.Direction == "in", Text: llm.MaskCredentials(x.Body)})
	}
	return out, nil
}

// Preview has the AI write the drafts of the cards whose rule asks for it, in place (AI Trigger → Coba kalimat;
// nothing is stored).
func (s *Service) Preview(ctx context.Context, rules []replyrule.Rule, cards []*replyrule.Suggestion, text string, history []replyrule.Line, d replyrule.Data) error {
	var models []llm.Provider
	for _, c := range cards {
		r := ruleOf(rules, c.RuleID)
		if c.Draft == "" {
			continue
		}
		if models == nil {
			if models = s.modelsFor(ctx, ""); len(models) == 0 {
				return ErrNoModel
			}
		}
		for _, model := range models {
			if draft, res, ok := s.phrase(ctx, model, r, c, text, history, d); ok {
				c.Draft, c.AI, c.AIModel = draft, true, res.Model
				break
			}
		}
	}
	return nil
}

func wanted(rules []replyrule.Rule, id string) bool { return ruleOf(rules, id).ID != "" }

func ruleOf(rules []replyrule.Rule, id string) replyrule.Rule {
	for _, r := range rules {
		if r.ID == id {
			return r
		}
	}
	return replyrule.Rule{}
}

// FactsHash identifies the facts a draft is written from: the template draft and its values.
func FactsHash(c *replyrule.Suggestion) string {
	keys := make([]string, 0, len(c.Vars()))
	for k := range c.Vars() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte(c.Draft))
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "\x00%s=%s", k, c.Vars()[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// CardsHash identifies the facts of several cards (a message's cards still without their AI draft).
func CardsHash(cards []*replyrule.Suggestion) string {
	hs := make([]string, 0, len(cards))
	for _, c := range cards {
		hs = append(hs, c.RuleID+":"+FactsHash(c))
	}
	sort.Strings(hs)
	sum := sha256.Sum256([]byte(strings.Join(hs, ",")))
	return hex.EncodeToString(sum[:])[:16]
}

var draftSchema = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"draft"},
	"properties": map[string]any{"draft": map[string]any{"type": "string"}}}

// phrase asks the model for the draft; ok is false when it failed or broke a fact (the template draft stays).
func (s *Service) phrase(ctx context.Context, model llm.Provider, r replyrule.Rule, c *replyrule.Suggestion, text string, history []replyrule.Line, d replyrule.Data) (string, llm.Response, bool) {
	// riwayat: the earlier messages, oldest first, so the AI answers in the conversation's context
	riwayat := make([]map[string]string, 0, len(history))
	for i := len(history) - 1; i >= 0; i-- {
		from := "kami"
		if history[i].In {
			from = "pelanggan"
		}
		riwayat = append(riwayat, map[string]string{"dari": from, "teks": history[i].Text})
	}
	input, _ := json.Marshal(map[string]any{"pesan": text, "riwayat": riwayat, "produk_dari_riwayat": c.FromHistory, "jenis": c.Action, "nilai": c.Vars(),
		"draf": c.Draft, "instruksi": r.AIInstruction})
	var names []string
	if d.Dealer != nil {
		names = append(names, d.Dealer.Name)
	}
	allowed := text + " " + c.Draft
	for _, v := range c.Vars() {
		allowed += " " + v
	}
	check := func(b json.RawMessage) error {
		var o struct{ Draft string }
		if err := json.Unmarshal(b, &o); err != nil {
			return err
		}
		return CheckDraft(c.Draft, allowed, o.Draft)
	}
	router := &llm.Router{Primary: model, St: s.St, Log: s.Log}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	res := router.Complete(ctx, llm.Request{Purpose: "reply.ai", Agent: "AI Trigger", System: llm.Prompt("system") + "\n\n" + llm.Prompt("reply"),
		Input: input, Schema: draftSchema, MaxTokens: 400, Names: names}, check)
	var o struct{ Draft string }
	if !llm.FromModel(res) || json.Unmarshal(res.JSON, &o) != nil || check(res.JSON) != nil {
		if s.Log != nil {
			s.Log.Warn("reply ai: template kept", "rule", r.Name, "provider", res.Provider)
		}
		return "", res, false
	}
	return strings.TrimSpace(o.Draft), res, true
}

var reNum = regexp.MustCompile(`\d[\d.,]*\d|\d`)

func digits(s string) string { return strings.NewReplacer(".", "", ",", "").Replace(s) }

// CheckDraft accepts an AI draft that keeps every number of the template draft and adds none that is not in the
// facts (allowed: the message, the template draft and its values); small counts up to 3 are free ("1 unit").
func CheckDraft(template, allowed, draft string) error {
	draft = strings.TrimSpace(draft)
	if draft == "" || len([]rune(draft)) > 700 {
		return errors.New("draf kosong atau terlalu panjang")
	}
	have := map[string]bool{}
	for _, n := range reNum.FindAllString(draft, -1) {
		have[digits(n)] = true
	}
	for _, n := range reNum.FindAllString(template, -1) {
		if !have[digits(n)] {
			return fmt.Errorf("angka %s hilang dari draf", n)
		}
	}
	ok := map[string]bool{"0": true, "1": true, "2": true, "3": true}
	for _, n := range reNum.FindAllString(allowed, -1) {
		ok[digits(n)] = true
	}
	for n := range have {
		if !ok[n] {
			return fmt.Errorf("angka %s tidak ada di fakta", n)
		}
	}
	return nil
}
