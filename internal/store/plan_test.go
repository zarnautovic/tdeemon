package store_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/plan"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/store"
)

func date(t *testing.T, s string) day.Date {
	t.Helper()
	d, err := day.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// lastEvent returns userID's latest event.
func lastEvent(t *testing.T, db *store.DB, userID uuid.UUID) event.Event {
	t.Helper()
	es, err := db.Events(t.Context(), userID, 1)
	if err != nil || len(es) != 1 {
		t.Fatalf("Events = %v, %v", es, err)
	}
	return es[0]
}

func TestProfile(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	today := day.DateOf(t0)
	if p, err := db.Profile(t.Context(), ana); err != nil || p != (profile.Profile{}) {
		t.Fatalf("Profile before = %+v, %v; want none", p, err)
	}
	p := profile.Profile{Sex: profile.Female, BirthDate: date(t, "1990-06-15"), HeightCm: 168.5}
	if err := db.SetProfile(t.Context(), ana, testActor, p, today, t0); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	if got, err := db.Profile(t.Context(), ana); err != nil || got != p {
		t.Errorf("Profile = %+v, %v; want %+v", got, err, p)
	}
	if got, _ := db.Profile(t.Context(), ivo); got != (profile.Profile{}) {
		t.Errorf("Ivo's profile = %+v, want none", got)
	}
	if e := lastEvent(t, db, ana); e.Action != event.ProfileSet || e.Entity != "profile" || e.Before == nil || e.After == nil {
		t.Errorf("the event = %+v", e)
	}
	// A part taken away is stored as none.
	p.HeightCm = 0
	if err := db.SetProfile(t.Context(), ana, testActor, p, today, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Profile(t.Context(), ana); got != p {
		t.Errorf("Profile = %+v, want %+v", got, p)
	}
	// The same profile again records nothing.
	n := countRows(t, db, "events")
	if err := db.SetProfile(t.Context(), ana, testActor, p, today, t0); err != nil || countRows(t, db, "events") != n {
		t.Errorf("the same profile = %v, %d events, want %d", err, countRows(t, db, "events"), n)
	}
}

func TestSetProfileRefuses(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	today := day.DateOf(t0)
	p := profile.Profile{Sex: profile.Female}
	if err := db.SetProfile(t.Context(), ana, testActor, profile.Profile{HeightCm: 1.7}, today, t0); err == nil || !strings.Contains(err.Error(), "invalid profile") {
		t.Errorf("a height in metres = %v", err)
	}
	if err := db.SetProfile(t.Context(), ana, testActor, profile.Profile{BirthDate: today.AddDays(1)}, today, t0); err == nil {
		t.Error("a birth after today passed")
	}
	if err := db.SetProfile(t.Context(), ana, event.Actor{}, p, today, t0); err == nil || !strings.Contains(err.Error(), "invalid actor") {
		t.Errorf("no actor = %v", err)
	}
	if err := db.SetProfile(t.Context(), uuid.NewV7(), testActor, p, today, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetProfile of no user = %v, want ErrNotFound", err)
	}
	if _, err := db.Profile(t.Context(), uuid.NewV7()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Profile of no user = %v, want ErrNotFound", err)
	}
}

// The limits of a plan depend on the profile and the user's limits, which
// only a web session changes: undo leaves them.
func TestUndoLeavesTheProfileAndTheLimits(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	if err := db.SetProfileNotes(t.Context(), ana, testActor, "No fish.", t0); err != nil {
		t.Fatal(err)
	}
	if err := db.SetProfile(t.Context(), ana, testActor, profile.Profile{Sex: profile.Male}, day.DateOf(t0), t0); err != nil {
		t.Fatal(err)
	}
	l := limits.UserDefaults()
	l.CutRate = -0.5
	if err := db.SetUserLimits(t.Context(), ana, testActor, l, limits.Instance(), t0); err != nil {
		t.Fatal(err)
	}
	// Undo passes over both, to the notes.
	if e := undo(t, db, ana); e.Action != event.NotesSet {
		t.Errorf("undo took back %s, want the notes", e.Action)
	}
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("the next undo = %v, want ErrNothingToUndo", err)
	}
	if p, _ := db.Profile(t.Context(), ana); p.Sex != profile.Male {
		t.Errorf("Profile = %+v, want it as set", p)
	}
	if got, _ := db.UserLimits(t.Context(), ana); got != l {
		t.Errorf("UserLimits = %+v, want them as set", got)
	}
}

func cut(t *testing.T, start, end string, rate float64) plan.Phase {
	t.Helper()
	p := plan.Phase{Type: plan.Cut, Start: date(t, start), RatePctWeek: rate}
	if end != "" {
		p.End = date(t, end)
	}
	return p
}

func TestPhases(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	setPhase := func(p plan.Phase) {
		t.Helper()
		if err := db.SetPhase(t.Context(), ana, testActor, p, t0); err != nil {
			t.Fatalf("SetPhase(%+v): %v", p, err)
		}
	}
	first := cut(t, "2026-09-07", "2026-10-31", -0.5)
	first.GoalWeight, first.MilestoneWeight = new(72.0), new(75.0)
	brk := plan.Phase{Type: plan.Maintain, Start: date(t, "2026-11-02"), End: date(t, "2026-11-08")}
	bulk := plan.Phase{Type: plan.Bulk, Start: date(t, "2027-01-04"), RatePctWeek: 0.25}
	setPhase(bulk)
	setPhase(first)
	setPhase(brk)
	if got, err := db.Phases(t.Context(), ana); err != nil || !reflect.DeepEqual(got, []plan.Phase{first, brk, bulk}) {
		t.Errorf("Phases = %+v, %v", got, err)
	}
	if got, _ := db.Phases(t.Context(), ivo); got != nil {
		t.Errorf("Ivo's phases = %+v", got)
	}
	// An edit of the phase that starts on that day.
	first.RatePctWeek, first.GoalWeight, first.MilestoneWeight = -0.7, nil, new(74.0)
	setPhase(first)
	if got, _ := db.Phases(t.Context(), ana); !reflect.DeepEqual(got[0], first) {
		t.Errorf("the edited phase = %+v, want %+v", got[0], first)
	}
	if e := lastEvent(t, db, ana); e.Action != event.PhaseSet || e.Entity != "2026-09-07" || e.Before == nil {
		t.Errorf("the edit's event = %+v", e)
	}

	// Overlaps are refused, and change nothing.
	for _, p := range []plan.Phase{
		cut(t, "2026-11-05", "", -0.5),           // inside the break
		cut(t, "2026-10-31", "", -0.5),           // on the first's end
		cut(t, "2026-12-01", "2027-01-04", -0.5), // up to the bulk's start
	} {
		if err := db.SetPhase(t.Context(), ana, testActor, p, t0); !errors.Is(err, store.ErrPhaseOverlap) {
			t.Errorf("SetPhase(%s to %s) = %v, want ErrPhaseOverlap", p.Start, p.End, err)
		}
	}
	if got, _ := db.Phases(t.Context(), ana); len(got) != 3 {
		t.Errorf("Phases after the overlaps = %+v", got)
	}
}

func TestDeletePhases(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	first := cut(t, "2026-09-07", "2026-10-31", -0.5)
	first.GoalWeight, first.MilestoneWeight = new(72.0), new(75.0)
	brk := plan.Phase{Type: plan.Maintain, Start: date(t, "2026-11-02"), End: date(t, "2026-11-08")}
	bulk := plan.Phase{Type: plan.Bulk, Start: date(t, "2027-01-04"), RatePctWeek: 0.25}
	edited := first
	edited.RatePctWeek, edited.GoalWeight = -0.7, nil
	for _, p := range []plan.Phase{bulk, first, brk, edited} {
		if err := db.SetPhase(t.Context(), ana, testActor, p, t0); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetPhase(t.Context(), ana, testActor, plan.Phase{Type: plan.Bulk, Start: date(t, "2027-02-01"), RatePctWeek: -1}, t0); err == nil ||
		!strings.Contains(err.Error(), "invalid phase") {
		t.Errorf("a bulk that loses = %v", err)
	}

	if err := db.DeletePhase(t.Context(), ana, testActor, brk.Start, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Phases(t.Context(), ana); !reflect.DeepEqual(got, []plan.Phase{edited, bulk}) {
		t.Errorf("Phases after the delete = %+v", got)
	}
	if err := db.DeletePhase(t.Context(), ana, testActor, brk.Start, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a second delete = %v, want ErrNotFound", err)
	}
	if err := db.DeletePhase(t.Context(), ivo, testActor, first.Start, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Ivo deleting Ana's phase = %v, want ErrNotFound", err)
	}
}

// A stored phase that breaks a rule of its shape is no overlap.
func TestATimelineWithAPhaseOfTheWrongShape(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	exec(t, db, `INSERT INTO phases (user_id, start, type, rate_pct_week, updated_at) VALUES (?, '2026-09-07', 'cut', 0.5, ?)`,
		ana.String(), t0.Format(time.RFC3339))
	err := db.SetPhase(t.Context(), ana, testActor, cut(t, "2027-01-04", "", -0.5), t0)
	if err == nil || errors.Is(err, store.ErrPhaseOverlap) || !strings.Contains(err.Error(), "the stored phase from 2026-09-07") {
		t.Errorf("SetPhase beside a stored phase of the wrong shape = %v", err)
	}
}

func dayTarget(kcal, protein, fat float64) plan.DayTarget {
	return plan.DayTarget{Kcal: kcal, Protein: protein, Fat: fat}
}

func TestTargets(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	first := plan.Target{EffectiveFrom: date(t, "2026-09-07"), Training: dayTarget(2600, 170, 75), Rest: dayTarget(2200, 160, 70)}
	second := first.ShiftKcal(-100)
	second.EffectiveFrom = date(t, "2026-10-05")
	for _, tg := range []plan.Target{second, first} {
		if err := db.SetTarget(t.Context(), ana, testActor, tg, t0); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := db.Targets(t.Context(), ana); err != nil || !reflect.DeepEqual(got, []plan.Target{first, second}) {
		t.Errorf("Targets = %+v, %v", got, err)
	}
	if got, _ := db.Targets(t.Context(), ivo); got != nil {
		t.Errorf("Ivo's targets = %+v", got)
	}
	// -0 is stored and recorded as 0, as the table reads it back.
	edited := second
	edited.Rest.Fat = 0
	edited.Training.Protein = negZero()
	if err := db.SetTarget(t.Context(), ana, testActor, edited, t0); err != nil {
		t.Fatal(err)
	}
	if e := lastEvent(t, db, ana); e.Action != event.TargetSet || e.Entity != "2026-10-05" || strings.Contains(string(e.After), "-0") {
		t.Errorf("the edit's event = %+v, %s", e, e.After)
	}
	if err := db.SetTarget(t.Context(), ana, testActor, plan.Target{EffectiveFrom: first.EffectiveFrom, Training: dayTarget(500, 170, 70), Rest: first.Rest}, t0); err == nil ||
		!strings.Contains(err.Error(), "invalid target") {
		t.Errorf("a target of the wrong shape = %v", err)
	}
	if err := db.DeleteTarget(t.Context(), ana, testActor, first.EffectiveFrom, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteTarget(t.Context(), ana, testActor, first.EffectiveFrom, t0); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a second delete = %v", err)
	}
}

func negZero() float64 {
	z := 0.0
	return -z
}

func TestTrainingDays(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	d := day.DateOf(t0)
	if err := db.ExcludeDay(t.Context(), ana, testActor, d, day.Exclusion{Reason: "travel"}, t0); err != nil {
		t.Fatal(err)
	}
	set := func(v *bool) {
		t.Helper()
		if err := db.SetTrainingDay(t.Context(), ana, testActor, d, v, t0); err != nil {
			t.Fatalf("SetTrainingDay(%v): %v", v, err)
		}
	}
	toggles := func() map[day.Date]bool {
		t.Helper()
		m, err := db.TrainingDayToggles(t.Context(), ana, d.AddDays(-7), d)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	set(new(true))
	set(new(false))
	if err := db.SetTrainingDay(t.Context(), ana, testActor, d.AddDays(-1), new(true), t0); err != nil {
		t.Fatal(err)
	}
	if got := toggles(); !reflect.DeepEqual(got, map[day.Date]bool{d: false, d.AddDays(-1): true}) {
		t.Errorf("toggles = %v", got)
	}
	if got, _ := db.TrainingDayToggles(t.Context(), ivo, d.AddDays(-7), d); len(got) != 0 {
		t.Errorf("Ivo's toggles = %v", got)
	}
	// The same toggle again, or none on a day without one, records nothing.
	n := countRows(t, db, "events")
	set(new(false))
	if err := db.SetTrainingDay(t.Context(), ana, testActor, d.AddDays(-3), nil, t0); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, "events") != n || countRows(t, db, "days") != 2 {
		t.Errorf("a toggle left as it was: %d events, %d days", countRows(t, db, "events"), countRows(t, db, "days"))
	}
	set(nil)
	if got := toggles(); !reflect.DeepEqual(got, map[day.Date]bool{d.AddDays(-1): true}) {
		t.Errorf("toggles after none = %v", got)
	}
	// Undo walks back through the toggles and leaves the exclusion.
	undo(t, db, ana) // nil
	undo(t, db, ana) // the day before
	undo(t, db, ana) // false
	if got := toggles(); !reflect.DeepEqual(got, map[day.Date]bool{d: true}) {
		t.Errorf("toggles after the undos = %v", got)
	}
	if ex, _ := db.Exclusions(t.Context(), ana, d, d); ex[d].Reason != "travel" {
		t.Errorf("the exclusion = %v", ex)
	}
	if err := db.SetTrainingDay(t.Context(), ana, testActor, day.Date{}, nil, t0); err == nil {
		t.Error("a toggle with no day passed")
	}
}

func TestTrainingWeekdays(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	if w, err := db.TrainingWeekdays(t.Context(), ana); err != nil || w != 0 {
		t.Fatalf("TrainingWeekdays before = %v, %v", w, err)
	}
	w, err := plan.WeekdaysOf(time.Monday, time.Wednesday, time.Friday)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetTrainingWeekdays(t.Context(), ana, testActor, w, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.TrainingWeekdays(t.Context(), ana); got != w {
		t.Errorf("TrainingWeekdays = %v, want %v", got, w)
	}
	if got, _ := db.TrainingWeekdays(t.Context(), ivo); got != 0 {
		t.Errorf("Ivo's = %v", got)
	}
	if err := db.SetTrainingWeekdays(t.Context(), ana, testActor, plan.Weekdays(1<<7), t0); err == nil || !strings.Contains(err.Error(), "invalid training weekdays") {
		t.Errorf("a bit that is no day = %v", err)
	}
}

func TestUserLimits(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	if l, err := db.UserLimits(t.Context(), ana); err != nil || l != limits.UserDefaults() {
		t.Fatalf("UserLimits before = %+v, %v", l, err)
	}
	l := limits.UserDefaults()
	l.CutRate, l.ProteinFloorGrams, l.DriftPct = -0.4, 150, 20
	if err := db.SetUserLimits(t.Context(), ana, testActor, l, limits.Instance(), t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.UserLimits(t.Context(), ana); got != l {
		t.Errorf("UserLimits = %+v, want %+v", got, l)
	}
	// Only the limits off their defaults are stored, so the others follow
	// a default a later release changes.
	var stored string
	if err := db.Read().QueryRowContext(t.Context(), "SELECT limits FROM plan_settings WHERE user_id = ?", ana.String()).Scan(&stored); err != nil ||
		stored != `{"cut_rate":-0.4,"drift_pct":20,"protein_floor_grams":150}` {
		t.Errorf("the stored limits = %s, %v", stored, err)
	}
	if got, _ := db.UserLimits(t.Context(), ivo); got != limits.UserDefaults() {
		t.Errorf("Ivo's = %+v", got)
	}
	// Training weekdays share the row, and each leaves the other.
	if err := db.SetTrainingWeekdays(t.Context(), ana, testActor, 1, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.UserLimits(t.Context(), ana); got != l {
		t.Errorf("UserLimits after the weekdays = %+v", got)
	}
	l.DriftPct = 18
	if err := db.SetUserLimits(t.Context(), ana, testActor, l, limits.Instance(), t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.TrainingWeekdays(t.Context(), ana); got != 1 {
		t.Errorf("TrainingWeekdays after the limits = %v, want 1", got)
	}
}

func TestSetUserLimitsRefuses(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	l := limits.UserDefaults()
	beyond := l
	beyond.KcalFloorFemale = 1100
	if err := db.SetUserLimits(t.Context(), ana, testActor, beyond, limits.Instance(), t0); !errors.Is(err, store.ErrLimitsBeyond) ||
		!strings.Contains(err.Error(), "kcal_floor_female") {
		t.Errorf("a limit past the instance's = %v", err)
	}
	bad := l
	bad.BulkRate = -1
	if err := db.SetUserLimits(t.Context(), ana, testActor, bad, limits.Instance(), t0); err == nil || !strings.Contains(err.Error(), "invalid limits") {
		t.Errorf("limits of the wrong shape = %v", err)
	}
	if err := db.SetUserLimits(t.Context(), ana, event.Actor{}, l, limits.Instance(), t0); err == nil {
		t.Error("no actor passed")
	}
}

// A limit the stored object does not name is at its default, and a name
// the code does not know is left.
func TestUserLimitsOfAPartialObject(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	if err := db.SetTrainingWeekdays(t.Context(), ana, testActor, 1, t0); err != nil {
		t.Fatal(err)
	}
	exec(t, db, `UPDATE plan_settings SET limits = '{"cut_rate": -0.3, "gone": 5}' WHERE user_id = ?`, ana.String())
	want := limits.UserDefaults()
	want.CutRate = -0.3
	if got, _ := db.UserLimits(t.Context(), ana); got != want {
		t.Errorf("UserLimits of a partial object = %+v, want %+v", got, want)
	}
}

func TestProfileNotes(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	if n, err := db.ProfileNotes(t.Context(), ana); err != nil || n != "" {
		t.Fatalf("ProfileNotes before = %q, %v", n, err)
	}
	for _, notes := range []string{"No fish.", "No fish.\nTrains at 6."} {
		if err := db.SetProfileNotes(t.Context(), ana, testActor, notes, t0); err != nil {
			t.Fatal(err)
		}
		if got, _ := db.ProfileNotes(t.Context(), ana); got != notes {
			t.Errorf("ProfileNotes = %q, want %q", got, notes)
		}
	}
	if got, _ := db.ProfileNotes(t.Context(), ivo); got != "" {
		t.Errorf("Ivo's = %q", got)
	}
	if err := db.SetProfileNotes(t.Context(), ana, testActor, " ", t0); err == nil || !strings.Contains(err.Error(), "invalid notes") {
		t.Errorf("blank notes = %v", err)
	}
	if err := db.SetProfileNotes(t.Context(), ana, testActor, "", t0); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, "profile_notes") != 0 {
		t.Error("empty notes left a row")
	}
	undo(t, db, ana)
	if got, _ := db.ProfileNotes(t.Context(), ana); got != "No fish.\nTrains at 6." {
		t.Errorf("after an undo = %q", got)
	}
	undo(t, db, ana)
	undo(t, db, ana)
	if got, _ := db.ProfileNotes(t.Context(), ana); got != "" {
		t.Errorf("after undoing every set = %q", got)
	}
}

// Each kind of change of the plan whose own data does not decode is
// passed over as unreadable, and the next undo moves on.
func TestUndoOfEveryPlanKindThatDoesNotDecode(t *testing.T) {
	t.Parallel()

	const (
		before = "UPDATE events SET before_state = ? WHERE id = ?"
		entity = "UPDATE events SET entity = ? WHERE id = ?"
	)
	d := day.DateOf(t0)
	for _, tc := range []struct {
		name, update, value string
		write               func(t *testing.T, db *store.DB, userID uuid.UUID) error
	}{
		{"training day", before, `{"training_day": "yes"}`, func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			if err := db.SetTrainingDay(t.Context(), userID, testActor, d, new(true), t0); err != nil {
				return err
			}
			return db.SetTrainingDay(t.Context(), userID, testActor, d, new(false), t0)
		}},
		{"training day of no day", entity, "not a day", func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			return db.SetTrainingDay(t.Context(), userID, testActor, d, new(true), t0)
		}},
		{"notes", before, `{"notes": 5}`, func(t *testing.T, db *store.DB, userID uuid.UUID) error {
			t.Helper()
			if err := db.SetProfileNotes(t.Context(), userID, testActor, "a", t0); err != nil {
				return err
			}
			return db.SetProfileNotes(t.Context(), userID, testActor, "b", t0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ana, _ := withTwoUsers(t)
			if err := db.ExcludeDay(t.Context(), ana, testActor, d.AddDays(10), day.Exclusion{Reason: "sick"}, t0); err != nil {
				t.Fatal(err)
			}
			if err := tc.write(t, db, ana); err != nil {
				t.Fatal(err)
			}
			e := lastEvent(t, db, ana)
			exec(t, db, tc.update, tc.value, e.ID.String())
			var conflict *store.UndoConflictError
			if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) || conflict.Reason != store.UndoUnreadable {
				t.Fatalf("undo = %v, want an UndoConflictError: %s", err, store.UndoUnreadable)
			}
			next, err := db.UndoLast(t.Context(), ana, testActor, t0)
			if errors.As(err, &conflict) {
				next = conflict.Event
			} else if err != nil {
				t.Fatalf("the next undo = %v", err)
			}
			if next.ID == e.ID {
				t.Error("the next undo met the same change again")
			}
		})
	}
}

// A change of the plan an import wrote over since is not taken back.
func TestUndoOfAToggleAfterAnImport(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	if err := db.SetTrainingDay(t.Context(), ana, testActor, d, new(true), t0); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTrainingDay(t.Context(), ana, importer, d, new(false), t0); err != nil {
		t.Fatal(err)
	}
	var conflict *store.UndoConflictError
	if _, err := db.UndoLast(t.Context(), ana, testActor, t0); !errors.As(err, &conflict) || conflict.Reason != store.UndoChanged {
		t.Fatalf("undo = %v, want %s", err, store.UndoChanged)
	}
	if got, _ := db.TrainingDayToggles(t.Context(), ana, d, d); got[d] {
		t.Errorf("toggles = %v, want the import's", got)
	}
}

// Undo leaves the plan: its changes must meet the limits as they are now,
// and go back as a change of the plan that Validate checks, not by undo
// through a tool. It takes back what came before them.
func TestUndoLeavesThePlan(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	tg := plan.Target{EffectiveFrom: d, Training: dayTarget(2400, 170, 70), Rest: dayTarget(2400, 160, 70)}
	p := cut(t, "2026-10-26", "", -0.5)
	for _, w := range []func() error{
		func() error { return db.SetProfileNotes(t.Context(), ana, testActor, "No fish.", t0) },
		func() error { return db.SetPhase(t.Context(), ana, testActor, p, t0) },
		func() error { return db.SetPhase(t.Context(), ana, testActor, cut(t, "2026-11-30", "", -0.4), t0) },
		func() error { return db.DeletePhase(t.Context(), ana, testActor, date(t, "2026-11-30"), t0) },
		func() error { return db.SetTarget(t.Context(), ana, testActor, tg, t0) },
		func() error { return db.SetTarget(t.Context(), ana, testActor, tg.ShiftKcal(100).ShiftKcal(-50), t0) },
		func() error { return db.DeleteTarget(t.Context(), ana, testActor, d, t0) },
		func() error { return db.SetTrainingWeekdays(t.Context(), ana, testActor, 3, t0) },
	} {
		if err := w(); err != nil {
			t.Fatal(err)
		}
	}
	if e := undo(t, db, ana); e.Action != event.NotesSet {
		t.Errorf("undo took back %s, want the notes", e.Action)
	}
	if got, _ := db.Phases(t.Context(), ana); len(got) != 1 || got[0] != p {
		t.Errorf("Phases = %+v, want them as they were", got)
	}
	if got, _ := db.Targets(t.Context(), ana); got != nil {
		t.Errorf("Targets = %+v, want none", got)
	}
	if got, _ := db.TrainingWeekdays(t.Context(), ana); got != 3 {
		t.Errorf("TrainingWeekdays = %v", got)
	}
}

func TestPlanStoreFailures(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithoutCancel(t.Context())
	d := day.DateOf(t0)
	tg := plan.Target{EffectiveFrom: d, Training: dayTarget(2400, 170, 70), Rest: dayTarget(2400, 170, 70)}
	for name, err := range map[string]error{
		"SetProfile":          db.SetProfile(ctx, ana, testActor, profile.Profile{Sex: profile.Male}, d, t0),
		"SetPhase":            db.SetPhase(ctx, ana, testActor, cut(t, "2026-10-26", "", -0.5), t0),
		"DeletePhase":         db.DeletePhase(ctx, ana, testActor, d, t0),
		"SetTarget":           db.SetTarget(ctx, ana, testActor, tg, t0),
		"DeleteTarget":        db.DeleteTarget(ctx, ana, testActor, d, t0),
		"SetTrainingDay":      db.SetTrainingDay(ctx, ana, testActor, d, new(true), t0),
		"SetTrainingWeekdays": db.SetTrainingWeekdays(ctx, ana, testActor, 1, t0),
		"SetUserLimits":       db.SetUserLimits(ctx, ana, testActor, limits.UserDefaults(), limits.Instance(), t0),
		"SetProfileNotes":     db.SetProfileNotes(ctx, ana, testActor, "a", t0),
	} {
		if err == nil || errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s on a closed database = %v", name, err)
		}
	}
	if _, err := db.Profile(ctx, ana); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("Profile = %v", err)
	}
	if _, err := db.Phases(ctx, ana); err == nil {
		t.Error("Phases succeeded")
	}
	if _, err := db.Targets(ctx, ana); err == nil {
		t.Error("Targets succeeded")
	}
	if _, err := db.TrainingDayToggles(ctx, ana, d, d); err == nil {
		t.Error("TrainingDayToggles succeeded")
	}
	if _, err := db.TrainingWeekdays(ctx, ana); err == nil {
		t.Error("TrainingWeekdays succeeded")
	}
	if _, err := db.UserLimits(ctx, ana); err == nil {
		t.Error("UserLimits succeeded")
	}
	if _, err := db.ProfileNotes(ctx, ana); err == nil {
		t.Error("ProfileNotes succeeded")
	}
}

// Writes need an actor, as every write does.
func TestPlanWritesNeedAnActor(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	d := day.DateOf(t0)
	tg := plan.Target{EffectiveFrom: d, Training: dayTarget(2400, 170, 70), Rest: dayTarget(2400, 170, 70)}
	none := event.Actor{}
	for name, err := range map[string]error{
		"SetPhase":            db.SetPhase(t.Context(), ana, none, cut(t, "2026-10-26", "", -0.5), t0),
		"DeletePhase":         db.DeletePhase(t.Context(), ana, none, d, t0),
		"SetTarget":           db.SetTarget(t.Context(), ana, none, tg, t0),
		"DeleteTarget":        db.DeleteTarget(t.Context(), ana, none, d, t0),
		"SetTrainingDay":      db.SetTrainingDay(t.Context(), ana, none, d, new(true), t0),
		"SetTrainingWeekdays": db.SetTrainingWeekdays(t.Context(), ana, none, 1, t0),
		"SetProfileNotes":     db.SetProfileNotes(t.Context(), ana, none, "a", t0),
	} {
		if err == nil || !strings.Contains(err.Error(), "invalid actor") {
			t.Errorf("%s with no actor = %v", name, err)
		}
	}
	if n := countRows(t, db, "events"); n != 0 {
		t.Errorf("%d events, want none", n)
	}
}

// A delete takes the user's own row, not another's on the same day.
func TestPlanDeletesAreTheUsers(t *testing.T) {
	t.Parallel()

	db, ana, ivo := withTwoUsers(t)
	p := cut(t, "2026-10-26", "", -0.5)
	tg := plan.Target{EffectiveFrom: p.Start, Training: dayTarget(2400, 170, 70), Rest: dayTarget(2400, 160, 70)}
	for _, u := range []uuid.UUID{ana, ivo} {
		if err := db.SetPhase(t.Context(), u, testActor, p, t0); err != nil {
			t.Fatal(err)
		}
		if err := db.SetTarget(t.Context(), u, testActor, tg, t0); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DeletePhase(t.Context(), ana, testActor, p.Start, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteTarget(t.Context(), ana, testActor, tg.EffectiveFrom, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Phases(t.Context(), ivo); len(got) != 1 {
		t.Errorf("Ivo's phases after Ana's delete = %+v", got)
	}
	if got, _ := db.Targets(t.Context(), ivo); len(got) != 1 {
		t.Errorf("Ivo's targets after Ana's delete = %+v", got)
	}
}

// An edit of a target changes every value of it.
func TestEditATarget(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	tg := plan.Target{EffectiveFrom: day.DateOf(t0), Training: dayTarget(2600, 170, 75), Rest: dayTarget(2200, 160, 70)}
	edited := plan.Target{EffectiveFrom: tg.EffectiveFrom, Training: dayTarget(2500, 175, 72), Rest: dayTarget(2100, 165, 65)}
	for _, x := range []plan.Target{tg, edited} {
		if err := db.SetTarget(t.Context(), ana, testActor, x, t0); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := db.Targets(t.Context(), ana); len(got) != 1 || got[0] != edited {
		t.Errorf("Targets = %+v, want %+v", got, edited)
	}
}

// UpdateUserLimits hands its change the limits stored, and stores nothing
// when the change refuses.
func TestUpdateUserLimits(t *testing.T) {
	t.Parallel()

	db, ana, _ := withTwoUsers(t)
	l := limits.UserDefaults()
	l.CutRate = -0.5
	if err := db.SetUserLimits(t.Context(), ana, testActor, l, limits.Instance(), t0); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("refused")
	if _, err := db.UpdateUserLimits(t.Context(), ana, testActor, limits.Instance(), func(current limits.Limits) (limits.Limits, error) {
		current.CutRate = -0.9
		return current, refused
	}, t0); !errors.Is(err, refused) {
		t.Errorf("a refused change = %v, want its error", err)
	}
	got, err := db.UpdateUserLimits(t.Context(), ana, testActor, limits.Instance(), func(current limits.Limits) (limits.Limits, error) {
		if current.CutRate != -0.5 {
			t.Errorf("the change got %v, want the stored -0.5", current.CutRate)
		}
		current.DriftPct = 12
		return current, nil
	}, t0)
	if err != nil || got.CutRate != -0.5 || got.DriftPct != 12 {
		t.Errorf("UpdateUserLimits = %+v, %v", got, err)
	}
	if stored, _ := db.UserLimits(t.Context(), ana); stored != got {
		t.Errorf("stored = %+v, want %+v", stored, got)
	}
}
