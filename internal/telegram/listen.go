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
// ctx ends. A message being handled when ctx ends is handled to its end,
// within handleLimit, and the rest of its batch is left for a restart. A request that fails is tried again after a pause: as long
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
			if ctx.Err() != nil {
				break // the rest come again after a restart
			}
			offset = u.ID + 1
			if u.ChatID != 0 {
				b.handle(ctx, u, handle)
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

// handleLimit bounds the handling of a message, which ctx's end does
// not cut short: a shutdown waits for it, and then for the confirmation,
// within the 15 seconds the server's requests get.
const handleLimit = 10 * time.Second

// handle gives u to handle with a context of its own, within
// handleLimit, so that a message being answered when ctx ends is not
// confirmed unanswered.
func (b *Bot) handle(ctx context.Context, u Update, handle func(context.Context, Update)) {
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), handleLimit)
	defer cancel()
	handle(hctx, u)
}

// pauseAfter is the pause after err, the one before it being prev.
func pauseAfter(prev time.Duration, err error) time.Duration {
	if te, ok := errors.AsType[*Error](err); ok && te.RetryAfter > 0 {
		return te.RetryAfter
	}
	return min(max(2*prev, time.Second), time.Minute)
}
