package api_test

import (
	"net/http"
	"testing"
)

// Push stok (Sam 19 Oct): the page says where its stock comes from and how often it is read; "Sinkron sekarang" is
// for those who hold the Push stok page.
func TestStockStatusAndSync(t *testing.T) {
	srv, _ := chatServer(t)
	code, out := get(t, srv, "/api/stock/status", "sam@gsi.co.id")
	if code != http.StatusOK || out["every_min"] != float64(5) || out["label"] == nil || out["can_sync"] != true {
		t.Fatalf("status %d %v", code, out)
	}
	if code, _ := post(t, srv.URL+"/api/stock/sync", "finance@gsi.co.id", map[string]any{}); code != http.StatusForbidden {
		t.Fatalf("finance (no Push stok) synced: %d", code)
	}
}
