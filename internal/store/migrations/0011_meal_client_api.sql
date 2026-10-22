-- A meal logged through the API with a token, a script's, says so:
-- client 'api'. The table is rebuilt for its CHECK of clients, keeping
-- the rowid.
CREATE TABLE meals_new (
    -- A UUIDv7 the client chose, so a retry of the same meal is one meal.
    -- Ids are per user, as for foods.
    id TEXT NOT NULL CHECK (length(id) = 36),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- RFC 3339 in UTC, to the second, so the text sorts as the time.
    eaten_at TEXT NOT NULL CHECK (
        eaten_at GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]T[0-2][0-9]:[0-5][0-9]:[0-6][0-9]Z'
    ),
    -- The logical day (day.Calendar.DayOf), YYYY-MM-DD.
    day TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    description TEXT NOT NULL DEFAULT '',
    client TEXT NOT NULL CHECK (client IN ('mcp', 'chat', 'web', 'telegram', 'import', 'api')),
    note TEXT NOT NULL DEFAULT '',
    -- meal.Meal.Fingerprint of what the client sent to create the meal:
    -- the same id with another fingerprint is another meal, not a retry.
    -- An edit leaves it, so a late retry of the create finds its meal.
    fingerprint TEXT NOT NULL CHECK (length(fingerprint) = 64),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, id)
) STRICT;

INSERT INTO meals_new (
    rowid, id, user_id, eaten_at, day, description, client, note,
    fingerprint, created_at, updated_at
)
SELECT
    rowid,
    id,
    user_id,
    eaten_at,
    day,
    description,
    client,
    note,
    fingerprint,
    created_at,
    updated_at
FROM meals;

DROP TABLE meals;
ALTER TABLE meals_new RENAME TO meals;

CREATE INDEX meals_user_day ON meals (user_id, day, eaten_at);

CREATE INDEX meals_user_eaten_at ON meals (user_id, eaten_at);
