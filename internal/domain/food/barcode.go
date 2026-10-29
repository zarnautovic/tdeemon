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
// (an EAN-13 is printed in groups), so a scan finds a food in whichever
// form it reads the code:
//   - a UPC-A (12 digits) as the EAN-13 it is, a 0 before it, and a
//     GTIN-14 that starts with a 0 as that EAN-13;
//   - a UPC-E (8 digits, a 0 or 1 first, whose check digit is its UPC-A's)
//     as the EAN-13 of the UPC-A it stands for;
//   - an EAN-8 as it is, and padded with zeros to 13 or 14 digits as
//     those 8.
//
// Anything else is an error, and so is a wrong check digit, which a digit
// misread off a photo makes. Eight digits that are a valid EAN-8 are one,
// as a code read gives no type.
func Barcode(s string) (string, error) {
	code := strings.ReplaceAll(s, " ", "")
	if n := len(code); n != 8 && n != 12 && n != 13 && n != 14 || strings.ContainsFunc(code, func(r rune) bool { return r < '0' || r > '9' }) {
		return "", fmt.Errorf("barcode %q: want 8, 12, 13 or 14 digits", s)
	}
	if len(code) == 8 && !checkDigitOK(code) && (code[0] == '0' || code[0] == '1') {
		code = upcA(code)
	}
	if !checkDigitOK(code) {
		return "", fmt.Errorf("barcode %q: its check digit is wrong, a digit misread or mistyped", s)
	}
	return canonical(code), nil
}

// canonical is the form Barcode keeps code in, a GTIN whose check digit
// holds: of 12 digits, or 14 that start with a 0, the EAN-13; of 13 that
// start with five zeros, the EAN-8.
func canonical(code string) string {
	switch {
	case len(code) == 12:
		code = "0" + code
	case len(code) == 14 && code[0] == '0':
		code = code[1:]
	}
	if len(code) == 13 && strings.HasPrefix(code, "00000") {
		code = code[5:]
	}
	return code
}

// upcA is the UPC-A a UPC-E stands for: its number system, its six digits
// with the zeros it leaves out put back as its sixth digit says, and its
// check digit.
func upcA(upce string) string {
	ns, d, check := upce[:1], upce[1:7], upce[7:]
	var body string
	switch d[5] {
	case '0', '1', '2':
		body = d[:2] + d[5:6] + "0000" + d[2:5]
	case '3':
		body = d[:3] + "00000" + d[3:5]
	case '4':
		body = d[:4] + "00000" + d[4:5]
	default:
		body = d[:5] + "0000" + d[5:6]
	}
	return ns + body + check
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
