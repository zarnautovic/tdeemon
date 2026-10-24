package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/history"
	"github.com/zarnautovic/tdeemon/internal/store"
)

type importHistoryFlags struct {
	email, healthDB, foods, plan, splitReport string
	dryRun                                    bool
}

func parseImportHistoryFlags(args []string) (importHistoryFlags, error) {
	var f importHistoryFlags
	set := flag.NewFlagSet("import-history", flag.ContinueOnError)
	set.SetOutput(io.Discard) // exitCode prints the error and the usage.
	set.StringVar(&f.email, "email", "", "")
	set.StringVar(&f.healthDB, "health-db", "", "")
	set.StringVar(&f.foods, "foods", "", "")
	set.StringVar(&f.plan, "plan", "", "")
	set.StringVar(&f.splitReport, "split-report", "", "")
	set.BoolVar(&f.dryRun, "dry-run", false, "")
	if err := set.Parse(args); errors.Is(err, flag.ErrHelp) {
		return f, err
	} else if err != nil {
		return f, fmt.Errorf("%w: import-history: %w", errUsage, err)
	}
	if set.NArg() > 0 {
		return f, fmt.Errorf("%w: import-history: unexpected argument %q", errUsage, set.Arg(0))
	}
	if f.splitReport != "" {
		// A report of the split imports nothing, into no user.
		switch {
		case f.plan != "" || f.email != "" || f.dryRun:
			return f, fmt.Errorf("%w: import-history: --split-report imports nothing; leave out --email, --plan and --dry-run", errUsage)
		case f.healthDB == "" && f.foods == "":
			return f, fmt.Errorf("%w: import-history: --split-report splits --health-db, --foods or both", errUsage)
		}
		return f, nil
	}
	if f.email == "" {
		return f, fmt.Errorf("%w: import-history: --email is required", errUsage)
	}
	if f.healthDB == "" && f.foods == "" && f.plan == "" {
		return f, fmt.Errorf("%w: import-history: name what to import: --health-db, --foods, --plan or more", errUsage)
	}
	return f, nil
}

// sources are what an import reads, before it writes anything.
type sources struct {
	days         []history.Day
	foods        []history.FoodRow
	foodsSkipped []string
	plan         history.PlanFile
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
	if f.plan != "" {
		file, err := os.Open(f.plan)
		if err != nil {
			return s, fmt.Errorf("the plan file: %w", err)
		}
		defer func() { _ = file.Close() }()
		if s.plan, err = history.ReadPlan(file); err != nil {
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
	if f.splitReport != "" {
		return writeSplitReport(ctx, f, out)
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
			fr.Skipped = append(src.foodsSkipped, fr.Skipped...)
			writeFoodReport(&b, u, fr, f.dryRun)
			if err != nil {
				return err
			}
		}
		var days history.Report
		if f.healthDB != "" {
			if days, err = history.Import(ctx, db, u, src.days, now, f.dryRun); err != nil {
				return err
			}
			writeReport(&b, u, days, f.dryRun)
		}
		// The plan last: its limits hold it to the weigh-ins the days
		// bring, which a dry run has not written.
		if f.plan != "" {
			lim := history.PlanLimits{Instance: cfg.Limits, Weighed: days.Weighed}
			r, err := history.ImportPlan(ctx, db, u, src.plan, lim, now, f.dryRun)
			if err != nil {
				return err
			}
			writePlanReport(&b, u, r, f.dryRun)
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

// writeSplitReport writes the split of the predecessor's meals and its
// food table's combinations to --split-report, a JSON file to look at
// before the import, readable by its owner only as it holds what they
// ate, and says what it holds. It imports nothing.
func writeSplitReport(ctx context.Context, f importHistoryFlags, out io.Writer) error {
	src, err := readSources(ctx, f)
	if err != nil {
		return err
	}
	r := history.ProposeSplits(src.days, src.foods)
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("the split report: %w", err)
	}
	if err := writePrivate(f.splitReport, append(body, '\n')); err != nil {
		return fmt.Errorf("the split report: %w", err)
	}
	n := r.Count()
	_, err = fmt.Fprintf(out, "Wrote the split to %s, and imported nothing: %d meals, %d of them with an amount and a food of the table for each part, "+
		"%d with an amount for each part, %d with an amount for some, %d with none; %d %s of the table that read as combinations.\n",
		f.splitReport, len(r.Meals), n[history.Known], n[history.Amounts], n[history.Partial], n[history.Untold], len(r.Foods), plural(len(r.Foods), "food", "foods"))
	return err
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// writePrivate writes body to path, readable by its owner only, whatever
// a file already there allowed: to a file of its own beside it, renamed
// over path once whole.
func writePrivate(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
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
	list(b, "Foods per portion though given by weight or volume, as per 100 they would be no food", r.PerPortion)
	list(b, "Rows skipped", r.Skipped)
}

// writePlanReport writes what an import of a plan into u did, or would
// do, what u had already, and what the limits warn of.
func writePlanReport(b *strings.Builder, u user.User, r history.PlanReport, dryRun bool) {
	fmt.Fprintf(b, "%s the plan for %s: %d parts.\n", verb(dryRun), u.Email, len(r.Written))
	list(b, "Written", r.Written)
	list(b, "Kept, as the user has them", r.Kept)
	warnings := make([]string, len(r.Warnings))
	for i, w := range r.Warnings {
		of := ""
		if w.DayType != "" {
			of = " (" + string(w.DayType) + " day)"
		}
		warnings[i] = fmt.Sprintf("%s%s: %s, the threshold %s", w.Rule, of, history.Number(w.Value), history.Number(w.Threshold))
	}
	list(b, "Warnings of the limits", warnings)
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
