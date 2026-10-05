// Command arc is the single Distri ARC Orbit binary: `arc api`, `arc worker`, `arc ctl <cmd>`.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/worker"
)

const usage = `arc — Distri ARC Orbit

  arc api                    REST + SSE API (and MCP from stage 07)
  arc worker                 background jobs (river): heartbeat, metrics, orchestrator, ingest, outbox
  arc ctl migrate            apply database migrations (goose)
  arc ctl seed [--if-empty]  load db/seed (18 sample dealers); idempotent
  arc ctl reset              drop everything, migrate and seed (dev only)
  arc ctl counts             print row counts
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cfg := config.Load()
	log := newLogger(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "api":
		err = runAPI(ctx, cfg, log)
	case "worker":
		err = runWorker(ctx, cfg, log)
	case "ctl":
		err = runCtl(ctx, cfg, log, os.Args[2:])
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

func open(ctx context.Context, cfg config.Config) (*store.Store, clock.Clock, error) {
	c, err := clock.New(pinned(cfg))
	if err != nil {
		return nil, nil, err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	return st, c, err
}

// pinned returns ARC_NOW only in development; production always runs on the real clock.
func pinned(cfg config.Config) string {
	if cfg.IsDev() {
		return cfg.Now
	}
	return ""
}

func runAPI(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	st, c, err := open(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	srv := &http.Server{Addr: cfg.APIAddr, Handler: api.New(cfg, st, c, log).Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	log.Info("api listening", "addr", cfg.APIAddr, "env", cfg.Env, "now", c.Now())
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runWorker(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	st, c, err := open(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	client, err := worker.New(st, c, log)
	if err != nil {
		return err
	}
	if err := client.Start(ctx); err != nil {
		return err
	}
	log.Info("worker started")
	<-ctx.Done()
	sh, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.Stop(sh)
}

func runCtl(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	fs := flag.NewFlagSet("ctl "+args[0], flag.ExitOnError)
	ifEmpty := fs.Bool("if-empty", false, "seed only when there are no dealers")
	_ = fs.Parse(args[1:])

	st, _, err := open(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	switch args[0] {
	case "migrate":
		if err := st.Migrate(ctx); err != nil {
			return err
		}
		log.Info("migrated")
	case "seed":
		if *ifEmpty {
			n, err := st.Q.CountSeeded(ctx)
			if err != nil {
				return err
			}
			if n.Dealers > 0 {
				log.Info("seed skipped: database not empty", "dealers", n.Dealers)
				return nil
			}
		}
		res, err := seed.Run(ctx, st, db.Seed)
		if err != nil {
			return err
		}
		printCounts(res)
	case "reset":
		if !cfg.IsDev() {
			return errors.New("reset is refused outside APP_ENV=dev")
		}
		if err := st.Reset(ctx); err != nil {
			return err
		}
		if err := st.Migrate(ctx); err != nil {
			return err
		}
		res, err := seed.Run(ctx, st, db.Seed)
		if err != nil {
			return err
		}
		printCounts(res)
	case "counts":
		res, err := st.Q.CountSeeded(ctx)
		if err != nil {
			return err
		}
		printCounts(res)
	default:
		return fmt.Errorf("unknown ctl command %q\n%s", args[0], usage)
	}
	return nil
}

func printCounts(r seed.Result) {
	fmt.Println(strings.Join([]string{
		fmt.Sprintf("dealers=%d", r.Dealers), fmt.Sprintf("orders=%d", r.Orders), fmt.Sprintf("invoices=%d", r.Invoices),
		fmt.Sprintf("payments=%d", r.Payments), fmt.Sprintf("contacts=%d", r.Contacts), fmt.Sprintf("signals=%d", r.Signals),
		fmt.Sprintf("stock_items=%d", r.StockItems), fmt.Sprintf("sales_users=%d", r.SalesUsers),
		fmt.Sprintf("commitments=%d", r.Commitments), fmt.Sprintf("policies=%d", r.Policies),
	}, " "))
}
