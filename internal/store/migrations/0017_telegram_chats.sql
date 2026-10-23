-- The Telegram chat a user's notifications go to: the morning summary and
-- the watchdog's alarms, sent by the instance's bot. An admin sets it
-- (tdeemon user set-telegram); no row, no Telegram.
CREATE TABLE telegram_chats (
    user_id TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- Telegram's id of the chat with the bot; a private chat's is the
    -- user's id there, a group's is below zero.
    chat_id INTEGER NOT NULL CHECK (chat_id <> 0),
    updated_at TEXT NOT NULL
) STRICT;
