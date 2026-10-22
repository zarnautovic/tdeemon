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

// ErrSchemaOutdated reports a database that this binary's migrations
// have not brought up to date yet.
var ErrSchemaOutdated = errors.New("database schema is older than this binary")

// migrationFile matches NNNN_snake_case_name.sql.
var migrationFile = regexp.MustCompile(`^(\d{4})_([a-z0-9]+(?:_[a-z0-9]+)*)\.sql$`)

// maxReportedViolations caps how many foreign key violations an error
// lists, so a broken data migration does not produce a megabyte message.
const maxReportedViolations = 5

// keepPreMigrateBackups is how many pre-migration snapshots are kept. It
// bounds the disk a crash loop can fill with copies of the database.
const keepPreMigrateBackups = 3

const preMigratePrefix = "pre-migrate-"

// backupTimeFormat is the time in a snapshot's name. It sorts lexically.
const backupTimeFormat = "20060102T150405.000000Z"

// preMigrateFile matches a finished snapshot and captures its schema
// version and its time.
var preMigrateFile = regexp.MustCompile(`^pre-migrate-(\d{4})-(\d{8}T\d{6}\.\d{6}Z)\.db$`)

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
//
// Call it at startup, before anything else writes: every migration runs
// on a connection of its own, so another writer could slip in between.
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

	var applied, pending []migration
	err = db.withSingleUseConn(ctx, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, createMigrationsTable); err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}
		if applied, err = appliedMigrations(ctx, conn); err != nil {
			return err
		}
		if pending, err = pendingMigrations(known, applied, logger); err != nil {
			return err
		}
		if len(pending) == 0 || len(applied) == 0 {
			return nil
		}
		dst, err := db.backup(ctx, conn, len(applied), logger)
		if err != nil {
			return fmt.Errorf("back up before migrating: %w", err)
		}
		logger.Info("backed up the database before migrating", "path", dst)
		return nil
	})
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	if len(pending) == 0 {
		logger.Info("database schema is up to date", "version", len(applied))
	} else {
		for _, m := range pending {
			start := time.Now()
			// A fresh connection per migration: a fresh install runs every
			// migration in one go, an upgrade only the new ones, and both
			// must see each migration start from the same connection state.
			err := db.withSingleUseConn(ctx, func(conn *sql.Conn) error {
				return applyMigration(ctx, conn, m)
			})
			if err != nil {
				return fmt.Errorf("migrate: %04d_%s: %w", m.version, m.name, err)
			}
			logger.Info("applied migration", "version", m.version, "name", m.name, "duration", time.Since(start))
		}
		logger.Info("database schema migrated", "from", len(applied), "to", len(known))
	}

	// analysis_limit keeps ANALYZE on a large table quick, as the SQLite
	// documentation advises for PRAGMA optimize. The statistics live in the
	// file, so the read pool benefits too. A failure only costs query plans.
	err = db.withSingleUseConn(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, "PRAGMA analysis_limit = 400; PRAGMA optimize = 0x10002")
		return err
	})
	if err != nil {
		logger.Warn("refreshing query planner statistics failed", "error", err)
	}
	return nil
}

// RequireCurrentSchema returns nil when the database has exactly the
// migrations this binary knows applied, and ErrSchemaOutdated or
// ErrSchemaTooNew otherwise. Commands that run next to a live server use
// it instead of Migrate: the runner assumes one process migrates at a
// time, and that process is the server.
//
// After it succeeds, every WriteTx on db checks inside its transaction
// that the schema is still at that version, so a newer server that
// migrates between the check and a write makes the write fail instead of
// writing into a schema this binary does not know. Call it before any
// write and from one goroutine.
func (db *DB) RequireCurrentSchema(ctx context.Context) error {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	return db.requireCurrentSchema(ctx, sub)
}

func (db *DB) requireCurrentSchema(ctx context.Context, fsys fs.FS) error {
	known, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	conn, err := db.read.Conn(ctx)
	if err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var exists bool
	err = conn.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = 'schema_migrations')").Scan(&exists)
	if err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	var applied []migration
	if exists {
		if applied, err = appliedMigrations(ctx, conn); err != nil {
			return fmt.Errorf("check schema: %w", err)
		}
	}
	if len(applied) > len(known) {
		// pendingMigrations would advise a restore, which is wrong for a
		// command next to a newer server that is running.
		return fmt.Errorf("%w: the database is at version %04d and this binary knows %04d; run the command from the tdeemon version the server runs",
			ErrSchemaTooNew, len(applied), len(known))
	}
	// A changed migration file is the server's to warn about.
	pending, err := pendingMigrations(known, applied, slog.New(slog.DiscardHandler))
	if err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w: the database is at version %04d and this binary expects %04d; start the server once to migrate it",
			ErrSchemaOutdated, len(applied), len(known))
	}
	db.requiredVersion.Store(int64(len(known)))
	return nil
}

// withSingleUseConn runs fn on the write connection and then discards the
// connection instead of returning it to the pool, so per-connection state
// fn or a migration changed (pragmas, TEMP objects) never reaches anyone
// else. The pool opens a fresh connection with the standard settings for
// the next user.
func (db *DB) withSingleUseConn(ctx context.Context, fn func(conn *sql.Conn) error) error {
	conn, err := db.write.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = conn.Close()
	}()
	return fn(conn)
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

// applyMigration runs m in one transaction on conn, which the caller
// discards afterwards.
func applyMigration(ctx context.Context, conn *sql.Conn, m migration) error {
	// SQLite cannot alter most column definitions in place; a migration
	// rebuilds the table instead (create, copy, drop, rename). With
	// foreign keys enforced, dropping a referenced table fails or
	// cascades, so they are off and the checks below run before the
	// commit. The pragma is a no-op inside a transaction, hence here; the
	// connection is discarded afterwards, so it is never switched back on.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

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
	if err := checkSchema(ctx, tx); err != nil {
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

// schemaProbe is the throwaway table checkSchema creates and renames.
const schemaProbe = "tdeemon_schema_probe"

// checkSchema fails if a view or trigger refers to a table that does not
// exist, or a view cannot be read. SQLite resolves those only when they
// are used, so a table rebuild that leaves one pointing at a dropped table
// would go unnoticed until the first read or write that touches it fails.
//
// Two checks, because neither covers the other:
//   - Renaming a table makes SQLite re-check every view and trigger: the
//     tables they use and the columns in their expressions. It does not
//     check a trigger's INSERT column list or UPDATE SET targets. A
//     throwaway table is created, renamed and dropped for it, with
//     legacy_alter_table and writable_schema off first, because a
//     migration may have turned them on and either one skips the check.
//   - Reading every view (LIMIT 0) also catches a view whose column list
//     no longer matches its query, or that calls a missing function.
func checkSchema(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "PRAGMA legacy_alter_table = OFF; PRAGMA writable_schema = OFF;"+
		" CREATE TABLE "+schemaProbe+" (x INTEGER);"+
		" ALTER TABLE "+schemaProbe+" RENAME TO "+schemaProbe+"_renamed;"+
		" DROP TABLE "+schemaProbe+"_renamed")
	if err != nil {
		return fmt.Errorf("leaves views or triggers that refer to something missing: %w", err)
	}
	return checkViews(ctx, tx)
}

// checkViews fails if a view cannot be read.
func checkViews(ctx context.Context, tx *sql.Tx) error {
	// All names first: the probes run on the same connection.
	views, err := viewNames(ctx, tx)
	if err != nil {
		return fmt.Errorf("list views: %w", err)
	}
	var broken []string
	for _, name := range views {
		if err := readView(ctx, tx, name); err != nil {
			broken = append(broken, fmt.Sprintf("%s (%v)", name, err))
		}
	}
	if len(broken) > 0 {
		return fmt.Errorf("leaves views that cannot be read: %s", strings.Join(broken, "; "))
	}
	return nil
}

func viewNames(ctx context.Context, tx *sql.Tx) (names []string, err error) {
	rows, err := tx.QueryContext(ctx, "SELECT name FROM sqlite_schema WHERE type = 'view' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// readView selects no rows from the view, which fails when it refers to
// something that no longer exists.
func readView(ctx context.Context, tx *sql.Tx, name string) (err error) {
	quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	rows, err := tx.QueryContext(ctx, "SELECT * FROM "+quoted+" LIMIT 0") //nolint:gosec // G202: a view name from sqlite_schema, quoted as an identifier
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	return rows.Err()
}

// backup writes a consistent snapshot of the database at schema version
// to the backups directory next to the database file and returns its
// path. Only the newest keepPreMigrateBackups snapshots are kept.
func (db *DB) backup(ctx context.Context, conn *sql.Conn, version int, logger *slog.Logger) (string, error) {
	dir := db.backupsDir()
	dst, err := snapshot(ctx, conn, dir, preMigratePrefix, fmt.Sprintf("%s%04d-%s.db", preMigratePrefix, version,
		time.Now().UTC().Format(backupTimeFormat)))
	if err != nil {
		return "", err
	}
	if err := pruneBackups(dir, dst); err != nil {
		logger.Warn("removing old pre-migration snapshots failed", "error", err)
	}
	return dst, nil
}

// backupsDir is the directory of the snapshots, next to the database file.
func (db *DB) backupsDir() string { return filepath.Join(filepath.Dir(db.path), "backups") }

// snapshot writes a consistent copy of the database through conn to dir,
// as name, and returns its path. The copy is written under a temporary
// name that starts with prefix and renamed when complete, so an
// interrupted snapshot never looks like a finished one.
func snapshot(ctx context.Context, conn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, dir, prefix, name string,
) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}

	// VACUUM INTO accepts an existing file only if it is empty, which is
	// what CreateTemp leaves; its random name never collides.
	tmp, err := os.CreateTemp(dir, prefix+"*.tmp")
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
	dst := filepath.Join(dir, name)
	if err := os.Rename(tmpPath, dst); err != nil {
		return "", err
	}
	done = true
	return dst, syncPath(dir)
}

// pruneBackups keeps the snapshot just written (current) and the newest
// snapshots of keepPreMigrateBackups-1 other schema versions, and removes
// temporary files a killed process left behind. Files that do not look
// like snapshots are not touched.
//
// One snapshot per version: a crash loop on a failing migration takes a
// new snapshot of the same version on every restart, and those copies
// must not push out the snapshots of earlier versions. Age comes from the
// time in the name, not from the version: after a restore the version
// goes down, and the snapshot taken then is the one most worth keeping.
func pruneBackups(dir, current string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type snapshot struct{ path, version, at string }
	var (
		others []snapshot
		stale  []string
	)
	seen := map[string]bool{}
	if m := preMigrateFile.FindStringSubmatch(filepath.Base(current)); m != nil {
		seen[m[1]] = true
	}
	for _, e := range entries {
		name, file := e.Name(), filepath.Join(dir, e.Name())
		if e.IsDir() || file == current || !strings.HasPrefix(name, preMigratePrefix) {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			stale = append(stale, file)
			continue
		}
		if m := preMigrateFile.FindStringSubmatch(name); m != nil {
			others = append(others, snapshot{path: file, version: m[1], at: m[2]})
		}
	}

	// Newest first; keep the first of each version until the limit.
	slices.SortFunc(others, func(a, b snapshot) int { return strings.Compare(b.at, a.at) })
	kept := 1 // current
	for _, s := range others {
		if seen[s.version] || kept >= keepPreMigrateBackups {
			stale = append(stale, s.path)
			continue
		}
		seen[s.version] = true
		kept++
	}

	var errs []error
	for _, f := range stale {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// syncPath flushes a file or directory to disk.
func syncPath(name string) error {
	f, err := os.Open(name) //nolint:gosec // G304: a path in the data directory, built by this package
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
