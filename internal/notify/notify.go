// Package notify sends a user a notification, the morning summary or the
// watchdog's alarms: in their Telegram chat, or by Web Push to their
// devices.
package notify

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// Blocked reports whether err is Telegram's refusal of a chat that will
// not take a message: one that blocked the bot (403), or one that is not
// there (400, chat not found). Trying again does not help.
func Blocked(err error) bool {
	te, ok := errors.AsType[*telegram.Error](err)
	return ok && (te.Code == http.StatusForbidden ||
		te.Code == http.StatusBadRequest && strings.Contains(strings.ToLower(te.Description), "chat not found"))
}

// Channels are the ways a notification goes out: the Telegram bot, to a
// user's chat, and Web Push, to each of their devices; one that is nil is
// off.
type Channels struct {
	DB   *store.DB
	Bot  *telegram.Bot
	Push *push.Sender
	// Logger takes the failures of a channel when another delivered;
	// slog.Default() when nil.
	Logger *slog.Logger
}

// Note is a notification in each channel's form.
type Note struct {
	// Text is Telegram's message.
	Text string
	// Push is what a device shows, and Message how the push service
	// keeps it (its payload is Push's).
	Push    push.Notification
	Message push.Message
}

// Has reports whether u has a channel: a Telegram chat with the bot on,
// or a device subscribed with push on. A user without one need not be
// written a note.
func (c Channels) Has(ctx context.Context, u user.User) (bool, error) {
	if c.Bot != nil {
		if _, ok, err := c.DB.TelegramChat(ctx, u.ID); err != nil || ok {
			return ok, err
		}
	}
	if c.Push != nil {
		subs, err := c.DB.PushSubscriptions(ctx, u.ID)
		return len(subs) > 0, err
	}
	return false, nil
}

// Send sends n to each channel of u's, and reports whether u has one: a
// user without one gets nothing, without an error. It returns an error
// only when no channel delivered, so a retry sends again what none took;
// that error is Blocked only when Telegram's refusal is all that failed.
// A failure beside a delivery is logged.
func (c Channels) Send(ctx context.Context, u user.User, n Note, now time.Time) (bool, error) {
	var has, sent bool
	var errs []error
	var blocked error
	if c.Bot != nil {
		ok, err := sendTelegram(ctx, c.DB, c.Bot, u, n.Text, now)
		has = has || ok
		switch {
		case Blocked(err):
			blocked = err
		case err != nil:
			errs = append(errs, err)
		case ok:
			sent = true
		}
	}
	if c.Push != nil {
		r := Push(ctx, c.DB, c.Push, u, n.Push, n.Message, now)
		has = has || r.Sent+r.Failed > 0
		sent = sent || r.Sent > 0
		if r.Err != nil {
			errs = append(errs, r.Err)
		}
	}
	switch {
	case sent:
		if err := errors.Join(append(errs, blocked)...); err != nil {
			c.logger().WarnContext(ctx, "a notification's channel failed", "user_id", u.ID.String(), "error", err)
		}
		return true, nil
	case len(errs) > 0:
		return true, errors.Join(errs...)
	case blocked != nil:
		return true, blocked
	}
	return has, nil
}

func (c Channels) logger() *slog.Logger {
	if c.Logger == nil {
		return slog.Default()
	}
	return c.Logger
}

// sendTelegram sends text to u's Telegram chat through bot, and reports
// whether u has one: a user without a chat gets nothing, without an
// error. A group that became a supergroup gets the text in the new chat,
// which becomes u's.
func sendTelegram(ctx context.Context, db *store.DB, bot *telegram.Bot, u user.User, text string, now time.Time) (bool, error) {
	chat, ok, err := db.TelegramChat(ctx, u.ID)
	if err != nil || !ok {
		return false, err
	}
	err = bot.Send(ctx, chat, text)
	if te, moved := errors.AsType[*telegram.Error](err); moved && te.MigrateTo != 0 {
		if err := db.SetTelegramChat(ctx, u.ID, te.MigrateTo, now); err != nil {
			return true, err
		}
		err = bot.Send(ctx, te.MigrateTo, text)
	}
	return true, err
}
