// Package auth handles credentials: password rules, hashing and checks.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// Password length, counted in characters (code points) after
// normalization, as NIST SP 800-63B-4 counts them. 15 is NIST's
// recommended minimum for a password that is the only factor, which it is
// here.
const (
	MinPasswordLen = 15
	MaxPasswordLen = 256
)

// Errors a password can break the rules with.
var (
	ErrPasswordTooShort = fmt.Errorf("password is too short; use at least %d characters", MinPasswordLen)
	ErrPasswordTooLong  = fmt.Errorf("password is too long; use at most %d characters", MaxPasswordLen)
	ErrPasswordEncoding = errors.New("password is not valid UTF-8")
)

// ErrInvalidHash reports a stored hash that is not an argon2id hash in
// the format HashPassword writes.
var ErrInvalidHash = errors.New("not an argon2id password hash")

// The argon2id cost: OWASP's minimum (19 MiB, two passes, one lane). A
// hash takes about 35 ms and 19 MiB on a two-core i5-8500T VM. Hashes
// carry their parameters, so raising them later still verifies the old
// hashes.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonSaltLen   = 16
	argonKeyLen    = 32
)

// Limits for what a stored hash may ask of a check: at most 256 MiB and
// ten passes (a few seconds), so a corrupted value cannot exhaust a small
// container. They leave room to raise the cost well past today's.
const (
	maxMemoryKiB = 256 * 1024
	maxTime      = 10
	minSaltLen   = 8
	minKeyLen    = 16
	maxKeyLen    = 1024
)

// CheckPassword reports whether password meets the rules for a new
// password.
func CheckPassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordEncoding
	}
	n := utf8.RuneCountInString(normalize(password))
	switch {
	case n < MinPasswordLen:
		return ErrPasswordTooShort
	case n > MaxPasswordLen:
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword checks password against the rules and returns its
// argon2id hash in the PHC string format:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
//
// with the salt and the hash in unpadded standard base64.
func HashPassword(password string) (string, error) {
	if err := CheckPassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLen)
	_, _ = rand.Read(salt) // crypto/rand.Read never fails.
	key := argon2.IDKey([]byte(normalize(password)), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches encoded, a hash from
// HashPassword, with the parameters stored in it. It does not apply the
// rules for new passwords: a password set under older rules still works.
// The error is ErrInvalidHash when encoded cannot be read.
func VerifyPassword(password, encoded string) (bool, error) {
	h, err := parseHash(encoded)
	if err != nil {
		return false, err
	}
	key := argon2.IDKey([]byte(normalize(password)), h.salt, h.time, h.memoryKiB, h.threads, uint32(len(h.key))) //nolint:gosec // G115: parseHash caps the key length at maxKeyLen
	return subtle.ConstantTimeCompare(key, h.key) == 1, nil
}

// MaxPasswordBytes is the longest password input VerifyLogin considers:
// MaxPasswordLen characters of four bytes each. Anything longer fails
// before normalization, which can make a string many times longer (one
// U+FDFA becomes 18 characters). A password typed in a decomposed form
// takes more bytes than its normal form, so near the length limit one can
// be refused: 1,024 bytes hold 113 Hangul syllables typed as separate
// jamo.
const MaxPasswordBytes = 4 * MaxPasswordLen

// VerifyLogin checks password against encoded, the stored hash of the
// account signing in, or "" when there is no such account or it has no
// password. Either way it runs one argon2id hash, so the time a sign-in
// takes does not tell which addresses have accounts; so does an overlong
// password, which never matches.
func VerifyLogin(password, encoded string) (bool, error) {
	if encoded == "" || len(password) > MaxPasswordBytes {
		_, _ = VerifyPassword(password, dummyHash())
		return false, nil
	}
	return VerifyPassword(password, encoded)
}

// dummyHash is a hash of a password nobody knows, with the current
// parameters, for VerifyLogin to spend the time on.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword(GeneratePassword()) // A generated password always meets the rules.
	return h
})

// NeedsRehash reports whether encoded was made with other parameters than
// HashPassword uses now, so a successful sign-in should store a new hash.
// A hash it cannot read needs no rehash: it never verifies.
func NeedsRehash(encoded string) bool {
	h, err := parseHash(encoded)
	if err != nil {
		return false
	}
	return h.memoryKiB != argonMemoryKiB || h.time != argonTime || h.threads != argonThreads ||
		len(h.salt) != argonSaltLen || len(h.key) != argonKeyLen
}

// GeneratePassword returns a random password with 130 bits of entropy:
// 26 characters of A–Z and 2–7, easy to read out and type.
func GeneratePassword() string { return rand.Text() }

// normalize applies NFKC, as NIST SP 800-63B-4 advises, so the same
// password typed on different systems (a composed or a decomposed "č",
// a full-width digit) hashes the same.
func normalize(password string) string { return norm.NFKC.String(password) }

type hash struct {
	memoryKiB, time uint32
	threads         uint8
	salt, key       []byte
}

func parseHash(encoded string) (hash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return hash{}, ErrInvalidHash
	}
	var h hash
	var err error
	if h.memoryKiB, h.time, h.threads, err = parseParams(parts[3]); err != nil {
		return hash{}, err
	}
	if h.salt, err = base64.RawStdEncoding.Strict().DecodeString(parts[4]); err != nil || len(h.salt) < minSaltLen {
		return hash{}, fmt.Errorf("%w: bad salt", ErrInvalidHash)
	}
	if h.key, err = base64.RawStdEncoding.Strict().DecodeString(parts[5]); err != nil || len(h.key) < minKeyLen || len(h.key) > maxKeyLen {
		return hash{}, fmt.Errorf("%w: bad hash", ErrInvalidHash)
	}
	return h, nil
}

// parseParams reads "m=<KiB>,t=<passes>,p=<lanes>", in that order.
func parseParams(s string) (memoryKiB, passes uint32, lanes uint8, err error) {
	fields := strings.Split(s, ",")
	if len(fields) != 3 {
		return 0, 0, 0, fmt.Errorf("%w: bad parameters", ErrInvalidHash)
	}
	var v [3]uint64
	for i, name := range []string{"m=", "t=", "p="} {
		num, ok := strings.CutPrefix(fields[i], name)
		if !ok {
			return 0, 0, 0, fmt.Errorf("%w: bad parameters", ErrInvalidHash)
		}
		if v[i], err = strconv.ParseUint(num, 10, 32); err != nil {
			return 0, 0, 0, fmt.Errorf("%w: bad parameters", ErrInvalidHash)
		}
	}
	m, t, p := v[0], v[1], v[2]
	// Argon2 needs at least 8 KiB of memory per lane.
	if p < 1 || p > 255 || t < 1 || t > maxTime || m < 8*p || m > maxMemoryKiB {
		return 0, 0, 0, fmt.Errorf("%w: parameters out of range", ErrInvalidHash)
	}
	return uint32(m), uint32(t), uint8(p), nil
}
