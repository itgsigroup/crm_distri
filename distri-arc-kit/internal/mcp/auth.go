package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"distri-arc/internal/auth"
	"distri-arc/internal/store"
	"distri-arc/internal/store/gen"
)

// Scopes a client can hold (06-mcp). "decide" does not exist: decisions are human-only.
const (
	ScopeRead        = "read"
	ScopeAnalyze     = "analyze"
	ScopeOrchestrate = "orchestrate"
)

// ValidScopes are the scopes a token may carry.
var ValidScopes = []string{ScopeRead, ScopeAnalyze, ScopeOrchestrate}

// Client is an authenticated MCP client.
type Client struct {
	ID     uuid.UUID
	Name   string
	Scopes []string
}

// Has reports whether the client holds a scope.
func (c *Client) Has(scope string) bool { return c != nil && slices.Contains(c.Scopes, scope) }

func hashSecret(secret string, salt []byte) string { return auth.HashWithSalt(secret, salt) }

func checkSecret(secret, stored string) bool { return auth.Check(secret, stored) }

// CreateToken registers a client and returns its token — shown once, stored only as an argon2id hash.
// Format: arc_<client id hex>_<secret>.
func CreateToken(ctx context.Context, q *gen.Queries, name string, scopes []string, owner *uuid.UUID) (string, gen.McpClient, error) {
	for _, s := range scopes {
		if !slices.Contains(ValidScopes, s) {
			return "", gen.McpClient{}, fmt.Errorf("unknown scope %q (read, analyze, orchestrate; decide is human-only)", s)
		}
	}
	if strings.TrimSpace(name) == "" || len(scopes) == 0 {
		return "", gen.McpClient{}, errors.New("name and at least one scope are required")
	}
	raw := make([]byte, 32)
	salt := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", gen.McpClient{}, err
	}
	if _, err := rand.Read(salt); err != nil {
		return "", gen.McpClient{}, err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	id := uuid.New()
	kind := "bearer"
	token := "arc_" + hex.EncodeToString(id[:]) + "_" + secret
	prefix := token[:12] + "…"
	hash := hashSecret(secret, salt)
	c, err := q.InsertMCPClient(ctx, gen.InsertMCPClientParams{ID: id, Name: &name, Kind: &kind, TokenHash: &hash, Scopes: scopes, OwnerID: owner, TokenPrefix: &prefix})
	if err != nil {
		return "", c, err
	}
	return token, c, nil
}

// ErrInvalidToken means the bearer token is unknown, malformed or revoked.
var ErrInvalidToken = errors.New("invalid token")

// Verifier checks bearer tokens against mcp_clients with a short in-memory cache.
type Verifier struct {
	st    *store.Store
	mu    sync.Mutex
	cache map[[32]byte]cached
	ttl   time.Duration
}

type cached struct {
	c  Client
	at time.Time
}

// NewVerifier builds a verifier.
func NewVerifier(st *store.Store) *Verifier {
	return &Verifier{st: st, cache: map[[32]byte]cached{}, ttl: time.Minute}
}

// Verify resolves a token to its client.
func (v *Verifier) Verify(ctx context.Context, token string) (*Client, error) {
	key := sha256.Sum256([]byte(token))
	v.mu.Lock()
	if c, ok := v.cache[key]; ok && time.Since(c.at) < v.ttl {
		v.mu.Unlock()
		cl := c.c
		return &cl, nil
	}
	v.mu.Unlock()
	rest, ok := strings.CutPrefix(token, "arc_")
	if !ok {
		return nil, ErrInvalidToken
	}
	idHex, secret, ok := strings.Cut(rest, "_")
	b, err := hex.DecodeString(idHex)
	if !ok || err != nil || len(b) != 16 {
		return nil, ErrInvalidToken
	}
	id, _ := uuid.FromBytes(b)
	row, err := v.st.Q.GetMCPClient(ctx, id)
	if err != nil || !row.Active || row.TokenHash == nil || !checkSecret(secret, *row.TokenHash) {
		return nil, ErrInvalidToken
	}
	c := Client{ID: row.ID, Name: deref(row.Name), Scopes: row.Scopes}
	v.mu.Lock()
	v.cache[key] = cached{c: c, at: time.Now()}
	v.mu.Unlock()
	return &c, nil
}

// Forget drops cached verifications (after a revoke).
func (v *Verifier) Forget() {
	v.mu.Lock()
	v.cache = map[[32]byte]cached{}
	v.mu.Unlock()
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}
