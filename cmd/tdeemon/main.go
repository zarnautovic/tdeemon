// Command tdeemon runs the tdeemon server.
//
// Configuration comes from the environment; see internal/config for the
// variables. The process stops gracefully on SIGINT or SIGTERM.
package main

import (
	"context"
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
)

// version can be set at build time with -ldflags "-X main.version=v1.2.3".
// When empty, the module version recorded by the Go toolchain is used.
var version string

func main() {
	if err := run(context.Background(), os.LookupEnv, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "tdeemon: %v\n", err)
		os.Exit(1)
	}
}

// run wires the server together and blocks until ctx is cancelled or a
// termination signal arrives. Its inputs are parameters so tests can
// drive it without touching the process environment.
func run(ctx context.Context, lookup config.LookupFunc, logOut io.Writer) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(lookup)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	logger := newLogger(cfg, logOut)
	slog.SetDefault(logger)
	logger.Info("starting tdeemon", "version", buildVersion(), "config", cfg)

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

	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
	}
	return server.Run(ctx, ln, api.NewHandler(), logger)
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
