package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/access"
	"distri-arc/internal/httpx"
	"distri-arc/internal/importer"
	"distri-arc/internal/store/gen"
)

// Master peran & akses (ADR 0019): CEO and admin see it, only the CEO changes it.
func (s *Server) roleRoutes(r chi.Router) {
	r.Get("/roles", s.listRoles)
	r.Post("/roles", s.saveRole)
	r.Put("/roles/{key}", s.saveRole)
	r.Delete("/roles/{key}", s.deleteRole)
}

var baseLabel = map[string][2]string{
	"ceo":       {"CEO", "Akses penuh · kebijakan · rilis kredit (tidak bisa dipersempit)"},
	"admin":     {"Admin", "Semua dealer & chat · pengguna · Pengaturan"},
	"finance":   {"Finance", "Semua dealer · kredit, kas, penagihan"},
	"sales":     {"Sales", "Hanya dealer miliknya & chat nomornya sendiri"},
	"warehouse": {"Gudang", "Stok, transfer, PO · grup gudang"},
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Peran & akses untuk CEO dan admin")
		return
	}
	rows, err := s.st.Q.ListRoles(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, ro := range rows {
		eff := access.Resolve(ro.Key, ro.Name, ro.Base, ro.Screens, ro.Decide, ro.WaAllowed)
		items = append(items, map[string]any{"key": ro.Key, "name": ro.Name, "description": ro.Description, "base": ro.Base, "system": ro.System,
			"active": ro.Active, "wa_allowed": eff.WAAllowed, "users": ro.Users, "screens": eff.Screens, "decide": eff.Decide,
			"all_screens": ro.Screens == nil, "all_decide": ro.Decide == nil, "updated_by": ro.UpdatedBy, "updated_at": ro.UpdatedAt})
	}
	bases := []map[string]any{}
	for _, b := range access.Bases {
		bases = append(bases, map[string]any{"key": b, "label": baseLabel[b][0], "desc": baseLabel[b][1], "screens": access.BaseScreens(b), "kinds": access.BaseKinds(b)})
	}
	kinds := []map[string]string{}
	for _, k := range access.BaseKinds("ceo") {
		kinds = append(kinds, map[string]string{"key": k, "label": access.KindLabels[k]})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "bases": bases, "screens": access.Screens, "kinds": kinds})
}

// saveRole creates (POST) or updates (PUT /roles/{key}) a role. Choices outside the base are dropped; a system
// role keeps its base; the CEO role is never narrowed.
func (s *Server) saveRole(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var in struct {
		Key         string   `json:"key"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Base        string   `json:"base"`
		Screens     []string `json:"screens"` // null = all of the base
		Decide      []string `json:"decide"`  // null = all of the base
		WAAllowed   *bool    `json:"wa_allowed"`
		Active      *bool    `json:"active"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	key := chi.URLParam(r, "key")
	var prev *gen.Role
	if key != "" {
		p, err := s.st.Q.GetRole(r.Context(), key)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "not_found", "Peran tidak ditemukan")
			return
		}
		prev = &p
		if in.Name == "" {
			in.Name = p.Name
		}
		if in.Base == "" || p.System {
			in.Base = p.Base
		}
	} else {
		key = strings.TrimSpace(in.Key)
		if key == "" {
			key = importer.Slug(in.Name)
		}
		if !access.ValidKey(key) {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Kode peran: huruf kecil, angka, tanda minus")
			return
		}
		if _, err := s.st.Q.GetRole(r.Context(), key); err == nil {
			httpx.Fail(w, http.StatusConflict, "exists", "Kode peran sudah dipakai")
			return
		}
	}
	if in.Name == "" || !slices.Contains(access.Bases, in.Base) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama peran dan jenis akses dasar")
		return
	}
	if in.Base == "ceo" && (prev == nil || prev.Base != "ceo") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Akses dasar CEO hanya untuk peran CEO bawaan")
		return
	}
	screens, decide := in.Screens, in.Decide
	if in.Base == "ceo" {
		screens, decide = nil, nil
	}
	if screens != nil {
		screens = access.Narrow(in.Base, access.BaseScreens(in.Base), screens)
		if len(screens) == 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih minimal satu menu")
			return
		}
	}
	if decide != nil {
		decide = access.Narrow(in.Base, access.BaseKinds(in.Base), decide)
	}
	wa, active := true, true
	if prev != nil {
		wa, active = prev.WaAllowed, prev.Active
	}
	if in.WAAllowed != nil {
		wa = *in.WAAllowed
	}
	if in.Active != nil {
		active = *in.Active
	}
	if prev != nil && prev.System && !active {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Peran bawaan tidak bisa dinonaktifkan")
		return
	}
	if err := s.st.Q.UpsertRole(r.Context(), gen.UpsertRoleParams{Key: key, Name: in.Name, Description: strings.TrimSpace(in.Description), Base: in.Base,
		Screens: screens, Decide: decide, WaAllowed: wa, Active: active, UpdatedBy: u.Email}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if prev != nil && prev.Base != in.Base {
		if err := s.st.Q.SyncRoleBase(r.Context(), gen.SyncRoleBaseParams{RoleKey: &key, Role: &in.Base}); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	s.auditUser(r, "role.save", "role:"+key, map[string]any{"base": in.Base, "screens": screens, "decide": decide, "wa_allowed": wa, "active": active})
	httpx.JSON(w, http.StatusOK, map[string]any{"key": key, "message": "Peran " + in.Name + " disimpan"})
}

func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ceoOnly(w, r); !ok {
		return
	}
	key := chi.URLParam(r, "key")
	n, err := s.st.Q.DeleteRole(r.Context(), key)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if n == 0 {
		httpx.Fail(w, http.StatusConflict, "in_use", "Peran bawaan atau masih dipakai pengguna tidak bisa dihapus")
		return
	}
	s.auditUser(r, "role.delete", "role:"+key, nil)
	w.WriteHeader(http.StatusNoContent)
}
