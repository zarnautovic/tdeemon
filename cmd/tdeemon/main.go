// Command tdeemon runs the tdeemon server.
//
// Configuration comes from the environment; see internal/config for the
// variables. The process stops gracefully on SIGINT or SIGTERM.
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

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/server"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/webui"
)

// version can be set at build time with -ldflags "-X main.version=v1.2.3".
// When empty, the module version recorded by the Go toolchain is used.
var version string

const usage = `Usage: tdeemon

Runs the tdeemon server. It takes no arguments: configure it with TDEEMON_*
environment variables, see
https://github.com/zarnautovic/tdeemon#configuration
`

// errUsage marks a command line tdeemon does not accept.
var errUsage = errors.New("tdeemon takes no arguments")

func main() {
	err := run(context.Background(), os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr)
	switch {
	case errors.Is(err, errUsage):
		fmt.Fprintf(os.Stderr, "tdeemon: %v\n\n%s", err, usage)
		os.Exit(2)
	case err != nil:
		fmt.Fprintf(os.Stderr, "tdeemon: %v\n", err)
		os.Exit(1)
	}
}

// run wires the server together and blocks until ctx is cancelled or a
// termination signal arrives. Its inputs are parameters so tests can
// drive it without touching the process environment.
//
// Any argument stops it before it reads the configuration or touches the
// database: a mistyped command (say, a health check run inside the
// container) must not start a second server on the live database.
func run(ctx context.Context, args []string, lookup config.LookupFunc, out, logOut io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "-help", "--help", "help":
			_, err := io.WriteString(out, usage)
			return err
		}
		return fmt.Errorf("%w, got %q", errUsage, args)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(lookup)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	logger := newLogger(cfg, logOut)
	slog.SetDefault(logger)
	version := buildVersion()
	logger.Info("starting tdeemon", "version", version, "config", cfg)

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

	app, embedded, err := webui.New()
	if err != nil {
		return err
	}
	logger.Info("web app", "embedded", embedded)
	handler, err := api.NewHandler(api.ServerInfo{Name: "tdeemon", Version: version}, app)
	if err != nil {
		return fmt.Errorf("api: %w", err)
	}

	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
	}
	return server.Run(ctx, ln, handler, logger)
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
