package api_test

import (
	"net/http"
	"testing"
)

// Big boards stay light: JSON is gzip-compressed, aging stock carries only the first candidates and their count,
// the Pusat kendali push list is short.
func TestLightResponses(t *testing.T) {
	srv, _ := chatServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/orbit", nil)
	req.Header.Set("X-Dev-User", "sam@gsi.co.id")
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := (&http.Transport{DisableCompression: true}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("orbit not compressed: %q", res.Header.Get("Content-Encoding"))
	}
	_, aging := get(t, srv, "/api/stock/aging", "sam@gsi.co.id")
	for _, x := range aging["items"].([]any) {
		it := x.(map[string]any)
		c, _ := it["candidates"].([]any)
		if len(c) > 3 || int(it["candidate_count"].(float64)) < len(c) {
			t.Fatalf("aging %v: %d candidates, count %v", it["name"], len(c), it["candidate_count"])
		}
	}
	if _, push := get(t, srv, "/api/stock/push", "sam@gsi.co.id"); len(push["items"].([]any)) > 10 {
		t.Fatalf("push list %d", len(push["items"].([]any)))
	}
}
