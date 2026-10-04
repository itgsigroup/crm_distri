// Command wa-bridge is the WhatsApp sidecar for ARC (ADR 0002, transport A).
// It links WhatsApp numbers as companion devices (QR), forwards every message
// as a WaEvent to the ARC API, and sends a message only after the API confirms
// that the referenced action was approved by a human.
//
// Endpoints (all except /health require X-ARC-Signature = HMAC-SHA256(BRIDGE_SECRET, body)):
//
//	POST /sessions                    {id, label, history_days} → {session, status, qr}
//	GET  /sessions/{id}/qr            current QR / status
//	GET  /sessions/{id}/groups        joined groups with members
//	GET  /sessions/{id}/contacts/{jid} profile (name, about, photo, business)
//	POST /sessions/{id}/send          {chat_id, text, action_id} → {wamid}
//	DELETE /sessions/{id}             log out and forget the device
//	GET  /health                      {sessions, queue, last_event}
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type config struct {
	Port        string
	Secret      string
	APIURL      string
	DatabaseURL string
	DataDir     string
	MaxPerHour  int
	MinDelay    time.Duration
	MaxDelay    time.Duration
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func loadConfig() config {
	maxPerHour, _ := strconv.Atoi(env("BRIDGE_MAX_SEND_PER_HOUR", "20"))
	return config{
		Port:        env("BRIDGE_PORT", "3001"),
		Secret:      env("BRIDGE_SECRET", "dev-bridge-secret"),
		APIURL:      strings.TrimRight(env("ARC_API_URL", "http://localhost:8000"), "/"),
		DatabaseURL: env("BRIDGE_DATABASE_URL", env("DATABASE_URL", "postgres://localhost:5433/arc?sslmode=disable")),
		DataDir:     env("BRIDGE_DATA_DIR", "./data"),
		MaxPerHour:  maxPerHour,
		MinDelay:    2 * time.Second,
		MaxDelay:    6 * time.Second,
	}
}

// withSearchPath adds search_path to a postgres URL unless one is already set.
func withSearchPath(dsn, schema string) string {
	if strings.Contains(dsn, "search_path=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := loadConfig()
	if os.Getenv("ARC_ENV") == "production" && cfg.Secret == "dev-bridge-secret" {
		slog.Error("BRIDGE_SECRET must be set in production")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		slog.Error("data dir", "err", err)
		os.Exit(1)
	}

	// Device keys live in their own schema so `arc reset` (which drops public) never unlinks phones.
	db, err := sql.Open("pgx", withSearchPath(cfg.DatabaseURL, "wa_bridge"))
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS wa_bridge`); err != nil {
		slog.Error("create schema", "err", err)
		os.Exit(1)
	}
	container := sqlstore.NewWithDB(db, "postgres", waLog.Stdout("store", "WARN", false))
	if err := container.Upgrade(ctx); err != nil {
		slog.Error("whatsmeow store upgrade", "err", err)
		os.Exit(1)
	}

	fwd := newForwarder(cfg)
	go fwd.retryLoop(ctx)
	mgr := newManager(cfg, container, fwd)
	if err := mgr.restore(ctx); err != nil {
		slog.Warn("restore sessions", "err", err)
	}

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: newHandler(cfg, mgr, fwd), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		mgr.disconnectAll()
	}()
	slog.Info("wa-bridge listening", "port", cfg.Port, "api", cfg.APIURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
