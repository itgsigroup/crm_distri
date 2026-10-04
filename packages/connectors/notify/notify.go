// Package notify delivers briefs and alerts (SMTP account of the ARC system,
// Basecamp message board / to-dos) with a Fake for tests and mock mode.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// Message is one notification.
type Message struct {
	To      []string
	Subject string
	Text    string
	HTML    string
}

// Notifier sends a message on one channel.
type Notifier interface {
	Channel() string
	Send(ctx context.Context, m Message) error
}

// SMTP sends e-mail through the ARC system account.
type SMTP struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

func (s *SMTP) Channel() string { return "email" }

// Send delivers a multipart (text + HTML) e-mail.
func (s *SMTP) Send(_ context.Context, m Message) error {
	boundary := fmt.Sprintf("arc-%d", time.Now().UnixNano())
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n",
		s.From, strings.Join(m.To, ", "), mime.QEncoding.Encode("utf-8", m.Subject), boundary)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", boundary, m.Text)
	if m.HTML != "" {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n", boundary, m.HTML)
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Pass, s.Host)
	}
	from := s.From
	if i := strings.Index(from, "<"); i >= 0 {
		from = strings.Trim(from[i:], "<>")
	}
	return smtp.SendMail(fmt.Sprintf("%s:%d", s.Host, s.Port), auth, from, m.To, b.Bytes())
}

// Basecamp posts to a project's message board and creates to-dos (Basecamp 3 API).
type Basecamp struct {
	Token     string
	AccountID string
	ProjectID string
	HTTP      *http.Client
}

func (bc *Basecamp) Channel() string { return "basecamp" }

func (bc *Basecamp) post(ctx context.Context, path string, body any) (map[string]any, error) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://3.basecampapi.com/%s%s", bc.AccountID, path), bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+bc.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ARC Relationship Core (arc@gsi.co.id)")
	c := bc.HTTP
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("basecamp: HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}

// Send posts a message on the project message board.
func (bc *Basecamp) Send(ctx context.Context, m Message) error {
	_, err := bc.post(ctx, fmt.Sprintf("/buckets/%s/message_boards/messages.json", bc.ProjectID), map[string]any{"subject": m.Subject, "content": m.HTML, "status": "active"})
	return err
}

// CreateTodo creates a to-do in the project's first to-do list.
func (bc *Basecamp) CreateTodo(ctx context.Context, title, assignee string) (string, error) {
	out, err := bc.post(ctx, fmt.Sprintf("/buckets/%s/todos.json", bc.ProjectID), map[string]any{"content": title, "description": "via ARC · untuk " + assignee})
	if err != nil {
		return "", err
	}
	return fmt.Sprint(out["id"]), nil
}

// TodoCreator is implemented by Basecamp and Fake.
type TodoCreator interface {
	CreateTodo(ctx context.Context, title, assignee string) (string, error)
}

// Fake records everything.
type Fake struct {
	mu    sync.Mutex
	Name  string
	Sent  []Message
	Todos []string
}

func (f *Fake) Channel() string {
	if f.Name == "" {
		return "fake"
	}
	return f.Name
}

// Send records the message.
func (f *Fake) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sent = append(f.Sent, m)
	return nil
}

// CreateTodo records a to-do.
func (f *Fake) CreateTodo(_ context.Context, title, assignee string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Todos = append(f.Todos, title+" → "+assignee)
	return fmt.Sprintf("fake-todo-%d", len(f.Todos)), nil
}

// Count returns the number of recorded messages.
func (f *Fake) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Sent)
}
