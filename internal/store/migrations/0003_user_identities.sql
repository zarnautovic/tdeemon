-- The identities at OpenID Connect providers that users sign in with. A
-- user is known to a provider by its subject, which never changes; the
-- email at the provider only links the identity the first time.
CREATE TABLE user_identities (
    issuer TEXT NOT NULL CHECK (issuer <> ''),
    subject TEXT NOT NULL CHECK (subject <> ''),
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY (issuer, subject)
) STRICT;

-- One identity per provider for each user.
CREATE UNIQUE INDEX user_identities_user_issuer ON user_identities (user_id, issuer);
