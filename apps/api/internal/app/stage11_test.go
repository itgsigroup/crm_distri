package app

import (
	"context"
	"math"
	"strings"
	"testing"
)

// Stage 11: the Collection agent proposes the right next step per project —
// Panti Rapih create_invoice, Semen Mitra a friendly reminder, Magelang SPM documents,
// Cakra schedule BAST — and does not repeat itself.
func TestStage11CollectionActions(t *testing.T) {
	a, _ := fresh(t)
	ctx := context.Background()
	// Remove the seeded fixture copies so the agent has to derive them itself.
	exec(t, a, `UPDATE opportunities SET next_action_id=NULL WHERE next_action_id IN (SELECT id FROM actions WHERE id LIKE 'l2c_%')`)
	exec(t, a, `DELETE FROM actions WHERE id LIKE 'l2c_%'`)
	n, err := a.Agents.Collection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ acc, typ, extra string }{
		{"panti", "create_invoice", "cash_item_id='l2c-panti'"},
		{"semen", "payment_reminder", "title ILIKE '%ramah%' AND payload->>'invoice_id'='inv-0702'"},
		{"magelang", "ask_spm_documents", "payload->>'invoice_id'='inv-0688'"},
		{"cakra", "schedule_bast", "cash_item_id='l2c-cakra'"},
	}
	for _, w := range want {
		q := `SELECT count(*) FROM actions WHERE account_id=$1 AND type=$2 AND status='proposed' AND agent='Collection agent' AND jsonb_array_length(evidence) > 0 AND ` + w.extra
		if c := count(t, a, q, w.acc, w.typ); c != 1 {
			t.Errorf("%s: %d %s proposals (want 1)", w.acc, c, w.typ)
		}
	}
	if count(t, a, `SELECT count(*) FROM actions WHERE account_id='salatiga' AND agent='Collection agent' AND status='proposed'`) != 0 {
		t.Error("Salatiga (still in preparation) got a collection action")
	}
	if count(t, a, `SELECT count(*) FROM actions WHERE account_id='magelang' AND type='payment_reminder'`) != 0 {
		t.Error("government account got a dunning reminder instead of the SPM check")
	}
	if n < 4 {
		t.Fatalf("collection created %d actions", n)
	}
	if again, _ := a.Agents.Collection(ctx); again != 0 {
		t.Fatalf("second collection run created %d", again)
	}
}

// Stage 11: the 30-day cash forecast is ≈ Rp 2,4 M and the Panti Rapih invoice what-if raises it.
func TestStage11CashForecast(t *testing.T) {
	a, srv := fresh(t)
	ctx := context.Background()
	_, base, err := a.Ins.CashForecast(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if math.Round(base/1e8)/10 != 2.4 {
		t.Fatalf("cash forecast Rp %.3f M, want ≈ 2,4", base/1e9)
	}
	_, what, err := a.Ins.CashForecast(ctx, "create_invoice:l2c-panti")
	if err != nil {
		t.Fatal(err)
	}
	if what <= base || math.Abs(what-2.652e9) > 0.01e9 {
		t.Fatalf("what-if Panti invoice Rp %.3f M (base %.3f), want ≈ 2,652", what/1e9, base/1e9)
	}
	// Same numbers through the API (and the CSV export).
	sam := login(t, srv, "sam@gsi.co.id")
	code, body := sam.do("GET", "/api/cash/forecast?what_if=create_invoice:l2c-panti", nil)
	if code != 200 || !strings.Contains(string(body), "2652") && !strings.Contains(string(body), "2,65") {
		t.Fatalf("cash forecast API: %d %.300s", code, body)
	}
	if code, _ := sam.do("GET", "/api/cash/forecast.csv", nil); code != 200 {
		t.Fatalf("cash forecast CSV: %d", code)
	}
}
