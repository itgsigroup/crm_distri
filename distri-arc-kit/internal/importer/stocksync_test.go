package importer

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"distri-arc/internal/clock"
	"distri-arc/internal/testdb"
)

// Push stok (Sam 19 Oct): between full syncs the stock alone is pulled from BigQuery — only the stock query runs,
// and the snapshot lands in stock_items.
func TestSyncBigQueryStock(t *testing.T) {
	st := testdb.New(t)
	queries := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600}`))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/queries"):
			b, _ := io.ReadAll(r.Body)
			queries = append(queries, string(b))
			_, _ = w.Write([]byte(`{"jobComplete":true,"schema":{"fields":[{"name":"sku","type":"STRING"},{"name":"product","type":"STRING"},{"name":"warehouse","type":"STRING"},{"name":"qty","type":"NUMERIC"},{"name":"unit_cost","type":"NUMERIC"},{"name":"age_days","type":"NUMERIC"}]},"rows":[{"f":[{"v":"IPC-2MP"},{"v":"Kamera IP 2MP"},{"v":"Semarang"},{"v":"42"},{"v":"350000"},{"v":"95"}]}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	sa, err := ParseServiceAccount(testKey(t, srv.URL+"/token"))
	if err != nil {
		t.Fatal(err)
	}
	im := Importer{St: st, Clock: clock.Fixed(time.Date(2026, 10, 19, 10, 0, 0, 0, clock.WIB)), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var cfg SourceConfig
	cfg.Mode = "bigquery"
	cfg.BigQuery.Queries = map[string]string{string(Stock): "select stock", string(Invoices): "select invoices"}
	if cfg.StockEvery() != 5*time.Minute {
		t.Fatalf("default stock interval %s", cfg.StockEvery())
	}
	if _, err := im.SyncBigQueryStock(context.Background(), &BigQuery{SA: sa, BaseURL: srv.URL}, cfg, "test"); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || !strings.Contains(queries[0], "select stock") {
		t.Fatalf("queries run: %v", queries)
	}
	var qty int
	if err := st.Pool.QueryRow(context.Background(), "select qty from stock_items where sku = 'IPC-2MP' and branch = 'Semarang'").Scan(&qty); err != nil || qty != 42 {
		t.Fatalf("stock %d %v", qty, err)
	}
}

// One failing query (a BigQuery table renamed: 404) no longer stops the pull (20 Oct): the other entities are staged
// and applied, the failure is reported and kept as a failed import run.
func TestSyncBigQueryOneQueryFails(t *testing.T) {
	st := testdb.New(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600}`))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/queries"):
			b, _ := io.ReadAll(r.Body)
			if strings.Contains(string(b), "select stock") {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not found: Table accurate_data.master_aging"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"jobComplete":true,"schema":{"fields":[{"name":"code","type":"STRING"},{"name":"name","type":"STRING"},{"name":"branch","type":"STRING"}]},"rows":[{"f":[{"v":"S01"},{"v":"Granike Monica"},{"v":"Jakarta"}]}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	sa, err := ParseServiceAccount(testKey(t, srv.URL+"/token"))
	if err != nil {
		t.Fatal(err)
	}
	im := Importer{St: st, Clock: clock.Fixed(time.Date(2026, 10, 20, 10, 0, 0, 0, clock.WIB)), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var cfg SourceConfig
	cfg.Mode = "bigquery"
	cfg.BigQuery.Queries = map[string]string{string(Stock): "select stock", string(Sales): "select sales"}
	rep, err := im.SyncBigQuery(context.Background(), &BigQuery{SA: sa, BaseURL: srv.URL}, cfg, false, "test")
	if err != nil {
		t.Fatalf("sync stopped on the stock query: %v", err)
	}
	if _, ok := rep.Failed[string(Stock)]; !ok || len(rep.Failed) != 1 {
		t.Fatalf("failed %v", rep.Failed)
	}
	var applied, failed int
	_ = st.Pool.QueryRow(context.Background(), "select count(*) from import_runs where source = 'transform' and entity = 'apply' and status = 'done'").Scan(&applied)
	_ = st.Pool.QueryRow(context.Background(), "select count(*) from import_runs where source = 'bigquery' and entity = 'stock' and status = 'failed'").Scan(&failed)
	if applied != 1 || failed != 1 {
		t.Fatalf("apply runs %d, failed stock runs %d", applied, failed)
	}
}
