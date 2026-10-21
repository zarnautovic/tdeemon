package api

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	l := newLimiter(3, time.Minute, func() time.Time { return now })
	for range 2 {
		l.fail("a")
	}
	if _, blocked := l.blocked("a"); blocked {
		t.Error("blocked after two failures of three")
	}
	now = now.Add(20 * time.Second)
	l.fail("a")
	if wait, blocked := l.blocked("a"); !blocked || wait != 40*time.Second {
		t.Errorf("after three failures: blocked %v for %v, want 40s from the first", blocked, wait)
	}
	if _, blocked := l.blocked("b"); blocked {
		t.Error("another key is blocked")
	}
	now = now.Add(40 * time.Second)
	if _, blocked := l.blocked("a"); blocked {
		t.Error("still blocked once the window has passed")
	}
	l.fail("a")
	if _, blocked := l.blocked("a"); blocked {
		t.Error("a new window starts with one failure")
	}
	for range 3 {
		l.fail("c")
	}
	l.reset("c")
	if _, blocked := l.blocked("c"); blocked {
		t.Error("blocked after a reset")
	}
}

// Past maxKeys, expired keys make room, and a key that still does not fit
// is not counted.
func TestLimiterBoundsItsKeys(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	l := newLimiter(1, time.Minute, func() time.Time { return now })
	l.maxKeys = 2
	l.fail("old")
	now = now.Add(time.Minute)
	l.fail("a")
	l.fail("b") // "old" expired: swept
	if _, blocked := l.blocked("b"); !blocked {
		t.Error("b was not counted, though an expired key made room")
	}
	l.fail("c") // full of live keys
	if _, blocked := l.blocked("c"); blocked {
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
