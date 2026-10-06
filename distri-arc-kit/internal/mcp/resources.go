package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"distri-arc/internal/agents"
	"distri-arc/internal/clock"
	"distri-arc/internal/views"
)

// Glossary is docs/design/01-glossary.md (kept identical by TestGlossaryInSync).
//
//go:embed resources/glossary.md
var Glossary string

func text(uri, mime, body string) *sdk.ReadResourceResult {
	return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: uri, MIMEType: mime, Text: body}}}
}

func (s *Server) registerResources() {
	s.srv.AddResource(&sdk.Resource{URI: "arc://glossary", Name: "glossary", Title: "Glosarium Orbit", Description: "Istilah dan rumus Orbit (siklus order, status, segmen, sisa limit, skor).", MIMEType: "text/markdown"},
		func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return text("arc://glossary", "text/markdown", Glossary), nil
		})
	s.srv.AddResource(&sdk.Resource{URI: "arc://policies", Name: "policies", Title: "Kebijakan aktif", Description: "Ambang orbit, segmen, kredit, follow-up, floor margin, matriks otonomi, izin MCP.", MIMEType: "application/json"},
		func(ctx context.Context, _ *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			rows, err := s.St.Q.ListPolicies(ctx)
			if err != nil {
				return nil, err
			}
			out := map[string]json.RawMessage{}
			for _, p := range rows {
				out[p.Key] = p.Value
			}
			b, _ := json.MarshalIndent(out, "", "  ")
			return text("arc://policies", "application/json", string(b)), nil
		})
	s.srv.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: "arc://dealer/{id}", Name: "dealer", Title: "Dealer", Description: "Ringkasan satu dealer (markdown).", MIMEType: "text/markdown"},
		func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			id := strings.TrimPrefix(req.Params.URI, "arc://dealer/")
			b, err := s.views.Board(ctx)
			if err != nil {
				return nil, err
			}
			it, err := s.dealer(b, id, "")
			if err != nil {
				return nil, sdk.ResourceNotFoundError(req.Params.URI)
			}
			return text(req.Params.URI, "text/markdown", dealerMarkdown(b, it)), nil
		})

	s.srv.AddPrompt(&sdk.Prompt{Name: "morning_brief", Title: "Ringkasan pagi", Description: "Ringkasan pagi dari siklus Orchestrator terakhir."},
		func(ctx context.Context, _ *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			b, err := s.views.Board(ctx)
			if err != nil {
				return nil, err
			}
			st, _ := s.St.Q.ListStockItems(ctx)
			brief := b.TemplateBrief(views.StockItems(st), views.BriefCounts{})
			var sb strings.Builder
			sb.WriteString("Susun ringkasan pagi untuk CEO GSI dari data Distri ARC berikut. Bahasa Indonesia, 4 poin, sebut dealer dan angka, akhiri dengan 3 keputusan yang menunggu.\n\n")
			pts, _ := json.MarshalIndent(brief.Points, "", "  ")
			sb.WriteString("Poin (order tepat jadwal, lewat jadwal, over limit/overdue, push stok):\n```json\n" + string(pts) + "\n```\n")
			if c, err := s.St.Q.LatestFullCycle(ctx); err == nil {
				fmt.Fprintf(&sb, "\nSiklus terakhir #%d: %d sinyal, %d otonom, %d keputusan, %d konflik. %s\n", deref(c.Number), deref(c.SignalsCount), deref(c.AutoCount), deref(c.DecisionCount), deref(c.ConflictCount), deref(c.Note))
			}
			sb.WriteString("\nGunakan tool proposals.list untuk antrean keputusan dan dealer.get untuk detail. Keputusan tetap di aplikasi.")
			return &sdk.GetPromptResult{Description: "Ringkasan pagi", Messages: []*sdk.PromptMessage{{Role: "user", Content: &sdk.TextContent{Text: sb.String()}}}}, nil
		})
	s.srv.AddPrompt(&sdk.Prompt{Name: "dealer_review", Title: "Review dealer", Description: "Review satu dealer: siklus order, sisa limit, product mix, langkah berikutnya.",
		Arguments: []*sdk.PromptArgument{{Name: "dealer", Description: "slug atau nama dealer", Required: true}}},
		func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			b, err := s.views.Board(ctx)
			if err != nil {
				return nil, err
			}
			it, err := s.dealer(b, req.Params.Arguments["dealer"], req.Params.Arguments["dealer"])
			if err != nil {
				return nil, err
			}
			msg := "Review dealer berikut untuk sales pemiliknya. Jelaskan apakah ia masih di orbit, kenapa, dan satu langkah berikutnya yang paling berdampak (dengan sumber). " +
				"Jangan menjanjikan harga di bawah tier; keputusan diambil di aplikasi.\n\n" + dealerMarkdown(b, it)
			return &sdk.GetPromptResult{Description: "Review " + it.Name, Messages: []*sdk.PromptMessage{{Role: "user", Content: &sdk.TextContent{Text: msg}}}}, nil
		})
}

func dealerMarkdown(b *views.Board, it views.BoardItem) string {
	m := it.Metrics
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n%s · cabang %s · tier %s · sales %s\n\n", it.Name, it.City, it.Branch, it.Tier, it.Owner.Name)
	fmt.Fprintf(&sb, "- Status: **%s** · segmen %s · skor %d\n", m.Status, m.Segment, m.Score)
	if m.Rhythm != nil && m.Last != nil {
		fmt.Fprintf(&sb, "- Siklus order %d hari · order terakhir %d hari lalu", *m.Rhythm, *m.Last)
		if m.DueIn != nil {
			fmt.Fprintf(&sb, " · jadwal order %d hari", *m.DueIn)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "- Omzet/bln %s · rata-rata order %s · share of wallet %d%%\n", agents.Rp(m.OmzetBln), agents.Rp(m.AvgOrder), m.SOW)
	fmt.Fprintf(&sb, "- Sisa limit: %s · exposure %s / limit %s · pola bayar %d hari\n", m.Credit.State, agents.Rp(m.Credit.Exposure), agents.Rp(it.CreditLimit), m.Credit.PayDays)
	for _, inv := range views.OpenInvoices(b.Data.Histories[it.UUID], b.Today) {
		fmt.Fprintf(&sb, "  - %s %s jatuh tempo %s%s\n", inv.Number, agents.Rp(inv.Residual), clock.DayMonth(inv.DueAt), map[bool]string{true: fmt.Sprintf(" · lewat %d hari", inv.LateDays), false: ""}[inv.LateDays > 0])
	}
	if it.Next != nil {
		fmt.Fprintf(&sb, "- Langkah berikutnya (%s, %s): %s\n", it.Next.Agent, it.Next.Status, it.Next.Title)
	}
	return sb.String()
}
