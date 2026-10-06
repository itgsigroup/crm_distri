package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/jobs"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// canManageNumber: CEO/admin manage every number; a sales user only their own numbers.
func canManageNumber(u User, n gen.WaNumber) bool {
	if isAdmin(u) {
		return true
	}
	return deref(u.Role) == "sales" && u.SalesUserID != nil && n.SalesID != nil && *n.SalesID == *u.SalesUserID
}

// addWANumber registers another WhatsApp number (many numbers per sales, or a team number such as CS kantor).
// The number is internal: direct messages between internal numbers are never stored (09-policies-security).
func (s *Server) addWANumber(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Menambah nomor WhatsApp oleh CEO atau admin")
		return
	}
	var in struct {
		WANumber  string     `json:"wa_number"`
		Label     string     `json:"label"`
		SalesID   *uuid.UUID `json:"sales_id"`
		SalesName string     `json:"sales_name"` // owner by name (Pengaturan form); empty = team number
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	n := wa.Digits(in.WANumber)
	label := strings.TrimSpace(in.Label)
	if !strings.HasPrefix(n, "62") || len(n) < 10 || len(n) > 15 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nomor harus nomor Indonesia, mis. 0812… atau +62812…")
		return
	}
	if label == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama/label nomor, mis. \"CS Kantor\" atau \"Andi (nomor 2)\"")
		return
	}
	if in.SalesID == nil && strings.TrimSpace(in.SalesName) != "" {
		sales, err := s.st.Q.ListSalesUsers(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		for _, su := range sales {
			if strings.EqualFold(su.Name, strings.TrimSpace(in.SalesName)) {
				id := su.ID
				in.SalesID = &id
			}
		}
		if in.SalesID == nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Sales tidak ditemukan")
			return
		}
	}
	if existing, err := s.st.Q.GetWANumber(r.Context(), n); err == nil && existing.State == "connected" {
		httpx.Fail(w, http.StatusConflict, "exists", "Nomor ini sudah terhubung")
		return
	}
	transport := s.cfg.WATransport
	if transport == "" {
		transport = "fake"
	}
	if err := s.st.Q.UpsertWANumber(r.Context(), gen.UpsertWANumberParams{WaNumber: n, SalesID: in.SalesID, Label: &label, Transport: transport, State: "unpaired"}); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	dept := "WhatsApp tim"
	if err := s.st.Q.UpsertInternalNumber(r.Context(), gen.UpsertInternalNumberParams{WaNumber: n, Label: &label, Department: &dept, IsSales: in.SalesID != nil}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "wa.number_added", "wa_number", nil, map[string]any{"wa_number": n, "label": label})
	httpx.JSON(w, http.StatusCreated, map[string]any{"wa_number": n, "label": label, "state": "unpaired"})
}

// deleteWANumber unlinks the device (worker) and removes a team number; a sales' main number stays, unpaired.
func (s *Server) deleteWANumber(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	n := wa.Digits(chi.URLParam(r, "wa"))
	row, err := s.st.Q.GetWANumber(r.Context(), n)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "unknown_number", "Nomor tidak terdaftar")
		return
	}
	if !canManageNumber(u, row) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO, admin, atau pemilik nomor")
		return
	}
	if s.jobs == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "no_queue", "Antrean tidak tersedia")
		return
	}
	if _, err := s.jobs.Insert(r.Context(), jobs.WAUnpairArgs{WANumber: n}, nil); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "wa.number_unlinked", "wa_number", nil, map[string]any{"wa_number": n})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"wa_number": n, "state": "unpaired"})
}
