package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

// Principal is who a request acts for.
type Principal struct {
	User user.User
	// SessionID is the session the request came with.
	SessionID string
	// SessionExpiresAt is when that session ends.
	SessionExpiresAt time.Time
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
