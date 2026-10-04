package insights

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"arc/packages/core/domain"
)

// PayPattern summarises how an account pays (median days from invoice to payment).
type PayPattern struct {
	MedianDays int
	Count      int
	Kind       string // tepat | lambat | termin_anggaran | baru
	OnTimePct  int
}

// PayPatterns returns the payment pattern per account.
func (s *Service) PayPatterns(ctx context.Context) (map[string]PayPattern, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT p.account_id, array_agg(p.days_to_pay ORDER BY p.days_to_pay), array_agg(p.term_days), a.is_government
		FROM payment_history p JOIN accounts a ON a.id=p.account_id GROUP BY p.account_id, a.is_government`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]PayPattern{}
	for rows.Next() {
		var acc string
		var days, terms []int32
		var gov bool
		if err := rows.Scan(&acc, &days, &terms, &gov); err != nil {
			return nil, err
		}
		med := int(days[len(days)/2])
		ontime := 0
		for i, d := range days {
			if int(d) <= int(terms[i])+5 {
				ontime++
			}
		}
		p := PayPattern{MedianDays: med, Count: len(days), OnTimePct: ontime * 100 / len(days)}
		switch {
		case gov && med > 60:
			p.Kind = "termin_anggaran"
		case med <= 36:
			p.Kind = "tepat"
		default:
			p.Kind = "lambat"
		}
		out[acc] = p
	}
	return out, rows.Err()
}

// Invoice is an open receivable.
type Invoice struct {
	ID, Number, AccountID, Account, Label, Category, PayPattern string
	Amount, Residual                                            float64
	InvoiceDate, DueDate                                        time.Time
	SPM                                                         *time.Time
	Gov                                                         bool
}

// OpenInvoices returns unpaid invoices of a category ("" = all).
func (s *Service) OpenInvoices(ctx context.Context, category string) ([]Invoice, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT id, number, COALESCE(account_id,''), account_name, label, category, pay_pattern, amount::float8, residual::float8,
		invoice_date, due_date, spm_submitted_at, is_government FROM invoices WHERE residual > 0 AND ($1='' OR category=$1) ORDER BY due_date`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invoice
	for rows.Next() {
		var i Invoice
		if err := rows.Scan(&i.ID, &i.Number, &i.AccountID, &i.Account, &i.Label, &i.Category, &i.PayPattern, &i.Amount, &i.Residual,
			&i.InvoiceDate, &i.DueDate, &i.SPM, &i.Gov); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// CashForecastItem is one expected receipt in the next 30 days.
type CashForecastItem struct {
	ID       string
	Label    string
	Detail   string
	Amount   float64
	P        float64
	Value    float64
	CashItem string
}

// CashForecast returns expected receipts within 30 days weighted by the account's
// payment pattern and the L2C stage. whatIf: "create_invoice:<cash_item_id>".
func (s *Service) CashForecast(ctx context.Context, whatIf string) ([]CashForecastItem, float64, error) {
	now := domain.Now()
	horizon := now.AddDate(0, 0, 30)
	patterns, err := s.PayPatterns(ctx)
	if err != nil {
		return nil, 0, err
	}
	invs, err := s.OpenInvoices(ctx, "project")
	if err != nil {
		return nil, 0, err
	}
	var items []CashForecastItem
	for _, inv := range invs {
		pat := patterns[inv.AccountID]
		expected := inv.DueDate
		if pat.MedianDays > 0 {
			if e := inv.InvoiceDate.AddDate(0, 0, pat.MedianDays); e.After(expected) {
				expected = e
			}
		}
		var p float64
		var detail string
		switch {
		case inv.Gov && pat.Kind == "termin_anggaran" && inv.SPM != nil:
			p = 0.4
			detail = fmt.Sprintf("%s · SPM diproses ±3 minggu", domain.FormatRp(inv.Residual))
		case inv.Gov && pat.Kind == "termin_anggaran":
			p = 0.25
			detail = fmt.Sprintf("%s · termin anggaran, SPM belum diajukan", domain.FormatRp(inv.Residual))
		case expected.After(horizon):
			continue
		default:
			p = 0.85
			if pat.Kind == "lambat" {
				p = 0.6
			}
			left := domain.DaysBetween(now, expected)
			if left < 7 {
				left = 7
			}
			if pat.MedianDays > 0 {
				detail = fmt.Sprintf("Invoice %s · pola bayar %d hari → ±%d hari lagi", domain.FormatRp(inv.Residual), pat.MedianDays, left)
			} else {
				detail = fmt.Sprintf("Invoice %s · jatuh tempo %s", domain.FormatRp(inv.Residual), domain.ShortDate(inv.DueDate))
			}
		}
		items = append(items, CashForecastItem{ID: inv.ID, Label: inv.Account, Detail: detail, Amount: inv.Residual, P: p})
	}
	rows, err := s.DB.Pool.Query(ctx, `SELECT e.id, e.label, e.detail, e.amount::float8, e.trigger, COALESCE(e.cash_item_id,'') FROM expected_receipts e ORDER BY e.amount DESC`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var it CashForecastItem
		var trigger string
		if err := rows.Scan(&it.ID, &it.Label, &it.Detail, &it.Amount, &trigger, &it.CashItem); err != nil {
			return nil, 0, err
		}
		switch trigger {
		case "after_bast":
			it.P = 0.6
		case "after_po":
			it.P = 0.7
		case "invoice_pending":
			it.P = 0.35
			if whatIf == "create_invoice:"+it.CashItem {
				it.P = 0.7
			}
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Amount > items[j].Amount })
	total := 0.0
	for i := range items {
		items[i].Value = math.Round(items[i].Amount*items[i].P/1e6) * 1e6
		total += items[i].Value
	}
	return items, total, nil
}

// AgingBucket is one row of the receivables aging card.
type AgingBucket struct {
	Label  string
	Amount float64
	Count  int
	Gov    int
}

// Aging buckets open project invoices by days past due.
func (s *Service) Aging(ctx context.Context) ([]AgingBucket, float64, int, error) {
	invs, err := s.OpenInvoices(ctx, "project")
	if err != nil {
		return nil, 0, 0, err
	}
	now := domain.Now()
	b := []AgingBucket{{Label: "Belum jatuh tempo"}, {Label: "1–30 hari"}, {Label: "31–60 hari"}, {Label: "> 60 hari"}}
	total := 0.0
	for _, inv := range invs {
		late := domain.DaysBetween(inv.DueDate, now)
		i := 0
		switch {
		case late > 60:
			i = 3
		case late > 30:
			i = 2
		case late > 0:
			i = 1
		}
		b[i].Amount += inv.Residual
		b[i].Count++
		if inv.Gov {
			b[i].Gov++
		}
		total += inv.Residual
	}
	return b, total, len(invs), nil
}

// FormatM renders billions with up to two decimals even below 1 M ("Rp 0,98 M").
func FormatM(v float64) string {
	if v == 0 {
		return "Rp 0"
	}
	s := fmt.Sprintf("%.2f", v/1e9)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return "Rp " + strings.ReplaceAll(s, ".", ",") + " M"
}

// L2CItem is a Won → Lunas row.
type L2CItem struct {
	ID, AccountID, Account, Project, SO, Owner, Note, Stage, InvoiceID string
	Value                                                              float64
	Days, Bench                                                        int
	ActionID                                                           string
}

// L2C returns lead-to-cash items with days in stage.
func (s *Service) L2C(ctx context.Context) ([]L2CItem, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT c.id, COALESCE(c.account_id,''), c.account_name, c.project, c.so_id, COALESCE(u.name,''), c.note, c.stage,
		COALESCE(c.invoice_id,''), c.value::float8, c.stage_entered_at, c.benchmark_days,
		COALESCE((SELECT a.id FROM actions a WHERE a.cash_item_id=c.id ORDER BY a.created_at DESC LIMIT 1),'')
		FROM cash_items c LEFT JOIN users u ON u.id=c.owner_user_id ORDER BY c.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := domain.Now()
	var out []L2CItem
	for rows.Next() {
		var it L2CItem
		var entered time.Time
		if err := rows.Scan(&it.ID, &it.AccountID, &it.Account, &it.Project, &it.SO, &it.Owner, &it.Note, &it.Stage, &it.InvoiceID, &it.Value, &entered, &it.Bench, &it.ActionID); err != nil {
			return nil, err
		}
		it.Days = domain.DaysBetween(entered, now)
		out = append(out, it)
	}
	return out, rows.Err()
}

// L2CTone classifies days vs benchmark: ≥ 2× bad, > 1× warn.
func L2CTone(it L2CItem) string {
	if it.Stage == "lunas" {
		return "good"
	}
	switch {
	case it.Bench > 0 && it.Days >= 2*it.Bench:
		return "bad"
	case it.Days > it.Bench:
		return "warn"
	}
	return "good"
}
