package api_test

import (
	"testing"
)

const ceo = "sam@gsi.co.id"

// Stage 01 acceptance: /api/orbit, /api/segmen/summary and /api/dealers/{mitra}.
func TestOrbitStatusesMatchMockup(t *testing.T) {
	srv := newServer(t)
	code, body := get(t, srv, "/api/orbit", ceo)
	if code != 200 {
		t.Fatalf("orbit %d", code)
	}
	list := body["items"].([]any)
	if len(list) != 18 {
		t.Fatalf("orbit dealers %d", len(list))
	}
	ring := map[string]int{}
	for _, it := range list {
		st := it.(map[string]any)["metrics"].(map[string]any)["status"].(string)
		if st == "Baru" {
			st = "Aktif"
		}
		ring[st]++
	}
	if ring["Key account"] != 8 || ring["Aktif"] != 6 || ring["At risk"] != 3 || ring["Churn"] != 1 {
		t.Fatalf("rings %v", ring)
	}
}

func TestSegmenSummary(t *testing.T) {
	srv := newServer(t)
	_, body := get(t, srv, "/api/segmen/summary", ceo)
	want := map[string]float64{"A": 7, "B": 3, "C": 4, "D": 3, "Baru": 1}
	for _, it := range body["items"].([]any) {
		row := it.(map[string]any)
		if row["count"].(float64) != want[row["segment"].(string)] {
			t.Errorf("segment %v count %v", row["segment"], row["count"])
		}
	}
}

func TestDealerMitra(t *testing.T) {
	srv := newServer(t)
	code, body := get(t, srv, "/api/dealers/mitra", ceo)
	if code != 200 {
		t.Fatalf("dealer %d", code)
	}
	m := body["metrics"].(map[string]any)
	cr := m["credit"].(map[string]any)
	if m["status"] != "At risk" || m["segment"] != "C" || cr["state"] != "over limit" || m["score"].(float64) != 44 {
		t.Fatalf("mitra metrics %v", m)
	}
	parts := m["score_parts"].(map[string]any)
	for k, v := range map[string]float64{"r": 49, "p": 40, "k": 50, "n": 0, "i": 80} {
		if parts[k].(float64) != v {
			t.Errorf("part %s = %v want %v", k, parts[k], v)
		}
	}
	if len(body["contacts"].([]any)) != 2 || len(body["timeline"].([]any)) == 0 {
		t.Errorf("contacts/timeline missing")
	}
	if code, _ := get(t, srv, "/api/dealers/tidak-ada", ceo); code != 404 {
		t.Errorf("unknown dealer %d", code)
	}
}

func TestPusatKendaliLists(t *testing.T) {
	srv := newServer(t)
	for path, want := range map[string]int{"/api/dealers/due?days=7": 6, "/api/dealers/drift": 4, "/api/dealers/credit-tight": 4} {
		_, body := get(t, srv, path, ceo)
		if n := len(body["items"].([]any)); n != want {
			t.Errorf("%s: %d items want %d", path, n, want)
		}
	}
	_, k := get(t, srv, "/api/kpi", ceo)
	if k["stock_turn_days"].(float64) != 46 || k["overdue_amount"].(float64) != 141e6 {
		t.Errorf("kpi %v", k)
	}
	_, s := get(t, srv, "/api/dealers?q=mitra", ceo)
	if items := s["items"].([]any); len(items) == 0 || items[0].(map[string]any)["id"] != "mitra" {
		t.Errorf("search: %v", s)
	}
}
