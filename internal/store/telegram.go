package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// SetTelegramChat sets the Telegram chat userID's notifications go to;
// chatID 0 removes it.
func (db *DB) SetTelegramChat(ctx context.Context, userID uuid.UUID, chatID int64, now time.Time) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		if chatID == 0 {
			_, err = tx.ExecContext(ctx, "DELETE FROM telegram_chats WHERE user_id = ?", userID.String())
		} else {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO telegram_chats (user_id, chat_id, updated_at) VALUES (?, ?, ?)
				ON CONFLICT (user_id) DO UPDATE SET chat_id = excluded.chat_id, updated_at = excluded.updated_at`,
				userID.String(), chatID, formatTime(now))
		}
		if err != nil {
			return fmt.Errorf("set telegram chat: %w", err)
		}
		return nil
	})
}

// TelegramChat returns the Telegram chat userID's notifications go to,
// and whether they have one.
func (db *DB) TelegramChat(ctx context.Context, userID uuid.UUID) (int64, bool, error) {
	var chatID int64
	err := db.read.QueryRowContext(ctx, "SELECT chat_id FROM telegram_chats WHERE user_id = ?", userID.String()).Scan(&chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("telegram chat: %w", err)
	}
	return chatID, true, nil
}

// TelegramChatUsers returns the users whose notifications go to the
// Telegram chat chatID, in the order they were created: the users a
// message in that chat speaks for, usually one, none for a chat that is
// no user's.
func (db *DB) TelegramChatUsers(ctx context.Context, chatID int64) ([]user.User, error) {
	rows, err := db.read.QueryContext(ctx, "SELECT "+userColumns+
		" FROM users WHERE id IN (SELECT user_id FROM telegram_chats WHERE chat_id = ?) ORDER BY created_at, id", chatID)
	if err != nil {
		return nil, fmt.Errorf("telegram chat users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []user.User
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("telegram chat users: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telegram chat users: %w", err)
	}
	return out, nil
}
