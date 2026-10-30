package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/meal"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// recipeAnswer is the parts of a recipe's answer the tests look at.
type recipeAnswer struct {
	Food struct {
		ID        string `json:"id"`
		Source    string `json:"source"`
		Nutrients struct {
			Kcal float64 `json:"kcal"`
		} `json:"nutrients"`
		Servings []struct {
			Name   string  `json:"name"`
			Amount float64 `json:"amount"`
		} `json:"servings"`
		Recipe *struct {
			Weight    float64 `json:"weight"`
			YieldG    float64 `json:"yield_g"`
			RawWeight float64 `json:"raw_weight"`
			Portions  int     `json:"portions"`
			PotG      float64 `json:"pot_g"`
			Total     struct {
				Kcal float64 `json:"kcal"`
			} `json:"total"`
			Portion *struct {
				Kcal float64 `json:"kcal"`
			} `json:"portion"`
			Items []struct {
				FoodID  string `json:"food_id"`
				Name    string `json:"name"`
				Changed bool   `json:"changed"`
			} `json:"items"`
		} `json:"recipe"`
	} `json:"food"`
	Warnings []struct {
		Code string `json:"code"`
	} `json:"warnings"`
}

func decodeRecipe(t *testing.T, body []byte) recipeAnswer {
	t.Helper()
	var a recipeAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return a
}

// pudding is a recipe of bread and a new food, milk: 4 slices (414.4
// kcal) and 300 ml (192 kcal), 420 g cooked in 4 portions.
func pudding(bread food.Food) string {
	return fmt.Sprintf(`{"name": "Bread pudding", "aliases": ["pudding"],
		"items": [{"food_id": %q, "servings": 4, "serving": "slice"},
			{"food": {"name": "Milk", "basis": "100ml", "source": "label", "nutrients": {"kcal": 64, "protein": 3.3, "carbs": 4.8, "fat": 3.5}}, "amount": 300, "unit": "ml"}],
		"yield_g": 420, "portions": 4, "pot_g": 800}`, bread.ID)
}

func TestUpsertRecipe(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	bread := saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	rec := in.do(t, http.MethodPost, "/api/v1/recipes", pudding(bread), cookies, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	a := decodeRecipe(t, rec.Body.Bytes())
	checkPudding(t, a, rec.Body.String())
	recipeSavedAgain(t, in, cookies, bread, a.Food.ID)
}

// checkPudding checks the answer of pudding's save, body.
func checkPudding(t *testing.T, a recipeAnswer, body string) {
	t.Helper()
	r := a.Food.Recipe
	switch {
	case a.Food.Source != "recipe" || r == nil:
		t.Fatalf("answer %s, want a recipe's food with its recipe", body)
	case r.Weight != 420 || r.YieldG != 420 || r.RawWeight != 460 || r.Portions != 4 || r.PotG != 800:
		t.Errorf("recipe %+v, want 420 g weighed of 460 g raw, 4 portions, a pot of 800 g", *r)
	case r.Total.Kcal != 606 || r.Portion == nil || r.Portion.Kcal != 152:
		t.Errorf("total %v, portion %v; want 606 and 152 kcal", r.Total, r.Portion)
	case len(r.Items) != 2 || r.Items[1].Name != "Milk" || r.Items[0].Changed:
		t.Errorf("items %+v, want the bread and the new milk", r.Items)
	}
	checkPuddingFood(t, a)
}

// checkPuddingFood checks the servings and warnings of pudding's save.
func checkPuddingFood(t *testing.T, a recipeAnswer) {
	t.Helper()
	switch {
	case len(a.Food.Servings) != 2 || a.Food.Servings[0].Name != "recipe" || a.Food.Servings[1].Amount != 105:
		t.Errorf("servings %+v, want the recipe of 420 g and a portion of 105 g", a.Food.Servings)
	case len(a.Warnings) != 0:
		t.Errorf("warnings %+v, want none", a.Warnings)
	}
}

// recipeSavedAgain saves pudding, the recipe id, again: by its name, by
// its id, by an id of none, and as a new recipe.
func recipeSavedAgain(t *testing.T, in *instance, cookies []*http.Cookie, bread food.Food, id string) {
	t.Helper()
	// Saved again by its name, not weighed: the same recipe, of 460 g.
	again := strings.Replace(pudding(bread), `"yield_g": 420, `, "", 1)
	rec := in.do(t, http.MethodPost, "/api/v1/recipes", again, cookies, nil)
	if b := decodeRecipe(t, rec.Body.Bytes()); rec.Code != http.StatusOK || b.Food.ID != id || b.Food.Recipe.Weight != 460 || b.Food.Recipe.YieldG != 0 {
		t.Errorf("saved again: %d %s, want the same recipe, of the raw 460 g", rec.Code, rec.Body.String())
	}
	// Corrected by its id; one of no recipe is a 404.
	byID := `{"id": "` + id + `", ` + strings.TrimPrefix(pudding(bread), "{")
	if rec := in.do(t, http.MethodPost, "/api/v1/recipes", byID, cookies, nil); rec.Code != http.StatusOK {
		t.Errorf("by its id: %d %s", rec.Code, rec.Body.String())
	}
	none := `{"id": "` + uuid.NewV7().String() + `", ` + strings.TrimPrefix(pudding(bread), "{")
	if rec := in.do(t, http.MethodPost, "/api/v1/recipes", none, cookies, nil); rec.Code != http.StatusNotFound {
		t.Errorf("by an id of none: %d %s, want 404", rec.Code, rec.Body.String())
	}
	// A new recipe of the name is in the way of create-recipe.
	if rec := in.do(t, http.MethodPost, "/api/v1/recipes/new", pudding(bread), cookies, nil); rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), id) {
		t.Errorf("create-recipe of a name taken: %d %s, want a 409 naming it", rec.Code, rec.Body.String())
	}
}

// A recipe is logged as any food: a portion, a share of the whole dish,
// grams.
func TestLogARecipe(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	bread := saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	a := decodeRecipe(t, in.do(t, http.MethodPost, "/api/v1/recipes", pudding(bread), cookies, nil).Body.Bytes())
	for _, tc := range []struct {
		quantity string
		kcal     float64
	}{
		{`"servings": 1, "serving": "portion"`, 152},
		{`"servings": 0.1, "serving": "recipe"`, 61},
		{`"amount": 210, "unit": "g"`, 303},
	} {
		body := fmt.Sprintf(`{"id": %q, "items": [{"food_id": %q, %s}]}`, meal.NewIDAt(in.clock.Now()), a.Food.ID, tc.quantity)
		rec := in.do(t, http.MethodPost, "/api/v1/meals", body, cookies, nil)
		var m struct {
			Meal struct {
				Items []struct {
					Source    string `json:"source"`
					Nutrients struct {
						Kcal float64 `json:"kcal"`
					} `json:"nutrients"`
				} `json:"items"`
			} `json:"meal"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil || rec.Code != http.StatusCreated || len(m.Meal.Items) != 1 ||
			m.Meal.Items[0].Nutrients.Kcal != tc.kcal || m.Meal.Items[0].Source != "recipe" {
			t.Errorf("%s: %d %s, want %v kcal of the recipe", tc.quantity, rec.Code, rec.Body.String(), tc.kcal)
		}
	}
}

// find_foods and the list say a recipe's food's recipe, without its
// ingredients; get_food gives them, each marked changed whose food gives
// another amount now.
func TestRecipeInFoods(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	bread := saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	a := decodeRecipe(t, in.do(t, http.MethodPost, "/api/v1/recipes", pudding(bread), cookies, nil).Body.Bytes())

	for _, path := range []string{"/api/v1/foods?query=pudd", "/api/v1/foods/list?source=recipe"} {
		rec := in.do(t, http.MethodGet, path, "", cookies, nil)
		var raw struct {
			Foods []json.RawMessage `json:"foods"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil || len(raw.Foods) != 1 {
			t.Fatalf("%s: %d %s, want the recipe alone", path, rec.Code, rec.Body.String())
		}
		f := decodeRecipe(t, []byte(`{"food": `+string(raw.Foods[0])+`}`))
		if r := f.Food.Recipe; r == nil || r.Weight != 420 || r.Portions != 4 || len(r.Items) != 0 {
			t.Errorf("%s: %s, want the recipe without its ingredients", path, rec.Body.String())
		}
	}

	bread.Nutrients.Kcal = 300
	if _, err := in.db.SaveFood(t.Context(), in.user.ID, isolationActor, bread, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	rec := in.do(t, http.MethodGet, "/api/v1/foods/"+a.Food.ID, "", cookies, nil)
	f := decodeRecipe(t, []byte(`{"food": `+rec.Body.String()+`}`))
	if r := f.Food.Recipe; rec.Code != http.StatusOK || r == nil || len(r.Items) != 2 || !r.Items[0].Changed || r.Items[1].Changed {
		t.Errorf("get_food: %d %s, want the bread marked changed, the milk not", rec.Code, rec.Body.String())
	}
}

func TestUpsertRecipeRefusals(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	bread := saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	a := decodeRecipe(t, in.do(t, http.MethodPost, "/api/v1/recipes", pudding(bread), cookies, nil).Body.Bytes())

	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{
			"an estimate", `{"name": "Plate", "items": [{"name": "A plate", "nutrients": {"kcal": 500, "protein": 20, "carbs": 50, "fat": 20}}]}`,
			http.StatusUnprocessableEntity, "no estimate",
		},
		{"no ingredients", `{"name": "Air", "items": []}`, http.StatusUnprocessableEntity, ""},
		{
			"a food's name", fmt.Sprintf(`{"name": "Ana's rye bread", "items": [{"food_id": %q, "amount": 100, "unit": "g"}]}`, bread.ID),
			http.StatusConflict, bread.ID.String(),
		},
		{
			"itself", fmt.Sprintf(`{"name": "Bread pudding", "items": [{"food_id": %q, "amount": 100, "unit": "g"}]}`, a.Food.ID),
			http.StatusUnprocessableEntity, "its own ingredient",
		},
		{
			"what no food has", fmt.Sprintf(`{"name": "Dense", "items": [{"food_id": %q, "amount": 1000, "unit": "g"}], "yield_g": 100}`, bread.ID),
			http.StatusUnprocessableEntity, "the dish of 100 g",
		},
		{
			"a serving it lacks", fmt.Sprintf(`{"name": "Odd", "items": [{"food_id": %q, "servings": 1, "serving": "cup"}]}`, bread.ID),
			http.StatusUnprocessableEntity, "no serving",
		},
	} {
		rec := in.do(t, http.MethodPost, "/api/v1/recipes", tc.body, cookies, nil)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: %d %s, want %d with %q", tc.name, rec.Code, rec.Body.String(), tc.status, tc.want)
		}
	}

	// A weight far off the raw ingredients' is a warning; so is the milk,
	// the user's food now.
	far := strings.Replace(pudding(bread), `"yield_g": 420`, `"yield_g": 2000`, 1)
	if a := decodeRecipe(t, in.do(t, http.MethodPost, "/api/v1/recipes", far, cookies, nil).Body.Bytes()); len(a.Warnings) != 2 ||
		a.Warnings[0].Code != "food_kept" || a.Warnings[1].Code != "yield_high" {
		t.Errorf("warnings %+v, want food_kept and yield_high", a.Warnings)
	}

	recipeFoodAsAFood(t, in, cookies, a.Food.ID)
}

// recipeFoodAsAFood saves the recipe id's food as a food: archived, which
// answers with its recipe, but not changed.
func recipeFoodAsAFood(t *testing.T, in *instance, cookies []*http.Cookie, id string) {
	t.Helper()
	rec := in.do(t, http.MethodGet, "/api/v1/foods/"+id, "", cookies, nil)
	var f map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	delete(f, "recipe")
	f["archived"] = true
	archived, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if rec := in.do(t, http.MethodPost, "/api/v1/foods", string(archived), cookies, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"archived":true`) ||
		!strings.Contains(rec.Body.String(), `"recipe":{`) {
		t.Errorf("archive the recipe's food: %d %s", rec.Code, rec.Body.String())
	}
	n, ok := f["nutrients"].(map[string]any)
	if !ok {
		t.Fatalf("nutrients %v", f["nutrients"])
	}
	n["kcal"] = 10.0
	changed, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if rec := in.do(t, http.MethodPost, "/api/v1/foods", string(changed), cookies, nil); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "only with its recipe") {
		t.Errorf("change the recipe's food: %d %s, want a 422", rec.Code, rec.Body.String())
	}
}

// Another user's food is no ingredient of a recipe: refused as one that
// does not exist.
func TestUpsertRecipeOfAnotherUsersFood(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	now := in.clock.Now()
	ivo := user.User{
		ID: uuid.NewV7(), Email: "ivo@example.com", Name: "Ivo", Role: user.RoleUser,
		Locale: "en", TimeZone: "UTC", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := in.db.CreateUser(t.Context(), ivo, isolationHash); err != nil {
		t.Fatal(err)
	}
	theirs := saveIsolationFoods(t, in.db, ivo, now)
	body := func(id string) string {
		return fmt.Sprintf(`{"name": "Croutons", "items": [{"food_id": %q, "amount": 200, "unit": "g"}]}`, id)
	}
	other := in.do(t, http.MethodPost, "/api/v1/recipes", body(theirs.ID.String()), []*http.Cookie{session}, nil)
	noneID := uuid.NewV7().String()
	none := in.do(t, http.MethodPost, "/api/v1/recipes", body(noneID), []*http.Cookie{session}, nil)
	if other.Code != http.StatusUnprocessableEntity ||
		strings.ReplaceAll(other.Body.String(), theirs.ID.String(), "X") != strings.ReplaceAll(none.Body.String(), noneID, "X") {
		t.Errorf("another's food: %d %s; none: %d %s; want the same 422", other.Code, other.Body.String(), none.Code, none.Body.String())
	}
}

// A recipe as an ingredient, reweighed, brings as much but weighs
// another amount: the ingredient is marked changed.
func TestRecipeIngredientReweighed(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	bread := saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	inner := decodeRecipe(t, in.do(t, http.MethodPost, "/api/v1/recipes", pudding(bread), cookies, nil).Body.Bytes())
	outer := fmt.Sprintf(`{"name": "Pudding trifle", "items": [{"food_id": %q, "servings": 1, "serving": "recipe"}]}`, inner.Food.ID)
	a := decodeRecipe(t, in.do(t, http.MethodPost, "/api/v1/recipes", outer, cookies, nil).Body.Bytes())
	if a.Food.Recipe == nil || len(a.Food.Recipe.Items) != 1 || a.Food.Recipe.Items[0].Changed {
		t.Fatalf("the trifle: %+v", a.Food.Recipe)
	}
	reweighed := strings.Replace(pudding(bread), `"yield_g": 420`, `"yield_g": 400`, 1)
	if rec := in.do(t, http.MethodPost, "/api/v1/recipes", reweighed, cookies, nil); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rec := in.do(t, http.MethodGet, "/api/v1/foods/"+a.Food.ID, "", cookies, nil)
	f := decodeRecipe(t, []byte(`{"food": `+rec.Body.String()+`}`))
	if r := f.Food.Recipe; r == nil || len(r.Items) != 1 || !r.Items[0].Changed {
		t.Errorf("get_food of the trifle: %s, want the pudding marked changed", rec.Body.String())
	}
}
