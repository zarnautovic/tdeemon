package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/history"
	"github.com/zarnautovic/tdeemon/internal/store"
)

type importHistoryFlags struct {
	email, healthDB, foods string
	dryRun                 bool
}

func parseImportHistoryFlags(args []string) (importHistoryFlags, error) {
	var f importHistoryFlags
	set := flag.NewFlagSet("import-history", flag.ContinueOnError)
	set.SetOutput(io.Discard) // exitCode prints the error and the usage.
	set.StringVar(&f.email, "email", "", "")
	set.StringVar(&f.healthDB, "health-db", "", "")
	set.StringVar(&f.foods, "foods", "", "")
	set.BoolVar(&f.dryRun, "dry-run", false, "")
	if err := set.Parse(args); errors.Is(err, flag.ErrHelp) {
		return f, err
	} else if err != nil {
		return f, fmt.Errorf("%w: import-history: %w", errUsage, err)
	}
	if set.NArg() > 0 {
		return f, fmt.Errorf("%w: import-history: unexpected argument %q", errUsage, set.Arg(0))
	}
	if f.email == "" {
		return f, fmt.Errorf("%w: import-history: --email is required", errUsage)
	}
	if f.healthDB == "" && f.foods == "" {
		return f, fmt.Errorf("%w: import-history: name what to import: --health-db, --foods or both", errUsage)
	}
	return f, nil
}

// sources are what an import reads, before it writes anything.
type sources struct {
	days         []history.Day
	foods        []history.FoodRow
	foodsSkipped []string
}

func readSources(ctx context.Context, f importHistoryFlags) (sources, error) {
	var s sources
	if f.healthDB != "" {
		days, err := history.Read(ctx, f.healthDB)
		if err != nil {
			return s, err
		}
		s.days = days
	}
	if f.foods != "" {
		file, err := os.Open(f.foods)
		if err != nil {
			return s, fmt.Errorf("the food table: %w", err)
		}
		defer func() { _ = file.Close() }()
		if s.foods, s.foodsSkipped, err = history.ReadFoods(file); err != nil {
			return s, err
		}
	}
	return s, nil
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
	src, err := readSources(ctx, f)
	if err != nil {
		return err
	}
	email := user.NormalizeEmail(f.email)
	var b strings.Builder
	err = withDatabase(ctx, cfg.DataDir, func(db *store.DB) error {
		u, err := db.UserByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no user with the address %q", email)
		} else if err != nil {
			return err
		}
		now := time.Now()
		if f.foods != "" {
			fr, err := history.ImportFoods(ctx, db, u, src.foods, now, f.dryRun)
			if err != nil {
				return err
			}
			fr.Skipped = append(src.foodsSkipped, fr.Skipped...)
			writeFoodReport(&b, u, fr, f.dryRun)
		}
		if f.healthDB != "" {
			r, err := history.Import(ctx, db, u, src.days, now, f.dryRun)
			if err != nil {
				return err
			}
			writeReport(&b, u, r, f.dryRun)
		}
		return nil
	})
	// What ran before a failure stays, and its report with it: a run again
	// goes on from there.
	if _, werr := io.WriteString(out, b.String()); werr != nil {
		return errors.Join(err, werr)
	}
	return err
}

func verb(dryRun bool) string {
	if dryRun {
		return "Would import (a dry run, nothing written)"
	}
	return "Imported"
}

// list writes title and lines, when there are any.
func list(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "%s (%d):\n", title, len(lines))
	for _, l := range lines {
		fmt.Fprintf(b, "  %s\n", l)
	}
}

// writeFoodReport writes what an import of foods into u did, or would do,
// and what it left out.
func writeFoodReport(b *strings.Builder, u user.User, r history.FoodReport, dryRun bool) {
	fmt.Fprintf(b, "%s foods for %s: %d new; %d kept, as the user has foods of their names.\n", verb(dryRun), u.Email, r.Created, r.Kept)
	list(b, "Foods renamed", r.Renamed)
	list(b, "Rows skipped", r.Skipped)
}

// writeReport writes what an import into u did, or would do, by u's
// calendar, and what it left out.
func writeReport(b *strings.Builder, u user.User, r history.Report, dryRun bool) {
	fmt.Fprintf(b, "%s for %s, whose days start at %02d:00 in %s: %d days, %d metric values, %d meals, of them %d rests of a day; %d days excluded.\n",
		verb(dryRun), u.Email, u.DayStartHour, u.TimeZone, r.Days, r.Metrics, r.Meals, r.Rests, len(r.Excluded))
	rejected := make([]string, len(r.Rejected))
	for i, x := range r.Rejected {
		rejected[i] = fmt.Sprintf("%s %s: %s", x.Date, x.Kind, x.Reason)
	}
	list(b, "Days excluded from calibration", r.Excluded)
	list(b, "Values left out", rejected)
	list(b, "Days whose meals have more kcal than the day's total, kept without a rest", r.Over)
	list(b, "Meals renamed", r.Renamed)
	list(b, "Skipped", r.Skipped)
}
