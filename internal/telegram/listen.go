package telegram

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Wait is how long a request for updates waits for one to come, within
// Telegram's longest.
const Wait = 50 * time.Second

// Listen gives handle the bot's messages, in order and once each, until
// ctx ends. A request that fails is tried again after a pause: as long
// as Telegram asks, or from a second, doubling up to a minute. When ctx
// ends, it confirms the updates it handled, so that a restart does not
// handle them again; an update handled just before a crash may come
// again.
func (b *Bot) Listen(ctx context.Context, logger *slog.Logger, handle func(context.Context, Update)) {
	var offset int64
	var pause time.Duration
	for ctx.Err() == nil {
		updates, err := b.Updates(ctx, offset, Wait)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			pause = pauseAfter(pause, err)
			logger.Warn("reading the Telegram bot's messages failed", "error", err, "retry_in", pause)
			select {
			case <-ctx.Done():
			case <-time.After(pause):
			}
			continue
		}
		pause = 0
		for _, u := range updates {
			offset = u.ID + 1
			if u.ChatID != 0 {
				handle(ctx, u)
			}
		}
	}
	if offset != 0 {
		// ctx has ended: the confirmation has a short while of its own.
		confirm, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := b.Updates(confirm, offset, 0); err != nil {
			logger.Warn("confirming the Telegram bot's messages failed", "error", err)
		}
	}
}

// pauseAfter is the pause after err, the one before it being prev.
func pauseAfter(prev time.Duration, err error) time.Duration {
	if te, ok := errors.AsType[*Error](err); ok && te.RetryAfter > 0 {
		return te.RetryAfter
	}
	return min(max(2*prev, time.Second), time.Minute)
}
