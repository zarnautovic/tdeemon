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
// (ErrSchemaTooNew).
func (db *DB) Migrate(ctx context.Context, logger *slog.Logger) error {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return db.migrate(ctx, sub, logger)
}

func (db *DB) migrate(ctx context.Context, fsys fs.FS, logger *slog.Logger) (err error) {
	known, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Pin the write connection: the foreign_keys pragma below is per
	// connection, and nothing else may write while migrations run.
	conn, err := db.write.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer func() { _ = conn.Close() }()

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
		return nil
	}

	if len(applied) > 0 {
		dst, err := db.backup(ctx, conn, fmt.Sprintf("pre-migrate-%04d", len(applied)))
		if err != nil {
			return fmt.Errorf("migrate: back up before migrating: %w", err)
		}
		logger.Info("backed up the database before migrating", "path", dst)
	}

	// SQLite cannot alter most column definitions in place; a migration
	// rebuilds the table instead (create, copy, drop, rename). With
	// foreign keys enforced, dropping a referenced table fails or
	// cascades, so they are off while migrations run and every migration
	// must pass foreign_key_check before it commits. The pragma is a
	// no-op inside a transaction, hence here.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("migrate: disable foreign keys: %w", err)
	}
	defer func() {
		if _, fkErr := conn.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys = ON"); fkErr != nil {
			// Never hand a connection without foreign keys back to the
			// pool; ErrBadConn makes database/sql discard it.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("migrate: re-enable foreign keys: %w", fkErr))
		}
	}()

	for _, m := range pending {
		start := time.Now()
		if err := applyMigration(ctx, conn, m); err != nil {
			return fmt.Errorf("migrate: %04d_%s: %w", m.version, m.name, err)
		}
		logger.Info("applied migration", "version", m.version, "name", m.name, "duration", time.Since(start))
	}
	logger.Info("database schema migrated", "from", len(applied), "to", len(known))
	return nil
}

// loadMigrations reads the migration files in fsys and checks that their
// versions run 1, 2, 3, ... with no gaps or duplicates. Files that do not
// end in .sql (such as README.md) are ignored.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	var out []migration
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".sql" {
			continue
		}
		parts := migrationFile.FindStringSubmatch(e.Name())
		if parts == nil {
			return nil, fmt.Errorf("migration file %q: want NNNN_snake_case_name.sql", e.Name())
		}
		version, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("migration file %q: %w", e.Name(), err)
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
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

// checkForeignKeys fails if any row references a missing parent.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
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

// backup writes a consistent snapshot of the database to the backups
// directory next to the database file and returns its path.
func (db *DB) backup(ctx context.Context, conn *sql.Conn, label string) (string, error) {
	dir := filepath.Join(filepath.Dir(db.path), "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, label+"-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return "", err
	}
	return dst, nil
}
