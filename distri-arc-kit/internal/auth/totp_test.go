package auth

import (
	"testing"
	"time"
)

// RFC 6238 appendix B (SHA1, secret "12345678901234567890"), last 6 digits.
func TestTOTPVectors(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	for _, c := range []struct {
		unix int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		got, err := TOTPCode(secret, TOTPStep(time.Unix(c.unix, 0)))
		if err != nil || got != c.want {
			t.Errorf("T=%d: %s, want %s (%v)", c.unix, got, c.want, err)
		}
	}
	at := time.Unix(1111111111, 0)
	if _, ok := VerifyTOTP(secret, "050471", at.Add(30*time.Second)); !ok {
		t.Error("one step of drift must pass")
	}
	if _, ok := VerifyTOTP(secret, "050471", at.Add(90*time.Second)); ok {
		t.Error("three steps late must fail")
	}
}

func TestSealOpen(t *testing.T) {
	s, err := Seal("GEZDGNBV", []byte("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if p, err := Open(s, []byte("k1")); err != nil || p != "GEZDGNBV" {
		t.Fatalf("open %q %v", p, err)
	}
	if _, err := Open(s, []byte("k2")); err == nil {
		t.Fatal("wrong key opened the secret")
	}
}
