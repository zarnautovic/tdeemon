# Database migrations

The `.sql` files in this directory are embedded into the binary and
applied in order when tdeemon starts. Applied versions are recorded in
the `schema_migrations` table.

## Rules

- **Name:** `NNNN_snake_case_name.sql`. Versions start at `0001` and run
  without gaps or duplicates. If two branches both add the next version,
  the one merged second renames its file.
- **Forward only.** There are no down migrations; a mistake is fixed by
  a new migration. Before tdeemon applies migrations to a database that
  already has a schema, it writes a snapshot to
  `DATA_DIR/backups/pre-migrate-<version>-<time>.db`, which is the way
  back (the top-level README has the restore steps). The newest snapshot
  of each schema version is kept, for the three newest versions.
- **Never edit a merged migration.** Instances that already ran it will
  not run it again. tdeemon logs a warning when an applied migration's
  name or checksum no longer matches the file.
- **One transaction per migration, on a connection of its own,** with
  foreign key enforcement off. A pragma or TEMP object one migration
  leaves behind never reaches the next, so a fresh install (every
  migration in one go) and an upgrade (only the new ones) run the same
  way. A file must not contain `BEGIN`, `COMMIT`, `END` or `ROLLBACK`; the
  runner rejects it before anything runs. `SAVEPOINT`, `RELEASE` and
  `ROLLBACK TO` are fine.
- **The schema must hold before the commit,** or the whole migration
  rolls back:
  - every foreign key has to name an existing table, and
    `PRAGMA foreign_key_check` must come back clean;
  - views and triggers must not use missing tables, or missing columns
    in their expressions (the runner renames a throwaway table, which
    makes SQLite re-check them);
  - every view must be readable.

  Not checked: the column list of an `INSERT` and the targets of
  `UPDATE … SET` inside a trigger. Review those by hand when a rebuild
  or `DROP COLUMN` touches a table a trigger writes to.
- **Rebuild tables in SQLite's order:**
  1. drop the views and triggers that use `x`, including triggers on
     other tables;
  2. create `x_new`, copy the rows, drop `x`, rename `x_new` to `x`;
  3. recreate `x`'s indexes and triggers, and the views and triggers
     dropped in step 1.

  With foreign keys off during migrations this works for tables other
  tables reference too. Renaming the old table first makes SQLite point
  other tables' foreign keys, views and triggers at the old name, which
  the runner rejects. Skipping step 1 fails as well: SQLite refuses the
  rename while a view or trigger still uses the dropped table.
  Recreated triggers fall under the "not checked" note above.
- **Keep the schema snapshot current.** After adding a migration, run
  `go test ./internal/store -run TestSchema -update` and commit
  `internal/store/testdata/schema.sql`. The test fails when the snapshot
  is stale, so every pull request shows the full resulting schema.

## Conventions

- `STRICT` tables.
- Every user-owned table has a `user_id` column.
- Rows that clients create carry a UUIDv7 primary key stored as `TEXT`.
- Timestamps are `TEXT` in RFC 3339, UTC (`2026-10-20T08:15:00Z`);
  calendar days are `TEXT` in `YYYY-MM-DD`.
- Queries stay portable SQL where practical. A Postgres port would need
  its own migrations (DDL differs), but not new queries.

## Why not goose

The runner (`../migrate.go` and `../sqlscan.go`) is a few hundred
lines, comments included, and covers what this project needs: embedded
files, one transaction per migration, a refusal to run against a
database migrated by a newer binary, a snapshot before migrating, and
schema checks that make table rebuilds safe. It assumes one
process migrates at a time.
[goose](https://github.com/pressly/goose) would add down migrations we
do not want, and its module requires drivers for databases we do not
use. Revisit this if tdeemon ever runs several replicas against one
Postgres database, where a migration lock matters.
