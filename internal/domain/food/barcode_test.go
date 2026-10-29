package food

import (
	"slices"
	"strings"
	"testing"
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
	} {
		if got, err := Barcode(in); err != nil || got != want {
			t.Errorf("Barcode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, want := range map[string]string{
		"4006381333932":  "check digit",
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
