package autopilot

import (
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/calibration"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// The made-up user the reviews are of: 180 cm, 90 kg (a BMI of 27.8),
// training on Monday, Wednesday and Friday, 2600 kcal on training days
// and 2400 on rest days (a weekly average of 2486), with 180 g of
// protein and 70 g of fat, so a rest day has 262 g of carbs.
var (
	person      = profile.Profile{Sex: profile.Male, BirthDate: day.Date{Year: 1990, Month: time.May, Day: 1}, HeightCm: 180}
	training, _ = plan.WeekdaysOf(time.Monday, time.Wednesday, time.Friday)
	// start is a Monday, the phase's first day.
	start      = day.Date{Year: 2026, Month: time.September, Day: 7}
	baseTarget = plan.Target{
		EffectiveFrom: start,
		Training:      plan.DayTarget{Kcal: 2600, Protein: 180, Fat: 70},
		Rest:          plan.DayTarget{Kcal: 2400, Protein: 180, Fat: 70},
	}
)

// user is a simulated user under review: the plan, the revisions the
// autopilot's history reads, and the data calibration measures.
type user struct {
	tb       testing.TB
	settings Settings
	phase    plan.Phase
	target   plan.Target
	kg       float64
	// measured is the rate calibration measures, in % a week.
	measured             float64
	loggedDays, weighIns int
	revisions            []Revision
	waiting              Waiting
	rejected             *float64
	estimate             func(*calibration.Estimate)
}

func newUser(tb testing.TB, mode Mode, ph plan.Phase) *user {
	tb.Helper()
	return &user{tb: tb, settings: Settings{Mode: mode, Weekday: time.Monday}, phase: ph, target: baseTarget, kg: 90, loggedDays: 7, weighIns: 7}
}

func (u *user) avg() float64 { return u.target.WeeklyAverage(training.Count()).Kcal }

// review decides the review on today, with a trusted estimate of the
// phase's own days whose TDEE is the one at which eating the target gives
// the measured rate: a plateau measures 0.
func (u *user) review(today day.Date) Review {
	u.tb.Helper()
	e := calibration.Estimate{
		Measured: true, Trusted: true, TrendKg: u.kg, KgPerWeek: plan.KgPerWeek(u.measured, u.kg),
		TDEE: u.avg() - plan.KcalPerDay(u.measured, u.kg), HalfWidth: 120, IntakeKcal: u.avg(),
		Body: calibration.Body{StepsRecent: 8000, StepsBefore: 8000},
		Now:  calibration.Now{LoggedDays: u.loggedDays, WeighIns: u.weighIns},
	}
	if u.estimate != nil {
		u.estimate(&e)
	}
	lv := limits.Levels{Instance: limits.Instance(), User: limits.UserDefaults()}
	v := calibration.Judge(e, &u.phase, &u.target, training, person, limits.Effective(lv.User, lv.Instance), calibration.DefaultParams())
	return Decide(Input{
		Today: today, Settings: u.settings, Estimate: e, Verdict: v,
		State:   calibration.StateOf(e, today, person, &u.phase, &u.target, training),
		Levels:  lv,
		History: HistoryOf(u.revisions, u.phase),
		Phases:  []plan.Phase{u.phase}, Targets: []plan.Target{u.target},
		Waiting: u.waiting, RejectedGapKcal: u.rejected,
	})
}

// apply applies r's target as by, as the store would.
func (u *user) apply(r Review, by By, today day.Date, tdee float64) {
	u.tb.Helper()
	if len(r.Changes.Targets) != 1 {
		u.tb.Fatalf("review on %s: changes %+v, want one target", today, r.Changes)
	}
	before := u.avg()
	u.target = *r.Changes.Targets[0].After
	u.revisions = append(u.revisions, Revision{Day: today, By: by, SetsKcal: true, DeltaKcal: u.avg() - before, TDEE: tdee})
}

func cut() plan.Phase { return plan.Phase{Type: plan.Cut, Start: start, RatePctWeek: -0.5} }

func TestAPlateauStepsWeeklyUntilTheDriftLimitThenProposes(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	// The plateau: the weight stands still on a cut of 0.5 % a week.
	u.measured = 0
	var outcomes []Outcome
	var stepDays []day.Date
	for week := range 6 {
		today := start.AddDays(7 * week)
		r := u.review(today)
		outcomes = append(outcomes, r.Outcome)
		checkPlateau(t, week, r)
		if r.Outcome == Applied {
			stepDays = append(stepDays, today)
			u.apply(r, ByAutopilot, today, r.TDEE)
		}
	}
	want := []Outcome{Skipped, Skipped, Applied, Applied, Applied, Proposed}
	if !equal(outcomes, want) {
		t.Fatalf("outcomes %v, want %v", outcomes, want)
	}
	for i := 1; i < len(stepDays); i++ {
		if n := stepDays[i].DaysSince(stepDays[i-1]); n < 7 {
			t.Errorf("steps %d days apart, want 7 at least", n)
		}
	}
	// The base is the TDEE at the first step, frozen: 15 % of 2486.
	h := HistoryOf(u.revisions, u.phase)
	if h.BaseFrom != stepDays[0] || h.DriftKcal != -300 {
		t.Errorf("history %+v, want the base from %s and a drift of -300", h, stepDays[0])
	}
}

// checkPlateau checks the review of a plateau's week: no step for the
// first 14 days, steps of 100 kcal, and a proposal past the drift.
func checkPlateau(t *testing.T, week int, r Review) {
	t.Helper()
	switch r.Outcome {
	case Applied:
		if r.Reason != ReasonCutSlow || r.StepKcal != -100 {
			t.Fatalf("week %d: %s %v kcal, want cut_slow -100", week, r.Reason, r.StepKcal)
		}
	case Proposed:
		if !r.PastDrift || r.StepKcal != -100 || r.DriftKcal != -400 {
			t.Errorf("week %d: proposed past drift %v, step %v, drift %v; want past it, -100, -400", week, r.PastDrift, r.StepKcal, r.DriftKcal)
		}
	case Hold:
	case Skipped:
		if r.Reason != ReasonFirstWait || r.From != start.AddDays(14) {
			t.Errorf("week %d: skipped for %s from %s, want first_wait from %s", week, r.Reason, r.From, start.AddDays(14))
		}
	}
}

func TestAStepComesAtLeastSevenDaysAfterTheLast(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	first := start.AddDays(14)
	r := u.review(first)
	if r.Outcome != Applied {
		t.Fatalf("first review: %s %s, want a step", r.Outcome, r.Reason)
	}
	u.apply(r, ByAutopilot, first, r.TDEE)
	// The user moved the review's day: a review 3 days later waits.
	u.settings.Weekday = time.Thursday
	r = u.review(first.AddDays(3))
	if r.Outcome != Skipped || r.Reason != ReasonInterval || r.From != first.AddDays(10) {
		t.Errorf("3 days later: %s %s from %s, want skipped for the interval from %s", r.Outcome, r.Reason, r.From, first.AddDays(10))
	}
	if r = u.review(first.AddDays(10)); r.Outcome != Applied {
		t.Errorf("10 days later: %s %s, want a step", r.Outcome, r.Reason)
	}
}

func TestProposeModeProposesEveryStep(t *testing.T) {
	t.Parallel()

	u := newUser(t, Propose, cut())
	for week := 2; week < 6; week++ {
		today := start.AddDays(7 * week)
		r := u.review(today)
		if r.Outcome != Proposed || r.PastDrift || r.StepKcal != -100 {
			t.Fatalf("week %d: %s, past drift %v, step %v; want a proposal of -100", week, r.Outcome, r.PastDrift, r.StepKcal)
		}
		// The user accepts it: a target they agreed to, so no drift.
		u.apply(r, ByAccepted, today, r.TDEE)
		if h := HistoryOf(u.revisions, u.phase); h.DriftKcal != 0 || h.AgreedOn != today || h.LastStep != today || h.WaitFrom != start {
			t.Fatalf("week %d: history %+v, want an agreed target and a step today", week, h)
		}
	}
}

func TestAPhaseChangeWaitsFourteenDays(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	r := u.review(start.AddDays(14))
	u.apply(r, ByAutopilot, start.AddDays(14), r.TDEE)
	// The user makes the cut faster three weeks in.
	changed := start.AddDays(21)
	u.phase.RatePctWeek = -0.7
	u.revisions = append(u.revisions, Revision{Day: changed, By: ByUser, ChangesPhase: true})
	for _, tc := range []struct {
		days int
		want Outcome
	}{{7, Skipped}, {13, Skipped}, {14, Applied}} {
		r := u.review(changed.AddDays(tc.days))
		if r.Outcome != tc.want {
			t.Errorf("%d days after the change: %s %s, want %s", tc.days, r.Outcome, r.Reason, tc.want)
		}
		if tc.want == Skipped && (r.Reason != ReasonFirstWait || r.From != changed.AddDays(14)) {
			t.Errorf("%d days after the change: %s from %s, want first_wait from %s", tc.days, r.Reason, r.From, changed.AddDays(14))
		}
	}
}

func TestARevertResetsTheDrift(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	for week := 2; week < 4; week++ {
		today := start.AddDays(7 * week)
		r := u.review(today)
		u.apply(r, ByAutopilot, today, r.TDEE)
	}
	if h := HistoryOf(u.revisions, u.phase); h.DriftKcal != -200 {
		t.Fatalf("drift %v after two steps, want -200", h.DriftKcal)
	}
	// The user takes the last step back: a target they agreed to.
	back := start.AddDays(24)
	u.target = u.target.ShiftKcal(100)
	u.revisions = append(u.revisions, Revision{Day: back, By: ByUser, SetsKcal: true, DeltaKcal: 100, TDEE: 2400})
	h := HistoryOf(u.revisions, u.phase)
	if h.DriftKcal != 0 || h.AgreedOn != back || h.WaitFrom != back || h.BaseTDEE != 2400 || h.BaseFrom != back {
		t.Errorf("history after the revert %+v, want no drift and a wait and a base from %s", h, back)
	}
	if r := u.review(back.AddDays(12)); r.Outcome != Skipped || r.Reason != ReasonFirstWait {
		t.Errorf("12 days after the revert: %s %s, want first_wait", r.Outcome, r.Reason)
	}
	r := u.review(back.AddDays(14))
	if r.Outcome != Applied || r.DriftKcal != -100 || r.DriftLimitKcal != 360 {
		t.Errorf("14 days after the revert: %s, drift %v of %v; want a step, -100 of 360 (15 %% of 2400)", r.Outcome, r.DriftKcal, r.DriftLimitKcal)
	}
}

func TestTooLittleDataTakesNoStep(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name               string
		logged, weighIns   int
		wantRule           string
		wantLogged, wantWI int
	}{
		{"3 logged days", 3, 7, limits.RuleLoggedDays, 3, 7},
		{"2 weigh-ins", 7, 2, limits.RuleWeighIns, 7, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := newUser(t, Auto, cut())
			u.loggedDays, u.weighIns = tc.logged, tc.weighIns
			r := u.review(start.AddDays(14))
			if r.Outcome != Skipped || r.Reason != ReasonFewData || r.Rule != tc.wantRule {
				t.Errorf("%s %s (%s), want skipped, few_data (%s)", r.Outcome, r.Reason, r.Rule, tc.wantRule)
			}
			if r.LoggedDays != tc.wantLogged || r.WeighIns != tc.wantWI || r.NeedsLoggedDays != 4 || r.NeedsWeighIns != 3 {
				t.Errorf("data %d/%d and %d/%d, want %d/4 and %d/3", r.LoggedDays, r.NeedsLoggedDays, r.WeighIns, r.NeedsWeighIns, tc.wantLogged, tc.wantWI)
			}
			if len(r.Changes.Targets) > 0 {
				t.Errorf("changes %+v, want none", r.Changes)
			}
		})
	}
}

func TestUnderweightProposesMaintenance(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	// 59 kg at 180 cm is a BMI of 18.2, still losing.
	u.kg, u.measured = 59, -0.4
	today := start.AddDays(35)
	r := u.review(today)
	if r.Outcome != Proposed || r.Reason != ReasonUnderweight || r.BMI >= limits.UnderweightBMI {
		t.Fatalf("%s %s at a BMI of %v, want a proposal of maintenance", r.Outcome, r.Reason, r.BMI)
	}
	ph := r.Changes.Phases
	if len(ph) != 2 || ph[0].After.End != today.AddDays(-1) || ph[1].After.Type != plan.Maintain || ph[1].Start != today {
		t.Fatalf("phases %+v, want the cut ending yesterday and maintenance from today", ph)
	}
	tg := r.Changes.Targets
	if len(tg) != 1 || r.StepKcal <= 0 || tg[0].After.WeeklyAverage(training.Count()).Kcal < r.TDEE-1 {
		t.Errorf("targets %+v, step %v, want the target raised to the TDEE %v", tg, r.StepKcal, r.TDEE)
	}
}

func TestABulkStepsUpAndDown(t *testing.T) {
	t.Parallel()

	bulk := plan.Phase{Type: plan.Bulk, Start: start, RatePctWeek: 0.25}
	for _, tc := range []struct {
		name     string
		measured float64
		reason   Reason
		step     float64
	}{
		{"too slow", 0.05, ReasonBulkSlow, 100},
		{"too fast", 0.6, ReasonBulkFast, -100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := newUser(t, Auto, bulk)
			u.measured = tc.measured
			r := u.review(start.AddDays(14))
			if r.Outcome != Applied || r.Reason != tc.reason || r.StepKcal != tc.step {
				t.Fatalf("%s %s %v, want applied %s %v", r.Outcome, r.Reason, r.StepKcal, tc.reason, tc.step)
			}
			after := r.Changes.Targets[0].After
			if after.Training.Kcal != 2600+tc.step || after.Rest.Kcal != 2400+tc.step || after.Rest.Protein != 180 || after.Rest.Fat != 70 {
				t.Errorf("target %+v, want both day types moved by %v and protein and fat kept", after, tc.step)
			}
		})
	}
}

func TestAStepDownThatLeavesTooFewCarbsIsNotTaken(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	// Rest days have 2000 - 720 - 630 = 650 kcal of carbs, 162.5 g; a step
	// of 100 leaves 137.5, and the training days' 2200 leave 187.5.
	u.target.Rest.Kcal = 2000
	u.target.Training.Kcal = 2200
	r := u.review(start.AddDays(14))
	if r.Outcome != Applied {
		t.Fatalf("with 137 g left: %s %s, want a step", r.Outcome, r.Reason)
	}
	u.apply(r, ByAutopilot, start.AddDays(14), r.TDEE)
	r = u.review(start.AddDays(21))
	if r.Outcome != Skipped || r.Reason != ReasonFewCarbs || r.DayType != plan.Rest || r.CarbsG != 112.75 {
		t.Errorf("%s %s, %v g on %q; want few_carbs, 112.75 g on rest days (a step of 99, 5 %% of 1986)", r.Outcome, r.Reason, r.CarbsG, r.DayType)
	}
}

func TestAfterARejectionTheGapMustGrowByAStep(t *testing.T) {
	t.Parallel()

	u := newUser(t, Propose, cut())
	today := start.AddDays(14)
	r := u.review(today)
	if r.Outcome != Proposed {
		t.Fatalf("%s %s, want a proposal", r.Outcome, r.Reason)
	}
	u.rejected = new(r.GapKcal)
	if r := u.review(today.AddDays(7)); r.Outcome != Hold || r.Reason != ReasonRejected {
		t.Errorf("the gap as it was: %s %s, want a hold for the rejection", r.Outcome, r.Reason)
	}
	// The weight now rises on the cut: the gap grew by more than a step.
	u.measured = 0.3
	if r := u.review(today.AddDays(14)); r.Outcome != Proposed {
		t.Errorf("a gap grown by a step: %s %s, want a proposal", r.Outcome, r.Reason)
	}
	// A gap the other way is no rejected one.
	u.measured = -1.2
	if r := u.review(today.AddDays(21)); r.Outcome != Proposed || r.StepKcal != 100 {
		t.Errorf("a gap the other way: %s %v, want a proposal up", r.Outcome, r.StepKcal)
	}
}

func TestReviewsThatTakeNoStep(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		setup  func(*user)
		want   Outcome
		reason Reason
	}{
		{"a coach's proposal waits", func(u *user) { u.waiting = WaitsOther }, Skipped, ReasonPending},
		{"its own proposal waits", func(u *user) { u.waiting = WaitsOwn }, Applied, ReasonCutSlow},
		{"not trusted", func(u *user) { u.estimate = func(e *calibration.Estimate) { e.Trusted = false } }, Skipped, ReasonNotTrusted},
		{"no estimate", func(u *user) { u.estimate = func(e *calibration.Estimate) { e.Measured = false } }, Skipped, ReasonNotTrusted},
		{"before the phase", func(u *user) { u.estimate = func(e *calibration.Estimate) { e.BeforePhase = true } }, Skipped, ReasonFirstWait},
		{"on target", func(u *user) { u.measured = -0.5 }, Hold, ReasonOnTarget},
		{"steps explain it", func(u *user) {
			u.estimate = func(e *calibration.Estimate) { e.Body.StepsRecent = 4000 }
		}, Hold, ReasonStepsGuard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := newUser(t, Auto, cut())
			tc.setup(u)
			r := u.review(start.AddDays(14))
			if r.Outcome != tc.want || r.Reason != tc.reason {
				t.Errorf("%s %s, want %s %s", r.Outcome, r.Reason, tc.want, tc.reason)
			}
		})
	}
}

func TestNoPlanTakesNoStep(t *testing.T) {
	t.Parallel()

	r := Decide(Input{Today: start, Settings: DefaultSettings(), Levels: limits.Levels{Instance: limits.Instance(), User: limits.UserDefaults()}})
	if r.Outcome != Skipped || r.Reason != ReasonNoPlan {
		t.Errorf("%s %s, want skipped for no plan", r.Outcome, r.Reason)
	}
}

func TestAHoldOfTheIntake(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	// The weight stands still, but the intake was 495 kcal over the
	// target: the target gives the pace.
	u.estimate = func(e *calibration.Estimate) {
		e.TDEE = u.avg() + plan.KcalPerDay(0.5, u.kg)
		e.IntakeKcal = e.TDEE
	}
	if r := u.review(start.AddDays(14)); r.Outcome != Hold || r.Reason != ReasonIntake {
		t.Errorf("%s %s, want a hold of the intake", r.Outcome, r.Reason)
	}
}

func TestRepeats(t *testing.T) {
	t.Parallel()

	hold := Review{Outcome: Hold, Reason: ReasonOnTarget, MeasuredPctWeek: -0.47, TargetKcal: 2179}
	few := Review{Outcome: Skipped, Reason: ReasonFewData, LoggedDays: 3, WeighIns: 4}
	step := Review{Outcome: Applied, Reason: ReasonCutSlow, StepKcal: -100}
	for _, tc := range []struct {
		name      string
		now, last Review
		want      bool
	}{
		{"the same hold", hold, hold, true},
		{"a hold at another pace", Review{Outcome: Hold, Reason: ReasonOnTarget, MeasuredPctWeek: -0.43, TargetKcal: 2179}, hold, false},
		{"the same data", few, few, true},
		{"other data", Review{Outcome: Skipped, Reason: ReasonFewData, LoggedDays: 2, WeighIns: 4}, few, false},
		{"no plan again", Review{Outcome: Skipped, Reason: ReasonNoPlan}, Review{Outcome: Skipped, Reason: ReasonNoPlan}, true},
		{"another reason", Review{Outcome: Skipped, Reason: ReasonNoPlan}, few, false},
		{"a step again", step, step, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.now.Repeats(tc.last); got != tc.want {
				t.Errorf("Repeats = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFirstStep(t *testing.T) {
	t.Parallel()

	h := History{WaitFrom: start, AgreedOn: start}
	l := limits.UserDefaults()
	// Monday the 7th: 14 days on is Monday the 21st; on Saturdays, the
	// 26th; and calibration expecting an estimate on Wednesday the 30th
	// moves it to Saturday, October 3.
	for _, tc := range []struct {
		name     string
		weekday  time.Weekday
		trusted  bool
		expected day.Date
		want     day.Date
	}{
		{"after the wait", time.Monday, true, day.Date{}, start.AddDays(14)},
		{"on another weekday", time.Saturday, true, day.Date{}, start.AddDays(19)},
		{"after calibration", time.Saturday, false, start.AddDays(23), start.AddDays(26)},
		{"a trusted TDEE needs no day", time.Monday, true, start.AddDays(23), start.AddDays(14)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := FirstStep(start, tc.weekday, h, l, tc.trusted, tc.expected); got != tc.want {
				t.Errorf("FirstStep = %s, want %s", got, tc.want)
			}
		})
	}
}

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSettingsValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		s    Settings
		ok   bool
	}{
		{"the defaults", DefaultSettings(), true},
		{"autopilot on Saturdays", Settings{Mode: Auto, Weekday: time.Saturday}, true},
		{"no mode", Settings{Weekday: time.Monday}, false},
		{"another mode", Settings{Mode: "always", Weekday: time.Monday}, false},
		{"a weekday past Saturday", Settings{Mode: Propose, Weekday: 7}, false},
		{"a weekday before Sunday", Settings{Mode: Propose, Weekday: -1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.s.Validate(); (err == nil) != tc.ok {
				t.Errorf("Validate() = %v, want ok %v", err, tc.ok)
			}
		})
	}
}

func TestNextOn(t *testing.T) {
	t.Parallel()

	// start is a Monday.
	for _, tc := range []struct {
		weekday time.Weekday
		want    day.Date
	}{{time.Monday, start}, {time.Tuesday, start.AddDays(1)}, {time.Sunday, start.AddDays(6)}} {
		if got := NextOn(start, tc.weekday); got != tc.want {
			t.Errorf("NextOn(%s, %s) = %s, want %s", start, tc.weekday, got, tc.want)
		}
	}
}

func TestAStepTheLimitsRefuseIsNotTaken(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	// A woman's floor is 1200 kcal; a man's 1500, which a step of 100
	// from 1550 on rest days breaks, with 6 g of fat left of carbs.
	u.target.Rest = plan.DayTarget{Kcal: 1550, Protein: 100, Fat: 20}
	r := u.review(start.AddDays(14))
	if r.Outcome != Skipped || r.Reason != ReasonLimit || r.Rule != limits.RuleKcalFloor {
		t.Errorf("%s %s (%s), want skipped for the kcal floor", r.Outcome, r.Reason, r.Rule)
	}
}

func TestARejectedMaintenanceHolds(t *testing.T) {
	t.Parallel()

	u := newUser(t, Auto, cut())
	u.kg, u.measured = 59, -0.4
	r := u.review(start.AddDays(35))
	u.rejected = new(r.GapKcal)
	if r := u.review(start.AddDays(42)); r.Outcome != Hold || r.Reason != ReasonRejected {
		t.Errorf("%s %s, want a hold for the rejection", r.Outcome, r.Reason)
	}
}

func TestStepReasons(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		t      plan.PhaseType
		action string
		step   float64
		want   Reason
	}{
		{plan.Cut, calibration.ActionLowerKcal, -100, ReasonCutSlow},
		{plan.Recomp, calibration.ActionRaiseKcal, 100, ReasonCutFast},
		{plan.Maintain, calibration.ActionRaiseKcal, 100, ReasonMaintainLoss},
		{plan.Maintain, calibration.ActionLowerKcal, -100, ReasonMaintainGain},
		{plan.Bulk, calibration.ActionShrinkSurplus, -100, ReasonBulkFat},
		{plan.Cut, calibration.ActionRaiseProteinOrKcal, 100, ReasonLeanLoss},
	} {
		if got := stepReason(tc.t, tc.action, tc.step); got != tc.want {
			t.Errorf("stepReason(%s, %s, %v) = %s, want %s", tc.t, tc.action, tc.step, got, tc.want)
		}
	}
}
