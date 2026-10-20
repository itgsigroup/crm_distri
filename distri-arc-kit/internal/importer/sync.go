package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"distri-arc/internal/auth"
	"distri-arc/internal/store/gen"
)

// SourceConfig is policy "data.source": where real data comes from and, for BigQuery, one read-only query per
// entity whose columns follow the Contract (SELECT kode AS code, …).
type SourceConfig struct {
	Mode     string `json:"mode"` // none | bigquery | csv
	BigQuery struct {
		ProjectID   string `json:"project_id"`
		Location    string `json:"location"`
		SyncMinutes int    `json:"sync_minutes"`
		// StockMinutes: the stock-only pull between full syncs (Push stok, Sam 19 Oct); 0 = every 5 minutes
		StockMinutes int               `json:"stock_minutes,omitempty"`
		Queries      map[string]string `json:"queries"` // entity → SQL
	} `json:"bigquery"`
}

// SecretKey is where the sealed service account key lives (table secrets).
const SecretKey = "bigquery.service_account"

// ErrNoCredentials means the BigQuery key was not uploaded yet.
var ErrNoCredentials = errors.New("kunci service account BigQuery belum diunggah")

// LoadConfig reads policy data.source.
func (im Importer) LoadConfig(ctx context.Context) (SourceConfig, error) {
	var c SourceConfig
	raw, err := im.St.Q.GetPolicy(ctx, "data.source")
	if err != nil {
		return SourceConfig{Mode: "none"}, nil
	}
	err = json.Unmarshal(raw.Value, &c)
	return c, err
}

// BigQueryClient opens the client with the sealed key (SESSION_SECRET).
func (im Importer) BigQueryClient(ctx context.Context, cfg SourceConfig, sessionSecret []byte) (*BigQuery, error) {
	sealed, err := im.St.Q.GetSecret(ctx, SecretKey)
	if err != nil {
		return nil, ErrNoCredentials
	}
	plain, err := auth.Open(sealed, sessionSecret)
	if err != nil {
		return nil, fmt.Errorf("kunci BigQuery tidak bisa dibuka (SESSION_SECRET berubah?): %w", err)
	}
	sa, err := ParseServiceAccount([]byte(plain))
	if err != nil {
		return nil, err
	}
	return &BigQuery{SA: sa, Project: cfg.BigQuery.ProjectID, Location: cfg.BigQuery.Location}, nil
}

// SyncReport is one BigQuery pull.
type SyncReport struct {
	Staged map[string]StageReport `json:"staged"`
	Apply  ApplyReport            `json:"apply"`
	// Failed: the entities whose query or staging failed this pull, with the error (the others were still applied)
	Failed map[string]string `json:"failed,omitempty"`
}

// SyncBigQuery runs every configured query, stages the rows and applies them. One failing query (a table renamed in
// BigQuery) no longer stops the rest: until 20 Oct the stock query's 404 skipped Apply on every pull, so invoices,
// payments and dealers stayed at 7 Oct. The failure is recorded as a failed import run and returned in Failed.
func (im Importer) SyncBigQuery(ctx context.Context, bq *BigQuery, cfg SourceConfig, full bool, by string) (SyncReport, error) {
	rep := SyncReport{Staged: map[string]StageReport{}, Failed: map[string]string{}}
	ran := 0
	for _, e := range Entities {
		sql := strings.TrimSpace(cfg.BigQuery.Queries[string(e)])
		if sql == "" {
			continue
		}
		rows, err := bq.Query(ctx, sql)
		if err != nil {
			rep.Failed[string(e)] = err.Error()
			im.recordFailed(ctx, e, by, err)
			continue
		}
		sr, err := im.Stage(ctx, e, rows, "bigquery", by)
		rep.Staged[string(e)] = sr
		if err != nil {
			rep.Failed[string(e)] = err.Error()
			continue
		}
		ran++
	}
	if ran == 0 {
		if len(rep.Failed) > 0 {
			return rep, fmt.Errorf("semua query BigQuery gagal: %s", failedText(rep.Failed))
		}
		return rep, errors.New("belum ada query BigQuery yang diisi")
	}
	ar, err := im.Apply(ctx, full, by)
	rep.Apply = ar
	return rep, err
}

// recordFailed keeps a query that failed before staging visible in the import runs (Pengaturan → Data).
func (im Importer) recordFailed(ctx context.Context, e Entity, by string, cause error) {
	run, err := im.St.Q.StartImportRun(ctx, gen.StartImportRunParams{Source: "bigquery", Entity: string(e), StartedBy: &by})
	if err != nil {
		return
	}
	msg := cause.Error()
	_ = im.St.Q.FinishImportRun(ctx, gen.FinishImportRunParams{ID: run, Status: "failed", Error: &msg})
}

// failedText is "stock: …; invoices: …" in entity order.
func failedText(f map[string]string) string {
	var parts []string
	for _, e := range Entities {
		if msg, ok := f[string(e)]; ok {
			parts = append(parts, string(e)+": "+msg)
		}
	}
	return strings.Join(parts, "; ")
}

// StockEvery is how often the stock alone is pulled from BigQuery (5 minutes unless set; never under 2).
func (c SourceConfig) StockEvery() time.Duration {
	m := c.BigQuery.StockMinutes
	if m <= 0 {
		m = 5
	}
	return time.Duration(max(m, 2)) * time.Minute
}

// SyncBigQueryStock pulls only the stock query and applies incrementally (no invoice is rebuilt): Push stok stays
// minutes behind BigQuery instead of a full sync's quarter hour (Sam 19 Oct).
func (im Importer) SyncBigQueryStock(ctx context.Context, bq *BigQuery, cfg SourceConfig, by string) (SyncReport, error) {
	rep := SyncReport{Staged: map[string]StageReport{}}
	sql := strings.TrimSpace(cfg.BigQuery.Queries[string(Stock)])
	if sql == "" {
		return rep, errors.New("query stok BigQuery belum diisi")
	}
	rows, err := bq.Query(ctx, sql)
	if err != nil {
		return rep, fmt.Errorf("%s: %w", Stock, err)
	}
	sr, err := im.Stage(ctx, Stock, rows, "bigquery", by)
	rep.Staged[string(Stock)] = sr
	if err != nil {
		return rep, fmt.Errorf("%s: %w", Stock, err)
	}
	ar, err := im.Apply(ctx, false, by)
	rep.Apply = ar
	return rep, err
}
