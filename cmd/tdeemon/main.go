// Command tdeemon runs the tdeemon server, and with a command, a task
// next to it (see usage).
//
// Configuration comes from the environment; see internal/config for the
// variables. The server stops gracefully on SIGINT or SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/ingest"
	"github.com/zarnautovic/tdeemon/internal/scheduler"
	"github.com/zarnautovic/tdeemon/internal/server"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/telegram"
	"github.com/zarnautovic/tdeemon/internal/watchdog"
	"github.com/zarnautovic/tdeemon/internal/webui"
)

// version can be set at build time with -ldflags "-X main.version=v1.2.3".
// When empty, the module version recorded by the Go toolchain is used.
var version string

const usage = `Usage:
  tdeemon                           run the server
  tdeemon user create [flags]       add a user
  tdeemon user set-password [flags] give a user a new password and end
                                    their sessions
  tdeemon user set-telegram [flags] set the Telegram chat a user's
                                    notifications go to
  tdeemon import-history [flags]    import a user's history from the
                                    predecessor, once
  tdeemon help                      show this help

Flags of user create:
  --email ADDRESS    the address the user signs in with (required)
  --name NAME        the name the app shows (default: the part of the
                     address before the @)
  --admin            make the user an admin
  --timezone ZONE    an IANA time zone such as Europe/Zagreb (default: TZ,
                     or UTC when TZ is not set)
  --password-stdin   read the password from the first line of standard
                     input; without it, tdeemon generates one and prints
                     it once

Flags of user set-password:
  --email ADDRESS    the user's address (required)
  --password-stdin   as for user create

Flags of user set-telegram:
  --email ADDRESS    the user's address (required)
  --chat-id ID       Telegram's id of the chat with the bot (required); 0
                     removes it. With a bot configured, the chat gets a
                     message first, and a chat that does not take it is
                     not set

Flags of import-history (--health-db, --foods, --plan, or more):
  --email ADDRESS    the user's address (required)
  --health-db PATH   the predecessor's database, health.db, which is only
                     read: the days' metrics and meals
  --foods PATH       the predecessor's food table, Baza-hrane.md: the
                     foods, each one the user has no food of its name
  --plan PATH        a plan file (JSON) written from the predecessor's
                     goals note: the profile, training weekdays, phases,
                     targets and notes, each the user has not got; the
                     limits hold the current phase and target, and the
                     ones after them, to the weigh-ins
  --dry-run          say what it would import, and write nothing

An import runs again without changing what it imported: a value merges
as before, and a meal keeps its id and what it held.

The user and import commands work on the database of a server that has
started at least once, which creates and migrates it; the server may be
running.

Configure tdeemon with TDEEMON_* environment variables; the user and import
commands read the same ones as the server. See
https://github.com/zarnautovic/tdeemon#configuration
`

// errUsage marks a command line tdeemon does not accept.
var errUsage = errors.New("invalid command line")

func main() {
	err := run(context.Background(), os.Args[1:], os.LookupEnv, os.Stdin, os.Stdout, os.Stderr)
	os.Exit(exitCode(err, os.Stderr))
}

// exitCode reports err on w, prefixed with the program name, and returns
// the exit status: 2 for a command line tdeemon does not accept (with the
// usage), 1 for any other error.
func exitCode(err error, w io.Writer) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		_, _ = fmt.Fprintf(w, "tdeemon: %v\n\n%s", err, usage)
		return 2
	default:
		_, _ = fmt.Fprintf(w, "tdeemon: %v\n", err)
		return 1
	}
}

// run runs the command in args: the server when there is none. Its
// inputs are parameters so tests can drive it without touching the
// process environment.
//
// An unknown command stops it before it reads the configuration or
// touches the database: a mistyped command (say, a health check run
// inside the container) must not start a second server on the live
// database.
func run(ctx context.Context, args []string, lookup config.LookupFunc, in io.Reader, out, logOut io.Writer) error {
	if len(args) == 0 {
		return serve(ctx, lookup, logOut)
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		_, err := io.WriteString(out, usage)
		return err
	case "user":
		return userCommand(ctx, args[1:], lookup, in, out)
	case "import-history":
		return importHistory(ctx, args[1:], lookup, out)
	}
	return fmt.Errorf("%w: unknown command %q", errUsage, args[0])
}

// serve wires the server together and blocks until ctx is cancelled or a
// termination signal arrives.
func serve(ctx context.Context, lookup config.LookupFunc, logOut io.Writer) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(lookup)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	logger := newLogger(cfg, logOut)
	slog.SetDefault(logger)
	serverVersion := buildVersion()
	logger.Info("starting tdeemon", "version", serverVersion, "config", cfg)

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("data directory: %w", err)
	}

	db, err := store.Open(ctx, filepath.Join(cfg.DataDir, store.FileName))
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("closing the database failed", "error", err)
		}
	}()
	if err := db.Migrate(ctx, logger); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	// The work in the background, the daily snapshots and the scheduler,
	// stops before the database closes: the defers run in reverse, cancel,
	// then wait, then Close.
	background, stopBackground := context.WithCancel(ctx)
	if cfg.BackupKeep > 0 {
		wait := db.StartDailySnapshots(background, cfg.BackupKeep, time.Hour, logger)
		defer wait()
	}
	defer stopBackground()

	app, embedded, err := webui.New()
	if err != nil {
		return err
	}
	logger.Info("web app", "embedded", embedded)
	apiCfg := api.Config{
		Info:      api.ServerInfo{Name: "tdeemon", Version: serverVersion},
		App:       app,
		DB:        db,
		BaseURL:   cfg.BaseURL,
		Logger:    logger,
		OIDC:      cfg.OIDC,
		SecretKey: cfg.SecretKey,
		// The limits on failed sign-ins count the client behind these.
		TrustedProxies: cfg.TrustedProxies,
		Limits:         cfg.Limits,
		// The raw bodies devices send, kept for 30 days.
		IngestArchive: &ingest.Archive{Dir: filepath.Join(cfg.DataDir, "ingest"), Keep: 30 * 24 * time.Hour},
	}
	handler, err := api.NewHandler(apiCfg)
	if err != nil {
		return fmt.Errorf("api: %w", err)
	}

	// The scheduler stops before the database closes, as the snapshots:
	// its wait is deferred after the Close, and before stopBackground.
	sched := &scheduler.Scheduler{DB: db, Logger: logger, Jobs: jobs(cfg, apiCfg)}
	wait := sched.Start(background, time.Minute)
	defer wait()
	defer stopBackground()

	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
	}
	return server.Run(ctx, ln, handler, logger)
}

// jobs are the scheduler's: the morning summary and the watchdog, with a
// Telegram bot to send them.
func jobs(cfg config.Config, apiCfg api.Config) []scheduler.Job {
	if cfg.TelegramBotToken == "" {
		return nil
	}
	bot := &telegram.Bot{Token: cfg.TelegramBotToken, BaseURL: cfg.TelegramAPIURL}
	return []scheduler.Job{api.MorningSummaryJob(apiCfg, bot), watchdog.Job(apiCfg.DB, bot, apiCfg.Logger)}
}

func newLogger(cfg config.Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == config.LogFormatText {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
