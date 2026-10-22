package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

const dailyPrefix = "daily-"

// dailyFile matches a finished daily snapshot and captures its time.
var dailyFile = regexp.MustCompile(`^daily-(\d{8}T\d{6}\.\d{6}Z)\.db$`)

// DailyEvery is how old the newest daily snapshot is when the next one is
// due: half an hour less than a day, so that checks once an hour find it
// due at the same hour each day, with room for a check a little early.
const DailyEvery = 23*time.Hour + 30*time.Minute

// DailySnapshot writes a snapshot of the database to the backups directory
// next to the database file, when there is none younger than DailyEvery
// (one of a time to come is none), and returns its path; "" when none
// was due. Before it writes, it removes the temporary files of one
// interrupted, so they never fill the room the new one needs; after, it
// keeps the newest keep daily snapshots up to now and removes the older
// ones. Those of a time to come stay, uncounted: a clock that was ahead
// named them, or one behind now, after a restart without the time, would
// take every real one for them. The snapshots before a migration are not
// touched. With a path, an error is about removing the old files.
func (db *DB) DailySnapshot(ctx context.Context, now time.Time, keep int) (string, error) {
	if keep < 1 {
		return "", fmt.Errorf("daily snapshot: keep %d, want 1 or more", keep)
	}
	dir := db.backupsDir()
	snapshots, err := dailySnapshots(dir)
	if err != nil {
		return "", fmt.Errorf("daily snapshot: %w", err)
	}
	// One of a time to come does not count, or a clock that was ahead
	// would stop the snapshots until it is reached.
	for _, s := range snapshots {
		if age := now.Sub(s.at); age >= 0 && age < DailyEvery {
			return "", nil
		}
	}
	// One that does not go is reported, but stops no snapshot.
	tempErr := removeDailyTemps(dir)
	// The read pool is query only, which refuses VACUUM INTO; the copy
	// holds the write connection, so writes wait for it, a moment for a
	// database of one household.
	dst, err := snapshot(ctx, db.write, dir, dailyPrefix, dailyPrefix+now.UTC().Format(backupTimeFormat)+".db")
	if err != nil {
		return "", fmt.Errorf("daily snapshot: %w", errors.Join(err, tempErr))
	}
	if err := errors.Join(tempErr, pruneDaily(snapshots, dst, now, keep)); err != nil {
		return dst, fmt.Errorf("removing old daily snapshots: %w", err)
	}
	return dst, nil
}

type dailySnap struct {
	path string
	at   time.Time
}

// dailySnapshots returns the finished daily snapshots in dir, the newest
// first; none when dir does not exist yet.
func dailySnapshots(dir string) ([]dailySnap, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []dailySnap
	for _, e := range entries {
		m := dailyFile.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		at, err := time.Parse(backupTimeFormat, m[1])
		if err != nil {
			continue
		}
		out = append(out, dailySnap{path: filepath.Join(dir, e.Name()), at: at})
	}
	slices.SortFunc(out, func(a, b dailySnap) int { return b.at.Compare(a.at) })
	return out, nil
}

// pruneDaily removes, of the daily snapshots there were before written
// was, those past the newest keep-1 up to now. Those of a time to come
// stay, and do not count.
func pruneDaily(before []dailySnap, written string, now time.Time, keep int) error {
	var errs []error
	kept := 1 // written
	for _, s := range before {
		if s.path == written || s.at.After(now) {
			continue
		}
		if kept >= keep {
			if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		kept++
	}
	return errors.Join(errs...)
}

// removeDailyTemps removes the temporary files a killed daily snapshot
// left in dir; only one snapshot runs at a time, so none is under way.
func removeDailyTemps(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if name := e.Name(); strings.HasPrefix(name, dailyPrefix) && strings.HasSuffix(name, ".tmp") {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// StartDailySnapshots takes a daily snapshot when one is due
// (DailySnapshot): at once, before it returns, and then every interval in
// the background until ctx is done. A snapshot that fails is logged, and
// the next check tries again. The returned wait blocks until the
// background checks have stopped; cancel ctx first.
func (db *DB) StartDailySnapshots(ctx context.Context, keep int, interval time.Duration, logger *slog.Logger) (wait func()) {
	db.dailySnapshot(ctx, keep, logger)
	var running sync.WaitGroup
	running.Go(func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				db.dailySnapshot(ctx, keep, logger)
			}
		}
	})
	return running.Wait
}

func (db *DB) dailySnapshot(ctx context.Context, keep int, logger *slog.Logger) {
	dst, err := db.DailySnapshot(ctx, time.Now(), keep)
	switch {
	case dst != "":
		logger.Info("took the daily snapshot", "path", dst)
		if err != nil {
			logger.Warn("removing old daily snapshots failed", "error", err)
		}
	case err != nil && ctx.Err() == nil:
		logger.Error("the daily snapshot failed", "error", err)
	}
}
