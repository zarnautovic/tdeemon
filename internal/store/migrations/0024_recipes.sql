-- A recipe: a dish cooked of the user's foods, which is a food of its own
-- (source recipe) whose values per 100 g the store works out of its
-- ingredients (internal/domain/recipe) and keeps in its foods row, so
-- search and a meal take it as any food. The recipe's id is its food's.
CREATE TABLE recipes (
    food_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    -- The weight of the cooked dish in grams; NULL when it was not
    -- weighed, and the raw ingredients' weight stands for it.
    yield_g REAL CHECK (yield_g > 0 AND yield_g <= 10000),
    -- How many portions the dish is divided into; NULL when it is not.
    portions INTEGER CHECK (portions BETWEEN 1 AND 100),
    -- The weight of the empty pot, kept for the next weighing.
    pot_g REAL CHECK (pot_g > 0 AND pot_g <= 10000),
    PRIMARY KEY (user_id, food_id),
    FOREIGN KEY (user_id, food_id) REFERENCES foods (user_id, id) ON DELETE CASCADE
) STRICT;

-- A recipe's ingredients, as a meal's items: the quantity as given and
-- the values the food brought when the recipe was saved, which a later
-- correction of the food does not change.
CREATE TABLE recipe_items (
    recipe_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    -- The ingredient's place in the recipe, from 0.
    position INTEGER NOT NULL CHECK (position >= 0),
    food_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK (name <> ''),
    brand TEXT NOT NULL DEFAULT '',
    quantity_amount REAL NOT NULL DEFAULT 0 CHECK (quantity_amount >= 0),
    quantity_unit TEXT NOT NULL DEFAULT '' CHECK (quantity_unit IN ('', 'g', 'ml')),
    quantity_servings REAL NOT NULL DEFAULT 0 CHECK (quantity_servings >= 0),
    quantity_serving TEXT NOT NULL DEFAULT '',
    quantity_portions REAL NOT NULL DEFAULT 0 CHECK (quantity_portions >= 0),
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
    source TEXT NOT NULL CHECK (source IN ('label', 'estimate', 'user', 'dish', 'recipe', 'off')),
    note TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, recipe_id, position),
    FOREIGN KEY (user_id, recipe_id) REFERENCES recipes (user_id, food_id) ON DELETE CASCADE,
    -- Foods are archived, never deleted, while a recipe uses them.
    FOREIGN KEY (user_id, food_id) REFERENCES foods (user_id, id)
) STRICT;

-- For the recipes a food is an ingredient of.
CREATE INDEX recipe_items_user_food ON recipe_items (user_id, food_id);

-- The events of a recipe's save: the table is rebuilt for its CHECK of
-- operations, keeping the rowid, which orders a user's events.
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
        'food.save', 'food.archive', 'recipe.save', 'meal.create', 'meal.replace', 'meal.delete',
        'day.exclude', 'day.include', 'metrics.merge', 'metric.set', 'metric.delete', 'undo',
        'profile.set', 'phase.set', 'phase.delete', 'target.set', 'target.delete',
        'day.training', 'plan.weekdays', 'limits.set', 'notes.set', 'workouts.set',
        'day.checkin', 'proposal.create', 'proposal.status'
    )),
    -- What changed: a food's (a recipe's too), a meal's or a proposal's
    -- id, a day, a day and a metric, a phase's start, a target's first
    -- day, "profile", "plan", "limits", "notes" or a source of workouts,
    -- or for an undo the event it took back.
    entity TEXT NOT NULL CHECK (entity <> ''),
    -- The entity's state as JSON; NULL where there was none.
    before_state TEXT CHECK (before_state IS NULL OR json_valid(before_state)),
    after_state TEXT CHECK (after_state IS NULL OR json_valid(after_state)),
    -- The undo that took the event back.
    undone_by TEXT CHECK (undone_by IS NULL OR length(undone_by) = 36),
    -- 1 when undo passed over the event: the entity changed since, or
    -- the change could not be read or put back.
    stale INTEGER NOT NULL DEFAULT 0 CHECK (stale IN (0, 1)),
    -- The id a client made for an undo, on the change that undo settled,
    -- or on the undo's own event when it found nothing to undo.
    undo_request TEXT CHECK (undo_request IS NULL OR length(undo_request) = 36),
    -- The change's event a write is part of: undo takes it back with
    -- that change, never on its own.
    part_of TEXT CHECK (part_of IS NULL OR length(part_of) = 36),
    PRIMARY KEY (user_id, id)
) STRICT;

INSERT INTO events_new (
    rowid, id, user_id, at, actor, client, model, operation, entity,
    before_state, after_state, undone_by, stale, undo_request, part_of
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
    stale,
    undo_request,
    part_of
FROM events;

DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

-- The order of a user's events is the order of their rows: rowid.
CREATE INDEX events_user ON events (user_id);

-- For an entity's history.
CREATE INDEX events_user_entity ON events (user_id, entity);

-- The writes that are part of a change.
CREATE INDEX events_user_part_of ON events (user_id, part_of)
WHERE part_of IS NOT NULL;

-- An undo sent again finds the change it settled.
CREATE UNIQUE INDEX events_user_undo_request ON events (user_id, undo_request)
WHERE undo_request IS NOT NULL;
