package api_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// exec runs stmt on the instance's database, to break it for a test.
func (in *instance) exec(t *testing.T, stmt string) {
	t.Helper()
	err := in.db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), stmt)
		return err
	})
	if err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

// logRecord is one JSON line of the instance's log.
type logRecord struct {
	Level       string `json:"level"`
	Msg         string `json:"msg"`
	Error       string `json:"error"`
	OperationID string `json:"operation_id"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	UserID      string `json:"user_id"`
}

func (in *instance) logRecords(t *testing.T) []logRecord {
	t.Helper()
	var out []logRecord
	sc := bufio.NewScanner(strings.NewReader(in.logs.String()))
	for sc.Scan() {
		var r logRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("decode the log line %s: %v", sc.Text(), err)
		}
		out = append(out, r)
	}
	return out
}

// An operation that fails inside the server answers 500 with no word of
// what failed, which could name a file, a query or a user; the log has
// it, once, with the request it came from.
func TestInternalErrorsStayInTheLog(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, op, method, path, body string
		signedIn                     bool
		// breakIt makes the operation fail; secret is what the failure
		// says, which the answer must not.
		breakIt func(t *testing.T, in *instance, session *http.Cookie) string
		secret  string
	}{
		{
			op: "login", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"email":"ana@example.com","password":"` + password + `"}`,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE users RENAME COLUMN password_hash TO gone")
				return ""
			}, secret: "password_hash",
		},
		{
			op: "logout", method: http.MethodPost, path: "/api/v1/auth/logout", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "DELETE", "sessions")
				return ""
			}, secret: "refused by the test",
		},
		{
			op: "list-tokens", method: http.MethodGet, path: "/api/v1/tokens", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE api_tokens RENAME TO gone")
				return ""
			}, secret: "no such table",
		},
		{
			op: "create-token", method: http.MethodPost, path: "/api/v1/tokens", body: `{"name":"phone","scopes":["read"]}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "api_tokens")
				return ""
			}, secret: "refused by the test",
		},
		{
			op: "revoke-token", method: http.MethodDelete, signedIn: true,
			breakIt: func(t *testing.T, in *instance, session *http.Cookie) string {
				t.Helper()
				created, rec := in.createToken(t, session, `{"name":"phone","scopes":["read"]}`)
				if rec.Code != http.StatusCreated {
					t.Fatalf("create a token: %d %s", rec.Code, rec.Body.String())
				}
				in.failWrites(t, "DELETE", "api_tokens")
				return "/api/v1/tokens/" + created.ID
			}, secret: "refused by the test",
		},
		{
			op: "get-profile", method: http.MethodGet, path: "/api/v1/profile", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE users RENAME COLUMN height_cm TO gone")
				return ""
			}, secret: "height_cm",
		},
		{
			op: "set-profile", method: http.MethodPut, path: "/api/v1/profile", body: `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "events")
				return ""
			}, secret: "refused by the test",
		},
		{
			name: "set-profile, the user's calendar",
			op:   "set-profile", method: http.MethodPut, path: "/api/v1/profile", body: `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "UPDATE users SET timezone = 'Nowhere/Bogus'")
				return ""
			}, secret: "Nowhere/Bogus",
		},
		{
			op: "get-limits", method: http.MethodGet, path: "/api/v1/limits", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE plan_settings RENAME TO gone")
				return ""
			}, secret: "plan_settings",
		},
		{
			op: "set-limits", method: http.MethodPut, path: "/api/v1/limits", body: `{"limits":{"cut_rate":-0.5}}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "events")
				return ""
			}, secret: "refused by the test",
		},
	} {
		name := tc.op
		if tc.name != "" {
			name = tc.name
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := newInstance(t, "http://localhost", passwordHash(t))
			session := sessionOf(t, in)
			var cookies []*http.Cookie
			if tc.signedIn {
				cookies = append(cookies, session)
			}
			path := tc.path
			if p := tc.breakIt(t, in, session); p != "" {
				path = p
			}
			rec := in.do(t, tc.method, path, tc.body, cookies, nil)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("%d %s, want 500", rec.Code, rec.Body.String())
			}
			var problem map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode %s: %v", rec.Body.String(), err)
			}
			if _, ok := problem["errors"]; ok || strings.Contains(rec.Body.String(), tc.secret) {
				t.Errorf("the answer %s tells what failed inside", rec.Body.String())
			}

			var logged []logRecord
			for _, r := range in.logRecords(t) {
				if strings.Contains(r.Error, tc.secret) {
					logged = append(logged, r)
				}
			}
			if len(logged) != 1 {
				t.Fatalf("the log has the failure %d times, want once:\n%s", len(logged), in.logs.String())
			}
			want := logRecord{Level: "ERROR", Msg: "an API operation failed", Error: logged[0].Error, OperationID: tc.op, Method: tc.method, Path: path}
			if tc.signedIn {
				want.UserID = in.user.ID.String()
			}
			if logged[0] != want {
				t.Errorf("logged %+v, want %+v", logged[0], want)
			}
		})
	}
}
