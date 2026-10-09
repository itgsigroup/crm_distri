package api_test

import (
	"strings"
	"testing"
)

// One sales' page (?sales=) on Pusat kendali and Orchestrator shows only that sales' dealers; a sales user always
// sees their own, whatever ?sales= says.
func TestSalesPageNeverMixesSales(t *testing.T) {
	srv := newServer(t)
	owners := func(path, user string) []string {
		t.Helper()
		code, out := get(t, srv, path, user)
		if code != 200 {
			t.Fatalf("%s: %d %v", path, code, out)
		}
		var names []string
		for _, x := range out["items"].([]any) {
			m := x.(map[string]any)
			if o, ok := m["owner"].(map[string]any); ok {
				names = append(names, o["name"].(string))
			} else if o, ok := m["sales"].(map[string]any); ok {
				names = append(names, o["name"].(string))
			}
		}
		return names
	}
	only := func(names []string, want string) {
		t.Helper()
		if len(names) == 0 {
			t.Fatalf("no rows for %s", want)
		}
		for _, n := range names {
			if !strings.EqualFold(n, want) {
				t.Fatalf("%s page shows %s: %v", want, n, names)
			}
		}
	}
	all := owners("/api/dealers/due?days=60", "sam@gsi.co.id")
	only(owners("/api/dealers/due?days=60&sales=andi", "sam@gsi.co.id"), "Andi")
	only(owners("/api/agenda?sales=andi", "sam@gsi.co.id"), "Andi")
	only(owners("/api/dealers/due?days=60&sales=dewi", "andi@gsi.co.id"), "Andi")
	if len(owners("/api/dealers/due?days=60&sales=andi", "sam@gsi.co.id")) >= len(all) {
		t.Fatalf("scoped list is not smaller than everyone's: %v", all)
	}
}
