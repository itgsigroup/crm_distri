package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"distri-arc/internal/config"
	"distri-arc/internal/outbox"
	"distri-arc/internal/store"
	"distri-arc/internal/wa"
)

// whatsmeowDB opens a database/sql handle whose search_path is the whatsmeow schema (device store).
func whatsmeowDB(url string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.RuntimeParams["search_path"] = "whatsmeow"
	return stdlib.OpenDB(*cfg), nil
}

// upgradeWhatsmeow applies whatsmeow's own versioned schema (run by `arc ctl migrate`, never at runtime).
func upgradeWhatsmeow(ctx context.Context, cfg config.Config) error {
	db, err := whatsmeowDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	return wa.WhatsmeowStore(db).Upgrade(ctx)
}

// newTransport builds the configured WhatsApp transport (WA_TRANSPORT).
func newTransport(ctx context.Context, cfg config.Config, st *store.Store, log *slog.Logger) (wa.Transport, error) {
	switch cfg.WATransport {
	case "whatsmeow":
		db, err := whatsmeowDB(cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		return wa.NewWhatsmeow(wa.WhatsmeowStore(db), cfg.WABackfillDays, log), nil
	case "cloudapi":
		return cloudTransport(cfg), nil
	case "baileys":
		return baileysTransport(cfg, st, log), nil
	case "fake", "":
		nums, err := st.Q.ListWANumbers(ctx)
		if err != nil {
			return nil, err
		}
		var accts []string
		for _, n := range nums {
			accts = append(accts, n.WaNumber)
		}
		return wa.NewFake(accts...), nil
	default:
		return nil, fmt.Errorf("WA_TRANSPORT %q: want fake, baileys, whatsmeow or cloudapi", cfg.WATransport)
	}
}

func cloudTransport(cfg config.Config) *wa.CloudAPI {
	return wa.NewCloudAPI(cfg.WACloudNumber, cfg.WACloudPhoneID, cfg.WACloudToken, cfg.WACloudVerify, cfg.WACloudSecret)
}

func sendRules(cfg config.Config) outbox.Rules {
	r := outbox.DefaultRules()
	r.GapMin, r.GapMax = config.Range(cfg.WASendGap, r.GapMin, r.GapMax)
	r.ReplyMin, r.ReplyMax = config.Range(cfg.WAReplyDelay, r.ReplyMin, r.ReplyMax)
	switch h := strings.TrimSpace(os.Getenv("WA_SEND_HOURS")); {
	case h == "off":
		r.SendFrom, r.SendTo = 0, 0
	case h != "":
		var a, b int
		if _, err := fmt.Sscanf(h, "%d-%d", &a, &b); err == nil && a >= 0 && b <= 24 && a < b {
			r.SendFrom, r.SendTo = a, b
		}
	}
	return r
}

// injectMessage feeds one fake inbound message through the ingest pipeline (arc ctl wa inject).
func injectMessage(ctx context.Context, st *store.Store, log *slog.Logger, from, to, text, name string, now time.Time) (wa.Result, error) {
	in := wa.NewIngestor(st, log)
	m := wa.Message{ID: fmt.Sprintf("INJ%d", now.UnixNano()), Account: to, ChatJID: wa.UserJID(from), FromNumber: wa.Digits(from), FromName: name, Text: text, Time: now}
	return in.Process(ctx, m)
}

// baileysTransport is the Baileys bridge transport (ADR 0017): the bridge confirms every send against the outbox,
// and names each linked device after its number's label or sales.
func baileysTransport(cfg config.Config, st *store.Store, log *slog.Logger) *wa.Baileys {
	return &wa.Baileys{
		BridgeURL: cfg.BridgeURL, Secret: cfg.BridgeSecret, Listen: cfg.BridgeListen, HistoryDays: cfg.WABackfillDays, Log: log,
		Approved: func(ctx context.Context, outboxID string) (bool, error) {
			id, err := uuid.Parse(outboxID)
			if err != nil {
				return false, err
			}
			return st.Q.OutboxApprovedForSend(ctx, id)
		},
		Accounts: func(ctx context.Context) []string {
			_ = st.Q.SetWANumbersTransport(ctx, "baileys")
			rows, err := st.Q.ListWANumbers(ctx)
			if err != nil {
				return nil
			}
			out := make([]string, 0, len(rows))
			for _, r := range rows {
				out = append(out, r.WaNumber)
			}
			return out
		},
		Label: func(ctx context.Context, account string) string {
			if n, err := st.Q.GetWANumber(ctx, account); err == nil && n.Label != nil && *n.Label != "" {
				return *n.Label
			}
			return account
		},
	}
}
