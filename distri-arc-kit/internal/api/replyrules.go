package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/domain"
	"distri-arc/internal/httpx"
	"distri-arc/internal/importer"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// Rekomendasi balasan (ADR 0038): rules managed in Pengaturan; the suggestion is computed when a chat is read.

func (s *Server) replyRuleRoutes(r chi.Router) {
	r.Get("/reply-rules", s.listReplyRules)
	r.Post("/reply-rules", s.saveReplyRule)
	r.Put("/reply-rules/{id}", s.saveReplyRule)
	r.Delete("/reply-rules/{id}", s.deleteReplyRule)
	r.Post("/reply-rules/test", s.testReplyRule)
}

func (s *Server) listReplyRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListReplyRules(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

type replyRuleIn struct {
	Name          string   `json:"name"`
	Action        string   `json:"action"`
	Keywords      []string `json:"keywords"`
	TemplateReady string   `json:"template_ready"`
	TemplateEmpty string   `json:"template_empty"`
	ShowQty       bool     `json:"show_qty"`
	Enabled       bool     `json:"enabled"`
}

func (s *Server) saveReplyRule(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO atau admin yang mengatur rekomendasi balasan")
		return
	}
	var in replyRuleIn
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
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
		in.Action = "stock"
	}
	switch {
	case in.Name == "" || len(kws) == 0:
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama aturan dan minimal satu kata kunci")
		return
	case in.Action != "stock":
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Jenis aturan yang tersedia: stok")
		return
	case strings.TrimSpace(in.TemplateReady) == "" || strings.TrimSpace(in.TemplateEmpty) == "":
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi draf jawaban untuk stok ready dan stok kosong")
		return
	}
	var row gen.ReplyRule
	var err error
	if id := chi.URLParam(r, "id"); id != "" {
		rid, perr := uuid.Parse(id)
		if perr != nil {
			httpx.Fail(w, http.StatusNotFound, "not_found", "Aturan tidak ditemukan")
			return
		}
		row, err = s.st.Q.UpdateReplyRule(r.Context(), gen.UpdateReplyRuleParams{ID: rid, Name: in.Name, Keywords: kws, TemplateReady: in.TemplateReady,
			TemplateEmpty: in.TemplateEmpty, ShowQty: in.ShowQty, Enabled: in.Enabled, UpdatedBy: u.Email})
	} else {
		row, err = s.st.Q.InsertReplyRule(r.Context(), gen.InsertReplyRuleParams{Name: in.Name, Action: in.Action, Keywords: kws, TemplateReady: in.TemplateReady,
			TemplateEmpty: in.TemplateEmpty, ShowQty: in.ShowQty, Enabled: in.Enabled, UpdatedBy: u.Email})
	}
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Aturan tidak ditemukan")
		return
	}
	s.audit(r, "reply_rule.save", "reply_rule", &row.ID, map[string]any{"name": row.Name, "keywords": row.Keywords, "enabled": row.Enabled})
	httpx.JSON(w, http.StatusOK, row)
}

func (s *Server) deleteReplyRule(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if !isAdmin(u) || err != nil {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO atau admin yang mengatur rekomendasi balasan")
		return
	}
	if n, err := s.st.Q.DeleteReplyRule(r.Context(), id); err != nil || n == 0 {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Aturan tidak ditemukan")
		return
	}
	s.audit(r, "reply_rule.delete", "reply_rule", &id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// testReplyRule tries a sentence against the enabled rules (Pengaturan → Coba kalimat).
func (s *Server) testReplyRule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text   string `json:"text"`
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	rules, stock, err := s.replyInputs(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"suggestion": replyrule.Suggest(rules, in.Text, in.Branch, stock)})
}

// replyInputs loads the enabled rules and the current stock.
func (s *Server) replyInputs(ctx context.Context) ([]replyrule.Rule, []domain.StockItem, error) {
	rows, err := s.st.Q.EnabledReplyRules(ctx)
	if err != nil {
		return nil, nil, err
	}
	rules := make([]replyrule.Rule, 0, len(rows))
	for _, x := range rows {
		rules = append(rules, replyrule.Rule{ID: x.ID.String(), Name: x.Name, Action: x.Action, Keywords: x.Keywords,
			TemplateReady: x.TemplateReady, TemplateEmpty: x.TemplateEmpty, ShowQty: x.ShowQty})
	}
	if len(rules) == 0 {
		return nil, nil, nil
	}
	st, err := s.st.Q.ListStockItems(ctx)
	if err != nil {
		return nil, nil, err
	}
	return rules, views.StockItems(st), nil
}

// suggestWindow: only recent customer messages get a suggestion (an old question is already settled one way or another).
const suggestWindow = 7 * 24 * time.Hour

// attachSuggestions adds Rekomendasi balasan to the recent customer messages of a conversation.
func (s *Server) attachSuggestions(ctx context.Context, msgs []MessageView, branch string, now time.Time) error {
	var recent []int
	for i, m := range msgs {
		if m.Direction == "in" && !m.Internal && !m.Deleted && !m.AutoReply && now.Sub(m.SentAt) <= suggestWindow {
			recent = append(recent, i)
		}
	}
	if len(recent) == 0 {
		return nil
	}
	rules, stock, err := s.replyInputs(ctx)
	if err != nil || len(rules) == 0 {
		return err
	}
	for _, i := range recent[max(0, len(recent)-30):] {
		msgs[i].Suggest = replyrule.Suggest(rules, msgs[i].Body, branch, stock)
	}
	return nil
}

// stockAsOf is when the stock behind the suggestions was synced (nil without suggestions or a sync).
func stockAsOf(ctx context.Context, s *Server, msgs []MessageView) *time.Time {
	for _, m := range msgs {
		if m.Suggest != nil {
			return importer.ProvenanceOf(ctx, s.st).SyncedAt
		}
	}
	return nil
}
