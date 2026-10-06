package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP is defined over HMAC-SHA1; every authenticator app expects it
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238): 30-second steps, 6 digits, HMAC-SHA1 — what Google Authenticator, Authy and 1Password use.
const (
	totpStep   = 30
	totpDigits = 6
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit secret in base32.
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// TOTPStep is the time step of t.
func TOTPStep(t time.Time) int64 { return t.Unix() / totpStep }

// TOTPCode is the code of one step.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%1_000_000), nil
}

// VerifyTOTP accepts the code of the current step or one step around it (clock drift) and returns the matched step,
// so the caller can refuse a step that was already used (replay).
func VerifyTOTP(secret, code string, t time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	now := TOTPStep(t)
	for _, s := range []int64{now, now - 1, now + 1} {
		c, err := TOTPCode(secret, s)
		if err == nil && hmac.Equal([]byte(c), []byte(code)) {
			return s, true
		}
	}
	return 0, false
}

// TOTPURI is the otpauth:// link an authenticator app reads (QR or manual entry).
func TOTPURI(issuer, account, secret string) string {
	v := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + v.Encode()
}

func sealKey(secret []byte) []byte {
	k := sha256.Sum256(append([]byte("distri-arc totp v1:"), secret...))
	return k[:]
}

// Seal encrypts a TOTP secret for the database (AES-256-GCM, key derived from SESSION_SECRET).
func Seal(plain string, sessionSecret []byte) (string, error) {
	block, err := aes.NewCipher(sealKey(sessionSecret))
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(plain), nil)), nil
}

// ErrSealed means a sealed value cannot be opened (wrong SESSION_SECRET or tampered).
var ErrSealed = errors.New("sealed value cannot be opened")

// Open decrypts a value sealed by Seal.
func Open(sealed string, sessionSecret []byte) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(sealed)
	if err != nil {
		return "", ErrSealed
	}
	block, err := aes.NewCipher(sealKey(sessionSecret))
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < g.NonceSize() {
		return "", ErrSealed
	}
	p, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	if err != nil {
		return "", ErrSealed
	}
	return string(p), nil
}
