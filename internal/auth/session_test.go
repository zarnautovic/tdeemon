package auth

import (
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// Hashes from the reference C implementation through argon2-cffi 25.1.0,
// with m=8, t=1, p=1 and the salt "0123456789abcdef": an old cost, so a
// sign-in should store a new hash.
const (
	oldCostHash      = "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE" // correct horse battery staple
	oldCostShortHash = "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$SDvn1ahgbEqoZxYWl4nLdCrT350l3ejghSpaRC0ZMXQ" // tooshort
)

func TestNewSessionToken(t *testing.T) {
	t.Parallel()

	token, id := NewSessionToken()
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(token) {
		t.Errorf("token %q is not 32 bytes of URL-safe base64", token)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(id) {
		t.Errorf("id %q is not a hex SHA-256", id)
	}
	if id != SessionID(token) {
		t.Error("the id is not SessionID(token)")
	}
	if id == token {
		t.Error("the id is the token itself")
	}
	if other, _ := NewSessionToken(); other == token {
		t.Error("two tokens are equal")
	}
	// A known value, so the hash cannot change unnoticed: a change would
	// sign every user out.
	if got := SessionID("token"); got != "3c469e9d6c5875d37a43f353d4f88e61fcf812c66eee3457465a40b0da4153e0" {
		t.Errorf("SessionID(token) = %s, want the SHA-256 of it", got)
	}
}

func TestPrincipalContext(t *testing.T) {
	t.Parallel()

	if _, ok := PrincipalFrom(t.Context()); ok {
		t.Error("a bare context carries a principal")
	}
	p := Principal{User: user.User{Name: "Ana"}, SessionID: "s"}
	got, ok := PrincipalFrom(WithPrincipal(t.Context(), p))
	if !ok || got.User.Name != "Ana" || got.SessionID != "s" {
		t.Errorf("PrincipalFrom = %+v, %v; want the principal put in", got, ok)
	}
}

func TestVerifyLogin(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// The most a password may be: 256 characters of four bytes.
	longest := strings.Repeat("😀", MaxPasswordLen)
	longestHash, err := HashPassword(longest)
	if err != nil {
		t.Fatalf("HashPassword(longest): %v", err)
	}
	// 120 Hangul syllables (한, 360 bytes), set in that form and typed as
	// three jamo each (1,080 bytes), which NFKC composes back.
	hangul := strings.Repeat("\ud55c", 120)
	jamo := strings.Repeat("\u1112\u1161\u11ab", 120)
	hangulHash, err := HashPassword(hangul)
	if err != nil {
		t.Fatalf("HashPassword(hangul): %v", err)
	}
	if ok, _ := VerifyPassword(jamo, hangulHash); !ok {
		t.Fatal("the jamo form does not verify without the length cap; the test proves nothing")
	}
	for _, tc := range []struct {
		name, password, hash string
		want                 bool
	}{
		{"right password", "correct horse battery staple", hash, true},
		{"wrong password", "wrong horse battery staple", hash, false},
		{"no account", "correct horse battery staple", "", false},
		// A password the old rules allowed still signs in.
		{"short password under an old hash", "tooshort", oldCostShortHash, true},
		// 1,200 bytes: refused before NFKC turns it into 7,200 characters.
		{"overlong input", strings.Repeat("\ufdfa", 400), hash, false},
		// The right password, but typed as 1,080 bytes of separate jamo.
		{"overlong form of the right password", jamo, hangulHash, false},
		{"longest input", longest, longestHash, true},
	} {
		ok, err := VerifyLogin(tc.password, tc.hash)
		if err != nil || ok != tc.want {
			t.Errorf("%s: VerifyLogin = %v, %v; want %v, nil", tc.name, ok, err, tc.want)
		}
	}
	// No account must still cost a hash with today's parameters.
	if NeedsRehash(dummyHash()) || !phc.MatchString(dummyHash()) {
		t.Errorf("the dummy hash %q does not use today's parameters", dummyHash())
	}
}

func TestNeedsRehash(t *testing.T) {
	t.Parallel()

	current, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	for _, tc := range []struct {
		name, hash string
		want       bool
	}{
		{"current parameters", current, false},
		{"old cost", oldCostHash, true},
		{"unreadable", "not a hash", false},
		{"empty", "", false},
		{"other salt length", "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFi$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE", true},
		{"other key length", "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04", true},
		{"other lanes", "$argon2id$v=19$m=19456,t=2,p=2$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE", true},
		{"other passes", "$argon2id$v=19$m=19456,t=3,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE", true},
	} {
		if got := NeedsRehash(tc.hash); got != tc.want {
			t.Errorf("%s: NeedsRehash = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// VerifyLogin must spend one argon2id hash, with today's memory, when no
// account matches or the input is overlong; otherwise how long a sign-in
// takes would tell which addresses have accounts. The allocation counter
// shows the hash ran, without timing anything. Not parallel: the counter
// is the whole process's.
func TestVerifyLoginSpendsAHash(t *testing.T) {
	PrepareLogin()
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	for _, tc := range []struct{ name, password, hash string }{
		{"no account", "correct horse battery staple", ""},
		{"overlong", strings.Repeat("a", MaxPasswordBytes+1), hash},
	} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, _ = VerifyLogin(tc.password, tc.hash)
		runtime.ReadMemStats(&after)
		if got := after.TotalAlloc - before.TotalAlloc; got < argonMemoryKiB*1024 {
			t.Errorf("%s: VerifyLogin allocated %d bytes, want at least %d (one argon2id hash)", tc.name, got, argonMemoryKiB*1024)
		}
	}
}
