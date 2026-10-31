-- The autopilot's weekly review (DESIGN.md §8): the user's autonomy and
-- the review's weekday, the autopilot as an actor of the events and the
-- proposals, the TDEE an applied proposal was agreed on (the base of the
-- autopilot's drift), and each review with its numbers.

-- propose: every step is a proposal; autopilot: a step applies by
-- itself, within the limits. The review's weekday, 0 Sunday to 6
-- Saturday, Monday by default.
ALTER TABLE plan_settings ADD COLUMN autonomy TEXT NOT NULL DEFAULT 'propose'
CHECK (autonomy IN ('propose', 'autopilot'));
ALTER TABLE plan_settings ADD COLUMN review_weekday INTEGER NOT NULL DEFAULT 1
CHECK (review_weekday BETWEEN 0 AND 6);

-- The events of the autopilot, and of its settings and reviews: the
-- table is rebuilt for its CHECKs, keeping the rowid, which orders a
-- user's events.
CREATE TABLE events_new (
    -- A UUIDv7 the store makes: the order of the ids is the order of
    -- the writes.
    id TEXT NOT NULL CHECK (length(id) = 36),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    at TEXT NOT NULL,
    -- The user, an import, or the autopilot, whose writes undo leaves.
    actor TEXT NOT NULL CHECK (actor IN ('user', 'import', 'autopilot')),
    client TEXT NOT NULL CHECK (client <> ''),
    -- The AI model that acted for the user, or ''.
    model TEXT NOT NULL DEFAULT '',
    operation TEXT NOT NULL CHECK (operation IN (
        'food.save', 'food.archive', 'recipe.save', 'meal.create', 'meal.replace', 'meal.delete',
        'day.exclude', 'day.include', 'metrics.merge', 'metric.set', 'metric.delete', 'undo',
        'profile.set', 'phase.set', 'phase.delete', 'target.set', 'target.delete',
        'day.training', 'plan.weekdays', 'limits.set', 'notes.set', 'workouts.set',
        'day.checkin', 'proposal.create', 'proposal.status', 'autopilot.set', 'autopilot.review'
    )),
    -- What changed: a food's (a recipe's too), a meal's or a proposal's
    -- id, a day, a day and a metric, a phase's start, a target's first
    -- day, "profile", "plan", "limits", "notes", "autopilot", a review's
    -- day ("review/YYYY-MM-DD") or a source of workouts, or for an undo
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

CREATE INDEX events_user ON events (user_id);

CREATE INDEX events_user_entity ON events (user_id, entity);

CREATE INDEX events_user_part_of ON events (user_id, part_of)
WHERE part_of IS NOT NULL;

CREATE UNIQUE INDEX events_user_undo_request ON events (user_id, undo_request)
WHERE undo_request IS NOT NULL;

-- The proposals, rebuilt for the autopilot as the one who proposed one
-- or applied its step, and the TDEE an applied one was agreed on,
-- keeping the rowid, which orders a user's proposals.
CREATE TABLE proposals_new (
    -- A UUIDv7 the store makes.
    id TEXT NOT NULL CHECK (length(id) = 36),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Pending until decided. A pending proposal past expires_at has
    -- expired, which is not stored: it follows from the time.
    status TEXT NOT NULL CHECK (
        status IN ('pending', 'applied', 'rejected', 'superseded', 'reverted')
    ),
    -- Who proposed it: the user (or a coach for them) or the autopilot,
    -- the client, and the AI model that acted for the user, or ''.
    actor TEXT NOT NULL DEFAULT 'user' CHECK (actor IN ('user', 'autopilot')),
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
    -- changed the plan under it: the user, an import or the autopilot,
    -- the client, and the model, or ''.
    decided_at TEXT,
    decided_actor TEXT CHECK (decided_actor IN ('user', 'import', 'autopilot')),
    decided_client TEXT CHECK (decided_client <> ''),
    decided_model TEXT,
    -- The proposal that took a reverted one back.
    reverted_by TEXT CHECK (length(reverted_by) = 36),
    -- The TDEE calibration trusted when it was applied, in kcal a day;
    -- NULL for none, or for a proposal not applied. The autopilot's
    -- drift from the target it set is a share of it.
    tdee REAL CHECK (tdee > 0),
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

INSERT INTO proposals_new (
    rowid, id, user_id, status, client, model, rationale, window_from, window_to, changes,
    reverts, created_at, expires_at, decided_at, decided_actor, decided_client,
    decided_model, reverted_by
)
SELECT
    rowid,
    id,
    user_id,
    status,
    client,
    model,
    rationale,
    window_from,
    window_to,
    changes,
    reverts,
    created_at,
    expires_at,
    decided_at,
    decided_actor,
    decided_client,
    decided_model,
    reverted_by
FROM proposals;

DROP TABLE proposals;
ALTER TABLE proposals_new RENAME TO proposals;

-- The weekly reviews, one a day at most: what each did and why, with
-- the numbers it went by (internal/domain/autopilot), and the proposal
-- it made or applied.
CREATE TABLE autopilot_reviews (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The user's day of the review, YYYY-MM-DD.
    day TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    outcome TEXT NOT NULL CHECK (outcome IN ('applied', 'proposed', 'hold', 'skipped')),
    reason TEXT NOT NULL CHECK (reason <> ''),
    -- The review's numbers as JSON.
    review TEXT NOT NULL CHECK (json_valid(review) AND json_type(review) = 'object'),
    proposal_id TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (user_id, day),
    FOREIGN KEY (user_id, proposal_id) REFERENCES proposals (user_id, id),
    CHECK ((outcome IN ('applied', 'proposed')) = (proposal_id IS NOT NULL))
) STRICT;

CREATE INDEX autopilot_reviews_proposal ON autopilot_reviews (user_id, proposal_id)
WHERE proposal_id IS NOT NULL;
