package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/auth"
	"distri-arc/internal/hermes"
	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// Hermes Agent → Chat (ADR 0067): each user chats with the company's Hermes Agent from GSI Orbit like in Telegram.
// Under /api/mcp/ so it follows the MCP screen; the Hermes URL and API key are set by the CEO and sealed.

func (s *Server) hermesRoutes(r chi.Router) {
	r.Get("/mcp/hermes/chat", s.hermesChat)
	r.Post("/mcp/hermes/chat", s.hermesAsk)
	r.Delete("/mcp/hermes/chat", s.hermesClear)
	r.Put("/mcp/hermes/config", s.hermesConfigSet)
	r.Delete("/mcp/hermes/config", s.hermesConfigDelete)
}

func (s *Server) hermes() hermes.Service {
	return hermes.Service{Pool: s.st.Pool, SessionKey: s.secret(), Log: s.log}
}

func (s *Server) hermesChat(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	h := s.hermes()
	msgs, err := h.List(r.Context(), u.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if msgs == nil {
		msgs = []hermes.Message{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"config": h.LoadConfig(r.Context()), "messages": msgs})
}

func (s *Server) hermesAsk(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" || utf8.RuneCountInString(text) > 8000 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Tulis pertanyaan (maks. 8.000 karakter)")
		return
	}
	m, err := s.hermes().Ask(r.Context(), u.ID, text)
	if errors.Is(err, hermes.ErrBusy) {
		httpx.Fail(w, http.StatusConflict, "busy", err.Error())
		return
	} else if errors.Is(err, hermes.ErrNotConnected) {
		httpx.Fail(w, http.StatusConflict, "not_connected", err.Error())
		return
	} else if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{"message": m})
}

func (s *Server) hermesClear(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if err := s.hermes().Clear(r.Context(), u.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hermesConfigSet checks the URL and key against Hermes (GET /v1/models) before sealing them. An empty api_key keeps
// the stored one (changing only the URL).
func (s *Server) hermesConfigSet(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var body struct {
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	base, err := hermes.NormalizeURL(body.URL)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	key := strings.TrimSpace(body.APIKey)
	if key == "" {
		if sealed, err := s.st.Q.GetSecret(r.Context(), hermes.SecretKey); err == nil {
			key, _ = auth.Open(sealed, s.secret())
		}
	}
	if key == "" || strings.ContainsAny(key, " \n\t") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi API key Hermes (API_SERVER_KEY di ~/.hermes/.env)")
		return
	}
	if err := (hermes.Client{BaseURL: base, Key: key}).Verify(r.Context()); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "unreachable", err.Error())
		return
	}
	for k, v := range map[string]string{hermes.SecretURL: base, hermes.SecretKey: key} {
		sealed, err := auth.Seal(v, s.secret())
		if err == nil {
			err = s.st.Q.SetSecret(r.Context(), gen.SetSecretParams{Key: k, Value: sealed, UpdatedBy: u.Email})
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	s.auditUser(r, "hermes.config_set", "secret:"+hermes.SecretURL, map[string]any{"url": base, "hint": hermes.KeyHint(key)})
	httpx.JSON(w, http.StatusOK, s.hermes().LoadConfig(r.Context()))
}

func (s *Server) hermesConfigDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ceoOnly(w, r); !ok {
		return
	}
	_ = s.st.Q.DeleteSecret(r.Context(), hermes.SecretURL)
	_ = s.st.Q.DeleteSecret(r.Context(), hermes.SecretKey)
	s.auditUser(r, "hermes.config_deleted", "secret:"+hermes.SecretURL, nil)
	w.WriteHeader(http.StatusNoContent)
}
