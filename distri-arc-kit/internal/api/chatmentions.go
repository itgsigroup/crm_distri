package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Tags in group chats (WhatsApp "@Budi"): who the composer can tag, and the tags a send from Chat carries. The text
// holds "@<number>" for each tag, as WhatsApp sends them; Chat shows the name.

// GroupMember is a member of a group as Chat shows it: tagged from the composer, listed under "Anggota", and a click
// opens the one-to-one chat with them (ThreadID when this GSI number already has one).
type GroupMember struct {
	Number   string `json:"number"`
	Name     string `json:"name"`
	Admin    bool   `json:"admin,omitempty"`
	GSI      bool   `json:"gsi,omitempty"` // a GSI number (employee)
	ThreadID string `json:"thread_id,omitempty"`
}

// groupMembers: the group's members from WhatsApp (through this thread's number), the people who wrote in it and the
// other GSI numbers in it, each with the name Chat knows; never this thread's own number. hidden: members whose
// number WhatsApp hides (they cannot be tagged or opened). Without the bridge, only who Chat has seen.
func (s *Server) groupMembers(ctx context.Context, t gen.GetThreadRow) (out []GroupMember, hidden int, err error) {
	if deref(t.Kind) != "group" {
		return nil, 0, nil
	}
	own, jid := wa.Digits(deref(t.SalesWa)), deref(t.WaJid)
	byNum := map[string]*GroupMember{}
	add := func(n, name string, admin bool) {
		if n = wa.Digits(n); n == "" || n == own {
			return
		}
		m := byNum[n]
		if m == nil {
			m = &GroupMember{Number: n}
			byNum[n] = m
		}
		if m.Name == "" {
			m.Name = strings.TrimSpace(name)
		}
		m.Admin = m.Admin || admin
	}
	// names: the contact saved on this number's phone, else the member's WhatsApp profile name (the roster), else the
	// profile name they wrote with, else the master data's name, else the masked number (Sam 16 Oct)
	for _, m := range s.rosterOf(ctx, own, jid) {
		if m.Number == "" {
			hidden++
			continue
		}
		add(m.Number, m.Name, m.Admin)
	}
	rows, err := s.st.Q.GroupMembersSeen(ctx, jid)
	if err != nil {
		return nil, 0, err
	}
	for _, m := range rows {
		add(m.Number, m.Name, false)
	}
	accs, err := s.st.Q.GroupThreadAccounts(ctx, t.ID)
	if err != nil {
		return nil, 0, err
	}
	for _, a := range accs {
		add(deref(a.Account), "", false)
	}
	jids := make([]string, 0, len(byNum))
	for n, m := range byNum {
		jids = append(jids, wa.UserJID(n))
		if m.Name == "" {
			if m.Name, err = s.st.Q.NumberName(ctx, n); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, 0, err
			}
		}
		if m.Name == "" {
			m.Name = wa.MaskNumber(n)
		}
		outsiders, err := s.st.Q.CountOutsiders(ctx, []string{n})
		if err != nil {
			return nil, 0, err
		}
		m.GSI = outsiders == 0
	}
	threads, err := s.st.Q.DirectThreadsOf(ctx, gen.DirectThreadsOfParams{Account: own, Jids: jids})
	if err != nil {
		return nil, 0, err
	}
	for _, th := range threads {
		if m := byNum[wa.Digits(th.Number)]; m != nil {
			m.ThreadID = th.ID.String()
		}
	}
	out = make([]GroupMember, 0, len(byNum))
	for _, m := range byNum {
		out = append(out, *m)
	}
	// admins first, then by name, like WhatsApp's group info
	sort.Slice(out, func(i, j int) bool {
		if out[i].Admin != out[j].Admin {
			return out[i].Admin
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, hidden, nil
}

// rosterEntry is a group's member list as one GSI number's phone sees it.
type rosterEntry struct {
	at time.Time
	ms []wa.RosterMember
}

// rosterTTL: how long a group's member list is reused (the bridge keeps its own for ten minutes).
const rosterTTL = 5 * time.Minute

// rosterOf is a group's members through a GSI number in it — names as that phone knows them — kept for rosterTTL;
// nil without the bridge, or when it cannot be reached (the error is logged, an older list is kept).
func (s *Server) rosterOf(ctx context.Context, account, groupJID string) []wa.RosterMember {
	r := s.groupRoster()
	if r == nil || account == "" {
		return nil
	}
	key := account + "|" + groupJID
	s.rostersMu.Lock()
	e, ok := s.rosters[key]
	s.rostersMu.Unlock()
	if ok && time.Since(e.at) < rosterTTL {
		return e.ms
	}
	ms, err := r.GroupMembers(ctx, account, groupJID)
	if err != nil {
		s.log.Warn("group members", "group", groupJID, "err", err) // the bridge down: who Chat has seen still shows
		return e.ms
	}
	s.rostersMu.Lock()
	if s.rosters == nil {
		s.rosters = map[string]rosterEntry{}
	}
	s.rosters[key] = rosterEntry{at: time.Now(), ms: ms}
	s.rostersMu.Unlock()
	return ms
}

// rosterNames: the names of a group's members as this GSI number's phone knows them, by number.
func (s *Server) rosterNames(ctx context.Context, account, groupJID string) map[string]string {
	names := map[string]string{}
	for _, m := range s.rosterOf(ctx, account, groupJID) {
		if m.Number != "" && m.Name != "" {
			names[m.Number] = m.Name
		}
	}
	return names
}

// chatMembers: GET /chat/threads/{id}/members — the group's members (Anggota, and who "@" offers); empty otherwise.
func (s *Server) chatMembers(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	ms, hidden, err := s.groupMembers(r.Context(), t)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if ms == nil {
		ms = []GroupMember{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": ms, "hidden": hidden})
}

// chatOpenMember: POST /chat/threads/{id}/members/{number}/open — the one-to-one chat of this group thread's GSI
// number with a member, made when it does not exist yet (nothing is sent). → {thread_id}
func (s *Server) chatOpenMember(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	n := wa.Digits(chi.URLParam(r, "number"))
	ms, _, err := s.groupMembers(r.Context(), t)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	i := slices.IndexFunc(ms, func(m GroupMember) bool { return m.Number == n })
	if i < 0 {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Nomor ini bukan anggota grup")
		return
	}
	if ms[i].ThreadID != "" {
		httpx.JSON(w, http.StatusOK, map[string]any{"thread_id": ms[i].ThreadID})
		return
	}
	id, err := wa.OpenDirect(r.Context(), s.st, deref(t.SalesWa), n, ms[i].Name)
	if errors.Is(err, wa.ErrPersonal) {
		httpx.Fail(w, http.StatusConflict, "personal", "Nomor ini ditandai pribadi — chat-nya tidak disimpan di GSI Orbit")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "chat.open_member", "chat_thread", &id, map[string]any{"group_thread": t.ID, "number": n})
	httpx.JSON(w, http.StatusCreated, map[string]any{"thread_id": id})
}

// sendMentions: the tags of a text sent from Chat to a group ("@<number>" in the text) — kept with the bubble (by
// name) and the numbers WhatsApp tags. Nothing for a one-to-one chat.
func (s *Server) sendMentions(ctx context.Context, t gen.GetThreadRow, text string) (json.RawMessage, []string, error) {
	tokens := wa.MentionTokens(text)
	if deref(t.Kind) != "group" || len(tokens) == 0 {
		return nil, nil, nil
	}
	members, _, err := s.groupMembers(ctx, t)
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
