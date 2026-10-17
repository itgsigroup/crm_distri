package analyst

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Analisa WhatsApp otomatis (ADR 0043): at the hours chosen on Pengaturan → API & model AI the server runs the
// Analisa WhatsApp task with one chosen API and model. A failed run stays failed — the other API does not take over,
// and the API second option for the claude.ai routine stays quiet while this is on.

// KeyWAAuto is the schedule row of Analisa WhatsApp otomatis.
const KeyWAAuto = "wa_analysis_auto"

// WAAutoSchedule is the schedule row's name (its runs are listed on the page).
const WAAutoSchedule = "Analisa WhatsApp otomatis · API"

// AutoPolicyKey holds the chosen API and model; the hours and on/off live on the schedule row.
const AutoPolicyKey = "wa.auto"

// WAAuto is the setting as the page shows and saves it.
type WAAuto struct {
	Enabled bool       `json:"enabled"`
	Hours   []int      `json:"hours"`   // WIB, 0–23
	Weekend bool       `json:"weekend"` // also on Sunday (Monday–Saturday otherwise)
	Engine  string     `json:"engine"`  // claude | openai
	Model   string     `json:"model"`   // a model of Engine
	NextRun *time.Time `json:"next_run_at,omitempty"`
}

type autoPolicy struct {
	Engine string `json:"engine"`
	Model  string `json:"model"`
}

// LoadWAAuto reads the setting; found is false before the migration's schedule row exists.
func LoadWAAuto(ctx context.Context, q *gen.Queries) (WAAuto, bool) {
	a := WAAuto{Engine: FallbackClaude, Model: LoadConfig(ctx, q).Model, Hours: []int{8, 10, 12, 14, 16}}
	if p, err := q.GetPolicy(ctx, AutoPolicyKey); err == nil {
		var v autoPolicy
		if json.Unmarshal(p.Value, &v) == nil && validFallback(v.Engine) && v.Engine != FallbackOff {
			a.Engine, a.Model = v.Engine, v.Model
		}
	}
	if !validEngineModel(a.Engine, a.Model) {
		cfg := LoadConfig(ctx, q)
		a.Model = map[bool]string{true: cfg.OpenAIModel, false: cfg.Model}[a.Engine == FallbackOpenAI]
	}
	k := KeyWAAuto
	sch, err := q.GetMCPScheduleByKey(ctx, &k)
	if err != nil {
		return a, false
	}
	a.Enabled = sch.Enabled
	if h, weekend, ok := ParseAutoCron(sch.Cron); ok {
		a.Hours, a.Weekend = h, weekend
	}
	if a.Enabled {
		a.NextRun = sch.NextRunAt
	}
	return a, true
}

func validEngineModel(engine, model string) bool {
	if engine == FallbackOpenAI {
		return ValidOpenAIModel(model)
	}
	return engine == FallbackClaude && ValidModel(model)
}

// AutoCron is the schedule's cron: minute 0 of each hour, Monday–Saturday unless weekend.
func AutoCron(hours []int, weekend bool) string {
	hs := make([]string, len(hours))
	for i, h := range hours {
		hs[i] = strconv.Itoa(h)
	}
	days := "1-6"
	if weekend {
		days = "*"
	}
	return "0 " + strings.Join(hs, ",") + " * * " + days
}

// ParseAutoCron reads back what AutoCron wrote (ok is false for any other cron).
func ParseAutoCron(expr string) (hours []int, weekend, ok bool) {
	f := strings.Fields(expr)
	if len(f) != 5 || f[0] != "0" || f[2] != "*" || f[3] != "*" || (f[4] != "*" && f[4] != "1-6") {
		return nil, false, false
	}
	for _, s := range strings.Split(f[1], ",") {
		h, err := strconv.Atoi(s)
		if err != nil || h < 0 || h > 23 {
			return nil, false, false
		}
		hours = append(hours, h)
	}
	return hours, f[4] == "*", true
}

// SaveWAAuto validates and stores the setting: the API and model in policy wa.auto, the hours and on/off on the
// schedule row (its next time is recomputed by the next tick).
func SaveWAAuto(ctx context.Context, st *store.Store, a WAAuto, by *uuid.UUID, now time.Time) error {
	if a.Engine != FallbackClaude && a.Engine != FallbackOpenAI {
		return fmt.Errorf("AI %q tidak dikenal", a.Engine)
	}
	if !validEngineModel(a.Engine, a.Model) {
		return fmt.Errorf("model %q bukan model %s", a.Model, ProviderLabel(a.Engine))
	}
	hours := slices.Clone(a.Hours)
	slices.Sort(hours)
	hours = slices.Compact(hours)
	if len(hours) == 0 {
		return errors.New("pilih minimal satu jam")
	}
	if hours[0] < 0 || hours[len(hours)-1] > 23 {
		return errors.New("jam harus 0–23")
	}
	b, _ := json.Marshal(autoPolicy{Engine: a.Engine, Model: a.Model})
	if _, err := st.Q.SetPolicy(ctx, gen.SetPolicyParams{Key: AutoPolicyKey, Value: b, UpdatedBy: by, UpdatedAt: now}); err != nil {
		return err
	}
	tag, err := st.Pool.Exec(ctx, `update mcp_schedules set cron = $2, enabled = $3, next_run_at = null, updated_at = now() where key = $1`,
		KeyWAAuto, AutoCron(hours, a.Weekend), a.Enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("jadwal Analisa WhatsApp otomatis belum ada — jalankan migrasi")
	}
	return nil
}

// WAAutoOn reports whether Analisa WhatsApp otomatis is on (the API second option for the routine then stays off).
func WAAutoOn(ctx context.Context, q *gen.Queries) bool {
	a, ok := LoadWAAuto(ctx, q)
	return ok && a.Enabled
}
