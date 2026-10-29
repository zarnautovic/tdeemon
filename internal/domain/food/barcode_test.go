package food

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"uuid"
)

func TestBarcode(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"4006381333931":     "4006381333931",
		"4 006381 333931":   "4006381333931",
		"96385074":          "96385074",
		"036000291452":      "0036000291452",
		"00036000291452":    "0036000291452",
		"10036000291459":    "10036000291459",
		"0036000291452":     "0036000291452",
		" 3850000000015   ": "3850000000015",
		// A UPC-E, of the UPC-A 042100005264; its digits are no EAN-8.
		"04252614": "0042100005264",
		// An EAN-8 padded to 13 and to 14 digits.
		"0000096385074":  "96385074",
		"00000096385074": "96385074",
	} {
		if got, err := Barcode(in); err != nil || got != want {
			t.Errorf("Barcode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, want := range map[string]string{
		"4006381333932":  "check digit",
		"04252615":       "check digit",
		"24252615":       "check digit",
		"4006381333391":  "check digit",
		"400638133393":   "check digit",
		"1234567":        "8, 12, 13 or 14 digits",
		"40063813339310": "check digit",
		"4006381-33393":  "8, 12, 13 or 14 digits",
		"":               "8, 12, 13 or 14 digits",
		"4OO6381333931":  "8, 12, 13 or 14 digits",
		"４００６３８１３３３９３１": "8, 12, 13 or 14 digits",
	} {
		if got, err := Barcode(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Barcode(%q) = %q, %v; want an error of %q", in, got, err, want)
		}
	}
}

func TestCanonicalBarcodes(t *testing.T) {
	t.Parallel()

	got := CanonicalBarcodes([]string{"036000291452", "96385074", "no barcode"})
	if want := []string{"0036000291452", "96385074", "no barcode"}; !slices.Equal(got, want) {
		t.Errorf("CanonicalBarcodes = %q, want %q", got, want)
	}
	if CanonicalBarcodes(nil) != nil {
		t.Error("none made some")
	}
}

func TestValidateBarcodes(t *testing.T) {
	t.Parallel()

	f := egg()
	f.Barcodes = []string{"4006381333931", "96385074"}
	if err := f.Validate(); err != nil {
		t.Fatalf("barcodes: %v", err)
	}
	many := make([]string, MaxBarcodes+1)
	for i := range many {
		many[i] = "4006381333931"
	}
	for name, tc := range map[string]struct {
		codes []string
		want  string
	}{
		"no barcode":           {[]string{"egg"}, "digits"},
		"a misread digit":      {[]string{"4006381333932"}, "check digit"},
		"not canonical":        {[]string{"036000291452"}, `want it as "0036000291452"`},
		"one twice":            {[]string{"96385074", "96385074"}, "given twice"},
		"too many barcodes":    {many, "at most 10"},
		"spaces, as on a pack": {[]string{"4 006381 333931"}, `want it as "4006381333931"`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := egg()
			f.Barcodes = tc.codes
			if err := f.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate = %v, want an error of %q", err, tc.want)
			}
		})
	}
}

// A food without barcodes is the JSON it was before foods had them, as
// main wrote it then: undo compares the state an event recorded with the
// food now, as JSON, and an event of before would be "changed since".
func TestFoodJSONWithoutBarcodes(t *testing.T) {
	t.Parallel()

	f := egg()
	f.ID = uuid.MustParse("0199f0a0-0000-7000-8000-0000000000e9")
	f.Aliases = []string{"hen's egg"}
	f.Nutrients.Salt = new(0.3)
	got, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	const before = `{"ID":"0199f0a0-0000-7000-8000-0000000000e9","Name":"Egg","Brand":"","Basis":"100g",` +
		`"Nutrients":{"Kcal":143,"Protein":12.6,"Carbs":0.7,"Fat":9.5,"Fiber":null,"SaturatedFat":null,"Sugars":null,"Salt":0.3,"Polyols":null,"Alcohol":null},` +
		`"PortionGrams":0,"Servings":[{"Name":"piece","Amount":60}],"Aliases":["hen's egg"],"Source":"estimate","Note":"","Archived":false}`
	if string(got) != before {
		t.Errorf("the JSON of a food without barcodes:\n%s\nwant it as before them:\n%s", got, before)
	}
	f.Barcodes = []string{"96385074"}
	if got, err := json.Marshal(f); err != nil || !strings.Contains(string(got), `"Barcodes":["96385074"]`) {
		t.Errorf("the JSON of a food with a barcode: %s, %v", got, err)
	}
}
