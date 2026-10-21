package api

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Limits on failures, counted in memory per client address (addrKey) and
// per account (the normalized address signed in with, known or not, so
// the limit tells nothing about which accounts exist).
const (
	// signInFailures failed sign-ins in signInWindow, per address and per
	// account, and further sign-ins answer 429 until the window passes. A
	// correct password then waits too: the limit is not a hint.
	signInFailures = 10
	signInWindow   = 15 * time.Minute
	// badTokenFailures requests with a Bearer token that is no live API
	// token in badTokenWindow, per address, and further Bearer requests
	// from there answer 429 until the window passes. A device with a
	// revoked token that retries every minute stays under it.
	badTokenFailures = 50
	badTokenWindow   = 15 * time.Minute
)

// maxLimitKeys bounds the memory of a limiter. Past it, expired keys are
// swept, and a key that still does not fit is not counted.
const maxLimitKeys = 100_000

// limiter counts failures per key. A key that reaches max failures is
// blocked until window has passed since its first failure; then it
// starts over.
type limiter struct {
	max     int
	window  time.Duration
	now     func() time.Time
	maxKeys int // maxLimitKeys; smaller in tests

	mu   sync.Mutex
	keys map[string]*failures
}

type failures struct {
	n     int
	since time.Time
}

func newLimiter(maxFailures int, window time.Duration, now func() time.Time) *limiter {
	return &limiter{max: maxFailures, window: window, now: now, maxKeys: maxLimitKeys, keys: map[string]*failures{}}
}

// blocked reports whether key is blocked now, and for how much longer.
func (l *limiter) blocked(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.keys[key]
	if !ok {
		return 0, false
	}
	now, end := l.now(), f.since.Add(l.window)
	if !now.Before(end) {
		delete(l.keys, key)
		return 0, false
	}
	return end.Sub(now), f.n >= l.max
}

// fail counts a failure for key.
func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if f, ok := l.keys[key]; ok && now.Before(f.since.Add(l.window)) {
		f.n++
		return
	}
	if len(l.keys) >= l.maxKeys {
		for k, f := range l.keys {
			if !now.Before(f.since.Add(l.window)) {
				delete(l.keys, k)
			}
		}
		if len(l.keys) >= l.maxKeys {
			return
		}
	}
	l.keys[key] = &failures{n: 1, since: now}
}

// reset forgets key's failures.
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.keys, key)
}

// retryAfter is the Retry-After header for a wait: whole seconds, rounded
// up, at least one.
func retryAfter(wait time.Duration) http.Header {
	secs := max(1, int(math.Ceil(wait.Seconds())))
	return http.Header{"Retry-After": {strconv.Itoa(secs)}}
}
