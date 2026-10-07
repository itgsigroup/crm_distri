package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/access"
	"distri-arc/internal/httpx"
	"distri-arc/internal/importer"
	"distri-arc/internal/store/gen"
)

// Role master (sidebar → Peran & akses, ADR 0020): roles are fully custom. Anyone with the page reads it; only a
// policy holder (CEO level) changes it, so nobody can widen their own access.
func (s *Server) roleRoutes(r chi.Router) {
	r.Get("/roles", s.listRoles)
	r.Post("/roles", s.saveRole)
	r.Put("/roles/{key}", s.saveRole)
	r.Delete("/roles/{key}", s.deleteRole)
}

func roleView(ro gen.ListRolesRow) map[string]any {
	eff := access.Resolve(ro.Key, ro.Name, ro.Base, ro.Screens, ro.Decide, ro.Scope, ro.Policies, ro.WaAllowed)
	return map[string]any{"key": ro.Key, "name": ro.Name, "description": ro.Description, "base": eff.Base, "active": ro.Active,
		"wa_allowed": eff.WAAllowed, "users": ro.Users, "screens": eff.Screens, "decide": eff.Decide, "scope": eff.Scope, "policies": eff.Policies,
		"updated_by": ro.UpdatedBy, "updated_at": ro.UpdatedAt}
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListRoles(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, ro := range rows {
		items = append(items, roleView(ro))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "screens": access.Screens, "kinds": access.Kinds()})
}

// saveRole creates (POST) or updates (PUT /roles/{key}) a role. The derived base is copied to its users; there is
// always at least one active user with policy rights.
func (s *Server) saveRole(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	var in struct {
		Key         string   `json:"key"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Screens     []string `json:"screens"`
		Decide      []string `json:"decide"`
		Scope       string   `json:"scope"`
		Policies    bool     `json:"policies"`
		WAAllowed   bool     `json:"wa_allowed"`
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
			httpx.Fail(w, http.StatusConflict, "exists", "Peran dengan nama ini sudah ada")
			return
		}
	}
	if in.Name == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama peran")
		return
	}
	ro := access.Normalize(access.Role{Key: key, Name: in.Name, Screens: in.Screens, Decide: in.Decide, Scope: in.Scope, Policies: in.Policies, WAAllowed: in.WAAllowed})
	if len(ro.Screens) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Centang minimal satu halaman")
		return
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	} else if prev != nil {
		active = prev.Active
	}
	if prev != nil && prev.Policies && (!ro.Policies || !active) {
		if n, _ := s.st.Q.PolicyHoldersExcept(r.Context(), gen.PolicyHoldersExceptParams{RoleKey: key, UserID: uuid.Nil}); n == 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Harus tetap ada satu pengguna aktif dengan hak kebijakan (setara CEO)")
			return
		}
	}
	if err := s.st.Q.UpsertRole(r.Context(), gen.UpsertRoleParams{Key: key, Name: in.Name, Description: strings.TrimSpace(in.Description), Base: ro.Base,
		Screens: ro.Screens, Decide: ro.Decide, WaAllowed: ro.WAAllowed, Active: active, Scope: ro.Scope, Policies: ro.Policies, UpdatedBy: u.Email}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.st.Q.SyncRoleBase(r.Context(), gen.SyncRoleBaseParams{RoleKey: &key, Role: ro.Base}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "role.save", "role:"+key, map[string]any{"screens": ro.Screens, "decide": ro.Decide, "scope": ro.Scope, "policies": ro.Policies, "wa_allowed": ro.WAAllowed, "active": active})
	httpx.JSON(w, http.StatusOK, map[string]any{"key": key, "base": ro.Base, "message": "Peran " + in.Name + " disimpan"})
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
		httpx.Fail(w, http.StatusConflict, "in_use", "Peran masih dipakai pengguna — pindahkan penggunanya dulu")
		return
	}
	s.auditUser(r, "role.delete", "role:"+key, nil)
	w.WriteHeader(http.StatusNoContent)
}
