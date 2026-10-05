package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"distri-arc/internal/clock"
	"distri-arc/internal/domain"
	"distri-arc/internal/jobs"
	"distri-arc/internal/llm"
	"distri-arc/internal/metrics"
	"distri-arc/internal/orchestrator"
	"distri-arc/internal/store/gen"
	"distri-arc/internal/views"
)

// ---------- inputs ----------

type empty struct{}

type dealerListIn struct {
	Status  string `json:"status,omitempty" jsonschema:"Key account | Aktif | At risk | Churn | Baru"`
	Segment string `json:"segment,omitempty" jsonschema:"A | B | C | D | Baru"`
	Sales   string `json:"sales,omitempty" jsonschema:"nama sales pemilik (Andi, Dewi, Rizky, Fajar)"`
	Q       string `json:"q,omitempty" jsonschema:"cari nama dealer"`
	Limit   int    `json:"limit,omitempty"`
}

type dealerIn struct {
	ID   string `json:"id,omitempty" jsonschema:"slug atau uuid dealer"`
	Name string `json:"name,omitempty" jsonschema:"nama dealer (bila id kosong)"`
}

type dueIn struct {
	Days  int    `json:"days,omitempty" jsonschema:"horizon hari (default 7)"`
	Sales string `json:"sales,omitempty"`
}

type salesIn struct {
	Sales string `json:"sales,omitempty"`
}

type creditIn struct {
	DealerID string `json:"dealer_id" jsonschema:"slug atau uuid dealer"`
	Amount   int64  `json:"amount,omitempty" jsonschema:"simulasi rilis (rupiah)"`
}

type stockIn struct {
	Branch  string `json:"branch,omitempty"`
	MinDays int    `json:"min_days,omitempty"`
}

type threadIn struct {
	DealerID string `json:"dealer_id"`
	Limit    int    `json:"limit,omitempty"`
}

type branchIn struct {
	Branch string `json:"branch,omitempty"`
}

type limitIn struct {
	Limit int `json:"limit,omitempty"`
}

type statusIn struct {
	Status string `json:"status,omitempty" jsonschema:"proposed (default) | approved | executed | rejected"`
}

type segmentIn struct {
	Segment string `json:"segment" jsonschema:"A | B | C | D"`
}

type daysIn struct {
	Days int `json:"days,omitempty"`
}

type scopeIn struct {
	Scope string `json:"scope" jsonschema:"all | screen:<orbit|segmen|stock|credit> | dealer:<slug> | agent:<nama agen>"`
}

type planIn struct {
	Date string `json:"date,omitempty" jsonschema:"YYYY-MM-DD (default hari ini)"`
}

type planUpdateIn struct {
	ItemID string `json:"item_id"`
	Action string `json:"action" jsonschema:"move | skip | add"`
	Time   string `json:"time,omitempty" jsonschema:"jam baru HH.MM (move/add)"`
	Text   string `json:"text,omitempty" jsonschema:"teks langkah (add)"`
	Reason string `json:"reason,omitempty"`
}

type agentRunIn struct {
	Agent    string `json:"agent" jsonschema:"AI Order | AI Follow-up | AI Kredit | AI Stok | AI Penagihan | AI Prospek"`
	DealerID string `json:"dealer_id,omitempty" jsonschema:"batasi ke satu dealer (scope dealer)"`
}

type inputGetIn struct {
	CycleID string `json:"cycle_id"`
	Agent   string `json:"agent"`
}

type submitIn struct {
	CycleID   string           `json:"cycle_id"`
	Agent     string           `json:"agent"`
	Proposals []map[string]any `json:"proposals" jsonschema:"proposal seperti candidates di Input (kind, dealer_id, title, why, preview, confidence, signal_ids)"`
}

type decideIn struct {
	ProposalID string `json:"proposal_id"`
	Decision   string `json:"decision"`
}

// ---------- read ----------

func dealerRow(it views.BoardItem) map[string]any {
	m := it.Metrics
	return map[string]any{"id": it.ID, "name": it.Name, "city": it.City, "branch": it.Branch, "tier": it.Tier, "sales": it.Owner.Name,
		"status": m.Status, "segment": m.Segment, "rhythm_days": m.Rhythm, "last_order_days": m.Last, "due_in": m.DueIn, "cyc": m.Cyc,
		"score": m.Score, "omzet_bln": m.OmzetBln, "avg_order": m.AvgOrder, "credit": m.Credit, "credit_limit": it.CreditLimit, "share_of_wallet": m.SOW,
		"root_cause": it.RootCause}
}

func bySales(items []views.BoardItem, sales string) []views.BoardItem {
	if sales == "" {
		return items
	}
	var out []views.BoardItem
	for _, it := range items {
		if strings.EqualFold(it.Owner.Name, sales) || strings.HasPrefix(strings.ToLower(it.Owner.Name), strings.ToLower(sales)) {
			out = append(out, it)
		}
	}
	return out
}

func (s *Server) dealer(b *views.Board, id, name string) (views.BoardItem, error) {
	if id != "" {
		if it, ok := b.Get(id); ok {
			return it, nil
		}
	}
	q := strings.ToLower(strings.TrimSpace(name + " " + id))
	for _, it := range b.Items {
		if q != "" && strings.Contains(strings.ToLower(it.Name), strings.TrimSpace(q)) {
			return it, nil
		}
	}
	return views.BoardItem{}, fmt.Errorf("dealer %q tidak ditemukan", strings.TrimSpace(id+" "+name))
}

func (s *Server) registerRead() {
	tool(s, "dealer.list", ScopeRead, "Daftar dealer dengan siklus order, status, segmen, sisa limit dan omzet/bln.", func(ctx context.Context, _ *Client, in dealerListIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		var out []map[string]any
		for _, it := range bySales(b.Items, in.Sales) {
			if (in.Status != "" && it.Metrics.Status != in.Status) || (in.Segment != "" && it.Metrics.Segment != in.Segment) ||
				(in.Q != "" && !strings.Contains(strings.ToLower(it.Name), strings.ToLower(in.Q))) {
				continue
			}
			out = append(out, dealerRow(it))
			if in.Limit > 0 && len(out) >= in.Limit {
				break
			}
		}
		return map[string]any{"items": out, "count": len(out)}, result{Summary: fmt.Sprintf("%d dealer", len(out))}, nil
	})
	tool(s, "dealer.get", ScopeRead, "Satu dealer lengkap: metrics, 5 komponen skor, PIC, memo, komitmen, sinyal terbaru.", func(ctx context.Context, _ *Client, in dealerIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		it, err := s.dealer(b, in.ID, in.Name)
		if err != nil {
			return nil, result{}, err
		}
		d, err := s.views.DealerDetail(ctx, b, it)
		if err != nil {
			return nil, result{}, err
		}
		if _, _, mask, _ := s.permissions(ctx); mask {
			return maskJSON(d), result{Summary: it.Name}, nil
		}
		return d, result{Summary: it.Name}, nil
	})
	tool(s, "segmen.list", ScopeRead, "Empat segmen (sering × besar): jumlah dealer, omzet/bln, % omzet, perpindahan 3 bulan.", func(ctx context.Context, _ *Client, _ empty) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		sum, total := views.SegmenSummary(b.Items)
		return map[string]any{"segments": sum, "omzet_total": total, "movers": views.SegmenMovers(b.Items)}, result{Summary: fmt.Sprintf("%d segmen", len(sum))}, nil
	})
	tool(s, "jadwal.due", ScopeRead, "Dealer yang jadwal ordernya dalam N hari, dengan rekomendasi order.", func(ctx context.Context, _ *Client, in dueIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		days := in.Days
		if days <= 0 {
			days = 7
		}
		st, _ := s.St.Q.ListStockItems(ctx)
		stock := views.StockItems(st)
		var out []map[string]any
		for _, it := range bySales(b.Due(days), in.Sales) {
			row := dealerRow(it)
			row["rekomendasi_order"] = metrics.OrderRecommendation(metrics.DealerView{ID: it.ID, Name: it.Name, Metrics: it.Metrics, Composition: it.Composition}, it.Branch, stock, b.Policies)
			out = append(out, row)
		}
		return map[string]any{"items": out, "count": len(out), "days": days}, result{Summary: fmt.Sprintf("%d dealer", len(out))}, nil
	})
	tool(s, "jadwal.lewat", ScopeRead, "Dealer lewat 1,2× siklus order, dengan akar terduga.", func(ctx context.Context, _ *Client, in salesIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		var out []map[string]any
		for _, it := range bySales(b.Drift(), in.Sales) {
			out = append(out, dealerRow(it))
		}
		return map[string]any{"items": out, "count": len(out)}, result{Summary: fmt.Sprintf("%d dealer", len(out))}, nil
	})
	tool(s, "kredit.check", ScopeRead, "Sisa limit dealer, state, dan simulasi rilis sebesar amount.", func(ctx context.Context, _ *Client, in creditIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		it, err := s.dealer(b, in.DealerID, "")
		if err != nil {
			return nil, result{}, err
		}
		cr := it.Metrics.Credit
		out := map[string]any{"dealer": it.Name, "credit": cr, "credit_limit": it.CreditLimit, "open_invoices": views.OpenInvoices(b.Data.Histories[it.UUID], b.Today)}
		if in.Amount > 0 {
			after := cr.Exposure + in.Amount
			out["simulasi"] = map[string]any{"amount": in.Amount, "exposure_after": after, "over_limit": it.CreditLimit > 0 && after > it.CreditLimit,
				"butuh": map[bool]string{true: "approve CEO (rilis di atas limit, SOP-SEC-001)", false: "dalam limit"}[it.CreditLimit > 0 && after > it.CreditLimit]}
		}
		return out, result{Summary: it.Name + " · " + cr.State}, nil
	})
	tool(s, "stok.aging", ScopeRead, "Stok menua per cabang dengan kandidat dealer push (glossary Push stok).", func(ctx context.Context, _ *Client, in stockIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		st, err := s.St.Q.ListStockItems(ctx)
		if err != nil {
			return nil, result{}, err
		}
		var out []views.AgingItem
		for _, x := range b.StockAging(views.StockItems(st), in.Branch) {
			if in.MinDays == 0 || x.AgeDays >= in.MinDays {
				out = append(out, x)
			}
		}
		return map[string]any{"items": out}, result{Summary: fmt.Sprintf("%d SKU", len(out))}, nil
	})
	tool(s, "chat.thread", ScopeRead, "Pesan WhatsApp dengan dealer (PII dimasking bila mask_pii_in_read).", func(ctx context.Context, _ *Client, in threadIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		it, err := s.dealer(b, in.DealerID, "")
		if err != nil {
			return nil, result{}, err
		}
		limit := in.Limit
		if limit <= 0 || limit > 100 {
			limit = 30
		}
		threads, err := s.St.Q.ListThreads(ctx)
		if err != nil {
			return nil, result{}, err
		}
		_, _, mask, _ := s.permissions(ctx)
		var names []string
		for _, c := range b.Data.Histories[it.UUID].Contacts {
			names = append(names, c.Name)
		}
		m := llm.NewMasker()
		var msgs []map[string]any
		for _, t := range threads {
			if t.DealerID == nil || *t.DealerID != it.UUID {
				continue
			}
			rows, err := s.St.Q.ListThreadMessages(ctx, gen.ListThreadMessagesParams{ThreadID: &t.ID, SentAt: s.Clock.Now().Add(24 * time.Hour), Limit: int32(limit)})
			if err != nil {
				return nil, result{}, err
			}
			for _, r := range rows {
				body, from, who := deref(r.Body), deref(r.FromNumber), deref(r.FromName)
				if mask {
					body, from, who = m.Mask(body, names), m.Mask(from, nil), m.Mask(who, names)
				}
				msgs = append(msgs, map[string]any{"at": r.SentAt, "direction": deref(r.Direction), "from": who, "from_number": from, "text": body, "sales": deref(t.SalesName)})
			}
		}
		return map[string]any{"dealer": it.Name, "masked": mask, "messages": msgs}, result{Summary: fmt.Sprintf("%d pesan", len(msgs))}, nil
	})
	tool(s, "kpi.utama", ScopeRead, "KPI utama: order tepat jadwal, DSO (order → bayar), perputaran stok.", func(ctx context.Context, _ *Client, in branchIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		st, _ := s.St.Q.ListStockItems(ctx)
		k := b.KPI(in.Branch, views.StockItems(st))
		return k, result{Summary: fmt.Sprintf("tepat jadwal %d%% · DSO %d hr", k.OnSchedulePct, k.DSODays)}, nil
	})
	tool(s, "cycles.recent", ScopeRead, "Riwayat siklus Orchestrator dengan counter.", func(ctx context.Context, _ *Client, in limitIn) (any, result, error) {
		n := in.Limit
		if n <= 0 || n > 50 {
			n = 10
		}
		rows, err := s.St.Q.ListCycles(ctx, int32(n))
		if err != nil {
			return nil, result{}, err
		}
		return map[string]any{"items": rows}, result{Summary: fmt.Sprintf("%d siklus", len(rows))}, nil
	})
	tool(s, "proposals.list", ScopeRead, "Antrean keputusan (baca saja — keputusan hanya di aplikasi).", func(ctx context.Context, _ *Client, in statusIn) (any, result, error) {
		st := in.Status
		if st == "" {
			st = "proposed"
		}
		rows, err := s.St.Q.ListProposals(ctx, gen.ListProposalsParams{Status: &st, Lim: 100})
		if err != nil {
			return nil, result{}, err
		}
		var out []map[string]any
		for _, p := range rows {
			out = append(out, map[string]any{"id": p.ID, "agent": p.Agent, "kind": p.Kind, "dealer": deref(p.DealerName), "title": p.Title, "why": p.Why,
				"confidence": p.Confidence, "autonomy": p.Autonomy, "status": p.Status, "due": deref(p.DueLabel), "signal_ids": p.SignalIds})
		}
		return map[string]any{"items": out, "count": len(out)}, result{Summary: fmt.Sprintf("%d proposal", len(out))}, nil
	})
}

// maskJSON masks phone numbers and e-mails in every string of v.
func maskJSON(v any) any {
	b, _ := json.Marshal(v)
	var tree any
	_ = json.Unmarshal(b, &tree)
	m := llm.NewMasker()
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case string:
			return m.Mask(t, nil)
		case []any:
			for i := range t {
				t[i] = walk(t[i])
			}
		case map[string]any:
			for k, e := range t {
				if k == "wa_number" || k == "phone" {
					t[k] = "<NO>"
					continue
				}
				t[k] = walk(e)
			}
		}
		return x
	}
	return walk(tree)
}

// ---------- analyze ----------

func (s *Server) registerAnalyze() {
	tool(s, "analisis.dealer", ScopeAnalyze, "Orchestrator menganalisis ulang satu dealer (scope dealer) dan mengembalikan saran terbarunya.", func(ctx context.Context, c *Client, in dealerIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		it, err := s.dealer(b, in.ID, in.Name)
		if err != nil {
			return nil, result{}, err
		}
		out, res, err := s.cycle(ctx, c, domain.Scope{Kind: "dealer", ID: it.ID})
		if err != nil {
			return nil, res, err
		}
		rows, _ := s.St.Q.ListProposals(ctx, gen.ListProposalsParams{Dealer: &it.UUID, Lim: 20})
		var ps []map[string]any
		for _, p := range rows {
			if p.CreatedAt.Before(clock.Today(s.Clock.Now())) {
				continue
			}
			ps = append(ps, map[string]any{"id": p.ID, "agent": p.Agent, "kind": p.Kind, "title": p.Title, "why": p.Why, "status": p.Status, "confidence": p.Confidence})
		}
		out["dealer"], out["proposals"] = dealerRow(it), ps
		return out, res, nil
	})
	tool(s, "analisis.segmen", ScopeAnalyze, "Ringkasan satu segmen + 3 tindakan.", func(ctx context.Context, _ *Client, in segmentIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		var ds []map[string]any
		var acts []string
		var omzet int64
		for _, it := range b.Items {
			if it.Metrics.Segment != in.Segment {
				continue
			}
			omzet += it.Metrics.OmzetBln
			ds = append(ds, dealerRow(it))
			if it.Next != nil && it.Next.Status == "proposed" && len(acts) < 3 {
				acts = append(acts, fmt.Sprintf("%s: %s (%s)", it.Name, it.Next.Title, it.Next.Agent))
			}
		}
		return map[string]any{"segment": in.Segment, "dealers": ds, "omzet_bln": omzet, "tindakan": acts}, result{Summary: fmt.Sprintf("segmen %s · %d dealer", in.Segment, len(ds))}, nil
	})
	tool(s, "analisis.kas", ScopeAnalyze, "Prediksi kas masuk tertimbang pola bayar + 2 tindakan yang paling menaikkan.", func(ctx context.Context, _ *Client, in daysIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		days := in.Days
		if days <= 0 {
			days = 30
		}
		ov, ar, _, fc, total := b.Credit(days)
		var acts []string
		for _, r := range ar {
			if r.Overdue > 0 && len(acts) < 2 {
				acts = append(acts, fmt.Sprintf("Tagih %s: lewat %d hari, %d jt", r.Name, r.LateDays, r.Overdue/1_000_000))
			}
		}
		return map[string]any{"overview": ov, "forecast": fc, "expected_total": total, "tindakan": acts}, result{Summary: fmt.Sprintf("kas masuk %d hari: Rp %d jt", days, total/1_000_000)}, nil
	})
	tool(s, "analisis.stok", ScopeAnalyze, "Kandidat push per SKU stok menua.", func(ctx context.Context, _ *Client, in branchIn) (any, result, error) {
		b, err := s.views.Board(ctx)
		if err != nil {
			return nil, result{}, err
		}
		st, err := s.St.Q.ListStockItems(ctx)
		if err != nil {
			return nil, result{}, err
		}
		items := b.StockAging(views.StockItems(st), in.Branch)
		return map[string]any{"items": items, "critical": b.StockCritical(views.StockItems(st))}, result{Summary: fmt.Sprintf("%d SKU menua", len(items))}, nil
	})
}

// ---------- orchestrate ----------

// cycle queues an Orchestrator cycle for an MCP client (trigger mcp) and waits briefly for it to finish.
func (s *Server) cycle(ctx context.Context, c *Client, sc domain.Scope) (map[string]any, result, error) {
	allow, _, _, maxCycles := s.permissions(ctx)
	if !allow {
		return nil, result{}, fmt.Errorf("%w: kebijakan mcp.permissions.allow_reanalyze mati", ErrForbidden)
	}
	n, err := s.St.Q.MCPCyclesSince(ctx, gen.MCPCyclesSinceParams{RequestedBy: &c.Name, Since: s.Clock.Now().Add(-time.Hour)})
	if err != nil {
		return nil, result{}, err
	}
	if int(n) >= maxCycles {
		return nil, result{}, fmt.Errorf("%w: maksimal %d siklus per jam untuk klien ini", ErrRateLimited, maxCycles)
	}
	var cyc gen.Cycle
	err = s.St.Tx(ctx, func(q *gen.Queries, tx pgx.Tx) error {
		cy, err := s.Orch.Queue(ctx, q, sc, domain.Trigger{Source: "mcp", By: c.Name, Via: "mcp"})
		if err != nil {
			return err
		}
		cyc = cy
		if s.Jobs != nil {
			_, err = s.Jobs.InsertTx(ctx, tx, jobs.CycleRunArgs{CycleID: cy.ID.String()}, nil)
		}
		return err
	})
	if errors.Is(err, orchestrator.ErrRunning) {
		return nil, result{}, errors.New("cycle_running: Orchestrator sedang berjalan, coba lagi setelah siklus selesai")
	}
	if err != nil {
		return nil, result{}, err
	}
	id := cyc.ID
	if s.Jobs == nil {
		if _, err := s.Orch.Execute(ctx, id); err != nil {
			return nil, result{CycleID: &id}, err
		}
	}
	wait := s.CycleCap
	if wait == 0 {
		wait = 25 * time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		cyc, err = s.St.Q.GetCycle(ctx, id)
		if err != nil || (cyc.Status != "queued" && cyc.Status != "running") || time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, result{CycleID: &id}, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	out := map[string]any{"cycle_id": id, "number": cyc.Number, "status": cyc.Status, "scope": cyc.Scope, "signals": cyc.SignalsCount,
		"auto": cyc.AutoCount, "decisions": cyc.DecisionCount, "conflicts": cyc.ConflictCount, "note": deref(cyc.Note)}
	if cyc.Status == "queued" || cyc.Status == "running" {
		out["hint"] = "siklus masih berjalan — cek orchestrator.status"
	}
	return out, result{Summary: fmt.Sprintf("siklus #%d · %s · %s", deref(cyc.Number), cyc.Scope, cyc.Status), CycleID: &id}, nil
}

func (s *Server) registerOrchestrate() {
	tool(s, "orchestrator.run", ScopeOrchestrate, "Siklus penuh Orchestrator (6 tahap). Hasil masuk antrean Keputusan, tidak langsung ke dealer.", func(ctx context.Context, c *Client, _ empty) (any, result, error) {
		return s.cycle(ctx, c, domain.Scope{Kind: "all"})
	})
	tool(s, "orchestrator.reanalyze", ScopeOrchestrate, "Analisis ulang dengan scope: all | screen:<orbit|segmen|stock|credit> | dealer:<slug> | agent:<nama>.", func(ctx context.Context, c *Client, in scopeIn) (any, result, error) {
		sc, err := domain.ParseScope(in.Scope)
		if err != nil {
			return nil, result{}, err
		}
		if sc.Kind == "dealer" {
			b, err := s.views.Board(ctx)
			if err != nil {
				return nil, result{}, err
			}
			it, err := s.dealer(b, sc.ID, "")
			if err != nil {
				return nil, result{}, err
			}
			sc.ID = it.ID
		}
		return s.cycle(ctx, c, sc)
	})
	tool(s, "orchestrator.agent.run", ScopeOrchestrate, "Menjalankan satu agen (scope agent; dengan dealer_id → scope dealer).", func(ctx context.Context, c *Client, in agentRunIn) (any, result, error) {
		if in.DealerID != "" {
			return s.cycle(ctx, c, domain.Scope{Kind: "dealer", ID: in.DealerID})
		}
		sc, err := domain.ParseScope("agent:" + in.Agent)
		if err != nil {
			return nil, result{}, err
		}
		return s.cycle(ctx, c, sc)
	})
	tool(s, "orchestrator.plan", ScopeOrchestrate, "Rencana hari ini (langkah, jam, otonom/butuh approve, status).", func(ctx context.Context, _ *Client, in planIn) (any, result, error) {
		day := clock.Today(s.Clock.Now())
		if in.Date != "" {
			t, err := time.ParseInLocation("2006-01-02", in.Date, clock.WIB)
			if err != nil {
				return nil, result{}, err
			}
			day = t
		}
		rows, err := s.St.Q.ListPlan(ctx, day)
		if err != nil {
			return nil, result{}, err
		}
		var out []map[string]any
		for _, it := range rows {
			out = append(out, map[string]any{"item_id": it.ID, "time": deref(it.TimeLabel), "agent": deref(it.Agent), "autonomy": deref(it.Autonomy),
				"text": stripLinks(deref(it.TextHtml)), "status": it.Status, "proposal_ids": it.ProposalIds})
		}
		return map[string]any{"date": day.Format("2006-01-02"), "items": out}, result{Summary: fmt.Sprintf("%d langkah", len(out))}, nil
	})
	tool(s, "orchestrator.plan.update", ScopeOrchestrate, "Mengusulkan perubahan Rencana hari ini (move | skip | add). Menjadi proposal yang butuh approve manusia.", func(ctx context.Context, c *Client, in planUpdateIn) (any, result, error) {
		_, allowPlan, _, _ := s.permissions(ctx)
		if !allowPlan {
			return nil, result{}, fmt.Errorf("%w: kebijakan mcp.permissions.allow_plan_update_proposal mati", ErrForbidden)
		}
		return s.planChange(ctx, c, in)
	})
	tool(s, "orchestrator.input.get", ScopeOrchestrate, "Input agen untuk satu siklus (dimasking) — klien MCP menjadi mesin analisis lalu memanggil orchestrator.submit.", func(ctx context.Context, _ *Client, in inputGetIn) (any, result, error) {
		id, err := uuid.Parse(in.CycleID)
		if err != nil {
			return nil, result{}, fmt.Errorf("cycle_id: %w", err)
		}
		raw, err := s.Orch.InputFor(ctx, id, in.Agent)
		if err != nil {
			return nil, result{CycleID: &id}, err
		}
		var v any
		_ = json.Unmarshal(raw, &v)
		return v, result{Summary: in.Agent, CycleID: &id}, nil
	})
	tool(s, "orchestrator.submit", ScopeOrchestrate, "Mengirim proposal untuk Input yang diminta. Divalidasi: kind agen, signal_ids hanya dari Input, dealer dikenal.", func(ctx context.Context, c *Client, in submitIn) (any, result, error) {
		id, err := uuid.Parse(in.CycleID)
		if err != nil {
			return nil, result{}, fmt.Errorf("cycle_id: %w", err)
		}
		raw, _ := json.Marshal(in.Proposals)
		res, err := s.Orch.Submit(ctx, id, in.Agent, raw, &c.ID)
		if err != nil {
			return nil, result{CycleID: &id}, err
		}
		return res, result{Summary: fmt.Sprintf("%d diterima · %d ditolak · %s", res.Accepted, len(res.Rejected), res.Mode), CycleID: &id}, nil
	})
	tool(s, "orchestrator.status", ScopeOrchestrate, "Status Orchestrator: siklus berjalan/terakhir, tahap, jalur, berikutnya.", func(ctx context.Context, _ *Client, _ empty) (any, result, error) {
		out := map[string]any{"running": false}
		if c, err := s.St.Q.LatestCycle(ctx); err == nil {
			out["cycle"] = map[string]any{"id": c.ID, "number": c.Number, "status": c.Status, "stage": c.Stage, "scope": c.Scope, "via": c.Via, "note": c.Note}
			out["running"] = c.Status == "running" || c.Status == "queued"
		}
		now := s.Clock.Now().In(clock.WIB)
		next := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, clock.WIB)
		if next.Hour() > 20 || next.Hour() < 6 {
			next = time.Date(now.Year(), now.Month(), now.Day()+1, 6, 0, 0, 0, clock.WIB)
		}
		out["next_at"] = next
		return out, result{Summary: fmt.Sprintf("running=%v", out["running"])}, nil
	})
	tool(s, "actions.decide", "decide", "Keputusan (setujui/tolak) — human-only: selalu ditolak untuk klien MCP.", func(context.Context, *Client, decideIn) (any, result, error) {
		return nil, result{}, ErrHumanOnly
	})
}

// planChange turns a plan update request into a plan_change proposal that a human approves in Keputusan.
func (s *Server) planChange(ctx context.Context, c *Client, in planUpdateIn) (any, result, error) {
	switch in.Action {
	case "move", "skip", "add":
	default:
		return nil, result{}, fmt.Errorf("action %q: move | skip | add", in.Action)
	}
	var item gen.PlanItem
	var sigs []uuid.UUID
	if in.Action != "add" {
		id, err := uuid.Parse(in.ItemID)
		if err != nil {
			return nil, result{}, fmt.Errorf("item_id: %w", err)
		}
		item, err = s.St.Q.GetPlanItem(ctx, id)
		if err != nil {
			return nil, result{}, fmt.Errorf("langkah %s tidak ditemukan", in.ItemID)
		}
		for _, pid := range item.ProposalIds {
			if p, err := s.St.Q.GetProposal(ctx, pid); err == nil {
				sigs = append(sigs, p.SignalIds...)
			}
		}
	}
	if len(sigs) == 0 {
		if l, err := s.St.Q.LatestStockSignals(ctx); err == nil && len(l) > 0 {
			sigs = []uuid.UUID{l[0].ID} // provenance: the newest signal the plan was built on
		}
	}
	if len(sigs) == 0 {
		return nil, result{}, errors.New("tidak ada sinyal sumber untuk perubahan ini")
	}
	text := stripLinks(deref(item.TextHtml))
	title := map[string]string{"move": fmt.Sprintf("Pindahkan langkah %s → %s", deref(item.TimeLabel), in.Time), "skip": "Lewati langkah " + deref(item.TimeLabel),
		"add": fmt.Sprintf("Tambah langkah %s: %s", in.Time, in.Text)}[in.Action]
	p := domain.Proposal{Agent: "MCP · " + c.Name, Kind: domain.KindPlanChange, Title: title, Summary: text,
		Why: strings.TrimSpace("Diusulkan klien MCP " + c.Name + ". " + in.Reason), Confidence: 0.7, SignalIDs: sigs, Autonomy: "approve", Icon: "cal", Button: "Setujui perubahan",
		Pills:     [][2]string{{"indigo", "MCP"}, {"neutral", c.Name}},
		Steps:     []string{"Rencana hari ini diperbarui setelah Anda setujui"},
		Payload:   map[string]any{"action": in.Action, "item_id": in.ItemID, "time": in.Time, "text": in.Text, "client": c.Name},
		DedupeKey: fmt.Sprintf("plan_change:%s:%s:%s:%s", in.Action, in.ItemID, in.Time, s.Clock.Now().Format("2006-01-02T15:04"))}
	pid, err := insertProposal(ctx, s, p)
	if err != nil {
		return nil, result{}, err
	}
	return map[string]any{"proposal_id": pid, "status": "proposed", "message": "Menunggu approve manusia di Keputusan"}, result{Summary: title}, nil
}

func stripLinks(s string) string {
	for {
		i := strings.Index(s, "[[")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "]]")
		if j < 0 {
			return s
		}
		inner := s[i+2 : i+j]
		if _, label, ok := strings.Cut(inner, "|"); ok {
			inner = label
		}
		s = s[:i] + inner + s[i+j+2:]
	}
}

func (s *Server) registerTools() {
	s.registerRead()
	s.registerAnalyze()
	s.registerOrchestrate()
}
