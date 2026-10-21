package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// isolationEnv is an instance with two users. ana owns things (a session,
// an API token); ivo is the other user every case acts as.
type isolationEnv struct {
	in       *instance
	ana, ivo *http.Cookie // their sessions
	anaToken createdToken
}

func newIsolationEnv(t *testing.T) isolationEnv {
	t.Helper()

	in, ana := signedIn(t)
	ivo := user.User{
		ID: uuid.NewV7(), Email: "ivo@example.com", Name: "Ivo", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: in.clock.Now(), UpdatedAt: in.clock.Now(),
	}
	if err := in.db.CreateUser(t.Context(), ivo, passwordHash(t)); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tok, rec := in.createToken(t, ana, `{"name":"ana's phone","scopes":["ingest","read","tools"]}`)
	if rec.Code != 201 {
		t.Fatalf("create ana's token: %d %s", rec.Code, rec.Body.String())
	}
	return isolationEnv{in: in, ana: ana, ivo: sessionCookie(t, in.login(t, "ivo@example.com", password)), anaToken: tok}
}

// noneOfAnas fails if body shows anything of ana's.
func (e isolationEnv) noneOfAnas(t *testing.T, what, body string) {
	t.Helper()
	for _, s := range []string{e.in.user.ID.String(), e.in.user.Email, e.anaToken.ID, e.anaToken.Token, "ana's phone"} {
		if strings.Contains(body, s) {
			t.Errorf("%s shows ana's %q: %s", what, s, body)
		}
	}
}

// anasThingsAreIntact fails if ivo's request ended ana's session or
// token.
func (e isolationEnv) anasThingsAreIntact(t *testing.T) {
	t.Helper()
	if rec := e.in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{e.ana}, nil); rec.Code != http.StatusOK {
		t.Errorf("ana's session after ivo's request: %d, want 200", rec.Code)
	}
	if rec := e.in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(e.anaToken.Token)); rec.Code != http.StatusOK {
		t.Errorf("ana's token after ivo's request: %d, want 200", rec.Code)
	}
}

// isolationCases has one case for every operation of the API: it runs the
// operation as ivo against what ana owns, and nothing of ana's may show
// or change. TestIsolationCoversEveryOperation fails for an operation
// without a case, so a new endpoint cannot skip this.
func isolationCases() map[string]func(t *testing.T, e isolationEnv) {
	get := func(path string) func(t *testing.T, e isolationEnv) {
		return func(t *testing.T, e isolationEnv) {
			t.Helper()
			rec := e.in.do(t, http.MethodGet, path, "", []*http.Cookie{e.ivo}, nil)
			e.noneOfAnas(t, "GET "+path, rec.Body.String())
			e.anasThingsAreIntact(t)
		}
	}
	return map[string]func(t *testing.T, e isolationEnv){
		"get-info":         get("/api/v1/info"),
		"get-auth-methods": get("/api/v1/auth/methods"),
		"get-session":      get("/api/v1/auth/session"),
		"whoami":           get("/api/v1/auth/whoami"),
		"list-tokens":      get("/api/v1/tokens"),
		"login": func(t *testing.T, e isolationEnv) {
			t.Helper()
			rec := e.in.login(t, "ivo@example.com", password)
			e.noneOfAnas(t, "ivo's sign-in", rec.Body.String())
			e.anasThingsAreIntact(t)
		},
		"logout": func(t *testing.T, e isolationEnv) {
			t.Helper()
			e.in.do(t, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{e.ivo}, nil)
			e.anasThingsAreIntact(t)
		},
		"create-token": func(t *testing.T, e isolationEnv) {
			t.Helper()
			ivos, _ := e.in.createToken(t, e.ivo, `{"name":"ivo's","scopes":["read"]}`)
			rec := e.in.do(t, http.MethodGet, "/api/v1/tokens", "", []*http.Cookie{e.ana}, nil)
			if strings.Contains(rec.Body.String(), ivos.ID) {
				t.Errorf("ivo's new token is in ana's list: %s", rec.Body.String())
			}
			if who := e.in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(ivos.Token)); !strings.Contains(who.Body.String(), "ivo@example.com") {
				t.Errorf("ivo's new token acts for %s, want ivo", who.Body.String())
			}
		},
		"revoke-token": func(t *testing.T, e isolationEnv) {
			t.Helper()
			rec := e.in.do(t, http.MethodDelete, "/api/v1/tokens/"+e.anaToken.ID, "", []*http.Cookie{e.ivo}, nil)
			if rec.Code != http.StatusNotFound {
				t.Errorf("ivo revoking ana's token: %d, want 404, as for a token that does not exist", rec.Code)
			}
			e.anasThingsAreIntact(t)
		},
	}
}

func TestIsolation(t *testing.T) {
	t.Parallel()

	for op, run := range isolationCases() {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			run(t, newIsolationEnv(t))
		})
	}
}

// Every operation in the OpenAPI document needs an isolation case.
func TestIsolationCoversEveryOperation(t *testing.T) {
	t.Parallel()

	raw, err := api.Spec()
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("decode the spec: %v", err)
	}
	cases := isolationCases()
	var missing []string
	for path, ops := range spec.Paths {
		for method, op := range ops {
			if _, ok := cases[op.OperationID]; !ok {
				missing = append(missing, strings.ToUpper(method)+" "+path+" ("+op.OperationID+")")
			}
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		t.Errorf("operations without an isolation case in isolationCases:\n  %s", strings.Join(missing, "\n  "))
	}
}
