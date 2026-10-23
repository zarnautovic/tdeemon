package telegram

import (
	"errors"
	"testing"
	"time"
)

// The pause after a failure is what Telegram asks for, or doubles from a
// second up to a minute.
func TestPauseAfter(t *testing.T) {
	t.Parallel()

	other := errors.New("boom")
	for _, tc := range []struct {
		prev time.Duration
		err  error
		want time.Duration
	}{
		{0, other, time.Second},
		{time.Second, other, 2 * time.Second},
		{40 * time.Second, other, time.Minute},
		{time.Minute, other, time.Minute},
		{0, &Error{Code: 429, RetryAfter: 7 * time.Second}, 7 * time.Second},
		{time.Minute, &Error{Code: 429, RetryAfter: 3 * time.Second}, 3 * time.Second},
		{0, &Error{Code: 409}, time.Second},
	} {
		if got := pauseAfter(tc.prev, tc.err); got != tc.want {
			t.Errorf("pauseAfter(%v, %v) = %v, want %v", tc.prev, tc.err, got, tc.want)
		}
	}
}
