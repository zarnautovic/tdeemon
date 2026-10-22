// Package eval_test runs an AI client, Claude Code without a window, over
// cases of logging through tdeemon's MCP server, and measures what a log
// costs: the rounds of tool calls and the input tokens of each message.
// It checks what each case left in the database, too.
//
// It runs only when asked, as it spends the quota of the Claude account
// signed in to Claude Code:
//
//	TDEEMON_EVAL=1 go test ./internal/eval -run TestEval -v -timeout 30m
//
// TDEEMON_EVAL_MODEL picks the model (sonnet when not set), and
// TDEEMON_EVAL_CASES names a file of more cases, in the form of
// testdata/cases.json: the real ones, which stay out of git.
package eval_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"math"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
)

// A case is what the user says, one message after another, in one
// session, and what it must leave.
type evalCase struct {
	Name string `json:"name"`
	// Lang is the language the user writes in, and names their foods in:
	// en or hr.
	Lang string   `json:"lang"`
	Say  []string `json:"say"`
	// Label, when set, is a food's label, drawn and sent as a photo with
	// the first message.
	Label *label `json:"label,omitempty"`
	// SeedMeal logs a meal before the case.
	SeedMeal bool   `json:"seed_meal,omitempty"`
	Expect   expect `json:"expect"`
}

type label struct {
	Name    string  `json:"name"`
	Brand   string  `json:"brand"`
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Carbs   float64 `json:"carbs"`
	Fat     float64 `json:"fat"`
	// Portion is a bar's grams.
	Portion float64 `json:"portion"`
}

type expect struct {
	// Meals and Items are what the day holds after the case.
	Meals *int `json:"meals,omitempty"`
	Items *int `json:"items,omitempty"`
	// Kcal is the range the day's total falls in.
	Kcal []float64 `json:"kcal,omitempty"`
	// NewFoods are words a food made in the case has in its name;
	// NoNewFoods says the case makes none.
	NewFoods   []string `json:"new_foods,omitempty"`
	NoNewFoods bool     `json:"no_new_foods,omitempty"`
	Weight     float64  `json:"weight,omitempty"`
	Mood       []int    `json:"mood,omitempty"`
	Hunger     []int    `json:"hunger,omitempty"`
	// MaxRounds bounds the rounds of tool calls of each message; 2 when
	// not set (DESIGN.md §5: a log is 1-2 calls).
	MaxRounds int `json:"max_rounds,omitempty"`
}

// systemPrompt is all the client says of itself: the policy comes from
// the server's instructions.
const systemPrompt = "You help the user log what they eat and keep track of their day, with the tdeemon tools. Be brief."

func TestEval(t *testing.T) {
	if os.Getenv("TDEEMON_EVAL") == "" {
		t.Skip("TDEEMON_EVAL is not set: the eval spends the Claude account's quota")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Fatalf("the eval runs Claude Code: %v", err)
	}
	cases := readCases(t, "testdata/cases.json")
	if more := os.Getenv("TDEEMON_EVAL_CASES"); more != "" {
		cases = append(cases, readCases(t, more)...)
	}
	model := os.Getenv("TDEEMON_EVAL_MODEL")
	if model == "" {
		model = "sonnet"
	}
	base := baseline(t, model)
	t.Logf("model %s; the client alone, a message without tools: %d input tokens", model, base)
	t.Logf("the tools and instructions, first sent: %d input tokens not from the cache", warmUp(t, model))
	var rows []string
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			rows = append(rows, runCase(t, c, model))
		})
	}
	t.Logf("\n%-32s %6s %6s %8s %8s  %s\n%s", "case", "calls", "rounds", "input", "uncached", "ok", strings.Join(rows, "\n"))
}

func readCases(t *testing.T, path string) []evalCase {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G703: the path is the developer's own, from TDEEMON_EVAL_CASES
	if err != nil {
		t.Fatal(err)
	}
	var cs []evalCase
	if err := json.Unmarshal(b, &cs); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return cs
}

// instance is a tdeemon of one user, served over HTTP, as the eval's
// client reaches it.
type instance struct {
	db    *store.DB
	user  user.User
	cal   day.Calendar
	url   string
	token string
}

var actor = event.Actor{Kind: event.User, Client: "eval"}

// names are the user's foods' names in each language: bread and its
// serving, yoghurt, egg and its serving, and the soup of a seeded meal.
var names = map[string][6]string{
	"en": {"Rye bread", "slice", "Greek yoghurt", "Egg", "piece", "Vegetable soup"},
	"hr": {"Raženi kruh", "šnita", "Grčki jogurt", "Jaje", "komad", "Juha od povrća"},
}

func newInstance(t *testing.T, seedMeal bool, lang string) instance {
	t.Helper()
	ctx := t.Context()
	db := storetest.Open(t)
	now := time.Now()
	u := user.User{
		ID: uuid.NewV7(), Email: "eval@example.com", Name: "Eval", Role: user.RoleUser, Locale: lang,
		TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(ctx, u, ""); err != nil {
		t.Fatal(err)
	}
	cal, err := day.NewCalendar(u.TimeZone, u.DayStartHour)
	if err != nil {
		t.Fatal(err)
	}
	today := cal.DayOf(now)
	n, ok := names[lang]
	if !ok {
		t.Fatalf("lang %q: want en or hr", lang)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SetProfile(ctx, u.ID, actor, profile.Profile{Sex: profile.Male, BirthDate: today.AddDays(-13000), HeightCm: 180}, today, now))
	must(db.SetPhase(ctx, u.ID, actor, plan.Phase{Type: plan.Cut, Start: today.AddDays(-30), RatePctWeek: -0.5}, now))
	d := plan.DayTarget{Kcal: 2200, Protein: 160, Fat: 65}
	must(db.SetTarget(ctx, u.ID, actor, plan.Target{EffectiveFrom: today.AddDays(-30), Training: d, Rest: d}, now))
	foods := []food.Food{
		{
			ID: uuid.NewV7(), Name: n[0], Basis: food.Per100g, Source: food.User,
			Nutrients: food.Nutrients{Kcal: 259, Protein: 8.5, Carbs: 48, Fat: 3.3}, Servings: []food.Serving{{Name: n[1], Amount: 40}},
		},
		{
			ID: uuid.NewV7(), Name: n[2], Brand: "Dukat", Basis: food.Per100g, Source: food.Label,
			Nutrients: food.Nutrients{Kcal: 73, Protein: 9.5, Carbs: 3.6, Fat: 2},
		},
		{
			ID: uuid.NewV7(), Name: n[3], Basis: food.Per100g, Source: food.Label,
			Nutrients: food.Nutrients{Kcal: 143, Protein: 12.6, Carbs: 0.7, Fat: 9.5}, Servings: []food.Serving{{Name: n[4], Amount: 60}},
		},
	}
	for i := range foods {
		if _, err := db.SaveFood(ctx, u.ID, actor, foods[i], now); err != nil {
			t.Fatal(err)
		}
	}
	if seedMeal {
		soup, err := meal.Estimate(n[5], food.Nutrients{Kcal: 180, Protein: 6, Carbs: 20, Fat: 8}, "")
		must(err)
		at := now.Add(-time.Hour)
		_, _, err = db.CreateMeal(ctx, u.ID, actor, meal.Meal{
			ID: meal.NewIDAt(at), EatenAt: at, Day: cal.DayOf(at),
			Client: meal.ClientWeb, Items: []meal.Item{soup},
		}, now)
		must(err)
	}
	token, hash := auth.NewAPIToken()
	must(db.CreateAPIToken(ctx, store.APIToken{
		ID: uuid.NewV7(), UserID: u.ID, Name: "eval",
		Scopes: auth.FormatScopes([]auth.Scope{auth.ScopeTools}), CreatedAt: now,
	}, hash))
	base, err := url.Parse("http://localhost")
	must(err)
	h, err := api.NewHandler(api.Config{
		Info: api.ServerInfo{Name: "tdeemon", Version: "eval"}, DB: db, BaseURL: base,
		Logger: slog.New(slog.DiscardHandler),
	})
	must(err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return instance{db: db, user: u, cal: cal, url: srv.URL + api.MCPPath, token: token}
}

// turn is what the client did for one message: its rounds of tool calls,
// the calls, and the input tokens of the message: all of them, and the
// fresh ones, not read from the cache, which a client pays in full.
type turn struct {
	calls, rounds int
	tokens, fresh int
	tools         []string
}

// run sends messages to Claude Code, connected to mcpConfig's server
// alone, one after the answer to the other, and returns its turns.
func run(t *testing.T, model, mcpConfig string, messages []map[string]any) []turn {
	t.Helper()
	args := []string{
		"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--model", model,
		"--tools", "", "--system-prompt", systemPrompt, "--setting-sources", "project", "--no-session-persistence",
		"--strict-mcp-config",
	}
	if mcpConfig != "" {
		args = append(args, "--mcp-config", mcpConfig, "--allowedTools", "mcp__tdeemon")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", args...) //nolint:gosec // G204: the arguments are the eval's own, the model the developer's
	cmd.Dir = t.TempDir()
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	send := func(i int) {
		line, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": messages[i]["content"]}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stdin.Write(append(line, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	send(0)
	var turns []turn
	var cur turn
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var ev streamLine
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Type != "result" {
			cur.add(t, &ev)
			continue
		}
		if ev.IsError {
			t.Errorf("the client failed: %s", ev.Result)
		}
		cur.tokens = ev.Usage.Input + ev.Usage.CacheCreation + ev.Usage.CacheRead
		cur.fresh = ev.Usage.Input + ev.Usage.CacheCreation
		turns = append(turns, cur)
		t.Logf("answer: %s", oneLine(ev.Result))
		cur = turn{}
		if len(turns) < len(messages) {
			send(len(turns))
		} else {
			_ = stdin.Close()
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("claude: %v: %s", err, errOut.String())
	}
	return turns
}

// streamLine is a line of Claude Code's stream-json output.
type streamLine struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type    string          `json:"type"`
			Name    string          `json:"name"`
			Input   json.RawMessage `json:"input"`
			IsError bool            `json:"is_error"`
			Content json.RawMessage `json:"content"`
		} `json:"content"`
	} `json:"message"`
	Usage struct {
		Input         int `json:"input_tokens"`
		CacheCreation int `json:"cache_creation_input_tokens"`
		CacheRead     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
}

// add counts the tool calls of an assistant's message, a round when it
// has any, and logs them and the errors the tools answer.
func (cur *turn) add(t *testing.T, ev *streamLine) {
	t.Helper()
	calls := 0
	for _, c := range ev.Message.Content {
		switch {
		case ev.Type == "assistant" && c.Type == "tool_use":
			calls++
			name := strings.TrimPrefix(c.Name, "mcp__tdeemon__")
			cur.tools = append(cur.tools, name)
			t.Logf("call %s %s", name, oneLine(string(c.Input)))
		case ev.Type == "user" && c.Type == "tool_result" && c.IsError:
			t.Logf("the tool answered an error: %s", oneLine(string(c.Content)))
		}
	}
	if calls > 0 {
		cur.calls += calls
		cur.rounds++
	}
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// baseline is the input tokens of a message with no tools: the client's
// own, which a log through tdeemon pays as well.
func baseline(t *testing.T, model string) int {
	t.Helper()
	turns := run(t, model, "", []map[string]any{{"content": []map[string]any{{"type": "text", "text": "Say ok."}}}})
	if len(turns) != 1 {
		t.Fatalf("the baseline: %d turns", len(turns))
	}
	return turns[0].tokens
}

// warmUp sends a message with the tools before the cases, so the first
// case does not pay for putting them in the cache, which the cases after
// it read; it returns what it paid.
func warmUp(t *testing.T, model string) int {
	t.Helper()
	turns := run(t, model, mcpConfig(t, newInstance(t, false, "en")), []map[string]any{{"content": []map[string]any{{"type": "text", "text": "Hi"}}}})
	if len(turns) != 1 {
		t.Fatalf("the warm-up: %d turns", len(turns))
	}
	return turns[0].fresh
}

// mcpConfig writes Claude Code's configuration of in's MCP server, and
// returns its path.
func mcpConfig(t *testing.T, in instance) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	b, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"tdeemon": map[string]any{
		"type": "http", "url": in.url, "headers": map[string]string{"Authorization": "Bearer " + in.token},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func runCase(t *testing.T, c evalCase, model string) string {
	t.Helper()
	in := newInstance(t, c.SeedMeal, c.Lang)
	cfg := mcpConfig(t, in)
	foodsBefore := foodNames(t, in)
	var messages []map[string]any
	for i, say := range c.Say {
		content := []map[string]any{{"type": "text", "text": say}}
		if i == 0 && c.Label != nil {
			content = append([]map[string]any{{"type": "image", "source": map[string]any{
				"type": "base64", "media_type": "image/png", "data": base64.StdEncoding.EncodeToString(drawLabel(t, c.Label)),
			}}}, content...)
		}
		messages = append(messages, map[string]any{"content": content})
	}
	turns := run(t, model, cfg, messages)
	if len(turns) != len(c.Say) {
		t.Fatalf("%d answers to %d messages", len(turns), len(c.Say))
	}
	ok := check(t, in, c.Expect, foodsBefore)
	maxRounds := c.Expect.MaxRounds
	if maxRounds == 0 {
		maxRounds = 2
	}
	var rows []string
	for i, tr := range turns {
		if tr.rounds > maxRounds {
			t.Errorf("message %d: %d rounds of tool calls (%s), want at most %d", i+1, tr.rounds, strings.Join(tr.tools, ", "), maxRounds)
			ok = false
		}
		// The cost target (DESIGN.md §6) is of what a log pays in full: a
		// client sends the same prefix with each round, read from the cache.
		if tr.fresh > 10000 {
			t.Errorf("message %d: %d input tokens not from the cache, want under 10000", i+1, tr.fresh)
			ok = false
		}
		rows = append(rows, fmt.Sprintf("%-32s %6d %6d %8d %8d  %v  %s", fmt.Sprintf("%s (%d)", c.Name, i+1), tr.calls, tr.rounds, tr.tokens, tr.fresh,
			map[bool]string{true: "ok", false: "FAIL"}[ok], strings.Join(tr.tools, " ")))
	}
	return strings.Join(rows, "\n")
}

// check holds the day to e, and says whether it holds.
func check(t *testing.T, in instance, e expect, foodsBefore []string) bool {
	t.Helper()
	ctx := t.Context()
	ok := true
	fail := func(format string, args ...any) {
		t.Helper()
		t.Errorf(format, args...)
		ok = false
	}
	today := in.cal.DayOf(time.Now())
	meals, err := in.db.MealsOfDay(ctx, in.user.ID, today)
	if err != nil {
		t.Fatal(err)
	}
	items := 0
	for i := range meals {
		items += len(meals[i].Items)
	}
	kcal := meal.DayTotal(meals).Kcal
	if e.Meals != nil && len(meals) != *e.Meals {
		fail("%d meals, want %d", len(meals), *e.Meals)
	}
	if e.Items != nil && items != *e.Items {
		fail("%d items, want %d", items, *e.Items)
	}
	if len(e.Kcal) == 2 && (kcal < e.Kcal[0] || kcal > e.Kcal[1]) {
		fail("%.0f kcal, want %v", kcal, e.Kcal)
	}
	return checkFoods(t, in, e, foodsBefore) && checkDay(t, in, e, today) && ok
}

// checkFoods holds the foods made in the case to e.
func checkFoods(t *testing.T, in instance, e expect, foodsBefore []string) bool {
	t.Helper()
	ok := true
	var made []string
	for _, n := range foodNames(t, in) {
		if !slices.Contains(foodsBefore, n) {
			made = append(made, n)
		}
	}
	if e.NoNewFoods && len(made) > 0 {
		t.Errorf("new foods %v, want none", made)
		ok = false
	}
	for _, w := range e.NewFoods {
		if !slices.ContainsFunc(made, func(n string) bool { return strings.Contains(strings.ToLower(n), w) }) {
			t.Errorf("no new food of %q among %v", w, made)
			ok = false
		}
	}
	return ok
}

// checkDay holds today's weight, mood and hunger to e.
func checkDay(t *testing.T, in instance, e expect, today day.Date) bool {
	t.Helper()
	ctx := t.Context()
	ok := true
	fail := func(format string, args ...any) {
		t.Helper()
		t.Errorf(format, args...)
		ok = false
	}
	if e.Weight != 0 {
		values, err := in.db.Metrics(ctx, in.user.ID, today, today)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(values[today], func(v metric.Value) bool {
			return v.Kind == metric.Weight && math.Abs(v.Value-e.Weight) < 0.05
		}) {
			fail("weights %v, want %v", values[today], e.Weight)
		}
	}
	if e.Mood != nil || e.Hunger != nil {
		c, err := in.db.CheckInOf(ctx, in.user.ID, today)
		if err != nil {
			t.Fatal(err)
		}
		if !within(c.Mood, e.Mood) {
			fail("mood %d, want %v", c.Mood, e.Mood)
		}
		if !within(c.Hunger, e.Hunger) {
			fail("hunger %d, want %v", c.Hunger, e.Hunger)
		}
	}
	return ok
}

// within says whether v is in the range r of two, or r is not set.
func within(v int, r []int) bool {
	return len(r) != 2 || (v >= r[0] && v <= r[1])
}

func foodNames(t *testing.T, in instance) []string {
	t.Helper()
	rows, err := in.db.Read().QueryContext(t.Context(), "SELECT name FROM foods WHERE user_id = ?", in.user.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	return names
}

// drawLabel draws l as a nutrition label, black on white, as a photo of
// a package would show it.
func drawLabel(t *testing.T, l *label) []byte {
	t.Helper()
	lines := []string{
		l.Brand + " " + l.Name,
		"",
		"Nutrition facts",
		"per 100 g        per bar (" + fmt.Sprint(l.Portion) + " g)",
		fmt.Sprintf("Energy  %4.0f kcal   %4.0f kcal", l.Kcal, l.Kcal*l.Portion/100),
		fmt.Sprintf("Fat     %4.1f g      %4.1f g", l.Fat, l.Fat*l.Portion/100),
		fmt.Sprintf("Carbs   %4.1f g      %4.1f g", l.Carbs, l.Carbs*l.Portion/100),
		fmt.Sprintf("Protein %4.1f g      %4.1f g", l.Protein, l.Protein*l.Portion/100),
	}
	const w, h, scale = 320, 130, 3
	small := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(small, small.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	d := font.Drawer{Dst: small, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13}
	for i, line := range lines {
		d.Dot = fixed.P(10, 18+i*14)
		d.DrawString(line)
	}
	big := image.NewRGBA(image.Rect(0, 0, w*scale, h*scale))
	for y := range h * scale {
		for x := range w * scale {
			big.Set(x, y, small.At(x/scale, y/scale))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, big); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
