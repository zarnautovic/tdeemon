package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/autopilot"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// reviewTTL is how long a push service keeps a review's message for a
// device that is off: the next review replaces it.
const reviewTTL = 7 * 24 * time.Hour

// reviewPhrases are the words of the weekly review's messages in a
// language (docs: the autopilot's design, §4). The Croatian addresses
// the user without a gender.
type reviewPhrases struct {
	event, takeBack                                  string
	targetDay, suggests, maintenance                 string
	holdPace, holdIntake, holdSteps, holdRejected    string
	noStep, fewLogged, fewWeighIns, notTrusted       string
	tooSoon, interval, noPlan, waits, fewCarbs       string
	limit, underweight                               string
	losing, gaining, plan, newAvg, kcalDay, tapStep  string
	answerBy, pastDrift, targetStays, stepsFell      string
	stepsRose, needs, nextReview, tdeeAbout, firstAt string
	ifLogging, firstFrom, setPlan, answerFirst       string
	carbsLeft, askCoach, bmiUnder, rejectedBody      string
	limitBody, drift, of, takeBackAt, weekStepsCarbs string
	training, rest, everyDay                         string
}

var reviewEnglish = reviewPhrases{
	event: "weekly review", takeBack: "Take it back in the plan: /plan",
	targetDay: "Target %s kcal a day", suggests: "Suggests %s kcal a day", maintenance: "Maintenance suggested",
	holdPace: "Holding: pace on plan", holdIntake: "Holding: the target fits", holdSteps: "Holding: steps explain it",
	holdRejected: "Holding: the target you chose",
	noStep:       "No step", fewLogged: "%d/%d logged days", fewWeighIns: "%d/%d weigh-ins", notTrusted: "TDEE not trusted yet",
	tooSoon: "too soon after a change", interval: "a step a week at most", noPlan: "no phase or target", waits: "a proposal waits",
	fewCarbs: "too few carbs", limit: "a limit", underweight: "BMI under 18.5",
	losing: "losing", gaining: "gaining", plan: "plan", newAvg: "New average %s kcal a day.", kcalDay: "kcal a day",
	tapStep: "Tap to see why, or take it back.", answerBy: "Accept or reject in the plan by %s.",
	pastDrift:   "It's past your drift limit (%s of %s kcal), so it asks first. Answer by %s.",
	targetStays: "The target stays %s kcal a day.", stepsFell: "but your steps fell %s %%", stepsRose: "but your steps rose %s %%",
	needs: "A step needs %d logged days and %d weigh-ins in the last 7.", nextReview: "Next review %s.",
	tdeeAbout: "Your TDEE is about %s ± %s kcal.", firstAt: "A first step about %s, if you keep logging as now.",
	ifLogging: "Keep logging and weighing in, and calibration measures it.", firstFrom: "The first step can come from %s.",
	setPlan:     "Set a phase and a kcal target in your plan, and the review judges your pace against them.",
	answerFirst: "Answer the proposal in your plan first; the review waits for it.",
	carbsLeft:   "%s would have %s g carbs, and it never changes protein or fat.", askCoach: "Ask your coach to change the split.",
	bmiUnder:     "Your BMI is %s, under 18.5, so the autopilot won't take the loss further. Accept or reject in the plan.",
	rejectedBody: "You rejected its last step: the target stays as you chose until the gap grows by a step.",
	limitBody:    "The step would break your limits (%s); your coach can look at the plan.",
	drift:        "Drift", of: "of", takeBackAt: "Take it back in the plan",
	weekStepsCarbs: "Both types of day move by %s kcal, all of it carbs (%s g): %s → %s kcal a day on average.",
	training:       "Training days", rest: "Rest days", everyDay: "Every day",
}

var reviewCroatian = reviewPhrases{
	event: "tjedni pregled", takeBack: "Vrati u planu: /plan",
	targetDay: "Cilj %s kcal dnevno", suggests: "Predlaže %s kcal dnevno", maintenance: "Predlaže održavanje",
	holdPace: "Bez promjene: tempo u planu", holdIntake: "Bez promjene: cilj odgovara", holdSteps: "Bez promjene: koraci",
	holdRejected: "Bez promjene: cilj ostaje tvoj",
	noStep:       "Bez koraka", fewLogged: "%d/%d upisana dana", fewWeighIns: "%d/%d vaganja", notTrusted: "TDEE još nije pouzdan",
	tooSoon: "prerano nakon promjene", interval: "najviše korak tjedno", noPlan: "nema faze ni cilja", waits: "prijedlog čeka",
	fewCarbs: "premalo UH", limit: "granica", underweight: "BMI ispod 18,5",
	losing: "tempo", gaining: "tempo", plan: "plan", newAvg: "Novi prosjek %s kcal dnevno.", kcalDay: "kcal dnevno",
	tapStep: "Dodirni za razlog ili vraćanje.", answerBy: "Prihvati ili odbij u planu do %s",
	pastDrift:   "To je preko granice odstupanja (%s od %s kcal), pa pita prvo. Odgovori do %s",
	targetStays: "Cilj ostaje %s kcal dnevno.", stepsFell: "ali koraka je %s %% manje", stepsRose: "ali koraka je %s %% više",
	needs: "Korak treba %d upisana dana i %d vaganja u zadnjih 7.", nextReview: "Sljedeći pregled %s",
	tdeeAbout: "TDEE je oko %s ± %s kcal.", firstAt: "Prvi korak oko %s, ako nastaviš upisivati kao sada.",
	ifLogging: "Nastavi upisivati i vagati se, pa će ga kalibracija izmjeriti.", firstFrom: "Prvi korak može doći od %s",
	setPlan:     "Postavi fazu i cilj kcal u planu, pa će pregled ocjenjivati tempo prema njima.",
	answerFirst: "Prvo odgovori na prijedlog u planu; pregled ga čeka.",
	carbsLeft:   "%s bi imali %s g UH, a proteine i masti nikad ne mijenja.", askCoach: "Zamoli trenera da promijeni raspodjelu.",
	bmiUnder:     "BMI ti je %s, ispod 18,5, pa autopilot ne nastavlja gubitak. Prihvati ili odbij u planu.",
	rejectedBody: "Zadnji korak je odbijen: cilj ostaje kakav je, dok razlika ne naraste za korak.",
	limitBody:    "Korak bi prešao tvoje granice (%s); trener može pogledati plan.",
	drift:        "Odstupanje", of: "od", takeBackAt: "Vrati u planu",
	weekStepsCarbs: "Oba tipa dana mijenjaju se za %s kcal, sve iz ugljikohidrata (%s g): prosjek %s → %s kcal dnevno.",
	training:       "Dani treninga", rest: "Dani odmora", everyDay: "Svi dani",
}

// reviewWords are the review's phrases in each language a user can have.
var reviewWords = map[string]reviewPhrases{"en": reviewEnglish, "hr": reviewCroatian}

// reviewWriter writes a review's message in a language.
type reviewWriter struct {
	writer
	r reviewPhrases
}

func newReviewWriter(locale string) reviewWriter {
	r, ok := reviewWords[locale]
	if !ok {
		r = reviewEnglish
	}
	return reviewWriter{writer: writer{phrasesOf(locale)}, r: r}
}

// sprintf formats with the writer's numbers already written.
func sprintf(format string, args ...any) string {
	return strings.TrimSpace(fmt.Sprintf(format, args...))
}

// title is the review's outcome in a few words: a push's title, and the
// morning summary's line.
func (w reviewWriter) title(r autopilot.Review) string {
	step := w.signed(r.StepKcal, 0)
	switch r.Outcome {
	case autopilot.Applied:
		return sprintf(w.r.targetDay, step)
	case autopilot.Proposed:
		if r.Reason == autopilot.ReasonUnderweight {
			return w.r.maintenance
		}
		return sprintf(w.r.suggests, step)
	case autopilot.Hold:
		switch r.Reason {
		case autopilot.ReasonStepsGuard:
			return w.r.holdSteps
		case autopilot.ReasonIntake:
			return w.r.holdIntake
		case autopilot.ReasonRejected:
			return w.r.holdRejected
		default:
			return w.r.holdPace
		}
	case autopilot.Skipped:
	}
	return w.r.noStep + ": " + w.why(r)
}

// why is what keeps a review from a step, in a few words.
func (w reviewWriter) why(r autopilot.Review) string {
	switch r.Reason {
	case autopilot.ReasonFewData:
		if r.LoggedDays < r.NeedsLoggedDays {
			return sprintf(w.r.fewLogged, r.LoggedDays, r.NeedsLoggedDays)
		}
		return sprintf(w.r.fewWeighIns, r.WeighIns, r.NeedsWeighIns)
	case autopilot.ReasonNotTrusted:
		return w.r.notTrusted
	case autopilot.ReasonFirstWait:
		return w.r.tooSoon
	case autopilot.ReasonInterval:
		return w.r.interval
	case autopilot.ReasonNoPlan:
		return w.r.noPlan
	case autopilot.ReasonPending:
		return w.r.waits
	case autopilot.ReasonFewCarbs:
		return w.r.fewCarbs
	case autopilot.ReasonUnderweight:
		return w.r.underweight
	default:
		return w.r.limit
	}
}

// pace says the measured rate against the plan's.
func (w reviewWriter) pace(r autopilot.Review) string {
	if w.decimalComma {
		return w.r.losing + " " + w.signed(r.MeasuredPctWeek, 2) + " " + w.perWeek + ", " + w.r.plan + " " + w.signed(r.TargetPctWeek, 2) + " " + w.perWeek
	}
	verb := w.r.losing
	if r.MeasuredPctWeek > 0 {
		verb = w.r.gaining
	}
	return verb + " " + w.num(abs(r.MeasuredPctWeek), 2) + "%/wk, " + w.r.plan + " " + w.num(abs(r.TargetPctWeek), 2) + "%"
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// body is the review's numbers and what to do, a push's body; next is
// the next review, answer the day its proposal waits until.
func (w reviewWriter) body(r autopilot.Review, next, answer day.Date) string {
	review := capitalize(w.r.event) + ": " + w.pace(r)
	switch r.Outcome {
	case autopilot.Applied:
		return review + ". " + sprintf(w.r.newAvg, w.num(r.AfterKcal, 0)) + " " + w.r.tapStep
	case autopilot.Proposed:
		switch {
		case r.Reason == autopilot.ReasonUnderweight:
			return sprintf(w.r.bmiUnder, w.num(r.BMI, 1))
		case r.PastDrift:
			return review + ". " + sprintf(w.r.pastDrift, w.signed(r.DriftKcal, 0), w.num(r.DriftLimitKcal, 0), w.shortDate(answer))
		}
		return review + ". " + sprintf(w.r.answerBy, w.shortDate(answer))
	case autopilot.Hold:
		stays := sprintf(w.r.targetStays, w.num(r.TargetKcal, 0))
		switch r.Reason {
		case autopilot.ReasonRejected:
			return w.r.rejectedBody
		case autopilot.ReasonStepsGuard:
			steps := w.r.stepsFell
			if r.StepsChangePct > 0 {
				steps = w.r.stepsRose
			}
			return review + ", " + sprintf(steps, w.num(abs(r.StepsChangePct), 0)) + ". " + stays
		default:
			return review + ". " + stays
		}
	case autopilot.Skipped:
	}
	return w.skipBody(r, next)
}

// skipBody says why no step came, and when one may.
func (w reviewWriter) skipBody(r autopilot.Review, next day.Date) string {
	nextReview := sprintf(w.r.nextReview, w.shortDate(next))
	switch r.Reason {
	case autopilot.ReasonFewData:
		return sprintf(w.r.needs, r.NeedsLoggedDays, r.NeedsWeighIns) + " " + nextReview
	case autopilot.ReasonNotTrusted:
		var parts []string
		if r.TDEE > 0 {
			parts = append(parts, sprintf(w.r.tdeeAbout, w.num(r.TDEE, 0), w.num(r.HalfWidth, 0)))
		}
		if !r.Expected.IsZero() {
			parts = append(parts, sprintf(w.r.firstAt, w.shortDate(r.Expected)))
		} else {
			parts = append(parts, w.r.ifLogging)
		}
		return strings.Join(parts, " ")
	case autopilot.ReasonFirstWait, autopilot.ReasonInterval:
		from := r.From
		if from.IsZero() {
			from = next
		}
		return sprintf(w.r.firstFrom, w.shortDate(from))
	case autopilot.ReasonNoPlan:
		return w.r.setPlan
	case autopilot.ReasonPending:
		return w.r.answerFirst + " " + nextReview
	case autopilot.ReasonFewCarbs:
		return sprintf(w.r.carbsLeft, w.dayTypeName(r), w.num(r.CarbsG, 0)) + " " + w.r.askCoach
	default:
		return sprintf(w.r.limitBody, r.Rule)
	}
}

func (w reviewWriter) dayTypeName(r autopilot.Review) string {
	switch r.DayType {
	case "training":
		return w.r.training
	case "rest":
		return w.r.rest
	}
	return w.r.everyDay
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// text is the review whole, Telegram's message: its day, what it did and
// why, the numbers, the drift in autopilot mode, and the next review.
func (w reviewWriter) text(r autopilot.Review, mode autopilot.Mode, next, answer day.Date) string {
	lines := []string{"Autopilot · " + w.r.event + ", " + w.date(r.Date), w.title(r), w.body(r, next, answer)}
	if r.Outcome == autopilot.Applied || r.Outcome == autopilot.Proposed && r.Reason != autopilot.ReasonUnderweight {
		lines = append(lines, sprintf(w.r.weekStepsCarbs, w.signed(r.StepKcal, 0), w.signed(r.StepKcal/4, 0),
			w.num(r.TargetKcal, 0), w.num(r.AfterKcal, 0)))
	}
	if mode == autopilot.Auto {
		d := w.r.drift + ": " + w.signed(r.DriftKcal, 0)
		if r.DriftLimitKcal > 0 {
			d += " " + w.r.of + " " + w.num(r.DriftLimitKcal, 0)
		}
		lines = append(lines, d+" kcal")
	}
	end := sprintf(w.r.nextReview, w.shortDate(next))
	if strings.Contains(lines[2], end) {
		// The body said it.
		end = ""
	}
	if r.Outcome == autopilot.Applied {
		end += " " + w.r.takeBack
	}
	if end = strings.TrimSpace(end); end != "" {
		lines = append(lines, end)
	}
	return strings.Join(lines, "\n")
}

// reviewNote is the review sr as a notification, in the language of
// locale: whole for Telegram, its title and body for a device, whose tap
// opens the plan. A hold or no step is sent silent: it asks nothing.
func reviewNote(sr store.StoredReview, settings autopilot.Settings, locale string) notify.Note {
	w := newReviewWriter(locale)
	r := sr.Review
	next := autopilot.NextOn(r.Date.AddDays(1), settings.Weekday)
	answer := r.Date.AddDays(7)
	quiet := r.Outcome == autopilot.Hold || r.Outcome == autopilot.Skipped
	m := push.Message{TTL: reviewTTL, Topic: "autopilot"}
	if quiet {
		m.Urgency = push.UrgencyLow
	}
	return notify.Note{
		Text:    w.text(r, settings.Mode, next, answer),
		Push:    push.Notification{Title: w.title(r), Body: w.body(r, next, answer), URL: "/plan", Tag: "autopilot", Silent: quiet},
		Message: m,
	}
}

// reviewLine is the morning summary's line of the review r: what it did,
// in a few words.
func reviewLine(r autopilot.Review, locale string) string {
	w := newReviewWriter(locale)
	line := "Autopilot: " + lowerFirst(w.title(r))
	if r.Outcome == autopilot.Applied {
		line += ", " + w.num(r.AfterKcal, 0) + " " + w.r.kcalDay
	}
	return line
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToLower(string(r[0])) + string(r[1:])
}
