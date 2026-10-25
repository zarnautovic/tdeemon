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
//   - /sazetak (or /sažetak, /summary) sends the morning summary as it
//     is now;
//   - /pomoc (or /help, /start), or any other message in a private
//     chat, lists the commands.
//
// The answer goes to the chat the message came in, for each user whose
// chat it is (telegram_chats), in their language. A chat that is no
// user's gets nothing; its id is logged, for the admin who sets it
// (tdeemon user set-telegram). A command older than commandAge gets
// nothing either, nor does, in a group, a message that is not a command,
// nor a service message anywhere. A command writes nothing; a group
// that became a supergroup moves its users to it.
func TelegramCommands(cfg Config, bot *telegram.Bot) func(context.Context, telegram.Update) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &sessions{db: cfg.DB, logger: cfg.Logger, now: cfg.Now, instance: cfg.Limits}
	return func(ctx context.Context, m telegram.Update) {
		now := s.now()
		switch {
		case m.MigrateTo != 0:
			s.followChat(ctx, m.ChatID, m.MigrateTo, now)
			return
		case m.MigrateFrom != 0:
			s.followChat(ctx, m.MigrateFrom, m.ChatID, now)
			return
		case strings.TrimSpace(m.Text) == "", m.ChatID < 0 && command(m.Text) == "":
			// A service message (someone joined, a pin), or a group's
			// talk that is not to the bot.
			return
		}
		users, err := s.db.TelegramChatUsers(ctx, m.ChatID)
		if err != nil {
			s.logger.Error("reading the users of a Telegram chat failed", "chat_id", m.ChatID, "error", err)
			return
		}
		if len(users) == 0 {
			s.logger.Info("a message to the Telegram bot from a chat that is no user's", "chat_id", m.ChatID)
			return
		}
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

// followChat moves the users of the group from to the supergroup it
// became, to, as notify.Send does when a send finds it out.
func (s *sessions) followChat(ctx context.Context, from, to int64, now time.Time) {
	users, err := s.db.TelegramChatUsers(ctx, from)
	if err != nil {
		s.logger.Error("reading the users of a Telegram chat failed", "chat_id", from, "error", err)
		return
	}
	for i := range users {
		if err := s.db.SetTelegramChat(ctx, users[i].ID, to, now); err != nil {
			s.logger.Error("following a Telegram group to its supergroup failed", "user_id", users[i].ID, "error", err)
		}
	}
}

// answer is the answer to u's message text at now: the summary to its
// command in any language, the commands to anything else.
func (s *sessions) answer(ctx context.Context, u user.User, text string, now time.Time) (string, error) {
	if isSummary(command(text)) {
		return s.morning(ctx, u, now)
	}
	return helpText(u.Locale), nil
}

// isSummary reports whether name is the summary's command in a language,
// or sažetak as typed, with the diacritic a command's name has not.
func isSummary(name string) bool {
	for _, w := range botWords {
		if w.summary.Name == name {
			return true
		}
	}
	return name == "sažetak"
}

// command returns the command text starts with, without its slash and
// the bot's name a group adds (/help@a_bot), in lower case; "" for a
// text that is not a command.
func command(text string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	name, ok := strings.CutPrefix(words[0], "/")
	if !ok {
		return ""
	}
	name, _, _ = strings.Cut(name, "@")
	return strings.ToLower(name)
}

// botLanguage is the bot's commands in a language: the summary's and the
// help's, as Telegram's menu lists them, and the help's text.
type botLanguage struct {
	summary, help telegram.Command
	helpText      string
}

// botWords are the bot's commands in each language a user can have
// (user.Locales), which a test holds to; English is the base.
var botWords = map[string]botLanguage{
	"en": {
		summary:  telegram.Command{Name: "summary", Description: "The morning summary, now"},
		help:     telegram.Command{Name: "help", Description: "The commands"},
		helpText: "Commands:\n/summary – the morning summary, now\n/help – these commands",
	},
	"hr": {
		summary:  telegram.Command{Name: "sazetak", Description: "Jutarnji sažetak, sada"},
		help:     telegram.Command{Name: "pomoc", Description: "Naredbe"},
		helpText: "Naredbe:\n/sazetak – jutarnji sažetak, sada\n/pomoc – ove naredbe",
	},
}

// botLanguageOf is the bot's commands in locale, English for one without
// them.
func botLanguageOf(locale string) botLanguage {
	if w, ok := botWords[locale]; ok {
		return w
	}
	return botWords["en"]
}

// BotCommands are the commands Telegram's menu lists for the bot, by the
// language of the app (telegram.Bot.SetCommands): each language's, and
// English for any other ("").
func BotCommands() map[string][]telegram.Command {
	out := map[string][]telegram.Command{}
	for locale, w := range botWords {
		if locale == "en" {
			locale = ""
		}
		out[locale] = []telegram.Command{w.summary, w.help}
	}
	return out
}

// helpText lists the commands, in the language of locale.
func helpText(locale string) string { return botLanguageOf(locale).helpText }
