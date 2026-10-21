package api_test

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

const password = "correct horse battery staple"

// clock is a settable time for the handler under test.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *clock) Set(t time.Time) { c.mu.Lock(); defer c.mu.Unlock(); c.now = t }

type instance struct {
	h     http.Handler
	db    *store.DB
	clock *clock
	user  user.User
}

// newInstance starts a handler for baseURL over a database with one user,
// ana@example.com, whose password hash is hash.
func newInstance(t *testing.T, baseURL, hash string) *instance {
	t.Helper()

	db := storetest.Open(t)
	c := &clock{now: time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)}
	u := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleAdmin,
		Locale: "hr", TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: c.now, UpdatedAt: c.now,
	}
	if err := db.CreateUser(t.Context(), u, hash); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse %s: %v", baseURL, err)
	}
	h, err := api.NewHandler(api.Config{Info: testInfo, DB: db, BaseURL: base, Logger: slog.New(slog.DiscardHandler), Now: c.Now})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return &instance{h: h, db: db, clock: c, user: u}
}

// passwordHash hashes password, the password of the test user.
func passwordHash(t *testing.T) string {
	t.Helper()
	h, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return h
}

func (in *instance) do(t *testing.T, method, path, body string, cookies []*http.Cookie, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, path, nil)
	} else {
		req = httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for k, v := range header {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	in.h.ServeHTTP(rec, req)
	return rec
}

func (in *instance) login(t *testing.T, email, pw string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	return in.do(t, http.MethodPost, "/api/v1/auth/login", string(body), cookies, nil)
}

// requestCookie is a cookie as a client sends it: a name and a value.
func requestCookie(name, value string) *http.Cookie {
	return &http.Cookie{Name: name, Value: value} //nolint:gosec // G124: a request's cookie carries no attributes.
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("response sets %d cookies, want 1: %v", len(cookies), rec.Header().Values("Set-Cookie"))
	}
	return cookies[0]
}

type sessionResponse struct {
	User struct {
		ID, Email, Name, Role, Locale, TimeZone string
		DayStartHour                            int `json:"day_start_hour"`
	} `json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
}

func decodeSession(t *testing.T, rec *httptest.ResponseRecorder) sessionResponse {
	t.Helper()
	var s sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return s
}

func TestLoginSetsTheSessionCookie(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	rec := in.login(t, " ANA@Example.com ", password)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	c := sessionCookie(t, rec)
	if c.Name != "tdeemon_session" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Secure {
		t.Errorf("cookie = %+v, want tdeemon_session, HttpOnly, SameSite=Lax, Path=/, not Secure over http", c)
	}
	if want := int(auth.SessionLifetime.Seconds()); c.MaxAge != want {
		t.Errorf("Max-Age = %d, want %d", c.MaxAge, want)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	s := decodeSession(t, rec)
	if s.User.ID != in.user.ID.String() || s.User.Email != "ana@example.com" || s.User.TimeZone != "Europe/Zagreb" ||
		s.User.DayStartHour != 4 || !s.ExpiresAt.Equal(in.clock.Now().Add(auth.SessionLifetime)) {
		t.Errorf("login body = %+v", s)
	}
}

// The database holds the token's hash, never the token.
func TestLoginStoresTheTokensHash(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	c := sessionCookie(t, in.login(t, "ana@example.com", password))
	if _, _, err := in.db.SessionUser(t.Context(), c.Value, in.clock.Now()); err == nil {
		t.Error("the session is stored under the token itself")
	}
	if _, _, err := in.db.SessionUser(t.Context(), auth.SessionID(c.Value), in.clock.Now()); err != nil {
		t.Errorf("no session under the token's hash: %v", err)
	}
}

func TestSessionAfterLogin(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	c := sessionCookie(t, in.login(t, "ana@example.com", password))
	rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get session: %d %s", rec.Code, rec.Body.String())
	}
	if s := decodeSession(t, rec); s.User.Email != "ana@example.com" || s.User.Role != "admin" ||
		!s.ExpiresAt.Equal(in.clock.Now().Add(auth.SessionLifetime)) {
		t.Errorf("session body = %+v", s)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestLoginOverHTTPSUsesAHostCookie(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "https://tdeemon.example", passwordHash(t))
	rec := in.login(t, "ana@example.com", password)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	c := sessionCookie(t, rec)
	if c.Name != "__Host-tdeemon_session" || !c.Secure || c.Domain != "" || c.Path != "/" {
		t.Errorf("cookie = %+v, want __Host-tdeemon_session, Secure, Path=/, no Domain", c)
	}
	// The plain name means nothing over HTTPS.
	plain := requestCookie("tdeemon_session", c.Value)
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{plain}, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("session with the plain cookie name: %d, want 401", rec.Code)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil); rec.Code != http.StatusOK {
		t.Errorf("session with the host cookie: %d, want 200", rec.Code)
	}
}

// Every failed sign-in looks the same from outside.
func TestLoginFailures(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	noPassword := user.User{
		ID: uuid.NewV7(), Email: "ivo@example.com", Name: "Ivo", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: in.clock.Now(), UpdatedAt: in.clock.Now(),
	}
	if err := in.db.CreateUser(t.Context(), noPassword, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	var first string
	for _, tc := range []struct{ name, email, password string }{
		{"wrong password", "ana@example.com", "wrong horse battery staple"},
		{"unknown address", "nobody@example.com", password},
		{"no password on the account", "ivo@example.com", password},
		{"address not UTF-8", "ana\xff@example.com", password},
	} {
		rec := in.login(t, tc.email, tc.password)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d %s, want 401", tc.name, rec.Code, rec.Body.String())
			continue
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: a failed sign-in set a cookie", tc.name)
		}
		if first == "" {
			first = rec.Body.String()
		} else if rec.Body.String() != first {
			t.Errorf("%s: body %s differs from %s", tc.name, rec.Body.String(), first)
		}
	}
	if !strings.Contains(first, "wrong email or password") {
		t.Errorf("failure body %s does not say wrong email or password", first)
	}
}

// Malformed sign-ins fail like wrong ones, and never echo the password
// back, as a validation error would.
func TestLoginWithMissingOrOddFields(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	long := strings.Repeat("p", 5000)
	for name, body := range map[string]string{ //nolint:gosec // G101: made-up passwords of a test user.
		"no password":         `{"email":"ana@example.com"}`,
		"empty password":      `{"email":"ana@example.com","password":""}`,
		"no email":            `{"password":"correct horse battery staple"}`,
		"overlong password":   `{"email":"ana@example.com","password":"` + long + `"}`,
		"an unexpected field": `{"email":"ana@example.com","password":"wrong horse battery staple","remember":true}`,
	} {
		rec := in.do(t, http.MethodPost, "/api/v1/auth/login", body, nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d %s, want 401", name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "horse") || strings.Contains(rec.Body.String(), long[:100]) {
			t.Errorf("%s: the response echoes the password: %s", name, rec.Body.String())
		}
	}
	// The extra field does not get in the way of a right password.
	body := `{"email":"ana@example.com","password":"` + password + `","remember":true}`
	if rec := in.do(t, http.MethodPost, "/api/v1/auth/login", body, nil, nil); rec.Code != http.StatusOK {
		t.Errorf("right password with an extra field: %d, want 200", rec.Code)
	}
}

func TestLogout(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	c := sessionCookie(t, in.login(t, "ana@example.com", password))

	rec := in.do(t, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{c}, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", rec.Code, rec.Body.String())
	}
	if cleared := sessionCookie(t, rec); cleared.Name != c.Name || cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Errorf("logout cookie = %+v, want the session cookie cleared", cleared)
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("session after logout: %d, want 401", rec.Code)
	}
	// Signing out twice, or without a session, is fine.
	for _, cookies := range [][]*http.Cookie{{c}, nil} {
		if rec := in.do(t, http.MethodPost, "/api/v1/auth/logout", "", cookies, nil); rec.Code != http.StatusNoContent {
			t.Errorf("logout again: %d, want 204", rec.Code)
		}
	}
}

func TestSessionWithoutACookie(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	for _, cookies := range [][]*http.Cookie{nil, {requestCookie("tdeemon_session", "forged")}, {requestCookie("tdeemon_session", "")}} {
		if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", cookies, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("session with %v: %d, want 401", cookies, rec.Code)
		}
	}
}

func TestSessionExpires(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	c := sessionCookie(t, in.login(t, "ana@example.com", password))
	start := in.clock.Now()

	in.clock.Set(start.Add(auth.SessionLifetime - time.Second))
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil); rec.Code != http.StatusOK {
		t.Errorf("a second before the end: %d, want 200", rec.Code)
	}
	in.clock.Set(start.Add(auth.SessionLifetime))
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("at the end: %d, want 401", rec.Code)
	}
}

// Use is recorded at most once an hour.
func TestSessionUseIsRecorded(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	c := sessionCookie(t, in.login(t, "ana@example.com", password))
	id := auth.SessionID(c.Value)
	start := in.clock.Now()
	lastSeen := func() time.Time {
		t.Helper()
		s, _, err := in.db.SessionUser(t.Context(), id, in.clock.Now())
		if err != nil {
			t.Fatalf("SessionUser: %v", err)
		}
		return s.LastSeenAt
	}

	for _, step := range []struct {
		at   time.Duration
		want time.Duration
	}{
		{59 * time.Minute, 0},                         // too soon: no write
		{2 * time.Hour, 2 * time.Hour},                // recorded
		{2*time.Hour + 30*time.Minute, 2 * time.Hour}, // too soon again
	} {
		in.clock.Set(start.Add(step.at))
		in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil)
		if got := lastSeen(); !got.Equal(start.Add(step.want)) {
			t.Errorf("after a request at +%v: last seen %v, want +%v", step.at, got.Sub(start), step.want)
		}
	}
}

// Signing in again ends the session the browser had, so a token planted
// before sign-in never becomes a signed-in one.
func TestLoginEndsThePreviousSession(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	old := sessionCookie(t, in.login(t, "ana@example.com", password))
	fresh := sessionCookie(t, in.login(t, "ana@example.com", password, old))
	if fresh.Value == old.Value {
		t.Fatal("signing in again kept the token")
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{old}, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("the old session after a new sign-in: %d, want 401", rec.Code)
	}
}

func storedHash(t *testing.T, in *instance) string {
	t.Helper()
	_, h, err := in.db.UserCredentials(t.Context(), "ana@example.com")
	if err != nil {
		t.Fatalf("UserCredentials: %v", err)
	}
	return h
}

// A sign-in with a hash of an older cost stores a new one, unless the
// password no longer meets the rules for a new password.
func TestLoginRehashes(t *testing.T) {
	t.Parallel()

	// Reference hashes (argon2-cffi 25.1.0) with m=8, t=1, p=1.
	const (
		oldCost      = "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE" // correct horse battery staple
		oldCostShort = "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$SDvn1ahgbEqoZxYWl4nLdCrT350l3ejghSpaRC0ZMXQ" // tooshort
	)
	in := newInstance(t, "http://localhost", oldCost)
	// A session from before the rehash, which must survive it.
	before := in.do(t, http.MethodPost, "/api/v1/auth/login",
		`{"email":"ana@example.com","password":"`+password+`"}`, nil, nil)
	earlier := sessionCookie(t, before)
	if rec := in.login(t, "ana@example.com", password); rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{earlier}, nil); rec.Code != http.StatusOK {
		t.Errorf("a session from before the rehash: %d, want 200", rec.Code)
	}
	if h := storedHash(t, in); !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") || auth.NeedsRehash(h) {
		t.Errorf("hash after sign-in = %q, want today's parameters", h)
	}
	if rec := in.login(t, "ana@example.com", password); rec.Code != http.StatusOK {
		t.Errorf("sign-in with the new hash: %d", rec.Code)
	}

	short := newInstance(t, "http://localhost", oldCostShort)
	if rec := short.login(t, "ana@example.com", "tooshort"); rec.Code != http.StatusOK {
		t.Fatalf("login with an old short password: %d %s", rec.Code, rec.Body.String())
	}
	if h := storedHash(t, short); h != oldCostShort {
		t.Errorf("hash after sign-in = %q, want the old one kept", h)
	}
}

// A corrupted stored hash fails the sign-in like a wrong password.
func TestLoginWithAnUnreadableHash(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "not an argon2 hash")
	if rec := in.login(t, "ana@example.com", password); rec.Code != http.StatusUnauthorized {
		t.Errorf("login: %d, want 401", rec.Code)
	}
}

// A browser's cross-origin POST is refused before anything runs; what
// comes from the instance's own origin, or from no browser, goes on.
func TestCrossOriginRequests(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "https://tdeemon.example", passwordHash(t))
	body, _ := json.Marshal(map[string]string{"email": "ana@example.com", "password": password})
	for _, tc := range []struct {
		name   string
		header map[string]string
		want   int
	}{
		{"cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-site fetch from a sibling", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"foreign origin", map[string]string{"Origin": "https://evil.example", "Host": "tdeemon.example"}, http.StatusForbidden},
		{"same-origin fetch", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusOK},
		{"own origin behind a proxy", map[string]string{"Origin": "https://tdeemon.example", "Host": "tdeemon:8080"}, http.StatusOK},
		{"no browser", nil, http.StatusOK},
	} {
		rec := in.do(t, http.MethodPost, "/api/v1/auth/login", string(body), nil, tc.header)
		if rec.Code != tc.want {
			t.Errorf("%s: %d %s, want %d", tc.name, rec.Code, rec.Body.String(), tc.want)
		}
		if tc.want == http.StatusForbidden && !strings.Contains(rec.Body.String(), "cross-origin request refused") {
			t.Errorf("%s: body %s, want the problem", tc.name, rec.Body.String())
		}
	}
	// Reads are never refused.
	rec := in.do(t, http.MethodGet, "/api/v1/info", "", nil, map[string]string{"Sec-Fetch-Site": "cross-site"})
	if rec.Code != http.StatusOK {
		t.Errorf("cross-site GET: %d, want 200", rec.Code)
	}
}

func TestNewHandlerNeedsItsDependencies(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	base := &url.URL{Scheme: "http", Host: "localhost"}
	logger := slog.New(slog.DiscardHandler)
	for name, cfg := range map[string]api.Config{
		"no database": {BaseURL: base, Logger: logger},
		"no base URL": {DB: db, Logger: logger},
		"no logger":   {DB: db, BaseURL: base},
	} {
		if _, err := api.NewHandler(cfg); err == nil {
			t.Errorf("%s: NewHandler succeeded", name)
		}
	}
}

// failWrites makes the database refuse one kind of write with a trigger.
func (in *instance) failWrites(t *testing.T, when, table string) {
	t.Helper()
	err := in.db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "CREATE TRIGGER fail_"+strings.ToLower(when)+"_"+table+
			" BEFORE "+when+" ON "+table+" BEGIN SELECT RAISE(ABORT, 'refused by the test'); END")
		return err
	})
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}
}

// When the database fails, a request that needs it fails with a 500 and
// nothing half-done; one that only records something goes on.
func TestDatabaseFailures(t *testing.T) {
	t.Parallel()

	t.Run("starting the session", func(t *testing.T) {
		t.Parallel()
		in := newInstance(t, "http://localhost", passwordHash(t))
		in.failWrites(t, "INSERT", "sessions")
		if rec := in.login(t, "ana@example.com", password); rec.Code != http.StatusInternalServerError || len(rec.Result().Cookies()) != 0 {
			t.Errorf("login: %d with cookies %v, want 500 and none", rec.Code, rec.Result().Cookies())
		}
	})
	t.Run("ending the session", func(t *testing.T) {
		t.Parallel()
		in := newInstance(t, "http://localhost", passwordHash(t))
		c := sessionCookie(t, in.login(t, "ana@example.com", password))
		in.failWrites(t, "DELETE", "sessions")
		if rec := in.do(t, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{c}, nil); rec.Code != http.StatusInternalServerError {
			t.Errorf("logout: %d, want 500", rec.Code)
		}
		// Signing in again cannot end the old session either; it goes on.
		if rec := in.login(t, "ana@example.com", password, c); rec.Code != http.StatusOK {
			t.Errorf("login over a session that cannot end: %d, want 200", rec.Code)
		}
	})
	t.Run("recording use", func(t *testing.T) {
		t.Parallel()
		in := newInstance(t, "http://localhost", passwordHash(t))
		c := sessionCookie(t, in.login(t, "ana@example.com", password))
		in.failWrites(t, "UPDATE", "sessions")
		in.clock.Set(in.clock.Now().Add(2 * time.Hour))
		if rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil); rec.Code != http.StatusOK {
			t.Errorf("session: %d, want 200", rec.Code)
		}
	})
	t.Run("rehashing", func(t *testing.T) {
		t.Parallel()
		in := newInstance(t, "http://localhost", "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE")
		in.failWrites(t, "UPDATE", "users")
		if rec := in.login(t, "ana@example.com", password); rec.Code != http.StatusOK {
			t.Errorf("login: %d, want 200 with the old hash kept", rec.Code)
		}
	})
	t.Run("reading", func(t *testing.T) {
		t.Parallel()
		in := newInstance(t, "http://localhost", passwordHash(t))
		c := sessionCookie(t, in.login(t, "ana@example.com", password))
		if err := in.db.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if rec := in.login(t, "ana@example.com", password); rec.Code != http.StatusInternalServerError {
			t.Errorf("login: %d, want 500", rec.Code)
		}
		rec := in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil)
		if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "the session could not be checked") {
			t.Errorf("session: %d %s, want the 500 problem", rec.Code, rec.Body.String())
		}
	})
}

// The database keeps times to the second; the sign-in's answer must give
// the same end of session as a later read.
func TestSessionTimesAreToTheSecond(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	in.clock.Set(in.clock.Now().Add(123456789 * time.Nanosecond))
	rec := in.login(t, "ana@example.com", password)
	login := decodeSession(t, rec)
	c := sessionCookie(t, rec)
	read := decodeSession(t, in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{c}, nil))
	if !login.ExpiresAt.Equal(read.ExpiresAt) || login.ExpiresAt.Nanosecond() != 0 {
		t.Errorf("expires_at at sign-in %v, read back %v; want the same whole second", login.ExpiresAt, read.ExpiresAt)
	}
}
