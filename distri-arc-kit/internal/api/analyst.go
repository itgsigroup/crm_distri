package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"distri-arc/internal/analyst"
	"distri-arc/internal/auth"
	"distri-arc/internal/clock"
	"distri-arc/internal/cron"
	"distri-arc/internal/httpx"
	"distri-arc/internal/jobs"
	"distri-arc/internal/mcp"
	"distri-arc/internal/store/gen"
)

// WithAnalystModel replaces the Claude client of inline runs and the key check (tests).
func (s *Server) WithAnalystModel(m func(key string) analyst.Model, verify func(ctx context.Context, key string) error) *Server {
	s.analystModel, s.verifyKey = m, verify
	return s
}

func (s *Server) analystRoutes(r chi.Router) {
	r.Get("/mcp/analyst", s.analystInfo)
	r.Put("/mcp/analyst", s.analystConfig)
	r.Put("/mcp/analyst/key", s.analystKey)
	r.Delete("/mcp/analyst/key", s.analystKeyDelete)
	r.Get("/mcp/cron", s.cronPreview)
	r.Get("/mcp/schedules", s.listSchedules)
	r.Post("/mcp/schedules", s.saveSchedule)
	r.Put("/mcp/schedules/{id}", s.saveSchedule)
	r.Delete("/mcp/schedules/{id}", s.deleteSchedule)
	r.Post("/mcp/schedules/{id}/run", s.runSchedule)
	r.Get("/mcp/schedules/{id}/runs", s.scheduleRuns)
	r.Get("/mcp/runs/{id}", s.scheduleRun)
}

func (s *Server) analystInfo(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	cfg := analyst.LoadConfig(r.Context(), s.st.Q)
	key, src := analyst.LoadKey(r.Context(), s.st.Q, s.secret(), s.cfg.AnthropicKey)
	spent, _ := s.st.Q.AnalystCostSince(r.Context(), clock.Today(s.clock.Now()))
	engine, hint := "template", ""
	if key != "" {
		engine, hint = "claude", analyst.KeyHint(key)
	}
	_, why := mcpGrantable(u)
	httpx.JSON(w, http.StatusOK, map[string]any{"engine": engine, "key_source": src, "key_hint": hint, "model": cfg.Model, "models": analyst.Models,
		"daily_budget_idr": cfg.DailyBudgetIDR, "spent_today_idr": spent.Cost, "runs_today": spent.Runs,
		"can_configure": deref(u.Role) == "ceo", "can_schedule": why == "", "min_gap_minutes": int(cron.MinGap.Minutes())})
}

func (s *Server) analystConfig(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var c analyst.Config
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&c); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	if err := analyst.SaveConfig(r.Context(), s.st.Q, c, nil, s.clock.Now()); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	s.auditUser(r, "analyst.config", "policy:"+analyst.PolicyKey, map[string]any{"model": c.Model, "daily_budget_idr": c.DailyBudgetIDR, "by": deref(u.Email)})
	s.analystInfo(w, r)
}

func (s *Server) analystKey(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
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
	if !strings.HasPrefix(key, "sk-ant-") || len(key) < 24 || strings.ContainsAny(key, " \n\t") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Kunci Claude API diawali sk-ant- (buat di console.anthropic.com → API keys)")
		return
	}
	verify := s.verifyKey
	if verify == nil {
		verify = analyst.VerifyKey
	}
	note := "Kunci diperiksa ke Anthropic dan disimpan terenkripsi"
	if err := verify(r.Context(), key); errors.Is(err, analyst.ErrKeyRejected) {
		httpx.Fail(w, http.StatusBadRequest, "key_rejected", err.Error())
		return
	} else if err != nil {
		note = "Kunci disimpan terenkripsi; Anthropic belum bisa dihubungi untuk memeriksanya (" + err.Error() + ")"
	}
	sealed, err := auth.Seal(key, s.secret())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.st.Q.SetSecret(r.Context(), gen.SetSecretParams{Key: analyst.SecretKey, Value: sealed, UpdatedBy: u.Email}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "analyst.key_set", "secret", map[string]any{"hint": analyst.KeyHint(key)})
	httpx.JSON(w, http.StatusOK, map[string]any{"message": note, "key_hint": analyst.KeyHint(key)})
}

func (s *Server) analystKeyDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ceoOnly(w, r); !ok {
		return
	}
	_ = s.st.Q.DeleteSecret(r.Context(), analyst.SecretKey)
	s.auditUser(r, "analyst.key_deleted", "secret", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cronPreview(w http.ResponseWriter, r *http.Request) {
	expr := r.URL.Query().Get("expr")
	spec, err := cron.Parse(expr)
	if err == nil {
		err = spec.CheckGap(s.clock.Now())
	}
	if err != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "expr": spec.String(), "description": cron.Describe(expr), "next": spec.NextN(s.clock.Now(), 5)})
}

type scheduleView struct {
	ID          uuid.UUID   `json:"id"`
	Name        string      `json:"name"`
	Prompt      string      `json:"prompt"`
	Cron        string      `json:"cron"`
	Description string      `json:"description"`
	Enabled     bool        `json:"enabled"`
	Scopes      []string    `json:"scopes"`
	MaxSteps    int32       `json:"max_steps"`
	NextRuns    []time.Time `json:"next_runs"`
	LastRunAt   *time.Time  `json:"last_run_at"`
	LastRun     any         `json:"last_run"`
	CreatedAt   time.Time   `json:"created_at"`
}

func (s *Server) scheduleView(sc gen.McpSchedule, last map[uuid.UUID]gen.LastMCPScheduleRunsRow) scheduleView {
	v := scheduleView{ID: sc.ID, Name: sc.Name, Prompt: sc.Prompt, Cron: sc.Cron, Description: cron.Describe(sc.Cron), Enabled: sc.Enabled,
		Scopes: sc.Scopes, MaxSteps: sc.MaxSteps, NextRuns: []time.Time{}, LastRunAt: sc.LastRunAt, CreatedAt: sc.CreatedAt}
	if spec, err := cron.Parse(sc.Cron); err == nil && sc.Enabled {
		v.NextRuns = spec.NextN(s.clock.Now(), 3)
	}
	if l, ok := last[sc.ID]; ok {
		v.LastRun = l
	}
	return v
}

func (s *Server) listSchedules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListMCPSchedules(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	lastRows, _ := s.st.Q.LastMCPScheduleRuns(r.Context())
	last := map[uuid.UUID]gen.LastMCPScheduleRunsRow{}
	for _, l := range lastRows {
		last[l.ScheduleID] = l
	}
	out := make([]scheduleView, 0, len(rows))
	for _, sc := range rows {
		out = append(out, s.scheduleView(sc, last))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

// scheduler is the user editing schedules: MCP Claude screen with all-data scope (like connecting Claude).
func (s *Server) scheduler(w http.ResponseWriter, r *http.Request) (User, []string, bool) {
	u, _ := CurrentUser(r.Context())
	scopes, why := mcpGrantable(u)
	if why != "" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", why)
		return u, nil, false
	}
	return u, scopes, true
}

func (s *Server) saveSchedule(w http.ResponseWriter, r *http.Request) {
	u, allowed, ok := s.scheduler(w, r)
	if !ok {
		return
	}
	var in struct {
		Name     string   `json:"name"`
		Prompt   string   `json:"prompt"`
		Cron     string   `json:"cron"`
		Enabled  *bool    `json:"enabled"`
		Scopes   []string `json:"scopes"`
		MaxSteps int32    `json:"max_steps"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	in.Name, in.Prompt = strings.TrimSpace(in.Name), strings.TrimSpace(in.Prompt)
	spec, err := cron.Parse(in.Cron)
	if err == nil {
		err = spec.CheckGap(s.clock.Now())
	}
	switch {
	case in.Name == "" || len(in.Name) > 80:
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nama jadwal wajib (maks 80 karakter)")
		return
	case len(in.Prompt) < 10 || len(in.Prompt) > 4000:
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Prompt analisis 10–4000 karakter")
		return
	case err != nil:
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Jadwal: "+err.Error())
		return
	}
	if in.MaxSteps == 0 {
		in.MaxSteps = 12
	}
	if in.MaxSteps < 2 || in.MaxSteps > 30 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Batas langkah 2–30 panggilan tool")
		return
	}
	scopes := []string{mcp.ScopeRead}
	for _, sc := range in.Scopes {
		if sc == mcp.ScopeRead || slices.Contains(scopes, sc) {
			continue
		}
		if !slices.Contains(allowed, sc) {
			httpx.Fail(w, http.StatusForbidden, "forbidden", "Izin "+sc+" tidak bisa Anda berikan ke jadwal")
			return
		}
		scopes = append(scopes, sc)
	}
	enabled := in.Enabled == nil || *in.Enabled
	next := analyst.NextRun(spec.String(), s.clock.Now())
	var sc gen.McpSchedule
	if idStr := chi.URLParam(r, "id"); idStr != "" {
		id, perr := uuid.Parse(idStr)
		if perr != nil {
			httpx.Fail(w, http.StatusNotFound, "not_found", "Jadwal tidak ditemukan")
			return
		}
		sc, err = s.st.Q.UpdateMCPSchedule(r.Context(), gen.UpdateMCPScheduleParams{ID: id, Name: in.Name, Prompt: in.Prompt, Cron: spec.String(),
			Enabled: enabled, Scopes: scopes, MaxSteps: in.MaxSteps, NextRunAt: next})
	} else {
		sc, err = s.st.Q.InsertMCPSchedule(r.Context(), gen.InsertMCPScheduleParams{Name: in.Name, Prompt: in.Prompt, Cron: spec.String(),
			Enabled: enabled, Scopes: scopes, MaxSteps: in.MaxSteps, CreatedBy: &u.ID, NextRunAt: next})
	}
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Jadwal tidak ditemukan")
		return
	}
	s.auditUser(r, "analyst.schedule_saved", "mcp_schedule:"+sc.ID.String(), map[string]any{"name": sc.Name, "cron": sc.Cron, "enabled": sc.Enabled, "scopes": sc.Scopes})
	httpx.JSON(w, http.StatusOK, s.scheduleView(sc, nil))
}

func (s *Server) scheduleID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Tidak ditemukan")
		return id, false
	}
	return id, true
}

func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.scheduler(w, r); !ok {
		return
	}
	id, ok := s.scheduleID(w, r)
	if !ok {
		return
	}
	client, err := s.st.Q.DeleteMCPSchedule(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Jadwal tidak ditemukan")
		return
	}
	if client != nil {
		_ = s.st.Q.RevokeMCPClient(r.Context(), *client)
	}
	s.auditUser(r, "analyst.schedule_deleted", "mcp_schedule:"+id.String(), nil)
	w.WriteHeader(http.StatusNoContent)
}

// runSchedule is "Jalankan sekarang": queued for the worker, or run here when there is no queue (tests).
func (s *Server) runSchedule(w http.ResponseWriter, r *http.Request) {
	u, _, ok := s.scheduler(w, r)
	if !ok {
		return
	}
	id, ok := s.scheduleID(w, r)
	if !ok {
		return
	}
	if _, err := s.st.Q.GetMCPSchedule(r.Context(), id); err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Jadwal tidak ditemukan")
		return
	}
	by := deref(u.Name)
	if s.jobs != nil {
		if _, err := s.jobs.Insert(r.Context(), jobs.AnalystRunArgs{ScheduleID: id.String(), By: by}, &river.InsertOpts{MaxAttempts: 1}); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		s.auditUser(r, "analyst.run", "mcp_schedule:"+id.String(), nil)
		httpx.JSON(w, http.StatusAccepted, map[string]any{"message": "Analisis dimulai — laporan muncul di riwayat dalam 1–3 menit"})
		return
	}
	run, err := s.analystRunner().Run(r.Context(), id, nil, "manual", by)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": "Analisis selesai", "run": run})
}

func (s *Server) analystRunner() *analyst.Runner {
	rn := &analyst.Runner{St: s.st, Clock: s.clock, Log: s.log, SessionKey: s.secret(), EnvKey: s.cfg.AnthropicKey, NewModel: s.analystModel}
	if s.mcp != nil {
		rn.Orch = s.mcp.Orch
	}
	return rn
}

func (s *Server) scheduleRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := s.scheduleID(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.st.Q.ListMCPScheduleRuns(r.Context(), gen.ListMCPScheduleRunsParams{ScheduleID: id, Limit: int32(limit)})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if rows == nil {
		rows = []gen.ListMCPScheduleRunsRow{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) scheduleRun(w http.ResponseWriter, r *http.Request) {
	id, ok := s.scheduleID(w, r)
	if !ok {
		return
	}
	run, err := s.st.Q.GetMCPScheduleRun(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Laporan tidak ditemukan")
		return
	}
	httpx.JSON(w, http.StatusOK, run)
}
