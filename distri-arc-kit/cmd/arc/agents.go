package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"strings"

	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/domain"
	"distri-arc/internal/jobs"
	"distri-arc/internal/llm"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
)

func newRouter(ctx context.Context, cfg config.Config, st *store.Store, log *slog.Logger) *llm.Router {
	pol, _ := policy.Load(ctx, st.Q)
	model := pol.LLM.Model
	if cfg.LLMModel != "" {
		model = cfg.LLMModel
	}
	return llm.NewRouter(llm.Config{Provider: cfg.LLMProvider, AnthropicKey: cfg.AnthropicKey, OpenAIKey: cfg.OpenAIKey, Model: model, Fallback: pol.LLM.Fallback, IDRPerUSD: cfg.LLMIDRPerUSD}, st, log)
}

func newOrchestrator(ctx context.Context, cfg config.Config, st *store.Store, c clock.Clock, log *slog.Logger) *orchestrator.Orchestrator {
	return &orchestrator.Orchestrator{St: st, Clock: c, Router: newRouter(ctx, cfg, st, log), Log: log, OdooWrite: cfg.OdooWrite}
}

// arc ctl reanalyze --scope all|screen:orbit|dealer:<slug>|agent:<name> [--if-empty]
// arc ctl agents run [--agent "AI Kredit"] [--dealer <slug>] [--if-empty]   (alias: a scoped cycle)
func runReanalyzeCtl(ctx context.Context, cfg config.Config, st *store.Store, c clock.Clock, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("reanalyze", flag.ExitOnError)
	scopeFlag := fs.String("scope", "all", "all | screen:<orbit|segmen|stock|credit> | dealer:<slug> | agent:<name>")
	agent := fs.String("agent", "", "agent name (agents run)")
	dealer := fs.String("dealer", "", "dealer slug (agents run)")
	ifEmpty := fs.Bool("if-empty", false, "skip when a cycle already ran")
	via := fs.String("via", "api", "api | mcp")
	_ = fs.Bool("all", false, "every agent (default)")
	_ = fs.Parse(args)
	s := *scopeFlag
	switch {
	case *agent != "":
		s = "agent:" + *agent
	case *dealer != "":
		s = "dealer:" + *dealer
	}
	sc, err := domain.ParseScope(s)
	if err != nil {
		return err
	}
	if *ifEmpty {
		if _, err := st.Q.LatestDoneCycle(ctx); err == nil {
			log.Info("reanalyze skipped: a cycle already ran")
			return nil
		}
	}
	orch := newOrchestrator(ctx, cfg, st, c, log)
	if ins, err := jobs.Inserter(st.Pool); err == nil {
		orch.Jobs = ins // automatic SO drafts reach the worker's outbox queue
	}
	rep, err := orch.Run(ctx, sc, domain.Trigger{Source: "manual", By: "arc ctl", Via: *via})
	if errors.Is(err, orchestrator.ErrRunning) {
		return errors.New("orchestrator sedang berjalan — coba lagi setelah siklus selesai")
	}
	if err != nil {
		return err
	}
	cid := rep.Cycle.ID
	stages, _ := st.Q.ListCycleStages(ctx, cid)
	fmt.Printf("siklus #%d · %s · %s · %d ms\n", deref(rep.Cycle.Number), sc.String(), rep.Cycle.Status, deref(rep.Cycle.DurationMs))
	for _, x := range stages {
		fmt.Printf("  %-10s %-8s %s\n", domain.StageLabels[x.Stage], x.Status, detailText(x.Detail))
	}
	for _, cf := range rep.Conflicts {
		if cf.Visible {
			fmt.Printf("  konflik %-24s %s ↔ %s · %s\n", cf.Rule, cf.AgentA, cf.AgentB, cf.Title)
		}
	}
	for _, p := range rep.Plan {
		fmt.Printf("  %s %-7s %-9s %s\n", p.Time, p.Autonomy, p.Status, stripLinks(p.Text))
	}
	if len(rep.Errors) > 0 {
		fmt.Println("  agen gagal:", rep.Errors)
	}
	fmt.Println(" ", deref(rep.Cycle.Note))
	return nil
}

// arc ctl cycle status
func runCycleCtl(ctx context.Context, st *store.Store, args []string) error {
	if len(args) == 0 || args[0] != "status" {
		return errors.New("usage: arc ctl cycle status")
	}
	rows, err := st.Q.ListCycles(ctx, 10)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("belum ada siklus")
		return nil
	}
	for _, c := range rows {
		fmt.Printf("#%-5d %s %-8s %-14s %-4s sinyal %-3d otonom %-3d keputusan %-3d konflik %-2d %s\n", deref(c.Number), c.StartedAt.In(clock.WIB).Format("02/01 15.04"),
			c.Status, c.Scope, deref(c.Via), deref(c.SignalsCount), deref(c.AutoCount), deref(c.DecisionCount), deref(c.ConflictCount), deref(c.Note))
	}
	return nil
}

func detailText(b []byte) string {
	var d struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(b, &d)
	return d.Text
}

func stripLinks(s string) string {
	for {
		i := strings.Index(s, "[[")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "]]")
		if j < 0 {
			return s
		}
		inner := s[i+2 : i+j]
		if _, label, ok := strings.Cut(inner, "|"); ok {
			inner = label
		}
		s = s[:i] + inner + s[i+j+2:]
	}
}
