package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/clock"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/httpx"
	"distri-arc/internal/pilot"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
)

func (s *Server) pilotRoutes(r chi.Router) {
	r.Get("/pilot", s.pilotReport)
	r.Get("/pilot/export.csv", s.pilotExport)
	r.Post("/pilot/mode", s.pilotMode)
	r.Post("/pilot/unlock", s.pilotUnlock)
	r.Get("/sow/top", s.sowTop)
	r.Post("/sow/confirm", s.sowConfirm)
}

func (s *Server) pilotSvc() pilot.Service { return pilot.Service{St: s.st, Clock: s.clock} }

// pilotReport is Pengaturan → Pilot (CEO/admin): the pilot so far plus the weekly snapshots.
func (s *Server) pilotReport(w http.ResponseWriter, r *http.Request) {
	if u, _ := CurrentUser(r.Context()); !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Dashboard pilot untuk CEO dan admin")
		return
	}
	svc := s.pilotSvc()
	from, to, err := svc.Period(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	rep, err := svc.Build(r.Context(), from, to)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	weeks, err := s.st.Q.ListPilotWeeks(r.Context(), rep.Branch)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	ws := make([]json.RawMessage, 0, len(weeks))
	for _, x := range weeks {
		ws = append(ws, x.Data)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"report": rep, "weeks": ws})
}

// pilotExport is the weekly CSV (?week=YYYY-MM-DD, any day of the week; default: the pilot so far).
func (s *Server) pilotExport(w http.ResponseWriter, r *http.Request) {
	if u, _ := CurrentUser(r.Context()); !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Ekspor pilot untuk CEO dan admin")
		return
	}
	svc := s.pilotSvc()
	from, to, err := svc.Period(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	name := "pilot-" + from.Format("20060102")
	if q := r.URL.Query().Get("week"); q != "" {
		t, err := time.ParseInLocation("2006-01-02", q, clock.WIB)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "week harus YYYY-MM-DD")
			return
		}
		from = pilot.Week(t)
		to = from.AddDate(0, 0, 7)
		name = "pilot-minggu-" + from.Format("20060102")
	}
	rep, err := svc.Build(r.Context(), from, to)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
	_, _ = w.Write(pilot.CSV(rep))
}

// pilotMode switches the pilot: shadow (starts it), live, off. CEO only.
func (s *Server) pilotMode(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if deref(u.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang mengubah mode pilot")
		return
	}
	var in struct {
		Mode   string `json:"mode"`
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	pp, err := s.pilotSvc().SetMode(r.Context(), in.Mode, in.Branch, &u)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"pilot": pp, "message": map[string]string{
		"shadow": "Pilot dimulai · mode bayangan: tidak ada kirim, semua saran butuh approve",
		"live":   "Mode live: kirim setelah disetujui · otonomi hanya untuk agen yang dibuka",
		"off":    "Pilot dimatikan · matriks otonomi berlaku penuh",
	}[pp.Mode]})
}

// pilotUnlock opens an agent's automatic steps after two weeks ≥ 80% (checked on the server). CEO only, mode live.
func (s *Server) pilotUnlock(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if deref(u.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Hanya CEO yang membuka otonomi")
		return
	}
	var in struct {
		Agent string `json:"agent"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in)
	if pol, err := policy.Load(r.Context(), s.st.Q); err == nil && pol.Pilot.Mode != "live" {
		httpx.Fail(w, http.StatusConflict, "pilot_not_live", "Buka otonomi setelah mode bayangan selesai (mode live)")
		return
	}
	pp, err := s.pilotSvc().Unlock(r.Context(), in.Agent, &u)
	if errors.Is(err, pilot.ErrNotEligible) {
		httpx.Fail(w, http.StatusConflict, "not_eligible", err.Error())
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"pilot": pp, "message": in.Agent + " boleh otonom sesuai matriks · berlaku di siklus berikutnya"})
}

// sowTop lists the dealers to confirm share of wallet for (sales: their own; others: the pilot branch).
func (s *Server) sowTop(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	pol, err := policy.Load(r.Context(), s.st.Q)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 || lim > 50 {
		lim = 20
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" && pol.Pilot.Mode != "off" {
		branch = pol.Pilot.Branch
	}
	var owner *uuid.UUID
	if deref(u.Role) == "sales" {
		owner, branch = u.SalesUserID, ""
	}
	q := pilot.Quarter(s.clock.Now())
	rows, err := s.st.Q.TopDealersForSOW(r.Context(), gen.TopDealersForSOWParams{Quarter: q, Branch: branch, Owner: owner, Lim: int32(lim)})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"quarter": q, "branch": branch, "items": nonNil(rows)})
}

// sowConfirm stores the sales confirmation of share of wallet for this quarter and recomputes those dealers.
func (s *Server) sowConfirm(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	var in struct {
		Items []struct {
			DealerID uuid.UUID `json:"dealer_id"`
			SOW      int       `json:"sow"`
			Note     string    `json:"note"`
		} `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil || len(in.Items) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi minimal satu dealer")
		return
	}
	role := deref(u.Role)
	if role != "sales" && !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Konfirmasi share of wallet oleh sales, CEO atau admin")
		return
	}
	now := s.clock.Now()
	q := pilot.Quarter(now)
	var ids []uuid.UUID
	err := s.st.Tx(r.Context(), func(qq *gen.Queries, _ pgx.Tx) error {
		for _, it := range in.Items {
			if it.SOW < 0 || it.SOW > 100 {
				return errors.New("share of wallet 0–100%")
			}
			id := it.DealerID.String()
			d, err := qq.GetDealer(r.Context(), &id)
			if err != nil {
				return errors.New("dealer tidak dikenal")
			}
			if role == "sales" && (d.OwnerID == nil || u.SalesUserID == nil || *d.OwnerID != *u.SalesUserID) {
				return errForbiddenDealer
			}
			note := it.Note
			if note == "" {
				note = "Dikonfirmasi " + deref(u.Name)
			}
			if err := qq.ConfirmSOW(r.Context(), gen.ConfirmSOWParams{DealerID: &d.ID, Quarter: q, Sow: int16(it.SOW), Note: &note, ConfirmedBy: u.SalesUserID, ConfirmedAt: now}); err != nil {
				return err
			}
			ids = append(ids, d.ID)
		}
		s.auditUser(r, "sow.confirm", "dealer", map[string]any{"quarter": q, "count": len(in.Items)})
		return nil
	})
	if errors.Is(err, errForbiddenDealer) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", err.Error())
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if _, err := dealersvc.New(s.st, s.clock).Recompute(r.Context(), ids...); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"confirmed": len(ids), "quarter": q, "message": strconv.Itoa(len(ids)) + " dealer dikonfirmasi · share of wallet & skor dihitung ulang"})
}

var errForbiddenDealer = errors.New("hanya dealer milik Anda")
