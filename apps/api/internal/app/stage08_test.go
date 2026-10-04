package app

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"arc/packages/connectors/odoo"
	"arc/packages/core/config"
	"arc/packages/core/domain"
)

func fakeOdoo(t *testing.T, a *App) *odoo.Fake {
	t.Helper()
	f, ok := a.OdooReader.(*odoo.Fake)
	if !ok {
		t.Fatalf("Odoo reader is %T, want *odoo.Fake", a.OdooReader)
	}
	return f
}

// Stage 08: a full sync repeated → 0 changes; touching one lead's write_date → exactly 1 update.
func TestStage08IncrementalSync(t *testing.T) {
	a, _ := fresh(t) // PostSeed already ran the first full sync
	ctx := context.Background()
	f := fakeOdoo(t, a)
	if n := count(t, a, `SELECT count(*) FROM odoo_records`); n < 40 {
		t.Fatalf("first sync mirrored only %d records", n)
	}
	accounts := count(t, a, `SELECT count(*) FROM accounts`)
	res, err := a.SyncOdoo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 0 || res.Linked != 0 || res.Proposed != 0 || res.CreateIn != 0 || res.L2CMoved != 0 {
		t.Fatalf("second sync changed something: %+v", res)
	}
	f.Touch("crm.lead", 203, "2026-09-29 08:15:00", odoo.Record{"probability": float64(65)})
	res, err = a.SyncOdoo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || res.PerModel["crm.lead"] != 1 {
		t.Fatalf("after touching one lead: %+v, want exactly 1 crm.lead update", res)
	}
	if count(t, a, `SELECT count(*) FROM opportunities WHERE id='semarang' AND source_id='203' AND probability=65`) != 1 {
		t.Fatal("linked opportunity did not take the Odoo value")
	}
	if count(t, a, `SELECT count(*) FROM accounts`) != accounts {
		t.Fatal("sync created accounts")
	}
	if len(f.Writes) != 0 {
		t.Fatalf("read-only sync wrote to Odoo: %+v", f.Writes)
	}
}

// Stage 08: of the 8 ARC opportunities 6 link automatically and 2 become link proposals;
// no duplicate accounts.
func TestStage08Linking(t *testing.T) {
	a, _ := fresh(t)
	ids := []string{}
	for _, d := range loadFixtures(t).Deals {
		ids = append(ids, d.ID)
	}
	linked := count(t, a, `SELECT count(*) FROM opportunities WHERE id = ANY($1) AND source_system='odoo' AND source_id IS NOT NULL AND locked_to_source`, ids)
	proposed := count(t, a, `SELECT count(*) FROM odoo_links WHERE arc_type='opportunity' AND arc_id = ANY($1) AND status='proposed'`, ids)
	if linked != 6 || proposed != 2 {
		t.Fatalf("linking: %d auto-linked, %d proposed — want 6 + 2", linked, proposed)
	}
	if n := count(t, a, `SELECT count(*) FROM actions WHERE type='link_to_odoo' AND status='proposed' AND payload->>'opportunity_id' = ANY($1)`, ids); n != 2 {
		t.Fatalf("%d link_to_odoo proposals, want 2", n)
	}
	if n := count(t, a, `SELECT count(*) FROM (SELECT odoo_partner_id FROM accounts WHERE odoo_partner_id IS NOT NULL GROUP BY 1 HAVING count(*) > 1) x`); n != 0 {
		t.Fatalf("%d Odoo partners linked to more than one account", n)
	}
	if n := count(t, a, `SELECT count(*) FROM (SELECT lower(name) FROM accounts GROUP BY 1 HAVING count(*) > 1) x`); n != 0 {
		t.Fatalf("%d duplicate account names", n)
	}
	if n := count(t, a, `SELECT count(*) FROM (SELECT source_id FROM opportunities WHERE source_system='odoo' GROUP BY 1 HAVING count(*) > 1) x`); n != 0 {
		t.Fatal("one Odoo lead linked to several ARC opportunities")
	}
	// Stage names come from Odoo crm.stage.
	if n := count(t, a, `SELECT count(*) FROM stage_definitions WHERE source_system='odoo'`); n != 4 {
		t.Fatalf("Odoo stages mirrored: %d", n)
	}
}

// Stage 08/11: lead-to-cash — the 6 projects sit in the mockup stage with the mockup day count.
func TestStage08L2CSixCases(t *testing.T) {
	a, _ := fresh(t)
	items, err := a.Ins.L2C(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	bySO := map[string]int{}
	for i, it := range items {
		bySO[it.SO] = i
	}
	fx := loadFixtures(t).L2C
	if len(fx) != 6 || len(items) != 6 {
		t.Fatalf("L2C items %d (fixture %d), want 6", len(items), len(fx))
	}
	for _, r := range fx {
		i, ok := bySO[r.SO]
		if !ok {
			t.Fatalf("SO %s missing", r.SO)
		}
		it := items[i]
		if it.Stage != domain.L2CStages[r.Stage-1] {
			t.Errorf("%s: stage %s, want %s", r.Acc, it.Stage, domain.L2CStages[r.Stage-1])
		}
		if math.Abs(float64(it.Days-r.Days)) > 1 || it.Bench != r.Bench {
			t.Errorf("%s: %d/%d days, want %d/%d", r.Acc, it.Days, it.Bench, r.Days, r.Bench)
		}
	}
	if n := count(t, a, `SELECT count(*) FROM cash_items WHERE source_system='odoo' AND source_id IS NOT NULL`); n != 6 {
		t.Fatalf("%d cash items mapped from Odoo sale orders, want 6", n)
	}
}

// stringLiterals returns every string literal of a Go file with the enclosing binary
// comparison (if any), so guards like `k == "stage_id"` can be told apart from uses.
func stringLiterals(t *testing.T, path string) map[string][]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]bool{} // literal → for each occurrence: is it inside an == / != comparison?
	var visit func(n ast.Node, inCmp bool)
	visit = func(n ast.Node, inCmp bool) {
		ast.Inspect(n, func(x ast.Node) bool {
			switch v := x.(type) {
			case *ast.BinaryExpr:
				if v.Op == token.EQL || v.Op == token.NEQ {
					visit(v.X, true)
					visit(v.Y, true)
					return false
				}
			case *ast.BasicLit:
				if v.Kind == token.STRING {
					s, _ := strconv.Unquote(v.Value)
					out[s] = append(out[s], inCmp)
				}
			}
			return true
		})
	}
	visit(file, false)
	return out
}

// Stage 08/10: grep tests — the read client never calls create/write/unlink; the writer
// never writes stage_id and has no unlink.
func TestStage08ConnectorSourceGuards(t *testing.T) {
	dir := filepath.Join(config.RepoRoot(), "packages", "connectors", "odoo")
	for _, f := range []string{"client.go", "xmlrpc.go"} {
		lits := stringLiterals(t, filepath.Join(dir, f))
		for _, banned := range []string{"create", "write", "unlink", "action_done", "message_post"} {
			if len(lits[banned]) > 0 {
				t.Errorf("%s contains the write method literal %q", f, banned)
			}
		}
	}
	w := stringLiterals(t, filepath.Join(dir, "writer.go"))
	if len(w["unlink"]) > 0 {
		t.Error("writer.go mentions unlink")
	}
	for _, inCmp := range w["stage_id"] {
		if !inCmp {
			t.Error(`writer.go uses "stage_id" outside the rejecting guard`)
		}
	}
	// Behavioural check of the same guard.
	if err := odoo.Validate(odoo.WriteOp{Model: "crm.lead", Method: "write", ID: 1, Values: map[string]any{"stage_id": 4}}); err == nil {
		t.Error("writer accepted stage_id")
	}
	if err := odoo.Validate(odoo.WriteOp{Model: "crm.lead", Method: "unlink", ID: 1}); err == nil {
		t.Error("writer accepted unlink")
	}
	if err := odoo.Validate(odoo.WriteOp{Model: "sale.order", Method: "write", ID: 1, Values: map[string]any{"state": "cancel"}}); err == nil {
		t.Error("writer accepted a non-whitelisted model")
	}
	if err := odoo.Validate(odoo.WriteOp{Model: "crm.lead", Method: "write", ID: 1, Values: map[string]any{"probability": 50}}); err != nil {
		t.Errorf("whitelisted probability write refused: %v", err)
	}
}
