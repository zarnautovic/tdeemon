package meal

import (
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/clientid"
)

// How near the clock a new meal's id must be (DESIGN.md §9), as for any
// id a client makes (clientid): an id already stored is a retry, which
// its fingerprint decides, however old.
const (
	MaxIDAge   = clientid.MaxAge
	MaxIDAhead = clientid.MaxAhead
)

// IDTime returns the time a version 7 UUID carries, to the millisecond,
// and false for any other id.
func IDTime(id uuid.UUID) (time.Time, bool) { return clientid.Time(id) }

// CheckNewID reports whether id can be the id of a new meal at now: a
// version 7 id made from MaxIDAge before now to MaxIDAhead after.
func CheckNewID(id uuid.UUID, now time.Time) error { return clientid.Check("meal", id, now) }

// NewIDAt returns a new version 7 id made at t, as a client logging a
// meal then would: for an import of meals logged long ago, and for tests
// on a clock of their own.
func NewIDAt(t time.Time) uuid.UUID { return clientid.NewAt(t) }

// MaxEatenAhead is how far after now a meal's time may be, a clock a
// little ahead of the server's, as for an id.
const MaxEatenAhead = clientid.MaxAhead

// CheckEatenBy reports a meal eaten at a time still to come at now, past
// MaxEatenAhead: a meal is logged once eaten, so a time to come is a
// mistake (an hour before the day's start is of the night after it).
func CheckEatenBy(eatenAt, now time.Time) error {
	if eatenAt.After(now.Add(MaxEatenAhead)) {
		return fmt.Errorf("eaten at %s, after now, %s: a meal is logged once eaten",
			eatenAt.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	}
	return nil
}
