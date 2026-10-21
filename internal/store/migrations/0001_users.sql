-- The people who use this instance. Every user-owned table refers to
-- users (id). The rules for the values (an email in lower case, a known
-- locale, an IANA time zone) live in internal/domain/user; the checks here
-- only keep obviously broken rows out.
CREATE TABLE users (
    -- A UUIDv7, in the canonical text form.
    id TEXT PRIMARY KEY,
    -- The sign-in name, trimmed and in lower case.
    email TEXT NOT NULL UNIQUE CHECK (email <> ''),
    name TEXT NOT NULL CHECK (name <> ''),
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    -- An argon2id hash in the PHC string format. NULL means the user
    -- cannot sign in with a password (single sign-on comes later).
    password_hash TEXT,
    locale TEXT NOT NULL,
    timezone TEXT NOT NULL,
    -- The hour at which the user's day starts: a meal before it belongs
    -- to the day before.
    day_start_hour INTEGER NOT NULL CHECK (day_start_hour BETWEEN 0 AND 23),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
