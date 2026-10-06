package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	db2 "distri-arc/db"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/ops"
	"distri-arc/internal/seed"
	"distri-arc/internal/store"
)

// runPDPCtl: arc ctl pdp export --dealer <slug> [--out file] | pdp delete --contact <wa> --by <email> --yes
func runPDPCtl(ctx context.Context, st *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: arc ctl pdp export --dealer <slug> [--out file] | pdp delete --contact <wa> --by <email> --yes")
	}
	fs := flag.NewFlagSet("pdp "+args[0], flag.ExitOnError)
	dealer := fs.String("dealer", "", "dealer slug or id")
	out := fs.String("out", "", "write the export to this file (default stdout)")
	contact := fs.String("contact", "", "WhatsApp number of the person")
	by := fs.String("by", "arc ctl", "who asked (audit)")
	yes := fs.Bool("yes", false, "confirm the deletion")
	_ = fs.Parse(args[1:])
	switch args[0] {
	case "export":
		if *dealer == "" {
			return errors.New("--dealer is required")
		}
		data, err := ops.ExportDealer(ctx, st, *dealer)
		if err != nil {
			return err
		}
		var pretty any
		_ = json.Unmarshal(data, &pretty)
		b, _ := json.MarshalIndent(pretty, "", "  ")
		if *out == "" {
			fmt.Println(string(b))
			return nil
		}
		return os.WriteFile(*out, b, 0o600)
	case "delete":
		if *contact == "" {
			return errors.New("--contact is required")
		}
		if !*yes {
			return errors.New("penghapusan tidak bisa dibatalkan: ulangi dengan --yes")
		}
		r, err := ops.DeleteContact(ctx, st, *contact, *by)
		if err != nil {
			return err
		}
		fmt.Printf("dihapus: %d kontak · %d thread · %d pesan · %d sinyal percakapan · %d identifikasi (order, invoice, metrik tetap)\n",
			r.Contacts, r.Threads, r.Messages, r.Signals, r.Identifications)
		return nil
	}
	return fmt.Errorf("pdp %s: unknown", args[0])
}

// runRetentionCtl: arc ctl retention purge | partitions
func runRetentionCtl(ctx context.Context, st *store.Store, c clock.Clock, args []string) error {
	if len(args) == 1 && args[0] == "partitions" {
		n, err := ops.EnsurePartitions(ctx, st, c.Now())
		fmt.Printf("partisi baru: %d\n", n)
		return err
	}
	if len(args) == 1 && args[0] == "purge" {
		r, err := ops.Purge(ctx, st, c.Now())
		if err != nil {
			return err
		}
		fmt.Printf("retensi: %d pesan chat · %d sinyal · %d llm_calls · %d identifikasi · %d partisi dibuang\n",
			r.ChatMessages, r.Signals, r.LLMCalls, r.Identifications, r.PartitionsDropped)
		return nil
	}
	return errors.New("usage: arc ctl retention purge|partitions")
}

// runCheckEnv prints the configuration check; it exits non-zero on a failure (and tries the database).
func runCheckEnv(ctx context.Context, cfg config.Config) error {
	issues := cfg.Check()
	if st, err := store.Open(ctx, cfg.DatabaseURL); err != nil {
		issues = append(issues, config.Issue{Level: "fail", Key: "DATABASE_URL", Msg: "tidak bisa terhubung: " + err.Error()})
	} else {
		if _, err := st.Q.Ping(ctx); err != nil {
			issues = append(issues, config.Issue{Level: "fail", Key: "DATABASE_URL", Msg: "ping gagal: " + err.Error()})
		}
		st.Close()
	}
	fails := 0
	for _, i := range issues {
		mark := map[string]string{"ok": "  OK  ", "warn": " WARN ", "fail": " FAIL "}[i.Level]
		fmt.Printf("[%s] %-22s %s\n", mark, i.Key, i.Msg)
		if i.Level == "fail" {
			fails++
		}
	}
	if fails > 0 {
		return fmt.Errorf("%d masalah konfigurasi", fails)
	}
	return nil
}

// runFingerprint prints a hash of every dealer's metrics (without the as_of timestamp) and the snapshot table
// (restore test: identical metrics).
func runFingerprint(ctx context.Context, st *store.Store) error {
	var metrics, daily string
	if err := st.Pool.QueryRow(ctx, `select coalesce(string_agg(slug || '=' || (metrics_current - 'as_of')::text, E'\n' order by slug), '') from dealers`).Scan(&metrics); err != nil {
		return err
	}
	if err := st.Pool.QueryRow(ctx, `select count(*)::text || ':' || coalesce(md5(string_agg(to_jsonb(x)::text, ',' order by x.dealer_id, x.as_of)), '')
		from dealer_metrics_daily x`).Scan(&daily); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(metrics + "\n" + daily))
	fmt.Println(hex.EncodeToString(sum[:]))
	return nil
}

// runWipe empties every data schema (public, WhatsApp sessions) and re-creates the empty installation: migrations
// and default policies. Production needs --confirm with the exact database name; take a backup first.
func runWipe(ctx context.Context, cfg config.Config, st *store.Store, args []string) error {
	fs := flag.NewFlagSet("wipe", flag.ExitOnError)
	confirm := fs.String("confirm", "", "the database name, typed exactly")
	_ = fs.Parse(args)
	var db string
	if err := st.Pool.QueryRow(ctx, "select current_database()").Scan(&db); err != nil {
		return err
	}
	if *confirm != db {
		return fmt.Errorf("menghapus semua data database %q tidak bisa dibatalkan: ulangi dengan --confirm %s (buat backup dulu: infra/backup.sh)", db, db)
	}
	if _, err := st.Pool.Exec(ctx, "drop schema if exists wa_bridge cascade; drop schema if exists whatsmeow cascade; drop schema public cascade; create schema public"); err != nil {
		return err
	}
	if err := st.Migrate(ctx); err != nil {
		return err
	}
	if err := upgradeWhatsmeow(ctx, cfg); err != nil {
		return err
	}
	n, err := seed.Policies(ctx, st, db2.Seed)
	if err != nil {
		return err
	}
	fmt.Printf("database %s dikosongkan · migrasi ulang · %d kebijakan default · buat akun: arc ctl user add …\n", db, n)
	return nil
}
