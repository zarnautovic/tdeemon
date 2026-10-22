package api_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
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

// testNow is the time of a new instance's clock.
var testNow = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

// knownFood is the id of a food a case saves, which its body names.
var knownFood = uuid.MustParse("01a16294-ab09-7d2f-8ab3-53f0c382f162")

// mealOf is the body of log_meal for a meal id of items.
func mealOf(id uuid.UUID, items ...string) string {
	return fmt.Sprintf(`{"id": %q, "items": [%s]}`, id, strings.Join(items, ", "))
}

// route is the path of the operation opID, as the document has it.
func route(t *testing.T, opID string) string {
	t.Helper()
	for _, op := range document().ops {
		if op.OperationID == opID {
			return op.Path
		}
	}
	t.Fatalf("no operation %s", opID)
	return ""
}

// checkLogged fails unless the log has a record that is want but for its
// error.
func (in *instance) checkLogged(t *testing.T, want logRecord) {
	t.Helper()
	for _, r := range in.logRecords(t) {
		want.Error = r.Error
		if r == want {
			return
		}
	}
	t.Errorf("the log has no %+v:\n%s", want, in.logs.String())
}

// checkLoggedOnce fails unless the log has one record whose error says
// secret, and it is want but for the error's text.
func (in *instance) checkLoggedOnce(t *testing.T, secret string, want logRecord) {
	t.Helper()
	var logged []logRecord
	for _, r := range in.logRecords(t) {
		if strings.Contains(r.Error, secret) {
			logged = append(logged, r)
		}
	}
	if len(logged) != 1 {
		t.Fatalf("the log has the failure %d times, want once:\n%s", len(logged), in.logs.String())
	}
	want.Error = logged[0].Error
	if logged[0] != want {
		t.Errorf("logged %+v, want %+v", logged[0], want)
	}
}

// mealFailCases fail a meal's update and delete at each step that reads
// or writes.
func mealFailCases() []failCase {
	var cases []failCase
	// A meal's update and delete, at each step that reads or writes.
	for _, b := range []struct{ name, method, body, stmt, inside string }{
		{"update-meal, the meal", http.MethodPatch, `{"note": "x"}`, "ALTER TABLE meals RENAME COLUMN note TO gone", "note"},
		{"update-meal, the change", http.MethodPatch, `{"note": "x"}`, "CREATE TRIGGER refuse BEFORE UPDATE ON meals BEGIN SELECT RAISE(ABORT, 'refused by the test'); END", "refused by the test"},
		{"update-meal, the day after", http.MethodPatch, `{"note": "x"}`, "ALTER TABLE targets RENAME TO gone", "targets"},
		{"update-meal, the user's calendar", http.MethodPatch, `{"eaten_at": "2026-10-21T07:00:00Z"}`, "UPDATE users SET timezone = 'Nowhere/Bogus'", "Nowhere/Bogus"},
		{"delete-meal, the meal", http.MethodDelete, "", "ALTER TABLE meals RENAME COLUMN note TO gone", "note"},
		{"delete-meal, the delete", http.MethodDelete, "", "CREATE TRIGGER refuse BEFORE DELETE ON meals BEGIN SELECT RAISE(ABORT, 'refused by the test'); END", "refused by the test"},
		{"delete-meal, the day after", http.MethodDelete, "", "ALTER TABLE targets RENAME TO gone", "targets"},
	} {
		cases = append(cases, failCase{
			name: b.name, op: strings.SplitN(b.name, ",", 2)[0], method: b.method, body: b.body, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				id := saveIsolationMeals(t, in.db, in.user, saveIsolationFoods(t, in.db, in.user, in.clock.Now()), in.clock.Now())
				in.exec(t, b.stmt)
				return "/api/v1/meals/" + id.String()
			}, inside: b.inside,
		})
	}
	return cases
}

// toolFailCases fail the tools that read, at each place they read from.
func toolFailCases() []failCase {
	var cases []failCase
	// The tools read from many places: each read that fails is a 500.
	for _, b := range []struct{ op, path, stmt, secret string }{
		{"get-me", "/api/v1/me", "ALTER TABLE users RENAME COLUMN height_cm TO gone", "height_cm"},
		{"get-me", "/api/v1/me", "ALTER TABLE phases RENAME TO gone", "phases"},
		{"get-me", "/api/v1/me", "ALTER TABLE targets RENAME TO gone", "targets"},
		{"get-me", "/api/v1/me", "ALTER TABLE profile_notes RENAME TO gone", "profile_notes"},
		{"get-me", "/api/v1/me", "ALTER TABLE plan_settings RENAME COLUMN training_weekdays TO gone", "training_weekdays"},
		{"get-me", "/api/v1/me", "ALTER TABLE days RENAME COLUMN training_day TO gone", "training_day"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE meals RENAME TO gone", "no such table: meals"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE metrics RENAME TO gone", "no such table: metrics"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE days RENAME COLUMN excluded_reason TO gone", "excluded_reason"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE days RENAME COLUMN training_day TO gone", "training_day"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE workout_days RENAME TO gone", "workout_days"},
		{"get-me", "/api/v1/me", "ALTER TABLE workout_days RENAME TO gone", "workout_days"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE targets RENAME TO gone", "targets"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE plan_settings RENAME COLUMN training_weekdays TO gone", "training_weekdays"},
		{"get-me", "/api/v1/me", "UPDATE users SET timezone = 'Nowhere/Bogus'", "Nowhere/Bogus"},
		{"get-day", "/api/v1/days/today", "UPDATE users SET timezone = 'Nowhere/Bogus'", "Nowhere/Bogus"},
		{"get-day", "/api/v1/days/today", "ALTER TABLE days RENAME COLUMN mood TO gone", "mood"},
	} {
		cases = append(cases, failCase{
			name: b.op + ", " + b.secret, op: b.op, method: http.MethodGet, path: b.path, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, b.stmt)
				return ""
			}, inside: b.secret,
		})
	}
	return append(cases, dayFailCases()...)
}

// dayFailCases are the 500s of the tools that change a day: each write
// that fails, and each read of the day they answer.
func dayFailCases() []failCase {
	var cases []failCase
	for _, b := range []struct{ op, method, path, body, stmt, secret string }{
		{"log-metric", http.MethodPut, "/api/v1/days/today/metrics/weight", `{"value": 80}`, "ALTER TABLE metrics RENAME TO gone", "metrics"},
		{"log-metric", http.MethodPut, "/api/v1/days/today/metrics/weight", `{"value": 80}`, "ALTER TABLE meals RENAME TO gone", "meals"},
		{"log-metric", http.MethodPut, "/api/v1/days/today/metrics/weight", `{"value": 80}`, "UPDATE users SET timezone = 'Nowhere/Bogus'", "Nowhere/Bogus"},
		{"delete-metric", http.MethodDelete, "/api/v1/days/today/metrics/weight", "", "ALTER TABLE metrics RENAME TO gone", "metrics"},
		{"delete-metric", http.MethodDelete, "/api/v1/days/today/metrics/weight", "", "UPDATE users SET timezone = 'Nowhere/Bogus'", "Nowhere/Bogus"},
		{"check-in", http.MethodPatch, "/api/v1/days/today", `{"mood": 3}`, "ALTER TABLE days RENAME COLUMN mood TO gone", "mood"},
		{"check-in", http.MethodPatch, "/api/v1/days/today", `{"mood": 3}`, "ALTER TABLE events RENAME TO gone", "events"},
		{"check-in", http.MethodPatch, "/api/v1/days/today", `{"mood": 3}`, "ALTER TABLE meals RENAME TO gone", "meals"},
		{"check-in", http.MethodPatch, "/api/v1/days/today", `{"mood": 3}`, "UPDATE users SET timezone = 'Nowhere/Bogus'", "Nowhere/Bogus"},
		{"undo-last", http.MethodPost, "/api/v1/undo", `{"id": "` + clientid.NewAt(testNow).String() + `"}`, "ALTER TABLE events RENAME COLUMN undo_request TO gone", "undo_request"},
	} {
		cases = append(cases, failCase{
			name: b.op + ", " + b.secret, op: b.op, method: b.method, path: b.path, body: b.body, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, b.stmt)
				return ""
			}, inside: b.secret,
		})
	}
	return cases
}

// failCase is a request that fails inside the server, and how.
type failCase struct {
	name, op, method, path, body string
	// signedIn sends the request with a session, withToken with an
	// API token of the ingest scope.
	signedIn, withToken bool
	// breakIt makes the operation fail; inside is what the failure
	// says, which the answer must not.
	breakIt func(t *testing.T, in *instance, session *http.Cookie) string
	inside  string
}

// An operation that fails inside the server answers 500 with no word of
// what failed, which could name a file, a query or a user; the log has
// it, once, with the request it came from.
func TestInternalErrorsStayInTheLog(t *testing.T) {
	t.Parallel()

	cases := []failCase{
		{
			op: "login", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"email":"ana@example.com","password":"` + password + `"}`,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE users RENAME COLUMN password_hash TO gone")
				return ""
			}, inside: "password_hash",
		},
		{
			op: "logout", method: http.MethodPost, path: "/api/v1/auth/logout", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "DELETE", "sessions")
				return ""
			}, inside: "refused by the test",
		},
		{
			op: "list-tokens", method: http.MethodGet, path: "/api/v1/tokens", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE api_tokens RENAME TO gone")
				return ""
			}, inside: "no such table",
		},
		{
			op: "create-token", method: http.MethodPost, path: "/api/v1/tokens", body: `{"name":"phone","scopes":["read"]}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "api_tokens")
				return ""
			}, inside: "refused by the test",
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
			}, inside: "refused by the test",
		},
		{
			op: "get-profile", method: http.MethodGet, path: "/api/v1/profile", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE users RENAME COLUMN height_cm TO gone")
				return ""
			}, inside: "height_cm",
		},
		{
			op: "set-profile", method: http.MethodPut, path: "/api/v1/profile", body: `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "events")
				return ""
			}, inside: "refused by the test",
		},
		{
			name: "set-profile, the user's calendar",
			op:   "set-profile", method: http.MethodPut, path: "/api/v1/profile", body: `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "UPDATE users SET timezone = 'Nowhere/Bogus'")
				return ""
			}, inside: "Nowhere/Bogus",
		},
		{
			op: "ingest-metrics", method: http.MethodPost, path: "/api/v1/ingest/metrics", body: `{"date":"2026-10-21","metrics":{"weight":77.7}}`, withToken: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE metrics RENAME TO gone")
				return ""
			}, inside: "no such table: metrics",
		},
		{
			op: "ingest-hae", method: http.MethodPost, path: "/api/v1/ingest/hae",
			body:      `{"data":{"metrics":[{"name":"step_count","units":"count","data":[{"date":"2026-10-20 00:00:00 +0200","qty":7300}]}]}}`,
			withToken: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "metrics")
				return ""
			}, inside: "refused by the test",
		},
		{
			op: "ingest-strong", method: http.MethodPost, path: "/api/v1/ingest/strong",
			body:      "Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n2026-10-21 06:55:30,Day B,58m,Deadlift,1,120,3,0\n",
			withToken: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "workout_days")
				return ""
			}, inside: "refused by the test",
		},
		{
			name: "find-foods, the foods", op: "find-foods", method: http.MethodGet, path: "/api/v1/foods?query=rye", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				saveIsolationFoods(t, in.db, in.user, in.clock.Now())
				in.exec(t, "ALTER TABLE foods RENAME COLUMN portion_g TO gone")
				return ""
			}, inside: "portion_g",
		},
		{
			name: "find-foods, the last uses", op: "find-foods", method: http.MethodGet, path: "/api/v1/foods?query=rye", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				saveIsolationFoods(t, in.db, in.user, in.clock.Now())
				in.exec(t, "ALTER TABLE meal_items RENAME TO gone")
				return ""
			}, inside: "meal_items",
		},
		{
			name: "log-meal, the meal of the id", op: "log-meal", method: http.MethodPost, path: "/api/v1/meals", signedIn: true,
			body: mealOf(meal.NewIDAt(testNow), `{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`),
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE meals RENAME COLUMN note TO gone")
				return ""
			}, inside: "note",
		},
		{
			name: "log-meal, a food by id", op: "log-meal", method: http.MethodPost, path: "/api/v1/meals", signedIn: true,
			body: mealOf(meal.NewIDAt(testNow), `{"food_id": "`+knownFood.String()+`", "amount": 50, "unit": "g"}`),
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				f := food.Food{ID: knownFood, Name: "Rye roll", Basis: food.Per100g, Source: food.Label, Nutrients: food.Nutrients{Kcal: 250, Protein: 8, Carbs: 46, Fat: 3}}
				if _, err := in.db.SaveFood(t.Context(), in.user.ID, isolationActor, f, in.clock.Now()); err != nil {
					t.Fatal(err)
				}
				in.exec(t, "ALTER TABLE foods RENAME COLUMN portion_g TO gone")
				return ""
			}, inside: "portion_g",
		},
		{
			name: "log-meal, a new food", op: "log-meal", method: http.MethodPost, path: "/api/v1/meals", signedIn: true,
			body: mealOf(meal.NewIDAt(testNow), `{"food": {"name": "Skyr", "basis": "100g", "source": "label", "nutrients": {"kcal": 63, "protein": 11, "carbs": 4, "fat": 0.2}}, "amount": 150, "unit": "g"}`),
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "foods")
				return ""
			}, inside: "refused by the test",
		},
		{
			name: "log-meal, the meal", op: "log-meal", method: http.MethodPost, path: "/api/v1/meals", signedIn: true,
			body: mealOf(meal.NewIDAt(testNow), `{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`),
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "meals")
				return ""
			}, inside: "refused by the test",
		},
		{
			name: "log-meal, the day after", op: "log-meal", method: http.MethodPost, path: "/api/v1/meals", signedIn: true,
			body: mealOf(meal.NewIDAt(testNow), `{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`),
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE targets RENAME TO gone")
				return ""
			}, inside: "targets",
		},
		{
			name: "log-meal, the user's calendar", op: "log-meal", method: http.MethodPost, path: "/api/v1/meals", signedIn: true,
			body: mealOf(meal.NewIDAt(testNow), `{"name": "Soup", "nutrients": {"kcal": 180, "protein": 6, "carbs": 20, "fat": 8}}`),
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "UPDATE users SET timezone = 'Nowhere/Bogus'")
				return ""
			}, inside: "Nowhere/Bogus",
		},
		{
			op: "get-food", method: http.MethodGet, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				bread := saveIsolationFoods(t, in.db, in.user, in.clock.Now())
				in.exec(t, "ALTER TABLE foods RENAME COLUMN portion_g TO gone")
				return "/api/v1/foods/" + bread.ID.String()
			}, inside: "portion_g",
		},
		{
			op: "upsert-food", method: http.MethodPost, path: "/api/v1/foods", signedIn: true,
			body: `{"name": "Skyr", "basis": "100g", "source": "label", "nutrients": {"kcal": 63, "protein": 11, "carbs": 4, "fat": 0.2}}`,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "foods")
				return ""
			}, inside: "refused by the test",
		},
		{
			name: "ingest-strong, the user's calendar",
			op:   "ingest-strong", method: http.MethodPost, path: "/api/v1/ingest/strong",
			body:      "Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n2026-10-21 06:55:30,Day B,58m,Deadlift,1,120,3,0\n",
			withToken: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "UPDATE users SET timezone = 'Nowhere/Bogus'")
				return ""
			}, inside: "Nowhere/Bogus",
		},
		{
			op: "get-limits", method: http.MethodGet, path: "/api/v1/limits", signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.exec(t, "ALTER TABLE plan_settings RENAME TO gone")
				return ""
			}, inside: "plan_settings",
		},
		{
			op: "set-limits", method: http.MethodPut, path: "/api/v1/limits", body: `{"limits":{"cut_rate":-0.5}}`, signedIn: true,
			breakIt: func(t *testing.T, in *instance, _ *http.Cookie) string {
				t.Helper()
				in.failWrites(t, "INSERT", "events")
				return ""
			}, inside: "refused by the test",
		},
	}
	cases = append(cases, mealFailCases()...)
	cases = append(cases, toolFailCases()...)
	for _, tc := range cases {
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
			var header map[string]string
			if tc.withToken {
				header = ingestToken(t, in, `"ingest"`)
			}
			rec := in.do(t, tc.method, path, tc.body, cookies, header)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("%d %s, want 500", rec.Code, rec.Body.String())
			}
			var problem map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode %s: %v", rec.Body.String(), err)
			}
			if _, ok := problem["errors"]; ok || strings.Contains(rec.Body.String(), tc.inside) {
				t.Errorf("the answer %s tells what failed inside", rec.Body.String())
			}

			want := logRecord{Level: "ERROR", Msg: "an API operation failed", OperationID: tc.op, Method: tc.method, Path: route(t, tc.op)}
			if tc.signedIn || tc.withToken {
				want.UserID = in.user.ID.String()
			}
			in.checkLoggedOnce(t, tc.inside, want)
		})
	}
}

// A client that leaves while its sign-in reads the credentials gets no
// error logged: it says nothing about the server, and a client can do it
// at will.
func TestGoneSignInLogsAtDebug(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	body := `{"email":"ana@example.com","password":"` + password + `"}`
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := in.serve(t, r)
	var failed []logRecord
	for _, l := range in.logRecords(t) {
		if l.Msg == "an API operation failed" {
			failed = append(failed, l)
		}
	}
	if rec.Code != http.StatusInternalServerError || len(failed) != 1 || failed[0].Level != "DEBUG" || !strings.Contains(failed[0].Error, "context canceled") {
		t.Errorf("%d %s, logged %+v; want a 500 logged once at debug level", rec.Code, rec.Body.String(), failed)
	}
}

// A body cut short, which huma answers with a 500 before any handler or
// limit runs, is logged at debug level: anyone can send one at will, and
// it says nothing about the server.
func TestCutBodiesLogAtDebug(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", passwordHash(t))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/login", iotest.ErrReader(io.ErrUnexpectedEOF))
	r.Header.Set("Content-Type", "application/json")
	rec := in.serve(t, r)
	var failed []logRecord
	for _, l := range in.logRecords(t) {
		if l.Msg == "an API operation failed" {
			failed = append(failed, l)
		}
	}
	if rec.Code != http.StatusInternalServerError || len(failed) != 1 || failed[0].Level != "DEBUG" {
		t.Errorf("%d %s, logged %+v; want a 500 logged once at debug level", rec.Code, rec.Body.String(), failed)
	}
}
