package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/analyst"
	"distri-arc/internal/auth"
	"distri-arc/internal/clock"
	"distri-arc/internal/httpx"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
)

// Pengaturan → API & model AI (ADR 0040): the Claude API and ChatGPT API keys and models, which of them is the second
// option when Claude through MCP has not answered, and what the API tokens cost.

// WithOpenAIVerify replaces the ChatGPT key check (tests).
func (s *Server) WithOpenAIVerify(verify func(ctx context.Context, key string) error) *Server {
	s.verifyOpenAI = verify
	return s
}

func (s *Server) aiAPIRoutes(r chi.Router) {
	r.Get("/ai/api", s.aiAPIInfo)
	r.Put("/ai/api", s.aiAPIConfig)
	r.Put("/ai/api/wa-auto", s.aiAPIWAAuto)
	r.Put("/ai/api/key/{provider}", s.aiAPIKey)
	r.Delete("/ai/api/key/{provider}", s.aiAPIKeyDelete)
	r.Get("/ai/routine", s.aiRoutine)
	r.Put("/ai/routine", s.aiRoutineSave)
	r.Delete("/ai/routine", s.aiRoutineDelete)
	r.Post("/ai/routine/test", s.aiRoutineTest)
}

type aiProvider struct {
	ID        string               `json:"id"` // claude | openai
	Label     string               `json:"label"`
	KeySource string               `json:"key_source"` // ui | env | ""
	KeyHint   string               `json:"key_hint"`
	Model     string               `json:"model"`
	Models    []analyst.ModelEntry `json:"models"`
}

type aiUsageDay struct {
	Day       string `json:"day"`
	Provider  string `json:"provider"`
	Calls     int64  `json:"calls"`
	TokensIn  int64  `json:"tokens_in"`
	TokensOut int64  `json:"tokens_out"`
	CostIDR   int64  `json:"cost_idr"`
}

func (s *Server) aiAPIInfo(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !can(u, "conn") && !can(u, "mcp") {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "API & model AI untuk pemegang menu Pengaturan atau MCP Claude")
		return
	}
	ctx := r.Context()
	now := s.clock.Now()
	today := clock.Today(now)
	cfg := analyst.LoadConfig(ctx, s.st.Q)
	ck, csrc := analyst.LoadKey(ctx, s.st.Q, s.secret(), s.cfg.AnthropicKey)
	ok, okSrc := analyst.LoadOpenAIKeyAITrigger(ctx, s.st.Q, s.secret(), s.cfg.OpenAIKey) // shown for AI Trigger, its only user (ADR 0051)
	hint := func(k string) string {
		if k == "" {
			return ""
		}
		return analyst.KeyHint(k)
	}
	providers := []aiProvider{
		{ID: analyst.FallbackClaude, Label: "Claude API", KeySource: csrc, KeyHint: hint(ck), Model: cfg.Model, Models: analyst.ModelList(ctx, analyst.FallbackClaude, ck)},
		{ID: analyst.FallbackOpenAI, Label: "ChatGPT API", KeySource: okSrc, KeyHint: hint(ok), Model: cfg.OpenAIModel, Models: analyst.ModelList(ctx, analyst.FallbackOpenAI, ok)},
	}
	ready, _, _ := analyst.FallbackReady(ctx, s.st.Q, now, s.secret(), s.cfg.AnthropicKey, s.cfg.OpenAIKey)
	spent, _ := s.st.Q.AnalystCostSince(ctx, today)

	fail := func(err error) { httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error()) }
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
	totals := map[string]gen.LLMUsageSinceRow{}
	for name, since := range map[string]time.Time{"today": today, "d7": today.AddDate(0, 0, -6), "d30": today.AddDate(0, 0, -29), "month": month} {
		v, err := s.st.Q.LLMUsageSince(ctx, since)
		if err != nil {
			fail(err)
			return
		}
		totals[name] = v
	}
	byModel, err := s.st.Q.LLMUsageByModel(ctx, today.AddDate(0, 0, -29))
	if err != nil {
		fail(err)
		return
	}
	models := make([]gen.LLMUsageByModelRow, 0, len(byModel))
	for _, m := range byModel {
		if m.Provider != "fake" && m.Provider != "template" && m.Provider != "" {
			models = append(models, m)
		}
	}
	rows, err := s.st.Q.LLMUsageDailyByProvider(ctx, today.AddDate(0, 0, -29))
	if err != nil {
		fail(err)
		return
	}
	daily := make([]aiUsageDay, 0, len(rows))
	for _, d := range rows {
		daily = append(daily, aiUsageDay{Day: d.Day.Format("2006-01-02"), Provider: d.Provider, Calls: d.Calls, TokensIn: d.TokensIn, TokensOut: d.TokensOut, CostIDR: d.CostIdr})
	}
	runs, err := s.st.Q.FallbackRuns(ctx, []string{analyst.FallbackScheduleCycle, analyst.FallbackScheduleWA})
	if err != nil {
		fail(err)
		return
	}
	auto, _ := analyst.LoadWAAuto(ctx, s.st.Q)
	autoRuns, err := s.st.Q.FallbackRuns(ctx, []string{analyst.WAAutoSchedule})
	if err != nil {
		fail(err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"wa_auto": auto, "wa_auto_runs": autoRuns,
		"fallback": cfg.Fallback, "daily_budget_idr": cfg.DailyBudgetIDR, "spent_today_idr": spent.Cost, "ready": ready,
		"providers": providers, "mcp_wait_sec": int(s.mcpWait().Seconds()), "fallback_window_sec": int((10 * time.Minute).Seconds()),
		"can_configure": deref(u.Role) == "ceo",
		"usage":         map[string]any{"totals": totals, "by_model": models, "daily": daily},
		"runs":          runs, "idr_per_usd": llm.IDRPerUSD,
	})
}

func (s *Server) aiAPIConfig(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var body struct {
		Fallback       *string `json:"fallback"`
		Model          *string `json:"model"`
		OpenAIModel    *string `json:"openai_model"`
		DailyBudgetIDR *int64  `json:"daily_budget_idr"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	c := analyst.LoadConfig(r.Context(), s.st.Q)
	if body.Fallback != nil {
		c.Fallback = *body.Fallback
	}
	if body.Model != nil {
		c.Model = *body.Model
	}
	if body.OpenAIModel != nil {
		c.OpenAIModel = *body.OpenAIModel
	}
	if body.DailyBudgetIDR != nil {
		c.DailyBudgetIDR = *body.DailyBudgetIDR
	}
	if err := analyst.SaveConfig(r.Context(), s.st.Q, c, nil, s.clock.Now()); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	s.auditUser(r, "ai_api.config", "policy:"+analyst.PolicyKey, map[string]any{"fallback": c.Fallback, "model": c.Model,
		"openai_model": c.OpenAIModel, "daily_budget_idr": c.DailyBudgetIDR, "by": deref(u.Email)})
	s.aiAPIInfo(w, r)
}

// aiAPIWAAuto saves Analisa WhatsApp otomatis: on/off, the hours and the API + model it runs with (ADR 0043).
func (s *Server) aiAPIWAAuto(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var a analyst.WAAuto
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&a); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	if err := analyst.SaveWAAuto(r.Context(), s.st, a, nil, s.clock.Now()); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	s.auditUser(r, "ai_api.wa_auto", "policy:"+analyst.AutoPolicyKey, map[string]any{"enabled": a.Enabled, "hours": a.Hours,
		"weekend": a.Weekend, "engine": a.Engine, "model": a.Model, "by": deref(u.Email)})
	s.aiAPIInfo(w, r)
}

func (s *Server) aiAPIKey(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	prov := chi.URLParam(r, "provider")
	if prov != analyst.FallbackClaude && prov != analyst.FallbackOpenAI {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Provider tidak dikenal")
		return
	}
	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	key := strings.TrimSpace(body.APIKey)
	secretKey, verify, rejected := analyst.SecretKey, s.verifyKey, analyst.ErrKeyRejected
	if verify == nil {
		verify = analyst.VerifyKey
	}
	if prov == analyst.FallbackOpenAI {
		if !strings.HasPrefix(key, "sk-") || strings.HasPrefix(key, "sk-ant-") || len(key) < 24 || strings.ContainsAny(key, " \n\t") {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Kunci ChatGPT API diawali sk- (buat di platform.openai.com → API keys)")
			return
		}
		secretKey, verify, rejected = analyst.SecretOpenAI, s.verifyOpenAI, analyst.ErrOpenAIKeyRejected
		if verify == nil {
			verify = analyst.VerifyOpenAIKey
		}
	} else if !strings.HasPrefix(key, "sk-ant-") || len(key) < 24 || strings.ContainsAny(key, " \n\t") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Kunci Claude API diawali sk-ant- (buat di console.anthropic.com → API keys)")
		return
	}
	label := analyst.ProviderLabel(prov)
	note := "Kunci " + label + " diperiksa dan disimpan terenkripsi"
	if err := verify(r.Context(), key); errors.Is(err, rejected) {
		httpx.Fail(w, http.StatusBadRequest, "key_rejected", err.Error())
		return
	} else if err != nil {
		note = "Kunci " + label + " disimpan terenkripsi; providernya belum bisa dihubungi untuk memeriksanya (" + err.Error() + ")"
	}
	sealed, err := auth.Seal(key, s.secret())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.st.Q.SetSecret(r.Context(), gen.SetSecretParams{Key: secretKey, Value: sealed, UpdatedBy: u.Email}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "ai_api.key_set", "secret:"+secretKey, map[string]any{"hint": analyst.KeyHint(key)})
	httpx.JSON(w, http.StatusOK, map[string]any{"message": note, "key_hint": analyst.KeyHint(key)})
}

func (s *Server) aiAPIKeyDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ceoOnly(w, r); !ok {
		return
	}
	secretKey := analyst.SecretKey
	switch chi.URLParam(r, "provider") {
	case analyst.FallbackClaude:
	case analyst.FallbackOpenAI:
		secretKey = analyst.SecretOpenAI
	default:
		httpx.Fail(w, http.StatusNotFound, "not_found", "Provider tidak dikenal")
		return
	}
	_ = s.st.Q.DeleteSecret(r.Context(), secretKey)
	s.auditUser(r, "ai_api.key_deleted", "secret:"+secretKey, nil)
	w.WriteHeader(http.StatusNoContent)
}
