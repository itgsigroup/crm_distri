// mcpcheck calls a Distri ARC MCP endpoint like a client would (smoke test for docs/mcp-clients.md):
//
//	go run ./tools/mcpcheck -url http://localhost:8080/mcp -token arc_… jadwal.due '{"sales":"Dewi"}'
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func main() {
	url := flag.String("url", "http://localhost:8080/mcp", "MCP endpoint")
	token := flag.String("token", os.Getenv("ARC_MCP_TOKEN"), "bearer token")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: mcpcheck [-url] [-token] <tool> [json-args]")
		os.Exit(2)
	}
	args := map[string]any{}
	if flag.NArg() > 1 {
		if err := json.Unmarshal([]byte(flag.Arg(1)), &args); err != nil {
			fmt.Fprintln(os.Stderr, "args:", err)
			os.Exit(2)
		}
	}
	ctx := context.Background()
	c := sdk.NewClient(&sdk.Implementation{Name: "mcpcheck", Version: "1"}, nil)
	cs, err := c.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: *url, HTTPClient: &http.Client{Transport: bearer(*token)}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer func() { _ = cs.Close() }()
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: flag.Arg(0), Arguments: args})
	if err != nil {
		fmt.Fprintln(os.Stderr, "call:", err)
		os.Exit(1)
	}
	for _, ct := range res.Content {
		if t, ok := ct.(*sdk.TextContent); ok {
			fmt.Println(t.Text)
		}
	}
	if res.IsError {
		os.Exit(1)
	}
}
