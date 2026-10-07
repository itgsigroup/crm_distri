package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"distri-arc/internal/auth"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/domain"
	"distri-arc/internal/httpx"
	"distri-arc/internal/importer"
	"distri-arc/internal/jobs"
	"distri-arc/internal/policy"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/wa"
)

// Pengaturan → Data & master: the real-data source (BigQuery / CSV), master-data mappings, customer master and the
// sales team. CEO configures the source and its key; CEO and admin import, map and edit master data.
func (s *Server) dataRoutes(r chi.Router) {
	r.Get("/data/status", s.dataStatus)
	r.Put("/data/source", s.dataSource)
	r.Put("/data/credentials", s.dataCredentials)
	r.Delete("/data/credentials", s.dataCredentialsDelete)
	r.Post("/data/test", s.dataTest)
	r.Get("/data/schema", s.dataSchema)
	r.Post("/data/sync", s.dataSync)
	r.Post("/data/apply", s.dataApply)
	r.Post("/data/import/{entity}", s.dataImport)
	r.Get("/data/template/{entity}", s.dataTemplate)
	r.Get("/data/mappings", s.dataMappings)
	r.Put("/data/mappings", s.dataSetMappings)
	r.Get("/data/customers", s.dataCustomers)
	r.Patch("/data/customers/{id}", s.dataCustomer)
	r.Get("/data/team", s.dataTeam)
	r.Post("/data/team", s.dataTeamCreate)
	r.Put("/data/team/{id}", s.dataTeamUpdate)
	r.Get("/data/branches", s.dataBranches)
}

func (s *Server) importer() importer.Importer {
	return importer.Importer{St: s.st, Clock: s.clock, Log: s.log}
}

func (s *Server) adminOnly(w http.ResponseWriter, r *http.Request) (User, bool) {
	u, _ := CurrentUser(r.Context())
	if !isAdmin(u) {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Data & master untuk CEO dan admin")
		return u, false
	}
	return u, true
}

func (s *Server) ceoOnly(w http.ResponseWriter, r *http.Request) (User, bool) {
	u, _ := CurrentUser(r.Context())
	if deref(u.Role) != "ceo" {
		httpx.Fail(w, http.StatusForbidden, "forbidden", "Sumber data dan kuncinya diatur CEO")
		return u, false
	}
	return u, true
}

func (s *Server) dataStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	ctx := r.Context()
	im := s.importer()
	cfg, _ := im.LoadConfig(ctx)
	cred := map[string]any{"set": false}
	if bq, err := im.BigQueryClient(ctx, cfg, s.secret()); err == nil {
		cred = map[string]any{"set": true, "client_email": bq.SA.ClientEmail, "project_id": bq.SA.ProjectID}
	} else if !errors.Is(err, importer.ErrNoCredentials) {
		cred = map[string]any{"set": true, "error": err.Error()}
	}
	staged, _ := s.st.Q.CountImportRows(ctx)
	counts, _ := s.st.Q.MappingCounts(ctx)
	runs, _ := s.st.Q.ListImportRuns(ctx, 12)
	contract := map[string][]importer.Column{}
	for _, e := range importer.Entities {
		contract[string(e)] = importer.Contract[e]
	}
	var dealers, invoices int64
	_ = s.st.Pool.QueryRow(ctx, "select (select count(*) from dealers), (select count(*) from invoices)").Scan(&dealers, &invoices)
	httpx.JSON(w, http.StatusOK, map[string]any{"source": cfg, "credentials": cred, "staged": nonNil(staged), "mappings": nonNil(counts), "runs": nonNil(runs),
		"contract": contract, "entities": importer.Entities, "categories": domain.Categories, "dealers": dealers, "invoices": invoices})
}

func (s *Server) dataSource(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	if _, err := policy.Save(r.Context(), s.st, "data.source", body, u.SalesUserID, deref(u.Email), time.Now()); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": "Sumber data disimpan"})
}

func (s *Server) dataCredentials(w http.ResponseWriter, r *http.Request) {
	u, ok := s.ceoOnly(w, r)
	if !ok {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "File terlalu besar")
		return
	}
	sa, err := importer.ParseServiceAccount(raw)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	sealed, err := auth.Seal(string(raw), s.secret())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.st.Q.SetSecret(r.Context(), gen.SetSecretParams{Key: importer.SecretKey, Value: sealed, UpdatedBy: u.Email}); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditUser(r, "data.credentials_set", "secret", map[string]any{"client_email": sa.ClientEmail, "project_id": sa.ProjectID})
	httpx.JSON(w, http.StatusOK, map[string]any{"client_email": sa.ClientEmail, "project_id": sa.ProjectID, "message": "Kunci BigQuery disimpan terenkripsi"})
}

func (s *Server) dataCredentialsDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ceoOnly(w, r); !ok {
		return
	}
	_ = s.st.Q.DeleteSecret(r.Context(), importer.SecretKey)
	s.auditUser(r, "data.credentials_deleted", "secret", nil)
	w.WriteHeader(http.StatusNoContent)
}

// dataTest runs each configured query with LIMIT 5 and checks the columns against the contract.
func (s *Server) dataTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	ctx := r.Context()
	im := s.importer()
	cfg, _ := im.LoadConfig(ctx)
	bq, err := im.BigQueryClient(ctx, cfg, s.secret())
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "no_credentials", err.Error())
		return
	}
	out := map[string]any{}
	for _, e := range importer.Entities {
		sql := strings.TrimSpace(cfg.BigQuery.Queries[string(e)])
		if sql == "" {
			continue
		}
		rows, err := bq.Query(ctx, "select * from ("+strings.TrimRight(sql, "; \n")+") limit 5")
		res := map[string]any{"ok": err == nil}
		if err != nil {
			res["error"] = err.Error()
		} else {
			var missing []string
			if len(rows) > 0 {
				for _, c := range importer.Contract[e] {
					if _, has := rows[0][c.Name]; c.Required && !has {
						missing = append(missing, c.Name)
					}
				}
			}
			res["rows"], res["missing"], res["sample"] = len(rows), nonNil(missing), rows
			res["ok"] = len(missing) == 0
		}
		out[string(e)] = res
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"results": out})
}

// dataSchema lists the BigQuery project's tables and columns (read-only) and proposes, per entity, a table, a
// column mapping and the import SQL. Nothing is saved: the person reviews and saves the queries.
func (s *Server) dataSchema(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	ctx := r.Context()
	im := s.importer()
	cfg, _ := im.LoadConfig(ctx)
	bq, err := im.BigQueryClient(ctx, cfg, s.secret())
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "no_credentials", err.Error())
		return
	}
	tables, err := bq.Schema(ctx, 400)
	if err != nil {
		httpx.Fail(w, http.StatusBadGateway, "bigquery", err.Error())
		return
	}
	project := bq.Project
	if project == "" {
		project = bq.SA.ProjectID
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"project": project, "tables": nonNil(tables), "suggestions": importer.Suggest(project, tables)})
}

func (s *Server) enqueue(w http.ResponseWriter, r *http.Request, args interface{ Kind() string }) bool {
	if s.jobs == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "no_queue", "Antrean tidak tersedia")
		return false
	}
	var err error
	switch a := args.(type) {
	case jobs.DataSyncArgs:
		_, err = s.jobs.Insert(r.Context(), a, nil)
	case jobs.DataApplyArgs:
		_, err = s.jobs.Insert(r.Context(), a, nil)
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return false
	}
	return true
}

// applyData queues the transform (worker); without a queue (tests, ctl-less setups) it runs here.
func (s *Server) applyData(w http.ResponseWriter, r *http.Request, full bool, by string) bool {
	if s.jobs != nil {
		return s.enqueue(w, r, jobs.DataApplyArgs{Full: full, By: by})
	}
	if _, err := s.importer().Apply(r.Context(), full, by); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return false
	}
	return true
}

func (s *Server) dataSync(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	full := r.URL.Query().Get("full") == "1"
	if s.enqueue(w, r, jobs.DataSyncArgs{Force: true, Full: full, By: deref(u.Email)}) {
		httpx.JSON(w, http.StatusAccepted, map[string]any{"message": "Sinkron BigQuery dimulai · hasilnya muncul di riwayat impor"})
	}
}

func (s *Server) dataApply(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	if s.enqueue(w, r, jobs.DataApplyArgs{Full: true, By: deref(u.Email)}) {
		httpx.JSON(w, http.StatusAccepted, map[string]any{"message": "Data diproses ulang dengan mapping terbaru"})
	}
}

func entityParam(w http.ResponseWriter, r *http.Request) (importer.Entity, bool) {
	e := importer.Entity(strings.TrimSuffix(chi.URLParam(r, "entity"), ".csv"))
	if _, ok := importer.Contract[e]; !ok {
		httpx.Fail(w, http.StatusNotFound, "unknown_entity", "Jenis data tidak dikenal")
		return e, false
	}
	return e, true
}

// dataImport stages a CSV file (body or multipart "file") and queues the transform.
func (s *Server) dataImport(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	e, ok := entityParam(w, r)
	if !ok {
		return
	}
	var src io.Reader = http.MaxBytesReader(w, r.Body, 64<<20)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		f, _, err := r.FormFile("file")
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih file CSV")
			return
		}
		defer f.Close()
		src = f
	}
	rows, err := importer.ParseCSV(src)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if len(rows) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "File kosong")
		return
	}
	rep, err := s.importer().Stage(r.Context(), e, rows, "csv", deref(u.Email))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if rep.Staged > 0 && !s.applyData(w, r, false, deref(u.Email)) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"report": rep, "message": fmt.Sprintf("%d baris diterima, %d dilewati · diproses di latar belakang", rep.Staged, rep.Skipped)})
}

func (s *Server) dataTemplate(w http.ResponseWriter, r *http.Request) {
	e, ok := entityParam(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="template-`+string(e)+`.csv"`)
	_, _ = w.Write([]byte(importer.Template(e)))
}

func (s *Server) dataMappings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	rows, err := s.st.Q.ListMappings(r.Context(), r.URL.Query().Get("kind"))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	type item struct {
		gen.DataMapping
		Suggested string `json:"suggested,omitempty"`
	}
	items := make([]item, 0, len(rows))
	for _, m := range rows {
		it := item{DataMapping: m}
		if m.Target == nil {
			it.Suggested = importer.SuggestTarget(m.Kind, m.SourceValue)
		}
		items = append(items, it)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// dataSetMappings saves mappings (target "" clears) and re-applies the staged data with them.
func (s *Server) dataSetMappings(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	var in []struct {
		Kind        string `json:"kind"`
		SourceValue string `json:"source_value"`
		Target      string `json:"target"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil || len(in) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Kirim daftar mapping")
		return
	}
	for _, m := range in {
		t := strings.TrimSpace(m.Target)
		switch m.Kind {
		case "category":
			if t != "" && domain.CategoryIndex(t) < 0 {
				httpx.Fail(w, http.StatusBadRequest, "invalid", "Kategori harus salah satu dari 6 kategori product mix")
				return
			}
		case "ctype":
			if t != "" && t != "reseller" && t != "si" {
				httpx.Fail(w, http.StatusBadRequest, "invalid", "Jenis pelanggan: reseller atau si")
				return
			}
		case "sales":
			if t != "" {
				if _, err := uuid.Parse(t); err != nil {
					httpx.Fail(w, http.StatusBadRequest, "invalid", "Pilih sales dari tim")
					return
				}
			}
		case "branch", "warehouse":
		default:
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Jenis mapping tidak dikenal")
			return
		}
		var tp *string
		if t != "" {
			tp = &t
		}
		if err := s.st.Q.SetMapping(r.Context(), gen.SetMappingParams{Kind: m.Kind, SourceValue: m.SourceValue, Target: tp, UpdatedBy: u.Email}); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	s.auditUser(r, "data.mappings", "data_mappings", map[string]any{"count": len(in)})
	if !s.applyData(w, r, true, deref(u.Email)) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": fmt.Sprintf("%d mapping disimpan · data diproses ulang", len(in))})
}

func (s *Server) dataCustomers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	q := r.URL.Query()
	lim, _ := strconv.Atoi(q.Get("limit"))
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	off, _ := strconv.Atoi(q.Get("offset"))
	rows, err := s.st.Q.MasterCustomers(r.Context(), gen.MasterCustomersParams{Q: strings.TrimSpace(q.Get("q")), Ctype: q.Get("type"), Incomplete: q.Get("incomplete") == "1",
		Lim: int32(lim), Off: int32(max(off, 0))})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

// dataCustomer edits a customer's master fields; edited fields are kept on the next import (master_locked).
func (s *Server) dataCustomer(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Pelanggan tidak ditemukan")
		return
	}
	var in struct {
		CustomerType     *string    `json:"customer_type"`
		Tier             *string    `json:"tier"`
		CreditLimit      *int64     `json:"credit_limit"`
		PaymentTermsDays *int32     `json:"payment_terms_days"`
		OwnerID          *uuid.UUID `json:"owner_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	var locked []string
	if in.CustomerType != nil {
		if *in.CustomerType != "reseller" && *in.CustomerType != "si" {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Jenis: reseller atau si")
			return
		}
		locked = append(locked, "customer_type")
	}
	if in.Tier != nil {
		if !slices.Contains([]string{"A", "B", "C"}, *in.Tier) {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Tier A, B atau C")
			return
		}
		locked = append(locked, "tier")
	}
	if in.CreditLimit != nil {
		if *in.CreditLimit < 0 {
			httpx.Fail(w, http.StatusBadRequest, "invalid", "Limit tidak boleh negatif")
			return
		}
		locked = append(locked, "credit_limit")
	}
	if in.PaymentTermsDays != nil {
		locked = append(locked, "payment_terms_days")
	}
	if in.OwnerID != nil {
		locked = append(locked, "owner_id")
	}
	if err := s.st.Q.UpdateCustomerMaster(r.Context(), gen.UpdateCustomerMasterParams{ID: id, CustomerType: in.CustomerType, Tier: in.Tier, CreditLimit: in.CreditLimit,
		PaymentTermsDays: in.PaymentTermsDays, OwnerID: in.OwnerID, Locked: locked}); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if _, err := dealersvc.New(s.st, s.clock).Recompute(r.Context(), id); err != nil {
		s.log.Warn("recompute after master edit", "err", err)
	}
	s.auditUser(r, "data.customer", "dealer", map[string]any{"id": id, "fields": locked})
	httpx.JSON(w, http.StatusOK, map[string]any{"message": "Master pelanggan disimpan · tidak tertimpa impor berikutnya"})
}

func (s *Server) dataTeam(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	rows, err := s.st.Q.ListTeam(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}

type teamBody struct {
	Name     string `json:"name"`
	Branch   string `json:"branch"`
	WANumber string `json:"wa_number"`
	Role     string `json:"role"`
	Active   *bool  `json:"active"`
}

func (s *Server) dataTeamCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	var in teamBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Branch) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama dan cabang")
		return
	}
	if in.Role == "" {
		in.Role = "sales"
	}
	if !slices.Contains(auth.Roles, in.Role) {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Peran tidak dikenal")
		return
	}
	var no *string
	if d := wa.Digits(in.WANumber); d != "" {
		no = &d
	}
	id, err := s.st.Q.CreateSalesProfile(r.Context(), gen.CreateSalesProfileParams{Name: strings.TrimSpace(in.Name), Branch: strings.TrimSpace(in.Branch), WaNumber: no, Role: in.Role})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nomor WhatsApp sudah dipakai anggota lain")
		return
	}
	s.auditUser(r, "data.team_create", "sales_user", map[string]any{"id": id, "name": in.Name})
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Anggota tim ditambahkan"})
}

func (s *Server) dataTeamUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOnly(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Anggota tidak ditemukan")
		return
	}
	var in teamBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Branch) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi nama dan cabang")
		return
	}
	var no *string
	if d := wa.Digits(in.WANumber); d != "" {
		no = &d
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	if err := s.st.Q.UpdateSalesProfile(r.Context(), gen.UpdateSalesProfileParams{ID: id, Name: strings.TrimSpace(in.Name), Branch: strings.TrimSpace(in.Branch), WaNumber: no, Active: active}); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Nomor WhatsApp sudah dipakai anggota lain")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": "Anggota tim diperbarui"})
}

func (s *Server) dataBranches(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.Branches(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(rows)})
}
