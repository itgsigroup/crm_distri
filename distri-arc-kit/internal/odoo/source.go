// Package odoo reads Odoo (source of truth for dealers, SO, invoices, payments, stock) over JSON-RPC and maps the
// records into Distri ARC's tables idempotently. The only write is a draft sale order (stage 05+), disabled
// unless ODOO_WRITE=true.
package odoo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Record is one row of search_read.
type Record map[string]any

// Source is what the syncer needs from Odoo.
type Source interface {
	Name() string
	SearchRead(ctx context.Context, model string, domain []any, fields []string) ([]Record, error)
	Create(ctx context.Context, model string, vals map[string]any) (int, error)
	// PostNote posts an internal note (chatter, not sent to the customer) on a record; writes must be enabled.
	PostNote(ctx context.Context, model string, id int, body string) (int, error)
	Version(ctx context.Context) (string, error)
}

// ErrWriteDisabled is returned by writes when ODOO_WRITE is not true.
var ErrWriteDisabled = errors.New("odoo write disabled (ODOO_WRITE=false)")

// Int reads an integer field (JSON numbers decode as float64).
func (r Record) Int(k string) int {
	switch v := r[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// Float reads a number field.
func (r Record) Float(k string) float64 {
	switch v := r[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

// Str reads a text field (Odoo sends false for empty).
func (r Record) Str(k string) string {
	if s, ok := r[k].(string); ok {
		return s
	}
	return ""
}

// Bool reads a boolean field.
func (r Record) Bool(k string) bool { b, _ := r[k].(bool); return b }

// M2O reads a many2one ([id, "name"] or false).
func (r Record) M2O(k string) (int, string) {
	a, ok := r[k].([]any)
	if !ok || len(a) < 2 {
		return 0, ""
	}
	id, _ := a[0].(float64)
	name, _ := a[1].(string)
	return int(id), name
}

// Ints reads an x2many id list.
func (r Record) Ints(k string) []int {
	a, _ := r[k].([]any)
	out := make([]int, 0, len(a))
	for _, v := range a {
		if f, ok := v.(float64); ok {
			out = append(out, int(f))
		}
	}
	return out
}

// Time reads an Odoo datetime ("2006-01-02 15:04:05", UTC).
func (r Record) Time(k string) *time.Time {
	s := r.Str(k)
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC)
	if err != nil {
		return nil
	}
	return &t
}

// Date reads an Odoo date ("2006-01-02") as midnight WIB.
func (r Record) Date(k string) *time.Time {
	s := r.Str(k)
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.FixedZone("WIB", 7*3600))
	if err != nil {
		return nil
	}
	return &t
}

// FormatTime renders t as an Odoo datetime literal for domains.
func FormatTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

// SourceID is the source_id stored in Distri ARC tables ("sale.order:4101").
func SourceID(model string, id int) string { return fmt.Sprintf("%s:%d", model, id) }

// Digits keeps the digits of a phone number, 0… → 62….
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if strings.HasPrefix(d, "0") {
		d = "62" + d[1:]
	}
	return d
}
