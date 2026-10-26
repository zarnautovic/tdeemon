package api

import (
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/day"
)

// metrics are a day's measurements, kind and value in turn.
func metrics(kv ...any) []metricBody {
	var out []metricBody
	for i := 0; i+1 < len(kv); i += 2 {
		kind, _ := kv[i].(string)
		v, _ := kv[i+1].(float64)
		out = append(out, metricBody{Kind: kind, Value: v})
	}
	return out
}

// The morning summary says each part that has something to say, in
// Croatian, and leaves out what did not come.
func TestMorningText(t *testing.T) {
	t.Parallel()

	today := day.Date{Year: 2026, Month: 10, Day: 23}
	full := dayBody{
		Date: "2026-10-22", Status: "complete", Type: "training",
		Total:   nutrientsBody{Kcal: 2310.4, Protein: 176.2, Carbs: 210, Fat: 70.4},
		Target:  &dayTargetBody{Kcal: 2400, Protein: 175, Carbs: 230, Fat: 75},
		Meals:   []mealBody{{}},
		Metrics: metrics("steps", 6512.0, "active_kcal", 520.4, "exercise_min", 45.0, "sleep_h", 5.0),
	}
	onTarget := verdictBody{Rate: "on_target", MeasuredPctWeek: new(-0.62), TargetPctWeek: new(-0.7), Action: "hold"}
	for _, tc := range []struct {
		name   string
		td, yd dayBody
		sum    summaryBody
		want   string
	}{
		{
			name: "everything",
			td:   dayBody{Metrics: metrics("sleep_h", 7.2, "weight", 132.44, "body_fat", 31.2)},
			yd:   full,
			sum: summaryBody{
				Trend: &trendBody{Kg: 133.06, Date: "2026-10-23"}, Verdict: onTarget,
				Recovery: recoveryBody{Flags: []string{"short_sleep", "low_hrv", "high_resting_hr"}, SleepH: new(6.1), HRV: new(38.4), RestingHR: new(61.0)},
			},
			want: `Jutarnji sažetak, petak 23. 10.
San: 7 h 12 min
Težina: 132,4 kg · trend 133,1 kg · tjelesna mast 31,2 %
Jučer (dan treninga): 2310/2400 kcal · P 176/175 g · UH 210/230 g · M 70/75 g
Aktivnost jučer: koraci 6512 · aktivne 520 kcal · vježbanje 45 min
Kalibracija: tempo −0,62 %/tj, cilj −0,70 %/tj, u planu
Oporavak: kratak san (prosjek 6,1 h) · nizak HRV (38 ms) · povišen puls u mirovanju (61 bpm)`,
		},
		{
			name: "nothing came",
			yd:   dayBody{Type: "rest", Status: "complete"},
			sum:  summaryBody{Verdict: verdictBody{Rate: "none", Reasons: []string{"no_phase", "no_target"}}},
			want: `Jutarnji sažetak, petak 23. 10.
Jučer: ništa upisano
Kalibracija: nema plana (faze i cilja), pa nema tempa za ocijeniti`,
		},
		{
			name: "no target, an old trend, a day excluded, zeros, too few data",
			td:   dayBody{Metrics: metrics("sleep_h", 0.0)},
			yd: dayBody{
				Type: "training", Status: "excluded", Total: nutrientsBody{Kcal: 1800, Protein: 120.5, Carbs: 150, Fat: 60},
				Meals: []mealBody{{}}, Metrics: metrics("steps", 0.0),
			},
			sum: summaryBody{Trend: &trendBody{Kg: 80, Date: "2026-10-20"}, Verdict: verdictBody{Rate: "none", Reasons: []string{"not_trusted"}}},
			want: `Jutarnji sažetak, petak 23. 10.
Težina: trend 80,0 kg (od 20. 10.)
Jučer (dan treninga, izuzet iz kalibracije): 1800 kcal · P 121 g · UH 150 g · M 60 g
Kalibracija: još nema pouzdane procjene`,
		},
	} {
		if got := morningText(today, tc.td, tc.yd, tc.sum, day.Date{}, "hr"); got != tc.want {
			t.Errorf("%s:\n%s\nwant\n%s", tc.name, got, tc.want)
		}
	}
}

// Calibration's line says the rate against the target and what the
// verdict proposes.
func TestCalibrationLine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		v    verdictBody
		want string
	}{
		{
			verdictBody{Rate: "below", MeasuredPctWeek: new(-1.1), TargetPctWeek: new(-0.7), Action: "raise_kcal", GapKcal: 149.6},
			"tempo −1,10 %/tj, cilj −0,70 %/tj, ispod plana; prijedlog: +150 kcal/dan",
		},
		{
			verdictBody{Rate: "above", MeasuredPctWeek: new(-0.2), TargetPctWeek: new(-0.7), Action: "lower_kcal", GapKcal: -200},
			"tempo −0,20 %/tj, cilj −0,70 %/tj, iznad plana; prijedlog: −200 kcal/dan",
		},
		{
			verdictBody{Rate: "above", MeasuredPctWeek: new(0.4), TargetPctWeek: new(0.25), Action: "shrink_surplus"},
			"tempo +0,40 %/tj, cilj +0,25 %/tj, iznad plana; prijedlog: manji suficit",
		},
		{
			verdictBody{Rate: "below", MeasuredPctWeek: new(-0.9), TargetPctWeek: new(-0.7), Action: "raise_protein_or_kcal"},
			"prijedlog: više proteina ili kcal",
		},
		{
			verdictBody{Rate: "on_target", MeasuredPctWeek: new(-0.001), TargetPctWeek: new(0.0), Action: "maintain"},
			"tempo 0,00 %/tj, cilj 0,00 %/tj, u planu; prijedlog: prijeđi na održavanje",
		},
	} {
		if got := (writer{croatian}).calibrationLine(tc.v); !strings.Contains(got, tc.want) {
			t.Errorf("%+v = %q, want %q in it", tc.v, got, tc.want)
		}
	}
}

// In English, and in any locale without its own phrases, the summary has
// English words and a decimal point.
func TestMorningTextInEnglish(t *testing.T) {
	t.Parallel()

	today := day.Date{Year: 2026, Month: 10, Day: 23}
	td := dayBody{Metrics: metrics("sleep_h", 6.75, "weight", 80.25)}
	yd := dayBody{
		Type: "training", Status: "complete", Total: nutrientsBody{Kcal: 2010, Protein: 150.4, Carbs: 200, Fat: 60},
		Target: &dayTargetBody{Kcal: 2100, Protein: 150, Carbs: 220, Fat: 65}, Meals: []mealBody{{}},
		Metrics: metrics("steps", 8000.0),
	}
	sum := summaryBody{
		Trend: &trendBody{Kg: 80.6, Date: "2026-10-19"},
		Verdict: verdictBody{
			Rate: "below", MeasuredPctWeek: new(-1.05), TargetPctWeek: new(-0.7), Action: "raise_kcal", GapKcal: 120,
		},
		Recovery: recoveryBody{Flags: []string{"short_sleep"}, SleepH: new(6.2)},
	}
	want := `Morning summary, Friday Oct 23
Sleep: 6 h 45 min
Weight: 80.3 kg · trend 80.6 kg (since Oct 19)
Yesterday (training day): 2010/2100 kcal · P 150/150 g · C 200/220 g · F 60/65 g
Activity yesterday: steps 8000
Calibration: rate −1.05 %/wk, target −0.70 %/wk, below plan; proposed: +120 kcal/day
Recovery: short sleep (average 6.2 h)`
	for _, locale := range []string{"en", "de"} {
		if got := morningText(today, td, yd, sum, day.Date{}, locale); got != want {
			t.Errorf("%s:\n%s\nwant\n%s", locale, got, want)
		}
	}
}

// A proposal of the plan that waits for the user's answer has a line of
// its own, after calibration's, with the day it waits until.
func TestMorningTextSaysAProposalWaits(t *testing.T) {
	t.Parallel()

	today := day.Date{Year: 2026, Month: 10, Day: 23}
	sum := summaryBody{Verdict: verdictBody{Rate: "none", Reasons: []string{"not_trusted"}}}
	yd := dayBody{Type: "rest", Status: "complete"}
	waits := day.Date{Year: 2026, Month: 10, Day: 30}
	for locale, want := range map[string]string{
		"hr": "Kalibracija: još nema pouzdane procjene\nPlan: prijedlog promjene čeka tvoj odgovor, do 30. 10.",
		"en": "Calibration: no estimate to trust yet\nPlan: a proposed change waits for your answer, until Oct 30",
	} {
		if got := morningText(today, dayBody{}, yd, sum, waits, locale); !strings.HasSuffix(got, want) {
			t.Errorf("%s:\n%s\nwant it to end\n%s", locale, got, want)
		}
	}
}
