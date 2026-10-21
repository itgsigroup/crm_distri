package importer

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
)

// SourceStock is one stock row of one SKU as the source (BigQuery / CSV) delivered it, with the branch its warehouse
// is mapped to — what Apply sums into stock_items (Cek stok: find where a difference comes from).
type SourceStock struct {
	Warehouse string    `json:"warehouse"`
	Branch    string    `json:"branch"` // the mapped branch; the warehouse itself when not mapped
	Mapped    bool      `json:"mapped"`
	Product   string    `json:"product"`
	Qty       int64     `json:"qty"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"` // when this row last changed in GSI Orbit
}

// StockSource lists the staged source rows of sku, mapped the way Apply maps them (Gudang → cabang).
func (im Importer) StockSource(ctx context.Context, sku string) ([]SourceStock, error) {
	rows, err := im.St.Q.StockSourceRows(ctx, sku)
	if err != nil || len(rows) == 0 {
		return []SourceStock{}, err
	}
	maps, err := im.St.Q.ListMappings(ctx, "warehouse")
	if err != nil {
		return nil, err
	}
	target := map[string]string{}
	for _, x := range maps {
		if x.Target != nil && *x.Target != "" {
			target[x.SourceValue] = *x.Target
		}
	}
	out := make([]SourceStock, 0, len(rows))
	for _, x := range rows {
		var r Row
		if json.Unmarshal(x.Data, &r) != nil {
			continue
		}
		wh := r.Get("warehouse")
		br, ok := target[strings.TrimSpace(wh)]
		if !ok {
			br = wh
		}
		out = append(out, SourceStock{Warehouse: wh, Branch: br, Mapped: ok, Product: r.Get("product"),
			Qty: int64(math.Round(Number(r.Get("qty")))), Source: x.Source, UpdatedAt: x.UpdatedAt})
	}
	return out, nil
}
