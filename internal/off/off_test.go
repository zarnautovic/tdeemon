package off_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/off"
)

// server is an Open Food Facts that answers with answer, status and the
// request it was asked, kept in asked.
func server(t *testing.T, status int, answer string, asked *http.Request) *off.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if asked != nil {
			*asked = *r
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(s.Close)
	return &off.Client{BaseURL: s.URL, UserAgent: "tdeemon/test (https://example.com)"}
}

// A yoghurt as Open Food Facts gives it: some numbers as text, its name
// in Croatian, two brands.
const yoghurt = `{"code": "3850000000015", "status": 1, "status_verbose": "product found", "product": {
	"code": "3850000000015", "product_name": "Greek yoghurt", "product_name_hr": "Grčki jogurt",
	"brands": "Dukat, Lactalis", "product_quantity_unit": "g", "serving_quantity": "150", "serving_quantity_unit": "g",
	"nutriments": {"energy-kcal_100g": 73, "energy-kj_100g": 305, "proteins_100g": "9.5", "carbohydrates_100g": 3.6,
		"sugars_100g": 3.6, "fat_100g": 2, "saturated-fat_100g": 1.3, "salt_100g": 0.1, "alcohol_100g": 0}}}`

func TestProduct(t *testing.T) {
	t.Parallel()

	var asked http.Request
	c := server(t, http.StatusOK, yoghurt, &asked)
	p, err := c.Product(t.Context(), "3850000000015", "hr")
	if err != nil {
		t.Fatal(err)
	}
	if asked.URL.Path != "/api/v2/product/3850000000015" || !strings.Contains(asked.URL.Query().Get("fields"), "product_name_hr") ||
		asked.Header.Get("User-Agent") != "tdeemon/test (https://example.com)" {
		t.Errorf("asked %s %s, user agent %q", asked.URL.Path, asked.URL.RawQuery, asked.Header.Get("User-Agent"))
	}
	want := off.Product{
		Code: "3850000000015", Name: "Grčki jogurt", Brand: "Dukat", Serving: 150, URL: c.BaseURL + "/product/3850000000015",
		Nutrients: map[string]float64{
			"energy-kcal": 73, "proteins": 9.5, "carbohydrates": 3.6, "sugars": 3.6, "fat": 2, "saturated-fat": 1.3, "salt": 0.1,
		},
	}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("the product: %+v, want %+v", p, want)
	}
}

// A product is a food of its name, brand, values, serving and barcode,
// from Open Food Facts; its alcohol, % vol, is left out.
func TestProductAsAFood(t *testing.T) {
	t.Parallel()

	p, err := server(t, http.StatusOK, yoghurt, nil).Product(t.Context(), "3850000000015", "hr")
	if err != nil {
		t.Fatal(err)
	}
	f, err := p.Food("mjera")
	if err != nil {
		t.Fatal(err)
	}
	f.ID = uuid.Nil()
	want := food.Food{
		Name: "Grčki jogurt", Brand: "Dukat", Basis: food.Per100g, Source: food.OpenFoodFacts,
		Nutrients: food.Nutrients{Kcal: 73, Protein: 9.5, Carbs: 3.6, Fat: 2, Sugars: new(3.6), SaturatedFat: new(1.3), Salt: new(0.1)},
		Servings:  []food.Serving{{Name: "mjera", Amount: 150}}, Barcodes: []string{"3850000000015"},
	}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("the food: %+v, want %+v", f, want)
	}
}

// A drink's values are per 100 ml; energy only in kJ is made kcal; the
// name in the main language stands in for one in the language asked.
func TestProductOfADrinkInKJ(t *testing.T) {
	t.Parallel()

	c := server(t, http.StatusOK, `{"status": 1, "product": {"code": "96385074", "product_name": "Orange juice",
		"product_quantity_unit": "ml", "serving_quantity": 250, "serving_quantity_unit": "ml",
		"categories_tags": ["en:plant-based-foods-and-beverages", "en:beverages", "en:fruit-juices"],
		"nutriments": {"energy_100g": 188, "proteins_100g": 0.7, "carbohydrates_100g": 10, "fat_100g": 0}}}`, nil)
	p, err := c.Product(t.Context(), "96385074", "en")
	if err != nil {
		t.Fatal(err)
	}
	f, err := p.Food("serving")
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Orange juice" || f.Basis != food.Per100ml || f.Nutrients.Kcal != 44.9 ||
		!reflect.DeepEqual(f.Servings, []food.Serving{{Name: "serving", Amount: 250}}) {
		t.Errorf("the juice: %+v", f)
	}
}

// An ice cream sold by volume is labelled per 100 g: its values are per
// 100 g, and its serving in ml is no serving of grams. The food keeps the
// barcode looked up, whatever form Open Food Facts answers under; a value
// or a serving no food has is left out, and the rest kept.
func TestProductOfAnIceCream(t *testing.T) {
	t.Parallel()

	c := server(t, http.StatusOK, `{"status": 1, "product": {"code": "36000291452", "product_name": "Vanilla ice cream",
		"product_quantity_unit": "ml", "serving_quantity": 125, "serving_quantity_unit": "ml",
		"categories_tags": ["en:frozen-foods", "en:desserts", "en:ice-creams"],
		"nutriments": {"energy-kcal_100g": 207, "proteins_100g": 3.5, "carbohydrates_100g": 24, "fat_100g": 11,
			"sugars_100g": 21, "salt_100g": 120}}}`, nil)
	p, err := c.Product(t.Context(), "0036000291452", "en")
	if err != nil {
		t.Fatal(err)
	}
	f, err := p.Food("serving")
	if err != nil {
		t.Fatal(err)
	}
	if f.Basis != food.Per100g || len(f.Servings) != 0 || !reflect.DeepEqual(f.Barcodes, []string{"0036000291452"}) ||
		f.Nutrients.Salt != nil || f.Nutrients.Sugars == nil || *f.Nutrients.Sugars != 21 {
		t.Errorf("the ice cream: %+v", f)
	}
	// A serving no food has, of grams.
	p.PerMilliliter, p.Serving = false, 20000
	if f, err := p.Food("serving"); err != nil || len(f.Servings) != 0 {
		t.Errorf("a serving of 20 kg: %+v, %v; want the food without it", f.Servings, err)
	}
}

func TestProductNotFound(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]*off.Client{
		"404":       server(t, http.StatusNotFound, `{"status": 0, "status_verbose": "product not found"}`, nil),
		"status 0":  server(t, http.StatusOK, `{"status": 0, "status_verbose": "product not found"}`, nil),
		"a product": server(t, http.StatusOK, `{"status": 1}`, nil),
	} {
		if _, err := c.Product(t.Context(), "4006381333931", "en"); !errors.Is(err, off.ErrNotFound) {
			t.Errorf("%s: %v, want ErrNotFound", name, err)
		}
	}
	for name, c := range map[string]*off.Client{
		"an error":       server(t, http.StatusServiceUnavailable, `busy`, nil),
		"broken answers": server(t, http.StatusOK, `{"status": 1, "product": `, nil),
		"a wrong URL":    server(t, http.StatusNotFound, `<html>404 page not found</html>`, nil),
	} {
		if _, err := c.Product(t.Context(), "4006381333931", "en"); err == nil || errors.Is(err, off.ErrNotFound) {
			t.Errorf("%s: %v, want an error other than ErrNotFound", name, err)
		}
	}
}

// A product without a name or a needed value, or with values no food
// has, makes no food.
func TestFoodOfAnIncompleteProduct(t *testing.T) {
	t.Parallel()

	whole := map[string]float64{"energy-kcal": 73, "proteins": 9.5, "carbohydrates": 3.6, "fat": 2}
	without := func(name string) map[string]float64 {
		n := map[string]float64{}
		for k, v := range whole {
			if k != name {
				n[k] = v
			}
		}
		return n
	}
	for name, p := range map[string]off.Product{
		"no name":     {Code: "3850000000015", Name: "  ", Nutrients: whole},
		"no energy":   {Code: "3850000000015", Name: "Yoghurt", Nutrients: without("energy-kcal")},
		"no fat":      {Code: "3850000000015", Name: "Yoghurt", Nutrients: without("fat")},
		"no food has": {Code: "3850000000015", Name: "Yoghurt", Nutrients: map[string]float64{"energy-kcal": 2000, "proteins": 9.5, "carbohydrates": 3.6, "fat": 2}},
	} {
		if f, err := p.Food("serving"); !errors.Is(err, off.ErrIncomplete) {
			t.Errorf("%s: %+v, %v; want ErrIncomplete", name, f, err)
		}
	}
	long := off.Product{Code: "3850000000015", Name: strings.Repeat("ž", food.MaxNameLen+5), Nutrients: whole}
	if f, err := long.Food("serving"); err != nil || len([]rune(f.Name)) != food.MaxNameLen {
		t.Errorf("a long name: %d characters, %v; want it cut", len([]rune(f.Name)), err)
	}
}
