-- Web sessions. The browser holds a random token in a cookie; the row
-- holds only the SHA-256 of that token, so a copy of the database signs
-- nobody in.
CREATE TABLE sessions (
    -- The hex SHA-256 of the session token.
    id TEXT PRIMARY KEY CHECK (length(id) = 64),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    -- When the user last used the session, to the hour or so: the server
    -- updates it at most once an hour.
    last_seen_at TEXT NOT NULL,
    -- The session ends here however active it is; the user signs in again.
    expires_at TEXT NOT NULL
) STRICT;

CREATE INDEX sessions_user_id ON sessions (user_id);

CREATE INDEX sessions_expires_at ON sessions (expires_at);
