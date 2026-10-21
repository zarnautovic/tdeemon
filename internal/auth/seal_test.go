package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestSealAndOpen(t *testing.T) {
	t.Parallel()

	secret := []byte(strings.Repeat("k", 32))
	sealed := Seal(secret, "oidc", []byte(`{"state":"x"}`))
	got, err := Open(secret, "oidc", sealed)
	if err != nil || string(got) != `{"state":"x"}` {
		t.Fatalf("Open = %q, %v; want the value", got, err)
	}
	if strings.ContainsAny(sealed, "+/=") {
		t.Errorf("sealed value %q is not URL-safe", sealed)
	}
}

func TestOpenRefuses(t *testing.T) {
	t.Parallel()

	secret := []byte(strings.Repeat("k", 32))
	sealed := Seal(secret, "oidc", []byte(`{"state":"x"}`))
	v, m, _ := strings.Cut(sealed, ".")
	other := Seal(secret, "oidc", []byte(`{"state":"y"}`))
	otherValue, _, _ := strings.Cut(other, ".")
	// A MAC that is right for bytes that are not base64.
	notBase64 := "!!!"
	rightMAC := base64.RawURLEncoding.EncodeToString(mac(secret, "oidc", notBase64))

	for _, tc := range []struct {
		name, purpose, sealed string
		secret                []byte
	}{
		{"another purpose", "session", sealed, secret},
		{"another key", "oidc", sealed, []byte(strings.Repeat("o", 32))},
		{"changed value", "oidc", otherValue + "." + m, secret},
		{"no MAC", "oidc", v, secret},
		{"empty", "oidc", "", secret},
		{"MAC not base64", "oidc", v + ".!!!", secret},
		{"value not base64", "oidc", notBase64 + "." + rightMAC, secret},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Open(tc.secret, tc.purpose, tc.sealed); !errors.Is(err, ErrBadSeal) {
				t.Errorf("Open = %v, want ErrBadSeal", err)
			}
		})
	}
}
