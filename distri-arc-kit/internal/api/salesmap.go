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

// Mapping sales (ADR 0024): one Pengguna (login account) ↔ many sales names from BigQuery. The source data spells
// one person several ways ("Granike Monica · Semua cabang", "Granike Monica M. · Semarang"); each name stays a
// profile (it is the import key) and is linked to a user by merging it into that user's main sales profile
// (users.sales_user_id, made on first link): its dealers, conversations, numbers and history move to the user, it
// disappears from every sales list, and the next import keeps it linked.

func (s *Server) salesMapRoutes(r chi.Router) {
	r.Get("/sales-map", s.salesMap)
	r.Post("/sales-map/merge", s.salesMerge)
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
	users, err := s.st.Q.ListMapUsers(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	owner := map[uuid.UUID]uuid.UUID{} // main sales profile → its user
	for _, u := range users {
		if u.SalesUserID != nil {
			if _, taken := owner[*u.SalesUserID]; !taken {
				owner[*u.SalesUserID] = u.ID
			}
		}
	}
	type item struct {
		gen.ListSalesMapRow
		UserID *uuid.UUID `json:"user_id"` // the Pengguna this name belongs to
		Main   bool       `json:"main"`    // it is that user's main profile (not removable)
	}
	items := make([]item, 0, len(rows))
	for _, x := range rows {
		root := x.ID
		if x.MergedInto != nil {
			root = *x.MergedInto
		}
		it := item{ListSalesMapRow: x}
		if u, ok := owner[root]; ok {
			it.UserID, it.Main = &u, root == x.ID
		}
		items = append(items, it)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "users": nonNil(users)})
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
		// UserID links the names to a Pengguna: they merge into the user's main sales profile (made when missing)
		UserID *uuid.UUID `json:"user_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil || len(in.Sources) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih nama sales dari data BigQuery")
		return
	}
	userName := ""
	if in.UserID != nil {
		id, name, msg, err := s.userMainProfile(ctx, *in.UserID, in.Sources)
		if msg != "" {
			httpx.Fail(w, http.StatusBadRequest, "invalid", msg)
			return
		}
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		in.TargetID, userName = &id, name
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
			var holder string // a name that is another user's main profile is not taken from them
			if err := tx.QueryRow(ctx, "select coalesce(name, email, '') from users where sales_user_id = $1 limit 1", src).Scan(&holder); err == nil {
				return errInvalid(row.Name + " adalah profil utama pengguna " + holder + " — lepas dulu dari pengguna itu")
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
	var bad errInvalid
	if errors.As(err, &bad) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", string(bad))
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "sales.merge", "sales_users", map[string]any{"sources": in.Sources, "target": in.TargetID, "user": in.UserID})
	if restored > 0 { // dealers of an un-merged spelling go back to it through the import mapping
		if !s.applyData(w, r, true, deref(u.Email)) {
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"merged": 0, "restored": restored, "message": plural(restored, "nama BigQuery dilepas") + " · dealernya kembali setelah data diproses ulang"})
		return
	}
	msg := plural(merged, "nama sales digabung ke "+target.Name)
	if userName != "" {
		msg = plural(merged, "nama BigQuery dihubungkan ke "+userName)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"merged": merged, "restored": 0, "target_id": target.ID, "message": msg})
}

// userMainProfile is the sales profile a user's BigQuery names merge into: users.sales_user_id, or a new profile
// named after the user (branch of a linked name) when the user has none yet. msg is a refusal for people.
func (s *Server) userMainProfile(ctx context.Context, userID uuid.UUID, sources []uuid.UUID) (uuid.UUID, string, string, error) {
	usr, err := s.st.Q.GetUserByID(ctx, userID)
	if err != nil || !usr.Active {
		return uuid.Nil, "", "Pengguna tidak ditemukan atau nonaktif", nil
	}
	name := deref(usr.Name)
	if name == "" {
		name = deref(usr.Email)
	}
	if usr.SalesUserID != nil {
		p, err := s.st.Q.GetSalesUser(ctx, *usr.SalesUserID)
		if err != nil {
			return uuid.Nil, "", "", err
		}
		if p.MergedInto != nil { // its profile was merged elsewhere: follow to the main one
			return *p.MergedInto, name, "", nil
		}
		if p.Role != "sales" {
			return uuid.Nil, "", "Profil " + p.Name + " milik " + name + " bukan sales", nil
		}
		return p.ID, name, "", nil
	}
	branch := "Semua cabang" // a real branch of one of the names beats "Semua cabang"
	for _, id := range sources {
		if src, err := s.st.Q.GetSalesUser(ctx, id); err == nil && src.Branch != "" && !strings.EqualFold(src.Branch, "Semua cabang") {
			branch = src.Branch
			break
		}
	}
	var id uuid.UUID
	err = s.st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		id, err = q.CreateSalesProfile(ctx, gen.CreateSalesProfileParams{Name: name, Branch: branch, Role: "sales", Email: usr.Email})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "update users set sales_user_id = $2 where id = $1", userID, id)
		return err
	})
	return id, name, "", err
}

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

type errInvalid string

func (e errInvalid) Error() string { return string(e) }

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
