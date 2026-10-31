package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/autopilot"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/scheduler"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// chat is a made-up Telegram API that keeps the messages sent.
type chat struct {
	mu   sync.Mutex
	sent []string
}

func newChat(t *testing.T) (*chat, *telegram.Bot) {
	t.Helper()
	c := &chat{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		text, _ := body["text"].(string)
		c.mu.Lock()
		c.sent = append(c.sent, text)
		c.mu.Unlock()
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	return c, &telegram.Bot{Token: "123456:made-up-token-for-tests-only-0123", BaseURL: srv.URL}
}

func (c *chat) messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sent...)
}

// reviewUser makes a user of locale with a Telegram chat, the history of
// saveSummaryHistory (a cut losing slower than planned, with a TDEE
// calibration trusts) when withPlan, and the autopilot's settings on
// mode with the review on now's weekday.
func reviewUser(t *testing.T, db *store.DB, email, locale string, mode autopilot.Mode, withPlan bool, now time.Time) user.User {
	t.Helper()
	u := user.User{
		ID: uuid.NewV7(), Email: email, Name: "Ana", Role: user.RoleUser, Locale: locale,
		TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), u, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTelegramChat(t.Context(), u.ID, 853, now); err != nil {
		t.Fatal(err)
	}
	if withPlan {
		saveReviewPlan(t, db, u, now)
	}
	if err := db.SetAutopilotSettings(t.Context(), u.ID, isolationActor, autopilot.Settings{Mode: mode, Weekday: now.Weekday()}, now); err != nil {
		t.Fatal(err)
	}
	return u
}

// saveReviewPlan gives u a profile, of a man of 180 cm, and the history
// of saveSummaryHistory.
func saveReviewPlan(t *testing.T, db *store.DB, u user.User, now time.Time) {
	t.Helper()
	pr := profile.Profile{Sex: profile.Male, BirthDate: day.Date{Year: 1990, Month: time.May, Day: 1}, HeightCm: 180}
	if err := db.SetProfile(t.Context(), u.ID, isolationActor, pr, day.DateOf(now), now); err != nil {
		t.Fatal(err)
	}
	saveSummaryHistory(t, db, u, now)
}

// wantDueAtNine checks that job is due for u at 09:00 of now's day, not
// before, and returns the occurrence.
func wantDueAtNine(t *testing.T, job scheduler.Job, u user.User, now time.Time) string {
	t.Helper()
	if _, due := job.Due(u, now.Add(-10*time.Minute)); due {
		t.Error("due before 09:00")
	}
	occurrence, due := job.Due(u, now)
	if !due || occurrence != day.DateOf(now).String() {
		t.Fatalf("Due at 09:05 = %q, %v", occurrence, due)
	}
	return occurrence
}

// wantIn checks that text has each of wants.
func wantIn(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
}

func TestAutopilotReviewJobTakesAStep(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	chat, bot := newChat(t)
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 10, 26, 9, 5, 0, 0, zagreb) // a Monday
	ana := reviewUser(t, db, "ana@example.com", "en", autopilot.Auto, true, now)
	job := api.AutopilotReviewJob(api.Config{DB: db, Limits: limits.Instance(), Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return now }}, bot)
	occurrence := wantDueAtNine(t, job, ana, now)
	if err := job.Run(t.Context(), ana, occurrence, now); err != nil {
		t.Fatal(err)
	}
	targets, err := db.Targets(t.Context(), ana.ID)
	if err != nil {
		t.Fatal(err)
	}
	if last := targets[len(targets)-1]; last.EffectiveFrom.String() != "2026-10-26" || last.Rest.Kcal != 2300 {
		t.Fatalf("the target now %+v, want 2300 kcal from today", last)
	}
	sent := chat.messages()
	if len(sent) != 1 {
		t.Fatalf("sent %q, want one message", sent)
	}
	wantIn(t, sent[0], "Autopilot · weekly review, Monday Oct 26", "Target −100 kcal a day", "New average 2300 kcal a day",
		"Drift: −100 of", "Take it back in the plan: /plan")
	// A run again, as after a crash, changes and sends nothing.
	if err := job.Run(t.Context(), ana, occurrence, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Targets(t.Context(), ana.ID); len(got) != len(targets) || len(chat.messages()) != 1 {
		t.Error("a second run of the day stepped or sent again")
	}
	// The morning summary says it.
	morning := api.MorningSummaryJob(api.Config{DB: db, Logger: slog.New(slog.DiscardHandler)}, bot)
	if err := morning.Run(t.Context(), ana, occurrence, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if sent := chat.messages(); len(sent) != 2 || !strings.Contains(sent[1], "Autopilot: target −100 kcal a day, 2300 kcal a day") {
		t.Errorf("the morning summary %q, want the autopilot's line", sent[len(sent)-1])
	}
}

func TestAutopilotReviewJobProposesAndWaitsForItsDay(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	chat, bot := newChat(t)
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 10, 26, 9, 5, 0, 0, zagreb)
	ivo := reviewUser(t, db, "ivo@example.com", "hr", autopilot.Propose, true, now)
	job := api.AutopilotReviewJob(api.Config{DB: db, Limits: limits.Instance(), Logger: slog.New(slog.DiscardHandler)}, bot)
	// Not its weekday: nothing.
	if err := job.Run(t.Context(), ivo, "2026-10-27", now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(chat.messages()) != 0 {
		t.Fatal("a review on another weekday")
	}
	if err := job.Run(t.Context(), ivo, "2026-10-26", now); err != nil {
		t.Fatal(err)
	}
	pending, err := db.Proposals(t.Context(), ivo.ID, 1, now, proposal.Pending)
	if err != nil || len(pending) != 1 || pending[0].Rationale != proposal.Autopilot {
		t.Fatalf("pending %+v, %v; want the autopilot's proposal", pending, err)
	}
	sent := chat.messages()
	if len(sent) != 1 || !strings.Contains(sent[0], "Predlaže −100 kcal dnevno") || !strings.Contains(sent[0], "do 2. 11.") {
		t.Errorf("sent %q, want the proposal in Croatian", sent)
	}
}

func TestAutopilotReviewJobSaysARepeatOnce(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	chat, bot := newChat(t)
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 10, 26, 9, 5, 0, 0, zagreb)
	ana := reviewUser(t, db, "ana@example.com", "en", autopilot.Auto, false, now)
	job := api.AutopilotReviewJob(api.Config{DB: db, Limits: limits.Instance(), Logger: slog.New(slog.DiscardHandler)}, bot)
	for week := range 2 {
		at := now.AddDate(0, 0, 7*week)
		if err := job.Run(t.Context(), ana, day.DateOf(at).String(), at); err != nil {
			t.Fatal(err)
		}
	}
	if sent := chat.messages(); len(sent) != 1 || !strings.Contains(sent[0], "No step: no phase or target") {
		t.Errorf("sent %q, want one message of no plan", sent)
	}
	reviews, err := db.Reviews(t.Context(), ana.ID, 5)
	if err != nil || len(reviews) != 2 {
		t.Errorf("reviews %+v, %v; want both stored", reviews, err)
	}
}

func TestAutopilotSettingsInTheApp(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	var got struct {
		Mode, Weekday, Next string
		FirstStep           *string `json:"first_step"`
		Drift               struct {
			UsedKcal  float64  `json:"used_kcal"`
			LimitPct  float64  `json:"limit_pct"`
			LimitKcal *float64 `json:"limit_kcal"`
		}
	}
	rec := in.do(t, http.MethodGet, "/api/v1/autopilot", "", session, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got.Mode != "propose" || got.Weekday != "monday" || got.Drift.LimitPct != 15 || got.Drift.LimitKcal != nil || got.FirstStep != nil {
		t.Errorf("the defaults %+v", got)
	}
	rec = in.do(t, http.MethodPut, "/api/v1/autopilot", `{"mode":"autopilot","weekday":"saturday"}`, session, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil || got.Mode != "autopilot" || got.Weekday != "saturday" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if next, err := day.ParseDate(got.Next); err != nil || next.Weekday() != time.Saturday {
		t.Errorf("next %q, want a Saturday", got.Next)
	}
	if rec := in.do(t, http.MethodPut, "/api/v1/autopilot", `{"weekday":"someday"}`, session, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an unknown weekday: %d %s", rec.Code, rec.Body.String())
	}
	wantCoachReadsOnly(t, in, session[0])
}

// wantCoachReadsOnly checks that a coach's token reads the autopilot's
// mode in the plan, and neither reads nor changes its settings.
func wantCoachReadsOnly(t *testing.T, in *instance, session *http.Cookie) {
	t.Helper()
	token, _ := in.createToken(t, session, `{"name":"coach","scopes":["read","tools"]}`)
	for _, m := range []string{http.MethodGet, http.MethodPut} {
		if rec := in.do(t, m, "/api/v1/autopilot", `{"mode":"propose"}`, nil, bearer(token.Token)); rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with a token: %d, want it refused", m, rec.Code)
		}
	}
	var plan struct {
		Autopilot struct{ Mode, Next string }
	}
	rec := in.do(t, http.MethodGet, "/api/v1/plan", "", nil, bearer(token.Token))
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); rec.Code != http.StatusOK || err != nil || plan.Autopilot.Mode != "autopilot" {
		t.Errorf("the plan's autopilot %+v: %d %s", plan.Autopilot, rec.Code, rec.Body.String())
	}
}

// The plan gives the autopilot's step its review.
func TestThePlanGivesTheStepsReview(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	now := in.clock.Now()
	saveReviewPlan(t, in.db, in.user, now)
	if err := in.db.SetAutopilotSettings(t.Context(), in.user.ID, isolationActor, autopilot.Settings{Mode: autopilot.Auto, Weekday: userToday(t, in).Weekday()}, now); err != nil {
		t.Fatal(err)
	}
	job := api.AutopilotReviewJob(api.Config{DB: in.db, Limits: limits.Instance(), Logger: slog.New(slog.DiscardHandler)}, nil)
	if err := job.Run(t.Context(), in.user, userToday(t, in).String(), now); err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Revisions []struct {
			Rationale string
			By        struct{ Client string }
			CheckIn   *struct {
				Outcome  string  `json:"outcome"`
				Reason   string  `json:"reason"`
				StepKcal float64 `json:"step_kcal"`
			} `json:"check_in"`
		}
		Autopilot struct {
			DriftKcal float64 `json:"drift_kcal"`
		}
	}
	rec := in.do(t, http.MethodGet, "/api/v1/plan", "", session, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); rec.Code != http.StatusOK || err != nil || len(plan.Revisions) == 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	r := plan.Revisions[0]
	if r.Rationale != "autopilot" || r.By.Client != "autopilot" || r.CheckIn == nil || r.CheckIn.Outcome != "applied" ||
		r.CheckIn.Reason != "cut_slow" || r.CheckIn.StepKcal != -100 || plan.Autopilot.DriftKcal != -100 {
		t.Errorf("the step %+v, drift %v", r, plan.Autopilot.DriftKcal)
	}
	wantLastApplied(t, in, session)
}

// wantLastApplied checks that the autopilot's last review is the step,
// and undo leaves it.
func wantLastApplied(t *testing.T, in *instance, session []*http.Cookie) {
	t.Helper()
	var ap struct {
		Last *struct{ Outcome string } `json:"last"`
	}
	rec := in.do(t, http.MethodGet, "/api/v1/autopilot", "", session, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &ap); err != nil || ap.Last == nil || ap.Last.Outcome != "applied" {
		t.Errorf("the autopilot's last review: %s", rec.Body.String())
	}
	if rec := in.do(t, http.MethodPost, "/api/v1/undo", "", session, nil); rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "target") {
		t.Errorf("undo took the step back: %s", rec.Body.String())
	}
}
