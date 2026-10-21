-- The people who use this instance. Every user-owned table refers to
-- users (id). The rules for the values (an email in lower case, a known
-- locale, an IANA time zone) live in internal/domain/user; the checks here
-- only keep obviously broken rows out.
CREATE TABLE users (
    -- A UUIDv7, in the canonical text form.
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    -- The sign-in name, in the form user.NormalizeEmail gives it.
    email TEXT NOT NULL CHECK (email <> ''),
    name TEXT NOT NULL CHECK (name <> ''),
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    -- An argon2id hash in the PHC string format. NULL means the user
    -- cannot sign in with a password (single sign-on comes later).
    password_hash TEXT,
    locale TEXT NOT NULL CHECK (locale <> ''),
    timezone TEXT NOT NULL CHECK (timezone <> ''),
    -- The hour at which the user's day starts: a meal before it belongs
    -- to the day before.
    day_start_hour INTEGER NOT NULL CHECK (day_start_hour BETWEEN 0 AND 23),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

-- One account per address. An index rather than a column constraint, so a
-- later migration can replace it (with a partial index, say, once accounts
-- can be deactivated) without rebuilding a table every other table refers
-- to.
CREATE UNIQUE INDEX users_email ON users (email);
