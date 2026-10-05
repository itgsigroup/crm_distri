package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/mcp"
	"distri-arc/internal/store"
)

// arc ctl mcp-token --name "Claude Desktop Sam" --scopes read,analyze,orchestrate
func runMCPToken(ctx context.Context, st *store.Store, args []string) error {
	fs := flag.NewFlagSet("mcp-token", flag.ExitOnError)
	name := fs.String("name", "", "client name shown in the call log")
	scopes := fs.String("scopes", "read", "read,analyze,orchestrate")
	_ = fs.Parse(args)
	token, c, err := mcp.CreateToken(ctx, st.Q, *name, strings.Split(*scopes, ","), nil)
	if err != nil {
		return err
	}
	fmt.Printf("klien  %s (%s)\nscope  %s\ntoken  %s\n\nToken hanya ditampilkan sekali. Simpan di konfigurasi klien MCP (docs/mcp-clients.md).\n", deref(c.Name), c.ID, strings.Join(c.Scopes, ", "), token)
	return nil
}

// arc ctl mcp-stdio --token <token>: the same tools over stdio; cycles run in this process.
func runMCPStdio(ctx context.Context, cfg config.Config, st *store.Store, c clock.Clock, args []string) error {
	fs := flag.NewFlagSet("mcp-stdio", flag.ExitOnError)
	token := fs.String("token", os.Getenv("ARC_MCP_TOKEN"), "MCP token (or ARC_MCP_TOKEN)")
	_ = fs.Parse(args)
	if *token == "" {
		return errors.New("--token (or ARC_MCP_TOKEN) is required")
	}
	// stdout carries the protocol: logs go to stderr only
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	m := mcp.New(st, c, log, newOrchestrator(ctx, cfg, st, c, log))
	client, err := m.Verifier().Verify(ctx, *token)
	if err != nil {
		return err
	}
	m.Fixed = client
	return m.MCP().Run(ctx, &sdk.StdioTransport{})
}
