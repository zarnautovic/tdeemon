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
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// maxPasswordInput is the longest password line read from standard
// input, in bytes.
const maxPasswordInput = auth.MaxPasswordBytes

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
	case "set-password":
		return setPassword(ctx, args[1:], lookup, in, out)
	case "set-telegram":
		return setTelegram(ctx, args[1:], lookup, out)
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
	password, generated, err := choosePassword(f.passwordStdin, in)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(ctx, password)
	if err != nil {
		return err
	}

	err = withDatabase(ctx, cfg.DataDir, func(db *store.DB) error {
		if err := db.CreateUser(ctx, u, hash); errors.Is(err, store.ErrEmailTaken) {
			return fmt.Errorf("%s: %w", u.Email, err)
		} else if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Created %s %s (id %s).\n", u.Role, u.Email, u.ID); err != nil {
		return err
	}
	return printPassword(out, password, generated)
}

type setPasswordFlags struct {
	email         string
	passwordStdin bool
}

func parseSetPasswordFlags(args []string) (setPasswordFlags, error) {
	var f setPasswordFlags
	set := flag.NewFlagSet("user set-password", flag.ContinueOnError)
	set.SetOutput(io.Discard) // exitCode prints the error and the usage.
	set.StringVar(&f.email, "email", "", "")
	set.BoolVar(&f.passwordStdin, "password-stdin", false, "")
	if err := set.Parse(args); errors.Is(err, flag.ErrHelp) {
		return f, err
	} else if err != nil {
		return f, fmt.Errorf("%w: user set-password: %w", errUsage, err)
	}
	if set.NArg() > 0 {
		return f, fmt.Errorf("%w: user set-password: unexpected argument %q", errUsage, set.Arg(0))
	}
	if f.email == "" {
		return f, fmt.Errorf("%w: user set-password: --email is required", errUsage)
	}
	return f, nil
}

// setPassword gives the user with --email a new password and ends their
// sessions: the way back when a password is lost or leaked. Like
// createUser, it never migrates.
func setPassword(ctx context.Context, args []string, lookup config.LookupFunc, in io.Reader, out io.Writer) error {
	f, err := parseSetPasswordFlags(args)
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
	password, generated, err := choosePassword(f.passwordStdin, in)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(ctx, password)
	if err != nil {
		return err
	}

	email := user.NormalizeEmail(f.email)
	err = withDatabase(ctx, cfg.DataDir, func(db *store.DB) error {
		u, err := db.UserByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no user with the address %q", email)
		} else if err != nil {
			return err
		}
		return db.SetPasswordHash(ctx, u.ID, hash, time.Now())
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Set a new password for %s and ended their sessions.\n", email); err != nil {
		return err
	}
	return printPassword(out, password, generated)
}

type setTelegramFlags struct {
	email  string
	chatID int64
}

func parseSetTelegramFlags(args []string) (setTelegramFlags, error) {
	var f setTelegramFlags
	set := flag.NewFlagSet("user set-telegram", flag.ContinueOnError)
	set.SetOutput(io.Discard) // exitCode prints the error and the usage.
	set.StringVar(&f.email, "email", "", "")
	chatID := set.String("chat-id", "", "")
	if err := set.Parse(args); errors.Is(err, flag.ErrHelp) {
		return f, err
	} else if err != nil {
		return f, fmt.Errorf("%w: user set-telegram: %w", errUsage, err)
	}
	if set.NArg() > 0 {
		return f, fmt.Errorf("%w: user set-telegram: unexpected argument %q", errUsage, set.Arg(0))
	}
	if f.email == "" || *chatID == "" {
		return f, fmt.Errorf("%w: user set-telegram: --email and --chat-id are required", errUsage)
	}
	var err error
	if f.chatID, err = strconv.ParseInt(*chatID, 10, 64); err != nil {
		return f, fmt.Errorf("%w: user set-telegram: --chat-id %q: want Telegram's id of the chat, 0 to remove it", errUsage, *chatID)
	}
	return f, nil
}

// telegramHello is the message that tells a chat it is the user's for
// notifications, by the user's locale; English for another.
var telegramHello = map[string]string{
	"en": "tdeemon is connected: your morning summary and the alerts when data stop coming arrive here.",
	"hr": "tdeemon je spojen: ovdje stižu jutarnji sažetak i upozorenja kad podaci prestanu stizati.",
}

// helloIn returns telegramHello in locale, or in English.
func helloIn(locale string) string {
	if h, ok := telegramHello[locale]; ok {
		return h
	}
	return telegramHello["en"]
}

// setTelegram sets the Telegram chat the user with --email gets their
// notifications in, or with --chat-id 0 removes it. With a bot configured
// it first sends the chat a message, so a wrong id fails here and is not
// saved. Like createUser, it never migrates.
func setTelegram(ctx context.Context, args []string, lookup config.LookupFunc, out io.Writer) error {
	f, err := parseSetTelegramFlags(args)
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
	email := user.NormalizeEmail(f.email)
	var said string
	err = withDatabase(ctx, cfg.DataDir, func(db *store.DB) error {
		u, err := db.UserByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no user with the address %q", email)
		} else if err != nil {
			return err
		}
		switch {
		case f.chatID == 0:
			said = fmt.Sprintf("Removed the Telegram chat of %s.\n", email)
		case cfg.TelegramBotToken == "":
			said = fmt.Sprintf("Set the Telegram chat of %s; no message was sent, as %sTELEGRAM_BOT_TOKEN is not set.\n", email, config.Prefix)
		default:
			bot := telegram.Bot{Token: cfg.TelegramBotToken, BaseURL: cfg.TelegramAPIURL}
			if err := bot.Send(ctx, f.chatID, helloIn(u.Locale)); err != nil {
				if _, refused := errors.AsType[*telegram.Error](err); refused {
					return fmt.Errorf("the chat %d did not take a message, so it is not set: %w", f.chatID, err)
				}
				return fmt.Errorf("telegram did not answer, so the chat is not set; try again: %w", err)
			}
			said = fmt.Sprintf("Set the Telegram chat of %s, and sent it a message.\n", email)
		}
		return db.SetTelegramChat(ctx, u.ID, f.chatID, time.Now())
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, said)
	return err
}

// choosePassword reads the password from in with --password-stdin, and
// generates one otherwise.
func choosePassword(fromStdin bool, in io.Reader) (password string, generated bool, err error) {
	if !fromStdin {
		return auth.GeneratePassword(), true, nil
	}
	password, err = readPassword(in)
	return password, false, err
}

func printPassword(out io.Writer, password string, generated bool) error {
	if !generated {
		return nil
	}
	_, err := fmt.Fprintf(out, "Password: %s\nIt is shown only this once; keep it in a password manager.\n", password)
	return err
}

// withDatabase runs fn on the database in dataDir, which a server must
// have created and migrated to this binary's schema. Every write fn makes
// checks that the schema is still that version.
func withDatabase(ctx context.Context, dataDir string, fn func(db *store.DB) error) (err error) {
	path := filepath.Join(dataDir, store.FileName)
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
	if err := db.RequireCurrentSchema(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return fn(db)
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
