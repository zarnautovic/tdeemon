package api

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// commandAge is how old a command may be and still be answered: one
// sent while the server restarted is, one from before it was down for
// long is not.
const commandAge = 5 * time.Minute

// TelegramCommands answers the commands a user sends the bot, the
// handle of telegram.Bot.Listen:
//
//   - /sazetak (or /summary) sends the morning summary as it is now;
//   - /pomoc (or /help, /start), or any other message, lists the
//     commands.
//
// The answer goes to the chat the message came in, for each user whose
// chat it is (telegram_chats), in their language. A chat that is no
// user's gets nothing; its id is logged, for the admin who sets it
// (tdeemon user set-telegram). A command older than commandAge gets
// nothing either. A command writes nothing.
func TelegramCommands(cfg Config, bot *telegram.Bot) func(context.Context, telegram.Update) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &sessions{db: cfg.DB, logger: cfg.Logger, now: cfg.Now, instance: cfg.Limits}
	return func(ctx context.Context, m telegram.Update) {
		users, err := s.db.TelegramChatUsers(ctx, m.ChatID)
		if err != nil {
			s.logger.Error("reading the users of a Telegram chat failed", "chat_id", m.ChatID, "error", err)
			return
		}
		if len(users) == 0 {
			s.logger.Info("a message to the Telegram bot from a chat that is no user's", "chat_id", m.ChatID)
			return
		}
		now := s.now()
		if now.Sub(m.Sent) > commandAge {
			s.logger.Info("a Telegram command too old to answer", "chat_id", m.ChatID, "sent", m.Sent)
			return
		}
		for i := range users {
			text, err := s.answer(ctx, users[i], m.Text, now)
			if err == nil {
				err = bot.Send(ctx, m.ChatID, text)
			}
			if err != nil {
				s.logger.Warn("answering a Telegram command failed", "user_id", users[i].ID, "chat_id", m.ChatID, "error", err)
			}
		}
	}
}

// answer is the answer to u's message text at now.
func (s *sessions) answer(ctx context.Context, u user.User, text string, now time.Time) (string, error) {
	switch command(text) {
	case "sazetak", "summary":
		return s.morning(ctx, u, now)
	}
	return helpText(u.Locale), nil
}

// command returns the command text starts with, without its slash and
// the bot's name a group adds (/help@a_bot), in lower case; "" for a
// text that is not a command.
func command(text string) string {
	word, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	name, ok := strings.CutPrefix(word, "/")
	if !ok {
		return ""
	}
	name, _, _ = strings.Cut(name, "@")
	return strings.ToLower(name)
}

// BotCommands are the commands Telegram's menu lists for the bot, by the
// language of the app (telegram.Bot.SetCommands): Croatian for hr, and
// English for any other.
func BotCommands() map[string][]telegram.Command {
	return map[string][]telegram.Command{
		"":   {{Name: "summary", Description: "The morning summary, now"}, {Name: "help", Description: "The commands"}},
		"hr": {{Name: "sazetak", Description: "Jutarnji sažetak, sada"}, {Name: "pomoc", Description: "Naredbe"}},
	}
}

// helpText lists the commands, in the language of locale.
func helpText(locale string) string {
	if locale == "hr" {
		return "Naredbe:\n/sazetak – jutarnji sažetak, sada\n/pomoc – ove naredbe"
	}
	return "Commands:\n/summary – the morning summary, now\n/help – these commands"
}
