-- A browser's Web Push subscription, one per device and browser that
-- turned the notifications on: where the push service takes a message
-- for it, and the keys to encrypt one with. An endpoint is one device's
-- and leads to one user, the one signed in when it subscribed.
CREATE TABLE push_subscriptions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL UNIQUE CHECK (endpoint LIKE 'https://%'),
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    -- What the device is, from its user agent ("iPhone · Safari"), for the
    -- list of devices.
    device TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_sent_at TEXT
) STRICT;

CREATE INDEX push_subscriptions_user ON push_subscriptions (user_id);
