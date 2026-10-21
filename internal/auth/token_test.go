package auth

import (
	"regexp"
	"slices"
	"testing"
)

func TestNewAPIToken(t *testing.T) {
	t.Parallel()

	token, hash := NewAPIToken()
	if !regexp.MustCompile(`^tdm_[A-Za-z0-9_-]{43}$`).MatchString(token) {
		t.Errorf("token %q is not tdm_ and 32 bytes of URL-safe base64", token)
	}
	if hash != SessionID(token) || len(hash) != 64 {
		t.Errorf("hash %q is not the token's hex SHA-256", hash)
	}
	if other, _ := NewAPIToken(); other == token {
		t.Error("two tokens are equal")
	}
}

func TestParseScopes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, in string
		want     []Scope
		ok       bool
	}{
		{"one", "ingest", []Scope{ScopeIngest}, true},
		{"sorted and without repeats", " tools read  tools ", []Scope{ScopeRead, ScopeTools}, true},
		{"all", "read ingest tools", []Scope{ScopeIngest, ScopeRead, ScopeTools}, true},
		{"none", "  ", nil, false},
		{"unknown", "ingest admin", nil, false},
		{"case matters", "Read", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseScopes(tc.in)
			if (err == nil) != tc.ok || !slices.Equal(got, tc.want) {
				t.Errorf("ParseScopes(%q) = %v, %v; want %v (ok %v)", tc.in, got, err, tc.want, tc.ok)
			}
		})
	}
}

func TestFormatScopes(t *testing.T) {
	t.Parallel()

	in := []Scope{ScopeTools, ScopeIngest, ScopeTools}
	if got := FormatScopes(in); got != "ingest tools" {
		t.Errorf("FormatScopes = %q, want %q", got, "ingest tools")
	}
	if !slices.Equal(in, []Scope{ScopeTools, ScopeIngest, ScopeTools}) {
		t.Errorf("FormatScopes changed its argument to %v", in)
	}
	if back, err := ParseScopes(FormatScopes(Scopes())); err != nil || !slices.Equal(back, []Scope{ScopeIngest, ScopeRead, ScopeTools}) {
		t.Errorf("a round trip of every scope = %v, %v", back, err)
	}
}

func TestPrincipalCan(t *testing.T) {
	t.Parallel()

	session := Principal{Via: ViaSession}
	token := Principal{Via: ViaToken, Scopes: []Scope{ScopeIngest}}
	for _, tc := range []struct {
		name  string
		p     Principal
		scope Scope
		want  bool
	}{
		{"a session may do anything", session, ScopeTools, true},
		{"a token within its scopes", token, ScopeIngest, true},
		{"a token outside its scopes", token, ScopeTools, false},
		{"nobody", Principal{Scopes: []Scope{ScopeIngest}}, ScopeIngest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.Can(tc.scope); got != tc.want {
				t.Errorf("Can(%s) = %v, want %v", tc.scope, got, tc.want)
			}
		})
	}
}
