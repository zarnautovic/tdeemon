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
// due: a little less than a day, so that checks once an hour keep the
// snapshots at about the same time of day.
const DailyEvery = 23 * time.Hour

// DailySnapshot writes a snapshot of the database to the backups directory
// next to the database file, when the newest daily snapshot there is
// DailyEvery old or there is none, and returns its path; "" when none was
// due. It keeps the newest keep daily snapshots, and removes the older
// ones and the temporary files of one interrupted; the snapshots before a
// migration are not touched.
func (db *DB) DailySnapshot(ctx context.Context, now time.Time, keep int) (string, error) {
	if keep < 1 {
		return "", fmt.Errorf("daily snapshot: keep %d, want 1 or more", keep)
	}
	dir := db.backupsDir()
	snapshots, err := dailySnapshots(dir)
	if err != nil {
		return "", fmt.Errorf("daily snapshot: %w", err)
	}
	if len(snapshots) > 0 && now.Sub(snapshots[0].at) < DailyEvery {
		return "", nil
	}
	// The read pool is query only, which refuses VACUUM INTO; the copy
	// holds the write connection, so writes wait for it, a moment for a
	// database of one household.
	dst, err := snapshot(ctx, db.write, dir, dailyPrefix, dailyPrefix+now.UTC().Format(backupTimeFormat)+".db")
	if err != nil {
		return "", fmt.Errorf("daily snapshot: %w", err)
	}
	return dst, pruneDaily(dir, keep)
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

// pruneDaily removes the daily snapshots in dir past the newest keep, and
// the temporary files a killed snapshot left behind.
func pruneDaily(dir string, keep int) error {
	snapshots, err := dailySnapshots(dir)
	if err != nil {
		return err
	}
	var stale []string
	for _, s := range snapshots[min(keep, len(snapshots)):] {
		stale = append(stale, s.path)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if name := e.Name(); !e.IsDir() && strings.HasPrefix(name, dailyPrefix) && strings.HasSuffix(name, ".tmp") {
			stale = append(stale, filepath.Join(dir, name))
		}
	}
	var errs []error
	for _, f := range stale {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
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
	if dst != "" {
		logger.Info("took the daily snapshot", "path", dst)
	}
	if err != nil && ctx.Err() == nil {
		logger.Error("the daily snapshot failed", "error", err)
	}
}
