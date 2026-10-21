package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// maxPasswordInput is the longest password line read from standard
// input, in bytes: a character takes at most four.
const maxPasswordInput = 4 * auth.MaxPasswordLen

// userCommand runs "tdeemon user <subcommand>".
func userCommand(ctx context.Context, args []string, lookup config.LookupFunc, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: user: name a subcommand, such as create", errUsage)
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		_, err := io.WriteString(out, usage)
		return err
	case "create":
		return createUser(ctx, args[1:], lookup, in, out)
	}
	return fmt.Errorf("%w: user: unknown subcommand %q", errUsage, args[0])
}

type createUserFlags struct {
	email, name, timeZone string
	admin, passwordStdin  bool
}

func parseCreateUserFlags(args []string) (createUserFlags, error) {
	var f createUserFlags
	set := flag.NewFlagSet("user create", flag.ContinueOnError)
	set.SetOutput(io.Discard) // exitCode prints the error and the usage.
	set.StringVar(&f.email, "email", "", "")
	set.StringVar(&f.name, "name", "", "")
	set.StringVar(&f.timeZone, "timezone", "", "")
	set.BoolVar(&f.admin, "admin", false, "")
	set.BoolVar(&f.passwordStdin, "password-stdin", false, "")
	if err := set.Parse(args); errors.Is(err, flag.ErrHelp) {
		return f, err
	} else if err != nil {
		return f, fmt.Errorf("%w: user create: %w", errUsage, err)
	}
	if set.NArg() > 0 {
		return f, fmt.Errorf("%w: user create: unexpected argument %q", errUsage, set.Arg(0))
	}
	if f.email == "" {
		return f, fmt.Errorf("%w: user create: --email is required", errUsage)
	}
	return f, nil
}

// createUser adds a user to the database of a server that has started at
// least once. It never migrates: the server may be running, and only the
// server migrates.
func createUser(ctx context.Context, args []string, lookup config.LookupFunc, in io.Reader, out io.Writer) error {
	f, err := parseCreateUserFlags(args)
	if errors.Is(err, flag.ErrHelp) {
		_, err = io.WriteString(out, usage)
		return err
	} else if err != nil {
		return err
	}
	cfg, err := config.Load(lookup)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	u := newUser(f, cfg.Location, time.Now())
	// The hint only helps when the address itself is fine; otherwise
	// Validate reports the address.
	if f.name == "" && user.ValidateEmail(u.Email) == nil {
		if err := user.ValidateName(u.Name); err != nil {
			return fmt.Errorf("invalid user: the name comes from the part of the address before the @, and %w; set one with --name", err)
		}
	}
	if err := u.Validate(); err != nil {
		return fmt.Errorf("invalid user:\n%w", err)
	}
	generated := !f.passwordStdin
	var password string
	if generated {
		password = auth.GeneratePassword()
	} else if password, err = readPassword(in); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	if err := storeUser(ctx, filepath.Join(cfg.DataDir, store.FileName), u, hash); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Created %s %s (id %s).\n", u.Role, u.Email, u.ID); err != nil {
		return err
	}
	if generated {
		_, err = fmt.Fprintf(out, "Password: %s\nIt is shown only this once; keep it in a password manager.\n", password)
	}
	return err
}

// storeUser writes u to the database at path, which a server must have
// created and migrated to this binary's schema.
func storeUser(ctx context.Context, path string, u user.User, passwordHash string) (err error) {
	// store.Open would create a missing file, and an empty database next
	// to a server that has not started yet helps nobody.
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no database at %s: start the server once first; it creates and migrates the database", path)
	} else if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	// From here on, every write checks the schema is still this version.
	if err := db.RequireCurrentSchema(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := db.CreateUser(ctx, u, passwordHash); err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			return fmt.Errorf("%s: %w", u.Email, err)
		}
		return err
	}
	return nil
}

// newUser builds the user the flags describe, with the defaults for the
// rest. The time zone defaults to the instance's TZ, or UTC when TZ is not
// set: the machine's local zone has no name to store.
func newUser(f createUserFlags, instanceZone *time.Location, now time.Time) user.User {
	u := user.User{
		ID:           uuid.NewV7(),
		Email:        user.NormalizeEmail(f.email),
		Name:         strings.TrimSpace(f.name),
		Role:         user.RoleUser,
		Locale:       user.DefaultLocale,
		TimeZone:     f.timeZone,
		DayStartHour: user.DefaultDayStartHour,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if u.Name == "" {
		u.Name, _, _ = strings.Cut(u.Email, "@")
	}
	if f.admin {
		u.Role = user.RoleAdmin
	}
	if u.TimeZone == "" {
		u.TimeZone = "UTC"
		if instanceZone != nil && instanceZone.String() != "Local" {
			u.TimeZone = instanceZone.String()
		}
	}
	return u
}

// readPassword reads the first line of in, without its line ending, and
// checks it against the password rules.
func readPassword(in io.Reader) (string, error) {
	// Three bytes past the limit hold a line ending and one byte too
	// many, so an overlong password is reported instead of cut short.
	r := bufio.NewReader(io.LimitReader(in, maxPasswordInput+3))
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read the password from standard input: %w", err)
	}
	password := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch {
	case password == "":
		return "", errors.New("no password on standard input")
	case len(password) > maxPasswordInput:
		return "", auth.ErrPasswordTooLong
	}
	if err := auth.CheckPassword(password); err != nil {
		return "", err
	}
	return password, nil
}
