package analyst

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"distri-arc/internal/auth"
	"distri-arc/internal/store/gen"
)

// Claude routine (ADR 0049): a cycle that waits for Claude through MCP fires a routine on claude.ai through its API
// trigger. The routine runs Claude in Anthropic's cloud on the Claude subscription, with the CRM-DISTRI connector — no
// Claude Desktop to open, no API key, no ChatGPT. The routine id and its trigger token live sealed in secrets.

const (
	SecretRoutineID    = "claude.routine_id"
	SecretRoutineToken = "claude.routine_token"
)

// routineBase is the routines API root (tests point it at a fake server).
var routineBase = "https://api.anthropic.com"

// SetRoutineBase points FireRoutine at another server (tests).
func SetRoutineBase(u string) func() {
	old := routineBase
	routineBase = u
	return func() { routineBase = old }
}

var routineIDRe = regexp.MustCompile(`^trig_[A-Za-z0-9]{8,64}$`)

// ValidRoutine checks the shape of a routine id (trig_…) and its trigger token (sk-ant-oat…).
func ValidRoutine(id, token string) error {
	if !routineIDRe.MatchString(id) {
		return errors.New("ID routine diawali trig_ (lihat alamat routine di claude.ai/code/routines)")
	}
	if !strings.HasPrefix(token, "sk-ant-oat") || len(token) < 24 || strings.ContainsAny(token, " \n\t") {
		return errors.New("token routine diawali sk-ant-oat (Edit routine → trigger API → Generate token)")
	}
	return nil
}

// LoadRoutine returns the routine id and trigger token, "" when not set.
func LoadRoutine(ctx context.Context, q *gen.Queries, sessionKey []byte) (id, token string) {
	open := func(key string) string {
		sealed, err := q.GetSecret(ctx, key)
		if err != nil {
			return ""
		}
		v, err := auth.Open(sealed, sessionKey)
		if err != nil {
			return ""
		}
		return v
	}
	id, token = open(SecretRoutineID), open(SecretRoutineToken)
	if id == "" || token == "" {
		return "", ""
	}
	return id, token
}

// SaveRoutine seals and stores the routine id and token.
func SaveRoutine(ctx context.Context, q *gen.Queries, id, token string, sessionKey []byte, by *string) error {
	for key, v := range map[string]string{SecretRoutineID: id, SecretRoutineToken: token} {
		sealed, err := auth.Seal(v, sessionKey)
		if err != nil {
			return err
		}
		if err := q.SetSecret(ctx, gen.SetSecretParams{Key: key, Value: sealed, UpdatedBy: by}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteRoutine forgets the routine.
func DeleteRoutine(ctx context.Context, q *gen.Queries) {
	_ = q.DeleteSecret(ctx, SecretRoutineID)
	_ = q.DeleteSecret(ctx, SecretRoutineToken)
}

// RoutineHint shows the routine without its token: "trig_01AB…wxyz".
func RoutineHint(id string) string {
	if len(id) <= 14 {
		return id
	}
	return id[:9] + "…" + id[len(id)-4:]
}

// FireRoutine starts one run of the routine with text as its payload and returns the run's claude.ai link. Each call
// is a new run (no idempotency key): callers fire once per cycle.
func FireRoutine(ctx context.Context, id, token, text string) (string, error) {
	body, _ := json.Marshal(map[string]string{"text": text})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, routineBase+"/v1/claude_code/routines/"+id+"/fire", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("routine Claude tidak bisa dihubungi: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var out struct {
		URL   string `json:"claude_code_session_url"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	if res.StatusCode != http.StatusOK {
		msg := out.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		switch res.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", fmt.Errorf("token routine Claude ditolak (%d) — buat token baru di claude.ai/code/routines: %s", res.StatusCode, msg)
		case http.StatusTooManyRequests:
			return "", fmt.Errorf("routine Claude sedang dibatasi (429) — coba lagi nanti: %s", msg)
		}
		return "", fmt.Errorf("routine Claude gagal dijalankan (%d): %s", res.StatusCode, msg)
	}
	return out.URL, nil
}
