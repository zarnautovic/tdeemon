-- The plan: the profile it needs, the phases of the goal timeline, the
-- targets by type of day, the training weekdays and the check-in's
-- toggle, the user's limits and the notes for a coach. The rules of the
-- values live in internal/domain/profile, plan and limits; the checks
-- here keep obviously broken rows out.

-- The profile, each part optional for logging and needed for a plan.
ALTER TABLE users ADD COLUMN sex TEXT CHECK (sex IN ('male', 'female'));
ALTER TABLE users ADD COLUMN birth_date TEXT
CHECK (birth_date GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]');
ALTER TABLE users ADD COLUMN height_cm REAL CHECK (height_cm BETWEEN 50 AND 275);

-- The check-in's toggle of the day's type: 1 a training day, 0 a rest
-- day, NULL when the user did not set it and the plan decides.
ALTER TABLE days ADD COLUMN training_day INTEGER CHECK (training_day IN (0, 1));

-- The phases of a user's goal timeline, one per start.
CREATE TABLE phases (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The phase's first day, YYYY-MM-DD.
    start TEXT NOT NULL CHECK (start GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    type TEXT NOT NULL CHECK (type IN ('cut', 'maintain', 'bulk', 'recomp')),
    -- Its planned last day; NULL for a phase open until the next starts.
    end_day TEXT CHECK (
        end_day GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]' AND end_day >= start
    ),
    -- The target rate in % of the trend weight a week, a minus a loss.
    rate_pct_week REAL NOT NULL,
    goal_weight REAL CHECK (goal_weight BETWEEN 20 AND 400),
    milestone_weight REAL CHECK (milestone_weight BETWEEN 20 AND 400),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, start)
) STRICT;

-- A user's targets, each from its day on until the next: kcal, and grams
-- of protein and fat, for training days and for rest days; the carbs are
-- what the kcal leave.
CREATE TABLE targets (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    effective_from TEXT NOT NULL
    CHECK (effective_from GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    training_kcal REAL NOT NULL CHECK (training_kcal > 0),
    training_protein REAL NOT NULL CHECK (training_protein >= 0),
    training_fat REAL NOT NULL CHECK (training_fat >= 0),
    rest_kcal REAL NOT NULL CHECK (rest_kcal > 0),
    rest_protein REAL NOT NULL CHECK (rest_protein >= 0),
    rest_fat REAL NOT NULL CHECK (rest_fat >= 0),
    -- The proposal that set the target, once proposals exist; NULL for
    -- one set otherwise.
    proposal_id TEXT CHECK (length(proposal_id) = 36),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, effective_from)
) STRICT;

-- A user's settings of the plan.
CREATE TABLE plan_settings (
    user_id TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- The training weekdays, bit i for time.Weekday(i), Sunday 0.
    training_weekdays INTEGER NOT NULL DEFAULT 0 CHECK (training_weekdays BETWEEN 0 AND 127),
    -- The user's limits as a JSON object of their names and values; a
    -- limit it does not name, or NULL for all, is at its default.
    limits TEXT CHECK (json_valid(limits) AND json_type(limits) = 'object'),
    updated_at TEXT NOT NULL
) STRICT;

-- What a coach should know of the user: preferences, dislikes, a
-- schedule. One text per user.
CREATE TABLE profile_notes (
    user_id TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    notes TEXT NOT NULL CHECK (notes <> ''),
    updated_at TEXT NOT NULL
) STRICT;

-- The events of the new kinds of writes: the table is rebuilt for its
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
        'day.training', 'plan.weekdays', 'limits.set', 'notes.set'
    )),
    -- What changed: a food's or a meal's id, a day, a day and a metric,
    -- a phase's start, a target's first day, "profile", "plan",
    -- "limits" or "notes", or for an undo the event it took back.
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
