// Package mcp serves Distri ARC over the Model Context Protocol (06-mcp.md, ADR 0004): read, analyze and
// orchestrate tools for Claude Desktop, ChatGPT and other agents. Every call is authenticated (bearer token
// → mcp_clients), scope-checked, rate-limited and recorded (mcp_calls + audit_log). There is no tool that
// decides or sends: actions.decide answers human_only and mcp.permissions.allow_send is locked in code.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/riverqueue/river"

	"distri-arc/internal/clock"
	"distri-arc/internal/events"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/policy"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// Version of the MCP server implementation.
const Version = "0.7.0"

// Tool errors (the error text the client sees starts with the code).
var (
	ErrForbidden   = errors.New("forbidden")
	ErrRateLimited = errors.New("rate_limited")
	ErrHumanOnly   = errors.New("human_only")
)

// CallsPerMinute is the per-client rate limit.
const CallsPerMinute = 60

// Server is the Distri ARC MCP server.
type Server struct {
	St       *store.Store
	Clock    clock.Clock
	Log      *slog.Logger
	Orch     *orchestrator.Orchestrator // Queue/Execute, input.get/submit
	Jobs     *river.Client[pgx.Tx]      // nil: cycles run inline (tests, stdio)
	Fixed    *Client                    // stdio: the client of the --token flag
	CycleCap time.Duration              // how long orchestrator.* waits for its cycle (default 25 s)
	// ResourceMetadataURL is sent in the 401 WWW-Authenticate header (OAuth discovery for Claude).
	ResourceMetadataURL string

	views    *views.Builder
	verifier *Verifier
	srv      *sdk.Server
	mu       sync.Mutex
	calls    map[uuid.UUID][]time.Time
	tools    []ToolInfo
}

// ToolInfo describes a registered tool (Pengaturan → Koneksi AI, layar Orchestrator).
type ToolInfo struct {
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	Description string `json:"description"`
}

// Tools lists the registered tools in registration order.
func (s *Server) Tools() []ToolInfo { return s.tools }

// New builds the server with every tool, resource and prompt registered.
func New(st *store.Store, c clock.Clock, log *slog.Logger, orch *orchestrator.Orchestrator) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{St: st, Clock: c, Log: log, Orch: orch, views: views.NewBuilder(st, c), verifier: NewVerifier(st), calls: map[uuid.UUID][]time.Time{}}
	s.srv = sdk.NewServer(&sdk.Implementation{Name: "distri-arc", Title: "Distri ARC Orbit", Version: Version},
		&sdk.ServerOptions{Instructions: "Distri ARC Orbit — CRM distribusi B2B GSI. Baca dealer, siklus order, sisa limit, stok; jalankan Orchestrator. " +
			"Keputusan (setujui/tolak) dan pengiriman ke dealer hanya di aplikasi oleh manusia."})
	s.registerTools()
	s.registerResources()
	return s
}

// Verifier exposes the token verifier (revoking clears its cache).
func (s *Server) Verifier() *Verifier { return s.verifier }

// MCP returns the SDK server (stdio, in-memory tests).
func (s *Server) MCP() *sdk.Server { return s.srv }

// Handler serves Streamable HTTP at /mcp behind bearer authentication.
func (s *Server) Handler() http.Handler {
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s.srv }, &sdk.StreamableHTTPOptions{Logger: s.Log, SessionTimeout: 30 * time.Minute, DisableLocalhostProtection: true})
	verify := func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		c, err := s.verifier.Verify(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		exp := c.Expires
		if exp.IsZero() {
			exp = time.Now().Add(time.Hour) // manual tokens do not expire; the SDK wants an expiry per request
		}
		return &auth.TokenInfo{Scopes: c.Scopes, UserID: c.ID.String(), Expiration: exp, Extra: map[string]any{"client": c}}, nil
	}
	// a 401 tells the client where to start OAuth (RFC 9728); behind nginx the API listens on loopback while the
	// Host header is the public name, so the SDK's DNS-rebinding guard (meant for local servers) stays off —
	// every request is authenticated by token anyway.
	return auth.RequireBearerToken(verify, &auth.RequireBearerTokenOptions{ResourceMetadataURL: s.ResourceMetadataURL})(h)
}

// client resolves the caller of a tool call.
func (s *Server) client(req *sdk.CallToolRequest) *Client {
	if req != nil && req.Extra != nil && req.Extra.TokenInfo != nil {
		if c, ok := req.Extra.TokenInfo.Extra["client"].(*Client); ok {
			return c
		}
	}
	return s.Fixed
}

func (s *Server) allow(id uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	keep := s.calls[id][:0]
	for _, t := range s.calls[id] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= CallsPerMinute {
		s.calls[id] = keep
		return false
	}
	s.calls[id] = append(keep, now)
	return true
}

// result is what a tool handler returns besides its output.
type result struct {
	Summary string
	CycleID *uuid.UUID
}

// handler is a tool implementation: typed input → output (any JSON) + call summary.
type handler[In any] func(ctx context.Context, c *Client, in In) (any, result, error)

// tool registers a tool with scope check, rate limit and recording.
func tool[In any](s *Server, name, scope, desc string, h handler[In]) {
	// Claude accepts tool names of letters, digits, _ and - only: dealer.list is served as dealer_list
	name = strings.ReplaceAll(name, ".", "_")
	s.tools = append(s.tools, ToolInfo{Name: name, Scope: scope, Description: desc})
	sdk.AddTool(s.srv, &sdk.Tool{Name: name, Description: desc}, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		t0 := time.Now()
		c := s.client(req)
		var (
			out any
			res result
			err error
		)
		switch {
		case c == nil:
			err = fmt.Errorf("%w: token diperlukan", ErrForbidden)
		case scope == "decide":
			err = fmt.Errorf("%w: keputusan hanya oleh manusia di aplikasi Distri ARC", ErrHumanOnly)
		case !c.Has(scope):
			err = fmt.Errorf("%w: tool %s butuh scope %q", ErrForbidden, name, scope)
		case !s.allow(c.ID):
			err = fmt.Errorf("%w: maksimal %d panggilan per menit", ErrRateLimited, CallsPerMinute)
		default:
			out, res, err = h(ctx, c, in)
		}
		s.record(ctx, c, name, in, res, err, time.Since(t0))
		if err != nil {
			return nil, nil, err
		}
		return nil, out, nil
	})
}

func status(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrHumanOnly):
		return "human_only"
	}
	return "error"
}

func (s *Server) record(ctx context.Context, c *Client, name string, in any, res result, err error, d time.Duration) {
	ctx = context.WithoutCancel(ctx)
	args, _ := json.Marshal(in)
	summary := res.Summary
	if err != nil {
		summary = err.Error()
	}
	var cid *uuid.UUID
	who := "anonymous"
	if c != nil {
		cid, who = &c.ID, c.Name
		_ = s.St.Q.TouchMCPClient(ctx, gen.TouchMCPClientParams{ID: c.ID, LastSeenAt: ptr(s.Clock.Now())})
	}
	st := status(err)
	if e := s.St.Q.InsertMCPCall(ctx, gen.InsertMCPCallParams{ClientID: cid, Tool: &name, Args: args, ResultSummary: &summary, CycleID: res.CycleID,
		DurationMs: ptr(int32(d.Milliseconds())), Status: st, CreatedAt: time.Now()}); e != nil { // wall time: the call log is real time
		s.Log.Warn("mcp_calls", "err", e)
	}
	actor, kind, action, entity := who, "mcp", "mcp."+name, "mcp_call"
	after, _ := json.Marshal(map[string]any{"status": st, "summary": summary})
	_ = s.St.Q.InsertAudit(ctx, gen.InsertAuditParams{Actor: &actor, ActorKind: &kind, Action: &action, Entity: &entity, After: after})
	_ = events.Notify(ctx, s.St.Pool, "mcp_call", map[string]any{"tool": name, "client": who, "status": st})
	s.Log.Info("mcp call", "client", who, "tool", name, "status", st, "ms", d.Milliseconds())
}

// permissions are the current mcp.permissions; allow_send is false whatever the table says.
func (s *Server) permissions(ctx context.Context) (allowReanalyze, allowPlan, mask bool, maxCycles int) {
	p, err := policy.Load(ctx, s.St.Q)
	if err != nil {
		return true, true, true, 6
	}
	m := p.MCP
	if m.MaxCyclesPerHour == 0 {
		m.MaxCyclesPerHour = 6
	}
	return m.AllowReanalyze, m.AllowPlanUpdateProposal, m.MaskPIIInRead, m.MaxCyclesPerHour
}

func ptr[T any](v T) *T { return &v }
