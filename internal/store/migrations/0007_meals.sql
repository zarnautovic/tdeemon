-- Meals, the items eaten in them, and what the user says about a day.
-- The arithmetic lives in internal/domain/meal; an item keeps the values
-- it was logged with, whatever happens to its food later.
CREATE TABLE meals (
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
    client TEXT NOT NULL CHECK (client IN ('mcp', 'chat', 'web', 'telegram', 'import')),
    note TEXT NOT NULL DEFAULT '',
    -- meal.Meal.Fingerprint of what the client sent to create the meal:
    -- the same id with another fingerprint is another meal, not a retry.
    -- An edit leaves it, so a late retry of the create finds its meal.
    fingerprint TEXT NOT NULL CHECK (length(fingerprint) = 64),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, id)
) STRICT;

CREATE INDEX meals_user_day ON meals (user_id, day, eaten_at);

CREATE INDEX meals_user_eaten_at ON meals (user_id, eaten_at);

CREATE TABLE meal_items (
    -- A UUIDv7 the client chose for a new item, or the store did; per
    -- user.
    id TEXT NOT NULL CHECK (length(id) = 36),
    meal_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    -- The item's place in the meal, from 0.
    position INTEGER NOT NULL CHECK (position >= 0),
    -- NULL for a one-off estimate.
    food_id TEXT,
    -- The food's name (with its brand) when logged, or the estimate's.
    name TEXT NOT NULL CHECK (name <> ''),
    -- The quantity as the client gave it: one of an amount with its unit,
    -- a number of a named serving, or a number of portions.
    quantity_amount REAL NOT NULL DEFAULT 0 CHECK (quantity_amount >= 0),
    quantity_unit TEXT NOT NULL DEFAULT '' CHECK (quantity_unit IN ('', 'g', 'ml')),
    quantity_servings REAL NOT NULL DEFAULT 0 CHECK (quantity_servings >= 0),
    quantity_serving TEXT NOT NULL DEFAULT '',
    quantity_portions REAL NOT NULL DEFAULT 0 CHECK (quantity_portions >= 0),
    -- The quantity in the food's unit, and the item's own nutrients.
    amount REAL NOT NULL CHECK (amount > 0),
    unit TEXT NOT NULL CHECK (unit IN ('g', 'ml', 'portion')),
    kcal REAL NOT NULL CHECK (kcal >= 0),
    protein REAL NOT NULL CHECK (protein >= 0),
    carbs REAL NOT NULL CHECK (carbs >= 0),
    fat REAL NOT NULL CHECK (fat >= 0),
    fiber REAL CHECK (fiber >= 0),
    saturated_fat REAL CHECK (saturated_fat >= 0),
    sugars REAL CHECK (sugars >= 0),
    salt REAL CHECK (salt >= 0),
    polyols REAL CHECK (polyols >= 0),
    alcohol REAL CHECK (alcohol >= 0),
    -- As foods.source, the planned recipe and off included.
    source TEXT NOT NULL CHECK (source IN ('label', 'estimate', 'user', 'dish', 'recipe', 'off')),
    note TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, id),
    FOREIGN KEY (user_id, meal_id) REFERENCES meals (user_id, id) ON DELETE CASCADE,
    -- Foods are archived, never deleted, while an item uses them.
    FOREIGN KEY (user_id, food_id) REFERENCES foods (user_id, id)
) STRICT;

CREATE UNIQUE INDEX meal_items_meal_position ON meal_items (user_id, meal_id, position);

-- For the last use of a food in search.
CREATE INDEX meal_items_user_food ON meal_items (user_id, food_id);

-- What the user says about a logical day. Open and complete follow from
-- the clock and are not stored; an exclusion is, with its reason.
CREATE TABLE days (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    -- Why the day is out of calibration; NULL when it is not.
    excluded_reason TEXT CHECK (excluded_reason <> ''),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, day)
) STRICT;
