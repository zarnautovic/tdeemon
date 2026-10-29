// Package off looks a product up by its barcode at Open Food Facts
// (openfoodfacts.org), a database of food products people fill in from
// their labels, open under the ODbL, and makes a new food of it: what a
// barcode a user scans may be when none of their foods has it.
package off

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
)

// DefaultBaseURL is the world's Open Food Facts.
const DefaultBaseURL = "https://world.openfoodfacts.org"

// timeout bounds a lookup, which a person waits for, a scan in hand.
const timeout = 8 * time.Second

// maxBody bounds the answer read: a product's fields asked for are a few
// kilobytes.
const maxBody = 1 << 20

// Why a lookup gives no food.
var (
	// ErrNotFound: Open Food Facts has no product of the barcode.
	ErrNotFound = errors.New("open food facts has no product of the barcode")
	// ErrIncomplete: the product lacks what a food needs (its name, its
	// energy, protein, carbs and fat per 100 g or 100 ml), or its values
	// break a food's rules.
	ErrIncomplete = errors.New("open food facts has the product without what a food needs")
)

// Client asks Open Food Facts.
type Client struct {
	// BaseURL is the server's; DefaultBaseURL when empty.
	BaseURL string
	// UserAgent names the app and where to reach its people, as Open Food
	// Facts asks of every client.
	UserAgent string
	// Client is the HTTP client; one with a timeout when nil.
	Client *http.Client
}

// Product is what Open Food Facts has of a product that a food takes.
type Product struct {
	// Code is the barcode looked up, which the food keeps: Open Food Facts
	// may answer under another form of it, or a product it merged.
	Code string
	// Name is the product's name in the language asked for, or else in
	// its main one.
	Name string
	// Brand is the first of its brands.
	Brand string
	// PerMilliliter says its values are per 100 ml: a drink's, sold by
	// volume. Open Food Facts keeps values per 100 g or 100 ml under one
	// name, and a label gives an ice cream sold by volume per 100 g.
	PerMilliliter bool
	// Nutrients per 100 g or 100 ml: those Open Food Facts has, by its
	// names (proteins, carbohydrates, saturated-fat…), and the energy in
	// kcal, from kJ when it has no kcal.
	Nutrients map[string]float64
	// Serving is the amount of a serving, in g or ml as the values are;
	// zero when unknown.
	Serving float64
	// URL is the product's page.
	URL string
}

// fields are the product's fields a lookup asks for, besides its name in
// the language asked for.
var fields = []string{
	"product_name", "brands", "categories_tags", "nutriments", "product_quantity_unit", "serving_quantity", "serving_quantity_unit",
}

// nutrientNames are the nutrients a food keeps, by their names at Open
// Food Facts. Its alcohol is % vol, not grams, and is left out.
var nutrientNames = []string{"proteins", "carbohydrates", "fat", "fiber", "saturated-fat", "sugars", "salt", "polyols"}

// Product returns the product of code (a barcode in food.Barcode's
// canonical form), with its name in language when it has one, or
// ErrNotFound.
func (c *Client) Product(ctx context.Context, code, language string) (Product, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	name := "product_name_" + language
	q := url.Values{"fields": {strings.Join(append(fields, name), ",")}}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v2/product/"+url.PathEscape(code)+"?"+q.Encode(), http.NoBody)
	if err != nil {
		return Product{}, fmt.Errorf("open food facts: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Product{}, fmt.Errorf("open food facts: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var answer struct {
		Status  int            `json:"status"`
		Product map[string]any `json:"product"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Product{}, fmt.Errorf("open food facts: %w", err)
	}
	// It answers a product it does not have 404, with a status of 0; a
	// 404 of another body is no answer of its (a wrong URL).
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return Product{}, fmt.Errorf("open food facts: %s", resp.Status)
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return Product{}, fmt.Errorf("open food facts: %s, the answer: %w", resp.Status, err)
	}
	if answer.Status != 1 || answer.Product == nil {
		return Product{}, ErrNotFound
	}
	return product(answer.Product, code, name, base), nil
}

// product reads p, Open Food Facts' product of code, its name in the
// language asked for under name.
func product(p map[string]any, code, name, base string) Product {
	out := Product{Code: code, Nutrients: map[string]float64{}, URL: base + "/product/" + url.PathEscape(code)}
	out.Name, _ = p[name].(string)
	if strings.TrimSpace(out.Name) == "" {
		out.Name, _ = p["product_name"].(string)
	}
	// Anyone may write them: no more of them goes on than a food holds.
	out.Name = clip(out.Name, food.MaxNameLen)
	brands, _ := p["brands"].(string)
	brand, _, _ := strings.Cut(brands, ",")
	out.Brand = clip(brand, food.MaxBrandLen)
	unit, _ := p["product_quantity_unit"].(string)
	categories, _ := p["categories_tags"].([]any)
	out.PerMilliliter = strings.EqualFold(unit, "ml") && slices.Contains(categories, any("en:beverages"))
	if v, ok := number(p["serving_quantity"]); ok && v > 0 {
		servingUnit, _ := p["serving_quantity_unit"].(string)
		if servingUnit == "" || strings.EqualFold(servingUnit, unitOf(out.PerMilliliter)) {
			out.Serving = v
		}
	}
	n, _ := p["nutriments"].(map[string]any)
	for _, name := range nutrientNames {
		if v, ok := number(n[name+"_100g"]); ok {
			out.Nutrients[name] = v
		}
	}
	if v, ok := number(n["energy-kcal_100g"]); ok {
		out.Nutrients["energy-kcal"] = v
	} else if kj, ok := number(n["energy-kj_100g"]); ok {
		out.Nutrients["energy-kcal"] = math.Round(kj/4.184*10) / 10
	} else if kj, ok := number(n["energy_100g"]); ok {
		// energy is in kJ.
		out.Nutrients["energy-kcal"] = math.Round(kj/4.184*10) / 10
	}
	return out
}

func unitOf(perMilliliter bool) string {
	if perMilliliter {
		return "ml"
	}
	return "g"
}

// number is v as a number: Open Food Facts gives some as text.
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, n >= 0 && !math.IsInf(n, 0)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil && f >= 0 && !math.IsInf(f, 0) && !math.IsNaN(f)
	}
	return 0, false
}

// Food returns a new food of p, not saved: its name and brand, its values
// per 100 g or 100 ml, a serving named serving when Open Food Facts knows
// its amount, the barcode, and Open Food Facts as its source. A product
// without a name or the four values a food needs, or whose four values
// break a food's rules, is ErrIncomplete; an optional value or the
// serving that breaks them (salt in mg, a typo) is left out.
func (p Product) Food(serving string) (food.Food, error) {
	need := func(name string) float64 {
		v, ok := p.Nutrients[name]
		if !ok {
			return math.NaN()
		}
		return v
	}
	f := food.Food{
		ID:     uuid.NewV7(),
		Name:   p.Name,
		Brand:  p.Brand,
		Basis:  food.Per100g,
		Source: food.OpenFoodFacts,
		Nutrients: food.Nutrients{
			Kcal: need("energy-kcal"), Protein: need("proteins"), Carbs: need("carbohydrates"), Fat: need("fat"),
		},
	}
	if p.PerMilliliter {
		f.Basis = food.Per100ml
	}
	if code, err := food.Barcode(p.Code); err == nil {
		f.Barcodes = []string{code}
	}
	if f.Name == "" || math.IsNaN(f.Nutrients.Kcal+f.Nutrients.Protein+f.Nutrients.Carbs+f.Nutrients.Fat) {
		return food.Food{}, ErrIncomplete
	}
	if err := f.Validate(); err != nil {
		return food.Food{}, fmt.Errorf("%w: %w", ErrIncomplete, err)
	}
	// Each of the rest is kept when the food holds with it.
	keep := func(add func(*food.Food)) {
		with := f
		add(&with)
		if with.Validate() == nil {
			f = with
		}
	}
	if p.Serving > 0 {
		keep(func(f *food.Food) { f.Servings = []food.Serving{{Name: serving, Amount: p.Serving}} })
	}
	for _, o := range []struct {
		name string
		at   func(*food.Nutrients) **float64
	}{
		{"fiber", func(n *food.Nutrients) **float64 { return &n.Fiber }},
		{"saturated-fat", func(n *food.Nutrients) **float64 { return &n.SaturatedFat }},
		{"sugars", func(n *food.Nutrients) **float64 { return &n.Sugars }},
		{"salt", func(n *food.Nutrients) **float64 { return &n.Salt }},
		{"polyols", func(n *food.Nutrients) **float64 { return &n.Polyols }},
	} {
		if v, ok := p.Nutrients[o.name]; ok {
			keep(func(f *food.Food) { *o.at(&f.Nutrients) = &v })
		}
	}
	return f, nil
}

// clip is s, its space trimmed, cut to at most n characters.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n]))
}
