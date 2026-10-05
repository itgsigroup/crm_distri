package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"

	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/llm"
	"distri-arc/internal/policy"
	"distri-arc/internal/proposals"
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

// arc ctl agents run [--all] [--agent "AI Follow-up"] [--dealer slug] [--if-empty]
func runAgentsCtl(ctx context.Context, cfg config.Config, st *store.Store, c clock.Clock, log *slog.Logger, args []string) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New(`usage: arc ctl agents run [--all] [--agent "AI Order"] [--dealer <slug>] [--if-empty]`)
	}
	fs := flag.NewFlagSet("agents run", flag.ExitOnError)
	_ = fs.Bool("all", false, "run every agent (default)")
	agent := fs.String("agent", "", "agent name")
	dealer := fs.String("dealer", "", "dealer slug")
	ifEmpty := fs.Bool("if-empty", false, "skip when proposals exist")
	_ = fs.Parse(args[1:])
	if *ifEmpty {
		var n int
		_ = st.Pool.QueryRow(ctx, "select count(*) from proposals where kind <> 'reply'").Scan(&n)
		if n > 0 {
			log.Info("agents skipped: proposals exist", "count", n)
			return nil
		}
	}
	r := &proposals.Runner{St: st, Clock: c, Router: newRouter(ctx, cfg, st, log), Log: log}
	res, err := r.Run(ctx, proposals.RunOptions{Agent: *agent, Dealer: *dealer})
	if err != nil {
		return err
	}
	for _, s := range res.Stored {
		fmt.Printf("%-9s %-13s %-15s %.2f  %s\n", s.Status, s.Proposal.Agent, s.Proposal.Kind, s.Proposal.Confidence, s.Proposal.Title)
	}
	fmt.Printf("stored=%d suppressed=%d duplicates=%d errors=%v\n", len(res.Stored), res.Suppressed, res.Duplicates, res.Errors)
	return nil
}
