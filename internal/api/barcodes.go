package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/domain/food"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/off"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// servingNames are what a serving of a product of Open Food Facts is
// called, in the user's language: the label's serving.
var servingNames = map[string]string{"en": "serving", "hr": "porcija"}

type barcodeInput struct {
	Code string `path:"code" minLength:"8" maxLength:"20" example:"3850000000015" doc:"An EAN-13, EAN-8, UPC-A or GTIN-14: its digits, the check digit last."`
}

type barcodeOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Barcode       string             `json:"barcode" doc:"The barcode as a food keeps it: a UPC-A as its EAN-13, a 0 before it."`
		Food          *foodBody          `json:"food,omitempty" doc:"Your food of the barcode, archived too, with its last use."`
		OpenFoodFacts *openFoodFactsBody `json:"open_food_facts,omitempty" doc:"Without a food of yours of the barcode, the product at Open Food Facts."`
		NamedFood     *foodBody          `json:"named_food,omitempty" doc:"Your food of the product's name and brand, with its last use, whose barcodes lack the barcode yet: the product as a meal's new food is it, and adds the barcode to it."`
	}
}

// openFoodFactsBody is a product of Open Food Facts as a new food.
type openFoodFactsBody struct {
	Food FoodFields `json:"food" doc:"The product as a new food of the barcode, not saved: what a save of a food, or a meal's new food, takes once you have seen it."`
	URL  string     `json:"url" format:"uri" doc:"The product's page at Open Food Facts, whose data are open under the ODbL: say where they come from."`
}

// lookupBarcode finds what a barcode is: a food of the user's, or else a
// product of Open Food Facts, made a new food for the user to see before
// it is saved, with their food of its name and brand, which the product
// is (a save of the product would correct it).
func (s *sessions) lookupBarcode(ctx context.Context, in *barcodeInput) (*barcodeOutput, error) {
	p, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	code, err := food.Barcode(in.Code)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	out := &barcodeOutput{CacheControl: "no-store"}
	out.Body.Barcode = code
	f, err := s.db.FoodByBarcode(ctx, p.User.ID, code)
	switch {
	case err == nil:
		out.Body.Food, err = s.usedFoodBody(ctx, p.User.ID, &f)
		if err != nil {
			return nil, internalError("looking up the barcode failed", err)
		}
		return out, nil
	case !errors.Is(err, store.ErrNotFound):
		return nil, internalError("looking up the barcode failed", err)
	case s.openFoodFacts == nil:
		return nil, huma.Error404NotFound("no food of yours has the barcode")
	}
	return s.atOpenFoodFacts(ctx, p.User, code, out)
}

// atOpenFoodFacts answers out with Open Food Facts' product of code, as
// a new food in u's language, and u's food of its name and brand.
func (s *sessions) atOpenFoodFacts(ctx context.Context, u user.User, code string, out *barcodeOutput) (*barcodeOutput, error) {
	if _, wait, ok := s.offLookups.take(u.ID.String()); !ok {
		return nil, huma.ErrorWithHeaders(huma.NewError(http.StatusTooManyRequests,
			"too many barcodes looked up at Open Food Facts; try again in a minute"), retryAfter(wait))
	}
	product, err := s.openFoodFacts.Product(ctx, code, u.Locale)
	switch {
	case errors.Is(err, off.ErrNotFound):
		return nil, huma.Error404NotFound("no food of yours has the barcode, nor has Open Food Facts a product of it")
	case err != nil:
		return nil, huma.Error502BadGateway("no food of yours has the barcode, and Open Food Facts did not answer: try again, or make the food",
			internalDetail(err))
	}
	switch named, err := s.db.FoodByKey(ctx, u.ID, product.Name, product.Brand); {
	case err == nil:
		if out.Body.NamedFood, err = s.usedFoodBody(ctx, u.ID, &named); err != nil {
			return nil, internalError("looking up the barcode failed", err)
		}
	case !errors.Is(err, store.ErrNotFound):
		return nil, internalError("looking up the barcode failed", err)
	}
	serving, ok := servingNames[u.Locale]
	if !ok {
		serving = servingNames["en"]
	}
	nf, err := product.Food(serving)
	switch {
	case errors.Is(err, off.ErrIncomplete) && out.Body.NamedFood != nil:
		return out, nil
	case errors.Is(err, off.ErrIncomplete):
		return nil, huma.Error404NotFound(fmt.Sprintf(
			"no food of yours has the barcode; Open Food Facts has the product (%q) without the values a food needs", product.Name))
	case err != nil:
		return nil, internalError("looking up the barcode failed", err)
	}
	out.Body.OpenFoodFacts = &openFoodFactsBody{Food: newFoodBody(&nf).FoodFields, URL: product.URL}
	return out, nil
}

// usedFoodBody is f with its last use, as find_foods gives it: the amount
// a scan of it offers.
func (s *sessions) usedFoodBody(ctx context.Context, userID uuid.UUID, f *food.Food) (*foodBody, error) {
	uses, err := s.db.LastUses(ctx, userID, []uuid.UUID{f.ID})
	if err != nil {
		return nil, err
	}
	b := newFoodBody(f)
	if u, ok := uses[f.ID]; ok {
		b.LastUse = newLastUseBody(u)
	}
	return &b, nil
}
