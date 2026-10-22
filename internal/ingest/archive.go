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

// Save writes body to userID's archive under name, the source and the
// format of body such as "hae.json" or "strong.csv", and removes the
// user's days older than Keep.
func (a *Archive) Save(userID uuid.UUID, name string, body []byte) error {
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
	file := filepath.Join(dir, t.Format("150405")+"-"+uuid.NewV7().String()+"-"+name+".gz")
	if err := writeGzip(file, body); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	return a.prune(userDir, t)
}

// SaveLatest keeps body as userID's latest of name, such as
// "strong.csv", outside the days that go after Keep: the last export
// that read, which the training module (P3) reads its sets from. It
// replaces the one before whole, or leaves it.
func (a *Archive) SaveLatest(userID uuid.UUID, name string, body []byte) error {
	userDir := filepath.Join(a.Dir, userID.String())
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	tmp := filepath.Join(userDir, "."+uuid.NewV7().String()+".tmp")
	if err := writeGzip(tmp, body); err != nil {
		return errors.Join(fmt.Errorf("archive: %w", err), os.Remove(tmp))
	}
	if err := os.Rename(tmp, filepath.Join(userDir, "latest-"+name+".gz")); err != nil {
		return errors.Join(fmt.Errorf("archive: %w", err), os.Remove(tmp))
	}
	return nil
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
