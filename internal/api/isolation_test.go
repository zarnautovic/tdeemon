package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// party is one user of an isolationEnv and what they own: a session and
// an API token with every scope.
type party struct {
	user    user.User
	session *http.Cookie
	token   createdToken
}

// tokenName is the name of the party's token, which only they may see.
func (p party) tokenName() string { return p.user.Name + "'s phone" }

// isolationEnv is an instance with two users: the owner, whose things a
// case tries to reach, and the other, who acts. ana is an admin and ivo
// is not; TestIsolation runs every case both ways, so an admin who sees
// more than their own fails as well as a user who does.
type isolationEnv struct {
	in           *instance
	owner, other party
}

func newIsolationEnv(t *testing.T, adminActs bool) isolationEnv {
	t.Helper()

	in, anaSession := signedIn(t)
	ivo := user.User{
		ID: uuid.NewV7(), Email: "ivo@example.com", Name: "Ivo", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: in.clock.Now(), UpdatedAt: in.clock.Now(),
	}
	if err := in.db.CreateUser(t.Context(), ivo, passwordHash(t)); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ana := party{user: in.user, session: anaSession}
	ivos := party{user: ivo, session: sessionCookie(t, in.login(t, ivo.Email, password))}
	for _, p := range []*party{&ana, &ivos} {
		tok, rec := in.createToken(t, p.session, `{"name":"`+p.tokenName()+`","scopes":["ingest","read","tools"]}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s's token: %d %s", p.user.Name, rec.Code, rec.Body.String())
		}
		p.token = tok
	}
	if adminActs {
		return isolationEnv{in: in, owner: ivos, other: ana}
	}
	return isolationEnv{in: in, owner: ana, other: ivos}
}

// noneOfTheOwners fails if body shows anything of the owner's.
func (e isolationEnv) noneOfTheOwners(t *testing.T, what, body string) {
	t.Helper()
	o := e.owner
	for _, s := range []string{o.user.ID.String(), o.user.Email, o.token.ID, o.token.Token, o.tokenName()} {
		if strings.Contains(body, s) {
			t.Errorf("%s shows the owner's %q: %s", what, s, body)
		}
	}
}

// ownerSees is what the owner's session shows of their account: their
// session and their tokens, each token's last use included.
func (e isolationEnv) ownerSees(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, path := range []string{"/api/v1/auth/session", "/api/v1/tokens"} {
		rec := e.in.do(t, http.MethodGet, path, "", []*http.Cookie{e.owner.session}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("the owner's GET %s: %d %s", path, rec.Code, rec.Body.String())
		}
		b.WriteString(rec.Body.String())
	}
	return b.String()
}

// ownersTokenWorks fails if the other's request ended the owner's token.
// It uses the token, so it comes after ownerSees compares.
func (e isolationEnv) ownersTokenWorks(t *testing.T) {
	t.Helper()
	if rec := e.in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(e.owner.token.Token)); rec.Code != http.StatusOK {
		t.Errorf("the owner's token after the other's request: %d, want 200", rec.Code)
	}
}

// isolationCase runs one operation as the other user, against what the
// owner has.
type isolationCase func(t *testing.T, e isolationEnv)

// read is the case of a GET: with the other's session and, withToken,
// their token too, it must answer 200 (so the request got to the query,
// not stuck at a check), show nothing of the owner's, and show what shows
// returns of the other's own (so an empty answer does not pass).
//
//nolint:thelper // A case is a test, not a helper: a failure should point into it.
func read(path string, withToken bool, shows func(e isolationEnv) []string) isolationCase {
	return func(t *testing.T, e isolationEnv) {
		ways := map[string]func() (*http.Cookie, map[string]string){
			"session": func() (*http.Cookie, map[string]string) { return e.other.session, nil },
		}
		if withToken {
			ways["token"] = func() (*http.Cookie, map[string]string) { return nil, bearer(e.other.token.Token) }
		}
		for way, creds := range ways {
			cookie, header := creds()
			var cookies []*http.Cookie
			if cookie != nil {
				cookies = append(cookies, cookie)
			}
			rec := e.in.do(t, http.MethodGet, path, "", cookies, header)
			if rec.Code != http.StatusOK {
				t.Errorf("GET %s with the other's %s: %d %s, want 200", path, way, rec.Code, rec.Body.String())
				continue
			}
			e.noneOfTheOwners(t, "GET "+path+" with the other's "+way, rec.Body.String())
			if shows == nil {
				continue
			}
			for _, s := range shows(e) {
				if !strings.Contains(rec.Body.String(), s) {
					t.Errorf("GET %s with the other's %s does not show their own %q: %s", path, way, s, rec.Body.String())
				}
			}
		}
	}
}

// isolationCases has one case for every operation in the OpenAPI
// document. TestIsolation checks after each that what the owner sees has
// not changed and that their token still works;
// TestIsolationCoversEveryOperation fails for an operation without a
// case, and for a case without an operation.
//
//nolint:thelper // The cases are tests, not helpers: a failure should point into them.
func isolationCases() map[string]isolationCase {
	email := func(e isolationEnv) []string { return []string{e.other.user.Email} }
	return map[string]isolationCase{
		"get-info":         read("/api/v1/info", true, nil),
		"get-auth-methods": read("/api/v1/auth/methods", false, nil),
		"get-session":      read("/api/v1/auth/session", false, email),
		"whoami":           read("/api/v1/auth/whoami", true, email),
		"list-tokens": read("/api/v1/tokens", false, func(e isolationEnv) []string {
			return []string{e.other.token.ID, e.other.tokenName()}
		}),
		"login": func(t *testing.T, e isolationEnv) {
			// With the other's session, which the sign-in ends.
			rec := e.in.login(t, e.other.user.Email, password, e.other.session)
			if rec.Code != http.StatusOK {
				t.Fatalf("the other's sign-in: %d %s", rec.Code, rec.Body.String())
			}
			e.noneOfTheOwners(t, "the other's sign-in", rec.Body.String())
			if old := e.in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{e.other.session}, nil); old.Code != http.StatusUnauthorized {
				t.Errorf("the other's previous session after signing in again: %d, want 401", old.Code)
			}
		},
		"logout": func(t *testing.T, e isolationEnv) {
			if rec := e.in.do(t, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{e.other.session}, nil); rec.Code != http.StatusNoContent {
				t.Errorf("the other's sign-out: %d, want 204", rec.Code)
			}
		},
		"create-token": func(t *testing.T, e isolationEnv) {
			created, rec := e.in.createToken(t, e.other.session, `{"name":"another","scopes":["read"]}`)
			if rec.Code != http.StatusCreated {
				t.Fatalf("the other's new token: %d %s", rec.Code, rec.Body.String())
			}
			e.noneOfTheOwners(t, "the other's new token", rec.Body.String())
			who := e.in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(created.Token))
			if !strings.Contains(who.Body.String(), e.other.user.Email) {
				t.Errorf("the other's new token acts for %s, want them", who.Body.String())
			}
		},
		"revoke-token": func(t *testing.T, e isolationEnv) {
			theirs := e.in.do(t, http.MethodDelete, "/api/v1/tokens/"+e.owner.token.ID, "", []*http.Cookie{e.other.session}, nil)
			none := e.in.do(t, http.MethodDelete, "/api/v1/tokens/"+uuid.NewV7().String(), "", []*http.Cookie{e.other.session}, nil)
			if theirs.Code != http.StatusNotFound || theirs.Code != none.Code || theirs.Body.String() != none.Body.String() {
				t.Errorf("revoking the owner's token: %d %s; a token that does not exist: %d %s; want the same 404",
					theirs.Code, theirs.Body.String(), none.Code, none.Body.String())
			}
		},
	}
}

func TestIsolation(t *testing.T) {
	t.Parallel()

	for who, adminActs := range map[string]bool{"a user acts": false, "an admin acts": true} {
		t.Run(who, func(t *testing.T) {
			t.Parallel()
			for op, run := range isolationCases() {
				t.Run(op, func(t *testing.T) {
					t.Parallel()
					e := newIsolationEnv(t, adminActs)
					before := e.ownerSees(t)
					run(t, e)
					if after := e.ownerSees(t); after != before {
						t.Errorf("what the owner sees changed:\nbefore %s\nafter  %s", before, after)
					}
					e.ownersTokenWorks(t)
				})
			}
		})
	}
}

// Every operation in the OpenAPI document has an isolation case, and
// every case an operation. Routes outside the document (the OpenID
// Connect browser flow, MCP to come) are not listed here: see
// docs/conventions.md.
func TestIsolationCoversEveryOperation(t *testing.T) {
	t.Parallel()

	cases := isolationCases()
	var missing []string
	inSpec := map[string]bool{}
	for _, op := range operations(t) {
		inSpec[op.id] = true
		if _, ok := cases[op.id]; !ok {
			missing = append(missing, op.method+" "+op.path+" ("+op.id+")")
		}
	}
	var stale []string
	for id := range cases {
		if !inSpec[id] {
			stale = append(stale, id)
		}
	}
	slices.Sort(missing)
	slices.Sort(stale)
	if len(missing) > 0 {
		t.Errorf("operations without a case in isolationCases:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("cases in isolationCases for operations the API no longer has: %s", strings.Join(stale, ", "))
	}
}
