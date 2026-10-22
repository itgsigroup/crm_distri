package api

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"distri-arc/internal/httpx"
	"distri-arc/internal/llm"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Cari di dalam chat (like WhatsApp's search in a conversation): the messages holding a word or a sentence, newest
// first, with the text around the match. The page jumps to one through GET /chat/threads/{id}?from=…&before=….

// ChatHit is one message that holds the words.
type ChatHit struct {
	ID        uuid.UUID `json:"id"`
	SentAt    time.Time `json:"sent_at"`
	Direction string    `json:"direction"`
	FromName  string    `json:"from_name"`
	Snippet   string    `json:"snippet"`
}

// maxChatHits caps one search (WhatsApp shows the newest first; a busier word is narrowed by typing more).
const maxChatHits = 300

func (s *Server) chatSearch(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	q := strings.Join(strings.Fields(r.URL.Query().Get("q")), " ")
	if utf8.RuneCountInString(q) < 2 {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": []ChatHit{}, "q": q})
		return
	}
	rows, err := s.st.Q.SearchThreadMessages(r.Context(), gen.SearchThreadMessagesParams{ThreadID: t.ID, Pattern: searchPattern(q), Lim: maxChatHits})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]ChatHit, 0, len(rows))
	for _, m := range rows {
		out = append(out, ChatHit{ID: m.ID, SentAt: m.SentAt, Direction: m.Direction, FromName: m.FromName, Snippet: snippet(m.Body, q, 48)})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "q": q, "capped": len(rows) == maxChatHits})
}

// MessageHit is one message found by Cari chat across every conversation, with the conversation it is in.
type MessageHit struct {
	ChatHit
	ThreadID  uuid.UUID `json:"thread_id"`
	Kind      string    `json:"kind"`
	GroupKind string    `json:"group_kind,omitempty"`
	Title     string    `json:"title"`
	PhoneName string    `json:"phone_name,omitempty"`
}

// maxMessageHits caps the "Pesan" results of Cari chat; the query reads more so the conversations a viewer may
// not read (another sales' number) do not empty the list.
const maxMessageHits = 60

// searchMessages is Cari chat over the text of every stored message (Sam 22 Oct: like WhatsApp, which finds the
// chats by name and the messages by their words): newest first, only conversations the viewer reads, of one number
// (account) or one team's numbers (team) when asked.
func (s *Server) searchMessages(w http.ResponseWriter, r *http.Request) {
	q := strings.Join(strings.Fields(r.URL.Query().Get("q")), " ")
	if utf8.RuneCountInString(q) < 2 {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": []MessageHit{}, "q": q})
		return
	}
	rows, err := s.st.Q.SearchAllMessages(r.Context(), gen.SearchAllMessagesParams{Pattern: searchPattern(q), Lim: maxMessageHits * 5})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	u, _ := CurrentUser(r.Context())
	viewer, err := s.chatViewerOf(r.Context(), u)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	account, team := wa.Digits(r.URL.Query().Get("account")), r.URL.Query().Get("team")
	out := []MessageHit{}
	for _, m := range rows {
		if m.ThreadID == nil || !viewer.sees(m.SalesID, m.Account) ||
			(account != "" && deref(m.Account) != account) || (team != "" && m.AccountTeam != team) {
			continue
		}
		h := MessageHit{ChatHit: ChatHit{ID: m.ID, SentAt: m.SentAt, Direction: m.Direction, FromName: m.FromName, Snippet: llm.MaskCredentials(snippet(m.Body, q, 40))},
			ThreadID: *m.ThreadID, Kind: m.Kind, Title: m.Title}
		if m.Kind == "group" {
			h.GroupKind = m.GroupKind
		} else {
			h.PhoneName = m.PhoneName
		}
		if out = append(out, h); len(out) == maxMessageHits {
			break
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "q": q, "capped": len(out) == maxMessageHits})
}

// searchPattern: the typed words as a case-insensitive regular expression, each word literal, any whitespace between.
func searchPattern(q string) string {
	words := strings.Fields(q)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	return strings.Join(words, `\s+`)
}

// snippet is the text around the first match of q (case-insensitive), about `around` characters on each side, on
// one line, with "…" where it was cut.
func snippet(body, q string, around int) string {
	text := []rune(strings.Join(strings.Fields(body), " "))
	lower, ql := []rune(strings.ToLower(string(text))), []rune(strings.ToLower(q))
	at := -1
	if len(lower) == len(text) { // lowercasing kept the length (rune by rune): positions line up
		for i := 0; i+len(ql) <= len(lower); i++ {
			if string(lower[i:i+len(ql)]) == string(ql) {
				at = i
				break
			}
		}
	}
	if at < 0 {
		at = 0
	}
	from, to := max(0, at-around), min(len(text), at+len(ql)+around)
	out := string(text[from:to])
	if from > 0 {
		out = "…" + out
	}
	if to < len(text) {
		out += "…"
	}
	return out
}
