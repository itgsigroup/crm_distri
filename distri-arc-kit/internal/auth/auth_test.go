package auth_test

import (
	"errors"
	"testing"
	"time"

	"distri-arc/internal/auth"
)

func TestHashAndToken(t *testing.T) {
	h, err := auth.Hash("kata-sandi-panjang")
	if err != nil || !auth.Check("kata-sandi-panjang", h) || auth.Check("salah", h) {
		t.Fatal("hash")
	}
	now := time.Now()
	secret := []byte("0123456789abcdef0123456789abcdef")
	tok := auth.Sign(auth.Claims{Sub: "u1", Email: "a@b", Role: "sales", Exp: now.Add(time.Hour).Unix()}, secret)
	if c, err := auth.Verify(tok, secret, now); err != nil || c.Email != "a@b" {
		t.Fatalf("verify %v", err)
	}
	if _, err := auth.Verify(tok, []byte("other-secret-other-secret-123456"), now); !errors.Is(err, auth.ErrToken) {
		t.Fatal("foreign secret accepted")
	}
	if _, err := auth.Verify(tok, secret, now.Add(2*time.Hour)); !errors.Is(err, auth.ErrExpired) {
		t.Fatal("expired accepted")
	}
	if _, err := auth.Verify(tok[:len(tok)-2]+"xx", secret, now); !errors.Is(err, auth.ErrToken) {
		t.Fatal("tampered accepted")
	}
}
