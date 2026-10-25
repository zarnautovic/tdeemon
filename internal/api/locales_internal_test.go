package api

import (
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// Every language a user can have has the morning summary's phrases and
// the bot's commands, so a language added to user.Locales does not fall
// to English in Telegram unseen.
func TestTelegramSpeaksEveryLocale(t *testing.T) {
	t.Parallel()

	for _, locale := range user.Locales() {
		if _, ok := morningWords[locale]; !ok {
			t.Errorf("the morning summary has no phrases in %q", locale)
		}
		if w, ok := botWords[locale]; !ok || w.summary.Name == "" || w.help.Name == "" || w.helpText == "" {
			t.Errorf("the bot has no commands in %q", locale)
		}
	}
}
