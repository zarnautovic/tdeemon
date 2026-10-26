-- The proposals of changes of the plan (DESIGN.md §8): a coach's or the
-- user's, with why and the days it was drawn from, and the changes, each
-- with the state before and after. The user consents to one, which
-- applies it; applied ones are the plan's revisions. The rules live in
-- internal/domain/proposal; the checks here keep obviously broken rows
-- out.
CREATE TABLE proposals (
    -- A UUIDv7 the store makes.
    id TEXT NOT NULL CHECK (length(id) = 36),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Pending until decided. A pending proposal past expires_at has
    -- expired, which is not stored: it follows from the time.
    status TEXT NOT NULL CHECK (
        status IN ('pending', 'applied', 'rejected', 'superseded', 'reverted')
    ),
    -- Who proposed it: the client, and the AI model that acted for the
    -- user, or ''.
    client TEXT NOT NULL CHECK (client <> ''),
    model TEXT NOT NULL DEFAULT '',
    rationale TEXT NOT NULL CHECK (rationale <> ''),
    -- The days it was drawn from, both included; both NULL for none.
    window_from TEXT CHECK (window_from GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    window_to TEXT CHECK (
        window_to GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]' AND window_to >= window_from
    ),
    -- The changes as JSON, each with the state before and after.
    changes TEXT NOT NULL CHECK (json_valid(changes) AND json_type(changes) = 'object'),
    -- The applied proposal whose changes this one takes back.
    reverts TEXT CHECK (length(reverts) = 36),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL CHECK (expires_at > created_at),
    -- When it stopped pending, and who applied it, rejected it, or
    -- changed the plan under it: the user or an import, the client, and
    -- the model, or ''.
    decided_at TEXT,
    decided_actor TEXT CHECK (decided_actor IN ('user', 'import')),
    decided_client TEXT CHECK (decided_client <> ''),
    decided_model TEXT,
    -- The proposal that took a reverted one back.
    reverted_by TEXT CHECK (length(reverted_by) = 36),
    PRIMARY KEY (user_id, id),
    FOREIGN KEY (user_id, reverts) REFERENCES proposals (user_id, id),
    FOREIGN KEY (user_id, reverted_by) REFERENCES proposals (user_id, id),
    CHECK ((window_from IS NULL) = (window_to IS NULL)),
    CHECK ((status = 'pending') = (decided_at IS NULL)),
    CHECK (
        (decided_at IS NULL) = (decided_actor IS NULL)
        AND (decided_at IS NULL) = (decided_client IS NULL)
        AND (decided_at IS NULL) = (decided_model IS NULL)
    ),
    CHECK ((status = 'reverted') = (reverted_by IS NOT NULL))
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
        'day.training', 'plan.weekdays', 'limits.set', 'notes.set', 'workouts.set',
        'day.checkin', 'proposal.create', 'proposal.status'
    )),
    -- What changed: a food's, a meal's or a proposal's id, a day, a day
    -- and a metric, a phase's start, a target's first day, "profile",
    -- "plan", "limits", "notes" or a source of workouts, or for an undo
    -- the event it took back.
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
    PRIMARY KEY (user_id, id)
) STRICT;

INSERT INTO events_new (
    rowid, id, user_id, at, actor, client, model, operation, entity,
    before_state, after_state, undone_by, stale, undo_request
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
    undo_request
FROM events;

DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

-- The order of a user's events is the order of their rows: rowid.
CREATE INDEX events_user ON events (user_id);

-- For an entity's history.
CREATE INDEX events_user_entity ON events (user_id, entity);

-- An undo sent again finds the change it settled.
CREATE UNIQUE INDEX events_user_undo_request ON events (user_id, undo_request)
WHERE undo_request IS NOT NULL;
