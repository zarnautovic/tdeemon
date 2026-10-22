package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
)

// TokenPrefix starts every API token, so a leaked one is easy to
// recognize, for a person and for a secret scanner.
const TokenPrefix = "tdm_"

// NewAPIToken returns a random API token (256 bits after TokenPrefix) and
// the hash the database keeps for it, the same SessionID uses.
func NewAPIToken() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails.
	token = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return token, SessionID(token)
}

// Scope is something an API token may do.
type Scope string

// Scopes.
const (
	// ScopeIngest sends health data from the phone (Health Auto Export).
	ScopeIngest Scope = "ingest"
	// ScopeTools runs the tools (MCP): those that read, and those that
	// log and change data.
	ScopeTools Scope = "tools"
	// ScopeRead reads data and changes nothing.
	ScopeRead Scope = "read"
)

// Scopes lists every scope.
func Scopes() []Scope { return []Scope{ScopeIngest, ScopeTools, ScopeRead} }

// ParseScopes reads scopes from s, separated by spaces, and returns them
// sorted and without repeats. Unknown or no scopes are an error.
func ParseScopes(s string) ([]Scope, error) {
	var out []Scope
	for f := range strings.FieldsSeq(s) {
		sc := Scope(f)
		if !slices.Contains(Scopes(), sc) {
			return nil, fmt.Errorf("unknown scope %q; want ingest, tools or read", f)
		}
		out = append(out, sc)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("a token needs at least one scope: ingest, tools or read")
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// FormatScopes writes scopes as the database keeps them: sorted, without
// repeats, separated by spaces.
func FormatScopes(scopes []Scope) string {
	s := slices.Clone(scopes)
	slices.Sort(s)
	s = slices.Compact(s)
	parts := make([]string, len(s))
	for i, sc := range s {
		parts[i] = string(sc)
	}
	return strings.Join(parts, " ")
}
