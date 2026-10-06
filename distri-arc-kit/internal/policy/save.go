package policy

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/domain"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

//go:embed schema/*.json
var schemas embed.FS

var (
	resolvedMu sync.Mutex
	resolved   = map[string]*jsonschema.Resolved{}
)

// Keys lists the editable policy keys (one schema each).
func Keys() []string {
	ents, _ := schemas.ReadDir("schema")
	var out []string
	for _, e := range ents {
		out = append(out, strings.TrimSuffix(e.Name(), ".json"))
	}
	return out
}

func schemaFor(key string) (*jsonschema.Resolved, error) {
	resolvedMu.Lock()
	defer resolvedMu.Unlock()
	if r, ok := resolved[key]; ok {
		return r, nil
	}
	b, err := schemas.ReadFile("schema/" + key + ".json")
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKey, key)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("schema %s: %w", key, err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("schema %s: %w", key, err)
	}
	resolved[key] = r
	return r, nil
}

// Errors of Validate/Save.
var (
	ErrUnknownKey = errors.New("kebijakan tidak dikenal")
	ErrInvalid    = errors.New("kebijakan tidak valid")
	ErrLocked     = errors.New("aturan terkunci")
)

// Validate checks a value against its key's JSON Schema and the rules that are locked in code (09-policies-security):
// SOP-SEC-001 cannot be switched off, releases above the limit always need the CEO, MCP never sends, credit releases
// and limit changes are never autonomous.
func Validate(key string, value json.RawMessage) error {
	rs, err := schemaFor(key)
	if err != nil {
		return err
	}
	var inst any
	if err := json.Unmarshal(value, &inst); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := rs.Validate(inst); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if key == "autonomy.matrix" {
		var m map[string]domain.AutonomyRow
		_ = json.Unmarshal(value, &m)
		for agent, row := range m {
			for _, k := range row.Auto {
				if k == domain.KindCreditRelease || k == domain.KindCreditLimit {
					return fmt.Errorf("%w: %s — %s tidak boleh otonom", ErrLocked, agent, k)
				}
			}
		}
	}
	if key == "orbit.thresholds" {
		var o domain.OrbitPolicy
		_ = json.Unmarshal(value, &o)
		if o.Churn <= o.Drift {
			return fmt.Errorf("%w: ambang churn harus lebih besar dari ambang lewat jadwal", ErrInvalid)
		}
	}
	return nil
}

// Save validates and writes a new version of a policy with its history, audit row and a NOTIFY so open screens
// refetch; the Orchestrator reads it on its next cycle.
func Save(ctx context.Context, st *store.Store, key string, value json.RawMessage, by *uuid.UUID, actor string, at time.Time) (gen.Policy, error) {
	if err := Validate(key, value); err != nil {
		return gen.Policy{}, err
	}
	var out gen.Policy
	err := st.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		p, err := q.SetPolicy(ctx, gen.SetPolicyParams{Key: key, Value: value, UpdatedBy: by, UpdatedAt: at})
		if err != nil {
			return err
		}
		out = p
		if err := q.InsertPolicyHistory(ctx, gen.InsertPolicyHistoryParams{Key: key, Version: p.Version, Value: value, UpdatedBy: by, UpdatedAt: &at}); err != nil {
			return err
		}
		kind, action, entity := "user", "policy.update", "policy:"+key
		after, _ := json.Marshal(map[string]any{"version": p.Version, "value": value})
		if err := q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, After: after}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "select pg_notify('policy_changed', $1)", fmt.Sprintf(`{"key":%q,"version":%d}`, key, p.Version))
		return err
	})
	return out, err
}
