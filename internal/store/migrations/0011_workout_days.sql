-- The days a source recorded a workout on: until the training module
-- (P3), the dates of Strong's export. A workout makes a day a training
-- day, unless the check-in's toggle (days.training_day) says otherwise
-- (plan.TypeOf). Each import of a source replaces its days: an export
-- holds the whole history.
CREATE TABLE workout_days (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('strong')),
    day TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    PRIMARY KEY (user_id, source, day)
) STRICT;

-- The events of the new kind of write: the table is rebuilt for its
-- CHECK of operations, keeping the rowid, which orders a user's events.
CREATE TABLE events_new (
    -- A UUIDv7 the store makes: the order of the ids is the order of
    -- the writes.
    id TEXT NOT NULL CHECK (length(id) = 36),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    at TEXT NOT NULL,
    actor TEXT NOT NULL CHECK (actor IN ('user', 'import')),
    client TEXT NOT NULL CHECK (client <> ''),
    -- The AI model that acted for the user, or ''.
    model TEXT NOT NULL DEFAULT '',
    operation TEXT NOT NULL CHECK (operation IN (
        'food.save', 'food.archive', 'meal.create', 'meal.replace', 'meal.delete',
        'day.exclude', 'day.include', 'metrics.merge', 'metric.set', 'metric.delete', 'undo',
        'profile.set', 'phase.set', 'phase.delete', 'target.set', 'target.delete',
        'day.training', 'plan.weekdays', 'limits.set', 'notes.set', 'workouts.set'
    )),
    -- What changed: a food's or a meal's id, a day, a day and a metric,
    -- a phase's start, a target's first day, "profile", "plan",
    -- "limits", "notes" or a source of workouts, or for an undo the event
    -- it took back.
    entity TEXT NOT NULL CHECK (entity <> ''),
    -- The entity's state as JSON; NULL where there was none.
    before_state TEXT CHECK (before_state IS NULL OR json_valid(before_state)),
    after_state TEXT CHECK (after_state IS NULL OR json_valid(after_state)),
    -- The undo that took the event back.
    undone_by TEXT CHECK (undone_by IS NULL OR length(undone_by) = 36),
    -- 1 when undo passed over the event: the entity changed since, or
    -- the change could not be read or put back.
    stale INTEGER NOT NULL DEFAULT 0 CHECK (stale IN (0, 1)),
    PRIMARY KEY (user_id, id)
) STRICT;

INSERT INTO events_new (
    rowid, id, user_id, at, actor, client, model, operation, entity,
    before_state, after_state, undone_by, stale
)
SELECT
    rowid,
    id,
    user_id,
    at,
    actor,
    client,
    model,
    operation,
    entity,
    before_state,
    after_state,
    undone_by,
    stale
FROM events;

DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

-- The order of a user's events is the order of their rows: rowid.
CREATE INDEX events_user ON events (user_id);

-- For an entity's history.
CREATE INDEX events_user_entity ON events (user_id, entity);
