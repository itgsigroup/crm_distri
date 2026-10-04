package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// signed wraps a handler with HMAC verification of the raw body (empty for GET/DELETE).
func signed(secret string, next func(http.ResponseWriter, *http.Request, []byte)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "body")
			return
		}
		if !verify(secret, body, r.Header.Get("X-ARC-Signature")) {
			writeErr(w, http.StatusUnauthorized, "signature tidak valid")
			return
		}
		next(w, r, body)
	}
}

func newHandler(cfg config, m *manager, f *forwarder) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		last := f.lastEvent
		f.mu.Unlock()
		var lastStr any
		if !last.IsZero() {
			lastStr = last.Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sessions": m.statuses(), "queue": f.queueLen(), "last_event": lastStr})
	})

	mux.HandleFunc("POST /sessions", signed(cfg.Secret, func(w http.ResponseWriter, r *http.Request, body []byte) {
		var in sessionMeta
		if err := json.Unmarshal(body, &in); err != nil || !sessionIDRe.MatchString(in.ID) {
			writeErr(w, http.StatusBadRequest, "id sesi tidak valid")
			return
		}
		st, err := m.create(r.Context(), sessionMeta{ID: in.ID, Label: in.Label, HistoryDays: in.HistoryDays})
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	}))

	mux.HandleFunc("GET /sessions/{id}/qr", signed(cfg.Secret, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		s, err := m.get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.snapshot())
	}))

	mux.HandleFunc("DELETE /sessions/{id}", signed(cfg.Secret, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if err := m.remove(r.Context(), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("GET /sessions/{id}/groups", signed(cfg.Secret, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		s, err := m.get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		if !s.client.IsLoggedIn() {
			writeErr(w, http.StatusConflict, errNotLinked.Error())
			return
		}
		groups, err := s.client.GetJoinedGroups(r.Context())
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		type item struct {
			JID  string    `json:"jid"`
			Meta groupMeta `json:"meta"`
		}
		out := make([]item, 0, len(groups))
		for _, g := range groups {
			meta := groupMeta{Name: g.Name}
			for _, p := range g.Participants {
				phone := p.PhoneNumber.User
				if phone == "" {
					phone = m.phoneOf(r.Context(), s, p.JID)
				}
				meta.Members = append(meta.Members, groupMember{JID: p.JID.String(), Phone: phone, Name: p.DisplayName})
			}
			out = append(out, item{JID: g.JID.String(), Meta: meta})
		}
		writeJSON(w, http.StatusOK, out)
	}))

	mux.HandleFunc("GET /sessions/{id}/contacts/{jid}", signed(cfg.Secret, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		s, err := m.get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		jid, err := types.ParseJID(r.PathValue("jid"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "jid tidak valid")
			return
		}
		if jid.Server == "" {
			jid = types.NewJID(jid.User, types.DefaultUserServer)
		}
		ctx := r.Context()
		prof := map[string]any{"name": "", "about": "", "has_photo": false, "business": ""}
		if c, err := s.client.Store.Contacts.GetContact(ctx, jid); err == nil && c.Found {
			prof["name"] = firstNonEmpty(c.BusinessName, c.FullName, c.PushName)
			prof["business"] = c.BusinessName
		}
		if infos, err := s.client.GetUserInfo(ctx, []types.JID{jid}); err == nil {
			if ui, ok := infos[jid]; ok {
				prof["about"] = ui.Status
				prof["has_photo"] = ui.PictureID != ""
				if ui.VerifiedName != nil && ui.VerifiedName.Details != nil {
					prof["business"] = ui.VerifiedName.Details.GetVerifiedName()
					if prof["name"] == "" {
						prof["name"] = prof["business"]
					}
				}
			}
		}
		if prof["has_photo"] == false {
			if pic, err := s.client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true}); err == nil && pic != nil {
				prof["has_photo"] = true
			}
		}
		writeJSON(w, http.StatusOK, prof)
	}))

	mux.HandleFunc("POST /sessions/{id}/send", signed(cfg.Secret, func(w http.ResponseWriter, r *http.Request, body []byte) {
		var in struct {
			ChatID   string `json:"chat_id"`
			Text     string `json:"text"`
			ActionID string `json:"action_id"`
		}
		if err := json.Unmarshal(body, &in); err != nil || in.ChatID == "" || in.Text == "" {
			writeErr(w, http.StatusBadRequest, "chat_id dan text wajib")
			return
		}
		wamid, err := m.send(r.Context(), r.PathValue("id"), in.ChatID, in.Text, in.ActionID)
		switch {
		case errors.Is(err, errNotApproved):
			writeErr(w, http.StatusForbidden, err.Error())
		case errors.Is(err, errRateLimited):
			writeErr(w, http.StatusTooManyRequests, err.Error())
		case errors.Is(err, errNoSession):
			writeErr(w, http.StatusNotFound, err.Error())
		case errors.Is(err, errNotLinked):
			writeErr(w, http.StatusConflict, err.Error())
		case err != nil:
			writeErr(w, http.StatusBadGateway, err.Error())
		default:
			writeJSON(w, http.StatusOK, map[string]string{"wamid": wamid})
		}
	}))
	return mux
}
