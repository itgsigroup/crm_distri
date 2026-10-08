package analyst

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"distri-arc/internal/clock"
)

// template writes a numbers-only report from the same MCP tools (no model): data_ringkasan and penjualan_bulanan.
func (r *Runner) template(ctx context.Context, cs *sdk.ClientSession, note string) outcome {
	o := outcome{status: "template", engine: "template"}
	get := func(name string, args map[string]any) map[string]any {
		t0 := time.Now()
		b, _ := json.Marshal(args)
		st := Step{Tool: name, Args: b, Status: "ok"}
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		st.MS = time.Since(t0).Milliseconds()
		var out map[string]any
		switch {
		case err != nil:
			st.Status, st.Summary = "error", err.Error()
		case res.IsError:
			st.Status, st.Summary = "error", clip(resultText(res), 200)
		default:
			_ = json.Unmarshal([]byte(resultText(res)), &out)
		}
		o.steps = append(o.steps, st)
		return out
	}
	sum := get("data_ringkasan", map[string]any{})
	if sum == nil {
		o.err = fmt.Errorf("data_ringkasan gagal: %s", o.steps[len(o.steps)-1].Summary)
		return o
	}
	months := get("penjualan_bulanan", map[string]any{"months": 3})

	now := r.Clock.Now().In(clock.WIB)
	var b strings.Builder
	fmt.Fprintf(&b, "## Ringkasan bisnis · %s\n\n> %s\n\n", longDate(now), note)

	dealer, kpi, kredit, stok := obj(sum, "dealer"), obj(sum, "kpi"), obj(sum, "kredit"), obj(sum, "stok")
	tgt := obj(kpi, "targets")
	aging := obj(stok, "menua_lebih_90_hari")
	b.WriteString("### Kondisi\n")
	fmt.Fprintf(&b, "- **Dealer**: %s · omzet %s/bln\n", num(n(dealer, "total")), rp(n(dealer, "omzet_bln")))
	fmt.Fprintf(&b, "- **Order tepat jadwal** %d%% (target %d%%) · **DSO** %d hari (target %d) · **Perputaran stok** %d hari (target %d)\n",
		n(kpi, "on_schedule_pct"), n(tgt, "on_schedule_pct"), n(kpi, "dso_days"), n(tgt, "dso_days"), n(kpi, "stock_turn_days"), n(tgt, "stock_turn_days"))
	fmt.Fprintf(&b, "- **Piutang** %s · lewat tempo %s (%d dealer, %d > 30 hari) · prediksi kas 30 hari %s\n",
		rp(n(kredit, "receivable")), rp(n(kredit, "overdue")), n(kredit, "overdue_dealers"), n(kredit, "overdue_over_30"), rp(n(sum, "prediksi_kas_30_hari")))
	fmt.Fprintf(&b, "- **Stok** %s · menua > 90 hari %d SKU (%s) · kritis %d SKU\n\n", rp(n(stok, "nilai")), n(aging, "sku"), rp(n(aging, "nilai")), n(stok, "kritis_sku"))

	if rows, ok := months["months"].([]any); ok && len(rows) > 0 {
		b.WriteString("### Penjualan 3 bulan\n| Bulan | Omzet | Order | Dealer aktif | Margin |\n|---|---:|---:|---:|---:|\n")
		for _, x := range rows {
			m, _ := x.(map[string]any)
			margin, _ := m["margin_pct"].(float64)
			label, _ := m["bulan"].(string)
			if t, err := time.ParseInLocation("2006-01", label, clock.WIB); err == nil {
				label = strings.TrimPrefix(clock.DayMonth(t), "1 ") + fmt.Sprintf(" %d", t.Year())
				if t.Year() == now.Year() && t.Month() == now.Month() {
					label += " (berjalan)"
				}
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s%% |\n", label, rp(n(m, "omzet")), num(n(m, "order")), num(n(m, "dealer_aktif")),
				strings.Replace(fmt.Sprintf("%.1f", margin), ".", ",", 1))
		}
		b.WriteString("\n")
	}
	table := func(title, col string, m map[string]any) {
		type row struct {
			k        string
			c, omzet int64
		}
		var rs []row
		for k, v := range m {
			x, _ := v.(map[string]any)
			rs = append(rs, row{k, n(x, "count"), n(x, "omzet_bln")})
		}
		if len(rs) == 0 {
			return
		}
		sort.Slice(rs, func(i, j int) bool { return rs[i].omzet > rs[j].omzet })
		fmt.Fprintf(&b, "### %s\n| %s | Dealer | Omzet/bln |\n|---|---:|---:|\n", title, col)
		for _, x := range rs[:min(10, len(rs))] {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", x.k, num(x.c), rp(x.omzet))
		}
		b.WriteString("\n")
	}
	table("Per cabang", "Cabang", obj(dealer, "per_cabang"))
	table("Status dealer", "Status", obj(dealer, "per_status"))

	var todo []string
	if v, t := n(kpi, "on_schedule_pct"), n(tgt, "on_schedule_pct"); t > 0 && v < t {
		todo = append(todo, fmt.Sprintf("Order tepat jadwal %d%% di bawah target %d%% — follow-up dealer lewat jadwal (Orbit → Lewat jadwal).", v, t))
	}
	if v, t := n(kpi, "dso_days"), n(tgt, "dso_days"); t > 0 && v > t {
		todo = append(todo, fmt.Sprintf("DSO %d hari di atas target %d — prioritaskan penagihan (Kredit·kas).", v, t))
	}
	if v := n(kredit, "overdue"); v > 0 {
		todo = append(todo, fmt.Sprintf("Piutang lewat tempo %s di %d dealer — tinjau limit dan jadwal tagih.", rp(v), n(kredit, "overdue_dealers")))
	}
	if v, t := n(kpi, "stock_turn_days"), n(tgt, "stock_turn_days"); t > 0 && v > t {
		todo = append(todo, fmt.Sprintf("Perputaran stok %d hari di atas target %d — jalankan Push stok untuk stok menua %s.", v, t, rp(n(aging, "nilai"))))
	}
	if len(todo) > 0 {
		b.WriteString("### Perlu perhatian\n")
		for i, t := range todo {
			fmt.Fprintf(&b, "%d. %s\n", i+1, t)
		}
	}
	o.report = strings.TrimSpace(b.String())
	return o
}

func obj(m map[string]any, k string) map[string]any {
	v, _ := m[k].(map[string]any)
	return v
}

// n reads a JSON number as int64 (0 when missing).
func n(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	}
	return 0
}

// num formats 2841 as 2.841.
func num(v int64) string {
	s := fmt.Sprint(v)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// rp formats rupiah the way the UI does: Rp40,9 M · Rp850 jt · Rp12.500.
func rp(v int64) string {
	a := v
	if a < 0 {
		a = -a
	}
	var s string
	switch {
	case a >= 1e9:
		s = strings.Replace(fmt.Sprintf("%.1f M", float64(a)/1e9), ".", ",", 1)
	case a >= 1e6:
		s = fmt.Sprintf("%d jt", a/1e6)
	default:
		s = num(a)
	}
	if v < 0 {
		return "-Rp" + s
	}
	return "Rp" + s
}

var dayNames = []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// longDate is "Rabu, 7 Okt 2026".
func longDate(t time.Time) string {
	return fmt.Sprintf("%s, %s %d", dayNames[t.Weekday()], clock.DayMonth(t), t.Year())
}
