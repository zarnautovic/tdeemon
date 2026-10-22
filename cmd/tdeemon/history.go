package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/history"
	"github.com/zarnautovic/tdeemon/internal/store"
)

type importHistoryFlags struct {
	email, healthDB string
	dryRun          bool
}

func parseImportHistoryFlags(args []string) (importHistoryFlags, error) {
	var f importHistoryFlags
	set := flag.NewFlagSet("import-history", flag.ContinueOnError)
	set.SetOutput(io.Discard) // exitCode prints the error and the usage.
	set.StringVar(&f.email, "email", "", "")
	set.StringVar(&f.healthDB, "health-db", "", "")
	set.BoolVar(&f.dryRun, "dry-run", false, "")
	if err := set.Parse(args); errors.Is(err, flag.ErrHelp) {
		return f, err
	} else if err != nil {
		return f, fmt.Errorf("%w: import-history: %w", errUsage, err)
	}
	if set.NArg() > 0 {
		return f, fmt.Errorf("%w: import-history: unexpected argument %q", errUsage, set.Arg(0))
	}
	if f.email == "" || f.healthDB == "" {
		return f, fmt.Errorf("%w: import-history: --email and --health-db are required", errUsage)
	}
	return f, nil
}

// importHistory imports the predecessor's history into the user with
// --email, once (DESIGN.md §10), and prints what it did. Like the user
// commands, it never migrates.
func importHistory(ctx context.Context, args []string, lookup config.LookupFunc, out io.Writer) error {
	f, err := parseImportHistoryFlags(args)
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
	days, err := history.Read(ctx, f.healthDB)
	if err != nil {
		return err
	}
	email := user.NormalizeEmail(f.email)
	var r history.Report
	err = withDatabase(ctx, cfg.DataDir, func(db *store.DB) error {
		u, err := db.UserByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no user with the address %q", email)
		} else if err != nil {
			return err
		}
		r, err = history.Import(ctx, db, u, days, time.Now(), f.dryRun)
		return err
	})
	if err != nil {
		// What failed did not change what ran before it: a run again goes
		// on from there.
		return err
	}
	return printReport(out, r, f.dryRun)
}

// printReport writes what an import did, or would do, and what it left
// out.
func printReport(out io.Writer, r history.Report, dryRun bool) error {
	var b strings.Builder
	verb := "Imported"
	if dryRun {
		verb = "Would import (a dry run, nothing written)"
	}
	fmt.Fprintf(&b, "%s: %d days, %d metric values, %d meals, of them %d rests of a day.\n", verb, r.Days, r.Metrics, r.Meals, r.Rests)
	list := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s (%d):\n", title, len(lines))
		for _, l := range lines {
			fmt.Fprintf(&b, "  %s\n", l)
		}
	}
	rejected := make([]string, len(r.Rejected))
	for i, x := range r.Rejected {
		rejected[i] = fmt.Sprintf("%s %s: %s", x.Date, x.Kind, x.Reason)
	}
	list("Values left out", rejected)
	list("Days whose meals are more than the day's total, kept without a rest", r.Over)
	list("Meals renamed", r.Renamed)
	list("Skipped", r.Skipped)
	_, err := io.WriteString(out, b.String())
	return err
}
