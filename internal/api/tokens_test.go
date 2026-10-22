package api_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/store"
)

type createdToken struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	Token  string   `json:"token"`
}

// signedIn returns an instance and ana's session cookie.
func signedIn(t *testing.T) (*instance, *http.Cookie) {
	t.Helper()
	in := newInstance(t, "http://localhost", passwordHash(t))
	return in, sessionCookie(t, in.login(t, "ana@example.com", password))
}

func (in *instance) createToken(t *testing.T, session *http.Cookie, body string) (createdToken, *httptest.ResponseRecorder) {
	t.Helper()
	var cookies []*http.Cookie
	if session != nil {
		cookies = append(cookies, session)
	}
	rec := in.do(t, http.MethodPost, "/api/v1/tokens", body, cookies, nil)
	var c createdToken
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
	}
	return c, rec
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestCreateAndUseAToken(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, rec := in.createToken(t, session, `{"name":" iPhone ","scopes":["tools","ingest"]}`)
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create: %d %s (Cache-Control %q)", rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
	}
	if !strings.HasPrefix(tok.Token, auth.TokenPrefix) || tok.Name != "iPhone" || strings.Join(tok.Scopes, " ") != "ingest tools" {
		t.Errorf("created = %+v, want a tdm_ token named iPhone with ingest and tools", tok)
	}

	who := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(tok.Token))
	if who.Code != http.StatusOK || !strings.Contains(who.Body.String(), `"via":"token"`) ||
		!strings.Contains(who.Body.String(), `"scopes":["ingest","tools"]`) || !strings.Contains(who.Body.String(), `"email":"ana@example.com"`) {
		t.Errorf("whoami with the token: %d %s", who.Code, who.Body.String())
	}
	// The scheme's case does not matter (RFC 6750 follows RFC 7235).
	if who := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, map[string]string{"Authorization": "bearer " + tok.Token}); who.Code != http.StatusOK {
		t.Errorf("whoami with a lower-case scheme: %d", who.Code)
	}
	if who := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", []*http.Cookie{session}, nil); !strings.Contains(who.Body.String(), `"via":"session"`) {
		t.Errorf("whoami with the session: %s", who.Body.String())
	}

	// The list shows it, never the token again.
	list := in.do(t, http.MethodGet, "/api/v1/tokens", "", []*http.Cookie{session}, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), tok.ID) || strings.Contains(list.Body.String(), tok.Token) {
		t.Errorf("list: %d %s", list.Code, list.Body.String())
	}
}

// operation is one operation of the API, as the OpenAPI document has it.
type operation struct {
	id, method, path string
	// route is the path as the document has it, with {parameters}.
	route string
	// statuses are the response codes the document lists for it.
	statuses []string
}

// operations lists every operation in the OpenAPI document, with path
// parameters filled in with a fresh UUID.
func operations(t *testing.T) []operation {
	t.Helper()
	raw, err := api.Spec()
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string         `json:"operationId"`
			Responses   map[string]any `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("decode the spec: %v", err)
	}
	param := regexp.MustCompile(`\{[^}]+\}`)
	var ops []operation
	for path, methods := range spec.Paths {
		for method, op := range methods {
			ops = append(ops, operation{
				op.OperationID, strings.ToUpper(method), param.ReplaceAllString(path, uuid.NewV7().String()), path,
				slices.Collect(maps.Keys(op.Responses)),
			})
		}
	}
	if len(ops) == 0 {
		t.Fatal("the spec has no operations")
	}
	return ops
}

// A token reaches only the operations open to it; everything else, a new
// endpoint included, is closed to tokens: managing tokens, reading the
// session, signing in or out. The session cookie next to the token (a
// browser that has both) does not change that. (An operation open to
// some scopes also answers 403 to a token with none of them, which
// TestRequestProblemsAreDocumented holds to the document.)
func TestTokensReachOnlyOperationsOpenToThem(t *testing.T) {
	t.Parallel()

	open := map[string]bool{"get-info": true, "whoami": true, "ingest-hae": true, "ingest-metrics": true, "get-me": true, "get-day": true}
	in, session := signedIn(t)
	tok, _ := in.createToken(t, session, `{"name":"Claude","scopes":["tools","read","ingest"]}`)
	for _, op := range operations(t) {
		t.Run(op.id, func(t *testing.T) {
			t.Parallel()
			for _, cookies := range [][]*http.Cookie{nil, {session}} {
				rec := in.do(t, op.method, op.path, "", cookies, bearer(tok.Token))
				refused := rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "closed to API tokens")
				if refused == open[op.id] {
					t.Errorf("%s %s with %d cookies: %d %s; open to tokens: %v", op.method, op.path, len(cookies), rec.Code, rec.Body.String(), open[op.id])
				}
			}
			// The document says so.
			if !open[op.id] && !slices.Contains(op.statuses, "403") {
				t.Errorf("the OpenAPI document lists %v for %s, not the 403 a token gets", op.statuses, op.id)
			}
		})
	}
}

// A token cannot sign out the session it came with: the session goes on.
func TestATokenCannotSignOut(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, _ := in.createToken(t, session, `{"name":"Claude","scopes":["tools"]}`)
	if rec := in.do(t, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{session}, bearer(tok.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("logout with a token: %d, want 403", rec.Code)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/tokens", "", []*http.Cookie{session}, nil); rec.Code != http.StatusOK {
		t.Errorf("the session after a token's logout: %d, want 200", rec.Code)
	}
}

func TestBadTokens(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	for name, header := range map[string]string{
		"unknown token":              "Bearer " + auth.TokenPrefix + strings.Repeat("A", 43),
		"two spaces":                 "Bearer  " + auth.TokenPrefix + strings.Repeat("A", 43),
		"no token":                   "Bearer",
		"empty token":                "Bearer ",
		"unknown, lower-case scheme": "bearer " + auth.TokenPrefix + "x",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// The session cookie does not rescue a bad token: a request that
			// tries a token is judged by it.
			rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", []*http.Cookie{session}, map[string]string{"Authorization": header})
			if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != `Bearer error="invalid_token"` {
				t.Errorf("%d, WWW-Authenticate %q; want 401 with the challenge", rec.Code, rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
	// A token with scopes tdeemon does not know (a row from a later
	// version, say) is not valid.
	token, hash := auth.NewAPIToken()
	odd := store.APIToken{ID: uuid.NewV7(), UserID: in.user.ID, Name: "odd", Scopes: "read admin", CreatedAt: in.clock.Now()}
	if err := in.db.CreateAPIToken(t.Context(), odd, hash); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(token)); rec.Code != http.StatusUnauthorized {
		t.Errorf("a token with an unknown scope: %d, want 401", rec.Code)
	}
	// Outside /api/ a header changes nothing.
	if rec := in.do(t, http.MethodGet, "/healthz", "", nil, map[string]string{"Authorization": "Bearer nonsense"}); rec.Code != http.StatusOK {
		t.Errorf("/healthz with a bad token: %d, want 200", rec.Code)
	}
}

// An Authorization header that is no API token leaves the request to the
// session cookie, and the sign-in still works: a reverse proxy's Basic
// credentials, or the ID token an authenticating proxy passes on as a
// Bearer, which the browser's requests then all carry.
func TestOtherCredentialsAreNoToken(t *testing.T) {
	t.Parallel()

	//nolint:gosec // G101: made-up credentials of other schemes, the point of the test.
	for name, header := range map[string]string{
		"Basic":            "Basic YW5hOnB3",
		"a proxy's Bearer": "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhbmEifQ.c2ln",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			otherCredentialsAreNoToken(t, map[string]string{"Authorization": header})
		})
	}
}

func otherCredentialsAreNoToken(t *testing.T, header map[string]string) {
	t.Helper()
	in := newInstance(t, "http://localhost", passwordHash(t))
	rec := in.do(t, http.MethodPost, "/api/v1/auth/login", `{"email":"ana@example.com","password":"`+password+`"}`, nil, header)
	if rec.Code != http.StatusOK {
		t.Fatalf("login with the header: %d %s, want 200", rec.Code, rec.Body.String())
	}
	session := sessionCookie(t, rec)
	if who := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", []*http.Cookie{session}, header); who.Code != http.StatusOK ||
		!strings.Contains(who.Body.String(), `"via":"session"`) {
		t.Errorf("whoami with the header and the session: %d %s, want the session", who.Code, who.Body.String())
	}
	if who := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, header); who.Code != http.StatusUnauthorized ||
		who.Header().Get("WWW-Authenticate") != "" {
		t.Errorf("whoami with the header only: %d, WWW-Authenticate %q; want 401 without a token challenge",
			who.Code, who.Header().Get("WWW-Authenticate"))
	}
}

func TestRevokeAToken(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, _ := in.createToken(t, session, `{"name":"iPhone","scopes":["ingest"]}`)
	if rec := in.do(t, http.MethodDelete, "/api/v1/tokens/"+tok.ID, "", []*http.Cookie{session}, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(tok.Token)); rec.Code != http.StatusUnauthorized {
		t.Errorf("the revoked token: %d, want 401", rec.Code)
	}
	for name, id := range map[string]string{"again": tok.ID, "unknown": uuid.NewV7().String()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if rec := in.do(t, http.MethodDelete, "/api/v1/tokens/"+id, "", []*http.Cookie{session}, nil); rec.Code != http.StatusNotFound {
				t.Errorf("revoke: %d, want 404", rec.Code)
			}
		})
	}
}

func TestCreateTokenRefuses(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	for name, body := range map[string]string{
		"an unknown scope": `{"name":"x","scopes":["admin"]}`,
		"no scopes":        `{"name":"x","scopes":[]}`,
		"a blank name":     `{"name":"   ","scopes":["read"]}`,
		"a control char":   `{"name":"a\u0007b","scopes":["read"]}`,
		"a repeated scope": `{"name":"x","scopes":["read","read"]}`,
		"two in one item":  `{"name":"x","scopes":["read tools"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, rec := in.createToken(t, session, body); rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%d %s, want 422", rec.Code, rec.Body.String())
			}
		})
	}
	if _, rec := in.createToken(t, nil, `{"name":"x","scopes":["read"]}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("create without a session: %d, want 401", rec.Code)
	}
}

// A token's use is recorded at most once an hour.
func TestTokenUseIsRecorded(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, _ := in.createToken(t, session, `{"name":"iPhone","scopes":["ingest"]}`)
	start := in.clock.Now()
	lastUsed := func() string {
		t.Helper()
		var b struct {
			Tokens []struct {
				LastUsedAt *time.Time `json:"last_used_at"`
			} `json:"tokens"`
		}
		rec := in.do(t, http.MethodGet, "/api/v1/tokens", "", []*http.Cookie{session}, nil)
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || len(b.Tokens) != 1 {
			t.Fatalf("list %s: %v", rec.Body.String(), err)
		}
		if b.Tokens[0].LastUsedAt == nil {
			return "never"
		}
		return b.Tokens[0].LastUsedAt.Sub(start).String()
	}
	if got := lastUsed(); got != "never" {
		t.Errorf("before any use: %s, want never", got)
	}
	in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(tok.Token))
	if got := lastUsed(); got != "0s" {
		t.Errorf("after the first use: %s, want 0s", got)
	}
	in.clock.Set(start.Add(30 * time.Minute))
	in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(tok.Token))
	if got := lastUsed(); got != "0s" {
		t.Errorf("half an hour later: %s, want still 0s", got)
	}
	in.clock.Set(start.Add(2 * time.Hour))
	in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(tok.Token))
	if got := lastUsed(); got != "2h0m0s" {
		t.Errorf("two hours later: %s, want 2h0m0s", got)
	}
}

func TestWhoamiWithoutAnyone(t *testing.T) {
	t.Parallel()

	in, _ := signedIn(t)
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("whoami without credentials: %d, want 401", rec.Code)
	}
}
