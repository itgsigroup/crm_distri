package odoo

import (
	"context"
	"fmt"
)

// DraftLine is one line of a draft sale order.
type DraftLine struct {
	ProductID int     `json:"product_id"` // product.product id
	Qty       float64 `json:"qty"`
	PriceUnit float64 `json:"price_unit"`
}

// CreateSODraft creates a quotation (state draft) for a dealer with a provenance note
// ("Dibuat Distri ARC · proposal <id> · disetujui <user>"). It is the only write Distri ARC makes to Odoo and
// fails with ErrWriteDisabled unless the source was built with writes enabled (ODOO_WRITE=true).
func CreateSODraft(ctx context.Context, src Source, partnerID int, lines []DraftLine, proposalID, approvedBy string) (int, error) {
	if len(lines) == 0 {
		return 0, fmt.Errorf("draft without lines")
	}
	ol := make([]any, 0, len(lines))
	for _, l := range lines {
		ol = append(ol, []any{0, 0, map[string]any{"product_id": l.ProductID, "product_uom_qty": l.Qty, "price_unit": l.PriceUnit}})
	}
	return src.Create(ctx, "sale.order", map[string]any{
		"partner_id": partnerID,
		"order_line": ol,
		"note":       fmt.Sprintf("Dibuat Distri ARC · proposal %s · disetujui %s", proposalID, approvedBy),
	})
}
