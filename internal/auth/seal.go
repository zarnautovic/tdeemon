package auth

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// ErrBadSeal reports a sealed value that was changed, sealed with another
// key or for another purpose, or is not a sealed value at all.
var ErrBadSeal = errors.New("sealed value is not valid")

// Seal returns value with an HMAC-SHA256 over it, as
// base64url(value).base64url(mac). The MAC key is derived from secret for
// purpose (HKDF), so a value sealed for one purpose never opens for
// another. Seal protects integrity only: value is readable.
func Seal(secret []byte, purpose string, value []byte) string {
	v := base64.RawURLEncoding.EncodeToString(value)
	return v + "." + base64.RawURLEncoding.EncodeToString(mac(secret, purpose, v))
}

// Open checks a value from Seal and returns it.
func Open(secret []byte, purpose, sealed string) ([]byte, error) {
	v, m, ok := strings.Cut(sealed, ".")
	if !ok {
		return nil, ErrBadSeal
	}
	got, err := base64.RawURLEncoding.DecodeString(m)
	if err != nil || !hmac.Equal(got, mac(secret, purpose, v)) {
		return nil, ErrBadSeal
	}
	value, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, ErrBadSeal
	}
	return value, nil
}

func mac(secret []byte, purpose, value string) []byte {
	// HKDF fails only for an output longer than 255 hashes.
	key, _ := hkdf.Key(sha256.New, secret, nil, "tdeemon seal: "+purpose, sha256.Size)
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}
