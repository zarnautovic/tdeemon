package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

var twoMigrations = map[string]string{
	"0001_a.sql": "CREATE TABLE a (x INTEGER) STRICT;",
	"0002_b.sql": "CREATE TABLE b (x INTEGER) STRICT;",
}

func TestRequireCurrentSchema(t *testing.T) {
	t.Parallel()

	first := map[string]string{"0001_a.sql": twoMigrations["0001_a.sql"]}
	for _, tc := range []struct {
		name     string
		migrated map[string]string // what the database got; nil: nothing
		known    map[string]string // what the binary knows
		want     error
	}{
		{"current", twoMigrations, twoMigrations, nil},
		{"never migrated", nil, twoMigrations, ErrSchemaOutdated},
		{"behind", first, twoMigrations, ErrSchemaOutdated},
		{"ahead", twoMigrations, first, ErrSchemaTooNew},
		{"nothing to migrate", nil, map[string]string{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := openEmpty(t)
			if tc.migrated != nil {
				if err := db.migrate(t.Context(), files(tc.migrated), discard()); err != nil {
					t.Fatalf("migrate: %v", err)
				}
			}
			if err := db.requireCurrentSchema(t.Context(), files(tc.known)); !errors.Is(err, tc.want) {
				t.Errorf("requireCurrentSchema = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRequireCurrentSchemaSaysWhatToDo(t *testing.T) {
	t.Parallel()

	ahead := openEmpty(t)
	if err := ahead.migrate(t.Context(), files(twoMigrations), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err := ahead.requireCurrentSchema(t.Context(), files(map[string]string{"0001_a.sql": twoMigrations["0001_a.sql"]}))
	if err == nil || !strings.Contains(err.Error(), "the tdeemon version the server runs") || strings.Contains(err.Error(), "restore") {
		t.Errorf("error for a newer database = %v, want advice to match the server's version and no restore", err)
	}

	db := openEmpty(t)
	if err := db.migrate(t.Context(), files(map[string]string{"0001_a.sql": twoMigrations["0001_a.sql"]}), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err = db.requireCurrentSchema(t.Context(), files(twoMigrations))
	for _, want := range []string{"version 0001", "expects 0002", "start the server once"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v does not contain %q", err, want)
		}
	}
}

// The check only reads: it must not create schema_migrations or apply
// anything, unlike Migrate.
func TestRequireCurrentSchemaChangesNothing(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.requireCurrentSchema(t.Context(), files(twoMigrations)); !errors.Is(err, ErrSchemaOutdated) {
		t.Fatalf("requireCurrentSchema = %v, want ErrSchemaOutdated", err)
	}
	for _, table := range []string{"schema_migrations", "a", "b"} {
		if tableExists(t, db, table) {
			t.Errorf("table %s exists after the check", table)
		}
	}
}

func TestRequireCurrentSchemaRejectsBadMigrations(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	err := db.requireCurrentSchema(t.Context(), files(map[string]string{"1_a.sql": "SELECT 1;"}))
	if err == nil || !strings.Contains(err.Error(), "want NNNN_snake_case_name.sql") {
		t.Errorf("requireCurrentSchema = %v, want the migration file error", err)
	}
}

// The embedded migrations: a database Migrate brought up to date passes.
func TestRequireCurrentSchemaEmbedded(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.RequireCurrentSchema(t.Context()); !errors.Is(err, ErrSchemaOutdated) {
		t.Errorf("before Migrate: %v, want ErrSchemaOutdated", err)
	}
	if err := db.Migrate(t.Context(), discard()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.RequireCurrentSchema(t.Context()); err != nil {
		t.Errorf("after Migrate: %v, want nil", err)
	}
}

// After the check, a write fails if a server migrated the database in the
// meantime, and does not run; a handle that never checked writes as usual.
func TestRequireCurrentSchemaGuardsLaterWrites(t *testing.T) {
	t.Parallel()

	first := files(map[string]string{"0001_a.sql": twoMigrations["0001_a.sql"]})
	tool := openEmpty(t)
	if err := tool.migrate(t.Context(), first, discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := tool.requireCurrentSchema(t.Context(), first); err != nil {
		t.Fatalf("requireCurrentSchema: %v", err)
	}
	write := func(db *DB) (ran bool, err error) {
		err = db.WriteTx(t.Context(), func(tx *sql.Tx) error {
			ran = true
			_, err := tx.ExecContext(t.Context(), "INSERT INTO a (x) VALUES (1)")
			return err
		})
		return ran, err
	}
	if _, err := write(tool); err != nil {
		t.Fatalf("write at the checked version: %v", err)
	}

	// A newer server opens the same file and migrates it.
	server, err := Open(t.Context(), tool.path)
	if err != nil {
		t.Fatalf("open the server's handle: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if err := server.migrate(t.Context(), files(twoMigrations), discard()); err != nil {
		t.Fatalf("server migrate: %v", err)
	}

	if ran, err := write(tool); !errors.Is(err, ErrSchemaTooNew) || ran {
		t.Errorf("write after the server migrated: ran %v, err %v; want ErrSchemaTooNew without running", ran, err)
	}
	if _, err := write(server); err != nil {
		t.Errorf("the server's own write: %v", err)
	}
}

// The other direction: a restore to an older snapshot under the tool.
func TestRequireCurrentSchemaGuardsAgainstAnOlderSchema(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), files(twoMigrations), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.requireCurrentSchema(t.Context(), files(twoMigrations)); err != nil {
		t.Fatalf("requireCurrentSchema: %v", err)
	}
	// Simulate the version dropping; the guard only counts rows.
	if _, err := db.write.ExecContext(t.Context(), "DELETE FROM schema_migrations WHERE version = 2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	err := db.WriteTx(t.Context(), func(*sql.Tx) error { return nil })
	if !errors.Is(err, ErrSchemaOutdated) {
		t.Errorf("WriteTx = %v, want ErrSchemaOutdated", err)
	}
}
