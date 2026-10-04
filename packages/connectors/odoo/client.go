// Package odoo is ARC's Odoo connector (XML-RPC, Odoo 16/17 SaaS).
//
// client.go is strictly read-only: it exposes search_read, read and fields_get.
// All writes live in writer.go behind a whitelist, dry-run and audit (Stage 10).
package odoo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Record is one Odoo record as returned by search_read.
type Record = map[string]any

// SearchOpts tunes search_read.
type SearchOpts struct {
	Limit     int
	Offset    int
	Order     string
	Companies []int
}

// Reader is the read-only surface used by the sync job.
type Reader interface {
	SearchRead(ctx context.Context, model string, domain []any, fields []string, opt SearchOpts) ([]Record, error)
	FieldsGet(ctx context.Context, model string) (map[string]any, error)
	Companies(ctx context.Context) ([]Record, error)
}

// rpc performs authenticated execute_kw calls; shared by Client and Writer.
type rpc struct {
	URL, DB, User, Key string
	uid                int64
	http               *http.Client
}

func (r *rpc) call(ctx context.Context, endpoint, method string, params ...any) (any, error) {
	body := EncodeCall(method, params...)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.URL, "/")+endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "text/xml")
		resp, err := r.http.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(500*(attempt+1)) * time.Millisecond)
			continue
		}
		v, err := DecodeResponse(resp.Body)
		resp.Body.Close()
		if err != nil {
			var f *Fault
			if errors.As(err, &f) {
				return nil, err
			}
			lastErr = err
			time.Sleep(time.Duration(500*(attempt+1)) * time.Millisecond)
			continue
		}
		return v, nil
	}
	return nil, lastErr
}

func (r *rpc) authenticate(ctx context.Context) error {
	if r.uid != 0 {
		return nil
	}
	v, err := r.call(ctx, "/xmlrpc/2/common", "authenticate", r.DB, r.User, r.Key, map[string]any{})
	if err != nil {
		return err
	}
	uid, ok := v.(int64)
	if !ok || uid == 0 {
		return errors.New("odoo: autentikasi gagal (periksa username & API key)")
	}
	r.uid = uid
	return nil
}

func (r *rpc) executeKW(ctx context.Context, model, method string, args []any, kw map[string]any) (any, error) {
	if err := r.authenticate(ctx); err != nil {
		return nil, err
	}
	return r.call(ctx, "/xmlrpc/2/object", "execute_kw", r.DB, r.uid, r.Key, model, method, args, kw)
}

// Client is the read-only Odoo client.
type Client struct {
	r         *rpc
	companies []int
}

// NewClient builds a client; companies are the [NEW] company ids to sync.
func NewClient(url, db, user, apiKey string, companies []int) *Client {
	return &Client{r: &rpc{URL: url, DB: db, User: user, Key: apiKey, http: &http.Client{Timeout: 60 * time.Second}}, companies: companies}
}

func (c *Client) ctxKW(opt SearchOpts) map[string]any {
	comp := opt.Companies
	if len(comp) == 0 {
		comp = c.companies
	}
	kw := map[string]any{}
	if len(comp) > 0 {
		ids := make([]any, len(comp))
		for i, x := range comp {
			ids[i] = x
		}
		kw["context"] = map[string]any{"allowed_company_ids": ids}
	}
	return kw
}

// SearchRead runs search_read with batching handled by the caller (limit/offset).
func (c *Client) SearchRead(ctx context.Context, model string, domain []any, fields []string, opt SearchOpts) ([]Record, error) {
	kw := c.ctxKW(opt)
	fs := make([]any, len(fields))
	for i, f := range fields {
		fs[i] = f
	}
	kw["fields"] = fs
	if opt.Limit > 0 {
		kw["limit"] = opt.Limit
	}
	if opt.Offset > 0 {
		kw["offset"] = opt.Offset
	}
	if opt.Order != "" {
		kw["order"] = opt.Order
	}
	if domain == nil {
		domain = []any{}
	}
	v, err := c.r.executeKW(ctx, model, "search_read", []any{domain}, kw)
	if err != nil {
		return nil, err
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("odoo: unexpected search_read result %T", v)
	}
	out := make([]Record, 0, len(arr))
	for _, x := range arr {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// FieldsGet returns field metadata (used to discover custom lead-to-cash date fields).
func (c *Client) FieldsGet(ctx context.Context, model string) (map[string]any, error) {
	v, err := c.r.executeKW(ctx, model, "fields_get", []any{}, map[string]any{"attributes": []any{"string", "type"}})
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	return m, nil
}

// Companies lists companies whose name starts with "[NEW]".
func (c *Client) Companies(ctx context.Context) ([]Record, error) {
	return c.SearchRead(ctx, "res.company", []any{[]any{"name", "ilike", "[NEW]"}}, []string{"id", "name"}, SearchOpts{})
}
