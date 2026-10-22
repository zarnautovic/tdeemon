package store

import (
	"reflect"
	"testing"
)

// 0011 rebuilds the events table for its new operation, as 0010 did:
// every row stays as it was, in the order of its rowid.
func TestMigrationOfWorkoutDaysKeepsTheEvents(t *testing.T) {
	t.Parallel()

	db := openEmpty(t)
	if err := db.migrate(t.Context(), upTo(t, "0010_plan.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0010: %v", err)
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
	for _, e := range []struct{ id, op, entity, before, after string }{
		{"0199f0a0-0000-7000-8000-00000000000c", "notes.set", "notes", "", `{"notes":"no fish"}`},
		{"0199f0a0-0000-7000-8000-00000000000a", "day.training", "2026-10-21", `{"training_day":null}`, `{"training_day":true}`},
	} {
		exec(`INSERT INTO events (id, user_id, at, actor, client, model, operation, entity, before_state, after_state, stale)
			VALUES (?, ?, '2026-10-21T08:00:00Z', 'user', 'web', '', ?, ?, nullif(?, ''), ?, 0)`,
			e.id, user, e.op, e.entity, e.before, e.after)
	}
	exec("UPDATE events SET rowid = rowid * 10 + 7")
	before := eventRows(t, db)

	if err := db.migrate(t.Context(), upTo(t, "0011_workout_days.sql"), discard()); err != nil {
		t.Fatalf("migrate to 0011: %v", err)
	}
	if after := eventRows(t, db); !reflect.DeepEqual(after, before) {
		t.Errorf("events after the migration:\n%q\nwant\n%q", after, before)
	}
	exec(`INSERT INTO events (id, user_id, at, actor, client, operation, entity)
		VALUES ('0199f0a0-0000-7000-8000-00000000000d', ?, '2026-10-21T09:00:00Z', 'import', 'strong', 'workouts.set', 'strong')`, user)
}
