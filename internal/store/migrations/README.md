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
  back. The three newest snapshots are kept.
- **Never edit a merged migration.** Instances that already ran it will
  not run it again. tdeemon logs a warning when an applied migration's
  name or checksum no longer matches the file.
- **One transaction per migration,** with foreign key enforcement off.
  A file must not contain `BEGIN`, `COMMIT`, `END` or `ROLLBACK`; the
  runner rejects it before anything runs. `SAVEPOINT`, `RELEASE` and
  `ROLLBACK TO` are fine.
- **Foreign keys must hold before the commit:** every foreign key has to
  name an existing table, and `PRAGMA foreign_key_check` must come back
  clean, or the whole migration rolls back.
- **Rebuild tables in SQLite's order:** create `x_new`, copy the rows,
  drop `x`, rename `x_new` to `x`. Renaming the old table first makes
  SQLite point other tables' foreign keys at the old name, which the
  check above rejects. With foreign keys off during migrations this
  works for tables other tables reference too.
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
foreign key checks that make table rebuilds safe. It assumes one
process migrates at a time.
[goose](https://github.com/pressly/goose) would add down migrations we
do not want, and its module requires drivers for databases we do not
use. Revisit this if tdeemon ever runs several replicas against one
Postgres database, where a migration lock matters.
