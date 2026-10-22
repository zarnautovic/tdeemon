package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// document is the API's description: the operations by the route they
// answer on, and the registry of the schemas they refer to. It is the
// committed document's, as TestOpenAPISpecIsCurrent holds.
var document = sync.OnceValue(func() documentRoutes {
	a := api.SpecAPI()
	d := documentRoutes{mux: http.NewServeMux(), ops: map[string]*huma.Operation{}, registry: a.OpenAPI().Components.Schemas}
	for path, item := range a.OpenAPI().Paths {
		for method, op := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPost: item.Post, http.MethodPut: item.Put,
			http.MethodPatch: item.Patch, http.MethodDelete: item.Delete,
		} {
			if op != nil {
				pattern := method + " " + path
				d.mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
				d.ops[pattern] = op
			}
		}
	}
	return d
})

type documentRoutes struct {
	mux      *http.ServeMux
	ops      map[string]*huma.Operation
	registry huma.Registry
}

// operation returns the operation the document has for r, if any.
func (d documentRoutes) operation(r *http.Request) *huma.Operation {
	_, pattern := d.mux.Handler(r)
	return d.ops[pattern]
}

// checkAnswer fails unless rec, the answer to r, is one the document
// describes: its status is among the operation's responses (or the
// operation has a default response), and its body
// is valid against the schema for its content type, all the way down
// through every $ref, array and object: the types, the required
// properties, no property the document leaves out. A response type the
// document misdescribes (a field huma skips, a nil slice that comes out
// as null) shows here, where the generated TypeScript types would not.
// It returns the operation's ID; an answer from no operation (an unknown
// path, a method no operation has, a HEAD request) is not checked, and
// gets "".
func checkAnswer(tb testing.TB, r *http.Request, rec *httptest.ResponseRecorder) string {
	tb.Helper()
	if r.Method == http.MethodHead {
		return ""
	}
	op := document().operation(r)
	if op == nil {
		return ""
	}
	checkBody(tb, op, rec)
	return op.OperationID
}

// checkBody is checkAnswer's check of rec, an answer of op.
func checkBody(tb testing.TB, op *huma.Operation, rec *httptest.ResponseRecorder) {
	tb.Helper()
	resp, ok := op.Responses[strconv.Itoa(rec.Code)]
	if !ok {
		resp, ok = op.Responses["default"]
	}
	if !ok {
		tb.Errorf("%s answered %d, which the document does not list: %s", op.OperationID, rec.Code, rec.Body.String())
		return
	}
	if len(resp.Content) == 0 {
		if rec.Body.Len() != 0 {
			tb.Errorf("%s %d: a body the document has none for: %s", op.OperationID, rec.Code, rec.Body.String())
		}
		return
	}
	ct, _, err := mime.ParseMediaType(rec.Header().Get("Content-Type"))
	if err != nil {
		tb.Errorf("%s %d: Content-Type %q: %v", op.OperationID, rec.Code, rec.Header().Get("Content-Type"), err)
		return
	}
	media, ok := resp.Content[ct]
	if !ok {
		tb.Errorf("%s %d: Content-Type %s, which the document does not list", op.OperationID, rec.Code, ct)
		return
	}
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		tb.Errorf("%s %d: decode %s: %v", op.OperationID, rec.Code, rec.Body.String(), err)
		return
	}
	res := &huma.ValidateResult{}
	huma.Validate(document().registry, media.Schema, huma.NewPathBuffer([]byte{}, 0), huma.ModeReadFromServer, v, res)
	for _, e := range res.Errors {
		tb.Errorf("%s %d: the answer does not match the document: %v\n%s", op.OperationID, rec.Code, e, rec.Body.String())
	}
}

// answerCase makes requests of one operation, at least one of which
// succeeds, with every field of the answer that can be there.
type answerCase func(t *testing.T, in *instance, session *http.Cookie)

// answerCases has a case for every operation in the document.
func answerCases() map[string]answerCase {
	get := func(path string) answerCase {
		return func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			in.do(t, http.MethodGet, path, "", []*http.Cookie{session}, nil)
		}
	}
	return map[string]answerCase{
		"get-info":         get("/api/v1/info"),
		"get-auth-methods": get("/api/v1/auth/methods"),
		"get-session":      get("/api/v1/auth/session"),
		"whoami":           get("/api/v1/auth/whoami"),
		"get-limits":       get("/api/v1/limits"),
		"ingest-hae": func(t *testing.T, in *instance, _ *http.Cookie) {
			t.Helper()
			in.do(t, http.MethodPost, "/api/v1/ingest/hae", haeBody, nil, ingestToken(t, in, `"ingest"`))
		},
		"ingest-metrics": func(t *testing.T, in *instance, _ *http.Cookie) {
			t.Helper()
			in.do(t, http.MethodPost, "/api/v1/ingest/metrics", `{"date":"2026-10-21","metrics":{"weight":77.7,"sleep_h":30,"typo":1}}`, nil, ingestToken(t, in, `"ingest"`))
		},
		"get-day": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			now := in.clock.Now()
			saveIsolationMeals(t, in.db, in.user, saveIsolationFoods(t, in.db, in.user, now), now)
			saveIsolationMetrics(t, in.db, in.user, now)
			saveIsolationPlan(t, in.db, in.user, now)
			// Today, and the excluded day before.
			in.do(t, http.MethodGet, "/api/v1/days/today", "", []*http.Cookie{session}, nil)
			in.do(t, http.MethodGet, "/api/v1/days/"+day.DateOf(now).AddDays(-1).String(), "", []*http.Cookie{session}, nil)
		},
		"find-foods": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			now := in.clock.Now()
			saveIsolationMeals(t, in.db, in.user, saveIsolationFoods(t, in.db, in.user, now), now)
			in.do(t, http.MethodGet, "/api/v1/foods?query=rye", "", []*http.Cookie{session}, nil)
		},
		"upsert-food": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			in.do(t, http.MethodPost, "/api/v1/foods", `{"name": "Rolls", "brand": "Bakery", "basis": "100g", "source": "label",
				"nutrients": {"kcal": 262.5, "protein": 8.75, "carbs": 50, "fat": 2.5, "fiber": 3, "saturated_fat": 0.4, "sugars": 2, "salt": 0.9, "polyols": 0, "alcohol": 0},
				"servings": [{"name": "roll", "amount": 80}], "aliases": ["morning rolls"], "note": "from the bakery"}`, []*http.Cookie{session}, nil)
			// A portion of the canteen's, which weighs what it says.
			in.do(t, http.MethodPost, "/api/v1/foods", `{"name": "Canteen goulash", "basis": "portion", "source": "estimate", "portion_grams": 450,
				"nutrients": {"kcal": 650, "protein": 40, "carbs": 50, "fat": 30}}`, []*http.Cookie{session}, nil)
		},
		"get-me": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			saveIsolationPlan(t, in.db, in.user, in.clock.Now())
			in.do(t, http.MethodGet, "/api/v1/me", "", []*http.Cookie{session}, nil)
		},
		"get-profile": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			// Empty, then whole.
			in.do(t, http.MethodGet, "/api/v1/profile", "", []*http.Cookie{session}, nil)
			in.do(t, http.MethodPut, "/api/v1/profile", `{"sex":"female","birth_date":"1990-06-15","height_cm":168.5}`, []*http.Cookie{session}, nil)
			in.do(t, http.MethodGet, "/api/v1/profile", "", []*http.Cookie{session}, nil)
		},
		"set-profile": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			in.do(t, http.MethodPut, "/api/v1/profile", `{"sex":"male","birth_date":"1985-01-20","height_cm":181}`, []*http.Cookie{session}, nil)
		},
		"set-limits": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			in.do(t, http.MethodPut, "/api/v1/limits", `{"limits":{"cut_rate":-0.5}}`, []*http.Cookie{session}, nil)
		},
		"list-tokens": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			// A token that was used, so the list has every field.
			created, _ := in.createToken(t, session, `{"name":"phone","scopes":["read"]}`)
			in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(created.Token))
			in.do(t, http.MethodGet, "/api/v1/tokens", "", []*http.Cookie{session}, nil)
		},
		"create-token": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			in.createToken(t, session, `{"name":"laptop","scopes":["read","tools"]}`)
		},
		"revoke-token": func(t *testing.T, in *instance, session *http.Cookie) {
			t.Helper()
			created, _ := in.createToken(t, session, `{"name":"old","scopes":["read"]}`)
			in.do(t, http.MethodDelete, "/api/v1/tokens/"+created.ID, "", []*http.Cookie{session}, nil)
		},
		"login": func(t *testing.T, in *instance, _ *http.Cookie) {
			t.Helper()
			in.login(t, "ana@example.com", password)
		},
		"logout": func(t *testing.T, in *instance, _ *http.Cookie) {
			t.Helper()
			c := sessionCookie(t, in.login(t, "ana@example.com", password))
			in.do(t, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{c}, nil)
		},
	}
}

// Every operation answers what the document says it does: serve checks
// each answer (checkAnswer), and each case must get a success of its
// operation checked.
func TestAnswersMatchTheDocument(t *testing.T) {
	t.Parallel()

	for op, run := range answerCases() {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			in, session := signedIn(t)
			run(t, in, session)
			if !in.checked(op) {
				t.Errorf("no success of %s was checked against the document", op)
			}
		})
	}
}

// TestAnswersCoverEveryOperation fails for an operation in the document
// without a case in answerCases, so a new operation adds its case.
func TestAnswersCoverEveryOperation(t *testing.T) {
	t.Parallel()

	cases := answerCases()
	var missing, stale []string
	have := map[string]bool{}
	for _, op := range document().ops {
		have[op.OperationID] = true
		if _, ok := cases[op.OperationID]; !ok {
			missing = append(missing, op.OperationID)
		}
	}
	for op := range cases {
		if !have[op] {
			stale = append(stale, op)
		}
	}
	slices.Sort(missing)
	slices.Sort(stale)
	if len(missing) > 0 {
		t.Errorf("operations without a case in answerCases:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("cases in answerCases for operations the API no longer has: %s", strings.Join(stale, ", "))
	}
}

// errorsOf is a testing.TB that keeps what a check reports.
type errorsOf struct {
	testing.TB
	errs []string
}

func (e *errorsOf) Errorf(format string, args ...any) {
	e.errs = append(e.errs, fmt.Sprintf(format, args...))
}

// checkAnswer finds what the document does not describe, also deep in
// the answer.
func TestCheckAnswer(t *testing.T) {
	t.Parallel()

	const good = `{"tokens":[{"id":"01a16294-ab09-7d2f-8ab3-53f0c382f162","name":"phone","scopes":["read"],"created_at":"2026-10-21T08:00:00Z"}]}`
	for _, tc := range []struct {
		name, method, path, contentType, body string
		status                                int
		want                                  string // in the report; "" for none
	}{
		{name: "a good answer", body: good},
		{name: "an empty list", body: `{"tokens":[]}`},
		{name: "a list that is null", body: `{"tokens":null}`, want: "expected array"},
		{name: "a property the document leaves out", body: `{"tokens":[],"next":1}`, want: "unexpected property"},
		{name: "a required property missing, deep down", body: strings.Replace(good, `"name":"phone",`, "", 1), want: "expected required property name"},
		{name: "a wrong type, deep down", body: strings.Replace(good, `["read"]`, `"read"`, 1), want: "expected array"},
		{name: "a value the document does not allow, deep down", body: strings.Replace(good, `["read"]`, `["admin"]`, 1), want: "expected value to be one of"},
		{name: "a status the document does not list", status: http.StatusTeapot, body: good, want: "does not list"},
		{name: "a content type the document does not list", contentType: "text/plain", body: good, want: "does not list"},
		{name: "a problem", status: http.StatusUnauthorized, contentType: "application/problem+json", body: `{"title":"Unauthorized","status":401,"detail":"sign in first"}`},
		{name: "a body where the document has none", method: http.MethodDelete, path: "/api/v1/tokens/x", status: http.StatusNoContent, body: "{}", want: "has none"},
		{name: "no operation", path: "/api/v1/typo", status: http.StatusNotFound, body: "<html>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			method, path, status, ct := http.MethodGet, "/api/v1/tokens", http.StatusOK, "application/json"
			if tc.method != "" {
				method = tc.method
			}
			if tc.path != "" {
				path = tc.path
			}
			if tc.status != 0 {
				status = tc.status
			}
			if tc.contentType != "" {
				ct = tc.contentType
			}
			rec := httptest.NewRecorder()
			rec.Header().Set("Content-Type", ct)
			rec.WriteHeader(status)
			_, _ = rec.WriteString(tc.body)
			e := &errorsOf{TB: t}
			checkAnswer(e, httptest.NewRequestWithContext(t.Context(), method, path, nil), rec)
			got := strings.Join(e.errs, "\n")
			if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
				t.Errorf("reported %q, want %q", got, tc.want)
			}
		})
	}
}

// nullableFields returns the fields of t, and of every type of this
// module within it, that can come out as null, or that the document says
// can although they never do:
//   - a pointer without omitempty, which huma describes as required and
//     not null;
//   - a slice or a map of pointers, whose elements huma describes as not
//     null;
//   - a slice that does not say nullable:"false", which huma describes as
//     null or an array, even with omitempty.
//
// A map comes out as null only when it is nil; the document says it
// cannot, and checkAnswer holds every answer to that. huma's own types
// (the problem's ErrorModel) are its affair.
func nullableFields(t reflect.Type, seen map[reflect.Type]bool) []string {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	// An answer's body is often a struct with no name, and no package.
	foreign := t.Name() != "" && !strings.HasPrefix(t.PkgPath(), "github.com/zarnautovic/tdeemon/")
	if t.Kind() != reflect.Struct || seen[t] || foreign {
		return nil
	}
	seen[t] = true
	var out []string
	for f := range slices.Values(reflect.VisibleFields(t)) {
		switch {
		case len(f.Index) > 1:
			continue // promoted: the embedded struct's own
		case f.Anonymous:
			out = append(out, nullableFields(f.Type, seen)...)
			continue
		case !f.IsExported() || f.Tag.Get("json") == "-":
			continue
		}
		if why := nullable(f); why != "" {
			out = append(out, t.String()+"."+f.Name+": "+why)
		}
		out = append(out, nullableFields(f.Type, seen)...)
	}
	return out
}

// nullable says why the field f can be null, or "" when it cannot.
func nullable(f reflect.StructField) string {
	omitempty := slices.Contains(strings.Split(f.Tag.Get("json"), ",")[1:], "omitempty")
	k := f.Type.Kind()
	switch {
	case k == reflect.Pointer && !omitempty:
		return "a pointer without omitempty"
	case (k == reflect.Slice || k == reflect.Map) && f.Type.Elem().Kind() == reflect.Pointer:
		return "a collection of pointers"
	case k == reflect.Slice && f.Tag.Get("nullable") != "false":
		return `a slice without nullable:"false"`
	}
	return ""
}

// bodyType returns the Go type of an answer's body. A body is a struct,
// which the document refers to: one that is not (a bare slice, whose
// document huma lets be null) is an error.
func bodyType(registry huma.Registry, media *huma.MediaType) (reflect.Type, error) {
	if media.Schema == nil || media.Schema.Ref == "" {
		return nil, errors.New("a body that is no struct: make it one, with its fields")
	}
	typ := registry.TypeFromRef(media.Schema.Ref)
	if typ == nil {
		return nil, fmt.Errorf("no Go type for %s", media.Schema.Ref)
	}
	return typ, nil
}

// No answer of the API can hold a null the document does not allow,
// whatever the data: the rule is in the types of the answers.
func TestAnswersHoldNoNull(t *testing.T) {
	t.Parallel()

	seen := map[reflect.Type]bool{}
	var bad []string
	for _, op := range document().ops {
		for _, resp := range op.Responses {
			for _, media := range resp.Content {
				typ, err := bodyType(document().registry, media)
				if err != nil {
					t.Errorf("%s: %v", op.OperationID, err)
					continue
				}
				bad = append(bad, nullableFields(typ, seen)...)
			}
		}
	}
	slices.Sort(bad)
	if len(bad) > 0 {
		t.Errorf("fields of answers that can be null:\n  %s", strings.Join(bad, "\n  "))
	}
}

// nullableFields finds the fields that can be null, also deep down.
func TestNullableFields(t *testing.T) {
	t.Parallel()

	type inner struct {
		Fine     []string           `json:"fine" nullable:"false"`
		Optional []string           `json:"optional,omitempty" nullable:"false"`
		Skipped  []string           `json:"skipped,omitempty"`
		Values   map[string]string  `json:"values"`
		List     []string           `json:"list"`
		Pointers []*string          `json:"pointers" nullable:"false"`
		ByName   map[string]*string `json:"by_name"`
		Absent   *string            `json:"absent,omitempty"`
	}
	type Embedded struct {
		Tags []string `json:"tags"`
	}
	type outer struct {
		Embedded
		Body struct {
			Problem huma.ErrorModel `json:"problem"`
			Names   []string        `json:"names"`
		} `json:"body"`
		Inner  *inner   `json:"inner"`
		Deep   []inner  `json:"deep" nullable:"false"`
		Hidden *string  `json:"-"`
		hidden *string  //nolint:unused // a field the check must pass over
		Fine   *inner   `json:"fine,omitempty"`
		Count  *float64 `json:"count"`
	}
	got := nullableFields(reflect.TypeFor[outer](), map[reflect.Type]bool{})
	slices.Sort(got)
	want := []string{
		"api_test.Embedded.Tags: a slice without nullable:\"false\"",
		"api_test.inner.ByName: a collection of pointers",
		"api_test.inner.List: a slice without nullable:\"false\"",
		"api_test.inner.Pointers: a collection of pointers",
		"api_test.inner.Skipped: a slice without nullable:\"false\"",
		"api_test.outer.Count: a pointer without omitempty",
		"api_test.outer.Inner: a pointer without omitempty",
		"struct { Problem huma.ErrorModel \"json:\\\"problem\\\"\"; Names []string \"json:\\\"names\\\"\" }.Names: a slice without nullable:\"false\"",
	}
	if !slices.Equal(got, want) {
		t.Errorf("nullableFields = %q, want %q", got, want)
	}
}

// requestProblem is a request that goes wrong, and the status it must get:
// 0 for any the document lists.
type requestProblem struct {
	name string
	req  *http.Request
	want int
}

// floodBadTokens sends bad tokens from addr until it gets a 429.
func floodBadTokens(t *testing.T, in *instance, addr string) {
	t.Helper()
	for i := range 100 {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/auth/whoami", nil)
		r.RemoteAddr = addr
		r.Header.Set("Authorization", "Bearer "+auth.TokenPrefix+"bogus")
		if rec := in.serve(t, r); rec.Code == http.StatusTooManyRequests {
			return
		} else if i == 99 {
			t.Fatalf("no 429 after %d bad tokens: %d", i+1, rec.Code)
		}
	}
}

// bodyProblems are the bodies op can get wrong, as the operation takes
// one: from a token (with every scope) when it is open to them, up to its
// own limit. A body huma does not read (SkipValidateBody) is the
// operation's to refuse, with a problem it lists.
func bodyProblems(op *huma.Operation, token string, request func(io.Reader, map[string]string) *http.Request) []requestProblem {
	asJSON := map[string]string{"Content-Type": "application/json"}
	if api.OpenToTokens(op) {
		asJSON["Authorization"] = "Bearer " + token
	}
	other := maps.Clone(asJSON)
	other["Content-Type"] = "application/x-www-form-urlencoded"
	limit := op.MaxBodyBytes
	if limit == 0 {
		limit = 1 << 20
	}
	notJSON, otherType := http.StatusBadRequest, http.StatusUnsupportedMediaType
	if op.SkipValidateBody {
		notJSON, otherType = 0, 0
	}
	return []requestProblem{
		{"a body that is not JSON", request(strings.NewReader("{"), asJSON), notJSON},
		{"another content type", request(strings.NewReader("a=b"), other), otherType},
		{"a body too large", request(strings.NewReader(`"`+strings.Repeat("a", int(limit))+`"`), asJSON), http.StatusRequestEntityTooLarge},
		{"a body too slow", request(iotest.ErrReader(os.ErrDeadlineExceeded), asJSON), http.StatusRequestTimeout},
	}
}

// The problems answered before an operation's handler runs are in the
// document, for every operation: huma's (an unknown query parameter; a
// body that is not JSON, comes with another content type, is too large or
// too slow) and the server's own (a bad API token, an address that sent
// too many, a browser's cross-origin request that is not a read). serve
// checks each answer (checkAnswer).
func TestRequestProblemsAreDocumented(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	// An address that sent too many bad tokens.
	const flooding = "192.0.2.99:1234"
	floodBadTokens(t, in, flooding)
	// A token that may only read, which the operations not open to it
	// refuse.
	reader, rec := in.createToken(t, session, `{"name":"reader","scopes":["read"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create a token: %d %s", rec.Code, rec.Body.String())
	}
	// A token with every scope, for the bodies of the operations open to
	// tokens (an import takes nothing else).
	every, rec := in.createToken(t, session, `{"name":"every","scopes":["ingest","read","tools"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create a token: %d %s", rec.Code, rec.Body.String())
	}
	type problem = requestProblem
	request := func(t *testing.T, op *huma.Operation, query string, body io.Reader, header map[string]string) *http.Request {
		t.Helper()
		path := strings.NewReplacer("{id}", uuid.NewV7().String(), "{date}", "today").Replace(op.Path)
		r := httptest.NewRequestWithContext(t.Context(), op.Method, path+query, body)
		for k, v := range header {
			if k == "RemoteAddr" {
				r.RemoteAddr = v
				continue
			}
			r.Header.Set(k, v)
		}
		r.AddCookie(session)
		return r
	}
	ops := slices.Collect(maps.Values(document().ops))
	for i, op := range ops {
		t.Run(op.OperationID, func(t *testing.T) {
			t.Parallel()
			problems := []problem{
				{"an unknown query parameter", request(t, op, "?typo=1", nil, nil), http.StatusUnprocessableEntity},
				// From an address of its own, which one bad token does not
				// stop.
				{"a bad API token", request(t, op, "", nil, map[string]string{
					"Authorization": "Bearer " + auth.TokenPrefix + "bogus", "RemoteAddr": fmt.Sprintf("192.0.2.%d:1234", i+1),
				}), http.StatusUnauthorized},
				{"an address that sent too many bad tokens", request(t, op, "", nil, map[string]string{
					"Authorization": "Bearer " + auth.TokenPrefix + "bogus", "RemoteAddr": flooding,
				}), http.StatusTooManyRequests},
			}
			// Its answer depends on the operation; serve holds it to the
			// document.
			in.serve(t, request(t, op, "", nil, map[string]string{"Authorization": "Bearer " + reader.Token, "RemoteAddr": fmt.Sprintf("192.0.2.%d:1234", 100+i)}))
			if op.Method != http.MethodGet {
				problems = append(problems, problem{"a cross-origin request", request(t, op, "", nil, map[string]string{"Sec-Fetch-Site": "cross-site"}), http.StatusForbidden})
			}
			if op.RequestBody != nil {
				problems = append(problems, bodyProblems(op, every.Token, func(body io.Reader, header map[string]string) *http.Request {
					return request(t, op, "", body, header)
				})...)
			}
			for _, p := range problems {
				if rec := in.serve(t, p.req); p.want != 0 && rec.Code != p.want {
					t.Errorf("%s: %d %s, want %d", p.name, rec.Code, rec.Body.String(), p.want)
				}
			}
		})
	}
}

// An operation that lists no errors has a default response, which
// covers every status it does not list.
func TestCheckBodyTakesTheDefault(t *testing.T) {
	t.Parallel()

	var problem *huma.Response
	for _, op := range document().ops {
		if problem = op.Responses[strconv.Itoa(http.StatusInternalServerError)]; problem != nil {
			break
		}
	}
	if problem == nil {
		t.Fatal("no operation lists a 500")
	}
	op := &huma.Operation{OperationID: "quiet", Responses: map[string]*huma.Response{"default": problem}}
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/problem+json")
	rec.WriteHeader(http.StatusNotFound)
	_, _ = rec.WriteString(`{"title":"Not Found","status":404,"detail":"no such thing"}`)
	e := &errorsOf{TB: t}
	checkBody(e, op, rec)
	if len(e.errs) > 0 {
		t.Errorf("reported %q, want nothing", e.errs)
	}
}

// bodyType refuses a body that is no struct, and finds a struct's type.
func TestBodyType(t *testing.T) {
	t.Parallel()

	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	type list struct {
		Items []string `json:"items" nullable:"false"`
	}
	ref := r.Schema(reflect.TypeFor[list](), true, "")
	if typ, err := bodyType(r, &huma.MediaType{Schema: ref}); err != nil || typ != reflect.TypeFor[list]() {
		t.Errorf("a struct: %v, %v", typ, err)
	}
	for name, schema := range map[string]*huma.Schema{
		"a bare slice":    r.Schema(reflect.TypeFor[[]list](), true, ""),
		"no schema":       nil,
		"an unknown $ref": {Ref: "#/components/schemas/Nothing"},
	} {
		if _, err := bodyType(r, &huma.MediaType{Schema: schema}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
