-- API tokens: how the phone (Health Auto Export) and AI clients (MCP)
-- act for a user without a browser. The token itself is shown once; the
-- row keeps only its SHA-256, so a copy of the database acts for nobody.
CREATE TABLE api_tokens (
    -- A UUIDv7: the handle the user sees and revokes the token by.
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    -- The hex SHA-256 of the token.
    token_hash TEXT NOT NULL CHECK (length(token_hash) = 64),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- What the user calls it, such as "iPhone" or "Claude".
    name TEXT NOT NULL CHECK (name <> ''),
    -- What it may do: scopes separated by spaces (ingest, tools, read).
    scopes TEXT NOT NULL CHECK (scopes <> ''),
    created_at TEXT NOT NULL,
    -- When it was last used, to the hour or so; NULL until its first use.
    last_used_at TEXT
) STRICT;

CREATE UNIQUE INDEX api_tokens_token_hash ON api_tokens (token_hash);

CREATE INDEX api_tokens_user_id ON api_tokens (user_id);
