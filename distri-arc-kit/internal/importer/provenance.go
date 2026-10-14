package importer

import (
	"context"
	"time"

	"distri-arc/internal/store"
)

// Provenance says where the sales numbers come from and when they were last synced (task 12 of the 14 Oct review):
// shown next to every KPI so two reports can be compared on the same footing.
type Provenance struct {
	Source   string     `json:"source"` // bigquery | csv | odoo | sample | none
	Label    string     `json:"label"`  // "Accurate (BigQuery)"
	SyncedAt *time.Time `json:"synced_at"`
	// Basis: what omzet means here — invoice totals as the source sends them (DPP when the query gives dpp)
	Basis string `json:"basis"`
}

// ProvenanceOf reads the data source policy and the last finished import.
func ProvenanceOf(ctx context.Context, st *store.Store) Provenance {
	im := Importer{St: st}
	c, _ := im.LoadConfig(ctx)
	p := Provenance{Source: c.Mode, Basis: "total faktur dari sumber (termasuk PPN bila sumber menyertakannya); faktur batal tidak dihitung; retur mengurangi bila dikirim sebagai faktur bernilai minus"}
	switch c.Mode {
	case "bigquery":
		p.Label = "Accurate (BigQuery)"
	case "csv":
		p.Label = "Unggahan CSV"
	default:
		var seeded bool
		_ = st.Pool.QueryRow(ctx, "select exists(select 1 from signal_keys where dedupe_key like 'seed:%')").Scan(&seeded)
		p.Source, p.Label = "sample", "Data contoh"
		if !seeded {
			p.Source, p.Label = "none", "Belum ada sumber data"
		}
	}
	if c.Mode == "bigquery" || c.Mode == "csv" {
		if r, err := st.Q.LastImportRun(ctx, c.Mode); err == nil {
			p.SyncedAt = r.FinishedAt
		}
		var dpp bool
		_ = st.Pool.QueryRow(ctx, "select exists(select 1 from import_rows where entity = 'invoices' and data ? 'dpp' limit 1)").Scan(&dpp)
		if dpp {
			p.Basis = "DPP (tanpa PPN) per faktur; faktur batal tidak dihitung; retur mengurangi bila dikirim sebagai faktur bernilai minus"
		}
	}
	return p
}
