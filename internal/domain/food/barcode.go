package food

import (
	"errors"
	"fmt"
	"strings"
)

// MaxBarcodes bounds a food's barcodes: one for each size of its package.
const MaxBarcodes = 10

// Barcode returns the canonical form of a product's barcode, a GTIN of 8,
// 12, 13 or 14 digits whose last is its check digit, its spaces left out
// (an EAN-13 is printed in groups): a UPC-A (12 digits) as the EAN-13 it
// is, a 0 before it, and a GTIN-14 that starts with a 0 as that EAN-13,
// so a scan finds a food in whichever form it reads the code. An EAN-8
// stays as it is. Anything else is an error, and so is a wrong check
// digit, which a digit misread off a photo makes.
func Barcode(s string) (string, error) {
	code := strings.ReplaceAll(s, " ", "")
	n := len(code)
	if n != 8 && n != 12 && n != 13 && n != 14 || strings.ContainsFunc(code, func(r rune) bool { return r < '0' || r > '9' }) {
		return "", fmt.Errorf("barcode %q: want 8, 12, 13 or 14 digits", s)
	}
	if !checkDigitOK(code) {
		return "", fmt.Errorf("barcode %q: its check digit is wrong, a digit misread or mistyped", s)
	}
	switch {
	case n == 12:
		return "0" + code, nil
	case n == 14 && code[0] == '0':
		return code[1:], nil
	}
	return code, nil
}

// checkDigitOK reports whether the last digit of code is the check digit
// of the others: their sum, weighted 3 and 1 in turn from the right, and
// the check digit make a multiple of 10.
func checkDigitOK(code string) bool {
	sum := int(code[len(code)-1] - '0')
	for i := len(code) - 2; i >= 0; i-- {
		d := int(code[i] - '0')
		if (len(code)-2-i)%2 == 0 {
			d *= 3
		}
		sum += d
	}
	return sum%10 == 0
}

// CanonicalBarcodes returns codes each in its canonical form (Barcode),
// or as it is when it is no barcode, for Validate to say why.
func CanonicalBarcodes(codes []string) []string {
	if codes == nil {
		return nil
	}
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = c
		if canonical, err := Barcode(c); err == nil {
			out[i] = canonical
		}
	}
	return out
}

// validateBarcodes reports the barcodes of a food that break the rules:
// each is a barcode in its canonical form, once.
func validateBarcodes(codes []string) error {
	if len(codes) > MaxBarcodes {
		return fmt.Errorf("%d barcodes, want at most %d", len(codes), MaxBarcodes)
	}
	var errs []error
	seen := map[string]bool{}
	for _, c := range codes {
		canonical, err := Barcode(c)
		switch {
		case err != nil:
			errs = append(errs, err)
			continue
		case canonical != c:
			errs = append(errs, fmt.Errorf("barcode %q: want it as %q", c, canonical))
			continue
		case seen[c]:
			errs = append(errs, fmt.Errorf("barcode %q is given twice", c))
		}
		seen[c] = true
	}
	return errors.Join(errs...)
}
