package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// ErrNoAccount reports a sign-in through a provider for which no user
// exists: the identity is not linked, and its email does not lead to a
// user who could be linked.
var ErrNoAccount = errors.New("no account on this instance")

// Identity is who a provider says signed in.
type Identity struct {
	Issuer, Subject string
	// Email is the address the provider gives, normalized
	// (user.NormalizeEmail); "" when it gave none.
	Email string
	// EmailVerified is the provider's word that the user controls Email.
	EmailVerified bool
}

// SignInWithIdentity returns the user that id belongs to. An identity
// seen before leads to its user. A new one is linked to the user with
// the same email, if the provider verified that email and the user has no
// identity at this issuer yet; otherwise it returns ErrNoAccount. Nobody
// gets an account by signing in: an admin creates users.
func (db *DB) SignInWithIdentity(ctx context.Context, id Identity, now time.Time) (user.User, error) {
	var u user.User
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT `+userColumnsOfU+` FROM user_identities AS i JOIN users AS u ON u.id = i.user_id
			WHERE i.issuer = ? AND i.subject = ?`, id.Issuer, id.Subject)
		var err error
		if u, err = scanUser(row.Scan); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if !id.EmailVerified || id.Email == "" {
			return ErrNoAccount
		}
		row = tx.QueryRowContext(ctx, `
			SELECT `+userColumnsOfU+` FROM users AS u
			WHERE u.email = ? AND NOT EXISTS (
				SELECT 1 FROM user_identities AS i WHERE i.user_id = u.id AND i.issuer = ?)`, id.Email, id.Issuer)
		if u, err = scanUser(row.Scan); errors.Is(err, sql.ErrNoRows) {
			return ErrNoAccount
		} else if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO user_identities (issuer, subject, user_id, created_at) VALUES (?, ?, ?, ?)",
			id.Issuer, id.Subject, u.ID.String(), formatTime(now))
		return err
	})
	if errors.Is(err, ErrNoAccount) {
		return user.User{}, ErrNoAccount
	}
	if err != nil {
		return user.User{}, fmt.Errorf("sign in with an identity: %w", err)
	}
	return u, nil
}
