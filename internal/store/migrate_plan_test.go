package store

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// upTo returns the embedded migrations up to version, the ones a database
// of that version has run.
func upTo(t *testing.T, last string) fs.FS {
	t.Helper()
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") || e.Name() > last {
			continue
		}
		b, err := fs.ReadFile(sub, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = &fstest.MapFile{Data: b}
	}
	return out
}

// 0010 rebuilds the events table for its new operations: every row stays
// as it was, in the order of its rowid, which orders a user's events
// whatever their ids say.
func TestMigrationOfThePlanKeepsTheEvents(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), upTo(t, "0009_events.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0009: %v", err)
	}
	const user = "0199f0a0-0000-7000-8000-000000000001"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.write.ExecContext(t.Context(), q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, email, name, role, locale, timezone, day_start_hour, created_at, updated_at)
		VALUES (?, 'ana@example.com', 'Ana', 'user', 'en', 'UTC', 4, '2026-10-21T08:00:00Z', '2026-10-21T08:00:00Z')`, user)
	// Ids out of the order of the rows, as a clock set back makes them.
	for _, e := range []struct{ id, op, entity, before, after, undoneBy string }{
		{"0199f0a0-0000-7000-8000-00000000000c", "food.save", "f", "", `{"Name":"Bar"}`, ""},
		{"0199f0a0-0000-7000-8000-00000000000a", "metric.set", "2026-10-21/weight", `{"Value":80}`, `{"Value":81}`, "0199f0a0-0000-7000-8000-00000000000b"},
		{"0199f0a0-0000-7000-8000-00000000000b", "undo", "0199f0a0-0000-7000-8000-00000000000a", `{"Value":81}`, `{"Value":80}`, ""},
	} {
		exec(`INSERT INTO events (id, user_id, at, actor, client, model, operation, entity, before_state, after_state, undone_by, stale)
			VALUES (?, ?, '2026-10-21T08:00:00Z', 'user', 'mcp', 'a model', ?, ?, nullif(?, ''), ?, nullif(?, ''), 1)`,
			e.id, user, e.op, e.entity, e.before, e.after, e.undoneBy)
	}
	// Rowids with gaps, as deletes leave them: a copy without them would
	// number the rows 1, 2, 3.
	exec("UPDATE events SET rowid = rowid * 10 + 7")
	before := eventRows(t, db)

	if err := db.migrate(t.Context(), upTo(t, "0010_plan.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0010: %v", err)
	}
	if after := eventRows(t, db); !reflect.DeepEqual(after, before) {
		t.Errorf("events after the migration:\n%q\nwant\n%q", after, before)
	}
	// And the new operations go in.
	exec(`INSERT INTO events (id, user_id, at, actor, client, operation, entity)
		VALUES ('0199f0a0-0000-7000-8000-00000000000d', ?, '2026-10-21T09:00:00Z', 'user', 'web', 'phase.set', '2026-10-26')`, user)
}

// eventRows returns every column of every event, in the order of rowid.
func eventRows(t *testing.T, db *DB) [][]string {
	t.Helper()
	rows, err := db.Read().QueryContext(t.Context(), `SELECT rowid, id, user_id, at, actor, client, model, operation, entity,
		coalesce(before_state, 'NULL'), coalesce(after_state, 'NULL'), coalesce(undone_by, 'NULL'), stale FROM events ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out [][]string
	for rows.Next() {
		r := make([]string, 13)
		ptrs := make([]any, len(r))
		for i := range r {
			ptrs[i] = &r[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
