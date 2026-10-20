package store

import (
	"bytes"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// openEmpty opens a database without applying any migration.
func openEmpty(t *testing.T) *DB {
	t.Helper()

	db, err := Open(t.Context(), filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func files(m map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range m {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func appliedVersions(t *testing.T, db *DB) []int {
	t.Helper()

	rows, err := db.Read().QueryContext(t.Context(), "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func tableExists(t *testing.T, db *DB, name string) bool {
	t.Helper()

	var n int
	err := db.Read().QueryRowContext(t.Context(),
		"SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", name).Scan(&n)
	if err != nil {
		t.Fatalf("look up table %s: %v", name, err)
	}
	return n == 1
}

func backups(t *testing.T, db *DB) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(db.path), "backups", "*.db"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	return matches
}

const (
	createParent = `CREATE TABLE parent (
	id   INTEGER PRIMARY KEY,
	name TEXT
) STRICT;`
	createChild = `CREATE TABLE child (
	id        INTEGER PRIMARY KEY,
	parent_id INTEGER NOT NULL REFERENCES parent (id)
) STRICT;
CREATE INDEX child_parent ON child (parent_id);`
)

func TestMigrateAppliesPendingMigrationsOnce(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	fsys := files(map[string]string{
		"README.md":               "ignored",
		"0001_create_parent.sql":  createParent,
		"0002_create_child.sql":   createChild,
		"0003_parent_trigger.sql": "CREATE TRIGGER parent_name AFTER INSERT ON parent BEGIN UPDATE parent SET name = 'x' WHERE id = new.id AND name IS NULL; END;",
	})

	if err := db.migrate(t.Context(), fsys, discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := appliedVersions(t, db); len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("applied versions = %v, want [1 2 3]", got)
	}
	for _, table := range []string{"parent", "child"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %s was not created", table)
		}
	}
	if b := backups(t, db); len(b) != 0 {
		t.Errorf("a fresh database was backed up: %v", b)
	}

	// Every statement of a multi-statement file ran, the trigger body
	// with its own semicolon included.
	if err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "INSERT INTO parent (id) VALUES (1)")
		return err
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var name string
	if err := db.Read().QueryRowContext(t.Context(), "SELECT name FROM parent WHERE id = 1").Scan(&name); err != nil || name != "x" {
		t.Errorf("trigger result = %q, %v; want \"x\"", name, err)
	}

	// A second run finds nothing to do and takes no backup.
	if err := db.migrate(t.Context(), fsys, discard()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := appliedVersions(t, db); len(got) != 3 {
		t.Errorf("applied versions after a second run = %v, want three", got)
	}
	if b := backups(t, db); len(b) != 0 {
		t.Errorf("a run without pending migrations took a backup: %v", b)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_broken.sql":        "CREATE TABLE half (n INTEGER) STRICT; INSERT INTO missing VALUES (1);",
	}), discard())
	if err == nil || !strings.Contains(err.Error(), "0002_broken") {
		t.Fatalf("migrate returned %v, want an error naming 0002_broken", err)
	}
	if got := appliedVersions(t, db); len(got) != 1 {
		t.Errorf("applied versions = %v, want only [1]", got)
	}
	if tableExists(t, db, "half") {
		t.Error("table from the failed migration survived the rollback")
	}

	// Fixing the file lets the next start continue from version 1, and
	// the database is backed up first because it already has a schema.
	err = db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_broken.sql":        "CREATE TABLE half (n INTEGER) STRICT;",
	}), discard())
	if err != nil {
		t.Fatalf("migrate after the fix: %v", err)
	}
	if !tableExists(t, db, "half") {
		t.Error("fixed migration did not run")
	}
	if b := backups(t, db); len(b) != 1 || !strings.Contains(filepath.Base(b[0]), "pre-migrate-0001-") {
		t.Errorf("backups = %v, want one pre-migrate-0001 snapshot", b)
	}
}

func TestMigrateRejectsForeignKeyViolations(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_create_child.sql":  createChild,
		"0003_orphan.sql":        "INSERT INTO child (id, parent_id) VALUES (1, 42);",
	}), discard())
	if err == nil || !strings.Contains(err.Error(), "foreign key violation") {
		t.Fatalf("migrate returned %v, want a foreign key violation", err)
	}
	if got := appliedVersions(t, db); len(got) != 2 {
		t.Errorf("applied versions = %v, want [1 2]", got)
	}
	assertForeignKeysEnforced(t, db)
}

// TestMigrateRebuildsReferencedTable runs SQLite's table rebuild on a
// table other rows reference, which only works with foreign keys off.
func TestMigrateRebuildsReferencedTable(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_create_child.sql":  createChild,
		"0003_seed.sql":          "INSERT INTO parent (id, name) VALUES (1, NULL); INSERT INTO child (id, parent_id) VALUES (1, 1);",
		"0004_parent_name_not_null.sql": `CREATE TABLE parent_new (
	id   INTEGER PRIMARY KEY,
	name TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO parent_new (id, name) SELECT id, coalesce(name, '') FROM parent;
DROP TABLE parent;
ALTER TABLE parent_new RENAME TO parent;`,
	}), discard())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var n int
	if err := db.Read().QueryRowContext(t.Context(),
		"SELECT count(*) FROM child JOIN parent ON parent.id = child.parent_id").Scan(&n); err != nil || n != 1 {
		t.Errorf("child rows joined to the rebuilt parent = %d, %v; want 1", n, err)
	}
	assertForeignKeysEnforced(t, db)
}

// assertForeignKeysEnforced checks that the write connection enforces
// foreign keys again after migrations ran.
func assertForeignKeysEnforced(t *testing.T, db *DB) {
	t.Helper()

	err := db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "INSERT INTO child (id, parent_id) VALUES (99, 404)")
		return err
	})
	if err == nil {
		t.Error("inserting an orphan after migrating succeeded; foreign keys are not enforced")
	}
}

// TestMigrateRejectsMissingForeignKeyTarget runs the rebuild in the
// wrong order (rename the old table first) on empty tables: SQLite then
// points the child's foreign key at the renamed table, which is dropped,
// and foreign_key_check alone finds no row to complain about.
func TestMigrateRejectsMissingForeignKeyTarget(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_create_child.sql":  createChild,
		"0003_rename_first.sql": `ALTER TABLE parent RENAME TO parent_old;
CREATE TABLE parent (
	id   INTEGER PRIMARY KEY,
	name TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO parent (id, name) SELECT id, coalesce(name, '') FROM parent_old;
DROP TABLE parent_old;`,
	}), discard())
	if err == nil || !strings.Contains(err.Error(), "child references missing table parent_old") {
		t.Fatalf("migrate returned %v, want the dangling foreign key reported", err)
	}
	if got := appliedVersions(t, db); len(got) != 2 {
		t.Errorf("applied versions = %v, want [1 2]", got)
	}
	if !tableExists(t, db, "parent") || tableExists(t, db, "parent_old") {
		t.Error("the failed rebuild was not rolled back")
	}
}

func TestMigrateRejectsTransactionControl(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_half.sql":          "CREATE TABLE a (x INTEGER) STRICT;\nCOMMIT;\nCREATE TABLE b (x INTEGER) STRICT;",
	}), discard())
	if err == nil || !strings.Contains(err.Error(), `"0002_half.sql", line 2: COMMIT`) {
		t.Fatalf("migrate returned %v, want the COMMIT on line 2 reported", err)
	}
	// The files are checked before anything runs.
	if tableExists(t, db, "parent") || tableExists(t, db, "a") {
		t.Error("migrations ran although one of the files was rejected")
	}
}

// TestMigrateDiscardsItsConnection checks that per-connection state a
// migration changes does not leak to later writers.
func TestMigrateDiscardsItsConnection(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	err := db.migrate(t.Context(), files(map[string]string{
		"0001_connection_state.sql": "PRAGMA busy_timeout = 1;\nCREATE TEMP TABLE leftover (x INTEGER);\nCREATE TABLE kept (x INTEGER) STRICT;",
	}), discard())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err = db.WriteTx(t.Context(), func(tx *sql.Tx) error {
		var timeout, fk, temp int
		if err := tx.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&timeout); err != nil {
			return err
		}
		if err := tx.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&fk); err != nil {
			return err
		}
		if err := tx.QueryRowContext(t.Context(),
			"SELECT count(*) FROM temp.sqlite_schema WHERE name = 'leftover'").Scan(&temp); err != nil {
			return err
		}
		if timeout != busyTimeoutMS || fk != 1 || temp != 0 {
			t.Errorf("write connection after migrating: busy_timeout=%d foreign_keys=%d temp tables=%d; want %d, 1, 0",
				timeout, fk, temp, busyTimeoutMS)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WriteTx: %v", err)
	}
}

func TestMigrateRefusesNewerDatabase(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_create_child.sql":  createChild,
	}), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
	}), discard())
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("migrate with an older set returned %v, want ErrSchemaTooNew", err)
	}
}

func TestMigrateWarnsAboutChangedMigration(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": createParent,
	}), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var logs bytes.Buffer
	err := db.migrate(t.Context(), files(map[string]string{
		"0001_create_parent.sql": "-- edited after release\n" + createParent,
	}), slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("migrate with an edited file: %v", err)
	}
	if !strings.Contains(logs.String(), "has changed since it ran") {
		t.Errorf("no warning about the edited migration; logs:\n%s", logs.String())
	}
}

func TestMigrateBacksUpBeforeMigrating(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	first := map[string]string{
		"0001_create_parent.sql": createParent,
		"0002_seed.sql":          "INSERT INTO parent (id, name) VALUES (1, 'kept');",
	}
	if err := db.migrate(t.Context(), files(first), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	first["0003_create_child.sql"] = createChild
	if err := db.migrate(t.Context(), files(first), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	b := backups(t, db)
	if len(b) != 1 || !strings.HasPrefix(filepath.Base(b[0]), "pre-migrate-0002-") {
		t.Fatalf("backups = %v, want one pre-migrate-0002 snapshot", b)
	}
	snap, err := Open(t.Context(), b[0])
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = snap.Close() })
	if got := appliedVersions(t, snap); len(got) != 2 {
		t.Errorf("backup is at versions %v, want [1 2]", got)
	}
	var name string
	if err := snap.Read().QueryRowContext(t.Context(), "SELECT name FROM parent WHERE id = 1").Scan(&name); err != nil || name != "kept" {
		t.Errorf("backup row = %q, %v; want \"kept\"", name, err)
	}
	if tableExists(t, snap, "child") {
		t.Error("backup already contains the table of the pending migration")
	}
}

// backupOnce takes one pre-migration snapshot of db at version 1.
func backupOnce(t *testing.T, db *DB) string {
	t.Helper()

	conn, err := db.write.Conn(t.Context())
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	dst, err := db.backup(t.Context(), conn, 1, discard())
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	return dst
}

func TestBackupNamesAreUnique(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	// Back to back, well within the same second.
	first, second := backupOnce(t, db), backupOnce(t, db)
	if first == second {
		t.Fatalf("two backups got the same name %s", first)
	}
	if b := backups(t, db); len(b) != 2 {
		t.Errorf("backups = %v, want two", b)
	}
}

func TestBackupKeepsNewestSnapshotsOnly(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	dir := filepath.Join(filepath.Dir(db.path), "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A killed backup's leftover, and a file the pruning must not touch.
	leftover := filepath.Join(dir, "pre-migrate-123.tmp")
	other := filepath.Join(dir, "nightly-20261020.db")
	for _, f := range []string{leftover, other} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	var made []string
	for range keepPreMigrateBackups + 2 {
		made = append(made, backupOnce(t, db))
	}

	got := backups(t, db) // *.db, so it includes the unrelated file
	want := append(made[len(made)-keepPreMigrateBackups:], other)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("backups after pruning = %v, want the %d newest snapshots plus %s", got, keepPreMigrateBackups, other)
	}
	if tmps, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmps) != 0 {
		t.Errorf("temporary files left behind: %v", tmps)
	}
}

func TestLoadMigrations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		want    []string // names in order
		wantErr string
	}{
		{
			name:  "ignores other files and directories",
			files: map[string]string{"README.md": "", "0001_init.sql": "", "0002_add_users.sql": "", "notes/0003_x.sql": ""},
			want:  []string{"init", "add_users"},
		},
		{name: "empty", files: map[string]string{"README.md": ""}},
		{name: "gap", files: map[string]string{"0001_a.sql": "", "0003_c.sql": ""}, wantErr: "want version 0002"},
		{name: "duplicate", files: map[string]string{"0001_a.sql": "", "0001_b.sql": ""}, wantErr: "want version 0002"},
		{name: "starts above one", files: map[string]string{"0002_a.sql": ""}, wantErr: "want version 0001"},
		{name: "short version", files: map[string]string{"1_init.sql": ""}, wantErr: "NNNN_snake_case_name.sql"},
		{name: "upper case", files: map[string]string{"0001_Init.sql": ""}, wantErr: "NNNN_snake_case_name.sql"},
		{name: "dash", files: map[string]string{"0001-init.sql": ""}, wantErr: "NNNN_snake_case_name.sql"},
		{name: "upper case extension", files: map[string]string{"0001_init.SQL": ""}, wantErr: "NNNN_snake_case_name.sql"},
		{name: "transaction control", files: map[string]string{"0001_init.sql": "CREATE TABLE a (x INTEGER);\nEND;"}, wantErr: "line 2: END"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := loadMigrations(files(tt.files))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("loadMigrations error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadMigrations: %v", err)
			}
			var names []string
			for _, m := range got {
				names = append(names, m.name)
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("names = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestEmbeddedMigrationsAreValid(t *testing.T) {
	t.Parallel()

	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	if _, err := loadMigrations(sub); err != nil {
		t.Fatalf("embedded migrations: %v", err)
	}
	if _, err := os.Stat(filepath.Join("migrations", "README.md")); err != nil {
		t.Errorf("migrations/README.md is missing: %v", err)
	}
}
