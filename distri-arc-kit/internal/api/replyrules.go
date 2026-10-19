package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/httpx"
	"distri-arc/internal/importer"
	"distri-arc/internal/replyai"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store/gen"
)

// AI Trigger (ADR 0038, ADR 0041): rules managed on their own page; the cards are computed when a chat is read, and
// an AI may write their drafts (the reply.ai job on a new message, or the card's button).

func (s *Server) replyRuleRoutes(r chi.Router) {
	r.Get("/reply-rules", s.listReplyRules)
	r.Post("/reply-rules", s.saveReplyRule)
	r.Put("/reply-rules/{id}", s.saveReplyRule)
	r.Delete("/reply-rules/{id}", s.deleteReplyRule)
	r.Post("/reply-rules/test", s.testReplyRule)
	r.Put("/reply-rules/engine", s.saveReplyEngine)
	r.Post("/chat/threads/{id}/messages/{mid}/ai-draft", s.replyAIDraft)
}

// listReplyRules: the triggers, the {placeholders} of each kind and whether an AI can write drafts.
func (s *Server) listReplyRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListReplyRules(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows), "placeholders": replyrule.Placeholders, "ai_ready": s.replyAI.HasModel(r.Context()),
		"engine": replyai.LoadEngine(r.Context(), s.st.Q), "engine_models": replyai.EngineModels})
}

// saveReplyEngine: PUT /reply-rules/engine {provider, model} — the AI that writes AI Trigger recommendations (ADR 0051).
func (s *Server) saveReplyEngine(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO atau admin yang mengatur AI Trigger")
		return
	}
	var e replyai.Engine
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&e); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	if err := replyai.SaveEngine(r.Context(), s.st.Q, e, s.clock.Now()); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	s.auditUser(r, "reply_ai.engine", "policy:"+replyai.PolicyKey, map[string]any{"provider": e.Provider, "model": e.Model})
	s.listReplyRules(w, r)
}

type replyRuleIn struct {
	Name          string   `json:"name"`
	Action        string   `json:"action"`
	Keywords      []string `json:"keywords"`
	TemplateReady string   `json:"template_ready"`
	TemplateEmpty string   `json:"template_empty"`
	ShowQty       bool     `json:"show_qty"`
	Enabled       bool     `json:"enabled"`
	AIPhrase      bool     `json:"ai_phrase"`
	AIInstruction string   `json:"ai_instruction"`
}

func (s *Server) saveReplyRule(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO atau admin yang mengatur AI Trigger")
		return
	}
	var in replyRuleIn
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.AIInstruction = strings.TrimSpace(in.AIInstruction)
	var kws []string
	seen := map[string]bool{}
	for _, k := range in.Keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" && !seen[k] {
			seen[k] = true
			kws = append(kws, k)
		}
	}
	if in.Action == "" {
		in.Action = replyrule.ActionStock
	}
	switch {
	case in.Name == "" || len(kws) == 0:
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama trigger dan minimal satu kata kunci")
		return
	case !replyrule.ValidAction(in.Action):
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Jenis trigger yang tersedia: stok, harga, limit & tagihan, status order")
		return
	case strings.TrimSpace(in.TemplateReady) == "" || strings.TrimSpace(in.TemplateEmpty) == "":
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi kedua draf jawaban")
		return
	case len([]rune(in.AIInstruction)) > 500:
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Instruksi AI maksimal 500 karakter")
		return
	}
	var row gen.ReplyRule
	var err error
	if id := chi.URLParam(r, "id"); id != "" {
		rid, perr := uuid.Parse(id)
		if perr != nil {
			httpx.Fail(w, http.StatusNotFound, "not_found", "Trigger tidak ditemukan")
			return
		}
		row, err = s.st.Q.UpdateReplyRule(r.Context(), gen.UpdateReplyRuleParams{ID: rid, Name: in.Name, Action: in.Action, Keywords: kws, TemplateReady: in.TemplateReady,
			TemplateEmpty: in.TemplateEmpty, ShowQty: in.ShowQty, Enabled: in.Enabled, UpdatedBy: u.Email, AiPhrase: in.AIPhrase, AiInstruction: in.AIInstruction})
	} else {
		row, err = s.st.Q.InsertReplyRule(r.Context(), gen.InsertReplyRuleParams{Name: in.Name, Action: in.Action, Keywords: kws, TemplateReady: in.TemplateReady,
			TemplateEmpty: in.TemplateEmpty, ShowQty: in.ShowQty, Enabled: in.Enabled, UpdatedBy: u.Email, AiPhrase: in.AIPhrase, AiInstruction: in.AIInstruction})
	}
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Trigger tidak ditemukan")
		return
	}
	s.audit(r, "reply_rule.save", "reply_rule", &row.ID, map[string]any{"name": row.Name, "action": row.Action, "keywords": row.Keywords,
		"enabled": row.Enabled, "ai_phrase": row.AiPhrase})
	httpx.JSON(w, http.StatusOK, row)
}

func (s *Server) deleteReplyRule(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if !isAdmin(u) || err != nil {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO atau admin yang mengatur AI Trigger")
		return
	}
	if n, err := s.st.Q.DeleteReplyRule(r.Context(), id); err != nil || n == 0 {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Trigger tidak ditemukan")
		return
	}
	s.audit(r, "reply_rule.delete", "reply_rule", &id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// testReplyRule tries a sentence against the enabled rules (AI Trigger → Coba kalimat), for a branch and a dealer
// (slug or id, optional); with ai the AI writes the drafts of the rules that ask for it (nothing is stored).
func (s *Server) testReplyRule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text   string   `json:"text"`
		Branch string   `json:"branch"`
		Dealer string   `json:"dealer"`
		AI     bool     `json:"ai"`
		Before []string `json:"history"` // earlier messages of the customer, oldest first
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	ctx := r.Context()
	rules, err := s.replyAI.Rules(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var dealer *uuid.UUID
	dealerName := ""
	if d := strings.TrimSpace(in.Dealer); d != "" {
		b, err := s.views.Metrics(ctx)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		it, ok := b.Get(d)
		if !ok {
			httpx.Fail(w, http.StatusNotFound, "not_found", "Dealer tidak ditemukan")
			return
		}
		dealer, dealerName = &it.UUID, it.Name
		if in.Branch == "" {
			in.Branch = it.Branch
		}
	}
	data, err := s.replyAI.Data(ctx, rules, []string{in.Text}, in.Branch, dealer)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var history []replyrule.Line
	for i := len(in.Before) - 1; i >= 0 && len(history) < replyrule.HistoryLines; i-- {
		if t := strings.TrimSpace(in.Before[i]); t != "" {
			history = append(history, replyrule.Line{In: true, Text: t})
		}
	}
	cards := replyrule.Suggest(rules, in.Text, history, data)
	aiErr := ""
	if in.AI {
		if err := s.replyAI.Preview(ctx, rules, cards, in.Text, history, data); err != nil {
			aiErr = err.Error()
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"suggestions": nonNil(cards), "dealer": dealerName, "branch": in.Branch, "ai_error": aiErr})
}

// replyAIDraft has the AI write the draft of one card now (the card's "Tulis dengan AI").
func (s *Server) replyAIDraft(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "mid"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	if _, err := s.st.Q.GetThreadMessage(r.Context(), gen.GetThreadMessageParams{ThreadID: t.ID, ID: mid}); err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pesan tidak ditemukan")
		return
	}
	var in struct {
		RuleID string `json:"rule_id"`
		// Provider: the AI the sales picks on the card — openai (ChatGPT) | claude; "" the AI Trigger engine (ADR 0051)
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || in.RuleID == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	if in.Provider != "" && in.Provider != "openai" && in.Provider != "claude" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "AI tidak dikenal")
		return
	}
	n, err := s.replyAI.Write(r.Context(), mid, &in.RuleID, in.Provider)
	switch {
	case errors.Is(err, replyai.ErrNoModel):
		msg := err.Error()
		if in.Provider != "" {
			msg = "Kunci " + map[string]string{"openai": "ChatGPT", "claude": "Claude"}[in.Provider] + " belum diisi di Pengaturan → API & model AI"
		}
		httpx.Fail(w, http.StatusConflict, "no_model", msg)
		return
	case errors.Is(err, replyai.ErrNoDraft):
		httpx.Fail(w, http.StatusConflict, "no_draft", "AI tidak berhasil menulis draf yang sesuai fakta — draf template tetap dipakai")
		return
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"written": n})
}

// suggestWindow: only recent customer messages get a suggestion (an old question is already settled one way or another).
const suggestWindow = 7 * 24 * time.Hour

// aiPendingFor: a card whose rule asks the AI shows "AI sedang menulis" this long after the message came in.
const aiPendingFor = 2 * time.Minute

// attachSuggestions adds the AI Trigger cards to the recent customer messages of a conversation, with the AI's drafts.
func (s *Server) attachSuggestions(ctx context.Context, msgs []MessageView, branch string, dealerID *uuid.UUID, now time.Time) error {
	var recent []int
	for i, m := range msgs {
		if m.Direction == "in" && !m.Internal && !m.Deleted && !m.AutoReply && now.Sub(m.SentAt) <= suggestWindow {
			recent = append(recent, i)
		}
	}
	if len(recent) == 0 {
		return nil
	}
	rules, err := s.replyAI.Rules(ctx)
	if err != nil || len(rules) == 0 {
		return err
	}
	recent = recent[max(0, len(recent)-30):]
	texts := make([]string, len(recent))
	for j, i := range recent {
		texts[j] = msgs[i].Body
	}
	data, err := s.replyAI.Data(ctx, rules, texts, branch, dealerID)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for _, i := range recent {
		msgs[i].Suggest = replyrule.Suggest(rules, msgs[i].Body, historyOf(msgs, i), data)
		if len(msgs[i].Suggest) > 0 {
			ids = append(ids, msgs[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	drafts, err := s.st.Q.ReplyAIDrafts(ctx, ids)
	if err != nil {
		return err
	}
	type key struct {
		msg  uuid.UUID
		rule string
	}
	byKey := map[key]gen.ReplyAiDraft{}
	for _, d := range drafts {
		byKey[key{d.MessageID, d.RuleID.String()}] = d
	}
	var model *bool
	for _, i := range recent {
		for _, c := range msgs[i].Suggest {
			if d, ok := byKey[key{msgs[i].ID, c.RuleID}]; ok && c.Draft != "" && d.FactsHash == replyai.FactsHash(c) {
				c.Draft, c.AI, c.AIModel = d.Draft, true, d.Model
				continue
			}
			if c.AIPhrase && c.Draft != "" && now.Sub(msgs[i].SentAt) < aiPendingFor {
				if model == nil {
					has := s.replyAI.HasModel(ctx)
					model = &has
				}
				c.AIPending = *model
			}
		}
	}
	return nil
}

// historyOf is the conversation before msgs[i] on this page, newest first: up to HistoryLines messages of the three
// days before it (as ReplyTriggerHistory, which the AI job reads).
func historyOf(msgs []MessageView, i int) []replyrule.Line {
	var out []replyrule.Line
	for j := i - 1; j >= 0 && len(out) < replyrule.HistoryLines; j-- {
		m := msgs[j]
		if msgs[i].SentAt.Sub(m.SentAt) > 72*time.Hour {
			break
		}
		if m.Deleted || strings.TrimSpace(m.Body) == "" {
			continue
		}
		out = append(out, replyrule.Line{In: m.Direction == "in", Text: m.Body})
	}
	return out
}

// stockAsOf is when the stock behind the suggestions was synced (nil without stock cards or a sync).
func stockAsOf(ctx context.Context, s *Server, msgs []MessageView) *time.Time {
	for _, m := range msgs {
		for _, c := range m.Suggest {
			if c.Action == replyrule.ActionStock || c.Action == replyrule.ActionPrice {
				return importer.ProvenanceOf(ctx, s.st).SyncedAt
			}
		}
	}
	return nil
}
