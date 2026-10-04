// Package app wires ARC together: HTTP API (UI + /api/v1 + webhooks), MCP
// server, OAuth, scheduler and the action executor.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"arc/apps/api/internal/seed"
	"arc/packages/connectors/google"
	"arc/packages/connectors/identity"
	"arc/packages/connectors/notify"
	"arc/packages/connectors/odoo"
	"arc/packages/connectors/whatsapp"
	"arc/packages/core/actions"
	"arc/packages/core/agents"
	"arc/packages/core/config"
	"arc/packages/core/domain"
	"arc/packages/core/insights"
	"arc/packages/core/llm"
	"arc/packages/core/storage"
)

// Version of the API.
const Version = "1.0.0"

// App holds every dependency.
type App struct {
	Cfg     config.Config
	DB      *storage.DB
	LLM     *llm.Router
	Fake    *llm.FakeProvider
	Ins     *insights.Service
	Actions *actions.Service
	Agents  *agents.Agents
	Log     *slog.Logger

	Bridge     *whatsapp.BridgeTransport
	Cloud      *whatsapp.CloudTransport
	FakeWA     *whatsapp.FakeTransport
	OdooReader odoo.Reader
	OdooWriter odoo.Writer
	OdooMock   bool
	Mail       google.MailSource
	Calendar   google.CalendarSource
	Drafts     google.DraftCreator
	GoogleMock bool
	Todos      notify.TodoCreator
	Notifiers  []notify.Notifier
	FakeNotify *notify.Fake

	limiter  *limiter
	debounce *debouncer
	root     string
}

// New builds the app. Connectors without credentials use their mock
// implementations (the stage is then done-with-mocks).
func New(ctx context.Context, cfg config.Config, db *storage.DB) (*App, error) {
	a := &App{Cfg: cfg, DB: db, Log: slog.Default(), limiter: newLimiter(), root: config.RepoRoot()}
	a.Ins = insights.New(db)
	a.Actions = actions.New(db)
	a.Fake = llm.NewFake()
	a.LLM = llm.NewRouter(a.logLLM)
	a.LLM.Register(a.Fake)
	provider := cfg.LLMProvider
	if cfg.AnthropicKey != "" {
		a.LLM.Register(llm.NewAnthropic(cfg.AnthropicKey))
	}
	if cfg.OpenAIKey != "" {
		a.LLM.Register(llm.NewOpenAI(cfg.OpenAIKey, ""))
	}
	if cfg.OllamaURL != "" {
		a.LLM.Register(llm.NewOllama(cfg.OllamaURL, ""))
	}
	if provider == "auto" {
		provider = "fake"
		if cfg.AnthropicKey != "" {
			provider = "anthropic"
		} else if cfg.OpenAIKey != "" {
			provider = "openai"
		} else if cfg.OllamaURL != "" {
			provider = "ollama"
		}
	}
	a.LLM.SetRoute(llm.Light, llm.Route{Provider: provider, Model: cfg.ModelLight})
	a.LLM.SetRoute(llm.Heavy, llm.Route{Provider: provider, Model: cfg.ModelHeavy})
	a.LLM.SetRoute(llm.Interactive, llm.Route{Provider: provider, Model: cfg.ModelInteractive})
	a.LLM.SetFallback(cfg.ModelFallback)

	a.Bridge = whatsapp.NewBridge(cfg.BridgeURL, cfg.BridgeSecret)
	if cfg.WACloudToken != "" {
		a.Cloud = whatsapp.NewCloud(cfg.WACloudToken, cfg.WACloudPhoneID, cfg.WACloudAppSecret)
	}
	a.FakeWA = whatsapp.NewFake()

	if cfg.OdooURL != "" && cfg.OdooAPIKey != "" {
		var comps []int
		for _, s := range strings.Split(cfg.OdooCompanies, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				comps = append(comps, n)
			}
		}
		a.OdooReader = odoo.NewClient(cfg.OdooURL, cfg.OdooDB, cfg.OdooUser, cfg.OdooAPIKey, comps)
		a.OdooWriter = odoo.NewWriter(cfg.OdooURL, cfg.OdooDB, cfg.OdooUser, cfg.OdooAPIKey)
	} else {
		f := odoo.NewFake()
		a.OdooReader, a.OdooWriter, a.OdooMock = f, f, true
	}

	fixtures := seed.FixtureDir(a.root)
	if cfg.GoogleClientID != "" {
		api := &google.API{Token: a.googleToken, HTTP: &http.Client{Timeout: 30 * time.Second}}
		a.Mail, a.Calendar, a.Drafts = api, api, api
	} else {
		fm := &google.FakeMail{Dir: filepath.Join(fixtures, "mail")}
		a.Mail, a.Drafts = fm, fm
		a.Calendar = &google.FakeCalendar{File: filepath.Join(fixtures, "calendar.json")}
		a.GoogleMock = true
	}

	a.FakeNotify = &notify.Fake{Name: "fake"}
	if cfg.SMTPHost != "" {
		a.Notifiers = append(a.Notifiers, &notify.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Pass: cfg.SMTPPass, From: cfg.SMTPFrom})
	}
	if cfg.BasecampToken != "" {
		bc := &notify.Basecamp{Token: cfg.BasecampToken, AccountID: cfg.BasecampAccountID, ProjectID: cfg.BasecampProjectID}
		a.Notifiers = append(a.Notifiers, bc)
		a.Todos = bc
	} else {
		a.Todos = a.FakeNotify
	}
	if len(a.Notifiers) == 0 {
		a.Notifiers = []notify.Notifier{a.FakeNotify}
	}

	var tc identity.Truecaller
	var web identity.WebSearch
	fakeID := identity.LoadFake(filepath.Join(fixtures, "identity.json"))
	tc, web = fakeID, fakeID
	if cfg.TruecallerKey != "" {
		tc = &identity.TruecallerAPI{Key: cfg.TruecallerKey}
	}
	if cfg.WebSearchKey != "" {
		web = &identity.SearchAPI{Key: cfg.WebSearchKey}
	}

	a.Agents = &agents.Agents{DB: db, LLM: a.LLM, Fake: a.Fake, Ins: a.Ins, Actions: a.Actions, Fx: agents.LoadFakeData(fixtures),
		Profiles: a.profileTransport(), Truecaller: tc, Web: web, Notifiers: a.Notifiers, Recipients: a.recipients, Log: a.Log}
	a.Agents.RegisterFakes()
	a.Actions.Exec = &executor{a: a}
	a.Actions.Emit = a.emitWebhook
	a.Actions.Notify = func(ctx context.Context, role, subject, body string) {
		for _, n := range a.Notifiers {
			_ = n.Send(ctx, notify.Message{To: a.recipients(ctx, role), Subject: subject, Text: body, HTML: "<p>" + body + "</p>"})
		}
	}
	a.debounce = newDebouncer(60*time.Second, func(threadID string) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := a.Agents.ExtractThread(ctx, threadID); err != nil {
			a.Log.Warn("debounced capture failed", "thread", threadID, "err", err)
		}
		a.afterCapture(ctx, threadID)
	})
	a.applyRoutingSetting(ctx)
	return a, nil
}

// profileTransport is the transport used for WhatsApp profile lookups.
func (a *App) profileTransport() whatsapp.Transport {
	if a.Cfg.Env == "test" {
		return a.FakeWA
	}
	return a.Bridge
}

func (a *App) logLLM(c llm.Call) {
	_, err := a.DB.Pool.Exec(context.Background(), `INSERT INTO llm_calls(tier,provider,model,tokens_in,tokens_out,cost_est,purpose,input_hash,duration_ms,ok,error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, string(c.Tier), c.Provider, c.Model, c.TokensIn, c.TokensOut, c.CostEst, c.Purpose, c.InputHash, c.DurationMS, c.OK, c.Error)
	if err != nil {
		a.Log.Warn("llm log failed", "err", err)
	}
	a.Log.Info("llm_call", "tier", c.Tier, "provider", c.Provider, "model", c.Model, "tokens_in", c.TokensIn, "tokens_out", c.TokensOut, "cost_usd", c.CostEst, "purpose", c.Purpose, "input_hash", c.InputHash, "ok", c.OK)
}

func (a *App) recipients(ctx context.Context, role string) []string {
	rows, err := a.DB.Pool.Query(ctx, `SELECT email FROM users WHERE role=$1`, role)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		_ = rows.Scan(&e)
		out = append(out, e)
	}
	return out
}

// applyRoutingSetting maps the UI routing labels to provider/model.
func (a *App) applyRoutingSetting(ctx context.Context) {
	var r map[string]string
	if !a.Ins.Setting(ctx, "routing", &r) {
		return
	}
	for tier, label := range r {
		t := llm.Tier(tier)
		if tier == "fallback" {
			switch label {
			case "OpenAI":
				a.LLM.SetFallback("openai")
			case "Claude":
				a.LLM.SetFallback("anthropic")
			case "Self-hosted":
				a.LLM.SetFallback("ollama")
			}
			continue
		}
		route := a.LLM.RouteFor(t)
		if route.Provider == "fake" {
			continue // mock mode keeps the deterministic provider
		}
		switch {
		case strings.HasPrefix(label, "Claude Haiku"):
			if _, ok := a.LLM.Provider("anthropic"); ok {
				a.LLM.SetRoute(t, llm.Route{Provider: "anthropic", Model: "claude-haiku-4-5"})
			}
		case strings.HasPrefix(label, "Claude Sonnet 5"):
			if _, ok := a.LLM.Provider("anthropic"); ok {
				a.LLM.SetRoute(t, llm.Route{Provider: "anthropic", Model: "claude-sonnet-5"})
			}
		case strings.HasPrefix(label, "Claude Opus"):
			if _, ok := a.LLM.Provider("anthropic"); ok {
				a.LLM.SetRoute(t, llm.Route{Provider: "anthropic", Model: "claude-opus-5-5"})
			}
		case strings.HasPrefix(label, "OpenAI"):
			if _, ok := a.LLM.Provider("openai"); ok {
				a.LLM.SetRoute(t, llm.Route{Provider: "openai"})
			}
		case strings.HasPrefix(label, "Self-hosted"):
			if _, ok := a.LLM.Provider("ollama"); ok {
				a.LLM.SetRoute(t, llm.Route{Provider: "ollama"})
			}
		}
	}
}

// PostSeed runs the agents once over freshly seeded data so every screen has
// computed content (brief, Odoo link state, Gmail/Calendar capture, Ask history).
func (a *App) PostSeed(ctx context.Context, f *seed.Fixtures) error {
	if _, err := a.RunJob(ctx, "sync_odoo"); err != nil {
		a.Log.Warn("post-seed odoo sync", "err", err)
	}
	for _, job := range []string{"capture_gmail", "capture_calendar", "tender_radar", "forecast", "coaching"} {
		if _, err := a.RunJob(ctx, job); err != nil {
			a.Log.Warn("post-seed job", "job", job, "err", err)
		}
	}
	var hasBrief bool
	_ = a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM briefs)`).Scan(&hasBrief)
	if !hasBrief {
		if _, err := a.Agents.GenerateBrief(ctx, "sore", false); err != nil {
			return fmt.Errorf("brief: %w", err)
		}
	}
	var hasAsk bool
	_ = a.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ask_history WHERE user_id='sam')`).Scan(&hasAsk)
	if !hasAsk {
		for _, q := range f.Extra.AskSeed {
			if _, err := a.Agents.Ask(ctx, "sam", q, "ask", insights.Scope{All: true}); err != nil {
				return fmt.Errorf("ask seed: %w", err)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	return nil
}

// Serve runs the HTTP server and the scheduler until ctx is cancelled.
func (a *App) Serve(ctx context.Context) error {
	srv := &http.Server{Addr: a.Cfg.Addr, Handler: a.Routes(), ReadHeaderTimeout: 10 * time.Second}
	if a.Cfg.SchedulerEnabled {
		go a.runScheduler(ctx)
		go a.runBridgeWatch(ctx)
	}
	errc := make(chan error, 1)
	go func() {
		a.Log.Info("arc api listening", "addr", a.Cfg.Addr, "llm", a.LLM.RouteFor(llm.Heavy).Provider, "odoo_mock", a.OdooMock, "google_mock", a.GoogleMock, "demo_clock", domain.ClockIsDemo())
		errc <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// stage reads .arc/progress.json to report the current stage in /health.
func (a *App) stage() any {
	raw, err := os.ReadFile(filepath.Join(a.root, ".arc", "progress.json"))
	if err != nil {
		return nil
	}
	var p struct {
		Current int `json:"current_stage"`
		Stages  []struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
			Name   string `json:"name"`
		} `json:"stages"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	return p.Current
}

// debouncer runs fn(key) once, `wait` after the last trigger for that key.
type debouncer struct {
	mu     sync.Mutex
	wait   time.Duration
	timers map[string]*time.Timer
	fn     func(string)
}

func newDebouncer(wait time.Duration, fn func(string)) *debouncer {
	return &debouncer{wait: wait, timers: map[string]*time.Timer{}, fn: fn}
}

func (d *debouncer) trigger(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.timers[key]; ok {
		t.Stop()
	}
	d.timers[key] = time.AfterFunc(d.wait, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()
		d.fn(key)
	})
}
