package api

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/autopilot"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/scheduler"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// morningTTL is how long a push service keeps the morning summary for a
// device that is off: past the evening it is old news.
const morningTTL = 12 * time.Hour

// MorningSummaryJob is the morning summary, the scheduler's job: at
// 10:00 on each user's clock (or up to 4 hours later, a server down then
// catching up), it sends the summary of last night and yesterday to each
// channel of a user's (notify.Channels): the whole of it to their
// Telegram chat (morningText), its short form to their devices
// (morningShort), a tap on which opens the whole in the app. A user
// without a channel gets nothing.
func MorningSummaryJob(cfg Config, bot *telegram.Bot) scheduler.Job {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &sessions{db: cfg.DB, logger: cfg.Logger, now: cfg.Now, instance: cfg.Limits}
	channels := notify.Channels{DB: cfg.DB, Bot: bot, Push: cfg.Push, Logger: cfg.Logger}
	return scheduler.Job{
		Name: "morning_summary",
		Due:  scheduler.DailyAt(10*time.Hour, 4*time.Hour),
		Run: func(ctx context.Context, u user.User, _ string, now time.Time) error {
			if ok, err := channels.Has(ctx, u); err != nil || !ok {
				return err
			}
			m, err := s.morningOf(ctx, u, now)
			if err != nil {
				return err
			}
			_, err = channels.Send(ctx, u, morningNote(m, u.Locale), now)
			if notify.Blocked(err) {
				// The user blocked the bot: trying again would not help.
				s.logger.Warn("the morning summary's chat refuses the bot", "user_id", u.ID, "error", err)
				return nil
			}
			return err
		},
	}
}

// morningData is what the morning summary shows: today so far (last
// night's sleep and this morning's weighing), yesterday, the summary, and
// the day a proposal of the plan waits for the user's answer until (the
// zero Date for none).
type morningData struct {
	date   day.Date
	td, yd dayBody
	sum    summaryBody
	waits  day.Date
	// review is the autopilot's last weekly review of the last 7 days,
	// nil for none, and reviewToday whether it is today's.
	review      *autopilot.Review
	reviewToday bool
}

// morning reads what the morning summary of u shows at now, and writes it
// whole.
func (s *sessions) morning(ctx context.Context, u user.User, now time.Time) (string, error) {
	m, err := s.morningOf(ctx, u, now)
	if err != nil {
		return "", err
	}
	return morningText(m.date, m.td, m.yd, m.sum, m.waits, m.review, u.Locale), nil
}

// morningOf reads what the morning summary of u shows at now. Last
// night's sleep and this morning's weighing are of the calendar date now,
// as a night is dated by the morning it ends: the logical day may be
// still yesterday's for a day that starts late. Yesterday is the last
// day that ended.
func (s *sessions) morningOf(ctx context.Context, u user.User, now time.Time) (morningData, error) {
	p := auth.Principal{User: u}
	cal, today, err := userDay(p, now)
	if err != nil {
		return morningData{}, err
	}
	m := morningData{date: day.DateOf(now.In(cal.Location))}
	if m.td, err = s.readDay(ctx, p, m.date, now); err != nil {
		return morningData{}, err
	}
	if m.yd, err = s.readDay(ctx, p, today.AddDays(-1), now); err != nil {
		return morningData{}, err
	}
	if m.sum, err = s.summary(ctx, p, 1, now); err != nil {
		return morningData{}, err
	}
	pending, err := s.db.Proposals(ctx, u.ID, 1, now, proposal.Pending)
	if err != nil {
		return morningData{}, err
	}
	if len(pending) > 0 && pending[0].Status == proposal.Pending {
		m.waits = day.DateOf(pending[0].ExpiresAt.In(cal.Location))
	}
	reviews, err := s.db.Reviews(ctx, u.ID, 1)
	if err != nil {
		return morningData{}, err
	}
	if len(reviews) > 0 && today.AddDays(-7).Before(reviews[0].Review.Date) && !today.Before(reviews[0].Review.Date) {
		m.review = &reviews[0].Review
		m.reviewToday = reviews[0].Review.Date == today
	}
	return m, nil
}

// morningNote is the morning summary of m as a notification, in the
// language of locale: whole for Telegram, short for a device, whose tap
// opens the whole on the day (the web app's /?summary=date). A newer one
// replaces it at the push service and on the device.
func morningNote(m morningData, locale string) notify.Note {
	w := writer{phrasesOf(locale)}
	return notify.Note{
		Text: morningText(m.date, m.td, m.yd, m.sum, m.waits, m.review, locale),
		Push: push.Notification{
			Title: w.title, Body: morningShort(m, locale), URL: "/?summary=" + m.date.String(), Tag: "morning",
		},
		Message: push.Message{TTL: morningTTL, Topic: "morning"},
	}
}

// morningShort is the morning summary in a line, for a notification a
// device shows in a few: yesterday's kcal against the target, this
// morning's weight, last night's sleep, and a proposal that waits; what
// did not come is left out.
func morningShort(m morningData, locale string) string {
	w := writer{phrasesOf(locale)}
	var parts []string
	switch {
	case len(m.yd.Meals) == 0:
		parts = append(parts, w.yesterday+": "+w.nothingLogged)
	case m.yd.Target != nil:
		parts = append(parts, w.yesterday+" "+w.num(m.yd.Total.Kcal, 0)+"/"+w.num(m.yd.Target.Kcal, 0)+" kcal")
	default:
		parts = append(parts, w.yesterday+" "+w.num(m.yd.Total.Kcal, 0)+" kcal")
	}
	if kg, ok := metricOf(m.td, "weight"); ok {
		parts = append(parts, w.num(kg, 1)+" kg")
	}
	if h, ok := metricOf(m.td, "sleep_h"); ok {
		minutes := int(math.Round(h * 60))
		parts = append(parts, fmt.Sprintf("%s %d h %02d min", strings.ToLower(w.sleep), minutes/60, minutes%60))
	}
	if !m.waits.IsZero() {
		parts = append(parts, w.proposalWaits)
	}
	if m.review != nil && m.reviewToday {
		parts = append(parts, "Autopilot: "+lowerFirst(newReviewWriter(locale).title(*m.review)))
	}
	return strings.Join(parts, " · ")
}

// morningText is the morning summary of today, from today so far (last
// night's sleep and this morning's weighing), yesterday, the summary and
// waits, the day a proposal of the plan waits for the user's answer until
// (the zero Date for none), and review, the autopilot's last weekly
// review (nil for none): a line for each part that has something to
// say, in the language of locale (English, or Croatian for hr). A value
// that did not come is left out, and nothing is guessed: the sleep is
// only the night that ended today.
func morningText(today day.Date, td, yd dayBody, sum summaryBody, waits day.Date, review *autopilot.Review, locale string) string {
	w := writer{phrasesOf(locale)}
	lines := []string{w.title + ", " + w.date(today)}
	for _, l := range []string{
		w.sleepLine(td), w.weightLine(td, sum, today), w.yesterdayLine(yd), w.activityLine(yd),
		w.calibrationLine(sum.Verdict), autopilotLine(review, locale), w.planLine(waits), w.recoveryLine(sum.Recovery),
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
	plan, waits, proposalWaits                             string
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
	calibration: "Calibration", noPlan: "no plan (a phase and a target), so no rate to judge",
	noEstimate: "no estimate to trust yet", pace: "rate", perWeek: "%/wk", target: "target",
	onTarget: "on plan", below: "below plan", above: "above plan", proposal: "proposed", perDay: "kcal/day",
	plan: "Plan", waits: "a proposed change waits for your answer, until", proposalWaits: "a change of the plan waits",
	moreProteinOrKcal: "more protein or kcal", smallerSurplus: "a smaller surplus", toMaintenance: "move to maintenance",
	recovery: "Recovery", shortSleep: "short sleep", average: "average", highRestingHR: "high resting heart rate", lowHRV: "low HRV",
	weekdays: [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	date:     func(p phrases, d day.Date) string { return p.weekdays[weekday(d)] + " " + p.short(p, d) },
	short: func(_ phrases, d day.Date) string {
		return time.Month(d.Month).String()[:3] + " " + strconv.Itoa(d.Day)
	},
}

var croatian = phrases{
	title: "Jutarnji sažetak", sleep: "San", weight: "Težina", trend: "trend", since: "od", bodyFat: "tjelesna mast",
	yesterday: "Jučer", trainingDay: "dan treninga", excluded: "izuzet iz kalibracije", nothingLogged: "ništa upisano",
	activity: "Aktivnost jučer", steps: "koraci", active: "aktivne", exercise: "vježbanje",
	calibration: "Kalibracija", noPlan: "nema plana (faze i cilja), pa nema tempa za ocijeniti",
	noEstimate: "još nema pouzdane procjene", pace: "tempo", perWeek: "%/tj", target: "cilj",
	onTarget: "u planu", below: "ispod plana", above: "iznad plana", proposal: "prijedlog", perDay: "kcal/dan",
	plan: "Plan", waits: "prijedlog promjene čeka tvoj odgovor, do", proposalWaits: "promjena plana čeka",
	moreProteinOrKcal: "više proteina ili kcal", smallerSurplus: "manji suficit", toMaintenance: "prijeđi na održavanje",
	recovery: "Oporavak", shortSleep: "kratak san", average: "prosjek", highRestingHR: "povišen puls u mirovanju", lowHRV: "nizak HRV",
	weekdays:     [7]string{"nedjelja", "ponedjeljak", "utorak", "srijeda", "četvrtak", "petak", "subota"},
	decimalComma: true,
	date:         func(p phrases, d day.Date) string { return p.weekdays[weekday(d)] + " " + p.short(p, d) },
	short:        func(_ phrases, d day.Date) string { return fmt.Sprintf("%d. %d.", d.Day, int(d.Month)) },
}

// morningWords are the summary's phrases in each language a user can have
// (user.Locales), which a test holds to.
var morningWords = map[string]phrases{"en": english, "hr": croatian}

// phrasesOf returns the phrases of locale, English for one without them.
func phrasesOf(locale string) phrases {
	if p, ok := morningWords[locale]; ok {
		return p
	}
	return english
}

// writer writes the lines of the morning summary in its phrases.
type writer struct{ phrases }

func (w writer) date(d day.Date) string { return w.phrases.date(w.phrases, d) }

func (w writer) shortDate(d day.Date) string { return w.short(w.phrases, d) }

// metricOf returns the day's value of kind, and whether it has one: a 0
// is none, as calibration reads it (no sleep, no steps).
func metricOf(d dayBody, kind string) (float64, bool) {
	for _, m := range d.Metrics {
		if m.Kind == kind && m.Value > 0 {
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
	var notes []string
	if yd.Type == "training" {
		notes = append(notes, w.trainingDay)
	}
	if yd.Status == "excluded" {
		notes = append(notes, w.excluded)
	}
	if len(notes) > 0 {
		label += " (" + strings.Join(notes, ", ") + ")"
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
	line := label + w.pace + " " + w.signed(*v.MeasuredPctWeek, 2) + " " + w.perWeek + ", " + w.target + " " + w.signed(*v.TargetPctWeek, 2) + " " + w.perWeek
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

// autopilotLine is the line of the autopilot's last review, none for
// nil.
func autopilotLine(r *autopilot.Review, locale string) string {
	if r == nil {
		return ""
	}
	return reviewLine(*r, locale)
}

// recoveryLine names what recovery flags, with its average.
// planLine says that a proposal of the plan waits for the user's answer,
// until the day waits; none for no proposal.
func (w writer) planLine(waits day.Date) string {
	if waits.IsZero() {
		return ""
	}
	return w.plan + ": " + w.waits + " " + w.shortDate(waits)
}

func (w writer) recoveryLine(r recoveryBody) string {
	var parts []string
	for _, f := range r.Flags {
		switch {
		case f == "short_sleep" && r.SleepH != nil:
			parts = append(parts, w.shortSleep+" ("+w.average+" "+w.num(*r.SleepH, 1)+" h)")
		case f == "high_resting_hr" && r.RestingHR != nil:
			parts = append(parts, w.highRestingHR+" ("+w.num(*r.RestingHR, 0)+" bpm)")
		case f == "low_hrv" && r.HRV != nil:
			parts = append(parts, w.lowHRV+" ("+w.num(*r.HRV, 0)+" ms)")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return w.recovery + ": " + strings.Join(parts, " · ")
}

// num writes v rounded to prec decimals, halves away from 0 as the rest
// of tdeemon rounds, with a decimal comma where the language has one.
func (w writer) num(v float64, prec int) string {
	p := math.Pow(10, float64(prec))
	s := strconv.FormatFloat(math.Round(v*p)/p, 'f', prec, 64)
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

type morningInput struct {
	Date string `query:"date" format:"date" doc:"The morning whose summary, on the user's calendar; today when absent."`
}

type morningOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Date  string `json:"date" format:"date" doc:"The morning's date."`
		Title string `json:"title" example:"Morning summary" doc:"What the summary is, in the user's language."`
		Text  string `json:"text" doc:"The summary whole, a part a line, as the Telegram chat gets it; its first line is the title and the date."`
		Short string `json:"short" doc:"The summary in a line, as a device's notification shows it."`
	}
}

// getMorning writes the morning summary of a date as it reads now: what
// a tap on the morning's notification opens. Today's is of now; one of
// before is of 10:00 that morning, as the job wrote it, with what was
// logged since, and without the line of a proposal that waited: the
// proposals keep no state of before, so one made since, or answered,
// would read as waiting then.
func (s *sessions) getMorning(ctx context.Context, in *morningInput) (*morningOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	cal, _, err := userDay(p, s.now())
	if err != nil {
		return nil, internalError("reading the user's calendar failed", err)
	}
	now := s.now()
	today := day.DateOf(now.In(cal.Location))
	at := now
	if in.Date != "" {
		d, err := day.ParseDate(in.Date)
		switch {
		case err != nil:
			return nil, huma.Error422UnprocessableEntity("the date is not a date")
		case today.Before(d):
			return nil, huma.Error422UnprocessableEntity("the morning of " + in.Date + " has not come yet")
		case d.Before(today):
			at = time.Date(d.Year, d.Month, d.Day, 10, 0, 0, 0, cal.Location)
		}
	}
	m, err := s.morningOf(ctx, p.User, at)
	if err != nil {
		return nil, internalError("reading the morning summary failed", err)
	}
	if at != now {
		m.waits = day.Date{}
	}
	n := morningNote(m, p.User.Locale)
	out := &morningOutput{CacheControl: "no-store"}
	out.Body.Date, out.Body.Title, out.Body.Text, out.Body.Short = m.date.String(), n.Push.Title, n.Text, n.Push.Body
	return out, nil
}

func (s *sessions) registerMorning(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "get-morning-summary",
		Method:      http.MethodGet,
		Path:        Prefix + "/morning",
		Summary:     "Get the morning summary",
		Description: "Needs a web session. The morning summary of a date, whole and in a line, as the notifications send it at 10:00, " +
			"read now: with what was logged since.",
		Tags:   []string{"summary"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, s.getMorning)
}
