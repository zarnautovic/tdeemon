-- Every write of a user's data, with who made it and the state before
-- and after (internal/domain/event): the audit trail, and what undo
-- takes back. Rows are added, and an undo marks the one it took back;
-- none is changed otherwise.
CREATE TABLE events (
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
        'day.exclude', 'day.include', 'metrics.merge', 'metric.set', 'metric.delete', 'undo'
    )),
    -- What changed: a food's or a meal's id, a day, a day and a metric,
    -- or for an undo the event it took back.
    entity TEXT NOT NULL CHECK (entity <> ''),
    -- The entity's state as JSON; NULL where there was none.
    before_state TEXT CHECK (before_state IS NULL OR json_valid(before_state)),
    after_state TEXT CHECK (after_state IS NULL OR json_valid(after_state)),
    -- The undo that took the event back.
    undone_by TEXT CHECK (undone_by IS NULL OR length(undone_by) = 36),
    PRIMARY KEY (user_id, id)
) STRICT;

-- For a meal's history: was it ever there (CreateMeal's tombstone).
CREATE INDEX events_user_entity ON events (user_id, entity);
