package store

import (
	"bytes"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
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
