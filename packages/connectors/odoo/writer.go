package odoo

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WriteOp is one guarded write.
type WriteOp struct {
	Model  string
	Method string // create | write | action_done (mail.activity) | message_post
	ID     int64
	Values map[string]any
	// ExpectedWriteDate aborts the write when the record changed in Odoo since the last sync.
	ExpectedWriteDate string
}

// allowed is the whitelist of models/methods/fields ARC may write (Stage 10).
var allowed = map[string]map[string][]string{
	"crm.lead":      {"create": {"name", "partner_id", "expected_revenue", "description", "user_id", "type", "tag_ids"}, "write": {"probability"}},
	"mail.activity": {"create": {"res_model_id", "res_model", "res_id", "summary", "note", "date_deadline", "user_id", "activity_type_id"}, "action_done": {}},
	"mail.message":  {"message_post": {"body", "model", "res_id"}},
	"res.partner":   {"create": {"name", "phone", "mobile", "email", "parent_id", "function"}, "write": {"phone", "mobile", "email", "function"}},
}

// ErrConflict is returned when Odoo changed the record after ARC's last sync.
type ErrConflict struct {
	Model      string
	ID         int64
	Have, Want string
}

func (e *ErrConflict) Error() string {
	return fmt.Sprintf("odoo: konflik write_date %s#%d (Odoo %s, ARC %s)", e.Model, e.ID, e.Have, e.Want)
}

// Validate checks an operation against the whitelist. stage_id and unlink are never allowed.
func Validate(op WriteOp) error {
	methods, ok := allowed[op.Model]
	if !ok {
		return fmt.Errorf("odoo writer: model %s tidak diizinkan", op.Model)
	}
	fields, ok := methods[op.Method]
	if !ok {
		return fmt.Errorf("odoo writer: %s.%s tidak diizinkan", op.Model, op.Method)
	}
	for k := range op.Values {
		if k == "stage_id" {
			return fmt.Errorf("odoo writer: stage milik Odoo, ARC tidak pernah menulis stage")
		}
		okField := false
		for _, f := range fields {
			if f == k {
				okField = true
			}
		}
		if !okField {
			return fmt.Errorf("odoo writer: field %s.%s tidak diizinkan", op.Model, k)
		}
	}
	return nil
}

// Marker is appended to every text ARC writes.
func Marker(source string) string { return "via ARC · sumber: " + source }

// Writer performs whitelisted writes.
type Writer interface {
	Apply(ctx context.Context, op WriteOp) (int64, error)
	CurrentWriteDate(ctx context.Context, model string, id int64) (string, error)
}

// RPCWriter writes to a real Odoo.
type RPCWriter struct{ r *rpc }

// NewWriter builds a writer for a real Odoo database.
func NewWriter(url, db, user, apiKey string) *RPCWriter {
	return &RPCWriter{r: &rpc{URL: url, DB: db, User: user, Key: apiKey, http: &http.Client{Timeout: 60 * time.Second}}}
}

// CurrentWriteDate reads write_date of one record.
func (w *RPCWriter) CurrentWriteDate(ctx context.Context, model string, id int64) (string, error) {
	v, err := w.r.executeKW(ctx, model, "read", []any{[]any{id}}, map[string]any{"fields": []any{"write_date"}})
	if err != nil {
		return "", err
	}
	arr, _ := v.([]any)
	if len(arr) == 0 {
		return "", fmt.Errorf("odoo: %s#%d tidak ditemukan", model, id)
	}
	m, _ := arr[0].(map[string]any)
	s, _ := m["write_date"].(string)
	return s, nil
}

// Apply validates and executes one operation.
func (w *RPCWriter) Apply(ctx context.Context, op WriteOp) (int64, error) {
	if err := Validate(op); err != nil {
		return 0, err
	}
	if op.ExpectedWriteDate != "" && op.ID != 0 {
		have, err := w.CurrentWriteDate(ctx, op.Model, op.ID)
		if err != nil {
			return 0, err
		}
		if have != op.ExpectedWriteDate {
			return 0, &ErrConflict{op.Model, op.ID, have, op.ExpectedWriteDate}
		}
	}
	switch op.Method {
	case "create":
		v, err := w.r.executeKW(ctx, op.Model, "create", []any{op.Values}, map[string]any{})
		if err != nil {
			return 0, err
		}
		id, _ := v.(int64)
		return id, nil
	case "write":
		_, err := w.r.executeKW(ctx, op.Model, "write", []any{[]any{op.ID}, op.Values}, map[string]any{})
		return op.ID, err
	case "action_done":
		_, err := w.r.executeKW(ctx, op.Model, "action_done", []any{[]any{op.ID}}, map[string]any{})
		return op.ID, err
	case "message_post":
		model, _ := op.Values["model"].(string)
		body, _ := op.Values["body"].(string)
		_, err := w.r.executeKW(ctx, model, "message_post", []any{[]any{op.ID}}, map[string]any{"body": body, "message_type": "comment", "subtype_xmlid": "mail.mt_note"})
		return op.ID, err
	}
	return 0, fmt.Errorf("odoo writer: metode %s tidak dikenal", op.Method)
}

// containsMarker reports whether a text carries the ARC source marker.
func containsMarker(s string) bool { return strings.Contains(s, "via ARC") }
