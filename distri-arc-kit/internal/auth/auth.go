// Package auth holds the login primitives (09-policies-security › Keamanan): argon2id password hashes, signed
// HttpOnly session tokens (JWT HS256, standard library only) and a login rate limiter.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters (OWASP minimum for interactive logins).
const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
)

// Hash returns an argon2id hash with a random salt: "argon2id$<salt>$<key>".
func Hash(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return HashWithSalt(secret, salt), nil
}

// HashWithSalt is Hash with a given salt (MCP tokens).
func HashWithSalt(secret string, salt []byte) string {
	k := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k))
}

// Check compares a secret with a stored hash in constant time.
func Check(secret, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(HashWithSalt(secret, salt)), []byte(stored)) == 1
}

// Claims are the session token contents.
type Claims struct {
	Sub   string `json:"sub"` // users.id
	Email string `json:"email"`
	Role  string `json:"role"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
}

// Errors of Verify.
var (
	ErrToken   = errors.New("sesi tidak valid")
	ErrExpired = errors.New("sesi berakhir")
)

var b64 = base64.RawURLEncoding

func mac(secret []byte, msg string) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(msg))
	return b64.EncodeToString(h.Sum(nil))
}

// Sign issues a session token.
func Sign(c Claims, secret []byte) string {
	head := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, _ := json.Marshal(c)
	msg := head + "." + b64.EncodeToString(body)
	return msg + "." + mac(secret, msg)
}

// Verify checks the signature and expiry of a token.
func Verify(token string, secret []byte, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrToken
	}
	if !hmac.Equal([]byte(mac(secret, parts[0]+"."+parts[1])), []byte(parts[2])) {
		return Claims{}, ErrToken
	}
	var head struct {
		Alg string `json:"alg"`
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &head) != nil || head.Alg != "HS256" {
		return Claims{}, ErrToken
	}
	bb, err := b64.DecodeString(parts[1])
	var c Claims
	if err != nil || json.Unmarshal(bb, &c) != nil || c.Sub == "" {
		return Claims{}, ErrToken
	}
	if now.Unix() >= c.Exp {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// Limiter counts failed logins per key (IP + email) in a sliding window.
type Limiter struct {
	Max    int
	Window time.Duration
	mu     sync.Mutex
	fails  map[string][]time.Time
}

// NewLimiter allows max failures per window.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, fails: map[string][]time.Time{}}
}

func (l *Limiter) recent(key string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range l.fails[key] {
		if now.Sub(t) < l.Window {
			keep = append(keep, t)
		}
	}
	l.fails[key] = keep
	return keep
}

// Blocked reports whether the key used up its attempts.
func (l *Limiter) Blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, now)) >= l.Max
}

// Fail records a failed attempt; Reset clears the key after a success.
func (l *Limiter) Fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key] = append(l.recent(key, now), now)
}

// Reset clears a key.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}

// Roles of Distri ARC users.
var Roles = []string{"ceo", "admin", "finance", "sales", "warehouse"}
