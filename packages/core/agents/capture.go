package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"arc/packages/core/domain"
	"arc/packages/core/llm"
	"arc/packages/core/prompts"
	"arc/packages/core/storage"
)

// CaptureVersion is bumped whenever the capture prompt or rules change.
const CaptureVersion = 1

// CaptureOutput is the structured result of the Capture agent.
type CaptureOutput struct {
	Summary     string  `json:"summary"`
	Sentiment   float64 `json:"sentiment"`
	Annotations []struct {
		Kind   string `json:"kind"`
		Tone   string `json:"tone"`
		Text   string `json:"text"`
		Action string `json:"action"`
	} `json:"annotations"`
	Commitments []struct {
		Who        string  `json:"who"`
		Text       string  `json:"text"`
		Due        *string `json:"due"`
		Quote      string  `json:"quote"`
		Confidence float64 `json:"confidence"`
	} `json:"commitments"`
	Fulfils []struct {
		Text  string `json:"text"`
		Quote string `json:"quote"`
	} `json:"fulfils"`
	Signals []struct {
		Type       string  `json:"type"`
		Severity   string  `json:"severity"`
		Title      string  `json:"title"`
		Detail     string  `json:"detail"`
		Quote      string  `json:"quote"`
		Confidence float64 `json:"confidence"`
	} `json:"signals"`
	Tasks []struct {
		Text         string `json:"text"`
		AssigneeHint string `json:"assignee_hint"`
		Quote        string `json:"quote"`
	} `json:"tasks"`
	PeopleMentioned []struct {
		Name string `json:"name"`
		Role string `json:"role"`
	} `json:"people_mentioned"`
}

// CaptureInput is the context handed to the provider (and the fake).
type CaptureInput struct {
	InteractionID  int64     `json:"interaction_id"`
	Channel        string    `json:"channel"`
	Direction      string    `json:"direction"`
	ThreadType     string    `json:"thread_type"` // cust | gext | gint | email | meeting
	ThreadName     string    `json:"thread_name"`
	Sender         string    `json:"sender"`
	SenderInternal bool      `json:"sender_internal"`
	Owner          string    `json:"owner"`
	Account        string    `json:"account"`
	Text           string    `json:"text"`
	OccurredAt     time.Time `json:"occurred_at"`
	Recent         []string  `json:"recent"`
}

var captureSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"summary", "sentiment", "annotations", "commitments", "fulfils", "signals", "tasks", "people_mentioned"},
	"properties": map[string]any{
		"summary":     map[string]any{"type": "string"},
		"sentiment":   map[string]any{"type": "number"},
		"annotations": arr(obj([]string{"kind", "tone", "text", "action"}, map[string]any{"kind": str(), "tone": str(), "text": str(), "action": str()})),
		"commitments": arr(obj([]string{"who", "text", "due", "quote", "confidence"}, map[string]any{"who": map[string]any{"type": "string", "enum": []string{"kami", "mereka"}}, "text": str(),
			"due": map[string]any{"type": []string{"string", "null"}}, "quote": str(), "confidence": map[string]any{"type": "number"}})),
		"fulfils":          arr(obj([]string{"text", "quote"}, map[string]any{"text": str(), "quote": str()})),
		"signals":          arr(obj([]string{"type", "severity", "title", "detail", "quote", "confidence"}, map[string]any{"type": str(), "severity": str(), "title": str(), "detail": str(), "quote": str(), "confidence": map[string]any{"type": "number"}})),
		"tasks":            arr(obj([]string{"text", "assignee_hint", "quote"}, map[string]any{"text": str(), "assignee_hint": str(), "quote": str()})),
		"people_mentioned": arr(obj([]string{"name", "role"}, map[string]any{"name": str(), "role": str()})),
	},
}

func str() map[string]any          { return map[string]any{"type": "string"} }
func arr(items any) map[string]any { return map[string]any{"type": "array", "items": items} }
func obj(req []string, props map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": req, "properties": props}
}

// ExtractPending runs Capture on every interaction not yet extracted at the
// current version (hourly job and the 60-second debounce for new messages).
func (a *Agents) ExtractPending(ctx context.Context, limit int) (int, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT id FROM interactions WHERE (NOT extracted OR (extraction_version > 0 AND extraction_version < $1)) AND channel NOT IN ('wa_aggregate','erp_event')
		ORDER BY occurred_at LIMIT $2`, CaptureVersion, limit)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if err := a.ExtractInteraction(ctx, id); err != nil {
			a.logger().Warn("capture failed", "interaction", id, "err", err)
			continue
		}
		n++
	}
	return n, nil
}

// ExtractThread extracts pending interactions of one chat thread (debounced mode).
func (a *Agents) ExtractThread(ctx context.Context, threadID string) (int, error) {
	rows, err := a.DB.Pool.Query(ctx, `SELECT id FROM interactions WHERE thread_id=$1 AND (NOT extracted OR (extraction_version > 0 AND extraction_version < $2)) ORDER BY occurred_at`, threadID, CaptureVersion)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := a.ExtractInteraction(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// ExtractInteraction analyses one interaction and persists annotations, commitments,
// signals and tasks. Re-running at the same version is a no-op (unique keys).
func (a *Agents) ExtractInteraction(ctx context.Context, id int64) error {
	var in CaptureInput
	var accountID, oppID, threadID, groupID string
	var groupRead *bool
	err := a.DB.Pool.QueryRow(ctx, `SELECT i.id, i.channel, i.direction, COALESCE(t.type, CASE WHEN g.type='internal' THEN 'gint' WHEN g.type='external' THEN 'gext' ELSE '' END, ''),
			COALESCE(t.name, g.name, i.subject, ''), i.sender_name, i.sender_internal, COALESCE(u.name,''), COALESCE(acc.name,''), i.body_text, i.occurred_at,
			COALESCE(i.account_id,''), COALESCE(i.opportunity_id,''), COALESCE(i.thread_id,''), COALESCE(i.group_id,''), g.read_policy
		FROM interactions i LEFT JOIN chat_threads t ON t.id=i.thread_id LEFT JOIN chat_groups g ON g.id=COALESCE(i.group_id, t.group_id)
		LEFT JOIN accounts acc ON acc.id=i.account_id LEFT JOIN users u ON u.id = i.user_ids[1]
		WHERE i.id=$1`, id).Scan(&in.InteractionID, &in.Channel, &in.Direction, &in.ThreadType, &in.ThreadName, &in.Sender, &in.SenderInternal, &in.Owner,
		&in.Account, &in.Text, &in.OccurredAt, &accountID, &oppID, &threadID, &groupID, &groupRead)
	if err != nil {
		return err
	}
	if in.ThreadType == "" {
		in.ThreadType = map[string]string{"email": "email", "meeting": "meeting", "call": "meeting", "document": "email", "form": "email", "note": "email"}[in.Channel]
	}
	mark := func() error {
		_, err := a.DB.Pool.Exec(ctx, `UPDATE interactions SET extracted=true, extraction_version=$2, updated_at=now() WHERE id=$1`, id, CaptureVersion)
		return err
	}
	// Privacy: internal 1:1 chats are never analysed; non-opt-in groups are never stored.
	if in.ThreadType == "internal" || strings.TrimSpace(in.Text) == "" || (groupRead != nil && !*groupRead) {
		return mark()
	}
	if threadID != "" {
		rows, err := a.DB.Pool.Query(ctx, `SELECT sender_name || ': ' || body_text FROM interactions WHERE thread_id=$1 AND occurred_at < $2 ORDER BY occurred_at DESC LIMIT 5`, threadID, in.OccurredAt)
		if err == nil {
			for rows.Next() {
				var s string
				_ = rows.Scan(&s)
				in.Recent = append([]string{s}, in.Recent...)
			}
			rows.Close()
		}
	}
	ctxJSON, _ := json.Marshal(in)
	var out CaptureOutput
	resp, err := a.LLM.CompleteJSON(ctx, llm.Request{Tier: llm.Light, Purpose: "capture", System: prompts.Get("capture/v1"),
		Messages: []llm.Message{{Role: "user", Content: string(ctxJSON)}}, Schema: captureSchema, MaxTokens: 4000, FakeInput: in}, &out)
	if err != nil {
		return err
	}
	model := resp.Model
	internalOnly := in.ThreadType == "gint"
	ev := func(quote string) []domain.Evidence {
		if quote == "" {
			quote = trunc(in.Text, 200)
		}
		return []domain.Evidence{{InteractionID: id, Quote: trunc(quote, 200), At: in.OccurredAt.Format(time.RFC3339)}}
	}
	return a.DB.Tx(ctx, func(tx pgxTx) error {
		for _, an := range out.Annotations {
			if internalOnly && an.Kind != "task" && an.Kind != "note" {
				continue
			}
			if an.Text == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO extractions(interaction_id,kind,tone,text,action_label,evidence,confidence,model,prompt_version,version)
				VALUES ($1,$2,$3,$4,$5,$6,0.85,$7,'capture/v1',$8) ON CONFLICT DO NOTHING`,
				id, validKind(an.Kind), defaultStr(an.Tone, "accent"), an.Text, an.Action, storage.JSON(ev("")), model, CaptureVersion); err != nil {
				return err
			}
		}
		if !internalOnly && accountID != "" {
			for _, c := range out.Commitments {
				if c.Quote == "" || c.Confidence <= 0 {
					continue // provenance required
				}
				due := resolveDue(c.Due, c.Text, in.OccurredAt)
				hash := storage.Hash(accountID, c.Who, strings.ToLower(strings.TrimSpace(c.Text)), weekKey(due))
				owner := any(nil)
				_ = tx.QueryRow(ctx, `SELECT owner_user_id FROM accounts WHERE id=$1`, accountID).Scan(&owner)
				if _, err := tx.Exec(ctx, `INSERT INTO commitments(id,dedupe_hash,account_id,opportunity_id,who,text,detail,due_at,status,origin_interaction_id,owner_user_id,evidence,confidence,model)
					VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,'open',$9,$10,$11,$12,$13)
					ON CONFLICT (dedupe_hash) DO UPDATE SET evidence=EXCLUDED.evidence, confidence=GREATEST(commitments.confidence, EXCLUDED.confidence), updated_at=now()`,
					"cm-"+hash[:16], hash, accountID, oppID, c.Who, c.Text, commitmentDetail(in, c.Who), due, id, owner, storage.JSON(ev(c.Quote)), c.Confidence, model); err != nil {
					return err
				}
			}
			for _, f := range out.Fulfils {
				if f.Quote == "" {
					continue
				}
				if _, err := tx.Exec(ctx, `UPDATE commitments SET status='done', resolved_by_interaction_id=$1, updated_at=now()
					WHERE account_id=$2 AND status IN ('open','late') AND (text ILIKE '%'||$3||'%' OR $3 ILIKE '%'||text||'%')`, id, accountID, f.Text); err != nil {
					return err
				}
			}
			for _, sg := range out.Signals {
				if sg.Quote == "" || sg.Type == "" {
					continue
				}
				key := sg.Type + ":" + firstNonEmpty(oppID, accountID)
				if _, err := tx.Exec(ctx, `INSERT INTO signals(type,severity,account_id,opportunity_id,title,detail,headline,evidence,confidence,dedupe_key,detected_at)
					VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (dedupe_key) DO UPDATE SET evidence=EXCLUDED.evidence, detected_at=EXCLUDED.detected_at, resolved_at=NULL, updated_at=now()`,
					sg.Type, defaultStr(sg.Severity, "warn"), accountID, oppID, sg.Title, sg.Detail, in.Account+" · "+channelLabel(in)+", "+domain.ShortDate(in.OccurredAt),
					storage.JSON(ev(sg.Quote)), sg.Confidence, key, domain.Now()); err != nil {
					return err
				}
			}
		}
		for _, t := range out.Tasks {
			if t.Text == "" {
				continue
			}
			var acc any
			if accountID != "" {
				acc = accountID
			}
			if _, err := tx.Exec(ctx, `INSERT INTO tasks(id,title,assignee,source_interaction_id,source_label,group_id,account_id,status,dedupe_hash)
				VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,'detected',$8) ON CONFLICT (dedupe_hash) DO NOTHING`,
				"task-"+storage.Hash(groupID, threadID, t.Text)[:12], t.Text, t.AssigneeHint, id, "pesan "+domain.ClockID(in.OccurredAt), groupID, acc, storage.Hash(groupID+threadID, t.Text)); err != nil {
				return err
			}
		}
		sent := any(nil)
		if !internalOnly {
			sent = out.Sentiment
		}
		if _, err := tx.Exec(ctx, `UPDATE interactions SET extracted=true, extraction_version=$2, summary=$3, sentiment=$4, updated_at=now() WHERE id=$1`, id, CaptureVersion, out.Summary, sent); err != nil {
			return err
		}
		return nil
	})
}

func commitmentDetail(in CaptureInput, who string) string {
	if who == "kami" {
		return fmt.Sprintf("%s · dijanjikan di %s %s", defaultStr(in.Owner, in.Sender), channelLabel(in), domain.ShortDate(in.OccurredAt))
	}
	return fmt.Sprintf("Dijanjikan %s · %s", in.Sender, domain.ShortDate(in.OccurredAt))
}

func channelLabel(in CaptureInput) string {
	switch in.Channel {
	case "email":
		return "email"
	case "meeting":
		return "meeting"
	case "wa_group_message":
		return "grup WhatsApp"
	}
	return "WhatsApp"
}

func validKind(k string) string {
	switch k {
	case "commitment", "signal", "task", "milestone", "sentiment", "note", "risk", "stage":
		return k
	}
	return "note"
}

func defaultStr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func weekKey(t *time.Time) string {
	if t == nil {
		return ""
	}
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-%02d", y, w)
}

var dayNames = map[string]time.Weekday{"minggu": time.Sunday, "senin": time.Monday, "selasa": time.Tuesday, "rabu": time.Wednesday, "kamis": time.Thursday, "jumat": time.Friday, "sabtu": time.Saturday}
var reDayWord = regexp.MustCompile(`(?i)\b(senin|selasa|rabu|kamis|jumat|sabtu|minggu depan|besok|hari ini|lusa)\b`)
var reDate = regexp.MustCompile(`(?i)\b(\d{1,2})\s+(jan|feb|mar|apr|mei|jun|jul|agu|sep|okt|nov|des)\b`)

// resolveDue turns "2026-10-01" or a day phrase into a deadline relative to the message.
func resolveDue(due *string, text string, at time.Time) *time.Time {
	if due != nil && *due != "" {
		if t, err := time.ParseInLocation("2006-01-02", *due, domain.Jakarta); err == nil {
			t = t.Add(17 * time.Hour)
			return &t
		}
	}
	base := domain.StartOfDay(at).Add(17 * time.Hour)
	if m := reDate.FindStringSubmatch(text); m != nil {
		var d int
		fmt.Sscanf(m[1], "%d", &d)
		months := map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "mei": 5, "jun": 6, "jul": 7, "agu": 8, "sep": 9, "okt": 10, "nov": 11, "des": 12}
		t := time.Date(at.Year(), time.Month(months[strings.ToLower(m[2])]), d, 17, 0, 0, 0, domain.Jakarta)
		if t.Before(at.AddDate(0, -2, 0)) {
			t = t.AddDate(1, 0, 0)
		}
		return &t
	}
	m := reDayWord.FindString(strings.ToLower(text))
	switch m {
	case "":
		return nil
	case "hari ini":
		return &base
	case "besok":
		t := base.AddDate(0, 0, 1)
		return &t
	case "lusa":
		t := base.AddDate(0, 0, 2)
		return &t
	case "minggu depan":
		t := base.AddDate(0, 0, 7)
		return &t
	}
	wd := dayNames[m]
	diff := (int(wd) - int(at.In(domain.Jakarta).Weekday()) + 7) % 7
	if diff == 0 {
		diff = 7
	}
	t := base.AddDate(0, 0, diff)
	return &t
}
