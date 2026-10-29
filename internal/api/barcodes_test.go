package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/off"
)

// openFoodFacts is a fake Open Food Facts: its URL, and how many times it
// was asked.
type openFoodFacts struct {
	url   string
	asked *atomic.Int32
}

// withOpenFoodFacts gives in an Open Food Facts that answers each product
// asked for with status and answer.
func withOpenFoodFacts(t *testing.T, in *instance, status int, answer string) openFoodFacts {
	t.Helper()
	var asked atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(s.Close)
	base, _ := url.Parse("http://localhost")
	h, err := api.NewHandler(api.Config{
		Info: testInfo, DB: in.db, BaseURL: base, Logger: slog.New(slog.NewJSONHandler(in.logs, nil)), Now: in.clock.Now,
		OpenFoodFacts: &off.Client{BaseURL: s.URL, UserAgent: "tdeemon/test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	in.h = h
	return openFoodFacts{url: s.URL, asked: &asked}
}

// A kefir as Open Food Facts has it, of a code none of Ana's foods has.
const kefir = `{"status": 1, "product": {"code": "4006381333931", "product_name": "Kefir", "product_name_hr": "Kefir 2,8 %",
	"brands": "Vindija", "product_quantity_unit": "ml", "serving_quantity": 250, "serving_quantity_unit": "ml",
	"categories_tags": ["en:dairies", "en:beverages", "en:dairy-drinks", "en:kefirs"],
	"nutriments": {"energy-kcal_100g": 59, "proteins_100g": 3.3, "carbohydrates_100g": 4.1, "fat_100g": 2.8, "salt_100g": 0.1}}}`

// A barcode is the user's food when one of theirs has it, in any form a
// scan reads, archived too; Open Food Facts is not asked then.
func TestLookupBarcodeOfAFoodOfYours(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	asked := withOpenFoodFacts(t, in, http.StatusOK, kefir).asked
	bread := saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	if err := in.db.SetFoodArchived(t.Context(), in.user.ID, isolationActor, bread.ID, true, in.clock.Now()); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"3850000000015", "3 850000 000015", "03850000000015"} {
		rec := in.do(t, http.MethodGet, "/api/v1/barcodes/"+url.PathEscape(code), "", session, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"barcode":"3850000000015"`) ||
			!strings.Contains(rec.Body.String(), `"id":"`+bread.ID.String()+`"`) || !strings.Contains(rec.Body.String(), `"archived":true`) ||
			strings.Contains(rec.Body.String(), "open_food_facts") {
			t.Errorf("%q: %d %s, want the archived bread", code, rec.Code, rec.Body.String())
		}
	}
	if asked.Load() != 0 {
		t.Errorf("Open Food Facts was asked %d times", asked.Load())
	}
	// A barcode misread is refused, saying why, and asks nobody.
	rec := in.do(t, http.MethodGet, "/api/v1/barcodes/3850000000016", "", session, nil)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "check digit") || asked.Load() != 0 {
		t.Errorf("a misread barcode: %d %s, asked %d", rec.Code, rec.Body.String(), asked.Load())
	}
}

// A barcode of no food of the user's is the product of Open Food Facts as
// a new food, in their language, not saved: saved, it is theirs, and found
// as theirs.
func TestLookupBarcodeAtOpenFoodFacts(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	server := withOpenFoodFacts(t, in, http.StatusOK, kefir)
	rec := in.do(t, http.MethodGet, "/api/v1/barcodes/4006381333931", "", session, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	checkJSON(t, rec.Body.Bytes(), `{"barcode": "4006381333931", "open_food_facts": {
		"food": {"name": "Kefir 2,8 %", "brand": "Vindija", "basis": "100ml", "source": "off",
			"nutrients": {"kcal": 59, "protein": 3.3, "carbs": 4.1, "fat": 2.8, "salt": 0.1},
			"servings": [{"name": "porcija", "amount": 250}], "barcodes": ["4006381333931"]},
		"url": "`+server.url+`/product/4006381333931"}}`)
	if _, err := foodNamed(t, in.db, in.user.ID, "Kefir 2,8 %", "Vindija"); err == nil {
		t.Error("the product was saved before the user saw it")
	}
	found := decodeJSON[struct {
		OpenFoodFacts struct {
			Food json.RawMessage `json:"food"`
		} `json:"open_food_facts"`
	}](t, rec.Body.Bytes())
	saved := in.do(t, http.MethodPost, "/api/v1/foods", string(found.OpenFoodFacts.Food), session, nil)
	if saved.Code != http.StatusOK {
		t.Fatalf("the product saved: %d %s", saved.Code, saved.Body.String())
	}
	rec = in.do(t, http.MethodGet, "/api/v1/barcodes/4006381333931", "", session, nil)
	if !strings.Contains(rec.Body.String(), `"food":{"id":`) || strings.Contains(rec.Body.String(), "open_food_facts") {
		t.Errorf("the barcode once saved: %d %s, want the food of the user's", rec.Code, rec.Body.String())
	}
}

// A product of the name and brand of a food of the user's is that food,
// whose barcodes lack the barcode yet: the answer gives it as named_food,
// never as the barcode's food, with the product, or alone when the
// product lacks its values.
func TestLookupBarcodeOfAProductYouHaveByName(t *testing.T) {
	t.Parallel()

	for name, answer := range map[string]string{
		"a whole product":           kefir,
		"a product without a value": strings.Replace(kefir, `"fat_100g": 2.8, `, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := newInstance(t, "http://localhost", "")
			session := []*http.Cookie{sessionOf(t, in)}
			withOpenFoodFacts(t, in, http.StatusOK, answer)
			rec := in.do(t, http.MethodPost, "/api/v1/foods/new", `{"name": "kefir 2,8 %", "brand": "VINDIJA", "basis": "100ml", "source": "user",
				"nutrients": {"kcal": 60, "protein": 3.2, "carbs": 4, "fat": 2.8}, "aliases": ["my kefir"]}`, session, nil)
			if rec.Code != http.StatusCreated {
				t.Fatalf("the user's kefir: %d %s", rec.Code, rec.Body.String())
			}
			mine := decodeJSON[struct {
				ID string `json:"id"`
			}](t, rec.Body.Bytes())
			rec = in.do(t, http.MethodGet, "/api/v1/barcodes/4006381333931", "", session, nil)
			body := rec.Body.String()
			if rec.Code != http.StatusOK || !strings.Contains(body, `"named_food":{"id":"`+mine.ID+`"`) ||
				strings.Contains(body, `"food":{"id"`) || strings.Contains(body, "open_food_facts") != (answer == kefir) {
				t.Errorf("%d %s, want the user's kefir", rec.Code, body)
			}
		})
	}
}

// A user looks up so many barcodes at Open Food Facts a minute; then 429,
// saying when to try again, while their own foods' barcodes still answer.
func TestLookupBarcodeAtOpenFoodFactsIsLimited(t *testing.T) {
	t.Parallel()

	in := newInstance(t, "http://localhost", "")
	session := []*http.Cookie{sessionOf(t, in)}
	server := withOpenFoodFacts(t, in, http.StatusOK, kefir)
	saveIsolationFoods(t, in.db, in.user, in.clock.Now())
	for range 30 {
		if rec := in.do(t, http.MethodGet, "/api/v1/barcodes/4006381333931", "", session, nil); rec.Code != http.StatusOK {
			t.Fatalf("a lookup: %d %s", rec.Code, rec.Body.String())
		}
	}
	rec := in.do(t, http.MethodGet, "/api/v1/barcodes/4006381333931", "", session, nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || server.asked.Load() != 30 {
		t.Errorf("the 31st lookup: %d %s, Retry-After %q; Open Food Facts asked %d times",
			rec.Code, rec.Body.String(), rec.Header().Get("Retry-After"), server.asked.Load())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/barcodes/3850000000015", "", session, nil); rec.Code != http.StatusOK {
		t.Errorf("their own food's barcode: %d %s", rec.Code, rec.Body.String())
	}
	in.clock.Set(in.clock.Now().Add(time.Minute))
	if rec := in.do(t, http.MethodGet, "/api/v1/barcodes/4006381333931", "", session, nil); rec.Code != http.StatusOK {
		t.Errorf("a minute later: %d %s", rec.Code, rec.Body.String())
	}
}

// A barcode neither has is 404, naming the product Open Food Facts has
// without its values; Open Food Facts that fails is 502, without its
// error, which the log has; and none is asked when the instance has none.
func TestLookupBarcodeNotFound(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		status       int
		answer       string
		want         int
		says, logged string
	}{
		"none at Open Food Facts": {http.StatusNotFound, `{"status": 0}`, http.StatusNotFound, "nor has Open Food Facts", ""},
		"without its values": {
			http.StatusOK, `{"status": 1, "product": {"product_name": "Mystery bar", "nutriments": {}}}`,
			http.StatusNotFound, `Open Food Facts has the product (\"Mystery bar\") without the values`, "",
		},
		"Open Food Facts fails": {http.StatusServiceUnavailable, `busy`, http.StatusBadGateway, "did not answer", "503 Service Unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := newInstance(t, "http://localhost", "")
			session := []*http.Cookie{sessionOf(t, in)}
			withOpenFoodFacts(t, in, tc.status, tc.answer)
			rec := in.do(t, http.MethodGet, "/api/v1/barcodes/96385074", "", session, nil)
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.says) || strings.Contains(rec.Body.String(), "503") {
				t.Errorf("%d %s, want %d saying %q", rec.Code, rec.Body.String(), tc.want, tc.says)
			}
			if tc.logged != "" && !strings.Contains(in.logs.String(), tc.logged) {
				t.Errorf("the log: %s, want %q", in.logs.String(), tc.logged)
			}
		})
	}
	in := newInstance(t, "http://localhost", "")
	rec := in.do(t, http.MethodGet, "/api/v1/barcodes/96385074", "", []*http.Cookie{sessionOf(t, in)}, nil)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "Open Food Facts") {
		t.Errorf("without Open Food Facts: %d %s", rec.Code, rec.Body.String())
	}
}
