// Package actions implements the Action lifecycle:
//
//	proposed → (approved | edited | rejected | snoozed) → executed | cancelled
//
// Agents and machine clients can only propose. Decisions are human-only and
// recorded for calibration; rejections suppress similar proposals for 14 days and
// three "Tidak sesuai kebijakan" rejections on the same pattern create a learned rule.
package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

// ErrHumanOnly is returned when a non-human actor tries to decide.
var ErrHumanOnly = errors.New("keputusan hanya untuk pengguna manusia")

// ErrInvalid is returned for malformed decisions.
var ErrInvalid = errors.New("keputusan tidak valid")

// Option is a custom decision button.
type Option struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Primary bool   `json:"primary,omitempty"`
	Result  string `json:"result,omitempty"`
	Toast   string `json:"toast,omitempty"`
}

// Impact is a number shown on policy items.
type Impact struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"`
}

// Pill is a tag in the queue header.
type Pill struct {
	K    string `json:"k"`
	T    string `json:"t"`
	Icon string `json:"icon,omitempty"`
}

// Action is the persisted proposal.
type Action struct {
	ID, Agent, Type, Kind, Title, ButtonLabel, Icon       string
	AccountID, OpportunityID, CashItemID, AccountName     string
	DueLabel                                              string
	DueAt                                                 *time.Time
	Summary, Why, Prep, Preview, PreviewFrom, ContextNote string
	Impact                                                []Impact
	Steps                                                 []string
	Options                                               []Option
	Tags                                                  []Pill
	Payload                                               map[string]any
	Evidence                                              []domain.Evidence
	Confidence                                            float64
	Model                                                 string
	InQueue                                               bool
	Status                                                string
	Decision                                              map[string]any
	ResultText                                            string
	ExecutedAt                                            *time.Time
	ProposedBy                                            string
	CreatedAt, UpdatedAt                                  time.Time
}

// Proposal is what agents submit.
type Proposal struct {
	ID            string
	Agent         string
	Type          string
	Kind          string
	Title         string
	ButtonLabel   string
	Icon          string
	AccountID     string
	OpportunityID string
	CashItemID    string
	DueLabel      string
	DueAt         *time.Time
	Summary       string
	Why           string
	Prep          string
	Preview       string
	PreviewFrom   string
	ContextNote   string
	Impact        []Impact
	Steps         []string
	Options       []Option
	Tags          []Pill
	Payload       map[string]any
	Evidence      []domain.Evidence
	Confidence    float64
	Model         string
	InQueue       bool
	// SignalFingerprint lets a proposal bypass a suppression when the underlying signal changed.
	SignalFingerprint string
	// OnePerAccount enforces a single active proposal of this agent per account (follow-up drafts).
	OnePerAccount bool
}

// Decision is a human decision.
type Decision struct {
	Decision string `json:"decision"`
	Option   string `json:"option"`
	Reason   string `json:"reason"`
	Note     string `json:"note"`
	Preview  string `json:"preview"`
}

// ExecResult is what an executor returns.
type ExecResult struct {
	Result string
	Toast  string
	Log    map[string]any
}

// Executor carries out an approved action (send via transport, Gmail draft, Odoo write…).
type Executor interface {
	Execute(ctx context.Context, a Action) (ExecResult, error)
}

// Emitter publishes outbound webhook events.
type Emitter func(ctx context.Context, event string, payload any)

// Service manages actions.
type Service struct {
	DB           *storage.DB
	Exec         Executor
	Emit         Emitter
	Notify       func(ctx context.Context, role, subject, body string)
	SuppressDays int
}

// New returns a service.
func New(db *storage.DB) *Service { return &Service{DB: db, SuppressDays: 14} }

const cols = `a.id,a.agent,a.type,a.kind,a.title,a.button_label,a.icon,COALESCE(a.account_id,''),COALESCE(a.opportunity_id,''),COALESCE(a.cash_item_id,''),
	COALESCE(acc.name, ci.account_name, ''),a.due_label,a.due_at,a.summary,a.why,a.prep,a.preview,a.preview_from,a.context_note,a.impact,a.steps,a.options,a.tags,
	a.payload,a.evidence,a.confidence::float8,a.model,a.in_queue,a.status,a.decision,a.result_text,a.executed_at,a.proposed_by,a.created_at,a.updated_at`

const from = ` FROM actions a LEFT JOIN accounts acc ON acc.id=a.account_id LEFT JOIN cash_items ci ON ci.id=a.cash_item_id `

func scan(row pgx.Row) (Action, error) {
	var a Action
	var impact, steps, options, tags, payload, evidence, decision []byte
	err := row.Scan(&a.ID, &a.Agent, &a.Type, &a.Kind, &a.Title, &a.ButtonLabel, &a.Icon, &a.AccountID, &a.OpportunityID, &a.CashItemID,
		&a.AccountName, &a.DueLabel, &a.DueAt, &a.Summary, &a.Why, &a.Prep, &a.Preview, &a.PreviewFrom, &a.ContextNote, &impact, &steps, &options, &tags,
		&payload, &evidence, &a.Confidence, &a.Model, &a.InQueue, &a.Status, &decision, &a.ResultText, &a.ExecutedAt, &a.ProposedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return a, storage.ErrNotFound
		}
		return a, err
	}
	_ = json.Unmarshal(impact, &a.Impact)
	_ = json.Unmarshal(steps, &a.Steps)
	_ = json.Unmarshal(options, &a.Options)
	_ = json.Unmarshal(tags, &a.Tags)
	_ = json.Unmarshal(payload, &a.Payload)
	_ = json.Unmarshal(evidence, &a.Evidence)
	if len(decision) > 0 {
		_ = json.Unmarshal(decision, &a.Decision)
	}
	if a.Payload == nil {
		a.Payload = map[string]any{}
	}
	return a, nil
}

// Get loads one action.
func (s *Service) Get(ctx context.Context, id string) (Action, error) {
	return scan(s.DB.Pool.QueryRow(ctx, `SELECT `+cols+from+` WHERE a.id=$1`, id))
}

// Filter for List.
type Filter struct {
	Status    string
	AccountID string
	InQueue   *bool
	Agent     string
	Limit     int
	Branch    string // restrict to accounts of a branch ("" = all)
	UserID    string
}

// List returns actions matching the filter, newest first (excluding historical ones).
func (s *Service) List(ctx context.Context, f Filter) ([]Action, error) {
	q := `SELECT ` + cols + from + ` WHERE a.type <> 'historical'`
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		q += fmt.Sprintf(" AND "+cond, len(args))
	}
	if f.Status != "" {
		if f.Status == "pending" {
			f.Status = domain.ActionProposed
		}
		add("a.status = $%d", f.Status)
	}
	if f.AccountID != "" {
		add("a.account_id = $%d", f.AccountID)
	}
	if f.InQueue != nil {
		add("a.in_queue = $%d", *f.InQueue)
	}
	if f.Agent != "" {
		add("a.agent = $%d", f.Agent)
	}
	if f.Branch != "" {
		args = append(args, f.Branch, f.UserID)
		q += fmt.Sprintf(" AND (acc.branch = $%d OR acc.owner_user_id = $%d OR a.account_id IS NULL)", len(args)-1, len(args))
	}
	q += " ORDER BY a.created_at"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.DB.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Action
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func agentKey(agent string) string { return strings.ToLower(strings.Fields(agent + " x")[0]) }

// Propose stores a new action unless provenance is missing, the pattern is
// suppressed (14 days after a rejection), a learned rule forbids it, or an
// equivalent proposal is already active. Returns the id and whether it was created.
func (s *Service) Propose(ctx context.Context, p Proposal, actor storage.Actor) (string, bool, error) {
	if actor.Type != "user" && len(p.Evidence) == 0 {
		return "", false, errors.New("proposal tanpa provenance tidak disimpan")
	}
	now := domain.Now()
	if p.AccountID != "" {
		var until time.Time
		var fp string
		err := s.DB.Pool.QueryRow(ctx, `SELECT until, signal_fingerprint FROM suppressions WHERE account_id=$1 AND agent=$2 AND type=$3`, p.AccountID, p.Agent, p.Type).Scan(&until, &fp)
		if err == nil && until.After(now) && (p.SignalFingerprint == "" || p.SignalFingerprint == fp) {
			return "", false, nil
		}
	}
	var blocked bool
	_ = s.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM learned_rules WHERE active AND pattern IN ($1, $2))`, agentKey(p.Agent)+":"+p.Type, agentKey(p.Agent)+":"+p.Type+":"+p.AccountID).Scan(&blocked)
	if blocked {
		return "", false, nil
	}
	if p.OpportunityID != "" || p.OnePerAccount {
		var exists bool
		_ = s.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM actions WHERE status IN ('proposed','snoozed') AND type <> 'historical'
			AND ((opportunity_id = NULLIF($1,'') AND agent=$3) OR ($4 AND account_id = NULLIF($2,'') AND agent=$3) OR (account_id = NULLIF($2,'') AND type=$5 AND agent=$3)))`,
			p.OpportunityID, p.AccountID, p.Agent, p.OnePerAccount, p.Type).Scan(&exists)
		if exists {
			return "", false, nil
		}
	}
	if p.ID == "" {
		p.ID = "act-" + storage.Hash(p.Agent, p.Type, p.AccountID, p.OpportunityID, p.Title, now.Format(time.RFC3339Nano))[:16]
	}
	if p.Kind == "" {
		p.Kind = "send"
	}
	if p.Icon == "" {
		p.Icon = "i-send"
	}
	if p.ButtonLabel == "" {
		p.ButtonLabel = "Setujui"
	}
	if p.Confidence == 0 {
		p.Confidence = 0.8
	}
	if p.Payload == nil {
		p.Payload = map[string]any{}
	}
	if p.SignalFingerprint != "" {
		p.Payload["signal_fingerprint"] = p.SignalFingerprint
	}
	proposedBy := "agent"
	if actor.Type == "user" || actor.Type == "machine" {
		proposedBy = actor.ID
	}
	tag, err := s.DB.Pool.Exec(ctx, `INSERT INTO actions(id,agent,type,kind,title,button_label,icon,account_id,opportunity_id,cash_item_id,due_label,due_at,summary,why,prep,preview,preview_from,context_note,
		impact,steps,options,tags,payload,evidence,confidence,model,in_queue,proposed_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28) ON CONFLICT (id) DO NOTHING`,
		p.ID, p.Agent, p.Type, p.Kind, p.Title, p.ButtonLabel, p.Icon, p.AccountID, p.OpportunityID, p.CashItemID, p.DueLabel, p.DueAt, p.Summary, p.Why, p.Prep, p.Preview,
		p.PreviewFrom, p.ContextNote, storage.JSON(p.Impact), storage.JSON(p.Steps), storage.JSON(p.Options), storage.JSON(p.Tags), storage.JSONObj(p.Payload),
		storage.JSON(p.Evidence), p.Confidence, p.Model, p.InQueue, proposedBy)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() == 0 {
		return p.ID, false, nil
	}
	if p.OpportunityID != "" {
		_, _ = s.DB.Pool.Exec(ctx, `UPDATE opportunities SET next_action_id=$1, updated_at=now() WHERE id=$2 AND (next_action_id IS NULL OR next_action_id NOT IN (SELECT id FROM actions WHERE status IN ('proposed','snoozed')))`, p.ID, p.OpportunityID)
	}
	_ = storage.Audit(ctx, s.DB.Pool, actor, "propose", "action", p.ID, map[string]any{"type": p.Type, "agent": p.Agent, "account": p.AccountID})
	if s.Emit != nil {
		s.Emit(ctx, "action.proposed", map[string]any{"id": p.ID, "type": p.Type, "agent": p.Agent, "title": p.Title, "account_id": p.AccountID})
	}
	return p.ID, true, nil
}

func validReason(r string) bool {
	for _, x := range domain.RejectReasons {
		if strings.EqualFold(x, r) {
			return true
		}
	}
	return false
}

// Decide applies a human decision and, for approvals, runs the executor.
// It returns the updated action and the toast text for the UI.
func (s *Service) Decide(ctx context.Context, id string, actor storage.Actor, d Decision) (Action, string, error) {
	if actor.Type != "user" {
		return Action{}, "", ErrHumanOnly
	}
	a, err := s.Get(ctx, id)
	if err != nil {
		return a, "", err
	}
	if a.Status != domain.ActionProposed && a.Status != domain.ActionSnoozed {
		return a, "", fmt.Errorf("%w: saran ini sudah diputuskan (%s)", ErrInvalid, a.Status)
	}
	now := domain.Now()
	rec := func(decision, option, reason string) error {
		_, err := s.DB.Pool.Exec(ctx, `INSERT INTO action_decisions(action_id,agent,type,account_id,user_id,decision,option,reason,note) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9)`,
			a.ID, a.Agent, a.Type, a.AccountID, actor.ID, decision, option, reason, d.Note)
		return err
	}
	decisionJSON := map[string]any{"user": actor.ID, "at": now.Format(time.RFC3339), "decision": d.Decision, "reason": d.Reason, "note": d.Note, "option": d.Option}
	var toast string
	switch d.Decision {
	case "reject":
		if !validReason(d.Reason) {
			return a, "", fmt.Errorf("%w: alasan penolakan wajib dipilih dari daftar", ErrInvalid)
		}
		reason := d.Reason
		if d.Note != "" {
			reason += " — " + d.Note
		}
		if _, err := s.DB.Pool.Exec(ctx, `UPDATE actions SET status='rejected', decision=$2, result_text=$3, updated_at=now() WHERE id=$1`, a.ID, storage.JSONObj(decisionJSON), "Ditolak · "+reason); err != nil {
			return a, "", err
		}
		if err := rec("reject", "", d.Reason); err != nil {
			return a, "", err
		}
		if a.AccountID != "" {
			fp, _ := a.Payload["signal_fingerprint"].(string)
			if _, err := s.DB.Pool.Exec(ctx, `INSERT INTO suppressions(account_id,agent,type,until,reason,signal_fingerprint) VALUES ($1,$2,$3,$4,$5,$6)
				ON CONFLICT (account_id,agent,type) DO UPDATE SET until=EXCLUDED.until, reason=EXCLUDED.reason, signal_fingerprint=EXCLUDED.signal_fingerprint`,
				a.AccountID, a.Agent, a.Type, now.AddDate(0, 0, s.SuppressDays), reason, fp); err != nil {
				return a, "", err
			}
		}
		if err := s.learn(ctx, a, d.Reason); err != nil {
			return a, "", err
		}
		toast = "Ditolak · ARC mencatat alasannya untuk kalibrasi"
	case "snooze":
		until := domain.StartOfDay(now).Add(24*time.Hour + 6*time.Hour + 45*time.Minute)
		if _, err := s.DB.Pool.Exec(ctx, `UPDATE actions SET status='snoozed', snoozed_until=$2, decision=$3, updated_at=now() WHERE id=$1`, a.ID, until, storage.JSONObj(decisionJSON)); err != nil {
			return a, "", err
		}
		if err := rec("snooze", "", ""); err != nil {
			return a, "", err
		}
		toast = "Dilewati · ARC mengingatkan lagi besok pagi"
	case "approve", "edit", "option":
		status := domain.ActionApproved
		if d.Decision == "edit" {
			if strings.TrimSpace(d.Preview) == "" {
				return a, "", fmt.Errorf("%w: draf hasil edit kosong", ErrInvalid)
			}
			status = domain.ActionEdited
			a.Preview = d.Preview
		}
		var opt *Option
		if d.Decision == "option" {
			for i := range a.Options {
				if a.Options[i].Key == d.Option {
					opt = &a.Options[i]
				}
			}
			if opt == nil {
				return a, "", fmt.Errorf("%w: opsi %q tidak dikenal", ErrInvalid, d.Option)
			}
			a.Payload["chosen_option"] = opt.Key
		}
		if _, err := s.DB.Pool.Exec(ctx, `UPDATE actions SET status=$2, preview=$3, decision=$4, payload=$5, updated_at=now() WHERE id=$1`,
			a.ID, status, a.Preview, storage.JSONObj(decisionJSON), storage.JSONObj(a.Payload)); err != nil {
			return a, "", err
		}
		if err := rec(d.Decision, d.Option, ""); err != nil {
			return a, "", err
		}
		a.Status = status
		res := ExecResult{}
		if s.Exec != nil {
			r, err := s.Exec.Execute(ctx, a)
			if err != nil {
				_, _ = s.DB.Pool.Exec(ctx, `UPDATE actions SET execution_log = execution_log || $2::jsonb, updated_at=now() WHERE id=$1`, a.ID, storage.JSON([]any{map[string]any{"at": now, "error": err.Error()}}))
				_ = storage.Audit(ctx, s.DB.Pool, actor, "execute_failed", "action", a.ID, map[string]any{"error": err.Error()})
				return a, "", fmt.Errorf("eksekusi gagal: %w", err)
			}
			res = r
		}
		if opt != nil {
			if opt.Result != "" {
				res.Result = opt.Result
			}
			if opt.Toast != "" {
				res.Toast = opt.Toast
			}
		}
		if res.Result == "" {
			res.Result = a.ResultText
		}
		if res.Result == "" {
			res.Result = "Dijalankan · " + domain.ClockID(now)
		}
		res.Result = strings.ReplaceAll(res.Result, "{time}", domain.ClockID(now))
		if res.Toast == "" {
			if t, ok := a.Payload["toast"].(string); ok && t != "" {
				res.Toast = t
			} else {
				res.Toast = a.Title + " · dijalankan"
			}
		}
		if _, err := s.DB.Pool.Exec(ctx, `UPDATE actions SET status='executed', executed_at=$2, result_text=$3, execution_log = execution_log || $4::jsonb, updated_at=now() WHERE id=$1`,
			a.ID, now, res.Result, storage.JSON([]any{map[string]any{"at": now, "by": actor.ID, "log": res.Log}})); err != nil {
			return a, "", err
		}
		toast = res.Toast
	default:
		return a, "", fmt.Errorf("%w: %q", ErrInvalid, d.Decision)
	}
	_ = storage.Audit(ctx, s.DB.Pool, actor, "decide:"+d.Decision, "action", a.ID, decisionJSON)
	out, err := s.Get(ctx, a.ID)
	return out, toast, err
}

// learn turns three "Tidak sesuai kebijakan" rejections of the same agent+type
// (30 days) into an automatic prohibition and notifies the CEO.
func (s *Service) learn(ctx context.Context, a Action, reason string) error {
	if !strings.EqualFold(reason, "Tidak sesuai kebijakan") {
		return nil
	}
	var n int
	if err := s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM action_decisions WHERE agent=$1 AND type=$2 AND decision='reject' AND reason ILIKE 'Tidak sesuai kebijakan%' AND decided_at >= $3`,
		a.Agent, a.Type, domain.Now().AddDate(0, 0, -30)).Scan(&n); err != nil {
		return err
	}
	if n < 3 {
		return nil
	}
	pattern := agentKey(a.Agent) + ":" + a.Type
	text := fmt.Sprintf("Saran “%s” dari %s dinonaktifkan setelah 3 penolakan “tidak sesuai kebijakan” dalam 30 hari.", typeLabel(a.Type), a.Agent)
	tag, err := s.DB.Pool.Exec(ctx, `INSERT INTO learned_rules(agent,pattern,text) VALUES ($1,$2,$3) ON CONFLICT (pattern) DO NOTHING`, a.Agent, pattern, text)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 && s.Notify != nil {
		s.Notify(ctx, domain.RoleCEO, "ARC mempelajari aturan baru", text)
	}
	return nil
}

// Blocked reports whether a learned rule forbids agent+type (any account).
func (s *Service) Blocked(ctx context.Context, agent, typ string) bool {
	var b bool
	_ = s.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM learned_rules WHERE active AND pattern=$1)`, agentKey(agent)+":"+typ).Scan(&b)
	return b
}

func typeLabel(t string) string {
	labels := map[string]string{"send_wa": "kirim WhatsApp", "send_email": "kirim email", "discount_exception": "diskon", "payment_reminder": "pengingat pembayaran",
		"create_invoice": "buat invoice", "meeting_brief": "meeting prep", "create_opportunity": "buat opportunity"}
	if l, ok := labels[t]; ok {
		return l
	}
	return strings.ReplaceAll(t, "_", " ")
}
