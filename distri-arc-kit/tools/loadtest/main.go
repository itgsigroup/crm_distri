// Command loadtest is the stage 13 performance check (dev only): it fills the database up to 50k signals and
// 20k chat messages spread over 24 months, prints EXPLAIN ANALYZE of the heaviest reads, then measures p50/p95 of
// the five heavy endpoints (orbit, segmen, relasi, dealer timeline, due) against a running api. Exit 1 when a p95
// is above the target (300 ms). tools/loadtest/k6.js is the same load for k6.
//
//	go run ./tools/loadtest [-signals 50000] [-chat 20000] [-n 200] [-c 8] [-url http://127.0.0.1:8080] [-cleanup]
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"distri-arc/internal/config"
)

var endpoints = []string{"/api/orbit", "/api/segmen", "/api/relasi", "/api/dealers/sinar", "/api/dealers/due"}

func main() {
	signals := flag.Int("signals", 50000, "total signals to reach")
	chat := flag.Int("chat", 20000, "total chat messages to reach")
	n := flag.Int("n", 200, "requests per endpoint")
	c := flag.Int("c", 8, "concurrent clients")
	url := flag.String("url", "http://127.0.0.1:8080", "api base url")
	user := flag.String("user", "sam@gsi.co.id", "X-Dev-User")
	target := flag.Duration("p95", 300*time.Millisecond, "p95 target")
	cleanup := flag.Bool("cleanup", false, "remove the generated rows and exit")
	flag.Parse()
	cfg := config.Load()
	if !cfg.IsDev() {
		fail("loadtest writes synthetic rows: APP_ENV=dev only")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fail(err.Error())
	}
	defer pool.Close()
	if *cleanup {
		for _, q := range []string{
			"delete from chat_message_keys where wa_msg_id like 'load:%'", "delete from chat_messages where wa_msg_id like 'load:%'",
			"delete from signal_keys where dedupe_key like 'load:%'", "delete from signals where dedupe_key like 'load:%'",
		} {
			must(pool.Exec(ctx, q))
		}
		fmt.Println("generated rows removed")
		return
	}
	fill(ctx, pool, *signals, *chat)
	explain(ctx, pool)
	ok := true
	fmt.Printf("\n%-22s %6s %8s %8s %8s\n", "endpoint", "n", "p50", "p95", "max")
	for _, e := range endpoints {
		d := hit(*url+e, *user, *n, *c)
		p50, p95, mx := d[len(d)/2], d[len(d)*95/100], d[len(d)-1]
		mark := ""
		if p95 > *target {
			mark, ok = "  > target", false
		}
		fmt.Printf("%-22s %6d %8s %8s %8s%s\n", e, len(d), ms(p50), ms(p95), ms(mx), mark)
	}
	if !ok {
		os.Exit(1)
	}
	fmt.Printf("\nOK: every p95 < %s at %d signals\n", *target, *signals)
}

// fill adds synthetic signals (wa/so/invoice/payment, a third with an agent conclusion so the Timeline has work) and
// chat messages until the totals are reached. Months up to 24 back get their partitions first.
func fill(ctx context.Context, pool *pgxpool.Pool, signals, chat int) {
	must(pool.Exec(ctx, "select arc_ensure_partitions('signals', 'occurred_at', (now() - interval '24 months')::date, now(), 3)"))
	must(pool.Exec(ctx, "select arc_ensure_partitions('chat_messages', 'sent_at', (now() - interval '24 months')::date, now(), 3)"))
	var have int
	_ = pool.QueryRow(ctx, "select count(*) from signals").Scan(&have)
	if add := signals - have; add > 0 {
		t := time.Now()
		must(pool.Exec(ctx, `
with d as (select array_agg(id) ids from dealers),
g as (
  select i, gen_random_uuid() id, now() - (random() * interval '730 days') ts,
         (array['wa','so','invoice','payment'])[1 + (i % 4)] kind, (select ids from d) ids
  from generate_series(1, $1::int) i
),
k as (insert into signal_keys (dedupe_key, signal_id, occurred_at) select 'load:' || i || ':' || id, id, ts from g)
insert into signals (id, kind, dealer_id, occurred_at, dedupe_key, summary, payload)
select id, kind, ids[1 + (i % array_length(ids, 1))], ts, 'load:' || i || ':' || id, 'Sinyal uji beban ' || i,
  case when i % 3 = 0 then jsonb_build_object('via', 'chat', 'who', 'Uji', 'text', 'Pesan uji ' || i, 'conclusion', 'Kesimpulan uji')
       else jsonb_build_object('via', 'chat', 'who', 'Uji', 'text', 'Pesan uji ' || i) end
from g`, add))
		fmt.Printf("signals: +%d in %s\n", add, time.Since(t).Round(time.Millisecond))
	}
	_ = pool.QueryRow(ctx, "select count(*) from chat_messages").Scan(&have)
	if add := chat - have; add > 0 {
		t := time.Now()
		must(pool.Exec(ctx, `
with th as (select array_agg(id) ids from chat_threads),
g as (select i, gen_random_uuid() id, now() - (random() * interval '85 days') ts, (select ids from th) ids from generate_series(1, $1::int) i),
k as (insert into chat_message_keys (wa_msg_id, message_id, sent_at) select 'load:' || i || ':' || id, id, ts from g)
insert into chat_messages (id, thread_id, wa_msg_id, direction, from_number, from_name, body, sent_at, status)
select id, ids[1 + (i % array_length(ids, 1))], 'load:' || i || ':' || id, case when i % 2 = 0 then 'in' else 'out' end,
  '62819' || lpad((i % 1000)::text, 8, '0'), 'Uji', 'Pesan uji beban ' || i, ts, 'received'
from g`, add))
		fmt.Printf("chat_messages: +%d in %s\n", add, time.Since(t).Round(time.Millisecond))
	}
	must(pool.Exec(ctx, "analyze signals; analyze chat_messages; analyze signal_keys; analyze chat_message_keys"))
}

// explain prints the plans of the reads behind the heavy endpoints (docs/PERF.md).
func explain(ctx context.Context, pool *pgxpool.Pool) {
	var sinar string
	_ = pool.QueryRow(ctx, "select id::text from dealers where slug = 'sinar'").Scan(&sinar)
	queries := map[string]string{
		"timeline (dealer)": `select x.at from (
  select s.occurred_at as at from signals s left join proposals rp on rp.id = nullif(s.payload->>'reply_to', '')::uuid
  where s.dealer_id = '` + sinar + `' and (s.payload ? 'conclusion' or s.payload ? 'reply_to')
  union all select p.decided_at from proposals p where p.dealer_id = '` + sinar + `' and p.decided_at is not null) x order by x.at desc limit 15`,
		"memo signals (dealer)": `select summary from signals where dealer_id = '` + sinar + `' and summary is not null order by occurred_at desc limit 40`,
		"relasi monthly (chat)": `select date_trunc('month', m.sent_at), count(*) from chat_messages m join chat_threads t on t.id = m.thread_id
  where m.sent_at >= now() - interval '90 days' group by 1`,
		"signals since (ingest)": `select count(*) from signals where occurred_at >= now() - interval '1 hour'`,
		"signal by id":           `select * from signals where id = (select signal_id from signal_keys limit 1)`,
	}
	keys := make([]string, 0, len(queries))
	for k := range queries {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		rows, err := pool.Query(ctx, "explain (analyze, buffers, costs off, summary on) "+queries[k])
		if err != nil {
			fail(k + ": " + err.Error())
		}
		var lines []string
		for rows.Next() {
			var l string
			_ = rows.Scan(&l)
			lines = append(lines, l)
		}
		rows.Close()
		fmt.Printf("\n== %s\n", k)
		for _, l := range lines {
			if strings.Contains(l, "Scan") || strings.Contains(l, "Execution Time") || strings.Contains(l, "Subplans Removed") {
				fmt.Println("  " + strings.TrimSpace(l))
			}
		}
	}
}

func hit(url, user string, n, c int) []time.Duration {
	var mu sync.Mutex
	var out []time.Duration
	jobs := make(chan struct{}, n)
	for range n {
		jobs <- struct{}{}
	}
	close(jobs)
	client := &http.Client{Timeout: 10 * time.Second}
	var wg sync.WaitGroup
	for range c {
		wg.Go(func() {
			for range jobs {
				req, _ := http.NewRequest(http.MethodGet, url, nil)
				req.Header.Set("X-Dev-User", user)
				t := time.Now()
				res, err := client.Do(req)
				d := time.Since(t)
				if err != nil || res.StatusCode != 200 {
					fail(fmt.Sprintf("%s: %v %v", url, err, res))
				}
				res.Body.Close()
				mu.Lock()
				out = append(out, d)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	slices.Sort(out)
	return out
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }

func must[T any](_ T, err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "loadtest:", msg)
	os.Exit(1)
}
