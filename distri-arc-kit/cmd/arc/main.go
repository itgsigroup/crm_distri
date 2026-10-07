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
	"distri-arc/internal/ask"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/dealersvc"
	"distri-arc/internal/events"
	"distri-arc/internal/identify"
	"distri-arc/internal/jobs"
	"distri-arc/internal/mcp"
	"distri-arc/internal/odoo"
	"distri-arc/internal/ops"
	"distri-arc/internal/orchestrator"
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
  arc ctl seed [--if-empty] [--policies-only]  load db/seed (18 sample dealers) | only default policies (prod); idempotent
  arc ctl reset              drop everything, migrate and seed (dev only)
  arc ctl counts             print row counts
  arc ctl recompute          recompute dealers.metrics_current (all dealers)
  arc ctl metrics --dealer <slug>  print a dealer's metrics and the 5 score components
  arc ctl wa inject --from <no> --text "…" [--to <sales no>] [--in 2h]  feed a fake inbound WhatsApp message
  arc ctl wa numbers         list paired sales numbers
  arc ctl odoo sync [--full] pull Odoo (ODOO_MODE=fake|rpc) into Distri ARC, read-only
  arc ctl odoo test          check the Odoo connection
  arc ctl reanalyze --scope all|screen:orbit|dealer:<slug>|agent:<name> [--if-empty]  run an Orchestrator cycle
  arc ctl agents run [--agent "AI Order"] [--dealer <slug>]  alias of reanalyze with that scope
  arc ctl cycle status       last cycles: status, counters, note
  arc ctl user add --email e --name n --role ceo|admin|finance|sales|warehouse --password p [--sales Andi]
  arc ctl user passwd --email e --password p
  arc ctl mcp-token --name "Claude Desktop Sam" --scopes read,analyze,orchestrate   create an MCP token (shown once)
  arc ctl mcp-stdio --token <token>   MCP over stdio for local clients (cycles run inline)
  arc ctl user totp-reset --email e   turn 2FA off for a user who lost the authenticator
  arc ctl check-env          validate the configuration (fails on production mistakes)
  arc ctl retention purge|partitions  apply policy retention now | create next months' partitions
  arc ctl pdp export --dealer <slug> [--out f]   everything stored about a dealer (UU PDP)
  arc ctl pdp delete --contact <wa> --by <email> --yes   erase a person (aggregates stay)
  arc ctl fingerprint        hash of all dealer metrics (restore test)
  arc ctl wipe --confirm <db>   empty every data schema, migrate, default policies (back up first; irreversible)
  arc ctl wa import-internal --csv f   internal numbers (wa_number,label,department,is_sales)
  arc ctl pilot start [--branch Semarang] | live | off   pilot mode (start = shadow: no sends, everything approve)
  arc ctl pilot status|audit [--week d]   pilot numbers per agent, KPI, privacy & send audit (exit 1 on a violation)
  arc ctl pilot snapshot|export [--week d] [--out f]   store a pilot week | weekly CSV
  arc ctl pilot rehearse [--days 21]   dev only: simulated pilot (14 days shadow + live) to test the tooling
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
	src, err := odooSource(cfg)
	if err != nil {
		return err
	}
	m := mcp.New(st, c, log, &orchestrator.Orchestrator{St: st, Clock: c, Log: log, OdooWrite: cfg.OdooWrite})
	m.Jobs = ins // MCP cycles run in the worker like every other cycle
	if !cfg.IsDev() && len(cfg.SessionSecret) < 32 {
		return errors.New("SESSION_SECRET (≥ 32 karakter) wajib di luar APP_ENV=dev")
	}
	a := api.New(cfg, st, c, log).WithJobs(ins).WithOdoo(src).WithMCP(m).WithIdentify(&identify.Service{St: st, Clock: c, Truecaller: truecaller(cfg)}).WithAsk(&ask.Service{St: st, Clock: c, Router: newRouter(ctx, cfg, st, log)})
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
	ingest.Transport = t.Name()
	src, err := odooSource(cfg)
	if err != nil {
		return err
	}
	orch := newOrchestrator(ctx, cfg, st, c, log)
	// a worker that stopped mid-cycle leaves it running; mark such cycles failed so the next one can start
	if err := st.Q.FailStaleCycles(ctx, c.Now().Add(-time.Hour)); err != nil {
		return err
	}
	idf := &identify.Service{St: st, Clock: c, Truecaller: truecaller(cfg)}
	if pr, ok := t.(wa.ProfileReader); ok {
		idf.Profiles = pr
	}
	client, err := worker.New(st, c, log, worker.Deps{Transport: t, Ingest: ingest, Rules: sendRules(cfg), Odoo: src, Orchestrator: orch, Identify: idf,
		Ops: ops.EnvFrom(cfg), AlertFrom: cfg.AlertWAFrom, AlertGroup: cfg.AlertWAGroup, SessionKey: cfg.SessionKey()})
	if err != nil {
		return err
	}
	orch.Jobs = client
	ingest.OnDealer = func(ctx context.Context, id uuid.UUID) {
		_, _ = client.Insert(ctx, jobs.RecomputeArgs{DealerIDs: []string{id.String()}}, nil)
	}
	ingest.OnNewNumber = func(ctx context.Context, number string) {
		_, _ = client.Insert(ctx, jobs.IdentifyArgs{WANumber: number}, nil)
	}
	if err := t.Start(ctx); err != nil {
		log.Error("wa transport start", "transport", t.Name(), "err", err)
	}
	go ingest.Run(ctx, t)
	if err := client.Start(ctx); err != nil {
		return err
	}
	log.Info("worker started", "wa_transport", t.Name(), "odoo", cfg.OdooMode)
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
	if args[0] == "check-env" {
		return runCheckEnv(ctx, cfg)
	}
	if args[0] == "pilot" {
		st, clk, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		if len(args) > 1 && args[1] == "rehearse" {
			return runRehearsal(ctx, cfg, st, clk, log, args[2:])
		}
		return runPilotCtl(ctx, st, clk, args[1:])
	}
	if args[0] == "wipe" {
		st, _, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		return runWipe(ctx, cfg, st, args[1:])
	}
	if args[0] == "pdp" || args[0] == "retention" || args[0] == "fingerprint" {
		st, clk, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		switch args[0] {
		case "pdp":
			return runPDPCtl(ctx, st, args[1:])
		case "retention":
			return runRetentionCtl(ctx, st, clk, args[1:])
		}
		return runFingerprint(ctx, st)
	}
	if args[0] == "user" {
		st, _, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		return runUserCtl(ctx, st, args[1:])
	}
	if args[0] == "mcp-token" || args[0] == "mcp-stdio" {
		st, clk, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		if args[0] == "mcp-token" {
			return runMCPToken(ctx, st, args[1:])
		}
		return runMCPStdio(ctx, cfg, st, clk, args[1:])
	}
	if args[0] == "agents" || args[0] == "reanalyze" || args[0] == "cycle" {
		st, clk, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		switch {
		case args[0] == "cycle":
			return runCycleCtl(ctx, st, args[1:])
		case args[0] == "agents" && (len(args) < 2 || args[1] != "run"):
			return errors.New(`usage: arc ctl agents run [--agent "AI Order"] [--dealer <slug>] [--if-empty]`)
		case args[0] == "agents":
			return runReanalyzeCtl(ctx, cfg, st, clk, log, args[2:])
		}
		return runReanalyzeCtl(ctx, cfg, st, clk, log, args[1:])
	}
	if args[0] == "odoo" {
		st, clk, err := open(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		return runOdooCtl(ctx, cfg, st, clk, log, args[1:])
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
	policiesOnly := fs.Bool("policies-only", false, "load only the default policies (production)")
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
		if *policiesOnly {
			n, err := seed.Policies(ctx, st, db.Seed)
			if err != nil {
				return err
			}
			log.Info("policies loaded", "keys", n)
			return nil
		}
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
		if err := demoPasswords(ctx, cfg, st, log); err != nil {
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
		if err := demoPasswords(ctx, cfg, st, log); err != nil {
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

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

func runWACtl(ctx context.Context, st *store.Store, c clock.Clock, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: arc ctl wa inject|numbers|import-internal --csv f")
	}
	fs := flag.NewFlagSet("wa "+args[0], flag.ExitOnError)
	from := fs.String("from", "", "sender number")
	to := fs.String("to", "", "sales number that receives it (default: the dealer owner's number)")
	text := fs.String("text", "", "message text")
	name := fs.String("name", "", "sender push name")
	in := fs.Duration("in", 0, "message time after now (dev clock: a reply arrives after the sent follow-up)")
	csvPath := fs.String("csv", "", "internal numbers CSV: wa_number,label,department,is_sales")
	_ = fs.Parse(args[1:])
	switch args[0] {
	case "import-internal":
		n, err := importInternal(ctx, st, *csvPath)
		if err != nil {
			return err
		}
		fmt.Printf("%d nomor internal diimpor (DM antar nomor ini tidak disimpan)\n", n)
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
		res, err := injectMessage(ctx, st, log, *from, acct, *text, *name, c.Now().Add(*in))
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

func odooSource(cfg config.Config) (odoo.Source, error) {
	return odoo.New(odoo.Config{Mode: cfg.OdooMode, URL: cfg.OdooURL, DB: cfg.OdooDB, User: cfg.OdooUser, APIKey: cfg.OdooAPIKey, Write: cfg.OdooWrite, SeedFS: db.Seed})
}

func runOdooCtl(ctx context.Context, cfg config.Config, st *store.Store, c clock.Clock, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: arc ctl odoo sync [--full] | test")
	}
	fs := flag.NewFlagSet("odoo "+args[0], flag.ExitOnError)
	full := fs.Bool("full", false, "ignore cursors and read everything")
	_ = fs.Parse(args[1:])
	src, err := odooSource(cfg)
	if err != nil {
		return err
	}
	if src == nil {
		return errors.New("ODOO_MODE=off")
	}
	switch args[0] {
	case "test":
		v, err := src.Version(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("odoo %s (%s) ok · write=%v\n", v, src.Name(), cfg.OdooWrite)
	case "sync":
		rep, err := odoo.NewSyncer(st, src, c, log).Run(ctx, *full)
		if err != nil {
			return err
		}
		if _, err := dealersvc.New(st, c).Recompute(ctx, rep.Dealers...); err != nil {
			return err
		}
		fmt.Printf("synced %v · %d signals · %d dealers touched · %dms\n", rep.Records, rep.Signals, len(rep.Dealers), rep.Duration)
	default:
		return fmt.Errorf("unknown odoo command %q", args[0])
	}
	return nil
}

// truecaller returns the Truecaller adapter: there is no public API key yet (OPEN-QUESTIONS), so lookups use the
// fake, which knows no numbers — identification relies on WA Business, Getcontact and Odoo.
func truecaller(cfg config.Config) identify.Truecaller {
	if cfg.TruecallerKey != "" {
		slog.Warn("TRUECALLER_API_KEY is set but no Truecaller client is wired yet; using the fake")
	}
	return identify.FakeTruecaller{}
}
