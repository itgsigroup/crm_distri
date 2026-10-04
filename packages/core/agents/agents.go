// Package agents contains ARC's agents. Each agent reads the graph, asks the
// LLM router (light/heavy/interactive) for structured output with provenance,
// and writes only to its own tables plus proposed Actions.
package agents

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"arc/packages/connectors/identity"
	"arc/packages/connectors/notify"
	"arc/packages/connectors/whatsapp"
	"arc/packages/core/actions"
	"arc/packages/core/insights"
	"arc/packages/core/llm"
	"arc/packages/core/storage"
)

// Agents bundles dependencies shared by all agents.
type Agents struct {
	DB         *storage.DB
	LLM        *llm.Router
	Fake       *llm.FakeProvider
	Ins        *insights.Service
	Actions    *actions.Service
	Fx         *FakeData
	Profiles   whatsapp.Transport // WhatsApp profile lookup (bridge)
	Truecaller identity.Truecaller
	Web        identity.WebSearch
	Notifiers  []notify.Notifier
	Recipients func(ctx context.Context, role string) []string
	Log        *slog.Logger
}

// FakeData holds fixture content the deterministic provider answers from.
type FakeData struct {
	Annotations  map[string]Annotation // message text -> annotation from the approved mockup
	Suggestions  map[string][]string   // chat thread id -> reply suggestions
	ThreadByName map[string]string
	Inbound      map[string]InboundFixture // normalized phone -> fixture
	AskCanned    map[string]string
	AskExtra     map[string]AskExtra
	AskSugg      map[string][]string
	Coaching     map[string]string
	TenderScore  map[string]int
	Memos        map[string]string
	AgentFeed    []map[string]string
}

// Annotation is a mockup message annotation.
type Annotation struct {
	K   string `json:"k"`
	T   string `json:"t"`
	Act string `json:"act"`
}

// InboundFixture is the identification result the mockup shows for a number.
type InboundFixture struct {
	ID     string `json:"id"`
	No     string `json:"no"`
	Status string `json:"status"`
	Score  *int   `json:"score"`
	Ident  struct {
		Name    string              `json:"name"`
		Role    string              `json:"role"`
		Company string              `json:"company"`
		Sources []map[string]string `json:"sources"`
	} `json:"ident"`
	Overview  string           `json:"overview"`
	Solutions []map[string]any `json:"solutions"`
	Questions []map[string]any `json:"questions"`
}

// AskExtra holds templated paragraphs for seeded Ask answers.
type AskExtra struct {
	Paragraphs []string `json:"paragraphs"`
	Followup   string   `json:"followup"`
}

// LoadFakeData reads tests/fixtures. Missing files yield empty maps.
func LoadFakeData(dir string) *FakeData {
	fx := &FakeData{Annotations: map[string]Annotation{}, Suggestions: map[string][]string{}, ThreadByName: map[string]string{},
		Inbound: map[string]InboundFixture{}, AskCanned: map[string]string{}, AskExtra: map[string]AskExtra{}, AskSugg: map[string][]string{},
		Coaching: map[string]string{}, TenderScore: map[string]int{}, Memos: map[string]string{}}
	read := func(name string, v any) {
		if raw, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			_ = json.Unmarshal(raw, v)
		}
	}
	var chats []struct {
		ID   string   `json:"id"`
		Name string   `json:"name"`
		Sug  []string `json:"sug"`
		Msgs []struct {
			T   string      `json:"t"`
			Ann *Annotation `json:"ann"`
		} `json:"msgs"`
	}
	read("chats.json", &chats)
	for _, c := range chats {
		fx.Suggestions[c.ID] = c.Sug
		fx.ThreadByName[c.Name] = c.ID
		for _, m := range c.Msgs {
			if m.Ann != nil && m.T != "" {
				fx.Annotations[m.T] = *m.Ann
			}
		}
	}
	var inbound []InboundFixture
	read("inbound.json", &inbound)
	var extra struct {
		InboundFull map[string]string   `json:"inbound_full"`
		Coaching    map[string]string   `json:"coaching"`
		AskExtra    map[string]AskExtra `json:"ask_extra"`
		AgentFeed   []map[string]string `json:"agent_feed"`
		Tenders     []struct {
			ID    string `json:"id"`
			Score int    `json:"research_score"`
		} `json:"tenders"`
	}
	read("seed_extra.json", &extra)
	for _, in := range inbound {
		if full, ok := extra.InboundFull[in.ID]; ok {
			fx.Inbound[normalize(full)] = in
		}
	}
	fx.Coaching = extra.Coaching
	fx.AskExtra = extra.AskExtra
	fx.AgentFeed = extra.AgentFeed
	for _, t := range extra.Tenders {
		fx.TenderScore[t.ID] = t.Score
	}
	read("ask_canned.json", &fx.AskCanned)
	read("ask_suggestions.json", &fx.AskSugg)
	var deals []struct {
		ID   string `json:"id"`
		Memo string `json:"memo"`
	}
	read("deals.json", &deals)
	for _, d := range deals {
		fx.Memos[d.ID] = d.Memo
	}
	return fx
}

func normalize(p string) string {
	out := make([]rune, 0, len(p))
	for _, r := range p {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	s := string(out)
	if len(s) > 0 && s[0] == '0' {
		s = "62" + s[1:]
	}
	return s
}

// RegisterFakes installs the deterministic handlers on the fake provider.
func (a *Agents) RegisterFakes() {
	if a.Fake == nil {
		return
	}
	a.Fake.Handle("capture", a.fakeCapture)
	a.Fake.Handle("suggest_replies", a.fakeSuggestions)
	a.Fake.Handle("identity_overview", a.fakeIdentity)
	a.Fake.Handle("research_solutions", a.fakeResearch)
	a.Fake.Handle("memory", a.fakeMemory)
	a.Fake.Handle("brief", a.fakeBrief)
	a.Fake.Handle("ask", a.fakeAsk)
	a.Fake.Handle("coaching", a.fakeCoaching)
	a.Fake.Handle("tender_match", a.fakeTender)
	a.Fake.Handle("followup_draft", a.fakeFollowupDraft)
	a.Fake.Handle("meeting_prep", a.fakeMeetingPrep)
}

func (a *Agents) logger() *slog.Logger {
	if a.Log != nil {
		return a.Log
	}
	return slog.Default()
}

// modelLabel renders a model id for the UI provenance line.
func ModelLabel(model string) string {
	switch model {
	case "claude-sonnet-5":
		return "Claude Sonnet 5"
	case "claude-sonnet-5-5":
		return "Claude Sonnet 5.5"
	case "claude-haiku-4-5":
		return "Claude Haiku 4.5"
	case "claude-opus-5-5":
		return "Claude Opus 5.5"
	case "fake-deterministic":
		return "mock deterministik (tanpa API key)"
	case "fixture", "":
		return "Claude Sonnet 5"
	}
	return model
}
