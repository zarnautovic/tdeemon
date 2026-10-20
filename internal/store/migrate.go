package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations
var embedded embed.FS

// ErrSchemaTooNew reports a database that a newer tdeemon has already
// migrated past the migrations this binary knows.
var ErrSchemaTooNew = errors.New("database schema is newer than this binary")

// migrationFile matches NNNN_snake_case_name.sql.
var migrationFile = regexp.MustCompile(`^(\d{4})_([a-z0-9]+(?:_[a-z0-9]+)*)\.sql$`)

// maxReportedViolations caps how many foreign key violations an error
// lists, so a broken data migration does not produce a megabyte message.
const maxReportedViolations = 5

// keepPreMigrateBackups is how many pre-migration snapshots are kept. It
// bounds the disk a crash loop can fill with copies of the database.
const keepPreMigrateBackups = 3

const preMigratePrefix = "pre-migrate-"

const createMigrationsTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	name       TEXT    NOT NULL,
	checksum   TEXT    NOT NULL,
	applied_at TEXT    NOT NULL
) STRICT`

type migration struct {
	version  int
	name     string
	sql      string
	checksum string // hex SHA-256 of the file
}

// Migrate applies every embedded migration the database has not seen
// yet. See migrations/README.md for the rules migrations follow.
//
// Before it changes a database that already has a schema, Migrate
// writes a snapshot to the backups directory next to the database file.
// It refuses to touch a database migrated by a newer binary
// (ErrSchemaTooNew). Afterwards it refreshes the query planner's
// statistics with PRAGMA optimize, which SQLite recommends when a
// long-lived connection opens and after schema changes.
func (db *DB) Migrate(ctx context.Context, logger *slog.Logger) error {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return db.migrate(ctx, sub, logger)
}

func (db *DB) migrate(ctx context.Context, fsys fs.FS, logger *slog.Logger) error {
	known, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Pin the write connection, so nothing else writes while migrations
	// run, and discard it afterwards instead of returning it to the pool:
	// foreign keys are switched off below, and a migration may change
	// other per-connection state (pragmas, TEMP objects). The pool opens
	// a fresh connection with the standard settings for the next writer.
	conn, err := db.write.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer func() {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = conn.Close()
	}()

	if _, err := conn.ExecContext(ctx, createMigrationsTable); err != nil {
		return fmt.Errorf("migrate: create schema_migrations: %w", err)
	}
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	pending, err := pendingMigrations(known, applied, logger)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if len(pending) == 0 {
		logger.Info("database schema is up to date", "version", len(applied))
	} else {
		if err := db.applyPending(ctx, conn, applied, pending, logger); err != nil {
			return err
		}
		logger.Info("database schema migrated", "from", len(applied), "to", len(known))
	}

	// analysis_limit keeps ANALYZE on a large table quick, as the SQLite
	// documentation advises for PRAGMA optimize. Both settings die with
	// the discarded connection. The statistics live in the file, so the
	// read pool benefits too. A failure here only costs query plans.
	if _, err := conn.ExecContext(ctx, "PRAGMA analysis_limit = 400; PRAGMA optimize = 0x10002"); err != nil {
		logger.Warn("refreshing query planner statistics failed", "error", err)
	}
	return nil
}

func (db *DB) applyPending(ctx context.Context, conn *sql.Conn, applied, pending []migration, logger *slog.Logger) error {
	if len(applied) > 0 {
		dst, err := db.backup(ctx, conn, len(applied), logger)
		if err != nil {
			return fmt.Errorf("migrate: back up before migrating: %w", err)
		}
		logger.Info("backed up the database before migrating", "path", dst)
	}

	// SQLite cannot alter most column definitions in place; a migration
	// rebuilds the table instead (create, copy, drop, rename). With
	// foreign keys enforced, dropping a referenced table fails or
	// cascades, so they are off while migrations run and every migration
	// must pass the foreign key checks before it commits. The pragma is a
	// no-op inside a transaction, hence here. The connection is discarded
	// afterwards, so it is never switched back on.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("migrate: disable foreign keys: %w", err)
	}

	for _, m := range pending {
		start := time.Now()
		if err := applyMigration(ctx, conn, m); err != nil {
			return fmt.Errorf("migrate: %04d_%s: %w", m.version, m.name, err)
		}
		logger.Info("applied migration", "version", m.version, "name", m.name, "duration", time.Since(start))
	}
	return nil
}

// loadMigrations reads the migration files in fsys and checks that their
// versions run 1, 2, 3, ... with no gaps or duplicates and that none of
// them ends the runner's transaction. Files that do not end in .sql, in
// any case, (such as README.md) are ignored.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(path.Ext(e.Name()), ".sql") {
			continue
		}
		parts := migrationFile.FindStringSubmatch(e.Name())
		if parts == nil {
			return nil, fmt.Errorf("migration file %q: want NNNN_snake_case_name.sql", e.Name())
		}
		version, _ := strconv.Atoi(parts[1]) // The pattern allows exactly four digits.
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		if stmt, line, found := transactionControl(string(data)); found {
			return nil, fmt.Errorf("migration file %q, line %d: %s ends the transaction the runner wraps every migration in; remove it",
				e.Name(), line, stmt)
		}
		sum := sha256.Sum256(data)
		out = append(out, migration{
			version:  version,
			name:     parts[2],
			sql:      string(data),
			checksum: hex.EncodeToString(sum[:]),
		})
	}

	// fs.ReadDir sorts by file name, so versions come in ascending order.
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration file %04d_%s.sql: want version %04d; versions start at 0001 with no gaps or duplicates",
				m.version, m.name, i+1)
		}
	}
	return out, nil
}

func appliedMigrations(ctx context.Context, conn *sql.Conn) ([]migration, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version, name, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []migration
	for rows.Next() {
		var m migration
		if err := rows.Scan(&m.version, &m.name, &m.checksum); err != nil {
			return nil, fmt.Errorf("read schema_migrations: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	return out, nil
}

// pendingMigrations returns the known migrations that have not been
// applied. Applied migrations must be exactly the first known ones.
func pendingMigrations(known, applied []migration, logger *slog.Logger) ([]migration, error) {
	if len(applied) > len(known) {
		return nil, fmt.Errorf("%w: the database is at version %04d, but this binary knows migrations up to %04d; run a newer tdeemon or restore a backup",
			ErrSchemaTooNew, applied[len(applied)-1].version, len(known))
	}
	for i, a := range applied {
		if a.version != i+1 {
			return nil, fmt.Errorf("schema_migrations is inconsistent: found version %04d where %04d was expected", a.version, i+1)
		}
		if k := known[i]; a.name != k.name || a.checksum != k.checksum {
			// Not fatal: a comment or whitespace fix is harmless, and
			// refusing to start would lock every instance out. A real
			// change has to come as a new migration, which the review
			// rules in migrations/README.md ask for.
			logger.Warn("an applied migration has changed since it ran; the change will not be applied",
				"version", a.version, "applied_name", a.name, "file_name", k.name)
		}
	}
	return known[len(applied):], nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, m migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if err := checkForeignKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)",
		m.version, m.name, m.checksum, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return tx.Commit()
}

// checkForeignKeys fails if a foreign key names a table that does not
// exist or if any row references a missing parent.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	if err := checkForeignKeyTargets(ctx, tx); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var found []string
	total := 0
	for rows.Next() {
		var (
			table, parent string
			rowid         sql.NullInt64
			fkid          int
		)
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("foreign key check: %w", err)
		}
		total++
		if len(found) < maxReportedViolations {
			found = append(found, fmt.Sprintf("%s rowid %d references a missing %s row", table, rowid.Int64, parent))
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	if total > 0 {
		return fmt.Errorf("leaves %d foreign key violation(s): %s", total, strings.Join(found, "; "))
	}
	return nil
}

// checkForeignKeyTargets fails if a foreign key names a table that does
// not exist. foreign_key_check misses that on empty tables, and it is
// what a table rebuild that starts by renaming the old table leaves
// behind: SQLite rewrites the references to the renamed table, which is
// then dropped, and every later insert fails.
func checkForeignKeyTargets(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT m.name, f."table"
		FROM sqlite_schema AS m
		JOIN pragma_foreign_key_list(m.name) AS f
		WHERE m.type = 'table'
		  AND NOT EXISTS (
			SELECT 1 FROM sqlite_schema AS p
			WHERE p.type = 'table' AND p.name = f."table" COLLATE NOCASE)
		ORDER BY m.name, f."table"`)
	if err != nil {
		return fmt.Errorf("foreign key targets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var missing []string
	for rows.Next() {
		var table, target string
		if err := rows.Scan(&table, &target); err != nil {
			return fmt.Errorf("foreign key targets: %w", err)
		}
		missing = append(missing, fmt.Sprintf("%s references missing table %s", table, target))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("foreign key targets: %w", err)
	}
	if len(missing) > 0 {
		return fmt.Errorf("leaves foreign keys to tables that do not exist: %s", strings.Join(missing, "; "))
	}
	return nil
}

// backup writes a consistent snapshot of the database at schema version
// to the backups directory next to the database file and returns its
// path. The snapshot is written under a temporary name and renamed when
// complete, so an interrupted backup never looks like a finished one.
// Only the newest keepPreMigrateBackups snapshots are kept.
func (db *DB) backup(ctx context.Context, conn *sql.Conn, version int, logger *slog.Logger) (string, error) {
	dir := filepath.Join(filepath.Dir(db.path), "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}

	// VACUUM INTO accepts an existing file only if it is empty, which is
	// what CreateTemp leaves; its random name never collides.
	tmp, err := os.CreateTemp(dir, preMigratePrefix+"*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", errors.Join(err, os.Remove(tmpPath))
	}
	done := false
	defer func() {
		if !done {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", tmpPath); err != nil {
		return "", err
	}
	if err := syncPath(tmpPath); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, fmt.Sprintf("%s%04d-%s.db", preMigratePrefix, version,
		time.Now().UTC().Format("20060102T150405.000000Z")))
	if err := os.Rename(tmpPath, dst); err != nil {
		return "", err
	}
	done = true
	if err := syncPath(dir); err != nil {
		return "", err
	}

	if err := pruneBackups(dir); err != nil {
		logger.Warn("removing old pre-migration snapshots failed", "error", err)
	}
	return dst, nil
}

// pruneBackups keeps the newest keepPreMigrateBackups snapshots and
// removes temporary files a killed process left behind. Other files in
// the directory are not touched.
func pruneBackups(dir string) error {
	snapshots, err := filepath.Glob(filepath.Join(dir, preMigratePrefix+"*.db"))
	if err != nil {
		return err
	}
	// Names hold the version and then the time, so they sort oldest first.
	slices.Sort(snapshots)
	stale := snapshots[:max(0, len(snapshots)-keepPreMigrateBackups)]

	leftovers, err := filepath.Glob(filepath.Join(dir, preMigratePrefix+"*.tmp"))
	if err != nil {
		return err
	}
	var errs []error
	for _, f := range slices.Concat(stale, leftovers) {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// syncPath flushes a file or directory to disk.
func syncPath(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
