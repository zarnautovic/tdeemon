package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// dayAnswer is the part of a day's answer the tests of its changes read.
type dayAnswer struct {
	Status         string `json:"status"`
	ExcludedReason string `json:"excluded_reason"`
	Type           string `json:"type"`
	TrainingToggle *bool  `json:"training_toggle"`
	Mood           int    `json:"mood"`
	Hunger         int    `json:"hunger"`
	Note           string `json:"note"`
	Metrics        []struct {
		Kind   string  `json:"kind"`
		Value  float64 `json:"value"`
		Source string  `json:"source"`
	} `json:"metrics"`
}

// changeDay sends a change of a day and reads the day it answers.
func changeDay(t *testing.T, in *instance, session []*http.Cookie, method, path, body string) (int, dayAnswer, string) {
	t.Helper()
	rec := in.do(t, method, path, body, session, nil)
	var a dayAnswer
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("decode the day: %v", err)
		}
	}
	return rec.Code, a, rec.Body.String()
}

// metricOf is the day's value of kind, and its source, or 0 and "".
func (a dayAnswer) metricOf(kind string) (float64, string) {
	for _, m := range a.Metrics {
		if m.Kind == kind {
			return m.Value, m.Source
		}
	}
	return 0, ""
}

// log_metric stores what the user gave, over an import, of today or a day
// before, and answers the day.
func TestLogMetric(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	saveIsolationMetrics(t, in.db, in.user, in.clock.Now())
	today := day.DateOf(in.clock.Now())
	code, a, body := changeDay(t, in, session, http.MethodPut, "/api/v1/days/today/metrics/steps", `{"value": 9000}`)
	if v, src := a.metricOf("steps"); code != http.StatusOK || v != 9000 || src != "manual" {
		t.Errorf("steps over an import: %d %s", code, body)
	}
	yesterday := today.AddDays(-1).String()
	code, a, body = changeDay(t, in, session, http.MethodPut, "/api/v1/days/"+yesterday+"/metrics/weight", `{"value": 80.4}`)
	if v, _ := a.metricOf("weight"); code != http.StatusOK || v != 80.4 {
		t.Errorf("a weight of the day before: %d %s", code, body)
	}
	if code, a, _ := changeDay(t, in, session, http.MethodGet, "/api/v1/days/today", ""); code != http.StatusOK {
		t.Fatal(code)
	} else if v, _ := a.metricOf("weight"); v != 81.5 {
		t.Errorf("today's weight = %v, want it as it was", v)
	}
	for _, tc := range []struct{ name, path, body, want string }{
		{"a day to come", "/api/v1/days/" + today.AddDays(1).String() + "/metrics/weight", `{"value": 80}`, "a day to come"},
		{"a weight no one has", "/api/v1/days/today/metrics/weight", `{"value": 1000}`, "weight"},
		{"no such kind", "/api/v1/days/today/metrics/waist", `{"value": 80}`, "kind"},
		{"no such date", "/api/v1/days/2026-13-01/metrics/weight", `{"value": 80}`, "the date"},
	} {
		if code, _, body := changeDay(t, in, session, http.MethodPut, tc.path, tc.body); code != http.StatusUnprocessableEntity || !strings.Contains(body, tc.want) {
			t.Errorf("%s: %d %s, want 422 saying %q", tc.name, code, body, tc.want)
		}
	}
}

// delete_metric deletes a day's value of a kind, and answers the day; a
// kind the day has none of is a 404.
func TestDeleteMetric(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	saveIsolationMetrics(t, in.db, in.user, in.clock.Now())
	code, a, body := changeDay(t, in, session, http.MethodDelete, "/api/v1/days/today/metrics/weight", "")
	if v, _ := a.metricOf("weight"); code != http.StatusOK || v != 0 {
		t.Errorf("the weight deleted: %d %s", code, body)
	}
	if v, _ := a.metricOf("steps"); v != 6400 {
		t.Errorf("the steps after the weight was deleted = %v, want them kept", v)
	}
	if code, _, body := changeDay(t, in, session, http.MethodDelete, "/api/v1/days/today/metrics/weight", ""); code != http.StatusNotFound || !strings.Contains(body, "no weight on") {
		t.Errorf("a weight deleted again: %d %s, want 404", code, body)
	}
}

// checkin says what the user says of a day, each field given and only
// those; empty values clear; one undo takes it all back.
func TestCheckIn(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	full := checkInView{status: "excluded", reason: "restaurant", dayType: "training", toggle: "true", mood: 4, hunger: 2, note: "slept well"}
	hungrier := full
	hungrier.hunger = 5
	for _, step := range []struct {
		name, method, body string
		want               checkInView
	}{
		{"a check-in", http.MethodPatch, `{"mood": 4, "hunger": 2, "note": "slept well", "training_day": "training", "excluded_reason": "restaurant"}`, full},
		{"a change of the hunger, which keeps the rest", http.MethodPatch, `{"hunger": 5}`, hungrier},
		{"nothing said, which changes nothing", http.MethodPatch, `{}`, hungrier},
		{"undo, which takes back the hunger", "undo", "", full},
	} {
		if step.method == "undo" {
			if _, err := in.db.UndoLast(t.Context(), in.user.ID, isolationActor, in.clock.Now()); err != nil {
				t.Fatal(err)
			}
			step.method = http.MethodGet
		}
		if code, a, body := changeDay(t, in, session, step.method, "/api/v1/days/today", step.body); code != http.StatusOK || a.view() != step.want {
			t.Errorf("%s: %d %s, want %+v", step.name, code, body, step.want)
		}
	}
}

// checkInView is what a day's answer says of its check-in.
type checkInView struct {
	status, reason, dayType, toggle string
	mood, hunger                    int
	note                            string
}

func (a dayAnswer) view() checkInView {
	v := checkInView{status: a.Status, reason: a.ExcludedReason, dayType: a.Type, mood: a.Mood, hunger: a.Hunger, note: a.Note}
	if a.TrainingToggle != nil {
		v.toggle = strconv.FormatBool(*a.TrainingToggle)
	}
	return v
}

// checkin's empty values clear what they name.
func TestCheckInClears(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	if code, _, body := changeDay(t, in, session, http.MethodPatch, "/api/v1/days/today",
		`{"mood": 4, "hunger": 2, "note": "slept well", "training_day": "training", "excluded_reason": "restaurant"}`); code != http.StatusOK {
		t.Fatalf("a check-in: %d %s", code, body)
	}
	code, a, body := changeDay(t, in, session, http.MethodPatch, "/api/v1/days/today",
		`{"mood": 0, "hunger": 0, "note": "", "training_day": "plan", "excluded_reason": ""}`)
	if want := (checkInView{status: "open", dayType: "rest"}); code != http.StatusOK || a.view() != want {
		t.Errorf("a check-in cleared: %d %s, want %+v", code, body, want)
	}
	if code, a, body := changeDay(t, in, session, http.MethodPatch, "/api/v1/days/today", `{"training_day": "rest"}`); code != http.StatusOK ||
		a.view() != (checkInView{status: "open", dayType: "rest", toggle: "false"}) {
		t.Errorf("a rest day: %d %s", code, body)
	}
}

// checkin refuses what a day cannot hold, and changes nothing.
func TestCheckInRefuses(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	for _, tc := range []struct{ name, body, want string }{
		{"a mood off the scale", `{"mood": 6}`, "mood"},
		{"a hunger below it", `{"hunger": -1}`, "hunger"},
		{"a note of a control character", `{"note": "a\u0000b"}`, "the note"},
		{"a reason of spaces", `{"excluded_reason": "   "}`, "needs a reason"},
		{"a reason too long", `{"excluded_reason": "` + strings.Repeat("a", day.MaxReasonLen+1) + `"}`, "excluded_reason"},
		{"no such type", `{"training_day": "maybe"}`, "training_day"},
		{"no such date", `{"mood": 3}`, "the date"},
	} {
		path := "/api/v1/days/today"
		if tc.name == "no such date" {
			path = "/api/v1/days/2026-02-30"
		}
		if code, _, body := changeDay(t, in, session, http.MethodPatch, path, tc.body); code != http.StatusUnprocessableEntity || !strings.Contains(body, tc.want) {
			t.Errorf("%s: %d %s, want 422 saying %q", tc.name, code, body, tc.want)
		}
	}
	if code, a, _ := changeDay(t, in, session, http.MethodGet, "/api/v1/days/today", ""); code != http.StatusOK || a.Mood != 0 || a.Note != "" || a.Status != "open" {
		t.Errorf("the day after the refusals: %+v", a)
	}
}
