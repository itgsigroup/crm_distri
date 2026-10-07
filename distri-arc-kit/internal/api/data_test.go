package api_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func send(t *testing.T, method, url, user, ctype string, body []byte) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("X-Dev-User", user)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Data & master: sales cannot see it; admin imports and maps; only the CEO sets the source and its key.
func TestDataMasterAccess(t *testing.T) {
	srv, _ := chatServer(t)
	if code, _ := get(t, srv, "/api/data/status", "andi@gsi.co.id"); code != http.StatusForbidden {
		t.Fatalf("sales sees data status: %d", code)
	}
	code, st := get(t, srv, "/api/data/status", "admin@gsi.co.id")
	if code != 200 || st["contract"] == nil || len(st["categories"].([]any)) != 6 {
		t.Fatalf("status %d %v", code, st)
	}
	if code, _ := send(t, "PUT", srv.URL+"/api/data/source", "admin@gsi.co.id", "application/json", []byte(`{"mode":"csv","bigquery":{"project_id":"","location":"","sync_minutes":60,"queries":{}}}`)); code != http.StatusForbidden {
		t.Fatalf("admin sets source: %d", code)
	}
	if code, body := send(t, "PUT", srv.URL+"/api/data/source", "sam@gsi.co.id", "application/json", []byte(`{"mode":"bigquery","bigquery":{"project_id":"gsi","location":"","sync_minutes":5,"queries":{}}}`)); code != http.StatusBadRequest {
		t.Fatalf("sync every 5 min accepted: %d %s", code, body)
	}
	if code, _ := send(t, "PUT", srv.URL+"/api/data/credentials", "sam@gsi.co.id", "application/json", []byte(`{"type":"authorized_user"}`)); code != http.StatusBadRequest {
		t.Fatalf("non service-account key accepted: %d", code)
	}
	csv := "code,name,customer_type,branch\nX1,Toko Uji,reseller,Kantor Pusat\nX2,,si,\n"
	code, body := send(t, "POST", srv.URL+"/api/data/import/customers", "admin@gsi.co.id", "text/csv", []byte(csv))
	if code != 200 || !strings.Contains(body, `"staged":1`) || !strings.Contains(body, `"skipped":1`) {
		t.Fatalf("import %d %s", code, body)
	}
	if code, _ := send(t, "PUT", srv.URL+"/api/data/mappings", "admin@gsi.co.id", "application/json", []byte(`[{"kind":"category","source_value":"Hikvision NVR","target":"Kamera"}]`)); code != http.StatusBadRequest {
		t.Fatalf("category outside the 6 KAT accepted: %d", code)
	}
	if code, body := send(t, "PUT", srv.URL+"/api/data/mappings", "admin@gsi.co.id", "application/json", []byte(`[{"kind":"branch","source_value":"Kantor Pusat","target":"Semarang"}]`)); code != 200 {
		t.Fatalf("mapping %d %s", code, body)
	}
	if code, body := send(t, "PUT", srv.URL+"/api/data/mappings", "admin@gsi.co.id", "application/json", []byte(`[{"kind":"category","source_value":"IP Camera","target":""}]`)); code != 200 {
		t.Fatalf("clear mapping %d %s", code, body)
	}
	if code, body := send(t, "GET", srv.URL+"/api/data/mappings?kind=category", "admin@gsi.co.id", "", nil); code != 200 || !strings.Contains(body, `"source_value":"IP Camera","target":null`) || !strings.Contains(body, `"suggested":"Kamera \u0026 NVR"`) {
		t.Fatalf("mapping suggestion %d %s", code, body)
	}
	if code, body := send(t, "GET", srv.URL+"/api/data/schema", "admin@gsi.co.id", "", nil); code != http.StatusBadRequest || !strings.Contains(body, "no_credentials") {
		t.Fatalf("schema without key %d %s", code, body)
	}
	if code, body := send(t, "GET", srv.URL+"/api/data/template/invoices", "admin@gsi.co.id", "", nil); code != 200 || !strings.HasPrefix(body, "number,customer_code,date") {
		t.Fatalf("template %d %q", code, body)
	}
}
