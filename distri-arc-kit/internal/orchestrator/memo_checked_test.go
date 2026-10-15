package orchestrator_test

import (
	"context"
	"testing"
)

// Belajar on a dealer with nothing to say yet (no signals): the dealer is marked checked and not read again until a
// new signal arrives — before, each cycle re-read every such dealer and logged "memo not stored" (17,000 a day).
func TestEmptyMemoIsCheckedOnce(t *testing.T) {
	st, o := setup(t)
	ctx := context.Background()
	if _, err := st.Pool.Exec(ctx, `insert into dealers (slug, name, tier, branch) select 'sepi-' || g, 'Toko Sepi ' || g, 'C', 'Semarang' from generate_series(1, 3) g`); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Run(ctx, all, sched); err != nil {
		t.Fatal(err)
	}
	quiet := func(sql string) int {
		var n int
		if err := st.Pool.QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := quiet(`select count(*) from dealers where slug like 'sepi-%' and memo_updated_at is not null and memo_sentences = '[]'::jsonb`); n != 3 {
		t.Fatalf("%d of 3 quiet dealers marked checked", n)
	}
	if n := quiet(`select count(*) from dealers d where slug like 'sepi-%' and exists (select 1 from signals s where s.dealer_id = d.id and s.occurred_at > d.memo_updated_at)`); n != 0 {
		t.Fatalf("%d quiet dealers would be read again", n)
	}
}
