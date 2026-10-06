package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"distri-arc/db"
	"distri-arc/internal/api"
	"distri-arc/internal/clock"
	"distri-arc/internal/config"
	"distri-arc/internal/seed"
	"distri-arc/internal/testdb"
)

// listKeys are JSON fields the web app iterates (.map/.filter): they must be [] when there is no data, never null.
var listKeys = map[string]bool{"items": true, "dealers": true, "points": true, "nodes": true, "edges": true, "pairs": true, "wa": true, "alerts": true,
	"models": true, "agents": true, "lessons": true, "runs": true, "staged": true, "mappings": true, "weeks": true, "audit": true, "problems": true}

func nullLists(v any, path string, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			if c == nil && listKeys[k] {
				*out = append(*out, path+"."+k)
			}
			nullLists(c, path+"."+k, out)
		}
	case []any:
		for _, c := range x {
			nullLists(c, path+"[]", out)
		}
	}
}

// Real-data installation before the first import: only policies and the CEO account. No screen may get a null list.
func TestEmptyInstallationHasNoNullLists(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Policies(ctx, st, db.Seed); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "insert into sales_users (name, branch, role, email) values ('Sam Setiadi', 'Semua cabang', 'ceo', 'ceo@x.id'), ('Budi', 'Semarang', 'sales', 'budi@x.id')"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "insert into users (email, name, role, sales_user_id, active) select email, name, role, id, true from sales_users"); err != nil {
		t.Fatal(err)
	}
	h := api.New(config.Config{Env: "dev", OdooMode: "off"}, st, clock.Fixed(time.Date(2026, 10, 6, 9, 0, 0, 0, clock.WIB)), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()
	for _, p := range []string{"me", "health", "dealers", "orbit", "orbit/summary", "orbit/movers", "segmen", "segmen/summary", "segmen/movers", "kpi", "agenda", "brief/today",
		"stock/aging", "stock/critical", "stock/push", "credit/overview", "credit/dealers", "credit/exposure", "credit/forecast", "relasi", "relasi/insights", "chat/threads",
		"wa/status", "proposals", "cycles/latest", "calibration", "pilot", "sales", "dealers/due", "data/status", "data/mappings", "data/customers", "data/team", "plan/today"} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/"+p, nil)
		req.Header.Set("X-Dev-User", "ceo@x.id")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode == http.StatusNotFound && strings.Contains(string(b), "404 page not found") {
			continue // route not in this build
		}
		if res.StatusCode != 200 {
			t.Errorf("%s: %d %s", p, res.StatusCode, b)
			continue
		}
		var v any
		_ = json.Unmarshal(b, &v)
		var nulls []string
		nullLists(v, p, &nulls)
		if len(nulls) > 0 {
			t.Errorf("null lists on an empty installation: %v", nulls)
		}
	}
}
