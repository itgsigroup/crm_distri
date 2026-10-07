package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// Branch master (sidebar → Cabang): every signed-in user reads the list (pickers); the Cabang page changes it.
func (s *Server) branchRoutes(r chi.Router) {
	r.Get("/branches", s.listBranches)
	r.Post("/branches", s.saveBranch)
	r.Put("/branches/{id}", s.saveBranch)
}

func (s *Server) listBranches(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListBranches(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

// saveBranch adds (POST) or changes (PUT) a branch; a rename carries over to people, dealers and source mappings.
func (s *Server) saveBranch(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	if !can(u, "branches") {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Peran Anda tidak membuka halaman Cabang")
		return
	}
	var in struct {
		Name    string `json:"name"`
		City    string `json:"city"`
		Address string `json:"address"`
		Active  *bool  `json:"active"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama cabang")
		return
	}
	name := strings.TrimSpace(in.Name)
	if strings.EqualFold(name, "Semua cabang") {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "\"Semua cabang\" bukan nama cabang")
		return
	}
	ctx := r.Context()
	if other, err := s.st.Q.BranchByName(ctx, name); err == nil && chi.URLParam(r, "id") != other.ID.String() {
		httpx.Fail(w, http.StatusConflict, "exists", "Cabang "+other.Name+" sudah ada")
		return
	}
	if chi.URLParam(r, "id") == "" {
		id, err := s.st.Q.InsertBranch(ctx, gen.InsertBranchParams{Name: name, City: strings.TrimSpace(in.City), Address: strings.TrimSpace(in.Address), UpdatedBy: u.Email})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		s.auditUser(r, "branch.create", "branch:"+id.String(), map[string]any{"name": name})
		httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Cabang " + name + " ditambahkan"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Cabang tidak ditemukan")
		return
	}
	cur, err := s.st.Q.GetBranch(ctx, id)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Cabang tidak ditemukan")
		return
	}
	active := cur.Active
	if in.Active != nil {
		active = *in.Active
	}
	err = s.st.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		if err := q.UpdateBranch(ctx, gen.UpdateBranchParams{ID: id, Name: name, City: strings.TrimSpace(in.City), Address: strings.TrimSpace(in.Address), Active: active, UpdatedBy: u.Email}); err != nil {
			return err
		}
		if cur.Name != name {
			return q.RenameBranchRefs(ctx, gen.RenameBranchRefsParams{NewName: name, OldName: cur.Name})
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "branch.update", "branch:"+id.String(), map[string]any{"name": name, "renamed_from": cur.Name, "active": active})
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "message": "Cabang " + name + " disimpan"})
}
