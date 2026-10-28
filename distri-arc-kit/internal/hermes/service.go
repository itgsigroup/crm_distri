package hermes

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"distri-arc/internal/auth"
)

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
	// Agent replaces the configured client (tests); nil = the sealed URL + key, or none (demo card)
	Agent Agent
}

// ErrBusy means the user's previous question is still being answered.
var ErrBusy = errors.New("lyra masih menjawab pertanyaan sebelumnya")

// ErrNotConnected means no Hermes URL and key are saved: the page has no Lyra to ask (there is no demo chat).
var ErrNotConnected = errors.New("lyra belum disambungkan ke GSI Orbit — CEO mengisi URL dan API key di tab Sambungan")

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

// agent is the configured Hermes; demo = true when none is saved.
func (s Service) agent(ctx context.Context) (Agent, bool) {
	if s.Agent != nil {
		return s.Agent, false
	}
	u, err1 := s.secret(ctx, SecretURL)
	k, err2 := s.secret(ctx, SecretKey)
	if err1 != nil || err2 != nil || u == "" || k == "" {
		return nil, true
	}
	return Client{BaseURL: u, Key: k}, false
}

// window is the configured Lyra for the Hermes page.
func (s Service) window(ctx context.Context) (Window, error) {
	u, err1 := s.secret(ctx, SecretURL)
	k, err2 := s.secret(ctx, SecretKey)
	if err1 != nil || err2 != nil || u == "" || k == "" {
		return nil, ErrNotConnected
	}
	return Client{BaseURL: u, Key: k}, nil
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

// Ask stores the question and a pending reply, then passes the question to Lyra in the background (an agent run with
// tools can outlast the reverse proxy's timeout, so the page polls for the reply). Orbit adds no prompt and no
// history: Lyra keeps the conversation, named after the user's first stored message so Hapus riwayat starts anew.
func (s Service) Ask(ctx context.Context, userID uuid.UUID, text string) (Message, error) {
	lyra, err := s.window(ctx)
	if err != nil {
		return Message{}, err
	}
	var busy bool
	_ = s.Pool.QueryRow(ctx, `select exists(select 1 from hermes_messages where user_id = $1 and status = 'pending' and created_at > now() - $2::interval)`,
		userID, staleAfter.String()).Scan(&busy)
	if busy {
		return Message{}, ErrBusy
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Message{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var q Message
	if err := tx.QueryRow(ctx, `insert into hermes_messages (user_id, role, status, content, finished_at) values ($1, 'user', 'done', $2, now())
		returning id, role, status, content, demo, created_at, finished_at`, userID, text).
		Scan(&q.ID, &q.Role, &q.Status, &q.Content, &q.Demo, &q.CreatedAt, &q.FinishedAt); err != nil {
		return Message{}, err
	}
	var replyID uuid.UUID
	if err := tx.QueryRow(ctx, `insert into hermes_messages (user_id, role, status, created_at) values ($1, 'assistant', 'pending', $2) returning id`,
		userID, q.CreatedAt).Scan(&replyID); err != nil {
		return Message{}, err
	}
	var first uuid.UUID
	if err := tx.QueryRow(ctx, `select id from hermes_messages where user_id = $1 order by created_at, role desc, id limit 1`, userID).Scan(&first); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, err
	}
	go s.answer(replyID, lyra, "orbit-user-"+userID.String(), "orbit-"+first.String(), text)
	return q, nil
}

func (s Service) answer(replyID uuid.UUID, lyra Window, sessionKey, conversation, question string) {
	ctx, cancel := context.WithTimeout(context.Background(), staleAfter)
	defer cancel()
	start := time.Now()
	text, err := lyra.Respond(ctx, sessionKey, conversation, question)
	status := "done"
	if err != nil {
		status, text = "error", err.Error()
	}
	if s.Log != nil {
		s.Log.Info("hermes chat", "purpose", "hermes.chat", "status", status, "conversation", conversation, "ms", time.Since(start).Milliseconds())
	}
	if _, err := s.Pool.Exec(context.Background(), `update hermes_messages set status = $2, content = $3, finished_at = now() where id = $1 and status = 'pending'`,
		replyID, status, text); err != nil && s.Log != nil {
		s.Log.Error("hermes chat: store reply", "err", err)
	}
}

// Clear deletes the user's conversation (Hapus riwayat); the next question opens a new conversation on Lyra.
func (s Service) Clear(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, "delete from hermes_messages where user_id = $1", userID)
	return err
}
