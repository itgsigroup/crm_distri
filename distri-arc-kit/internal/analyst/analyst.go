// Package analyst runs scheduled analysis (ADR 0022): on a cron schedule Claude analyses GSI Orbit through the
// same MCP tools a connected Claude uses, and writes a Markdown report. The tools are served in-process (in-memory
// MCP transport) under the schedule's own mcp_clients row, so every call is scope-checked, rate-limited and logged
// like any other MCP call. Without an Anthropic API key (or over the daily budget) the run still produces a
// template report from the same tools — numbers only, no model.
package analyst

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"distri-arc/internal/auth"
	"distri-arc/internal/clock"
	"distri-arc/internal/events"
	"distri-arc/internal/llm"
	"distri-arc/internal/mcp"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Names used in secrets, policies and llm_calls.
const (
	SecretKey  = "anthropic.api_key"
	PolicyKey  = "mcp.analyst"
	Purpose    = "mcp.schedule"
	AgentName  = "Analis terjadwal"
	maxResult  = 40000 // characters of one tool result sent to the model
	maxTokens  = 12000
	stepBuffer = 4 // model turns allowed beyond max_steps (the final write-up)
)

// Config is policy mcp.analyst.
type Config struct {
	Model       string `json:"model"`        // Claude API model
	OpenAIModel string `json:"openai_model"` // ChatGPT API model
	// Fallback: the API that analyses when Claude through MCP did not answer in time (ADR 0040) — off | claude | openai
	Fallback       string `json:"fallback"`
	DailyBudgetIDR int64  `json:"daily_budget_idr"` // 0 = no limit (both APIs together)
}

// Fallback values.
const (
	FallbackOff    = "off"
	FallbackClaude = "claude"
	FallbackOpenAI = "openai"
)

// Provider is the API the server's runs use: ChatGPT when it is the chosen second option, Claude otherwise.
func (c Config) Provider() string {
	if c.Fallback == FallbackOpenAI {
		return FallbackOpenAI
	}
	return FallbackClaude
}

// ProviderModel is the model of Provider.
func (c Config) ProviderModel() string {
	if c.Provider() == FallbackOpenAI {
		return c.OpenAIModel
	}
	return c.Model
}

// ModelInfo is a model the analyst may use.
type ModelInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Models: the Claude models GSI Orbit knows, newest tier first (prices in llm.Prices). With a key the page also lists
// what Anthropic's Models API returns (LiveModels).
var Models = []ModelInfo{
	{"claude-fable-5-1", "Claude Fable 5.1"},
	{"claude-fable-5", "Claude Fable 5"},
	{"claude-opus-5-5", "Claude Opus 5.5"},
	{"claude-opus-5", "Claude Opus 5"},
	{"claude-opus-4-8", "Claude Opus 4.8"},
	{"claude-opus-4-7", "Claude Opus 4.7"},
	{"claude-opus-4-6", "Claude Opus 4.6"},
	{"claude-sonnet-5-5", "Claude Sonnet 5.5"},
	{"claude-sonnet-5", "Claude Sonnet 5"},
	{"claude-sonnet-4-6", "Claude Sonnet 4.6"},
	{"claude-haiku-5-5", "Claude Haiku 5.5"},
	{"claude-haiku-4-5", "Claude Haiku 4.5"},
}

// OpenAIModels: the ChatGPT models GSI Orbit knows (prices in llm.Prices). With a key the page lists every chat
// model OpenAI's Models API returns (LiveModels).
var OpenAIModels = []ModelInfo{
	{"gpt-5", "GPT-5"},
	{"gpt-5-mini", "GPT-5 mini"},
	{"gpt-5-nano", "GPT-5 nano"},
	{"gpt-4.1", "GPT-4.1"},
	{"gpt-4.1-mini", "GPT-4.1 mini"},
	{"gpt-4.1-nano", "GPT-4.1 nano"},
	{"gpt-4o", "GPT-4o"},
	{"gpt-4o-mini", "GPT-4o mini"},
	{"o3", "o3"},
	{"o4-mini", "o4-mini"},
}

// Model ids the provider may name (any model it lists, not only the ones above).
var (
	claudeID = regexp.MustCompile(`^claude-[a-z0-9.-]{2,60}$`)
	openaiID = regexp.MustCompile(`^(gpt-|chatgpt-|o[1-9])[a-z0-9.-]{0,60}$`)
)

// DefaultConfig applies when the policy row is missing.
var DefaultConfig = Config{Model: "claude-opus-5-5", OpenAIModel: "gpt-5", Fallback: FallbackOff, DailyBudgetIDR: 49500} // $3 at Rp16.500

// LoadConfig reads policy mcp.analyst.
func LoadConfig(ctx context.Context, q *gen.Queries) Config {
	c := DefaultConfig
	if p, err := q.GetPolicy(ctx, PolicyKey); err == nil {
		_ = json.Unmarshal(p.Value, &c)
	}
	if !ValidModel(c.Model) {
		c.Model = DefaultConfig.Model
	}
	if !ValidOpenAIModel(c.OpenAIModel) {
		c.OpenAIModel = DefaultConfig.OpenAIModel
	}
	if !validFallback(c.Fallback) {
		c.Fallback = FallbackOff
	}
	return c
}

// ValidOpenAIModel reports whether id is a ChatGPT model id.
func ValidOpenAIModel(id string) bool { return openaiID.MatchString(id) }

func validFallback(f string) bool {
	return f == FallbackOff || f == FallbackClaude || f == FallbackOpenAI
}

// ValidModel reports whether id is a Claude model id.
func ValidModel(id string) bool { return claudeID.MatchString(id) }

// adaptiveModel: Claude 4.6 and later think adaptively; older models (Haiku 4.5, Sonnet 4.5, 3.x) take no adaptive
// thinking parameter and get none.
var adaptiveModel = regexp.MustCompile(`^claude-(?:opus|sonnet|haiku|fable|mythos)-(\d+)(?:-(\d+))?`)

func adaptiveThinking(id string) bool {
	m := adaptiveModel.FindStringSubmatch(id)
	if m == nil {
		return false
	}
	major, minor := atoi(m[1]), atoi(m[2])
	if minor > 99 { // a date suffix, not a minor version (claude-opus-4-20250514)
		minor = 0
	}
	return major >= 5 || (major == 4 && minor >= 6)
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// SaveConfig writes policy mcp.analyst.
func SaveConfig(ctx context.Context, q *gen.Queries, c Config, by *uuid.UUID, now time.Time) error {
	if !ValidModel(c.Model) {
		return fmt.Errorf("model %q tidak dikenal", c.Model)
	}
	if c.OpenAIModel == "" {
		c.OpenAIModel = DefaultConfig.OpenAIModel
	}
	if !ValidOpenAIModel(c.OpenAIModel) {
		return fmt.Errorf("model %q tidak dikenal", c.OpenAIModel)
	}
	if c.Fallback == "" {
		c.Fallback = FallbackOff
	}
	if !validFallback(c.Fallback) {
		return fmt.Errorf("opsi kedua %q tidak dikenal", c.Fallback)
	}
	if c.DailyBudgetIDR < 0 {
		return errors.New("anggaran tidak boleh negatif")
	}
	b, _ := json.Marshal(c)
	_, err := q.SetPolicy(ctx, gen.SetPolicyParams{Key: PolicyKey, Value: b, UpdatedBy: by, UpdatedAt: now})
	return err
}

// LoadKey returns the Anthropic API key and where it came from: "ui" (sealed in secrets), "env" or "".
func LoadKey(ctx context.Context, q *gen.Queries, sessionKey []byte, envKey string) (string, string) {
	if sealed, err := q.GetSecret(ctx, SecretKey); err == nil {
		if k, err := auth.Open(sealed, sessionKey); err == nil && k != "" {
			return k, "ui"
		}
	}
	if envKey != "" {
		return envKey, "env"
	}
	return "", ""
}

// KeyHint shows the key without revealing it: sk-ant-…wxyz.
func KeyHint(k string) string {
	if len(k) < 12 {
		return "…"
	}
	return k[:7] + "…" + k[len(k)-4:]
}

// Model is one turn of the conversation with Claude (the Messages API; scripted in tests).
type Model interface {
	Step(ctx context.Context, p anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error)
}

type anthropicModel struct{ c anthropic.Client }

// NewAnthropic calls the Messages API with key.
func NewAnthropic(key string) Model {
	return anthropicModel{c: anthropic.NewClient(option.WithAPIKey(key), option.WithRequestTimeout(5*time.Minute), option.WithMaxRetries(2))}
}

func (m anthropicModel) Step(ctx context.Context, p anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	return m.c.Beta.Messages.New(ctx, p)
}

// Runner executes schedule runs.
type Runner struct {
	St         *store.Store
	Clock      clock.Clock
	Log        *slog.Logger
	Orch       *orchestrator.Orchestrator // analisis_* tools run Orchestrator cycles
	SessionKey []byte                     // opens the sealed API key
	EnvKey     string                     // ANTHROPIC_API_KEY (when LLM_PROVIDER=anthropic)
	EnvOpenAI  string                     // OPENAI_API_KEY
	NewModel   func(key string) Model     // default NewAnthropic
	NewChat    func(key string) ChatModel // default NewOpenAIChat
}

// Step is one tool call of a run (shown under the report).
type Step struct {
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args,omitempty"`
	Status  string          `json:"status"`
	MS      int64           `json:"ms"`
	Summary string          `json:"summary,omitempty"`
}

type outcome struct {
	status, engine, model, report string
	steps                         []Step
	in, out                       int64
	cost                          int64
	err                           error
}

// Run executes one run of a schedule. A scheduled slot runs at most once (unique schedule+slot): a second call
// for the same slot returns (nil, nil). Model and tool failures are recorded on the run, not returned.
func (r *Runner) Run(ctx context.Context, id uuid.UUID, slot *time.Time, trigger, by string) (*gen.McpScheduleRun, error) {
	q := r.St.Q
	sch, err := q.GetMCPSchedule(ctx, id)
	if err != nil {
		return nil, err
	}
	if trigger == "schedule" && !sch.Enabled {
		return nil, nil
	}
	run, err := q.InsertMCPScheduleRun(ctx, gen.InsertMCPScheduleRunParams{ScheduleID: id, Slot: slot, Trigger: trigger, TriggeredBy: &by})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = events.Notify(ctx, r.St.Pool, "mcp_schedule", map[string]any{"schedule_id": id, "run_id": run.ID, "status": "running"})
	o := r.execute(ctx, sch)
	if o.status == "" {
		o.status = "ok"
	}
	if o.err == nil {
		o.err = requiredOutcome(sch, o)
	}
	var errText *string
	if o.err != nil {
		o.status = "error"
		errText = ptr(o.err.Error())
		r.Log.Warn("scheduled analysis failed", "schedule", sch.Name, "err", o.err)
	}
	steps, _ := json.Marshal(nonNil(o.steps))
	fin := r.Clock.Now()
	ctx = context.WithoutCancel(ctx)
	if err := q.FinishMCPScheduleRun(ctx, gen.FinishMCPScheduleRunParams{ID: run.ID, Status: o.status, Engine: &o.engine, Model: &o.model,
		Report: &o.report, Steps: steps, TokensIn: int32(o.in), TokensOut: int32(o.out), CostIdr: o.cost, Error: errText, FinishedAt: &fin}); err != nil {
		return nil, err
	}
	_ = q.TouchMCPScheduleRun(ctx, gen.TouchMCPScheduleRunParams{ID: id, LastRunAt: &run.StartedAt})
	_ = events.Notify(ctx, r.St.Pool, "mcp_schedule", map[string]any{"schedule_id": id, "run_id": run.ID, "status": o.status})
	r.Log.Info("scheduled analysis", "schedule", sch.Name, "status", o.status, "engine", o.engine, "steps", len(o.steps), "cost_idr", o.cost)
	out, err := q.GetMCPScheduleRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	res := gen.McpScheduleRun{ID: out.ID, ScheduleID: out.ScheduleID, Slot: out.Slot, Trigger: out.Trigger, TriggeredBy: out.TriggeredBy,
		Status: out.Status, Engine: out.Engine, Model: out.Model, Report: out.Report, Steps: out.Steps, TokensIn: out.TokensIn,
		TokensOut: out.TokensOut, CostIdr: out.CostIdr, Error: out.Error, StartedAt: out.StartedAt, FinishedAt: out.FinishedAt}
	return &res, nil
}

func (r *Runner) execute(ctx context.Context, sch gen.McpSchedule) outcome {
	client, err := r.client(ctx, sch)
	if err != nil {
		return outcome{engine: "template", err: err}
	}
	srv := mcp.New(r.St, r.Clock, r.Log, r.Orch)
	srv.Fixed = client
	cs, err := connect(ctx, srv)
	if err != nil {
		return outcome{engine: "template", err: err}
	}
	defer func() { _ = cs.Close() }()

	cfg := LoadConfig(ctx, r.St.Q)
	prov := cfg.Provider()
	var key string
	if prov == FallbackOpenAI {
		key, _ = LoadOpenAIKey(ctx, r.St.Q, r.SessionKey, r.EnvOpenAI)
	} else {
		key, _ = LoadKey(ctx, r.St.Q, r.SessionKey, r.EnvKey)
	}
	label := ProviderLabel(prov)
	spent, _ := r.St.Q.AnalystCostSince(ctx, clock.Today(r.Clock.Now()))
	strict := true
	if pol, err := policy.Load(ctx, r.St.Q); err == nil {
		strict = pol.LLM.RequireAI
	}
	if strict { // require_ai: no AI, no report — the run fails instead of writing a template
		switch {
		case key == "":
			return outcome{engine: "none", err: fmt.Errorf("wajib AI: kunci %s belum diisi (Pengaturan → API & model AI) — analisis gagal, tanpa laporan template", label)}
		case cfg.DailyBudgetIDR > 0 && spent.Cost >= cfg.DailyBudgetIDR:
			return outcome{engine: "none", err: fmt.Errorf("wajib AI: anggaran harian %s sudah terpakai (%s) — analisis gagal, tanpa laporan template", rp(cfg.DailyBudgetIDR), rp(spent.Cost))}
		}
	}
	switch {
	case key == "":
		return r.template(ctx, cs, "Mode template — kunci "+label+" belum diisi (Pengaturan → API & model AI). Angka dari tool MCP; analisis dan rekomendasi AI aktif setelah kunci diisi.")
	case cfg.DailyBudgetIDR > 0 && spent.Cost >= cfg.DailyBudgetIDR:
		return r.template(ctx, cs, fmt.Sprintf("Mode template — anggaran harian analisis %s sudah terpakai (%s). AI dipakai lagi besok atau setelah anggaran dinaikkan.", rp(cfg.DailyBudgetIDR), rp(spent.Cost)))
	}
	tools := toolset(srv.Tools(), sch.Scopes)
	var o outcome
	if prov == FallbackOpenAI {
		newChat := r.NewChat
		if newChat == nil {
			newChat = NewOpenAIChat
		}
		o = r.openai(ctx, cs, newChat(key), cfg.OpenAIModel, sch, tools)
	} else {
		newModel := r.NewModel
		if newModel == nil {
			newModel = NewAnthropic
		}
		o = r.claude(ctx, cs, newModel(key), cfg.Model, sch, tools)
	}
	if o.err != nil && o.report == "" && !strict {
		// keep the numbers coming even when the model fails; the run stays marked as an error
		t := r.template(ctx, cs, label+" gagal ("+o.err.Error()+") — laporan template dari tool MCP.")
		o.report, o.steps = t.report, append(o.steps, t.steps...)
	}
	return o
}

// client is the schedule's mcp_clients row (created on first use; name and scopes follow the schedule).
func (r *Runner) client(ctx context.Context, sch gen.McpSchedule) (*mcp.Client, error) {
	name := "Terjadwal · " + sch.Name
	if sch.ClientID != nil {
		if err := r.St.Q.UpdateScheduleMCPClient(ctx, gen.UpdateScheduleMCPClientParams{ID: *sch.ClientID, Name: &name, Scopes: sch.Scopes}); err != nil {
			return nil, err
		}
		return &mcp.Client{ID: *sch.ClientID, Name: name, Scopes: sch.Scopes}, nil
	}
	c, err := r.St.Q.InsertScheduleMCPClient(ctx, gen.InsertScheduleMCPClientParams{Name: &name, Scopes: sch.Scopes, OwnerID: sch.CreatedBy})
	if err != nil {
		return nil, err
	}
	if err := r.St.Q.SetMCPScheduleClient(ctx, gen.SetMCPScheduleClientParams{ID: sch.ID, ClientID: &c.ID}); err != nil {
		return nil, err
	}
	return &mcp.Client{ID: c.ID, Name: name, Scopes: sch.Scopes}, nil
}

// connect opens an in-process MCP session to srv.
func connect(ctx context.Context, srv *mcp.Server) (*sdk.ClientSession, error) {
	ct, st := sdk.NewInMemoryTransports()
	if _, err := srv.MCP().Connect(ctx, st, nil); err != nil {
		return nil, err
	}
	return sdk.NewClient(&sdk.Implementation{Name: "distri-arc-analyst", Version: mcp.Version}, nil).Connect(ctx, ct, nil)
}

// toolset is the tools a schedule may use: its scopes, never decide.
func toolset(all []mcp.ToolInfo, scopes []string) map[string]bool {
	out := map[string]bool{}
	for _, t := range all {
		if t.Scope != "decide" && slices.Contains(scopes, t.Scope) {
			out[t.Name] = true
		}
	}
	return out
}

func (r *Runner) claude(ctx context.Context, cs *sdk.ClientSession, m Model, model string, sch gen.McpSchedule, allowed map[string]bool) outcome {
	o := outcome{engine: "claude", model: model}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		o.err = err
		return o
	}
	var tools []anthropic.BetaToolUnionParam
	for _, t := range list.Tools {
		if !allowed[t.Name] {
			continue
		}
		tools = append(tools, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name: t.Name, Description: anthropic.String(t.Description), InputSchema: inputSchema(t.InputSchema)}})
	}
	now := r.Clock.Now().In(clock.WIB)
	system := llm.Prompt("analyst") + fmt.Sprintf("\nSaat ini: %s pukul %s WIB. Batas: %d panggilan tool.", longDate(now), now.Format("15.04"), sch.MaxSteps)
	masker := llm.NewMasker()
	p := anthropic.BetaMessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		System:    []anthropic.BetaTextBlockParam{{Text: system, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}},
		Tools:     tools,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(sch.Prompt))},
	}
	if adaptiveThinking(model) {
		p.Thinking = anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}
	}
	calls := 0
	for turn := 0; turn < int(sch.MaxSteps)+stepBuffer; turn++ {
		t0 := time.Now()
		msg, err := m.Step(ctx, p)
		if err != nil {
			o.err = modelError(err)
			r.record(ctx, model, 0, 0, time.Since(t0), true)
			return o
		}
		in := msg.Usage.InputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.CacheReadInputTokens
		o.in, o.out = o.in+in, o.out+msg.Usage.OutputTokens
		o.cost += llm.Cost(model, in, msg.Usage.OutputTokens)
		r.record(ctx, model, in, msg.Usage.OutputTokens, time.Since(t0), false)
		p.Messages = append(p.Messages, msg.ToParam())
		switch msg.StopReason {
		case anthropic.BetaStopReasonToolUse:
			var results []anthropic.BetaContentBlockParamUnion
			for _, b := range msg.Content {
				tu, ok := b.AsAny().(anthropic.BetaToolUseBlock)
				if !ok {
					continue
				}
				if calls >= int(sch.MaxSteps) {
					results = append(results, anthropic.NewBetaToolResultBlock(tu.ID, "Batas panggilan tool tercapai. Tulis laporan akhir sekarang dari data yang sudah ada.", true))
					continue
				}
				calls++
				text, st := r.call(ctx, cs, tu.Name, tu.Input, allowed, masker)
				o.steps = append(o.steps, st)
				results = append(results, anthropic.NewBetaToolResultBlock(tu.ID, text, st.Status != "ok"))
			}
			p.Messages = append(p.Messages, anthropic.NewBetaUserMessage(results...))
		case anthropic.BetaStopReasonPauseTurn:
			continue
		case anthropic.BetaStopReasonRefusal:
			o.err = errors.New("permintaan ditolak Claude — ubah prompt jadwal")
			return o
		default:
			o.report = strings.TrimSpace(masker.Unmask(text(msg)))
			if msg.StopReason == anthropic.BetaStopReasonMaxTokens {
				o.report += "\n\n_(Laporan terpotong: batas panjang jawaban tercapai.)_"
			}
			if o.report == "" {
				o.err = errors.New("laporan kosong dari Claude")
			}
			return o
		}
	}
	o.err = fmt.Errorf("analisis belum selesai setelah %d langkah — naikkan batas langkah atau persempit prompt", int(sch.MaxSteps)+stepBuffer)
	return o
}

func modelError(err error) error {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case 401:
			return errors.New("kunci Claude API ditolak (401) — periksa atau ganti kunci")
		case 429:
			return errors.New("batas pemakaian Claude API tercapai (429) — coba lagi nanti")
		}
		return fmt.Errorf("galat Claude API %d", apierr.StatusCode)
	}
	return err
}

// call runs one MCP tool; the result goes to the model with phone numbers and e-mails masked.
func (r *Runner) call(ctx context.Context, cs *sdk.ClientSession, name string, input any, allowed map[string]bool, m *llm.Masker) (string, Step) {
	args, _ := json.Marshal(input)
	args = []byte(m.Unmask(string(args))) // the model may pass back a placeholder it saw
	st := Step{Tool: name, Args: args, Status: "ok"}
	if !allowed[name] {
		st.Status, st.Summary = "forbidden", "tool di luar izin jadwal"
		return "Tool " + name + " tidak diizinkan untuk jadwal ini.", st
	}
	t0 := time.Now()
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: json.RawMessage(args)})
	st.MS = time.Since(t0).Milliseconds()
	if err != nil {
		st.Status, st.Summary = "error", err.Error()
		return "Gagal: " + err.Error(), st
	}
	out := resultText(res)
	if res.IsError {
		st.Status, st.Summary = "error", clip(out, 200)
		return out, st
	}
	st.Summary = fmt.Sprintf("%d karakter", len(out))
	return clip(maskJSON(out, m), maxResult), st
}

func resultText(res *sdk.CallToolResult) string {
	if res.StructuredContent != nil {
		if b, err := json.Marshal(res.StructuredContent); err == nil {
			return string(b)
		}
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*sdk.TextContent); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}

// maskJSON masks PII inside string values only (rupiah amounts are numbers and stay readable).
func maskJSON(s string, m *llm.Masker) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return m.Mask(s, nil)
	}
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case string:
			return m.Mask(t, nil)
		case []any:
			for i := range t {
				t[i] = walk(t[i])
			}
		case map[string]any:
			for k := range t {
				t[k] = walk(t[k])
			}
		}
		return x
	}
	b, _ := json.Marshal(walk(v))
	return string(b)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…(dipotong; pakai filter/limit/offset untuk sisanya)"
}

func text(msg *anthropic.BetaMessage) string {
	var sb strings.Builder
	for _, b := range msg.Content {
		if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}

// inputSchema converts an MCP tool's JSON schema to the Messages API shape.
func inputSchema(s any) anthropic.BetaToolInputSchemaParam {
	var m map[string]any
	if b, err := json.Marshal(s); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	out := anthropic.BetaToolInputSchemaParam{Properties: map[string]any{}}
	if p, ok := m["properties"]; ok {
		out.Properties = p
	}
	if req, ok := m["required"].([]any); ok {
		for _, x := range req {
			if s, ok := x.(string); ok {
				out.Required = append(out.Required, s)
			}
		}
	}
	extra := map[string]any{}
	for k, v := range m {
		if k != "properties" && k != "required" && k != "type" && k != "$schema" {
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		out.ExtraFields = extra
	}
	return out
}

func (r *Runner) record(ctx context.Context, model string, in, out int64, d time.Duration, failed bool) {
	agent, provider, purpose := AgentName, "anthropic", Purpose
	if !strings.HasPrefix(model, "claude-") {
		provider = "openai"
	}
	if failed {
		purpose += " (error)"
	}
	cost := llm.Cost(model, in, out)
	if _, err := r.St.Q.InsertLLMCall(context.WithoutCancel(ctx), gen.InsertLLMCallParams{Agent: &agent, Provider: &provider, Model: &model, Purpose: &purpose,
		TokensIn: ptr(int32(in)), TokensOut: ptr(int32(out)), CostIdr: &cost, DurationMs: ptr(int32(d.Milliseconds()))}); err != nil {
		r.Log.Warn("llm_calls insert", "err", err)
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func ptr[T any](v T) *T { return &v }

// ErrKeyRejected means Anthropic answered 401/403 for the key.
var ErrKeyRejected = errors.New("kunci Claude API ditolak Anthropic — periksa kuncinya (console.anthropic.com → API keys)")

// VerifyKey checks a key against the Models API (free; no tokens are used). Network errors are returned as-is so
// the caller can still save the key when Anthropic is unreachable from the server.
func VerifyKey(ctx context.Context, key string) error {
	c := anthropic.NewClient(option.WithAPIKey(key), option.WithRequestTimeout(15*time.Second), option.WithMaxRetries(1))
	_, err := c.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(1)})
	var apierr *anthropic.Error
	if errors.As(err, &apierr) && (apierr.StatusCode == 401 || apierr.StatusCode == 403) {
		return ErrKeyRejected
	}
	return err
}

// KeyWAAnalysis is the schedule that writes Analisa WhatsApp of Pusat kendali every 2 hours (ADR 0037).
const KeyWAAnalysis = "wa_analysis"

// requiredOutcome: a schedule the app depends on succeeds only when Claude did its part — for Analisa WhatsApp, an
// accepted orchestrator_submit. Anything else (template mode, no submit, a refused submit) is a failed run, so the
// label in Pusat kendali never says "berhasil" for an analysis that did not land.
func requiredOutcome(sch gen.McpSchedule, o outcome) error {
	if sch.Key == nil || *sch.Key != KeyWAAnalysis {
		return nil
	}
	if o.engine != "claude" && o.engine != "openai" {
		return errors.New("analisa WhatsApp butuh AI: kunci API belum diisi atau anggaran habis (Pengaturan → API & model AI)")
	}
	for _, st := range o.steps {
		if st.Tool == "orchestrator_submit" && st.Status == "ok" {
			return nil
		}
	}
	for _, st := range o.steps {
		if st.Tool == "orchestrator_submit" {
			return errors.New("claude mengirim Analisa WhatsApp tetapi ditolak: " + st.Summary)
		}
	}
	return errors.New("claude selesai tanpa mengirim Analisa WhatsApp (orchestrator_submit) — Pusat kendali tidak diperbarui")
}
