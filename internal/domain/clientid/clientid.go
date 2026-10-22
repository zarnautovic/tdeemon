// Package clientid holds the rules for the ids a client makes for what it
// writes (a meal, an undo), so the same write sent again is known as a
// retry (DESIGN.md §9): a version 7 UUID, made near the clock.
package clientid

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
	"uuid"
)

// How near the clock a new id must be: a client makes it when it writes,
// so an id from more than a day ago, or from ahead of the clock, is made
// up or reused. A day leaves an offline client its day without a network;
// an id already stored is a retry, however old.
const (
	MaxAge   = 24 * time.Hour
	MaxAhead = 5 * time.Minute
)

// Time returns the time a version 7 UUID carries, to the millisecond,
// and false for any other id.
func Time(id uuid.UUID) (time.Time, bool) {
	if id[6]>>4 != 7 || id[8]>>6 != 0b10 {
		return time.Time{}, false
	}
	var ms [8]byte
	copy(ms[2:], id[:6])
	return time.UnixMilli(int64(binary.BigEndian.Uint64(ms[:]))).UTC(), true //nolint:gosec // G115: 48 bits fit
}

// Check reports whether id can be the id of a new what (a meal, an undo)
// at now: a version 7 id made from MaxAge before now to MaxAhead after.
func Check(what string, id uuid.UUID, now time.Time) error {
	at, ok := Time(id)
	switch {
	case !ok:
		return fmt.Errorf("the %s's id %s is not a version 7 UUID: make one for each new %s", what, id, what)
	case at.Before(now.Add(-MaxAge)):
		return fmt.Errorf("the %s's id %s was made at %s, more than a day before now: make a new one", what, id, at.Format(time.RFC3339))
	case at.After(now.Add(MaxAhead)):
		return fmt.Errorf("the %s's id %s was made at %s, ahead of the clock: make a new one", what, id, at.Format(time.RFC3339))
	}
	return nil
}

// NewAt returns a new version 7 id made at t, as a client writing then
// would: for an import of what was written long ago, and for tests on a
// clock of their own.
func NewAt(t time.Time) uuid.UUID {
	var id uuid.UUID
	_, _ = rand.Read(id[6:])
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(t.UnixMilli()))
	copy(id[:6], ms[2:])
	id[6] = 0x70 | id[6]&0x0f
	id[8] = 0x80 | id[8]&0x3f
	return id
}
