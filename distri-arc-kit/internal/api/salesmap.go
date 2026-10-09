package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/store/gen"
)

// Mapping sales (ADR 0024): the source data spells one person several ways ("Granike Monica · Semua cabang",
// "Granike Monica M. · Semarang"). Each spelling stays a profile (it is the import key) but can be merged into the
// GSI Orbit sales it belongs to: its dealers, conversations, numbers and history move there, it disappears from
// every sales list, and the next import keeps it merged.

func (s *Server) salesMapRoutes(r chi.Router) {
	r.Get("/sales-map", s.salesMap)
	r.Post("/sales-map/merge", s.salesMerge)
	r.Post("/sales-map/profiles", s.salesMapCreate)
}

// salesMapCreate adds the GSI Orbit sales that source spellings are merged into (e.g. "Granike Monika").
func (s *Server) salesMapCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.salesMapper(w, r); !ok {
		return
	}
	var in struct {
		Name   string `json:"name"`
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama sales")
		return
	}
	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = "Semua cabang"
	}
	id, err := s.st.Q.CreateSalesProfile(r.Context(), gen.CreateSalesProfileParams{Name: strings.TrimSpace(in.Name), Branch: branch, Role: "sales"})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "sales.create", "sales_users", map[string]any{"id": id, "name": in.Name})
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Sales " + strings.TrimSpace(in.Name) + " ditambahkan"})
}

func (s *Server) salesMapper(w http.ResponseWriter, r *http.Request) (User, bool) {
	u, _ := CurrentUser(r.Context())
	if !can(u, "users") && !can(u, "conn") {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Mapping sales untuk pemegang menu Pengguna atau Pengaturan")
		return u, false
	}
	return u, true
}

func (s *Server) salesMap(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.salesMapper(w, r); !ok {
		return
	}
	rows, err := s.st.Q.ListSalesMap(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

// salesMerge merges sources into a target (target_id null: un-merge them — each becomes its own sales again).
func (s *Server) salesMerge(w http.ResponseWriter, r *http.Request) {
	u, ok := s.salesMapper(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var in struct {
		Sources  []uuid.UUID `json:"sources"`
		TargetID *uuid.UUID  `json:"target_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil || len(in.Sources) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih nama sales dari data sumber")
		return
	}
	var target gen.SalesUser
	if in.TargetID != nil {
		t, err := s.st.Q.GetSalesUser(ctx, *in.TargetID)
		if err != nil || t.Role != "sales" {
			httpx.Fail(w, http.StatusNotFound, "not_found", "Sales tujuan tidak ditemukan")
			return
		}
		if t.MergedInto != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid", t.Name+" sudah digabung ke sales lain — pilih sales utamanya")
			return
		}
		target = t
	}
	merged, restored := 0, 0
	err := s.st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		for _, src := range in.Sources {
			row, err := q.GetSalesUser(ctx, src)
			if err != nil {
				return errNotFound("Sales " + src.String() + " tidak ditemukan")
			}
			if in.TargetID == nil { // un-merge: its own sales again; its dealers come back with the next import
				if row.MergedInto == nil {
					continue
				}
				if _, err := tx.Exec(ctx, "update sales_users set merged_into = null, active = true where id = $1", src); err != nil {
					return err
				}
				if err := mapSource(ctx, tx, row, src, u); err != nil {
					return err
				}
				restored++
				continue
			}
			if src == target.ID {
				continue
			}
			// whatever was merged into this source follows it to the target
			if _, err := tx.Exec(ctx, "update sales_users set merged_into = $2 where merged_into = $1", src, target.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "update sales_users set merged_into = $2, active = false where id = $1", src, target.ID); err != nil {
				return err
			}
			if err := moveSales(ctx, tx, src, target.ID); err != nil {
				return err
			}
			if err := mapSource(ctx, tx, row, target.ID, u); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "update data_mappings set target = $2, updated_by = $3, updated_at = now() where kind = 'sales' and target = $1",
				src.String(), target.ID.String(), u.Email); err != nil {
				return err
			}
			merged++
		}
		return nil
	})
	var nf errNotFound
	if errors.As(err, &nf) {
		httpx.Fail(w, http.StatusNotFound, "not_found", string(nf))
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "sales.merge", "sales_users", map[string]any{"sources": in.Sources, "target": in.TargetID})
	if restored > 0 { // dealers of an un-merged spelling go back to it through the import mapping
		if !s.applyData(w, r, true, deref(u.Email)) {
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"merged": 0, "restored": restored, "message": plural(restored, "nama sales dipisah lagi") + " · dealernya kembali setelah data diproses ulang"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"merged": merged, "restored": 0, "message": plural(merged, "nama sales digabung ke "+target.Name)})
}

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

func plural(n int, what string) string { return fmt.Sprintf("%d %s", n, what) }

// moveSales gives everything of one sales profile to another (decisions keep who decided).
func moveSales(ctx context.Context, tx pgx.Tx, from, to uuid.UUID) error {
	for _, q := range []string{
		"update dealers set owner_id = $2 where owner_id = $1",
		"update chat_threads t set sales_id = $2 where sales_id = $1 and not exists (select 1 from chat_threads x where x.sales_id = $2 and x.wa_jid = t.wa_jid)",
		"update wa_numbers set sales_id = $2 where sales_id = $1",
		"update signals set sales_id = $2 where sales_id = $1",
		"update users set sales_user_id = $2 where sales_user_id = $1 and not exists (select 1 from users x where x.sales_user_id = $2)",
		`insert into interactions_monthly (sales_id, dealer_id, month, n, source, as_of)
		   select $2, dealer_id, month, n, source, as_of from interactions_monthly where sales_id = $1
		 on conflict (sales_id, dealer_id, month, source) do update set n = interactions_monthly.n + excluded.n,
		   as_of = greatest(interactions_monthly.as_of, excluded.as_of)`,
		"delete from interactions_monthly where sales_id = $1 and $2::uuid is not null",
		"update sales_users set wa_number = (select wa_number from sales_users where id = $1) where id = $2 and wa_number is null",
	} {
		if _, err := tx.Exec(ctx, q, from, to); err != nil {
			return err
		}
	}
	return nil
}

// mapSource points the import mapping of a profile's source spellings (code and name) at a sales.
func mapSource(ctx context.Context, tx pgx.Tx, row gen.SalesUser, to uuid.UUID, u User) error {
	for _, raw := range []*string{row.SourceID, &row.Name} {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `insert into data_mappings (kind, source_value, target, updated_by, updated_at) values ('sales', $1, $2, $3, now())
			on conflict (kind, source_value) do update set target = excluded.target, updated_by = excluded.updated_by, updated_at = now()`,
			strings.TrimSpace(*raw), to.String(), u.Email); err != nil {
			return err
		}
	}
	return nil
}
