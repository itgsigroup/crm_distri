package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Tags in group chats (WhatsApp "@Budi"): who the composer can tag, and the tags a send from Chat carries. The text
// holds "@<number>" for each tag, as WhatsApp sends them; Chat shows the name.

// GroupMember is someone the composer can tag in a group.
type GroupMember struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

// groupMembers: the people who wrote in the group and the other GSI numbers in it, each with the name Chat knows,
// never this thread's own number.
func (s *Server) groupMembers(ctx context.Context, t gen.GetThreadRow) ([]GroupMember, error) {
	if deref(t.Kind) != "group" {
		return nil, nil
	}
	own := deref(t.SalesWa)
	seen := map[string]string{}
	rows, err := s.st.Q.GroupMembersSeen(ctx, deref(t.WaJid))
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		seen[wa.Digits(m.Number)] = m.Name
	}
	accs, err := s.st.Q.GroupThreadAccounts(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	for _, a := range accs {
		if n := wa.Digits(deref(a.Account)); n != "" {
			if _, ok := seen[n]; !ok {
				seen[n] = a.AccountLabel
			}
		}
	}
	out := []GroupMember{}
	for n, name := range seen {
		if n == "" || n == own {
			continue
		}
		known, err := s.st.Q.NumberName(ctx, n)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if known != "" {
			name = known
		}
		if strings.TrimSpace(name) == "" {
			name = wa.MaskNumber(n)
		}
		out = append(out, GroupMember{Number: n, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// chatMembers: GET /chat/threads/{id}/members — who the composer offers after "@" (groups only; empty otherwise).
func (s *Server) chatMembers(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	ms, err := s.groupMembers(r.Context(), t)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if ms == nil {
		ms = []GroupMember{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": ms})
}

// sendMentions: the tags of a text sent from Chat to a group ("@<number>" in the text) — kept with the bubble (by
// name) and the numbers WhatsApp tags. Nothing for a one-to-one chat.
func (s *Server) sendMentions(ctx context.Context, t gen.GetThreadRow, text string) (json.RawMessage, []string, error) {
	tokens := wa.MentionTokens(text)
	if deref(t.Kind) != "group" || len(tokens) == 0 {
		return nil, nil, nil
	}
	members, err := s.groupMembers(ctx, t)
	if err != nil {
		return nil, nil, err
	}
	names := map[string]string{}
	for _, m := range members {
		names[m.Number] = m.Name
	}
	var ms []wa.Mention
	var numbers []string
	for _, tok := range tokens {
		name := names[tok]
		if name == "" {
			if name, err = s.st.Q.NumberName(ctx, tok); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, err
			}
		}
		if name == "" {
			name = wa.MaskNumber(tok)
		}
		ms = append(ms, wa.Mention{Token: tok, Number: tok, Name: name})
		numbers = append(numbers, tok)
	}
	raw, _ := json.Marshal(ms)
	return raw, numbers, nil
}
