-- A day's measurements, one row per kind: weight, steps, sleep and the
-- rest (internal/domain/metric, which also holds the merge rules).
CREATE TABLE metrics (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The logical day, YYYY-MM-DD.
    day TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    metric TEXT NOT NULL CHECK (metric IN (
        'weight', 'body_fat', 'lean_mass', 'steps', 'active_kcal', 'basal_kcal',
        'sleep_h', 'rhr', 'hrv', 'vo2max', 'exercise_min'
    )),
    value REAL NOT NULL,
    -- manual: the user gave it, and it beats an import.
    source TEXT NOT NULL CHECK (source IN ('manual', 'import')),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, day, metric)
) STRICT;
