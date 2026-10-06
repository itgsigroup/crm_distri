package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
)

func policyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, policy.ErrUnknownKey):
		httpx.Fail(w, http.StatusNotFound, "unknown_policy", err.Error())
	case errors.Is(err, policy.ErrInvalid), errors.Is(err, policy.ErrLocked):
		httpx.Fail(w, http.StatusBadRequest, "invalid_policy", err.Error())
	default:
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

// putPolicy writes a new version of one policy (CEO): schema-validated, versioned, audited; applies on the next cycle.
func (s *Server) putPolicy(w http.ResponseWriter, r *http.Request) {
	u, ok := ceo(r)
	if !ok {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang dapat mengubah kebijakan")
		return
	}
	key := chi.URLParam(r, "key")
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil || !json.Valid(b) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format kebijakan tidak valid")
		return
	}
	p, err := policy.Save(r.Context(), s.st, key, b, u.SalesUserID, deref(u.Email), time.Now())
	if err != nil {
		policyError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"key": p.Key, "value": p.Value, "version": p.Version, "updated_at": p.UpdatedAt, "message": "Tersimpan · berlaku di siklus berikutnya"})
}

func (s *Server) policyHistory(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 50 {
		n = 10
	}
	rows, err := s.st.Q.ListPolicyHistory(r.Context(), gen.ListPolicyHistoryParams{Key: chi.URLParam(r, "key"), Limit: int32(n)})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}
