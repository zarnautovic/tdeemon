package meal

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
	"uuid"
)

// How near the clock a new meal's id must be (DESIGN.md §9): a client
// makes it when the meal is logged, so an id from more than a day ago, or
// from ahead of the clock, is made up or reused. A day leaves an offline
// client its day without a network; an id already stored is a retry,
// which its fingerprint decides, however old.
const (
	MaxIDAge   = 24 * time.Hour
	MaxIDAhead = 5 * time.Minute
)

// IDTime returns the time a version 7 UUID carries, to the millisecond,
// and false for any other id.
func IDTime(id uuid.UUID) (time.Time, bool) {
	if id[6]>>4 != 7 || id[8]>>6 != 0b10 {
		return time.Time{}, false
	}
	var ms [8]byte
	copy(ms[2:], id[:6])
	return time.UnixMilli(int64(binary.BigEndian.Uint64(ms[:]))).UTC(), true //nolint:gosec // G115: 48 bits fit
}

// CheckNewID reports whether id can be the id of a new meal at now: a
// version 7 id made from MaxIDAge before now to MaxIDAhead after.
func CheckNewID(id uuid.UUID, now time.Time) error {
	at, ok := IDTime(id)
	switch {
	case !ok:
		return fmt.Errorf("the meal's id %s is not a version 7 UUID: make one when the meal is logged", id)
	case at.Before(now.Add(-MaxIDAge)):
		return fmt.Errorf("the meal's id %s was made at %s, more than a day before now: make a new one", id, at.Format(time.RFC3339))
	case at.After(now.Add(MaxIDAhead)):
		return fmt.Errorf("the meal's id %s was made at %s, ahead of the clock: make a new one", id, at.Format(time.RFC3339))
	}
	return nil
}

// NewIDAt returns a new version 7 id made at t, as a client logging a
// meal then would: for an import of meals logged long ago, and for tests
// on a clock of their own.
func NewIDAt(t time.Time) uuid.UUID {
	var id uuid.UUID
	_, _ = rand.Read(id[6:])
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(t.UnixMilli()))
	copy(id[:6], ms[2:])
	id[6] = 0x70 | id[6]&0x0f
	id[8] = 0x80 | id[8]&0x3f
	return id
}
