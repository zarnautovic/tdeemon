package api

import (
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/autopilot"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// A review of Monday, November 2: a cut losing 0.31 % a week against
// 0.50 %.
func reviewOf(outcome autopilot.Outcome, reason autopilot.Reason) autopilot.Review {
	return autopilot.Review{
		Date: day.Date{Year: 2026, Month: time.November, Day: 2}, Outcome: outcome, Reason: reason,
		StepKcal: -100, MeasuredPctWeek: -0.31, TargetPctWeek: -0.5, TDEE: 2540, HalfWidth: 310,
		TargetKcal: 2179, AfterKcal: 2079, LoggedDays: 3, WeighIns: 4, NeedsLoggedDays: 4, NeedsWeighIns: 3,
		StepsChangePct: -34, CarbsG: 63, DayType: "rest", BMI: 18.4, DriftKcal: -200, DriftLimitKcal: 390,
		Expected: day.Date{Year: 2026, Month: time.November, Day: 18}, Rule: "kcal_floor",
	}
}

func TestReviewMessages(t *testing.T) {
	t.Parallel()

	settings := autopilot.Settings{Mode: autopilot.Auto, Weekday: time.Monday}
	for _, tc := range []struct {
		outcome    autopilot.Outcome
		reason     autopilot.Reason
		en, hr     string
		pastDrift  bool
		wantSilent bool
	}{
		{autopilot.Applied, autopilot.ReasonCutSlow, "Target −100 kcal a day", "Cilj −100 kcal dnevno", false, false},
		{autopilot.Proposed, autopilot.ReasonCutSlow, "Suggests −100 kcal a day", "Predlaže −100 kcal dnevno", false, false},
		{autopilot.Proposed, autopilot.ReasonCutSlow, "past your drift limit (−200 of 390 kcal)", "preko granice odstupanja (−200 od 390 kcal)", true, false},
		{autopilot.Proposed, autopilot.ReasonUnderweight, "Your BMI is 18.4", "BMI ti je 18,4", false, false},
		{autopilot.Hold, autopilot.ReasonOnTarget, "Holding: pace on plan", "Bez promjene: tempo u planu", false, true},
		{autopilot.Hold, autopilot.ReasonIntake, "The target stays 2179 kcal a day.", "Cilj ostaje 2179 kcal dnevno.", false, true},
		{autopilot.Hold, autopilot.ReasonStepsGuard, "but your steps fell 34 %", "ali koraka je 34 % manje", false, true},
		{autopilot.Hold, autopilot.ReasonRejected, "You rejected its last step", "Zadnji korak je odbijen", false, true},
		{autopilot.Skipped, autopilot.ReasonFewData, "No step: 3/4 logged days", "Bez koraka: 3/4 upisana dana", false, true},
		{autopilot.Skipped, autopilot.ReasonNotTrusted, "about 2540 ± 310 kcal. A first step about Nov 18", "oko 2540 ± 310 kcal. Prvi korak oko 18. 11.", false, true},
		{autopilot.Skipped, autopilot.ReasonFirstWait, "No step: too soon after a change", "Bez koraka: prerano nakon promjene", false, true},
		{autopilot.Skipped, autopilot.ReasonInterval, "a step a week at most", "najviše korak tjedno", false, true},
		{autopilot.Skipped, autopilot.ReasonNoPlan, "Set a phase and a kcal target", "Postavi fazu i cilj kcal", false, true},
		{autopilot.Skipped, autopilot.ReasonPending, "Answer the proposal in your plan first", "Prvo odgovori na prijedlog", false, true},
		{autopilot.Skipped, autopilot.ReasonFewCarbs, "Rest days would have 63 g carbs", "Dani odmora bi imali 63 g UH", false, true},
		{autopilot.Skipped, autopilot.ReasonLimit, "(kcal_floor)", "(kcal_floor)", false, true},
		{autopilot.Skipped, autopilot.ReasonUnderweight, "No step: BMI under 18.5", "Bez koraka: BMI ispod 18,5", false, true},
	} {
		t.Run(string(tc.outcome)+" "+string(tc.reason), func(t *testing.T) {
			t.Parallel()

			r := reviewOf(tc.outcome, tc.reason)
			r.PastDrift = tc.pastDrift
			for locale, want := range map[string]string{"en": tc.en, "hr": tc.hr} {
				n := reviewNote(store.StoredReview{Review: r}, settings, locale)
				all := n.Text + "\n" + n.Push.Title + "\n" + n.Push.Body
				if !strings.Contains(all, want) {
					t.Errorf("%s: no %q in\n%s", locale, want, all)
				}
				if strings.Contains(all, "%!") {
					t.Errorf("%s: a format error in\n%s", locale, all)
				}
				if n.Push.Silent != tc.wantSilent || n.Push.Tag != "autopilot" || n.Push.URL != "/plan" ||
					(n.Message.Urgency == push.UrgencyLow) != tc.wantSilent {
					t.Errorf("%s: push %+v, %+v; want silent %v under the tag autopilot", locale, n.Push, n.Message, tc.wantSilent)
				}
			}
		})
	}
}

func TestReviewWordsCoverEveryLocale(t *testing.T) {
	t.Parallel()

	for _, l := range user.Locales() {
		if _, ok := reviewWords[l]; !ok {
			t.Errorf("no review's words in %q", l)
		}
	}
}

func TestTheMorningLineOfAReview(t *testing.T) {
	t.Parallel()

	r := reviewOf(autopilot.Applied, autopilot.ReasonCutSlow)
	if got, want := reviewLine(r, "hr"), "Autopilot: cilj −100 kcal dnevno, 2079 kcal dnevno"; got != want {
		t.Errorf("reviewLine = %q, want %q", got, want)
	}
	m := morningData{review: &r, reviewToday: true}
	if got := morningShort(m, "en"); !strings.Contains(got, "Autopilot: target −100 kcal a day") {
		t.Errorf("morningShort = %q, want the review's title", got)
	}
}
