package importer

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testKey(t *testing.T, tokenURI string) []byte {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	p := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	j, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": "gsi-data", "client_email": "distri-arc@gsi-data.iam.gserviceaccount.com", "private_key": p, "token_uri": tokenURI})
	return j
}

// Token exchange, an unfinished job, and two result pages.
func TestBigQueryQuery(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || strings.Count(r.Form.Get("assertion"), ".") != 2 {
				w.WriteHeader(400)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600}`))
		case r.Header.Get("Authorization") != "Bearer tok-1":
			w.WriteHeader(401)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/projects/gsi-data/queries"):
			_, _ = w.Write([]byte(`{"jobComplete":false,"jobReference":{"jobId":"job-1","location":"asia-southeast2"}}`))
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/queries/job-1"):
			calls++
			if r.URL.Query().Get("location") != "asia-southeast2" {
				w.WriteHeader(400)
				return
			}
			if r.URL.Query().Get("pageToken") == "" {
				_, _ = w.Write([]byte(`{"jobComplete":true,"schema":{"fields":[{"name":"CODE","type":"STRING"},{"name":"total","type":"NUMERIC"}]},"rows":[{"f":[{"v":"C001"},{"v":"12000000"}]}],"pageToken":"p2"}`))
				return
			}
			_, _ = w.Write([]byte(`{"jobComplete":true,"schema":{"fields":[{"name":"CODE","type":"STRING"},{"name":"total","type":"NUMERIC"}]},"rows":[{"f":[{"v":"C002"},{"v":null}]}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	sa, err := ParseServiceAccount(testKey(t, srv.URL+"/token"))
	if err != nil {
		t.Fatal(err)
	}
	bq := &BigQuery{SA: sa, BaseURL: srv.URL}
	rows, err := bq.Query(context.Background(), "select 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["code"] != "C001" || rows[0]["total"] != "12000000" || rows[1]["total"] != "" || calls != 2 {
		t.Fatalf("rows %v calls %d", rows, calls)
	}
	if _, err := ParseServiceAccount([]byte(`{"type":"authorized_user"}`)); err == nil {
		t.Fatal("non service-account key accepted")
	}
}
