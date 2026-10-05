package domain

import (
	"fmt"
	"strings"
)

// Stages of an Orchestrator cycle, in order (04-orchestrator.md).
var Stages = []string{"ingest", "analyze", "synthesize", "decide", "execute", "learn"}

// StageLabels are the UI names of the stages.
var StageLabels = map[string]string{"ingest": "Ingest", "analyze": "Analisis", "synthesize": "Sintesis", "decide": "Keputusan", "execute": "Eksekusi", "learn": "Belajar"}

// Scope is what a cycle analyses: all | screen:<orbit|segmen|stock|credit|dealer> | dealer:<slug> | agent:<name>.
type Scope struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

// ParseScope reads "all", "screen:orbit", "dealer:mitra", "agent:AI Kredit".
func ParseScope(s string) (Scope, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "all" || s == "semua" {
		return Scope{Kind: "all"}, nil
	}
	kind, id, ok := strings.Cut(s, ":")
	if !ok || id == "" {
		return Scope{}, fmt.Errorf("scope %q: want all | screen:<name> | dealer:<id> | agent:<name>", s)
	}
	switch kind {
	case "screen":
		switch id {
		case "orbit", "segmen", "stock", "credit", "relasi", "dealer":
		default:
			return Scope{}, fmt.Errorf("unknown screen %q", id)
		}
	case "dealer":
	case "agent":
		known := false
		for _, a := range AgentNames {
			known = known || a == id
		}
		if !known {
			return Scope{}, fmt.Errorf("unknown agent %q", id)
		}
	default:
		return Scope{}, fmt.Errorf("unknown scope kind %q", kind)
	}
	return Scope{Kind: kind, ID: id}, nil
}

func (s Scope) String() string {
	if s.Kind == "all" || s.Kind == "" {
		return "all"
	}
	return s.Kind + ":" + s.ID
}

// Label is the Indonesian name of the scope used in toasts and the history ("orbit", "semua", "dealer Mitra").
func (s Scope) Label() string {
	switch s.Kind {
	case "", "all":
		return "semua"
	case "screen":
		return map[string]string{"orbit": "orbit", "segmen": "segmen", "stock": "push stok", "credit": "kredit", "relasi": "peta relasi", "dealer": "dealer"}[s.ID]
	case "agent":
		return s.ID
	}
	return "dealer " + s.ID
}

// Agents returns the agents a scope runs (nil = all), per the scope table of 04-orchestrator.md.
func (s Scope) Agents() []string {
	switch s.Kind {
	case "screen":
		switch s.ID {
		case "orbit", "segmen", "relasi":
			return []string{"AI Follow-up", "AI Kredit"}
		case "stock":
			return []string{"AI Stok"}
		case "credit":
			return []string{"AI Kredit", "AI Penagihan"}
		}
	case "agent":
		return []string{s.ID}
	}
	return nil
}

// Trigger is who started a cycle.
type Trigger struct {
	Source string `json:"source"` // schedule | manual | mcp
	By     string `json:"by"`     // user email or MCP client
	Via    string `json:"via"`    // api | mcp
}

// Conflict is one rule that fired in Sintesis.
type Conflict struct {
	Rule       string   `json:"rule"`
	DealerID   *string  `json:"dealer_id,omitempty"` // board id (slug)
	AgentA     string   `json:"agent_a"`
	AgentB     string   `json:"agent_b"`
	Title      string   `json:"title"`
	Resolution string   `json:"resolution"`
	Tone       string   `json:"tone"`
	Visible    bool     `json:"visible"`
	Keys       []string `json:"keys"` // dedupe keys of the proposals it touched
}

// Plan step statuses.
const (
	PlanScheduled = "scheduled"
	PlanRunning   = "running"
	PlanDone      = "done"
	PlanWaiting   = "waiting"
	PlanSkipped   = "skipped"
)

// PlanItem is one step of Rencana hari ini. Text uses [[dealer:<slug>|Label]] links rendered by the UI.
type PlanItem struct {
	Seq      int      `json:"seq"`
	Time     string   `json:"time"`
	Agent    string   `json:"agent"`
	Autonomy string   `json:"autonomy"` // auto | approve
	Text     string   `json:"text"`
	Keys     []string `json:"keys"` // dedupe keys of the proposals behind the step
	Status   string   `json:"status"`
	Link     string   `json:"link,omitempty"`
	WaitFor  string   `json:"wait_for,omitempty"`
}
