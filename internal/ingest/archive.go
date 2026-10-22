package ingest

import (
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"uuid"
)

// Archive keeps the raw bodies devices send, compressed, for a while:
// to find what a sync sent when its values look wrong, and to read it
// again after a fix. It lives in the data directory, outside the
// database and its backups.
type Archive struct {
	// Dir holds a directory per user, and in it one per day received.
	Dir string
	// Keep is how long a body is kept; the days older go when the user's
	// next body is saved.
	Keep time.Duration
	// Now returns the current time; time.Now when nil.
	Now func() time.Time
}

// dayLayout names a day's directory.
const dayLayout = "2006-01-02"

// Save writes body, of a source such as "hae", to userID's archive, and
// removes the user's days older than Keep.
func (a *Archive) Save(userID uuid.UUID, source string, body []byte) error {
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	t := now().UTC()
	userDir := filepath.Join(a.Dir, userID.String())
	dir := filepath.Join(userDir, t.Format(dayLayout))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	name := filepath.Join(dir, t.Format("150405")+"-"+uuid.NewV7().String()+"-"+source+".json.gz")
	if err := writeGzip(name, body); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	return a.prune(userDir, t)
}

func writeGzip(name string, body []byte) (err error) {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: the name is made here, under the archive's directory
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	zw := gzip.NewWriter(f)
	if _, err := zw.Write(body); err != nil {
		return err
	}
	return zw.Close()
}

// prune removes the days of userDir older than Keep at t. A directory
// whose name is no day is left.
func (a *Archive) prune(userDir string, t time.Time) error {
	entries, err := os.ReadDir(userDir)
	if err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	oldest := t.Add(-a.Keep).Format(dayLayout)
	var errs []error
	for _, e := range entries {
		if _, err := time.Parse(dayLayout, e.Name()); err != nil || !e.IsDir() || e.Name() >= oldest {
			continue
		}
		if err := os.RemoveAll(filepath.Join(userDir, e.Name())); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	return nil
}
