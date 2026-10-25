package watchdog

import (
	"testing"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// Every language a user can have has the alarms' phrases, so a language
// added to user.Locales does not fall to English unseen.
func TestAlarmsSpeakEveryLocale(t *testing.T) {
	t.Parallel()

	for _, locale := range user.Locales() {
		if p, ok := words[locale]; !ok || p.noMetrics == "" || p.silentDevice == "" || p.days == nil {
			t.Errorf("the alarms have no phrases in %q", locale)
		}
	}
}
