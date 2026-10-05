package llm

import (
	"context"
	"encoding/json"
	"io/fs"
)

// Fake answers from fixtures (testdata keyed by purpose+hash) and otherwise returns the request's deterministic
// fallback, so cycles always complete without a model (10-testing-ops › Fake & stub).
type Fake struct {
	Fixtures map[string]json.RawMessage // key: purpose or purpose:hash
	Calls    []Request
}

// NewFake returns a fake provider; fixtures may be nil.
func NewFake(fsys fs.FS) *Fake {
	f := &Fake{Fixtures: map[string]json.RawMessage{}}
	if fsys != nil {
		_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			b, err := fs.ReadFile(fsys, p)
			if err == nil {
				var m map[string]json.RawMessage
				if json.Unmarshal(b, &m) == nil {
					for k, v := range m {
						f.Fixtures[k] = v
					}
				}
			}
			return nil
		})
	}
	return f
}

func (f *Fake) Name() string  { return "fake" }
func (f *Fake) Model() string { return "fake" }

// Complete returns a fixture or the fallback.
func (f *Fake) Complete(_ context.Context, r Request) (Response, error) {
	f.Calls = append(f.Calls, r)
	if v, ok := f.Fixtures[r.Purpose+":"+Hash(r.System, r.Input)]; ok {
		return Response{JSON: v, Provider: "fake", Model: "fake"}, nil
	}
	if v, ok := f.Fixtures[r.Purpose]; ok {
		return Response{JSON: v, Provider: "fake", Model: "fake"}, nil
	}
	return Response{JSON: r.Fallback, Provider: "fake", Model: "fake", Fallback: true}, nil
}
