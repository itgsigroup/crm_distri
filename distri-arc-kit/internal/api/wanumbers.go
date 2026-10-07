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

// canManageNumber: CEO/admin manage every number; anyone else only the number they hold.
func canManageNumber(u User, n gen.WaNumber) bool {
	if isAdmin(u) {
		return true
	}
	if n.UserID != nil && *n.UserID == u.ID {
		return true
	}
	return deref(u.Role) == "sales" && u.SalesUserID != nil && n.SalesID != nil && *n.SalesID == *u.SalesUserID
}

// addWANumber gives a user their WhatsApp number in Chat (ADR 0019: one user holds one number). The number comes
// from the user master (Pengaturan → Pengguna & peran); when the user has none yet it may be filled here and is saved
// on the user. CEO/admin add for anyone, other users only for themselves. The number is internal: direct messages
// between internal numbers are never stored (09-policies-security).
func (s *Server) addWANumber(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	var in struct {
		UserID   uuid.UUID `json:"user_id"`
		WANumber string    `json:"wa_number"` // only when the user has no number yet
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || in.UserID == uuid.Nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih pengguna dari master pengguna")
		return
	}
	if !isAdmin(me) && in.UserID != me.ID {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Menambah nomor pengguna lain oleh CEO atau admin")
		return
	}
	ctx := r.Context()
	usr, err := s.st.Q.GetUserByID(ctx, in.UserID)
	if err != nil || !usr.Active {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pengguna tidak ditemukan atau nonaktif")
		return
	}
	if !usr.WaAllowed && deref(usr.Role) != "ceo" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Peran pengguna ini tidak memegang nomor WhatsApp — ubah di Pengaturan → Peran & akses")
		return
	}
	n := deref(usr.WaNumber)
	if n == "" {
		n = wa.Digits(in.WANumber)
		if !strings.HasPrefix(n, "62") || len(n) < 10 || len(n) > 15 {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Pengguna ini belum punya nomor WhatsApp — isi nomor Indonesia, mis. 0812… atau +62812…")
			return
		}
	}
	if held, err := s.st.Q.GetWANumberByUser(ctx, &usr.ID); err == nil && held.WaNumber != n {
		httpx.Fail(w, http.StatusConflict, "has_number", deref(usr.Name)+" sudah memegang "+wa.MaskNumber(held.WaNumber)+" — satu pengguna satu nomor")
		return
	}
	if existing, err := s.st.Q.GetWANumber(ctx, n); err == nil && existing.UserID != nil && *existing.UserID != usr.ID {
		httpx.Fail(w, http.StatusConflict, "exists", "Nomor ini sudah dipegang pengguna lain")
		return
	}
	if deref(usr.WaNumber) == "" {
		if err := s.st.Q.SetUserProfile(ctx, gen.SetUserProfileParams{ID: usr.ID, WaNumber: &n}); err != nil {
			httpx.Fail(w, http.StatusConflict, "wa_taken", "Nomor WhatsApp ini sudah dipegang pengguna lain — satu nomor untuk satu pengguna")
			return
		}
	}
	if usr.SalesUserID != nil {
		_ = s.st.Q.SetSalesProfileWA(ctx, gen.SetSalesProfileWAParams{ID: *usr.SalesUserID, WaNumber: n})
	}
	transport := s.cfg.WATransport
	if transport == "" {
		transport = "fake"
	}
	var sales *uuid.UUID
	if deref(usr.Role) == "sales" {
		sales = usr.SalesUserID // a sales' conversations follow their dealers and stay visible only to them
	}
	label := deref(usr.Name)
	if err := s.st.Q.UpsertWANumber(ctx, gen.UpsertWANumberParams{WaNumber: n, SalesID: sales, Label: &label, Transport: transport, State: "unpaired", UserID: &usr.ID}); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	dept := "WhatsApp tim"
	if err := s.st.Q.UpsertInternalNumber(ctx, gen.UpsertInternalNumberParams{WaNumber: n, Label: &label, Department: &dept, IsSales: sales != nil}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "wa.number_added", "wa_number", nil, map[string]any{"wa_number": n, "user_id": usr.ID})
	httpx.JSON(w, http.StatusCreated, map[string]any{"wa_number": n, "label": label, "user_id": usr.ID, "state": "unpaired"})
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
