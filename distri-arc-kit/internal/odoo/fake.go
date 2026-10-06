package odoo

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
)

// Fake serves db/seed/odoo/<model>.json as an Odoo database. Domains support (field, op, value) leaves with
// =, !=, >, >=, <, <=, in and implicit AND — enough for the syncer.
type Fake struct {
	mu      sync.Mutex
	models  map[string][]Record
	Write   bool
	created []Record
	notes   []Note
	// FailNext makes the next write fail (tests of the outbox error path).
	FailNext error
}

// Note is an internal note posted on a record (fake chatter).
type Note struct {
	Model string
	ID    int
	Body  string
}

// NewFake loads every <model>.json under dir of fsys.
func NewFake(fsys fs.FS, dir string) (*Fake, error) {
	f := &Fake{models: map[string][]Record{}}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var rows []Record
		if err := json.Unmarshal(b, &rows); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		f.models[strings.TrimSuffix(e.Name(), ".json")] = rows
	}
	return f, nil
}

func (f *Fake) Name() string                            { return "fake" }
func (f *Fake) Version(context.Context) (string, error) { return "17.0 (fake)", nil }

// SearchRead filters a model; fields are ignored (all fields are returned).
func (f *Fake) SearchRead(_ context.Context, model string, domain []any, _ []string) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Record
	for _, r := range f.models[model] {
		if match(r, domain) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Create appends a record (writes must be enabled, as with the real client).
func (f *Fake) Create(_ context.Context, model string, vals map[string]any) (int, error) {
	if !f.Write {
		return 0, ErrWriteDisabled
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.FailNext; err != nil {
		f.FailNext = nil
		return 0, err
	}
	id := 900000 + len(f.created) + 1
	r := Record{"id": float64(id)}
	for k, v := range vals {
		r[k] = v
	}
	f.models[model] = append(f.models[model], r)
	f.created = append(f.created, r)
	return id, nil
}

// PostNote records a note (writes must be enabled).
func (f *Fake) PostNote(_ context.Context, model string, id int, body string) (int, error) {
	if !f.Write {
		return 0, ErrWriteDisabled
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.FailNext; err != nil {
		f.FailNext = nil
		return 0, err
	}
	f.notes = append(f.notes, Note{Model: model, ID: id, Body: body})
	return len(f.notes), nil
}

// Notes returns the notes posted so far.
func (f *Fake) Notes() []Note {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Note(nil), f.notes...)
}

// Created returns the records created so far.
func (f *Fake) Created() []Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Record(nil), f.created...)
}

// Set replaces a field of a record (tests simulate Odoo changes).
func (f *Fake) Set(model string, id int, field string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.models[model] {
		if r.Int("id") == id {
			r[field] = v
		}
	}
}

func match(r Record, domain []any) bool {
	for _, leaf := range domain {
		l, ok := leaf.([]any)
		if !ok || len(l) != 3 {
			continue // "&" operators: everything is AND here
		}
		field, _ := l[0].(string)
		op, _ := l[1].(string)
		if !cmp(r[field], op, l[2]) {
			return false
		}
	}
	return true
}

func isM2O(a []any) bool {
	if len(a) != 2 {
		return false
	}
	_, isName := a[1].(string)
	return isName
}

func scalar(v any) any {
	if a, ok := v.([]any); ok && isM2O(a) {
		return a[0] // many2one → id
	}
	return v
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func eq(a, b any) bool {
	if x, ok := num(a); ok {
		if y, ok := num(b); ok {
			return x == y
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func cmp(fieldVal any, op string, want any) bool {
	v := scalar(fieldVal)
	switch op {
	case "=":
		return eq(v, want)
	case "!=":
		return !eq(v, want)
	case "in":
		list, _ := want.([]any)
		if ids, ok := fieldVal.([]any); ok && !isM2O(ids) {
			for _, id := range ids { // x2many: any member in list
				for _, w := range list {
					if eq(id, w) {
						return true
					}
				}
			}
			return false
		}
		for _, w := range list {
			if eq(v, w) {
				return true
			}
		}
		return false
	case ">", ">=", "<", "<=":
		x, xs := num(v)
		y, ys := num(want)
		if xs && ys {
			return (op == ">" && x > y) || (op == ">=" && x >= y) || (op == "<" && x < y) || (op == "<=" && x <= y)
		}
		a, b := fmt.Sprint(v), fmt.Sprint(want)
		return (op == ">" && a > b) || (op == ">=" && a >= b) || (op == "<" && a < b) || (op == "<=" && a <= b)
	}
	return false
}
