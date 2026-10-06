package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/identify"
	"distri-arc/internal/jobs"
)

func (s *Server) identifyRoutes(r chi.Router) {
	r.Post("/chat/identify", s.chatIdentify)
	r.Post("/identifications/import", s.importIdentifications)
}

// chatIdentify identifies an inbound number now (Getcontact, Truecaller, Odoo) and asks the worker to add the
// WhatsApp Business profile it can read through the sales number's connection.
func (s *Server) chatIdentify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WANumber string `json:"wa_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.WANumber) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "wa_number wajib diisi")
		return
	}
	if s.idf == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Identifikasi belum aktif")
		return
	}
	res, err := s.idf.Identify(r.Context(), body.WANumber)
	switch {
	case errors.Is(err, identify.ErrNotInbound):
		httpx.Fail(w, http.StatusForbidden, "not_inbound", err.Error())
		return
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if s.jobs != nil {
		_, _ = s.jobs.Insert(r.Context(), jobs.IdentifyArgs{WANumber: res.WANumber}, nil)
	}
	s.auditUser(r, "identify.number", "identification:"+res.WANumber, map[string]any{"score": res.Score})
	httpx.JSON(w, http.StatusOK, res)
}

// importIdentifications takes a manual Getcontact export (CSV: number,name[,tags]) as the request body or as the
// "file" field of a multipart form.
func (s *Server) importIdentifications(w http.ResponseWriter, r *http.Request) {
	var src io.Reader = http.MaxBytesReader(w, r.Body, 2<<20)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		f, _, err := r.FormFile("file")
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "unggah file CSV di field 'file'")
			return
		}
		defer func() { _ = f.Close() }()
		src = f
	}
	u, _ := CurrentUser(r.Context())
	res, err := identify.ImportGetcontact(r.Context(), s.st.Q, src, u.SalesUserID, s.clock.Now())
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "CSV tidak terbaca: "+err.Error())
		return
	}
	s.auditUser(r, "identify.import", "getcontact", res)
	httpx.JSON(w, http.StatusOK, res)
}
