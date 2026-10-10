package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Router applies policy llm.routing: the primary provider (2 attempts), then the backup, then the deterministic
// fallback. It masks PII on the way out, unmasks on the way back, validates the JSON and logs every call.
type Router struct {
	Primary Provider
	Backup  Provider // may be nil
	St      *store.Store
	Log     *slog.Logger
}

// Validator checks a decoded answer (required keys, lengths).
type Validator func(json.RawMessage) error

// Complete returns a validated answer; it never fails the caller: on error it returns the fallback.
// HasModel: a real model answers (not the fake provider of LLM_PROVIDER=fake / no API key).
func (r *Router) HasModel() bool {
	if r == nil || r.Primary == nil {
		return false
	}
	_, fake := r.Primary.(*Fake)
	return !fake
}

// FromModel: a response a model wrote (not the fake provider, not the template fallback after a failure).
func FromModel(res Response) bool {
	return res.Provider != "" && res.Provider != "fake" && res.Provider != "template" && !res.Fallback
}

func (r *Router) Complete(ctx context.Context, req Request, valid Validator) Response {
	m := NewMasker()
	masked := req
	masked.System = m.Mask(req.System, req.Names)
	masked.Input = []byte(m.Mask(string(req.Input), req.Names))
	try := func(p Provider) (Response, error) {
		if p == nil {
			return Response{}, fmt.Errorf("no provider")
		}
		res, err := p.Complete(ctx, masked)
		r.record(ctx, req, masked, p, res, err)
		if err != nil {
			return res, err
		}
		res.JSON = []byte(m.Unmask(string(res.JSON)))
		if !json.Valid(res.JSON) {
			return res, fmt.Errorf("%s returned invalid JSON", p.Name())
		}
		if valid != nil {
			if err := valid(res.JSON); err != nil {
				return res, err
			}
		}
		return res, nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		if res, err := try(r.Primary); err == nil {
			return res
		} else if r.Log != nil {
			r.Log.Warn("llm primary failed", "purpose", req.Purpose, "attempt", attempt+1, "err", err)
		}
		if _, isFake := r.Primary.(*Fake); isFake {
			break
		}
	}
	if r.Backup != nil {
		if res, err := try(r.Backup); err == nil {
			return res
		}
	}
	return Response{JSON: req.Fallback, Provider: "template", Model: "template", Fallback: true}
}

func (r *Router) record(ctx context.Context, req, masked Request, p Provider, res Response, err error) {
	if r.St == nil {
		return
	}
	var cycle *uuid.UUID
	if req.CycleID != nil {
		if id, e := uuid.Parse(*req.CycleID); e == nil {
			cycle = &id
		}
	}
	agent, provider, model, purpose := req.Agent, p.Name(), p.Model(), req.Purpose
	if res.Model != "" {
		model = res.Model
	}
	hash := Hash(masked.System, masked.Input)
	in, out, cost := int32(res.TokensIn), int32(res.TokensOut), res.CostIDR
	dur := int32(res.Duration / time.Millisecond)
	if err != nil {
		purpose += " (error)"
	}
	if _, e := r.St.Q.InsertLLMCall(ctx, gen.InsertLLMCallParams{CycleID: cycle, Agent: &agent, Provider: &provider, Model: &model, Purpose: &purpose,
		InputHash: &hash, TokensIn: &in, TokensOut: &out, CostIdr: &cost, DurationMs: &dur}); e != nil && r.Log != nil {
		r.Log.Warn("llm_calls insert", "err", e)
	}
}
