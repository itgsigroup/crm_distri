package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	qrcode "github.com/skip2/go-qrcode"

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

// createWALink starts linking a new WhatsApp number (Chat → + Nomor): the phone scans a QR — or types a code, which
// needs the number — and WhatsApp reports the number; then a person gives it to a user (assignWANumber).
func (s *Server) createWALink(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	var in struct {
		Method string `json:"method"` // qr | code
		Phone  string `json:"phone"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	if !isAdmin(me) && !roleOf(me).WAAllowed {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Peran Anda tidak memegang nomor WhatsApp")
		return
	}
	if !isAdmin(me) && me.WaNumber != nil {
		httpx.Fail(w, http.StatusConflict, "has_number", "Anda sudah memegang "+wa.MaskNumber(*me.WaNumber)+" — satu pengguna satu nomor")
		return
	}
	method, phone := "qr", ""
	if in.Method == "code" {
		if s.cfg.WATransport != "baileys" && s.cfg.WATransport != "" && s.cfg.WATransport != "fake" {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Tautan dengan kode hanya untuk transport Baileys — pakai QR")
			return
		}
		phone = wa.Digits(in.Phone)
		if !strings.HasPrefix(phone, "62") || len(phone) < 10 || len(phone) > 15 {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Untuk kode, isi nomor WhatsApp yang akan ditautkan, mis. 0812…")
			return
		}
		method = "code"
	}
	if s.jobs == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "no_queue", "Antrean tidak tersedia")
		return
	}
	raw := make([]byte, 6)
	_, _ = rand.Read(raw)
	session := "link-" + hex.EncodeToString(raw)
	var ph *string
	if phone != "" {
		ph = &phone
	}
	if err := s.st.Q.InsertWALink(r.Context(), gen.InsertWALinkParams{SessionID: session, Method: method, Phone: ph, CreatedBy: me.Email}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if _, err := s.jobs.Insert(r.Context(), jobs.WAPairArgs{Session: session, Phone: phone, Method: map[bool]string{true: "code", false: ""}[method == "code"]}, nil); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "wa.link_started", "wa_link", nil, map[string]any{"session": session, "method": method})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"session_id": session, "state": "pairing"})
}

// getWALink is the state of a new link: its QR (as a PNG) or code while pairing, then the linked number.
func (s *Server) getWALink(w http.ResponseWriter, r *http.Request) {
	l, err := s.st.Q.GetWALink(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Tautan tidak ditemukan")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := map[string]any{"session_id": l.SessionID, "method": l.Method, "state": l.State, "error": l.Error}
	if l.State == "pairing" && l.Qr != nil {
		if code, ok := strings.CutPrefix(*l.Qr, wa.PairCodePrefix); ok {
			out["pair_code"] = code
		} else if png, err := qrcode.Encode(*l.Qr, qrcode.Medium, 256); err == nil {
			out["qr_png"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
	}
	if l.WaNumber != nil {
		out["wa_number"], out["masked"] = *l.WaNumber, wa.MaskNumber(*l.WaNumber)
		if n, err := s.st.Q.GetWANumber(r.Context(), *l.WaNumber); err == nil {
			out["user_id"] = n.UserID
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// assignWANumber gives a linked number to a user of the user master (one user, one number; user_id null releases
// it). Pengguna page holders assign to anyone; others only take an unassigned number for themselves.
func (s *Server) assignWANumber(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	ctx := r.Context()
	n := wa.Digits(chi.URLParam(r, "wa"))
	num, err := s.st.Q.GetWANumber(ctx, n)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "unknown_number", "Nomor belum tertaut — scan dulu di Chat → + Nomor")
		return
	}
	var in struct {
		UserID *uuid.UUID `json:"user_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih pengguna")
		return
	}
	manager := can(me, "users")
	if !manager && (in.UserID == nil || *in.UserID != me.ID || num.UserID != nil) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Memberi nomor ke pengguna lain lewat halaman Pengguna (pemegang menu Pengguna)")
		return
	}
	if in.UserID == nil { // release
		err := s.st.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
			if err := q.SetWANumberUser(ctx, gen.SetWANumberUserParams{WaNumber: n}); err != nil {
				return err
			}
			if err := q.ClearUserWANumber(ctx, &n); err != nil {
				return err
			}
			return q.AssignThreadsSales(ctx, gen.AssignThreadsSalesParams{Account: &n})
		})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		s.audit(r, "wa.number_released", "wa_number", nil, map[string]any{"wa_number": n})
		httpx.JSON(w, http.StatusOK, map[string]any{"wa_number": n, "message": "Nomor dilepas dari penggunanya"})
		return
	}
	usr, err := s.st.Q.GetUserByID(ctx, *in.UserID)
	if err != nil || !usr.Active {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pengguna tidak ditemukan atau nonaktif")
		return
	}
	if !usr.WaAllowed && deref(usr.Role) != "ceo" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Peran "+deref(usr.Name)+" tidak memegang nomor WhatsApp — ubah di Peran & akses")
		return
	}
	if held, err := s.st.Q.GetWANumberByUser(ctx, &usr.ID); err == nil && held.WaNumber != n {
		httpx.Fail(w, http.StatusConflict, "has_number", deref(usr.Name)+" sudah memegang "+wa.MaskNumber(held.WaNumber)+" — satu pengguna satu nomor; lepas dulu nomor itu")
		return
	}
	var sales *uuid.UUID
	if deref(usr.Role) == "sales" {
		sales = usr.SalesUserID // a sales' conversations follow their dealers and stay visible only to them
	}
	label := deref(usr.Name)
	err = s.st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		if err := q.ClearUserWANumber(ctx, &n); err != nil { // the number's previous holder
			return err
		}
		if err := q.SetWANumberUser(ctx, gen.SetWANumberUserParams{WaNumber: n, UserID: &usr.ID, SalesID: sales, Label: &label}); err != nil {
			return err
		}
		if err := q.SetUserWANumber(ctx, gen.SetUserWANumberParams{ID: usr.ID, WaNumber: &n}); err != nil {
			return err
		}
		if usr.SalesUserID != nil {
			if _, err := tx.Exec(ctx, "update sales_users set wa_number = null where wa_number = $1 and id <> $2", n, *usr.SalesUserID); err != nil {
				return err
			}
			if err := q.SetSalesProfileWA(ctx, gen.SetSalesProfileWAParams{ID: *usr.SalesUserID, WaNumber: n}); err != nil {
				return err
			}
		}
		dept := "WhatsApp tim"
		if err := q.UpsertInternalNumber(ctx, gen.UpsertInternalNumberParams{WaNumber: n, Label: &label, Department: &dept, IsSales: sales != nil}); err != nil {
			return err
		}
		return q.AssignThreadsSales(ctx, gen.AssignThreadsSalesParams{Account: &n, SalesID: sales})
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "wa.number_assigned", "wa_number", nil, map[string]any{"wa_number": n, "user_id": usr.ID})
	httpx.JSON(w, http.StatusOK, map[string]any{"wa_number": n, "user_id": usr.ID, "label": label, "message": wa.MaskNumber(n) + " dipegang " + label})
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
