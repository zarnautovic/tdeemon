package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"slices"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// SessionLifetime is how long a web session lasts from sign-in, however
// active it is. Then the user signs in again.
const SessionLifetime = 30 * 24 * time.Hour

// NewSessionToken returns a random session token for the cookie (256
// bits, URL-safe base64) and the ID the database keeps for it.
func NewSessionToken() (token, id string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails.
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, SessionID(token)
}

// SessionID returns the ID under which the database keeps token: its
// hex SHA-256, so the database never holds a token that would sign
// anyone in. A fast hash is enough: the token has 256 random bits.
func SessionID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Via is how a request proved who it acts for.
type Via int

// The ways a request can authenticate.
const (
	// ViaSession is a web session's cookie: a person in a browser.
	ViaSession Via = iota + 1
	// ViaToken is an API token: a device or a client acting for the user.
	ViaToken
)

// Principal is who a request acts for.
type Principal struct {
	User user.User
	Via  Via
	// SessionID is the session the request came with (ViaSession).
	SessionID string
	// SessionExpiresAt is when that session ends (ViaSession).
	SessionExpiresAt time.Time
	// TokenID is the API token's handle (ViaToken).
	TokenID string
	// Scopes are what the token may do (ViaToken).
	Scopes []Scope
	// Client is what a token's request came through when not the API
	// itself: ClientMCP for a tool an MCP client called.
	Client string
}

// ClientMCP is the Client of a request an MCP client made.
const ClientMCP = "mcp"

// Can reports whether p may act in scope: a session may do anything its
// user may, a token only what its scopes allow.
func (p Principal) Can(scope Scope) bool {
	return p.Via == ViaSession || (p.Via == ViaToken && slices.Contains(p.Scopes, scope))
}

type principalKey struct{}

// WithPrincipal returns a copy of ctx that carries p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal ctx carries, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
