// Command arc is the ARC backend: HTTP API + MCP server + scheduler, plus
// maintenance subcommands.
//
//	arc serve            run the API (default)
//	arc migrate          apply database migrations
//	arc seed             load tests/fixtures (idempotent)
//	arc reset            drop all data (development only)
//	arc brief [--send]   generate the daily brief now
//	arc job <name>       run one scheduled job once
//	arc eval             run the capture evaluation set
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"arc/apps/api/internal/app"
	"arc/apps/api/internal/seed"
	"arc/packages/core/config"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	cfg := config.Load()
	if !cfg.ClockAnchor.IsZero() {
		domain.SetClockAnchor(cfg.ClockAnchor)
	}
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, cmd, args); err != nil {
		slog.Error("arc failed", "cmd", cmd, "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, cmd string, args []string) error {
	db, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	switch cmd {
	case "reset":
		if cfg.Env == "production" {
			return fmt.Errorf("reset ditolak di production")
		}
		if err := db.ResetSchema(ctx); err != nil {
			return err
		}
		slog.Info("schema reset")
		return nil
	case "migrate":
		applied, err := db.Migrate(ctx)
		slog.Info("migrations", "applied", applied)
		return err
	case "seed":
		if _, err := db.Migrate(ctx); err != nil {
			return err
		}
		f, err := seed.Load(seed.FixtureDir(config.RepoRoot()))
		if err != nil {
			return err
		}
		if err := seed.Run(ctx, db, f); err != nil {
			return err
		}
		a, err := app.New(ctx, cfg, db)
		if err != nil {
			return err
		}
		if err := a.PostSeed(ctx, f); err != nil {
			return err
		}
		slog.Info("seed complete")
		return nil
	case "serve":
		if err := cfg.Validate(); err != nil {
			return err
		}
		if _, err := db.Migrate(ctx); err != nil {
			return err
		}
		a, err := app.New(ctx, cfg, db)
		if err != nil {
			return err
		}
		return a.Serve(ctx)
	case "brief":
		fs := flag.NewFlagSet("brief", flag.ExitOnError)
		send := fs.Bool("send", false, "deliver via notifier")
		_ = fs.Parse(args)
		a, err := app.New(ctx, cfg, db)
		if err != nil {
			return err
		}
		b, err := a.Agents.GenerateBrief(ctx, "manual", *send)
		if err != nil {
			return err
		}
		fmt.Println(b.TextBody)
		return nil
	case "job":
		if len(args) == 0 {
			return fmt.Errorf("usage: arc job <name>")
		}
		a, err := app.New(ctx, cfg, db)
		if err != nil {
			return err
		}
		out, err := a.RunJob(ctx, args[0])
		fmt.Println(out)
		return err
	case "eval":
		a, err := app.New(ctx, cfg, db)
		if err != nil {
			return err
		}
		report, err := a.Agents.RunEval(ctx, config.RepoRoot())
		fmt.Println(report)
		return err
	}
	return fmt.Errorf("unknown command %q", cmd)
}
