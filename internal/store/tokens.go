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

// APIToken is an API token as the database keeps it: everything but the
// token itself.
type APIToken struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Name   string
	// Scopes are separated by spaces (auth.FormatScopes).
	Scopes    string
	CreatedAt time.Time
	// LastUsedAt is zero until the token's first use.
	LastUsedAt time.Time
}

// tokenFields receives the columns tokenColumns lists.
type tokenFields struct {
	id, name, scopes, created string
	lastUsed                  sql.NullString
}

const tokenColumns = "t.id, t.name, t.scopes, t.created_at, t.last_used_at" //nolint:gosec // G101: column names, not a credential.

func (f *tokenFields) dest() []any {
	return []any{&f.id, &f.name, &f.scopes, &f.created, &f.lastUsed}
}

func (f *tokenFields) token(userID uuid.UUID) (APIToken, error) {
	t := APIToken{UserID: userID, Name: f.name, Scopes: f.scopes}
	var err error
	if t.ID, err = uuid.Parse(f.id); err != nil {
		return APIToken{}, fmt.Errorf("token id %q: %w", f.id, err)
	}
	if t.CreatedAt, err = parseTime(f.created); err != nil {
		return APIToken{}, err
	}
	if f.lastUsed.Valid {
		if t.LastUsedAt, err = parseTime(f.lastUsed.String); err != nil {
			return APIToken{}, err
		}
	}
	return t, nil
}

// CreateAPIToken stores t under tokenHash (auth.NewAPIToken).
func (db *DB) CreateAPIToken(ctx context.Context, t APIToken, tokenHash string) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO api_tokens (id, token_hash, user_id, name, scopes, created_at) VALUES (?, ?, ?, ?, ?, ?)",
			t.ID.String(), tokenHash, t.UserID.String(), t.Name, t.Scopes, formatTime(t.CreatedAt))
		if err != nil {
			return fmt.Errorf("create API token: %w", err)
		}
		return nil
	})
}

// APITokenUser returns the token stored under tokenHash and its user, or
// ErrNotFound.
func (db *DB) APITokenUser(ctx context.Context, tokenHash string) (APIToken, user.User, error) {
	var f tokenFields
	row := db.read.QueryRowContext(ctx, `
		SELECT `+userColumnsOfU+`, `+tokenColumns+`
		FROM api_tokens AS t JOIN users AS u ON u.id = t.user_id
		WHERE t.token_hash = ?`, tokenHash)
	u, err := scanUser(row.Scan, f.dest()...)
	if errors.Is(err, sql.ErrNoRows) {
		return APIToken{}, user.User{}, ErrNotFound
	}
	if err != nil {
		return APIToken{}, user.User{}, fmt.Errorf("API token: %w", err)
	}
	t, err := f.token(u.ID)
	if err != nil {
		return APIToken{}, user.User{}, fmt.Errorf("API token: %w", err)
	}
	return t, u, nil
}

// APITokens returns the tokens of the user with userID, oldest first.
func (db *DB) APITokens(ctx context.Context, userID uuid.UUID) (tokens []APIToken, err error) {
	rows, err := db.read.QueryContext(ctx,
		"SELECT "+tokenColumns+" FROM api_tokens AS t WHERE t.user_id = ? ORDER BY t.created_at, t.id", userID.String())
	if err != nil {
		return nil, fmt.Errorf("API tokens: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var f tokenFields
		if err := rows.Scan(f.dest()...); err != nil {
			return nil, fmt.Errorf("API tokens: %w", err)
		}
		t, err := f.token(userID)
		if err != nil {
			return nil, fmt.Errorf("API tokens: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("API tokens: %w", err)
	}
	return tokens, nil
}

// RevokeAPIToken deletes the token with id if it belongs to the user with
// userID, or returns ErrNotFound: another user's token looks exactly like
// one that does not exist.
func (db *DB) RevokeAPIToken(ctx context.Context, userID, id uuid.UUID) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM api_tokens WHERE id = ? AND user_id = ?", id.String(), userID.String())
		if err != nil {
			return fmt.Errorf("revoke API token: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("revoke API token: %w", err)
		} else if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// TouchAPIToken records that userID's token with id was used at now.
func (db *DB) TouchAPIToken(ctx context.Context, userID, id uuid.UUID, now time.Time) error {
	return db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE api_tokens SET last_used_at = ? WHERE id = ? AND user_id = ?",
			formatTime(now), id.String(), userID.String()); err != nil {
			return fmt.Errorf("touch API token: %w", err)
		}
		return nil
	})
}
