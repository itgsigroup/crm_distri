// Command arc is the single Distri ARC Orbit binary: `arc api`, `arc worker`, `arc ctl <cmd>`.
package main

import (
	"context"

	"errors"
	"flag"
	"fmt"
	"github.com/google/uuid"
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
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/events"
	"distri-arc/internal/jobs"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
	"distri-arc/internal/views"
	"distri-arc/internal/wa"
	"distri-arc/internal/worker"
)

const usage = `arc — Distri ARC Orbit

  arc api                    REST + SSE API (and MCP from stage 07)
  arc worker                 background jobs (river): heartbeat, metrics, orchestrator, ingest, outbox
  arc ctl migrate            apply database migrations (goose)
  arc ctl seed [--if-empty]  load db/seed (18 sample dealers); idempotent
  arc ctl reset              drop everything, migrate and seed (dev only)
  arc ctl counts             print row counts
  arc ctl recompute          recompute dealers.metrics_current (all dealers)
  arc ctl metrics --dealer <slug>  print a dealer's metrics and the 5 score components
  arc ctl wa inject --from <no> --text "…" [--to <sales no>]  feed a fake inbound WhatsApp message
  arc ctl wa numbers         list paired sales numbers
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
	ins, err := jobs.Inserter(st.Pool)
	if err != nil {
		return err
	}
	a := api.New(cfg, st, c, log).WithJobs(ins)
	if cfg.WATransport == "cloudapi" {
		a.WithCloudWebhook(cloudTransport(cfg))
	}
	go events.Listen(ctx, st.Pool, a.Hub(), log)
	srv := &http.Server{Addr: cfg.APIAddr, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
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
	t, err := newTransport(ctx, cfg, st, log)
	if err != nil {
		return err
	}
	ingest := wa.NewIngestor(st, log)
	client, err := worker.New(st, c, log, worker.Deps{Transport: t, Ingest: ingest, Rules: sendRules(cfg)})
	if err != nil {
		return err
	}
	ingest.OnDealer = func(ctx context.Context, id uuid.UUID) {
		_, _ = client.Insert(ctx, jobs.RecomputeArgs{DealerIDs: []string{id.String()}}, nil)
	}
	if err := t.Start(ctx); err != nil {
		log.Error("wa transport start", "transport", t.Name(), "err", err)
	}
	go ingest.Run(ctx, t)
	if err := client.Start(ctx); err != nil {
		return err
	}
	log.Info("worker started", "wa_transport", t.Name())
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
	if args[0] == "wa" {
		st, clk, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		return runWACtl(ctx, st, clk, log, args[1:])
	}
	fs := flag.NewFlagSet("ctl "+args[0], flag.ExitOnError)
	ifEmpty := fs.Bool("if-empty", false, "seed only when there are no dealers")
	dealerFlag := fs.String("dealer", "", "dealer slug or id")
	_ = fs.Parse(args[1:])

	st, clk, err := open(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := dealersvc.New(st, clk)
	switch args[0] {
	case "migrate":
		if err := st.Migrate(ctx); err != nil {
			return err
		}
		if err := upgradeWhatsmeow(ctx, cfg); err != nil {
			return fmt.Errorf("whatsmeow store: %w", err)
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
		if _, err := svc.Recompute(ctx); err != nil {
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
		if err := upgradeWhatsmeow(ctx, cfg); err != nil {
			return err
		}
		res, err := seed.Run(ctx, st, db.Seed)
		if err != nil {
			return err
		}
		if _, err := svc.Recompute(ctx); err != nil {
			return err
		}
		printCounts(res)
	case "recompute":
		ms, err := svc.Recompute(ctx)
		if err != nil {
			return err
		}
		log.Info("recomputed", "dealers", len(ms))
	case "wa":
		return runWACtl(ctx, st, clk, log, args[1:])
	case "metrics":
		return printMetrics(ctx, st, clk, *dealerFlag)
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

func printMetrics(ctx context.Context, st *store.Store, c clock.Clock, id string) error {
	if id == "" {
		return errors.New("--dealer is required")
	}
	b, err := views.NewBuilder(st, c).Board(ctx)
	if err != nil {
		return err
	}
	it, ok := b.Get(id)
	if !ok {
		return fmt.Errorf("dealer %q not found", id)
	}
	m := it.Metrics
	opt := func(p *int) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprint(*p)
	}
	fmt.Printf("%s (%s · %s · tier %s · sales %s)\n", it.Name, it.City, it.Branch, it.Tier, it.Owner.Name)
	fmt.Printf("  siklus order   %s hr · terakhir %s hr · cyc %.2f · jadwal order %s · status %s · aktivitas %s\n", opt(m.Rhythm), opt(m.Last), m.Cyc, opt(m.DueIn), m.Status, m.Activity)
	fmt.Printf("  segmen         %s · %.2f×/bln · Rp %d/order · omzet Rp %d/bln\n", m.Segment, deref(m.Freq), m.AvgOrder, m.OmzetBln)
	fmt.Printf("  share of wallet %d%% (%s) · product mix %d/6 %v\n", m.SOW, m.SOWSource, m.Mix, m.MixCats)
	room := "cash"
	if m.Credit.Room != nil {
		room = fmt.Sprintf("%.0f%%", *m.Credit.Room*100)
	}
	fmt.Printf("  sisa limit     %s · exposure Rp %d / limit Rp %d · ruang %s · pola bayar %d hr · tepat waktu %d%%\n", m.Credit.State, m.Credit.Exposure, m.Credit.Limit, room, m.Credit.PayDays, m.Credit.OnTime)
	fmt.Printf("  PIC aktif      %d\n", m.PICActive)
	p := m.ScoreParts
	fmt.Printf("  skor dealer    %d  (siklus %d · SOW %d · mix %d · limit %d · PIC %d)\n", m.Score, p.Rhythm, p.SOW, p.Mix, p.Credit, p.Contact)
	return nil
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func runWACtl(ctx context.Context, st *store.Store, c clock.Clock, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: arc ctl wa inject|numbers")
	}
	fs := flag.NewFlagSet("wa "+args[0], flag.ExitOnError)
	from := fs.String("from", "", "sender number")
	to := fs.String("to", "", "sales number that receives it (default: the dealer owner's number)")
	text := fs.String("text", "", "message text")
	name := fs.String("name", "", "sender push name")
	_ = fs.Parse(args[1:])
	switch args[0] {
	case "inject":
		if *from == "" || *text == "" {
			return errors.New("--from and --text are required")
		}
		acct := *to
		if acct == "" {
			n := wa.Digits(*from)
			if ct, err := st.Q.FindContactByNumber(ctx, &n); err == nil && ct.DealerOwner != nil {
				for _, s := range must(st.Q.ListSalesUsers(ctx)) {
					if s.ID == *ct.DealerOwner && s.WaNumber != nil {
						acct = *s.WaNumber
					}
				}
			}
		}
		if acct == "" {
			acct = "6281234504471"
		}
		res, err := injectMessage(ctx, st, log, *from, acct, *text, *name, c.Now())
		if err != nil {
			return err
		}
		fmt.Printf("stored=%v reason=%s thread=%s signal=%s\n", res.Stored, res.Reason, res.ThreadID, res.SignalID)
		if res.DealerID != nil {
			if _, err := dealersvc.New(st, c).Recompute(ctx, *res.DealerID); err != nil {
				return err
			}
		}
	case "numbers":
		nums, err := st.Q.ListWANumbers(ctx)
		if err != nil {
			return err
		}
		for _, n := range nums {
			fmt.Printf("%-15s %-8s %-10s %s\n", n.WaNumber, deref2(n.SalesName), n.Transport, n.State)
		}
	default:
		return fmt.Errorf("unknown wa command %q", args[0])
	}
	return nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func deref2(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
