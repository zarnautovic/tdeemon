package api

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/scheduler"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// MorningSummaryJob is the morning summary, the scheduler's job: at
// 10:00 on each user's clock (or up to 4 hours later, a server down then
// catching up), it sends a user with a Telegram chat the summary of last
// night and yesterday (morningText). A user without a chat gets nothing.
func MorningSummaryJob(cfg Config, bot *telegram.Bot) scheduler.Job {
	s := &sessions{db: cfg.DB, logger: cfg.Logger, now: cfg.Now, instance: cfg.Limits}
	return scheduler.Job{
		Name: "morning_summary",
		Due:  scheduler.DailyAt(10*time.Hour, 4*time.Hour),
		Run: func(ctx context.Context, u user.User, _ string, now time.Time) error {
			chat, ok, err := s.db.TelegramChat(ctx, u.ID)
			if err != nil || !ok {
				return err
			}
			text, err := s.morning(ctx, u, now)
			if err != nil {
				return err
			}
			return bot.Send(ctx, chat, text)
		},
	}
}

// morning reads what the morning summary of u shows at now, and writes it.
func (s *sessions) morning(ctx context.Context, u user.User, now time.Time) (string, error) {
	p := auth.Principal{User: u}
	_, today, err := userDay(p, now)
	if err != nil {
		return "", err
	}
	td, err := s.readDay(ctx, p, today, now)
	if err != nil {
		return "", err
	}
	yd, err := s.readDay(ctx, p, today.AddDays(-1), now)
	if err != nil {
		return "", err
	}
	sum, err := s.summary(ctx, p, 1, now)
	if err != nil {
		return "", err
	}
	return morningText(today, td, yd, sum, u.Locale), nil
}

// morningText is the morning summary of today, from today so far (last
// night's sleep and this morning's weighing), yesterday and the summary:
// a line for each part that has something to say, in the language of
// locale (English, or Croatian for hr). A value that did not come is left
// out, and nothing is guessed: the sleep is only the night that ended
// today.
func morningText(today day.Date, td, yd dayBody, sum summaryBody, locale string) string {
	w := writer{phrasesOf(locale)}
	lines := []string{w.title + ", " + w.date(today)}
	for _, l := range []string{
		w.sleepLine(td), w.weightLine(td, sum, today), w.yesterdayLine(yd), w.activityLine(yd),
		w.calibrationLine(sum.Verdict), w.recoveryLine(sum.Recovery),
	} {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// phrases are the words of the morning summary in a language.
type phrases struct {
	title, sleep, weight, trend, since, bodyFat            string
	yesterday, trainingDay, excluded, nothingLogged        string
	activity, steps, active, exercise                      string
	calibration, noPlan, noEstimate, pace, perWeek, target string
	onTarget, below, above, proposal, perDay               string
	moreProteinOrKcal, smallerSurplus, toMaintenance       string
	recovery, shortSleep, average, highRestingHR, lowHRV   string
	weekdays                                               [7]string
	decimalComma                                           bool
	// date writes a day with its weekday, short without it.
	date, short func(p phrases, d day.Date) string
}

var english = phrases{
	title: "Morning summary", sleep: "Sleep", weight: "Weight", trend: "trend", since: "since", bodyFat: "body fat",
	yesterday: "Yesterday", trainingDay: "training day", excluded: "excluded from calibration", nothingLogged: "nothing logged",
	activity: "Activity yesterday", steps: "steps", active: "active", exercise: "exercise",
	calibration: "Calibration", noPlan: "no plan (phase and target), so no estimate of the rate",
	noEstimate: "no estimate to trust yet", pace: "rate", perWeek: "%/wk", target: "target",
	onTarget: "on plan", below: "below plan", above: "above plan", proposal: "proposed", perDay: "kcal/day",
	moreProteinOrKcal: "more protein or kcal", smallerSurplus: "a smaller surplus", toMaintenance: "move to maintenance",
	recovery: "Recovery", shortSleep: "short sleep", average: "average", highRestingHR: "high resting heart rate", lowHRV: "low HRV",
	weekdays: [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	date:     func(p phrases, d day.Date) string { return p.weekdays[weekday(d)] + " " + p.short(p, d) },
	short: func(_ phrases, d day.Date) string {
		return time.Month(d.Month).String()[:3] + " " + strconv.Itoa(d.Day)
	},
}

var croatian = phrases{
	title: "Jutarnji sažetak", sleep: "San", weight: "Težina", trend: "trend", since: "od", bodyFat: "masnoća",
	yesterday: "Jučer", trainingDay: "dan treninga", excluded: "izuzet iz kalibracije", nothingLogged: "ništa upisano",
	activity: "Aktivnost jučer", steps: "koraci", active: "aktivne", exercise: "vježbanje",
	calibration: "Kalibracija", noPlan: "nema plana (faze i cilja), pa nema ni procjene tempa",
	noEstimate: "još nema pouzdane procjene", pace: "tempo", perWeek: "%/tj", target: "cilj",
	onTarget: "u planu", below: "ispod plana", above: "iznad plana", proposal: "prijedlog", perDay: "kcal/dan",
	moreProteinOrKcal: "više proteina ili kcal", smallerSurplus: "manji suficit", toMaintenance: "prijeđi na održavanje",
	recovery: "Oporavak", shortSleep: "kratak san", average: "prosjek", highRestingHR: "povišen puls u mirovanju", lowHRV: "nizak HRV",
	weekdays:     [7]string{"nedjelja", "ponedjeljak", "utorak", "srijeda", "četvrtak", "petak", "subota"},
	decimalComma: true,
	date:         func(p phrases, d day.Date) string { return p.weekdays[weekday(d)] + " " + p.short(p, d) },
	short:        func(_ phrases, d day.Date) string { return fmt.Sprintf("%d. %d.", d.Day, int(d.Month)) },
}

// phrasesOf returns the phrases of locale, English for one without them.
func phrasesOf(locale string) phrases {
	if locale == "hr" {
		return croatian
	}
	return english
}

// writer writes the lines of the morning summary in its phrases.
type writer struct{ phrases }

func (w writer) date(d day.Date) string { return w.phrases.date(w.phrases, d) }

func (w writer) shortDate(d day.Date) string { return w.short(w.phrases, d) }

func metricOf(d dayBody, kind string) (float64, bool) {
	for _, m := range d.Metrics {
		if m.Kind == kind {
			return m.Value, true
		}
	}
	return 0, false
}

func (w writer) sleepLine(td dayBody) string {
	h, ok := metricOf(td, "sleep_h")
	if !ok {
		return ""
	}
	minutes := int(math.Round(h * 60))
	return fmt.Sprintf("%s: %d h %02d min", w.sleep, minutes/60, minutes%60)
}

func (w writer) weightLine(td dayBody, sum summaryBody, today day.Date) string {
	var parts []string
	if kg, ok := metricOf(td, "weight"); ok {
		parts = append(parts, w.num(kg, 1)+" kg")
	}
	if sum.Trend != nil {
		trend := w.trend + " " + w.num(sum.Trend.Kg, 1) + " kg"
		if d, err := day.ParseDate(sum.Trend.Date); err == nil && d != today && d != today.AddDays(-1) {
			trend += " (" + w.since + " " + w.shortDate(d) + ")"
		}
		parts = append(parts, trend)
	}
	if pct, ok := metricOf(td, "body_fat"); ok {
		parts = append(parts, w.bodyFat+" "+w.num(pct, 1)+" %")
	}
	if len(parts) == 0 {
		return ""
	}
	return w.weight + ": " + strings.Join(parts, " · ")
}

func (w writer) yesterdayLine(yd dayBody) string {
	label := w.yesterday
	if yd.Type == "training" {
		label += " (" + w.trainingDay + ")"
	}
	if yd.Status == "excluded" {
		label += " (" + w.excluded + ")"
	}
	if len(yd.Meals) == 0 {
		return label + ": " + w.nothingLogged
	}
	t := yd.Total
	part := func(name string, v, target float64, unit string) string {
		if yd.Target == nil {
			return name + w.num(v, 0) + unit
		}
		return name + w.num(v, 0) + "/" + w.num(target, 0) + unit
	}
	var tg dayTargetBody
	if yd.Target != nil {
		tg = *yd.Target
	}
	carbs := "C "
	fat := "F "
	if w.decimalComma {
		carbs, fat = "UH ", "M "
	}
	return label + ": " + strings.Join([]string{
		part("", t.Kcal, tg.Kcal, " kcal"), part("P ", t.Protein, tg.Protein, " g"),
		part(carbs, t.Carbs, tg.Carbs, " g"), part(fat, t.Fat, tg.Fat, " g"),
	}, " · ")
}

func (w writer) activityLine(yd dayBody) string {
	var parts []string
	if v, ok := metricOf(yd, "steps"); ok {
		parts = append(parts, w.steps+" "+w.num(v, 0))
	}
	if v, ok := metricOf(yd, "active_kcal"); ok {
		parts = append(parts, w.active+" "+w.num(v, 0)+" kcal")
	}
	if v, ok := metricOf(yd, "exercise_min"); ok {
		parts = append(parts, w.exercise+" "+w.num(v, 0)+" min")
	}
	if len(parts) == 0 {
		return ""
	}
	return w.activity + ": " + strings.Join(parts, " · ")
}

// calibrationLine says calibration's verdict in a line: why there is
// none, or the measured rate against the target and what it proposes.
func (w writer) calibrationLine(v verdictBody) string {
	label := w.calibration + ": "
	switch {
	case slices.Contains(v.Reasons, "no_phase") || slices.Contains(v.Reasons, "no_target"):
		return label + w.noPlan
	case v.Rate == "none" || v.MeasuredPctWeek == nil || v.TargetPctWeek == nil:
		return label + w.noEstimate
	}
	line := label + w.pace + " " + w.signed(*v.MeasuredPctWeek, 2) + " " + w.perWeek + ", " + w.target + " " + w.signed(*v.TargetPctWeek, 2) + " %"
	switch v.Rate {
	case "on_target":
		line += ", " + w.onTarget
	case "below":
		line += ", " + w.below
	case "above":
		line += ", " + w.above
	}
	if a := w.action(v); a != "" {
		line += "; " + w.proposal + ": " + a
	}
	return line
}

func (w writer) action(v verdictBody) string {
	switch v.Action {
	case "raise_kcal", "lower_kcal":
		return w.signed(math.Round(v.GapKcal), 0) + " " + w.perDay
	case "raise_protein_or_kcal":
		return w.moreProteinOrKcal
	case "shrink_surplus":
		return w.smallerSurplus
	case "maintain":
		return w.toMaintenance
	}
	return ""
}

// recoveryLine names what recovery flags, with its average.
func (w writer) recoveryLine(r recoveryBody) string {
	var parts []string
	for _, f := range r.Flags {
		switch {
		case f == "short_sleep" && r.SleepH != nil:
			parts = append(parts, w.shortSleep+" ("+w.average+" "+w.num(*r.SleepH, 1)+" h)")
		case f == "high_resting_hr" && r.RestingHR != nil:
			parts = append(parts, w.highRestingHR+" ("+w.num(*r.RestingHR, 0)+")")
		case f == "low_hrv" && r.HRV != nil:
			parts = append(parts, w.lowHRV+" ("+w.num(*r.HRV, 0)+" ms)")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return w.recovery + ": " + strings.Join(parts, " · ")
}

// num writes v with prec decimals, with a decimal comma where the
// language has one.
func (w writer) num(v float64, prec int) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if w.decimalComma {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// signed writes v as num does, with its sign once rounded: none for 0.
func (w writer) signed(v float64, prec int) string {
	p := math.Pow(10, float64(prec))
	switch r := math.Round(v*p) / p; {
	case r > 0:
		return "+" + w.num(r, prec)
	case r < 0:
		return "−" + w.num(-r, prec)
	}
	return w.num(0, prec)
}

func weekday(d day.Date) time.Weekday {
	return time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.UTC).Weekday()
}
