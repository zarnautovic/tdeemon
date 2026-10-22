package clientid_test

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
)

// A new id is a version 7 id near the clock, and a refusal names what
// the id is of.
func TestCheck(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	for name, id := range map[string]uuid.UUID{
		"made now":         clientid.NewAt(now),
		"a day before":     clientid.NewAt(now.Add(-clientid.MaxAge)),
		"a little ahead":   clientid.NewAt(now.Add(clientid.MaxAhead)),
		"made a while ago": clientid.NewAt(now.Add(-time.Hour)),
	} {
		if err := clientid.Check("undo", id, now); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, tc := range map[string]struct {
		id   uuid.UUID
		want string
	}{
		"version 4":          {uuid.NewV4(), "the undo's id"},
		"older than a day":   {clientid.NewAt(now.Add(-clientid.MaxAge - time.Second)), "more than a day before"},
		"ahead of the clock": {clientid.NewAt(now.Add(clientid.MaxAhead + time.Second)), "ahead of the clock"},
	} {
		if err := clientid.Check("undo", tc.id, now); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	if at, ok := clientid.Time(clientid.NewAt(now)); !ok || !at.Equal(now) {
		t.Errorf("Time = %v, %v; want %v", at, ok, now)
	}
}
