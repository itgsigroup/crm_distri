package odoo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// RPC talks to Odoo's /jsonrpc endpoint with an API key (execute_kw). Writes are refused unless Write is true.
type RPC struct {
	URL, DB, User, APIKey string
	Write                 bool
	HTTP                  *http.Client
	mu                    sync.Mutex
	uid                   int
	seq                   atomic.Int64
}

// NewRPC configures the client.
func NewRPC(url, db, user, key string, write bool) *RPC {
	return &RPC{URL: url, DB: db, User: user, APIKey: key, Write: write, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

func (c *RPC) Name() string { return "rpc" }

func (c *RPC) call(ctx context.Context, service, method string, args ...any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "call", "id": c.seq.Add(1), "params": map[string]any{"service": service, "method": method, "args": args}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/jsonrpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string                   `json:"message"`
			Data    struct{ Message string } `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("odoo %s.%s: %w", service, method, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("odoo %s.%s: %s %s", service, method, out.Error.Message, out.Error.Data.Message)
	}
	return out.Result, nil
}

func (c *RPC) login(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.uid != 0 {
		return c.uid, nil
	}
	raw, err := c.call(ctx, "common", "authenticate", c.DB, c.User, c.APIKey, map[string]any{})
	if err != nil {
		return 0, err
	}
	var uid int
	if err := json.Unmarshal(raw, &uid); err != nil || uid == 0 {
		return 0, fmt.Errorf("odoo login failed for %s", c.User)
	}
	c.uid = uid
	return uid, nil
}

func (c *RPC) executeKw(ctx context.Context, model, method string, args []any, kw map[string]any) (json.RawMessage, error) {
	uid, err := c.login(ctx)
	if err != nil {
		return nil, err
	}
	return c.call(ctx, "object", "execute_kw", c.DB, uid, c.APIKey, model, method, args, kw)
}

// SearchRead pages through search_read (500 per page).
func (c *RPC) SearchRead(ctx context.Context, model string, domain []any, fields []string) ([]Record, error) {
	var out []Record
	for offset := 0; ; offset += 500 {
		raw, err := c.executeKw(ctx, model, "search_read", []any{domain}, map[string]any{"fields": fields, "limit": 500, "offset": offset, "order": "id"})
		if err != nil {
			return nil, err
		}
		var page []Record
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < 500 {
			return out, nil
		}
	}
}

// Create creates a record (only when writes are enabled).
func (c *RPC) Create(ctx context.Context, model string, vals map[string]any) (int, error) {
	if !c.Write {
		return 0, ErrWriteDisabled
	}
	raw, err := c.executeKw(ctx, model, "create", []any{vals}, map[string]any{})
	if err != nil {
		return 0, err
	}
	var id int
	return id, json.Unmarshal(raw, &id)
}

// Version returns the server version (connection test).
func (c *RPC) Version(ctx context.Context) (string, error) {
	raw, err := c.call(ctx, "common", "version")
	if err != nil {
		return "", err
	}
	var v struct {
		ServerVersion string `json:"server_version"`
	}
	_ = json.Unmarshal(raw, &v)
	if _, err := c.login(ctx); err != nil {
		return v.ServerVersion, err
	}
	return v.ServerVersion, nil
}
