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
// blocked until window has passed since its first attempt; then it
// starts over.
type limiter struct {
	max     int
	window  time.Duration
	now     func() time.Time
	maxKeys int // maxLimitKeys; smaller in tests

	mu   sync.Mutex
	keys map[string]*failures
	// oldest is the earliest start of a window in keys, or before it: no
	// key can expire before oldest+window, so a full map is swept only
	// from then on, and at most every window/maxSweeps (nextSweep): keys
	// expire one by one, and a full map would be swept for every new one.
	oldest, nextSweep time.Time
	sweeps            int // how many sweeps ran, for tests
}

// maxSweeps bounds how often a full limiter is swept in one window. A
// key that could take an expired one's place waits for the next sweep,
// a window/maxSweeps at most (14 seconds of 15 minutes).
const maxSweeps = 64

type failures struct {
	n     int
	since time.Time
}

// reservation is an attempt take reserved: its key, and the window it
// counts in, so giving it back once that window has passed changes
// nothing.
type reservation struct {
	key   string
	since time.Time
}

func newLimiter(maxFailures int, window time.Duration, now func() time.Time) *limiter {
	return &limiter{max: maxFailures, window: window, now: now, maxKeys: maxLimitKeys, keys: map[string]*failures{}}
}

// take reserves an attempt for key. It counts as a failure until release
// gives it back, so attempts still in flight count against the limit as
// much as those that failed: a burst sent at once cannot slip past it. It
// reports false, and how long to wait, when key is blocked.
func (l *limiter) take(key string) (reservation, time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if f, ok := l.keys[key]; ok && now.Before(f.since.Add(l.window)) {
		if f.n >= l.max {
			return reservation{}, f.since.Add(l.window).Sub(now), false
		}
		f.n++
		return reservation{key, f.since}, 0, true
	}
	delete(l.keys, key)
	if len(l.keys) >= l.maxKeys && !now.Before(l.oldest.Add(l.window)) && !now.Before(l.nextSweep) {
		l.sweep(now)
	}
	if len(l.keys) >= l.maxKeys {
		return reservation{}, 0, true // Not counted: see maxLimitKeys.
	}
	if len(l.keys) == 0 || now.Before(l.oldest) {
		l.oldest = now
	}
	l.keys[key] = &failures{n: 1, since: now}
	return reservation{key, now}, 0, true
}

// sweep removes the keys whose window has passed and finds the oldest of
// the others.
func (l *limiter) sweep(now time.Time) {
	l.sweeps++
	l.nextSweep = now.Add(l.window / maxSweeps)
	l.oldest = now
	for k, f := range l.keys {
		if !now.Before(f.since.Add(l.window)) {
			delete(l.keys, k)
		} else if f.since.Before(l.oldest) {
			l.oldest = f.since
		}
	}
}

// release gives back an attempt take reserved that did not fail. A key
// with no attempts left goes, so attempts given back fill no map.
func (l *limiter) release(r reservation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.keys[r.key]
	if !ok || !f.since.Equal(r.since) || f.n == 0 {
		return
	}
	f.n--
	if f.n == 0 {
		delete(l.keys, r.key)
	}
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
