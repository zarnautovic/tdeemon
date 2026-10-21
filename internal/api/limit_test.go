package api_test

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/auth"
)

// from is in.do's header map for a request from addr (host:port).
func from(addr string, header map[string]string) map[string]string {
	h := map[string]string{"RemoteAddr": addr}
	maps.Copy(h, header)
	return h
}

func (in *instance) loginFrom(t *testing.T, addr, email, pw string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, pw)
	return in.do(t, http.MethodPost, "/api/v1/auth/login", body, nil, from(addr, nil))
}

// Ten failed sign-ins from one address in 15 minutes, and that address
// gets 429 with Retry-After until they have passed, the right password
// included; another address goes on. A success from there in between
// does not start the address's count over: signing in to one's own
// account must not wipe the failures against others.
func TestFailedSignInsFromOneAddress(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	const attacker = "198.51.100.7:4000"
	for i := range 10 {
		if i == 9 {
			if res := in.loginFrom(t, attacker, "ana@example.com", password); res.Code != http.StatusOK {
				t.Fatalf("a success after nine failures: %d, want 200", res.Code)
			}
		}
		// Another account each time, so only the address adds up.
		if res := in.loginFrom(t, attacker, fmt.Sprintf("guess%d@example.com", i), "wrong password, surely"); res.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: %d, want 401", i+1, res.Code)
		}
	}
	res := in.loginFrom(t, attacker, "ana@example.com", password)
	if res.Code != http.StatusTooManyRequests || res.Header().Get("Retry-After") != "900" {
		t.Errorf("the right password after ten failures: %d, Retry-After %q; want 429 and 900", res.Code, res.Header().Get("Retry-After"))
	}
	if res := in.loginFrom(t, "198.51.100.8:4000", "ana@example.com", password); res.Code != http.StatusOK {
		t.Errorf("from another address: %d, want 200", res.Code)
	}
	in.clock.Set(in.clock.Now().Add(15 * time.Minute))
	if res := in.loginFrom(t, attacker, "ana@example.com", password); res.Code != http.StatusOK {
		t.Errorf("after 15 minutes: %d, want 200", res.Code)
	}
}

// Ten failed sign-ins for one account, from any addresses, and the
// account gets 429 everywhere until the window passes; a success before
// that starts its count over. An address nobody has counts the same, so
// the answer tells nothing about which accounts exist.
func TestFailedSignInsForOneAccount(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	addr := func(i int) string { return fmt.Sprintf("203.0.113.%d:5000", i) }
	for _, email := range []string{"ana@example.com", "nobody@example.com"} {
		for i := range 9 {
			if res := in.loginFrom(t, addr(i), email, "wrong password, surely"); res.Code != http.StatusUnauthorized {
				t.Fatalf("%s, failure %d: %d, want 401", email, i+1, res.Code)
			}
		}
	}
	// Ana's success starts her count over; nobody's goes on.
	if res := in.loginFrom(t, addr(100), "ana@example.com", password); res.Code != http.StatusOK {
		t.Fatalf("ana after nine failures: %d, want 200", res.Code)
	}
	if res := in.loginFrom(t, addr(101), "nobody@example.com", "wrong password, surely"); res.Code != http.StatusUnauthorized {
		t.Fatalf("nobody's tenth failure: %d, want 401", res.Code)
	}
	if res := in.loginFrom(t, addr(102), "nobody@example.com", "wrong password, surely"); res.Code != http.StatusTooManyRequests {
		t.Errorf("nobody after ten failures: %d, want 429", res.Code)
	}
	for i := range 9 {
		if res := in.loginFrom(t, addr(110+i), "ana@example.com", "wrong password, surely"); res.Code != http.StatusUnauthorized {
			t.Fatalf("ana, failure %d after the success: %d, want 401", i+1, res.Code)
		}
	}
	if res := in.loginFrom(t, addr(120), "ana@example.com", password); res.Code != http.StatusOK {
		t.Errorf("ana after nine new failures: %d, want 200 (the success had started over)", res.Code)
	}
}

// Fifty bad API tokens from one address in 15 minutes, and every Bearer
// request from there gets 429, a good token included, until they have
// passed. The session cookie and other addresses go on.
func TestBadTokensFromOneAddress(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, _ := in.createToken(t, session, `{"name":"iPhone","scopes":["ingest"]}`)
	const attacker = "198.51.100.9:6000"
	for i := range 50 {
		bad := bearer(auth.TokenPrefix + strings.Repeat("A", 42) + fmt.Sprint(i%10))
		if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, from(attacker, bad)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("bad token %d: %d, want 401", i+1, rec.Code)
		}
	}
	rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, from(attacker, bearer(tok.Token)))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "900" {
		t.Errorf("the good token after fifty bad ones: %d, Retry-After %q; want 429 and 900", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", []*http.Cookie{session}, from(attacker, nil)); rec.Code != http.StatusOK {
		t.Errorf("the session from there: %d, want 200", rec.Code)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, from("198.51.100.10:6000", bearer(tok.Token))); rec.Code != http.StatusOK {
		t.Errorf("the good token from another address: %d, want 200", rec.Code)
	}
	in.clock.Set(in.clock.Now().Add(15 * time.Minute))
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, from(attacker, bearer(tok.Token))); rec.Code != http.StatusOK {
		t.Errorf("after 15 minutes: %d, want 200", rec.Code)
	}
}
