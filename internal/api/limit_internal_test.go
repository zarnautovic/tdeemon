package api

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	l := newLimiter(3, time.Minute, func() time.Time { return now })
	var first reservation
	for i := range 3 {
		r, _, ok := l.take("a")
		if !ok {
			t.Fatalf("attempt %d refused, want three", i+1)
		}
		if i == 0 {
			first = r
		}
		now = now.Add(10 * time.Second)
	}
	if _, wait, ok := l.take("a"); ok || wait != 30*time.Second {
		t.Errorf("a fourth attempt: allowed %v, wait %v; want refused for the 30s left of the window", ok, wait)
	}
	if _, _, ok := l.take("b"); !ok {
		t.Error("another key is refused")
	}
	// An attempt given back makes room for one more.
	l.release(first)
	if _, _, ok := l.take("a"); !ok {
		t.Error("refused after an attempt was given back")
	}
	now = now.Add(30 * time.Second)
	if _, _, ok := l.take("a"); !ok {
		t.Error("still refused once the window has passed")
	}
	for range 3 {
		l.take("c")
	}
	l.reset("c")
	if _, _, ok := l.take("c"); !ok {
		t.Error("refused after a reset")
	}
}

// An attempt given back once its window has passed does not count against
// the next window; a key with no attempts left goes from the map.
func TestLimiterReleases(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	l := newLimiter(2, time.Minute, func() time.Time { return now })
	old, _, _ := l.take("a")
	now = now.Add(time.Minute) // a new window
	l.take("a")
	l.take("a")
	l.release(old)
	if _, _, ok := l.take("a"); ok {
		t.Error("an attempt from the window before made room in this one")
	}
	r, _, _ := l.take("b")
	l.release(r)
	if _, ok := l.keys["b"]; ok {
		t.Error("a key with no attempts left stayed in the map")
	}
}

// Past maxKeys, expired keys make room, and a key that still does not fit
// is not counted. Attempts given back take no room.
func TestLimiterBoundsItsKeys(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	l := newLimiter(1, time.Minute, func() time.Time { return now })
	l.maxKeys = 2
	for i := range 100 {
		r, _, _ := l.take(fmt.Sprint("given back ", i))
		l.release(r)
	}
	l.take("old")
	now = now.Add(time.Minute)
	l.take("a")
	l.take("b") // "old" expired: swept
	if _, _, ok := l.take("b"); ok {
		t.Error("b was not counted, though an expired key made room")
	}
	l.take("c") // full of live keys
	if _, _, ok := l.take("c"); !ok {
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
