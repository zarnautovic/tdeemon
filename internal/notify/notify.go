// Package notify sends a user a notification in their Telegram chat:
// the morning summary, the watchdog's alarms.
package notify

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
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

// Send sends text to u's Telegram chat through bot, and reports whether
// u has one: a user without a chat gets nothing, without an error. A
// group that became a supergroup gets the text in the new chat, which
// becomes u's.
func Send(ctx context.Context, db *store.DB, bot *telegram.Bot, u user.User, text string, now time.Time) (bool, error) {
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
