package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
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

// A token cannot do what only a person in a browser may: manage tokens,
// read the session.
func TestTokensCannotActForASession(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, _ := in.createToken(t, session, `{"name":"Claude","scopes":["tools","read","ingest"]}`)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/tokens", ""},
		{http.MethodPost, "/api/v1/tokens", `{"name":"more","scopes":["read"]}`},
		{http.MethodDelete, "/api/v1/tokens/" + tok.ID, ""},
		{http.MethodGet, "/api/v1/auth/session", ""},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			rec := in.do(t, tc.method, tc.path, tc.body, nil, bearer(tok.Token))
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "needs a web session") {
				t.Errorf("%d %s, want 403", rec.Code, rec.Body.String())
			}
		})
	}
	// Signing out with a token ends nothing; the token and the session go on.
	if rec := in.do(t, http.MethodPost, "/api/v1/auth/logout", "", nil, bearer(tok.Token)); rec.Code != http.StatusNoContent {
		t.Errorf("logout with a token: %d", rec.Code)
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
		"no tdm_ prefix":             "Bearer " + strings.Repeat("A", 47),
		"another scheme":             "Basic YW5hOnB3",
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
	// Outside /api/ a header changes nothing.
	if rec := in.do(t, http.MethodGet, "/healthz", "", nil, map[string]string{"Authorization": "Bearer nonsense"}); rec.Code != http.StatusOK {
		t.Errorf("/healthz with a bad token: %d, want 200", rec.Code)
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
		if rec := in.do(t, http.MethodDelete, "/api/v1/tokens/"+id, "", []*http.Cookie{session}, nil); rec.Code != http.StatusNotFound {
			t.Errorf("revoke %s: %d, want 404", name, rec.Code)
		}
	}
}

// Another user's token looks exactly like one that does not exist.
func TestTokensOfAnotherUser(t *testing.T) {
	t.Parallel()

	in, anaSession := signedIn(t)
	ivo := user.User{
		ID: uuid.NewV7(), Email: "ivo@example.com", Name: "Ivo", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: in.clock.Now(), UpdatedAt: in.clock.Now(),
	}
	if err := in.db.CreateUser(t.Context(), ivo, passwordHash(t)); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ivoSession := sessionCookie(t, in.login(t, "ivo@example.com", password))
	anas, _ := in.createToken(t, anaSession, `{"name":"ana's","scopes":["read"]}`)

	if rec := in.do(t, http.MethodGet, "/api/v1/tokens", "", []*http.Cookie{ivoSession}, nil); strings.Contains(rec.Body.String(), anas.ID) {
		t.Errorf("ivo's list shows ana's token: %s", rec.Body.String())
	}
	if rec := in.do(t, http.MethodDelete, "/api/v1/tokens/"+anas.ID, "", []*http.Cookie{ivoSession}, nil); rec.Code != http.StatusNotFound {
		t.Errorf("ivo revoking ana's token: %d, want 404", rec.Code)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(anas.Token)); rec.Code != http.StatusOK {
		t.Errorf("ana's token after ivo's attempt: %d, want 200", rec.Code)
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
