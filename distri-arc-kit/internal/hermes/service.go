package hermes

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"distri-arc/internal/auth"
)

// historyTurns is how many earlier turns are sent with each question (Hermes' Chat Completions is stateless).
const historyTurns = 30

// staleAfter: a reply still pending this long (API restarted mid-run) is shown as failed.
const staleAfter = 10 * time.Minute

// Message is one stored chat bubble.
type Message struct {
	ID         uuid.UUID  `json:"id"`
	Role       string     `json:"role"`   // user | assistant
	Status     string     `json:"status"` // pending | done | error
	Content    string     `json:"content"`
	Demo       bool       `json:"demo"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// Service stores conversations and asks Hermes.
type Service struct {
	Pool       *pgxpool.Pool
	SessionKey []byte
	Log        *slog.Logger
	// Agent replaces the configured client (tests); nil = the sealed URL + key, or Fake when none
	Agent Agent
}

// ErrBusy means the user's previous question is still being answered.
var ErrBusy = errors.New("hermes masih menjawab pertanyaan sebelumnya")

func (s Service) secret(ctx context.Context, key string) (string, error) {
	var sealed string
	if err := s.Pool.QueryRow(ctx, "select value from secrets where key = $1", key).Scan(&sealed); err != nil {
		return "", err
	}
	return auth.Open(sealed, s.SessionKey)
}

// Config is what the settings tab shows (never the key itself).
type Config struct {
	Configured bool   `json:"configured"`
	URL        string `json:"url"`
	KeyHint    string `json:"key_hint"`
}

// KeyHint shows the last 4 characters of a key.
func KeyHint(key string) string {
	if len(key) <= 4 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}

// LoadConfig reads the sealed URL and key.
func (s Service) LoadConfig(ctx context.Context) Config {
	u, err1 := s.secret(ctx, SecretURL)
	k, err2 := s.secret(ctx, SecretKey)
	if err1 != nil || err2 != nil || u == "" || k == "" {
		return Config{}
	}
	return Config{Configured: true, URL: u, KeyHint: KeyHint(k)}
}

// agent is the configured Hermes, or Fake (demo = true).
func (s Service) agent(ctx context.Context) (Agent, bool) {
	if s.Agent != nil {
		return s.Agent, false
	}
	u, err1 := s.secret(ctx, SecretURL)
	k, err2 := s.secret(ctx, SecretKey)
	if err1 != nil || err2 != nil || u == "" || k == "" {
		return Fake{}, true
	}
	return Client{BaseURL: u, Key: k}, false
}

// List returns the user's conversation, oldest first (last 200).
func (s Service) List(ctx context.Context, userID uuid.UUID) ([]Message, error) {
	_, _ = s.Pool.Exec(ctx, `update hermes_messages set status = 'error', content = 'Jawaban terputus (server dimulai ulang). Kirim ulang pertanyaannya.', finished_at = now()
		where user_id = $1 and status = 'pending' and created_at < now() - $2::interval`, userID, staleAfter.String())
	rows, err := s.Pool.Query(ctx, `select id, role, status, content, demo, created_at, finished_at from (
		select * from hermes_messages where user_id = $1 order by created_at desc, role desc limit 200) m
		order by created_at, role desc`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Message, error) {
		var m Message
		err := r.Scan(&m.ID, &m.Role, &m.Status, &m.Content, &m.Demo, &m.CreatedAt, &m.FinishedAt)
		return m, err
	})
}

// Ask stores the question and a pending reply, then answers in the background (an agent run with MCP tools can
// outlast the reverse proxy's timeout, so the page polls for the reply). userName goes into the system prompt.
func (s Service) Ask(ctx context.Context, userID uuid.UUID, userName, text string) (Message, error) {
	var busy bool
	_ = s.Pool.QueryRow(ctx, `select exists(select 1 from hermes_messages where user_id = $1 and status = 'pending' and created_at > now() - $2::interval)`,
		userID, staleAfter.String()).Scan(&busy)
	if busy {
		return Message{}, ErrBusy
	}
	history, err := s.history(ctx, userID)
	if err != nil {
		return Message{}, err
	}
	agent, demo := s.agent(ctx)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var q Message
	if err := tx.QueryRow(ctx, `insert into hermes_messages (user_id, role, status, content, demo, finished_at) values ($1, 'user', 'done', $2, $3, now())
		returning id, role, status, content, demo, created_at, finished_at`, userID, text, demo).
		Scan(&q.ID, &q.Role, &q.Status, &q.Content, &q.Demo, &q.CreatedAt, &q.FinishedAt); err != nil {
		return Message{}, err
	}
	var replyID uuid.UUID
	if err := tx.QueryRow(ctx, `insert into hermes_messages (user_id, role, status, demo, created_at) values ($1, 'assistant', 'pending', $2, $3) returning id`,
		userID, demo, q.CreatedAt).Scan(&replyID); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	msgs := append([]Msg{{Role: "system", Content: systemPrompt(userName)}}, history...)
	msgs = append(msgs, Msg{Role: "user", Content: text})
	go s.answer(replyID, agent, "orbit-user-"+userID.String(), msgs)
	return q, nil
}

func (s Service) answer(replyID uuid.UUID, agent Agent, sessionKey string, msgs []Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), staleAfter)
	defer cancel()
	start := time.Now()
	text, err := agent.Chat(ctx, sessionKey, msgs)
	status := "done"
	if err != nil {
		status, text = "error", err.Error()
	}
	if s.Log != nil {
		s.Log.Info("hermes chat", "purpose", "hermes.chat", "status", status, "turns", len(msgs), "ms", time.Since(start).Milliseconds())
	}
	if _, err := s.Pool.Exec(context.Background(), `update hermes_messages set status = $2, content = $3, finished_at = now() where id = $1 and status = 'pending'`,
		replyID, status, text); err != nil && s.Log != nil {
		s.Log.Error("hermes chat: store reply", "err", err)
	}
}

// history is the user's last answered turns, oldest first; failed replies and demo answers are left out.
func (s Service) history(ctx context.Context, userID uuid.UUID) ([]Msg, error) {
	rows, err := s.Pool.Query(ctx, `select role, content from (
		select role, content, created_at from hermes_messages where user_id = $1 and status = 'done' and not demo
		order by created_at desc, role desc limit $2) m order by created_at, role desc`, userID, historyTurns)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Msg, error) {
		var m Msg
		err := r.Scan(&m.Role, &m.Content)
		return m, err
	})
}

// Clear deletes the user's conversation (Hapus riwayat).
func (s Service) Clear(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, "delete from hermes_messages where user_id = $1", userID)
	return err
}

func systemPrompt(userName string) string {
	name := strings.TrimSpace(userName)
	if name == "" {
		name = "pengguna"
	}
	return "Anda dihubungi dari halaman Hermes di aplikasi GSI Orbit (CRM distribusi PT Gosyen Solusi Indonesia) oleh " + name + ". " +
		"Jawab dalam Bahasa Indonesia, ringkas dan jelas. Untuk data dealer, penjualan, piutang, dan stok pakai tool MCP distri_crm. " +
		"Anda hanya membaca dan mengusulkan: keputusan dan pengiriman ke dealer dilakukan manusia di GSI Orbit."
}
