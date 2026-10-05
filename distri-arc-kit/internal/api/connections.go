package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"distri-arc/internal/domain"
	"distri-arc/internal/httpx"
	"distri-arc/internal/jobs"
	"distri-arc/internal/odoo"
	"distri-arc/internal/store/gen"
)

// WithOdoo gives the API the Odoo source for the connection test.
func (s *Server) WithOdoo(src odoo.Source) *Server { s.odoo = src; return s }

func (s *Server) connectionRoutes(r chi.Router) {
	r.Get("/connections", s.connections)
	r.Post("/connections/odoo/test", s.odooTest)
	r.Post("/connections/odoo/sync", s.odooSync)
	r.Get("/connections/odoo/categories", s.odooCategories)
	r.Put("/connections/odoo/categories", s.putOdooCategories)
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	state, err := s.st.Q.ListSyncState(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	nums, _ := s.st.Q.ListWANumbers(r.Context())
	connected := 0
	for _, n := range nums {
		if n.State == "connected" {
			connected++
		}
	}
	mode := s.cfg.OdooMode
	if mode == "" {
		mode = "fake"
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"odoo": map[string]any{"mode": mode, "write": s.cfg.OdooWrite, "url": s.cfg.OdooURL, "models": state},
		"wa":   map[string]any{"transport": s.cfg.WATransport, "numbers": len(nums), "connected": connected},
	})
}

func (s *Server) odooTest(w http.ResponseWriter, r *http.Request) {
	if s.odoo == nil {
		httpx.Fail(w, http.StatusConflict, "odoo_off", "Odoo dimatikan (ODOO_MODE=off)")
		return
	}
	v, err := s.odoo.Version(r.Context())
	if err != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "mode": s.odoo.Name()})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "version": v, "mode": s.odoo.Name(), "write": s.cfg.OdooWrite})
}

func (s *Server) odooSync(w http.ResponseWriter, r *http.Request) {
	if s.jobs == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "no_queue", "Antrean tidak tersedia")
		return
	}
	var in struct {
		Full bool `json:"full"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if _, err := s.jobs.Insert(r.Context(), jobs.OdooSyncArgs{Full: in.Full}, nil); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "odoo.sync", "odoo", nil, map[string]any{"full": in.Full})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"queued": true, "full": in.Full})
}

func (s *Server) odooCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListCategoryMap(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var odooCats []odoo.Record
	if s.odoo != nil {
		odooCats, _ = s.odoo.SearchRead(r.Context(), "product.category", nil, []string{"id", "name", "complete_name"})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"map": rows, "odoo": odooCats, "kat": domain.Categories})
}

func (s *Server) putOdooCategories(w http.ResponseWriter, r *http.Request) {
	var in []struct {
		OdooCategoryID int32  `json:"odoo_category_id"`
		Kat            string `json:"kat"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Format tidak valid")
		return
	}
	for _, c := range in {
		if domain.CategoryIndex(c.Kat) < 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid_kat", "Kategori harus salah satu dari 6 KAT: "+c.Kat)
			return
		}
	}
	for _, c := range in {
		if err := s.st.Q.UpsertCategoryMap(r.Context(), gen.UpsertCategoryMapParams{OdooCategoryID: c.OdooCategoryID, Kat: c.Kat}); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	s.audit(r, "category_map.update", "category_map", nil, map[string]any{"rows": len(in)})
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
