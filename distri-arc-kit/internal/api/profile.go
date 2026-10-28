package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Foto & profil WhatsApp (ADR 0071): the chat list shows each contact's and group's profile photo, and a click on it
// opens their WhatsApp profile — photo, name, number, Info — with the groups they share with GSI's numbers.

// photoFresh / noPhotoFresh: how long a cached photo, or "no photo", is served before WhatsApp is asked again.
const (
	photoFresh   = 3 * 24 * time.Hour
	noPhotoFresh = 24 * time.Hour
)

// photoSlots caps the photo queries running at once (a chat list asks for every visible avatar together).
var photoSlots = make(chan struct{}, 4)

func (s *Server) profileRoutes(r chi.Router) {
	r.Get("/chat/threads/{id}/photo", s.chatPhoto)
	r.Get("/chat/threads/{id}/profile", s.chatProfile)
}

// photoFetcher reads profile photos: the media double when it can (tests), else the Baileys bridge.
func (s *Server) photoFetcher() wa.PhotoFetcher {
	f, _ := s.mediaFetcher().(wa.PhotoFetcher)
	return f
}

// threadAccount is the GSI number a conversation is held by.
func threadAccount(t gen.GetThreadRow) string {
	if a := deref(t.Account); a != "" {
		return a
	}
	return deref(t.SalesWa)
}

type cachedPhoto struct {
	has  bool
	mime string
	data []byte
	at   time.Time
}

// photo is a jid's profile photo: the cached one while fresh, else WhatsApp's (stored). ok = false: none to show.
func (s *Server) photo(ctx context.Context, account, jid string, full bool) (cachedPhoto, bool) {
	var c cachedPhoto
	err := s.st.Pool.QueryRow(ctx, "select has, mime, coalesce(data, ''::bytea), fetched_at from wa_photos where jid = $1 and full_size = $2", jid, full).
		Scan(&c.has, &c.mime, &c.data, &c.at)
	cached := err == nil
	if cached && ((c.has && time.Since(c.at) < photoFresh) || (!c.has && time.Since(c.at) < noPhotoFresh)) {
		return c, c.has
	}
	f := s.photoFetcher()
	if f == nil || account == "" {
		return c, cached && c.has
	}
	select {
	case photoSlots <- struct{}{}:
		defer func() { <-photoSlots }()
	case <-ctx.Done():
		return c, cached && c.has
	}
	fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	data, mime, err := f.Photo(fctx, account, jid, full)
	switch {
	case err == nil && len(data) > 0:
		c = cachedPhoto{has: true, mime: mime, data: data, at: time.Now()}
	case errors.Is(err, wa.ErrNoPhoto):
		c = cachedPhoto{at: time.Now()}
	default: // not connected or the hourly cap: a stale photo is better than none; nothing is stored
		return c, cached && c.has
	}
	if _, err := s.st.Pool.Exec(ctx, `insert into wa_photos (jid, full_size, account, has, mime, data, fetched_at) values ($1, $2, $3, $4, $5, $6, now())
		on conflict (jid, full_size) do update set account = excluded.account, has = excluded.has, mime = excluded.mime, data = excluded.data, fetched_at = now()`,
		jid, full, account, c.has, c.mime, c.data); err != nil {
		s.log.Warn("wa photo cache", "err", err)
	}
	return c, c.has
}

// chatPhoto: GET /chat/threads/{id}/photo[?full=1] — the contact's or group's profile photo; 404 when there is none
// (the page shows the initials).
func (s *Server) chatPhoto(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	jid := deref(t.WaJid)
	if jid == "" {
		httpx.Fail(w, http.StatusNotFound, "no_photo", "Tidak ada foto profil")
		return
	}
	c, ok := s.photo(r.Context(), threadAccount(t), jid, r.URL.Query().Get("full") == "1")
	if !ok {
		w.Header().Set("Cache-Control", "private, max-age=600")
		httpx.Fail(w, http.StatusNotFound, "no_photo", "Tidak ada foto profil")
		return
	}
	mime := c.mime
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/jpeg"
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	writeMedia(w, mime, "", bytes.NewReader(c.data), s.log)
}

// ProfileGroup is a WhatsApp group the contact shares with a GSI number.
type ProfileGroup struct {
	JID      string `json:"jid"`
	Name     string `json:"name"`
	ThreadID string `json:"thread_id,omitempty"` // the group's chat in GSI Orbit, when the viewer reads it
	Kind     string `json:"kind,omitempty"`      // external | internal
	Members  int    `json:"members"`
	Admin    bool   `json:"admin,omitempty"` // the contact is an admin of it
	Via      string `json:"via"`             // the GSI number's label
}

// chatProfile: GET /chat/threads/{id}/profile — the WhatsApp profile of a chat's contact (name, number, Info,
// business) and the groups they are in with GSI's numbers the viewer reads; a group: its name and member count.
func (s *Server) chatProfile(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadParam(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	out := map[string]any{"kind": deref(t.Kind), "title": deref(t.Title), "phone_name": deref(t.PhoneName), "account_label": t.AccountLabel}
	if deref(t.Kind) == "group" {
		ms, hidden, err := s.groupMembers(ctx, t)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out["members"] = len(ms) + hidden
		httpx.JSON(w, http.StatusOK, out)
		return
	}
	number := phoneOfJID(t.WaJid)
	out["number"] = number
	if pr, ok := s.mediaFetcher().(wa.ProfileReader); ok && deref(t.WaJid) != "" {
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if p, err := pr.Profile(pctx, threadAccount(t), deref(t.WaJid)); err == nil {
			out["name"], out["about"], out["business"] = p.Name, p.About, p.Business
		}
		cancel()
	}
	groups := []ProfileGroup{}
	if number != "" {
		var err error
		if groups, err = s.sharedGroups(r, number); err != nil {
			s.log.Warn("profile groups", "err", err) // the profile shows without them
		}
	}
	out["groups"] = groups
	httpx.JSON(w, http.StatusOK, out)
}

// sharedGroups are the groups of the GSI numbers the viewer reads that number is a member of, one per group.
func (s *Server) sharedGroups(r *http.Request, number string) ([]ProfileGroup, error) {
	ctx := r.Context()
	lister, _ := s.groupRoster().(wa.GroupLister)
	if lister == nil {
		return []ProfileGroup{}, nil
	}
	u, _ := CurrentUser(ctx)
	v, err := s.chatViewerOf(ctx, u)
	if err != nil {
		return nil, err
	}
	nums, err := s.st.Q.ListWANumbers(ctx)
	if err != nil {
		return nil, err
	}
	byJID := map[string]*ProfileGroup{}
	for _, n := range nums {
		acct := n.WaNumber
		if n.State != "connected" || !v.sees(n.SalesID, &acct) {
			continue
		}
		gctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		gs, err := lister.Groups(gctx, acct)
		cancel()
		if err != nil {
			continue // a number that does not answer: its groups are left out
		}
		for _, g := range gs {
			for _, m := range g.Members {
				if m.Number != number || byJID[g.JID] != nil {
					continue
				}
				via := deref(n.Label)
				if via == "" {
					via = wa.MaskNumber(acct)
				}
				byJID[g.JID] = &ProfileGroup{JID: g.JID, Name: g.Name, Members: len(g.Members), Admin: m.Admin, Via: via}
			}
		}
	}
	out := make([]ProfileGroup, 0, len(byJID))
	if len(byJID) == 0 {
		return out, nil
	}
	jids := make([]string, 0, len(byJID))
	for j := range byJID {
		jids = append(jids, j)
	}
	rows, err := s.st.Pool.Query(ctx, `select t.id::text, t.wa_jid, t.account, t.sales_id, coalesce(g.kind, '') from chat_threads t
		left join wa_groups g on g.id = t.group_id where t.wa_jid = any($1)`, jids)
	if err == nil {
		for rows.Next() {
			var id, jid, kind string
			var acct *string
			var sales *uuid.UUID
			if rows.Scan(&id, &jid, &acct, &sales, &kind) != nil {
				continue
			}
			if g := byJID[jid]; g != nil {
				if g.Kind == "" {
					g.Kind = kind
				}
				if g.ThreadID == "" && v.sees(sales, acct) {
					g.ThreadID = id
				}
			}
		}
		rows.Close()
	}
	for _, g := range byJID {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
