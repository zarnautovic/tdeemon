package auth

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCheckPassword(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, password string
		want           error
	}{
		{"empty", "", ErrPasswordTooShort},
		{"one short", strings.Repeat("a", MinPasswordLen-1), ErrPasswordTooShort},
		{"shortest", strings.Repeat("a", MinPasswordLen), nil},
		// 15 characters but 30 bytes: length counts characters.
		{"shortest in letters with diacritics", strings.Repeat("č", MinPasswordLen), nil},
		{"longest", strings.Repeat("a", MaxPasswordLen), nil},
		{"one long", strings.Repeat("a", MaxPasswordLen+1), ErrPasswordTooLong},
		// A decomposed "č" (c + combining caron) is one character after
		// normalization, so 14 of them are too short.
		{"decomposed letters count once", strings.Repeat("č", MinPasswordLen-1), ErrPasswordTooShort},
		{"spaces are characters", "correct horse battery staple", nil},
		{"invalid UTF-8", strings.Repeat("a", MinPasswordLen) + "\xff", ErrPasswordEncoding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := CheckPassword(tc.password); !errors.Is(err, tc.want) {
				t.Errorf("CheckPassword = %v, want %v", err, tc.want)
			}
		})
	}
}

var phc = regexp.MustCompile(`^\$argon2id\$v=19\$m=19456,t=2,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)

func TestHashAndVerify(t *testing.T) {
	t.Parallel()

	const password = "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !phc.MatchString(hash) {
		t.Errorf("hash %q is not an argon2id PHC string with the default parameters", hash)
	}
	if again, _ := HashPassword(password); again == hash {
		t.Error("two hashes of one password are equal; the salt is not random")
	}

	for _, tc := range []struct {
		name, password string
		want           bool
	}{
		{"same password", password, true},
		{"another password", "correct horse battery stapler", false},
		{"different case", strings.ToUpper(password), false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, err := VerifyPassword(tc.password, hash)
			if err != nil || ok != tc.want {
				t.Errorf("VerifyPassword = %v, %v; want %v, nil", ok, err, tc.want)
			}
		})
	}
}

func TestHashPasswordAppliesTheRules(t *testing.T) {
	t.Parallel()

	if _, err := HashPassword("short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("HashPassword(short) = %v, want ErrPasswordTooShort", err)
	}
}

// The same password typed as composed or decomposed characters, or with
// full-width digits, must match whichever form it was set in: both
// HashPassword and VerifyPassword normalize.
func TestNormalization(t *testing.T) {
	t.Parallel()

	forms := []struct{ name, password string }{
		{"composed", "čaša mlijeka 1234"},         // č is U+010D
		{"decomposed", "c\u030caša mlijeka 1234"}, // c + combining caron (U+030C)
		{"full-width digits", "čaša mlijeka \uff11\uff12\uff13\uff14"},
	}
	for _, set := range forms {
		t.Run("set "+set.name, func(t *testing.T) {
			t.Parallel()
			hash, err := HashPassword(set.password)
			if err != nil {
				t.Fatalf("HashPassword: %v", err)
			}
			for _, typed := range forms {
				if ok, err := VerifyPassword(typed.password, hash); err != nil || !ok {
					t.Errorf("verify %s = %v, %v; want true", typed.name, ok, err)
				}
			}
		})
	}
}

// The most a stored hash may ask for is accepted (and costly: it is not
// run here). Just past it, see TestVerifyRejectsInvalidHashes.
func TestParseHashLimits(t *testing.T) {
	t.Parallel()

	salt, key := strings.Repeat("A", 22), strings.Repeat("A", 1366) // 16 and 1024 bytes
	for _, params := range []string{"m=262144,t=10,p=1", "m=8,t=1,p=1", "m=2040,t=1,p=255"} {
		t.Run(params, func(t *testing.T) {
			t.Parallel()
			if _, err := parseHash("$argon2id$v=19$" + params + "$" + salt + "$" + key); err != nil {
				t.Errorf("parseHash: %v", err)
			}
		})
	}
}

// A hash written with other parameters still verifies: the parameters
// come from the hash, not from the current defaults.
func TestVerifyUsesTheStoredParameters(t *testing.T) {
	t.Parallel()

	// "correct horse battery staple" with m=8, t=1, p=1 and the salt
	// "0123456789abcdef", hashed by the reference C implementation through
	// argon2-cffi 25.1.0:
	//   argon2.low_level.hash_secret(password, salt, time_cost=1,
	//     memory_cost=8, parallelism=1, hash_len=32, type=Type.ID)
	const hash = "$argon2id$v=19$m=8,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$XdTJKPvVpEdCNu922dgwQXj+XfOITS04PpG3cDMn5sE"
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Errorf("VerifyPassword = %v, %v; want true", ok, err)
	}
}

func TestVerifyRejectsInvalidHashes(t *testing.T) {
	t.Parallel()

	good, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	parts := strings.Split(good, "$")
	with := func(i int, v string) string {
		p := append([]string(nil), parts...)
		p[i] = v
		return strings.Join(p, "$")
	}
	for _, tc := range []struct{ name, hash string }{
		{"empty", ""},
		{"bcrypt", "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"},
		{"argon2i", with(1, "argon2i")},
		{"old version", with(2, "v=16")},
		{"too few fields", strings.Join(parts[:5], "$")},
		{"text before", "x" + good},
		{"parameters out of order", with(3, "t=2,m=19456,p=1")},
		{"missing parameter", with(3, "m=19456,t=2")},
		{"no threads", with(3, "m=19456,t=2,p=0")},
		{"no passes", with(3, "m=0,t=0,p=1")},
		{"too little memory", with(3, "m=7,t=1,p=1")},
		{"too much memory", with(3, "m=262145,t=1,p=1")},
		{"too many passes", with(3, "m=19456,t=11,p=1")},
		{"not a number", with(3, "m=lots,t=2,p=1")},
		{"negative", with(3, "m=-1,t=2,p=1")},
		{"salt not base64", with(4, "!!!!")},
		{"padded salt", with(4, parts[4]+"==")},
		{"short salt", with(4, "c2FsdA")},
		{"hash not base64", with(5, "!!!!")},
		{"short hash", with(5, "c2hvcnQ")},
		{"long hash", with(5, strings.Repeat("A", 1367))}, // 1025 bytes
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, err := VerifyPassword("correct horse battery staple", tc.hash)
			if ok || !errors.Is(err, ErrInvalidHash) {
				t.Errorf("VerifyPassword = %v, %v; want false, ErrInvalidHash", ok, err)
			}
		})
	}
}

func TestGeneratePassword(t *testing.T) {
	t.Parallel()

	a, b := GeneratePassword(), GeneratePassword()
	if a == b {
		t.Error("two generated passwords are equal")
	}
	if !regexp.MustCompile(`^[A-Z2-7]{26}$`).MatchString(a) {
		t.Errorf("generated password %q is not 26 characters of A-Z and 2-7", a)
	}
	if err := CheckPassword(a); err != nil {
		t.Errorf("a generated password breaks the rules: %v", err)
	}
}

// A hash waits for a free slot: with every slot taken, a check (here the
// dummy one of an unknown address) does not run until one is released.
func TestHashingWaitsForASlot(t *testing.T) {
	t.Parallel()

	for range cap(hashSlots) {
		hashSlots <- struct{}{}
	}
	held := cap(hashSlots)
	t.Cleanup(func() {
		for range held {
			<-hashSlots
		}
	})
	done := make(chan struct{})
	go func() {
		_, _ = VerifyLogin("correct horse battery staple", "")
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("a hash ran with every slot taken")
	case <-time.After(500 * time.Millisecond):
	}
	<-hashSlots
	held--
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the hash did not run after a slot was released")
	}
}
