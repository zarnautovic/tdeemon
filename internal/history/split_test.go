package history_test

import (
	"reflect"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/history"
)

// table is a food table of made-up foods, one of them a combination.
var table = []history.FoodRow{
	{Line: 3, Name: "Obični jogurt", Portion: "100 g"},
	{Line: 4, Name: "Pileća šunka", Portion: "100 g"},
	{Line: 5, Name: "Jagode", Portion: "100 g"},
	{Line: 6, Name: "Banana", Portion: "1 srednja"},
	{Line: 7, Name: "Zobene pahuljice", Portion: "100 g"},
	{Line: 8, Name: "Skyr + zobene + jagode", Portion: "150 g skyra + 40 g zobenih pahuljica + 100 g jagoda"},
	{Line: 9, Name: "Varivo od graha", Portion: "1/4 lonca"},
	{Line: 10, Name: "Sir", Portion: "100 g"},
	{Line: 11, Name: "Riža", Portion: "100 g"},
	{Line: 12, Name: "Đumbir", Portion: "100 g"},
}

// splitOf is the one meal of text split against table.
func splitOf(t *testing.T, text string) history.MealSplit {
	t.Helper()
	days := []history.Day{{Date: "2026-09-01", Meals: []history.Meal{{Time: "08:00", Description: text, Nutrients: food.Nutrients{Kcal: 500}}}}}
	r := history.ProposeSplits(days, table)
	if len(r.Meals) != 1 {
		t.Fatalf("meals = %+v, want one", r.Meals)
	}
	return r.Meals[0]
}

// A meal's text splits into its parts, each with its amount and the food
// of the table it reads as.
func TestProposeSplitsParts(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, text string
		want       []history.Part
		class      history.MealClass
	}{
		{
			"parts joined with a plus, an amount after the name or in brackets",
			"jogurt (300 g) + zobene pahuljice 40 g + jagode (~100 g)",
			[]history.Part{
				{Text: "jogurt (300 g)", Name: "jogurt", Amount: 300, Unit: "g", Food: "Obični jogurt"},
				{Text: "zobene pahuljice 40 g", Name: "zobene pahuljice", Amount: 40, Unit: "g", Food: "Zobene pahuljice"},
				{Text: "jagode (~100 g)", Name: "jagode", Amount: 100, Unit: "g", Food: "Jagode"},
			},
			history.Known,
		},
		{
			"a label before a colon, parts after it at commas and an i, a count of pieces",
			"tost: 120 g kruha, 2 jaja i 30 g sira",
			[]history.Part{
				{Text: "120 g kruha", Name: "kruha", Amount: 120, Unit: "g"},
				{Text: "2 jaja", Name: "jaja", Amount: 2, Unit: "piece"},
				{Text: "30 g sira", Name: "sira", Amount: 30, Unit: "g", Food: "Sir"},
			},
			history.Amounts,
		},
		{
			"a comma before a count is no part of its own; the weight in brackets is the amount",
			"pistacije, 15 komada (~15 g jezgre)",
			[]history.Part{{Text: "pistacije, 15 komada (~15 g jezgre)", Name: "pistacije", Amount: 15, Unit: "g"}},
			history.Amounts,
		},
		{
			"a count after a comma",
			"banana, 1 srednja",
			[]history.Part{{Text: "banana, 1 srednja", Name: "banana", Amount: 1, Unit: "piece", Food: "Banana"}},
			history.Known,
		},
		{
			"an i inside a part's name stays, one after a bracket splits",
			"tost sa sirom i šunkom (2 kriške) i jogurt (200 ml)",
			[]history.Part{
				{Text: "tost sa sirom i šunkom (2 kriške)", Name: "tost sa sirom i šunkom", Amount: 2, Unit: "piece"},
				{Text: "jogurt (200 ml)", Name: "jogurt", Amount: 200, Unit: "ml", Food: "Obični jogurt"},
			},
			history.Amounts,
		},
		{
			"times a count, litres and decilitres, a range, a fraction",
			"jogurt (2 × 150 g) + radler (0,5 L) + sok 2 dl + varivo od graha (~450–500 g) + 1/4 kolača",
			[]history.Part{
				{Text: "jogurt (2 × 150 g)", Name: "jogurt", Amount: 300, Unit: "g", Food: "Obični jogurt"},
				{Text: "radler (0,5 L)", Name: "radler", Amount: 500, Unit: "ml"},
				{Text: "sok 2 dl", Name: "sok", Amount: 200, Unit: "ml"},
				{Text: "varivo od graha (~450–500 g)", Name: "varivo od graha", Amount: 475, Unit: "g", Food: "Varivo od graha"},
				{Text: "1/4 kolača", Name: "kolača", Amount: 0.25, Unit: "piece"},
			},
			history.Amounts,
		},
		{
			"a label stays when what follows it is no parts of their own amounts",
			"Pizza: šunka, sir, gljive",
			[]history.Part{{Text: "Pizza: šunka, sir, gljive", Name: "Pizza: šunka, sir, gljive"}},
			history.Untold,
		},
		{
			"grams written out, decagrams, kilograms, thousands, fillers",
			"100 gr piletine + 200 grama riže + pršut 5 dag + jogurt 0,5 kg + 1.000 g krumpira + oko 30 g sira",
			[]history.Part{
				{Text: "100 gr piletine", Name: "piletine", Amount: 100, Unit: "g"},
				{Text: "200 grama riže", Name: "riže", Amount: 200, Unit: "g", Food: "Riža"},
				{Text: "pršut 5 dag", Name: "pršut", Amount: 50, Unit: "g"},
				{Text: "jogurt 0,5 kg", Name: "jogurt", Amount: 500, Unit: "g", Food: "Obični jogurt"},
				{Text: "1.000 g krumpira", Name: "krumpira", Amount: 1000, Unit: "g"},
				{Text: "oko 30 g sira", Name: "sira", Amount: 30, Unit: "g", Food: "Sir"},
			},
			history.Amounts,
		},
		{
			"kcal, milligrams, a fraction of nothing and nothing are no amounts",
			"250 kcal sladoleda + 200 mg kofeina + 1/0 kolača + 0 g maslaca",
			[]history.Part{
				{Text: "250 kcal sladoleda", Name: "kcal sladoleda"},
				{Text: "200 mg kofeina", Name: "mg kofeina"},
				{Text: "1/0 kolača", Name: "kolača"},
				{Text: "0 g maslaca", Name: "maslaca"},
			},
			history.Untold,
		},
		{
			"a plate without amounts",
			"wok piletina s rižom i povrćem (velika zdjela)",
			[]history.Part{{Text: "wok piletina s rižom i povrćem (velika zdjela)", Name: "wok piletina s rižom i povrćem"}},
			history.Untold,
		},
		{
			"some parts with an amount",
			"juha + 2 šnite kruha",
			[]history.Part{
				{Text: "juha", Name: "juha"},
				{Text: "2 šnite kruha", Name: "kruha", Amount: 2, Unit: "piece"},
			},
			history.Partial,
		},
	} {
		got := splitOf(t, c.text)
		if !reflect.DeepEqual(got.Parts, c.want) {
			t.Errorf("%s: parts of %q =\n%+v\nwant\n%+v", c.name, c.text, got.Parts, c.want)
		}
		if got.Class != c.class {
			t.Errorf("%s: class %s, want %s", c.name, got.Class, c.class)
		}
	}
}

// A part reads as a food of the table by its words, inflected too, but
// not by a word that only starts alike, and not as a combination.
func TestProposeSplitsFoods(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]string{
		"jogurta (100 g)":        "Obični jogurt",
		"pileće šunke (50 g)":    "Pileća šunka",
		"grill piletina (150 g)": "",
		// "piletina" is not "pileća": they start alike only.
		"piletina (150 g)": "",
		// A combination is no food a part reads as.
		"skyr sa zobenim (190 g)":   "",
		"zobenih pahuljica (40 g)":  "Zobene pahuljice",
		"skyr (150 g)":              "",
		"jagoda (100 g)":            "Jagode",
		"mali sok od jagoda (2 dl)": "",
		"sirup (20 ml)":             "",
		"rižoto (300 g)":            "",
		// "pola" and "malo" name no food.
		"pola banane":   "Banana",
		"dumbir (10 g)": "Đumbir",
	} {
		if got := splitOf(t, text).Parts[0].Food; got != want {
			t.Errorf("%q reads as %q, want %q", text, got, want)
		}
	}
}

// A combination of the table is split from its portion, which names the
// parts; the other foods are none.
func TestProposeSplitsCombinations(t *testing.T) {
	t.Parallel()

	r := history.ProposeSplits(nil, table)
	want := []history.FoodSplit{{
		Line: 8, Name: "Skyr + zobene + jagode", Portion: "150 g skyra + 40 g zobenih pahuljica + 100 g jagoda",
		Parts: []history.Part{
			{Text: "150 g skyra", Name: "skyra", Amount: 150, Unit: "g"},
			{Text: "40 g zobenih pahuljica", Name: "zobenih pahuljica", Amount: 40, Unit: "g", Food: "Zobene pahuljice"},
			{Text: "100 g jagoda", Name: "jagoda", Amount: 100, Unit: "g", Food: "Jagode"},
		},
	}}
	if !reflect.DeepEqual(r.Foods, want) {
		t.Errorf("combinations =\n%+v\nwant\n%+v", r.Foods, want)
	}
	if r.Meals == nil || len(r.Meals) != 0 {
		t.Errorf("meals = %#v, want none, as a list", r.Meals)
	}
}

// A report counts its meals by class, and keeps each meal's day, time,
// text and kcal.
func TestProposeSplitsCount(t *testing.T) {
	t.Parallel()

	days := []history.Day{
		{Date: "2026-09-01", Meals: []history.Meal{
			{Time: "08:00", Description: "jagode (100 g)", Nutrients: food.Nutrients{Kcal: 32}},
			{Time: "13:00", Description: "ručak u restoranu", Nutrients: food.Nutrients{Kcal: 900}},
		}},
		{Date: "2026-09-02", Meals: []history.Meal{{Description: "banana, 1 srednja", Nutrients: food.Nutrients{Kcal: 105}}}},
	}
	r := history.ProposeSplits(days, table)
	if got, want := r.Count(), map[history.MealClass]int{history.Known: 2, history.Untold: 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("count = %v, want %v", got, want)
	}
	if m := r.Meals[1]; m.Date != "2026-09-01" || m.Index != 1 || m.Time != "13:00" || m.Description != "ručak u restoranu" || m.Kcal != 900 {
		t.Errorf("the second meal = %+v", m)
	}
}
