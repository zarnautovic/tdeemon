package api_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// isolationHash is both users' password hash: a reference hash
// (argon2-cffi 25.1.0, m=8, t=1, p=1) of password. It is quick to check,
// and a sign-in replaces it, so the sign-in case writes a hash too.
const isolationHash = "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE"

// party is one user of an isolationEnv and what they own: a session, an
// API token with every scope that was used once, one never used, and an
// OpenID Connect identity. A row in each state the queries tell apart.
type party struct {
	user    user.User
	session *http.Cookie
	token   createdToken
	spare   createdToken
	// bread is their food with a serving and an alias.
	bread food.Food
	// meal is their lunch's id.
	meal uuid.UUID
}

// tokenName is the name of the party's token, which only they may see.
func (p party) tokenName() string { return p.user.Name + "'s phone" }

// isolationEnv is an instance with two users: the owner, whose things a
// case tries to reach, and the other, who acts. ana is an admin and ivo
// is not; TestIsolation runs every case both ways, so an admin who sees
// more than their own fails as well as a user who does.
type isolationEnv struct {
	in           *instance
	hits         *hits
	owner, other party
	// ownerValues are the owner's values in the database that the other
	// has none of: no answer to the other may show them.
	ownerValues []string
}

func newIsolationEnv(t *testing.T, adminActs bool) isolationEnv {
	t.Helper()

	in := newInstance(t, "http://localhost", isolationHash)
	h := &hits{next: in.h}
	in.h = h
	now := in.clock.Now()
	ivo := user.User{
		ID: uuid.NewV7(), Email: "ivo@example.com", Name: "Ivo", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := in.db.CreateUser(t.Context(), ivo, isolationHash); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ana, ivos := party{user: in.user}, party{user: ivo}
	for _, p := range []*party{&ana, &ivos} {
		// A session straight in the store: no sign-in to spend a hash on.
		token, id := auth.NewSessionToken()
		s := store.Session{ID: id, UserID: p.user.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(auth.SessionLifetime)}
		if err := in.db.CreateSession(t.Context(), s); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		p.session = sessionRequestCookie(token)
		tok, rec := in.createToken(t, p.session, `{"name":"`+p.tokenName()+`","scopes":["ingest","read","tools"]}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s's token: %d %s", p.user.Name, rec.Code, rec.Body.String())
		}
		p.token = tok
		// Used once, so the token has a last use a case could overwrite.
		if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(tok.Token)); rec.Code != http.StatusOK {
			t.Fatalf("%s's token: %d", p.user.Name, rec.Code)
		}
		p.bread = saveIsolationFoods(t, in.db, p.user, now)
		p.meal = saveIsolationMeals(t, in.db, p.user, p.bread, now)
		saveIsolationMetrics(t, in.db, p.user, now)
		saveIsolationPlan(t, in.db, p.user, now)
		identity := store.Identity{Issuer: "https://idp.example", Subject: "subject-of-" + p.user.Name, Email: p.user.Email, EmailVerified: true}
		if _, err := in.db.SignInWithIdentity(t.Context(), identity, now); err != nil {
			t.Fatalf("link %s's identity: %v", p.user.Name, err)
		}
	}
	// The never-used tokens after every use above, so a use that marks
	// other tokens cannot spoil the state that is to catch it.
	for _, p := range []*party{&ana, &ivos} {
		var rec *httptest.ResponseRecorder
		if p.spare, rec = in.createToken(t, p.session, `{"name":"`+p.user.Name+`'s spare","scopes":["read"]}`); rec.Code != http.StatusCreated {
			t.Fatalf("create %s's spare token: %d %s", p.user.Name, rec.Code, rec.Body.String())
		}
	}

	e := isolationEnv{in: in, hits: h, owner: ana, other: ivos}
	if adminActs {
		e.owner, e.other = ivos, ana
	}
	e.bothHaveRowsEverywhere(t)
	_, ownerValues := e.rowsOf(t, e.owner.user.ID)
	_, otherValues := e.rowsOf(t, e.other.user.ID)
	for _, v := range ownerValues {
		if len(v) >= 8 && !slices.Contains(otherValues, v) {
			e.ownerValues = append(e.ownerValues, v)
		}
	}
	slices.Sort(e.ownerValues)
	e.ownerValues = slices.Compact(e.ownerValues)
	return e
}

// bothHaveRowsEverywhere fails unless both users have rows in every table
// of user data: a table the fixture leaves empty compares [] with [] and
// proves nothing. A new kind of data adds rows of it above.
func (e isolationEnv) bothHaveRowsEverywhere(t *testing.T) {
	t.Helper()
	for _, tb := range e.userTables(t) {
		for _, u := range []*user.User{&e.owner.user, &e.other.user} {
			if rows, _ := e.tableRows(t, tb, u.ID); len(rows) == 0 {
				t.Fatalf("%s has no rows in %s: newIsolationEnv must give both users rows of every kind", u.Name, tb.name)
			}
		}
	}
}

// rowsOf returns every row of the database that belongs to the user with
// id (the users row, and the rows of every table with a user_id column),
// one string per row, sorted, and the text values in them.
func (e isolationEnv) rowsOf(t *testing.T, id uuid.UUID) (rows, values []string) {
	t.Helper()
	for _, tb := range e.userTables(t) {
		r, v := e.tableRows(t, tb, id)
		rows, values = append(rows, r...), append(values, v...)
	}
	slices.Sort(rows)
	return rows, values
}

// userTable is a table with a user's rows, and the column that names the
// user.
type userTable struct{ name, column string }

// isolationActor writes the fixtures: the user, through the web app.
var isolationActor = event.Actor{Kind: event.User, Client: "web"}

// saveIsolationFoods gives u a food with a serving and an alias, and an
// archived one per portion, their names u's own, and returns the first.
func saveIsolationFoods(t *testing.T, db *store.DB, u user.User, now time.Time) food.Food {
	t.Helper()
	// A food with a serving and an alias, its name the party's own.
	bread := food.Food{
		ID: uuid.NewV7(), Name: u.Name + "'s rye bread", Basis: food.Per100g, Source: food.User,
		Nutrients: food.Nutrients{Kcal: 259, Protein: 8.5, Carbs: 48, Fat: 3.3, Fiber: new(6.0)},
		Servings:  []food.Serving{{Name: "slice", Amount: 40}}, Aliases: []string{u.Name + "'s loaf"},
	}
	if _, err := db.SaveFood(t.Context(), u.ID, isolationActor, bread, now); err != nil {
		t.Fatalf("save %s's food: %v", u.Name, err)
	}
	// And an archived one, a portion.
	stew := food.Food{
		ID: uuid.NewV7(), Name: u.Name + "'s canteen stew", Basis: food.PerPortion, Source: food.Estimate,
		Nutrients: food.Nutrients{Kcal: 780, Protein: 38, Carbs: 85, Fat: 28}, PortionGrams: 450,
	}
	if _, err := db.SaveFood(t.Context(), u.ID, isolationActor, stew, now); err != nil {
		t.Fatalf("save %s's food: %v", u.Name, err)
	}
	if err := db.SetFoodArchived(t.Context(), u.ID, isolationActor, stew.ID, true, now); err != nil {
		t.Fatalf("archive %s's food: %v", u.Name, err)
	}
	return bread
}

// saveIsolationMeals gives u a meal of bread and a one-off estimate, and
// an excluded day, their text u's own, and returns the meal's id.
func saveIsolationMeals(t *testing.T, db *store.DB, u user.User, bread food.Food, now time.Time) uuid.UUID {
	t.Helper()
	toast, err := meal.FromFood(bread, meal.Quantity{Servings: 2, Serving: "slice"}, u.Name+"'s toast")
	if err != nil {
		t.Fatal(err)
	}
	soup, err := meal.Estimate(u.Name+"'s canteen soup", food.Nutrients{Kcal: 180, Protein: 6, Carbs: 20, Fat: 8}, "")
	if err != nil {
		t.Fatal(err)
	}
	m := meal.Meal{
		ID: meal.NewIDAt(now), EatenAt: now, Day: day.DateOf(now), Description: u.Name + "'s lunch", Client: meal.ClientMCP,
		Items: []meal.Item{toast, soup},
	}
	if _, _, err := db.CreateMeal(t.Context(), u.ID, isolationActor, m, now); err != nil {
		t.Fatalf("log %s's meal: %v", u.Name, err)
	}
	if err := db.ExcludeDay(t.Context(), u.ID, isolationActor, day.DateOf(now).AddDays(-1), day.Exclusion{Reason: u.Name + "'s birthday dinner"}, now); err != nil {
		t.Fatalf("exclude %s's day: %v", u.Name, err)
	}
	// A meal of their own two days before, which the summary of the
	// days up to yesterday shows.
	stew, err := meal.Estimate(u.Name+"'s stew", food.Nutrients{Kcal: isolationKcal(u), Protein: 40, Carbs: 100, Fat: 30}, "")
	if err != nil {
		t.Fatal(err)
	}
	at := now.AddDate(0, 0, -2)
	before := meal.Meal{ID: meal.NewIDAt(at), EatenAt: at, Day: day.DateOf(at), Description: u.Name + "'s dinner", Client: meal.ClientMCP, Items: []meal.Item{stew}}
	if _, _, err := db.CreateMeal(t.Context(), u.ID, isolationActor, before, now); err != nil {
		t.Fatalf("log %s's meal two days before: %v", u.Name, err)
	}
	return m.ID
}

// isolationKcal is the kcal of u's own meal two days before: the first
// letter of the name (A, I) sets them apart.
func isolationKcal(u user.User) float64 { return 1000 + float64(u.Name[0]%20) }

// saveIsolationMetrics gives u a day of metrics, a manual weight and
// imported steps, and a weigh-in of their own two days before, which
// the summary of the days up to yesterday shows.
func saveIsolationMetrics(t *testing.T, db *store.DB, u user.User, now time.Time) {
	t.Helper()
	if _, err := db.MergeMetrics(t.Context(), u.ID, isolationActor, day.DateOf(now), []metric.Value{
		{Kind: metric.Weight, Value: 81.5, Source: metric.Manual},
		{Kind: metric.Steps, Value: 6400, Source: metric.Import},
	}, now); err != nil {
		t.Fatalf("save %s's metrics: %v", u.Name, err)
	}
	if _, err := db.MergeMetrics(t.Context(), u.ID, isolationActor, day.DateOf(now).AddDays(-2), []metric.Value{
		{Kind: metric.Weight, Value: isolationWeight(u), Source: metric.Import},
	}, now); err != nil {
		t.Fatalf("save %s's weigh-in: %v", u.Name, err)
	}
}

// isolationWeight is u's own weigh-in two days before: the first letter
// of the name (A, I) sets it apart.
func isolationWeight(u user.User) float64 { return 80 + float64(u.Name[0]%20)/10 }

// saveIsolationPlan gives u a profile, a phase, a target, training
// weekdays, limits of their own, a toggle of a day's type, a day of a
// workout and notes for a coach, which name u, so they are text unique to u.
func saveIsolationPlan(t *testing.T, db *store.DB, u user.User, now time.Time) {
	t.Helper()
	today := day.DateOf(now)
	ctx := t.Context()
	// Values of each user's own: the first letter of the name (A, I)
	// sets them apart, so a case can tell whose it shows.
	n := int(u.Name[0] % 20)
	p := profile.Profile{Sex: profile.Female, BirthDate: today.AddDays(-12000 - n), HeightCm: 160 + float64(n)}
	if err := db.SetProfile(ctx, u.ID, isolationActor, p, today, now); err != nil {
		t.Fatalf("save %s's profile: %v", u.Name, err)
	}
	if err := db.SetPhase(ctx, u.ID, isolationActor, plan.Phase{Type: plan.Cut, Start: today, RatePctWeek: -0.5, GoalWeight: new(70.0)}, now); err != nil {
		t.Fatalf("save %s's phase: %v", u.Name, err)
	}
	d := plan.DayTarget{Kcal: 2100, Protein: 150, Fat: 60}
	if err := db.SetTarget(ctx, u.ID, isolationActor, plan.Target{EffectiveFrom: today, Training: d, Rest: d}, now); err != nil {
		t.Fatalf("save %s's target: %v", u.Name, err)
	}
	if err := db.SetTrainingWeekdays(ctx, u.ID, isolationActor, plan.Weekdays(0b0101010), now); err != nil {
		t.Fatalf("save %s's training weekdays: %v", u.Name, err)
	}
	l := limits.UserDefaults()
	l.ProteinFloorGrams = 100 + float64(n)
	if err := db.SetUserLimits(ctx, u.ID, isolationActor, l, limits.Instance(), now); err != nil {
		t.Fatalf("save %s's limits: %v", u.Name, err)
	}
	if err := db.SetTrainingDay(ctx, u.ID, isolationActor, today, new(true), now); err != nil {
		t.Fatalf("save %s's training day: %v", u.Name, err)
	}
	if _, _, err := db.SetWorkoutDays(ctx, u.ID, isolationActor, store.Strong, []day.Date{today.AddDays(-1)}, now); err != nil {
		t.Fatalf("save %s's days of workouts: %v", u.Name, err)
	}
	if err := db.RecordJobRun(ctx, u.ID, "morning_summary", today.String(), now); err != nil {
		t.Fatalf("save %s's run of a job: %v", u.Name, err)
	}
	if err := db.SetTelegramChat(ctx, u.ID, 1000+int64(n), now); err != nil {
		t.Fatalf("save %s's Telegram chat: %v", u.Name, err)
	}
	if err := db.SetProfileNotes(ctx, u.ID, isolationActor, "Notes of "+u.Name+": no fish, trains at dawn.", now); err != nil {
		t.Fatalf("save %s's notes: %v", u.Name, err)
	}
	if _, err := db.CheckIn(ctx, u.ID, isolationActor, today, func(c store.DayCheckIn) store.DayCheckIn {
		c.Mood, c.Hunger, c.Note = 1+n%5, 1+(n+2)%5, u.Name+"'s check-in: a long walk"
		return c
	}, now); err != nil {
		t.Fatalf("save %s's check-in: %v", u.Name, err)
	}
}

// noOnesData are the tables that hold no user's data.
var noOnesData = []string{"schema_migrations"}

// userTables lists users and every table with a user_id column, from the
// schema itself. A table with neither fails the test: every table of a
// user's rows has a user_id (internal/store/migrations/README.md), and
// the others are listed in noOnesData, or are the shadow tables of a
// full-text index.
func (e isolationEnv) userTables(t *testing.T) []userTable {
	t.Helper()
	res, err := e.in.db.Read().QueryContext(t.Context(), `
		SELECT m.name, EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'user_id'),
			EXISTS (SELECT 1 FROM sqlite_master AS v
				WHERE v.type = 'table' AND v.sql LIKE 'CREATE VIRTUAL TABLE%' AND m.name IN
					(v.name || '_data', v.name || '_idx', v.name || '_content', v.name || '_docsize', v.name || '_config'))
		FROM sqlite_master AS m WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY m.name`)
	if err != nil {
		t.Fatalf("list the tables: %v", err)
	}
	defer func() { _ = res.Close() }()
	var out []userTable
	for res.Next() {
		var name string
		var hasUserID, shadow bool
		if err := res.Scan(&name, &hasUserID, &shadow); err != nil {
			t.Fatalf("scan a table: %v", err)
		}
		switch {
		case shadow: // compared through its virtual table, which has a user_id
		case name == "users":
			out = append(out, userTable{name, "id"})
		case hasUserID:
			out = append(out, userTable{name, "user_id"})
		case !slices.Contains(noOnesData, name):
			t.Fatalf("table %s has no user_id column: the isolation test cannot tell whose its rows are", name)
		}
	}
	if err := res.Err(); err != nil {
		t.Fatalf("list the tables: %v", err)
	}
	return out
}

// tableRows returns the rows of tb that belong to the user with id, as
// rowsOf does.
func (e isolationEnv) tableRows(t *testing.T, tb userTable, id uuid.UUID) (rows, values []string) {
	t.Helper()
	// The names come from the schema, not from input.
	q := `SELECT * FROM "` + tb.name + `" WHERE ` + tb.column + ` = ?`
	res, err := e.in.db.Read().QueryContext(t.Context(), q, id.String())
	if err != nil {
		t.Fatalf("read %s: %v", tb.name, err)
	}
	defer func() { _ = res.Close() }()
	cols, err := res.Columns()
	if err != nil {
		t.Fatalf("columns of %s: %v", tb.name, err)
	}
	for res.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := res.Scan(ptrs...); err != nil {
			t.Fatalf("scan %s: %v", tb.name, err)
		}
		var row strings.Builder
		row.WriteString(tb.name)
		for i, v := range vals {
			fmt.Fprintf(&row, " %s=%v", cols[i], v)
			if s, ok := v.(string); ok {
				values = append(values, s)
			}
		}
		rows = append(rows, row.String())
	}
	if err := res.Err(); err != nil {
		t.Fatalf("read %s: %v", tb.name, err)
	}
	return rows, values
}

// noneOfTheOwners fails if body shows anything of the owner's.
func (e isolationEnv) noneOfTheOwners(t *testing.T, what, body string) {
	t.Helper()
	for _, s := range append([]string{e.owner.token.Token}, e.ownerValues...) {
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
	for _, path := range []string{"/api/v1/auth/session", "/api/v1/tokens", "/api/v1/profile", "/api/v1/limits"} {
		rec := e.in.do(t, http.MethodGet, path, "", []*http.Cookie{e.owner.session}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("the owner's GET %s: %d %s", path, rec.Code, rec.Body.String())
		}
		b.WriteString(rec.Body.String())
	}
	return b.String()
}

// ownersTokenWorks fails if the other's request ended the owner's token.
// It uses the token, so it comes after the comparisons.
func (e isolationEnv) ownersTokenWorks(t *testing.T) {
	t.Helper()
	if rec := e.in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(e.owner.token.Token)); rec.Code != http.StatusOK {
		t.Errorf("the owner's token after the other's request: %d, want 200", rec.Code)
	}
}

// hits records, while on, the requests that got a 2xx answer, so a case
// can be held to reaching its own operation, and every answer, headers
// and body, so none escapes the search for the owner's values.
type hits struct {
	next    http.Handler
	mu      sync.Mutex
	on      bool
	got     []string // "METHOD /path"
	answers []answer
}

// answer is a response the other got: what was asked, and the headers
// and body that came back.
type answer struct{ request, text string }

func (h *hits) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
	h.next.ServeHTTP(sw, r)
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.on {
		return
	}
	if sw.code < 300 {
		h.got = append(h.got, r.Method+" "+r.URL.Path)
	}
	h.answers = append(h.answers, answer{r.Method + " " + r.URL.Path, fmt.Sprintf("%v %s", w.Header(), sw.body.String())})
}

// recorded returns the answers recorded so far.
func (h *hits) recorded() []answer {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.answers)
}

func (h *hits) record(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.on = on
}

// reached reports whether a recorded request matched op.
func (h *hits) reached(op operation) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	route := regexp.MustCompile(`^` + op.method + ` ` + regexp.MustCompile(`\\\{[^}]+\\\}`).ReplaceAllString(regexp.QuoteMeta(op.route), `[^/]+`) + `$`)
	return slices.ContainsFunc(h.got, route.MatchString)
}

type statusWriter struct {
	http.ResponseWriter
	code int
	body bytes.Buffer
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// isolationCase runs one operation as the other user, against what the
// owner has.
type isolationCase func(t *testing.T, e isolationEnv)

// read is the case of a GET: with the other's session and, withToken,
// their token too, it must answer 200 (so the request got to the query,
// not stuck at a check) and, where the answer has user data, show what
// shows returns of the other's own (so an empty answer does not pass).
// TestIsolation searches every answer for the owner's values.
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

// ingestCase is the case of an import: the other's token sends body, for
// today, to path.
//
//nolint:thelper // A case is a test, not a helper: a failure should point into it.
func ingestCase(path string, body func(today string) string) isolationCase {
	return func(t *testing.T, e isolationEnv) {
		rec := e.in.do(t, http.MethodPost, path, body(day.DateOf(e.in.clock.Now()).String()), nil, bearer(e.other.token.Token))
		if rec.Code != http.StatusOK {
			t.Errorf("the other's import to %s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

// isolateMeal is the case of the operation that changes a meal (update-
// meal, delete-meal): the owner's meal answers the other as one that does
// not exist; their own they change.
func isolateMeal(method, body string) isolationCase {
	return func(t *testing.T, e isolationEnv) {
		t.Helper()
		theirs := e.in.do(t, method, "/api/v1/meals/"+e.owner.meal.String(), body, nil, bearer(e.other.token.Token))
		none := e.in.do(t, method, "/api/v1/meals/"+uuid.NewV7().String(), body, nil, bearer(e.other.token.Token))
		if theirs.Code != http.StatusNotFound || theirs.Body.String() != none.Body.String() {
			t.Errorf("the owner's meal: %d %s; one that does not exist: %d %s; want the same 404", theirs.Code, theirs.Body.String(), none.Code, none.Body.String())
		}
		if own := e.in.do(t, method, "/api/v1/meals/"+e.other.meal.String(), body, nil, bearer(e.other.token.Token)); own.Code != http.StatusOK {
			t.Errorf("their own meal: %d %s", own.Code, own.Body.String())
		}
	}
}

// isolateDay is the case of a change of a day: the other changes their
// own today; the owner's day stays as it was.
func isolateDay(method, path, body string) isolationCase {
	return func(t *testing.T, e isolationEnv) {
		t.Helper()
		if rec := e.in.do(t, method, "/api/v1/days/today"+path, body, nil, bearer(e.other.token.Token)); rec.Code != http.StatusOK {
			t.Errorf("their own day: %d %s", rec.Code, rec.Body.String())
		}
	}
}

// isolateUndo is undo-last's case: the other takes back their own latest
// change; the owner's stay as they were.
func isolateUndo(t *testing.T, e isolationEnv) {
	t.Helper()
	body := `{"id": "` + clientid.NewAt(e.in.clock.Now()).String() + `"}`
	if rec := e.in.do(t, http.MethodPost, "/api/v1/undo", body, nil, bearer(e.other.token.Token)); rec.Code != http.StatusOK {
		t.Errorf("their own undo: %d %s", rec.Code, rec.Body.String())
	}
}

// isolateLogMeal is log-meal's case: the other logs a meal of their own,
// of their own bread; the owner's meals stay as they were. (TestLogMealIDsArePerUser
// logs under the owner's meal's id, which this case cannot: its answer
// would show it.)
func isolateLogMeal(t *testing.T, e isolationEnv) {
	t.Helper()
	body := fmt.Sprintf(`{"id": %q, "description": "%s's dinner", "items": [{"food_id": %q, "servings": 1, "serving": "slice"}]}`,
		meal.NewIDAt(e.in.clock.Now()), e.other.user.Name, e.other.bread.ID)
	if rec := e.in.do(t, http.MethodPost, "/api/v1/meals", body, nil, bearer(e.other.token.Token)); rec.Code != http.StatusCreated {
		t.Errorf("the other's meal: %d %s", rec.Code, rec.Body.String())
	}
}

// isolateGetFood is get-food's case: the owner's food answers the other
// as one that does not exist; their own they get.
func isolateGetFood(t *testing.T, e isolationEnv) {
	t.Helper()
	theirs := e.in.do(t, http.MethodGet, "/api/v1/foods/"+e.owner.bread.ID.String(), "", nil, bearer(e.other.token.Token))
	none := e.in.do(t, http.MethodGet, "/api/v1/foods/"+uuid.NewV7().String(), "", nil, bearer(e.other.token.Token))
	if theirs.Code != http.StatusNotFound || theirs.Body.String() != none.Body.String() {
		t.Errorf("the owner's food: %d %s; one that does not exist: %d %s; want the same 404", theirs.Code, theirs.Body.String(), none.Code, none.Body.String())
	}
	if own := e.in.do(t, http.MethodGet, "/api/v1/foods/"+e.other.bread.ID.String(), "", []*http.Cookie{e.other.session}, nil); own.Code != http.StatusOK {
		t.Errorf("their own food: %d %s", own.Code, own.Body.String())
	}
}

// isolateUpsertFood is upsert-food's case: the other saves a food of
// their own, with a token, and the owner's foods stay as they were.
// (TestUpsertFoodIDsArePerUser saves the owner's name, brand and alias,
// which this case cannot: its answer would show them.)
func isolateUpsertFood(t *testing.T, e isolationEnv) {
	t.Helper()
	body := fmt.Sprintf(`{"name": "%s's crackers", "basis": "100g", "source": "user", "nutrients": {"kcal": 420, "protein": 10, "carbs": 70, "fat": 10}}`,
		e.other.user.Name)
	rec := e.in.do(t, http.MethodPost, "/api/v1/foods", body, nil, bearer(e.other.token.Token))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), e.other.user.Name+"'s crackers") {
		t.Errorf("the other's food: %d %s", rec.Code, rec.Body.String())
	}
}

// isolateListFoods is list-foods' case: the other's foods, of both lists,
// and none of the owner's.
func isolateListFoods(t *testing.T, e isolationEnv) {
	t.Helper()
	read("/api/v1/foods/list?order=use", false, func(e isolationEnv) []string {
		return []string{e.other.user.Name + "'s rye bread", e.other.user.Name + "'s loaf"}
	})(t, e)
	read("/api/v1/foods/list?archived=true", false, func(e isolationEnv) []string {
		return []string{e.other.user.Name + "'s canteen stew"}
	})(t, e)
}

// isolateCreateFood is create-food's case: the other makes a food of
// their own, and the owner's stay as they were.
func isolateCreateFood(t *testing.T, e isolationEnv) {
	t.Helper()
	body := fmt.Sprintf(`{"name": "%s's kefir", "basis": "100ml", "source": "label", "nutrients": {"kcal": 41, "protein": 3.4, "carbs": 4, "fat": 1.5}}`,
		e.other.user.Name)
	rec := e.in.do(t, http.MethodPost, "/api/v1/foods/new", body, []*http.Cookie{e.other.session}, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), e.other.user.Name+"'s kefir") {
		t.Errorf("the other's food: %d %s", rec.Code, rec.Body.String())
	}
}

// isolateSetSettings is set-settings' case: the other changes their own.
func isolateSetSettings(t *testing.T, e isolationEnv) {
	t.Helper()
	rec := e.in.do(t, http.MethodPatch, "/api/v1/me", `{"name": "Another name", "timezone": "Asia/Tokyo", "day_start_hour": 5}`,
		[]*http.Cookie{e.other.session}, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), e.other.user.Email) {
		t.Errorf("the other's settings: %d %s", rec.Code, rec.Body.String())
	}
}

// isolateSetNotes is set-notes' case: the other sets their own notes.
func isolateSetNotes(t *testing.T, e isolationEnv) {
	t.Helper()
	rec := e.in.do(t, http.MethodPut, "/api/v1/me/notes", `{"notes": "New notes of the other"}`, []*http.Cookie{e.other.session}, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("the other's notes: %d %s", rec.Code, rec.Body.String())
	}
}

// isolateRevokeTokens is revoke-tokens' case: the other's tokens go, and
// the owner's work, which isolated checks.
func isolateRevokeTokens(t *testing.T, e isolationEnv) {
	t.Helper()
	rec := e.in.do(t, http.MethodDelete, "/api/v1/tokens", "", []*http.Cookie{e.other.session}, nil)
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, `"revoked":`) || strings.Contains(body, `"revoked":0`) {
		t.Errorf("the other's tokens: %d %s", rec.Code, body)
	}
}

// isolationCases has one case for every operation in the OpenAPI
// document. TestIsolation checks after each that no answer showed the
// owner's values, that the case got a 2xx from its operation, that the
// owner's rows in the database and what the owner sees have not changed,
// and that the owner's token still works;
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
			// With the other's session, which the sign-in ends; the
			// password's old hash is replaced, a write of its own.
			rec := e.in.login(t, e.other.user.Email, password, e.other.session)
			if rec.Code != http.StatusOK {
				t.Fatalf("the other's sign-in: %d %s", rec.Code, rec.Body.String())
			}
			if old := e.in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{e.other.session}, nil); old.Code != http.StatusUnauthorized {
				t.Errorf("the other's previous session after signing in again: %d, want 401", old.Code)
			}
			fresh := e.in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{sessionCookie(t, rec)}, nil)
			if !strings.Contains(fresh.Body.String(), e.other.user.Email) {
				t.Errorf("the new session is not the other's: %d %s", fresh.Code, fresh.Body.String())
			}
		},
		"logout": func(t *testing.T, e isolationEnv) {
			if rec := e.in.do(t, http.MethodPost, "/api/v1/auth/logout", "", []*http.Cookie{e.other.session}, nil); rec.Code != http.StatusNoContent {
				t.Errorf("the other's sign-out: %d, want 204", rec.Code)
			}
			if rec := e.in.do(t, http.MethodGet, "/api/v1/auth/session", "", []*http.Cookie{e.other.session}, nil); rec.Code != http.StatusUnauthorized {
				t.Errorf("the other's session after signing out: %d, want 401", rec.Code)
			}
		},
		"create-token": func(t *testing.T, e isolationEnv) {
			created, rec := e.in.createToken(t, e.other.session, `{"name":"another","scopes":["read"]}`)
			if rec.Code != http.StatusCreated {
				t.Fatalf("the other's new token: %d %s", rec.Code, rec.Body.String())
			}
			who := e.in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(created.Token))
			if !strings.Contains(who.Body.String(), e.other.user.Email) {
				t.Errorf("the other's new token acts for %s, want them", who.Body.String())
			}
		},
		"get-profile": read("/api/v1/profile", false, func(e isolationEnv) []string {
			return []string{fmt.Sprintf(`"height_cm":%d`, 160+int(e.other.user.Name[0]%20))}
		}),
		"set-settings":  isolateSetSettings,
		"set-notes":     isolateSetNotes,
		"revoke-tokens": isolateRevokeTokens,
		"get-history": read("/api/v1/history", false, func(e isolationEnv) []string {
			return []string{fmt.Sprintf(`"weight_kg":%v`, isolationWeight(e.other.user))}
		}),
		// The summary holds no text of the user's: the store's queries by
		// the user keep it theirs.
		"get-summary": read("/api/v1/summary", true, func(e isolationEnv) []string {
			return []string{fmt.Sprintf(`"weight_kg":%v`, isolationWeight(e.other.user)), fmt.Sprintf(`"kcal":%v`, isolationKcal(e.other.user))}
		}),
		"get-day": read("/api/v1/days/today", true, func(e isolationEnv) []string {
			return []string{e.other.user.Name + "'s lunch", e.other.user.Name + "'s toast"}
		}),
		"find-foods": read("/api/v1/foods?query=rye", true, func(e isolationEnv) []string {
			return []string{e.other.user.Name + "'s rye bread", e.other.user.Name + "'s loaf"}
		}),
		"list-foods":    isolateListFoods,
		"upsert-food":   isolateUpsertFood,
		"create-food":   isolateCreateFood,
		"get-food":      isolateGetFood,
		"log-meal":      isolateLogMeal,
		"update-meal":   isolateMeal(http.MethodPatch, `{"note": "seen"}`),
		"delete-meal":   isolateMeal(http.MethodDelete, ""),
		"log-metric":    isolateDay(http.MethodPut, "/metrics/weight", `{"value": 77.3}`),
		"delete-metric": isolateDay(http.MethodDelete, "/metrics/weight", ""),
		"check-in":      isolateDay(http.MethodPatch, "", `{"mood": 2, "note": "a quiet day", "training_day": "rest", "excluded_reason": "sick"}`),
		"undo-last":     isolateUndo,
		"get-me": read("/api/v1/me", true, func(e isolationEnv) []string {
			return []string{"Notes of " + e.other.user.Name, fmt.Sprintf(`"height_cm":%d`, 160+int(e.other.user.Name[0]%20))}
		}),
		"get-limits": read("/api/v1/limits", false, func(e isolationEnv) []string {
			return []string{fmt.Sprintf(`"protein_floor_grams":%d`, 100+int(e.other.user.Name[0]%20))}
		}),
		"set-profile": func(t *testing.T, e isolationEnv) {
			rec := e.in.do(t, http.MethodPut, "/api/v1/profile", `{"sex":"male","birth_date":"1985-01-20","height_cm":181}`, []*http.Cookie{e.other.session}, nil)
			if rec.Code != http.StatusOK {
				t.Errorf("the other's profile: %d %s", rec.Code, rec.Body.String())
			}
		},
		"set-limits": func(t *testing.T, e isolationEnv) {
			rec := e.in.do(t, http.MethodPut, "/api/v1/limits", `{"limits":{"cut_rate":-0.4}}`, []*http.Cookie{e.other.session}, nil)
			if want := fmt.Sprintf(`"protein_floor_grams":%d`, 100+int(e.other.user.Name[0]%20)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
				t.Errorf("the other's limits: %d %s", rec.Code, rec.Body.String())
			}
		},
		"ingest-hae": ingestCase("/api/v1/ingest/hae", func(today string) string {
			return `{"data": {"metrics": [{"name": "step_count", "units": "count", "data": [{"date": "` + today + ` 00:00:00 +0000", "qty": 99999}]}]}}`
		}),
		"ingest-metrics": ingestCase("/api/v1/ingest/metrics", func(today string) string {
			return `{"date": "` + today + `", "metrics": {"steps": 99999, "weight": 70.5}}`
		}),
		// The owner has a workout yesterday, which the other's export lacks.
		"ingest-strong": ingestCase("/api/v1/ingest/strong", func(today string) string {
			return "Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Seconds\n" + today + " 07:00:00,Day A,1h,Squat,1,90,5,0\n"
		}),
		"revoke-token": func(t *testing.T, e isolationEnv) {
			theirs := e.in.do(t, http.MethodDelete, "/api/v1/tokens/"+e.owner.token.ID, "", []*http.Cookie{e.other.session}, nil)
			none := e.in.do(t, http.MethodDelete, "/api/v1/tokens/"+uuid.NewV7().String(), "", []*http.Cookie{e.other.session}, nil)
			if theirs.Code != http.StatusNotFound || theirs.Code != none.Code || theirs.Body.String() != none.Body.String() {
				t.Errorf("revoking the owner's token: %d %s; a token that does not exist: %d %s; want the same 404",
					theirs.Code, theirs.Body.String(), none.Code, none.Body.String())
			}
			// Their own goes.
			if own := e.in.do(t, http.MethodDelete, "/api/v1/tokens/"+e.other.token.ID, "", []*http.Cookie{e.other.session}, nil); own.Code != http.StatusNoContent {
				t.Errorf("revoking their own token: %d, want 204", own.Code)
			}
		},
	}
}

func TestIsolation(t *testing.T) {
	t.Parallel()

	ops := map[string]operation{}
	for _, op := range operations(t) {
		ops[op.id] = op
	}
	for who, adminActs := range map[string]bool{"a user acts": false, "an admin acts": true} {
		t.Run(who, func(t *testing.T) {
			t.Parallel()
			for op, run := range isolationCases() {
				t.Run(op, func(t *testing.T) {
					t.Parallel()
					e := newIsolationEnv(t, adminActs)
					e.isolated(t, run, func() {
						if o, ok := ops[op]; ok && !e.hits.reached(o) {
							t.Errorf("the case got no 2xx from %s %s, its operation", o.method, o.route)
						}
					})
				})
			}
		})
	}
}

// isolated runs the case run in e and fails unless the owner was left as
// they were: no answer showed the owner's values, their rows and what
// they see did not change, and their token works; reached checks what
// the case reached.
func (e isolationEnv) isolated(t *testing.T, run isolationCase, reached func()) {
	t.Helper()
	sees := e.ownerSees(t)
	rows, _ := e.rowsOf(t, e.owner.user.ID)
	// Two hours on, a request records the use of its session or token
	// again: the case's requests write, and a write to one of the owner's
	// rows shows.
	e.in.clock.Set(e.in.clock.Now().Add(2 * time.Hour))
	e.hits.record(true)
	run(t, e)
	e.hits.record(false)
	for _, a := range e.hits.recorded() {
		e.noneOfTheOwners(t, "the answer to "+a.request, a.text)
	}
	reached()
	if after, _ := e.rowsOf(t, e.owner.user.ID); !slices.Equal(after, rows) {
		t.Errorf("the owner's rows changed:\nbefore %s\nafter  %s", strings.Join(rows, "\n       "), strings.Join(after, "\n       "))
	}
	if after := e.ownerSees(t); after != sees {
		t.Errorf("what the owner sees changed:\nbefore %s\nafter  %s", sees, after)
	}
	e.ownersTokenWorks(t)
}

// Every operation in the OpenAPI document has an isolation case, and
// every case an operation. Routes outside the document (the OpenID
// Connect browser flow, and MCP, whose cases are mcpIsolationCases) are
// not listed here: see docs/conventions.md.
func TestIsolationCoversEveryOperation(t *testing.T) {
	t.Parallel()

	cases := isolationCases()
	var missing []string
	inSpec := map[string]bool{}
	for _, op := range operations(t) {
		inSpec[op.id] = true
		if _, ok := cases[op.id]; !ok {
			missing = append(missing, op.method+" "+op.route+" ("+op.id+")")
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
