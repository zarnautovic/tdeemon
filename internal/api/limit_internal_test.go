package api

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	l := newLimiter(3, time.Minute, func() time.Time { return now })
	for i := range 3 {
		if _, ok := l.take("a"); !ok {
			t.Fatalf("attempt %d refused, want three", i+1)
		}
		now = now.Add(10 * time.Second)
	}
	if wait, ok := l.take("a"); ok || wait != 30*time.Second {
		t.Errorf("a fourth attempt: allowed %v, wait %v; want refused for the 30s left of the window", ok, wait)
	}
	if _, ok := l.take("b"); !ok {
		t.Error("another key is refused")
	}
	// An attempt given back makes room for one more.
	l.release("a")
	if _, ok := l.take("a"); !ok {
		t.Error("refused after an attempt was given back")
	}
	now = now.Add(30 * time.Second)
	if _, ok := l.take("a"); !ok {
		t.Error("still refused once the window has passed")
	}
	for range 3 {
		l.take("c")
	}
	l.reset("c")
	if _, ok := l.take("c"); !ok {
		t.Error("refused after a reset")
	}
}

// Past maxKeys, expired keys make room, and a key that still does not fit
// is not counted.
func TestLimiterBoundsItsKeys(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	l := newLimiter(1, time.Minute, func() time.Time { return now })
	l.maxKeys = 2
	l.take("old")
	now = now.Add(time.Minute)
	l.take("a")
	l.take("b") // "old" expired: swept
	if _, ok := l.take("b"); ok {
		t.Error("b was not counted, though an expired key made room")
	}
	l.take("c") // full of live keys
	if _, ok := l.take("c"); !ok {
		t.Error("c was counted past the bound")
	}
	if len(l.keys) > 2 {
		t.Errorf("%d keys, want at most 2", len(l.keys))
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()

	for wait, want := range map[time.Duration]string{time.Millisecond: "1", 90 * time.Second: "90", 1500 * time.Millisecond: "2"} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			if got := retryAfter(wait).Get("Retry-After"); got != want {
				t.Errorf("retryAfter(%v) = %q, want %q", wait, got, want)
			}
		})
	}
}
