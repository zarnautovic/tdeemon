package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
)

// Why a new identity cannot be linked to a user.
var (
	// ErrNoAccount: no user has the identity's email, or it has none.
	ErrNoAccount = errors.New("no account on this instance")
	// ErrEmailNotLinkable: the provider gives an address that normalizing
	// changes in more than ASCII letter case, so it matches no account.
	ErrEmailNotLinkable = errors.New("the provider's email address is not written as addresses here are")
	// ErrEmailNotVerified: the provider does not vouch for the email.
	ErrEmailNotVerified = errors.New("the provider did not verify the email address")
	// ErrAlreadyLinked: the user with that email already has another
	// identity at this provider.
	ErrAlreadyLinked = errors.New("the account is linked to another identity at this provider")
)

// Identity is who a provider says signed in.
type Identity struct {
	Issuer, Subject string
	// Email is the address the provider gives, normalized
	// (user.NormalizeEmail); "" when it gave none or EmailNotLinkable.
	Email string
	// EmailNotLinkable: the provider gave an address that normalizing
	// changes in more than ASCII letter case, so it links nothing.
	EmailNotLinkable bool
	// EmailVerified is the provider's word that the user controls Email.
	EmailVerified bool
}

// SignInWithIdentity returns the user that id belongs to. An identity
// seen before leads to its user. A new one is linked to the user with
// the same email, if the provider verified that email and the user has no
// identity at this issuer yet; otherwise it returns ErrEmailNotLinkable,
// ErrNoAccount, ErrEmailNotVerified or ErrAlreadyLinked. Nobody gets an account by signing in:
// an admin creates users.
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

		if id.EmailNotLinkable {
			return ErrEmailNotLinkable
		}
		if id.Email == "" {
			return ErrNoAccount
		}
		if !id.EmailVerified {
			return ErrEmailNotVerified
		}
		var linked bool
		row = tx.QueryRowContext(ctx, `
			SELECT `+userColumnsOfU+`, EXISTS (
				SELECT 1 FROM user_identities AS i WHERE i.user_id = u.id AND i.issuer = ?)
			FROM users AS u WHERE u.email = ?`, id.Issuer, id.Email)
		if u, err = scanUser(row.Scan, &linked); errors.Is(err, sql.ErrNoRows) {
			return ErrNoAccount
		} else if err != nil {
			return err
		}
		if linked {
			return ErrAlreadyLinked
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO user_identities (issuer, subject, user_id, created_at) VALUES (?, ?, ?, ?)",
			id.Issuer, id.Subject, u.ID.String(), formatTime(now))
		return err
	})
	if errors.Is(err, ErrNoAccount) || errors.Is(err, ErrEmailNotLinkable) || errors.Is(err, ErrEmailNotVerified) ||
		errors.Is(err, ErrAlreadyLinked) {
		return user.User{}, err
	}
	if err != nil {
		return user.User{}, fmt.Errorf("sign in with an identity: %w", err)
	}
	return u, nil
}
