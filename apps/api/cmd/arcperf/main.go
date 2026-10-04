// Command arcperf is the stage-13 load check: it seeds the fixtures into a
// scratch database, adds bulk data (default 500 accounts, 10,000 interactions),
// then measures the main read endpoints and fails when any p95 exceeds the budget.
//
//	go run ./apps/api/cmd/arcperf            # uses ARC_PERF_DATABASE_URL (default arc_perf)
//	go run ./apps/api/cmd/arcperf -n 50      # requests per endpoint
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"arc/apps/api/internal/app"
	"arc/apps/api/internal/seed"
	"arc/packages/core/config"
	"arc/packages/core/domain"
	"arc/packages/core/storage"
)

func main() {
	accounts := flag.Int("accounts", 500, "total accounts after bulk load")
	interactions := flag.Int("interactions", 10000, "bulk interactions to add")
	n := flag.Int("n", 30, "requests per endpoint")
	budget := flag.Duration("p95", 300*time.Millisecond, "p95 budget per endpoint")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	ctx := context.Background()
	cfg := config.Load()
	cfg.DatabaseURL = envOr("ARC_PERF_DATABASE_URL", "postgres://localhost:5433/arc_perf?sslmode=disable")
	cfg.Env, cfg.SchedulerEnabled = "test", false
	cfg.AnthropicKey, cfg.OpenAIKey, cfg.OllamaURL, cfg.OdooURL, cfg.GoogleClientID = "", "", "", "", ""
	domain.SetClockAnchor(time.Date(2026, 9, 28, 17, 2, 0, 0, domain.Jakarta))

	// The run wipes the database: refuse anything that is not clearly a scratch DB.
	if u, err := url.Parse(cfg.DatabaseURL); err != nil || !strings.Contains(path.Base(u.Path), "perf") {
		must(fmt.Errorf("refusing to wipe %q: database name must contain \"perf\"", cfg.DatabaseURL))
	}
	db, err := storage.Open(ctx, cfg.DatabaseURL)
	must(err)
	defer db.Close()
	must(db.ResetSchema(ctx))
	_, err = db.Migrate(ctx)
	must(err)
	f, err := seed.Load(seed.FixtureDir(config.RepoRoot()))
	must(err)
	must(seed.Run(ctx, db, f))
	a, err := app.New(ctx, cfg, db)
	must(err)
	must(a.PostSeed(ctx, f))
	must(bulk(ctx, db, *accounts, *interactions))
	_, _ = db.Pool.Exec(ctx, `ANALYZE`)

	var nAcc, nInt int
	_ = db.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accounts), (SELECT count(*) FROM interactions)`).Scan(&nAcc, &nInt)
	fmt.Printf("data: %d accounts, %d interactions\n", nAcc, nInt)

	srv := httptest.NewServer(a.Routes())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	body, _ := json.Marshal(map[string]string{"email": "sam@gsi.co.id", "password": "arc12345"})
	resp, err := cl.Post(srv.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	must(err)
	resp.Body.Close()

	endpoints := []string{"/api/today", "/api/shell", "/api/accounts", "/api/accounts/rsud", "/api/chat/threads", "/api/pipeline", "/api/forecast",
		"/api/prospects", "/api/cash", "/api/network?period=3", "/api/actions?status=proposed", "/api/people"}
	failed := false
	fmt.Printf("%-30s %8s %8s %8s\n", "endpoint", "p50", "p95", "max")
	for _, ep := range endpoints {
		var lat []time.Duration
		for i := 0; i < *n; i++ {
			t0 := time.Now()
			r, err := cl.Get(srv.URL + ep)
			if err != nil {
				fmt.Println(ep, "error:", err)
				failed = true
				break
			}
			_, _ = bytes.NewBuffer(nil).ReadFrom(r.Body)
			r.Body.Close()
			if r.StatusCode != 200 {
				fmt.Println(ep, "HTTP", r.StatusCode)
				failed = true
				break
			}
			lat = append(lat, time.Since(t0))
		}
		if len(lat) == 0 {
			continue
		}
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		p50, p95, mx := lat[len(lat)/2], lat[(len(lat)*95+99)/100-1], lat[len(lat)-1]
		mark := ""
		if p95 > *budget {
			mark, failed = "  ✗ over budget", true
		}
		fmt.Printf("%-30s %8s %8s %8s%s\n", ep, ms(p50), ms(p95), ms(mx), mark)
	}
	if failed {
		fmt.Printf("FAIL: p95 budget %s\n", *budget)
		os.Exit(1)
	}
	fmt.Printf("OK: all p95 < %s\n", *budget)
}

// bulk adds synthetic accounts (with one person and one customer thread each)
// and interactions spread over the last 180 days.
func bulk(ctx context.Context, db *storage.DB, accounts, interactions int) error {
	branches := []string{"Semarang", "Yogyakarta", "Surabaya", "Jakarta"}
	owners := []string{"andi", "dewi", "rizky", "fajar"}
	var have int
	_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM accounts`).Scan(&have)
	stmts := []string{
		fmt.Sprintf(`INSERT INTO accounts(id,name,sector,branch,owner_user_id,health,normal_rhythm_days)
			SELECT 'perf-'||g, 'PT Beban Uji '||g, (ARRAY['Swasta','Pemerintah','Kesehatan','Pendidikan'])[1+g%%4], (ARRAY['%s'])[1+g%%4], (ARRAY['%s'])[1+g%%4], 30+g%%70, 7
			FROM generate_series(1, %d) g`, strings.Join(branches, "','"), strings.Join(owners, "','"), max(accounts-have, 0)),
		`INSERT INTO people(id,name,role,account_id,phones,strength)
			SELECT 'perfp-'||substr(a.id,6), 'Kontak '||a.name, 'Pengadaan', a.id, ARRAY['+62811'||lpad(substr(a.id,6),7,'0')], 1 FROM accounts a WHERE a.id LIKE 'perf-%'`,
		`INSERT INTO chat_threads(id,session_id,chat_jid,type,name,account_id,person_id,last_at)
			SELECT 'perft-'||substr(a.id,6), (ARRAY['s-andi','s-dewi'])[1+(substr(a.id,6)::int)%2], '62811'||lpad(substr(a.id,6),7,'0')||'@s.whatsapp.net', 'cust', a.name, a.id, 'perfp-'||substr(a.id,6), now()
			FROM accounts a WHERE a.id LIKE 'perf-%'`,
		fmt.Sprintf(`INSERT INTO interactions(channel,direction,occurred_at,thread_id,body_text,raw_ref,wamid,account_id,sender_name,transport,extracted,extraction_version)
			SELECT 'wa_message', CASE WHEN g%%3=0 THEN 'out' ELSE 'in' END, now() - (g %% 180) * interval '1 day' - (g %% 600) * interval '1 minute',
				t.id, 'Pesan uji beban nomor '||g||' tentang penawaran CCTV dan jadwal survei.', 'perf:'||g, 'PERF'||g, t.account_id, 'Kontak', 'fake', true, 1
			FROM generate_series(1, %d) g JOIN LATERAL (SELECT id, account_id FROM chat_threads WHERE id = 'perft-'||(1 + g %% GREATEST(%d,1))) t ON true`, interactions, max(accounts-have, 1)),
	}
	for _, s := range stmts {
		if _, err := db.Pool.Exec(ctx, s); err != nil {
			return fmt.Errorf("bulk: %w", err)
		}
	}
	return nil
}

func ms(d time.Duration) string { return fmt.Sprintf("%.0fms", float64(d.Microseconds())/1000) }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "arcperf:", err)
		os.Exit(1)
	}
}
