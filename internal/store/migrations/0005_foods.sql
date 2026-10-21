-- The user's food database. The rules for the values live in
-- internal/domain/food; the checks here only keep obviously broken rows
-- out.
CREATE TABLE foods (
    -- A UUIDv7, chosen by the client that first saved the food.
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (name <> ''),
    brand TEXT NOT NULL DEFAULT '',
    -- name and brand as food.Key compares them: the same name and brand
    -- are the same food (see foods_user_name_brand).
    name_key TEXT NOT NULL CHECK (name_key <> ''),
    brand_key TEXT NOT NULL DEFAULT '',
    -- What the nutrients refer to: 100 g, 100 ml or one portion.
    basis TEXT NOT NULL CHECK (basis IN ('100g', '100ml', 'portion')),
    kcal REAL NOT NULL CHECK (kcal >= 0),
    protein REAL NOT NULL CHECK (protein >= 0),
    carbs REAL NOT NULL CHECK (carbs >= 0),
    fat REAL NOT NULL CHECK (fat >= 0),
    -- Optional nutrients, in grams like the macronutrients; NULL when
    -- unknown.
    fiber REAL CHECK (fiber >= 0),
    saturated_fat REAL CHECK (saturated_fat >= 0),
    sugars REAL CHECK (sugars >= 0),
    salt REAL CHECK (salt >= 0),
    polyols REAL CHECK (polyols >= 0),
    alcohol REAL CHECK (alcohol >= 0),
    -- The weight of one portion of a food per portion, when known.
    portion_g REAL CHECK (portion_g IS NULL OR (basis = 'portion' AND portion_g > 0)),
    -- recipe and off (Open Food Facts) are planned (DESIGN.md §4); they
    -- are allowed now, so adding them needs no rebuild of the table.
    source TEXT NOT NULL CHECK (source IN ('label', 'estimate', 'user', 'dish', 'recipe', 'off')),
    note TEXT NOT NULL DEFAULT '',
    archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX foods_user_name_brand ON foods (user_id, name_key, brand_key);

-- For the foreign keys of the child tables, which name the user as well,
-- so a child row can never belong to another user's food.
CREATE UNIQUE INDEX foods_id_user ON foods (id, user_id);

-- Named amounts of a food by weight or volume: "piece" is 60 g of egg.
CREATE TABLE food_servings (
    food_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK (name <> ''),
    name_key TEXT NOT NULL CHECK (name_key <> ''),
    -- In the food's unit, grams or ml.
    amount REAL NOT NULL CHECK (amount > 0),
    PRIMARY KEY (food_id, name_key),
    FOREIGN KEY (food_id, user_id) REFERENCES foods (id, user_id) ON DELETE CASCADE
) STRICT;

-- Other names the user calls a food by. An alias leads to one food.
CREATE TABLE food_aliases (
    food_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    alias TEXT NOT NULL CHECK (alias <> ''),
    alias_key TEXT NOT NULL CHECK (alias_key <> ''),
    PRIMARY KEY (food_id, alias_key),
    FOREIGN KEY (food_id, user_id) REFERENCES foods (id, user_id) ON DELETE CASCADE
) STRICT;

CREATE UNIQUE INDEX food_aliases_user_alias ON food_aliases (user_id, alias_key);
