package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/httpx"
	"distri-arc/internal/importer"
	"distri-arc/internal/replyrule"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// Cek stok (21 Oct): one product's stock as GSI Orbit and the AI Trigger read it — per branch, the source rows per
// warehouse behind it, and what the "Tanya stok" card answers — next to the real stock a user counts (Accurate, the
// warehouse). A difference in the source rows is the data (BigQuery query, Gudang mapping); a difference only in the
// answer is the AI Trigger.
func (s *Server) stockCheckRoutes(r chi.Router) {
	r.Get("/stock/products", s.stockProducts)
	r.Get("/stock/check", s.stockCheck)
	r.Get("/stock/checks", s.stockChecks)
	r.Post("/stock/checks", s.stockCheckSave)
}

// StockProduct is one row of Cek stok's table: a product's stock in total and per branch, and its last real count.
type StockProduct struct {
	SKU       string           `json:"sku"`
	Name      string           `json:"name"`
	Category  string           `json:"category"`
	Qty       int64            `json:"qty"`
	ByBranch  map[string]int64 `json:"by_branch"`
	LastCheck *LastStockCheck  `json:"last_check"`
}

// LastStockCheck is a product's last saved count: when, how many branches, how many differed.
type LastStockCheck struct {
	At       time.Time `json:"at"`
	Branches int64     `json:"branches"`
	Off      int64     `json:"off"`
}

// stockProducts is every product in the stock (search, sort and pages are the table's, in the browser).
func (s *Server) stockProducts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.Q.ListStockProducts(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	checks, err := s.st.Q.LastStockChecks(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	last := map[string]*LastStockCheck{}
	for _, c := range checks {
		last[c.Sku] = &LastStockCheck{At: c.CheckedAt, Branches: c.Branches, Off: c.Off}
	}
	out := make([]StockProduct, 0, len(rows))
	for _, x := range rows {
		p := StockProduct{SKU: x.Sku, Name: x.Name, Category: x.Category, Qty: x.Qty, ByBranch: map[string]int64{}, LastCheck: last[strings.ToLower(x.Sku)]}
		_ = json.Unmarshal(x.ByBranch, &p.ByBranch)
		out = append(out, p)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// StockCheckBranch is one branch of the product: what GSI Orbit reads, what the source rows mapped to it add up to,
// and the last real count.
type StockCheckBranch struct {
	Branch    string          `json:"branch"`
	ArcQty    int             `json:"arc_qty"`
	SourceQty int64           `json:"source_qty"`
	LastCheck *gen.StockCheck `json:"last_check"`
}

// stockCheck is one product (?sku=) for Cek stok; ?branch= is the conversation branch the AI answer is for ("" =
// every branch, as for a sales covering every branch).
func (s *Server) stockCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sku := strings.TrimSpace(r.URL.Query().Get("sku"))
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	items, err := s.st.Q.StockItemsBySKU(ctx, sku)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	src, err := importer.Importer{St: s.st}.StockSource(ctx, sku)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if sku == "" || (len(items) == 0 && len(src) == 0) {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Produk tidak ada di data stok")
		return
	}
	checks, err := s.st.Q.ListStockChecks(ctx, gen.ListStockChecksParams{Sku: sku, Lim: 200})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	masters, err := s.st.Q.Branches(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	product := ""
	by := map[string]*StockCheckBranch{}
	at := func(b string) *StockCheckBranch {
		x := by[b]
		if x == nil {
			x = &StockCheckBranch{Branch: b}
			by[b] = x
		}
		return x
	}
	for _, b := range masters {
		at(b)
	}
	for _, it := range items {
		product = it.Name
		at(it.Branch).ArcQty += int(it.Qty)
	}
	for _, x := range src {
		if product == "" {
			product = x.Product
		}
		at(x.Branch).SourceQty += x.Qty
	}
	for i := range checks { // newest first: the first of a branch is its last count
		if x := at(checks[i].Branch); x.LastCheck == nil {
			x.LastCheck = &checks[i]
		}
	}
	branches := make([]StockCheckBranch, 0, len(by))
	for _, x := range by {
		branches = append(branches, *x)
	}
	sort.Slice(branches, func(i, j int) bool {
		a, b := branches[i], branches[j]
		if (a.ArcQty > 0 || a.SourceQty > 0) != (b.ArcQty > 0 || b.SourceQty > 0) {
			return a.ArcQty > 0 || a.SourceQty > 0
		}
		return strings.ToLower(a.Branch) < strings.ToLower(b.Branch)
	})

	answer, err := s.stockAnswer(r, sku, branch)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	p := importer.ProvenanceOf(ctx, s.st)
	if len(checks) > 30 {
		checks = checks[:30]
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sku": sku, "product": product, "branch": branch, "branches": branches,
		"source_rows": src, "answer": answer, "source": p.Label, "synced_at": p.SyncedAt, "checks": checks})
}

// stockAnswer is the "Tanya stok" card for sku in a conversation of branch, from the same rule and data as Chat.
func (s *Server) stockAnswer(r *http.Request, sku, branch string) (*replyrule.Suggestion, error) {
	ctx := r.Context()
	rule := replyrule.Rule{ID: "", Name: "Tanya stok", Action: replyrule.ActionStock, ShowQty: true,
		TemplateReady: "Ready {jumlah} unit {produk} di {cabang}.", TemplateEmpty: "Mohon maaf, {produk} di {cabang} sedang kosong."}
	if rules, err := s.replyAI.Rules(ctx); err == nil {
		for _, x := range rules {
			if x.Action == replyrule.ActionStock {
				rule = x
				break
			}
		}
	}
	all, err := s.st.Q.ListStockItems(ctx)
	if err != nil {
		return nil, err
	}
	return replyrule.StockFor(rule, sku, replyrule.Data{Branch: branch, Stock: views.StockItems(all)}), nil
}

// stockChecks is the history of real counts (?sku= one product; ?limit=, 100 by default).
func (s *Server) stockChecks(w http.ResponseWriter, r *http.Request) {
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 || lim > 500 {
		lim = 100
	}
	rows, err := s.st.Q.ListStockChecks(r.Context(), gen.ListStockChecksParams{Sku: strings.TrimSpace(r.URL.Query().Get("sku")), Lim: int32(lim)})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, rows)
}

// stockCheckSave stores the real counts of one product, one row per branch; GSI Orbit's quantity is read here, not
// taken from the page, so the difference is against what the AI read at that moment.
func (s *Server) stockCheckSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		SKU    string `json:"sku"`
		Note   string `json:"note"`
		Counts []struct {
			Branch  string `json:"branch"`
			RealQty *int   `json:"real_qty"`
		} `json:"counts"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); err != nil || strings.TrimSpace(in.SKU) == "" {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Body tidak valid")
		return
	}
	items, err := s.st.Q.StockItemsBySKU(ctx, strings.TrimSpace(in.SKU))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	src, err := importer.Importer{St: s.st}.StockSource(ctx, in.SKU)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if len(items) == 0 && len(src) == 0 {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Produk tidak ada di data stok")
		return
	}
	sku, product := strings.TrimSpace(in.SKU), ""
	arc := map[string]int{}
	for _, it := range items {
		sku, product = it.Sku, it.Name
		arc[strings.ToLower(it.Branch)] += int(it.Qty)
	}
	if product == "" {
		product = src[0].Product
	}
	u, _ := CurrentUser(ctx)
	synced := importer.ProvenanceOf(ctx, s.st).SyncedAt
	note := strings.TrimSpace(in.Note)
	if len([]rune(note)) > 500 {
		note = string([]rune(note)[:500])
	}
	saved := 0
	err = s.st.Tx(ctx, func(q *gen.Queries, _ pgx.Tx) error {
		for _, c := range in.Counts {
			b := strings.TrimSpace(c.Branch)
			if b == "" || c.RealQty == nil || *c.RealQty < 0 {
				continue
			}
			if _, err := q.InsertStockCheck(ctx, gen.InsertStockCheckParams{Sku: sku, Product: product, Branch: b, ArcQty: int32(arc[strings.ToLower(b)]),
				RealQty: int32(*c.RealQty), Note: note, SyncedAt: synced, CheckedBy: &u.ID, CheckedByName: deref(u.Name)}); err != nil {
				return err
			}
			saved++
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if saved == 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid", "Isi stok real minimal satu cabang")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"saved": saved, "at": time.Now().UTC()})
}
