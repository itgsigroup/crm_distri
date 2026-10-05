// Package policy loads the versioned `policies` rows (JSONB) into the typed domain.PolicySet. Missing keys fall
// back to domain.DefaultPolicies so a fresh database still computes with the glossary defaults.
package policy

import (
	"context"
	"encoding/json"
	"fmt"

	"distri-arc/internal/domain"
	"distri-arc/internal/store/gen"
)

// Load returns the current policy set.
func Load(ctx context.Context, q *gen.Queries) (domain.PolicySet, error) {
	rows, err := q.ListPolicies(ctx)
	if err != nil {
		return domain.PolicySet{}, err
	}
	p := domain.DefaultPolicies()
	if len(rows) == 0 {
		return p, nil
	}
	obj := map[string]json.RawMessage{}
	for _, r := range rows {
		obj[r.Key] = r.Value
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("policies: %w", err)
	}
	return p, nil
}
