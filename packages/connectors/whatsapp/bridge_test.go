package whatsapp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The bridge's anti-ban refusals reach the user as a readable Indonesian message.
func TestBridgeRefusalIsReadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !Verify("s3cret", body, r.Header.Get("X-ARC-Signature")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Kontak belum pernah mengirim pesan ke nomor ini","code":"first_contact"}`))
	}))
	defer srv.Close()
	_, err := NewBridge(srv.URL, "s3cret").Send(context.Background(), "s-andi", "6281@s.whatsapp.net", "Halo", "act-1")
	var be *BridgeError
	if !errors.As(err, &be) || be.Code != "first_contact" || be.Status != http.StatusForbidden {
		t.Fatalf("want BridgeError first_contact, got %v", err)
	}
	if err.Error() != "WhatsApp: Kontak belum pernah mengirim pesan ke nomor ini" {
		t.Fatalf("message: %q", err.Error())
	}
}
