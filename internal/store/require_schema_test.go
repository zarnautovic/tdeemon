package store

import (
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

	db := openEmpty(t)
	if err := db.migrate(t.Context(), files(map[string]string{"0001_a.sql": twoMigrations["0001_a.sql"]}), discard()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err := db.requireCurrentSchema(t.Context(), files(twoMigrations))
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
