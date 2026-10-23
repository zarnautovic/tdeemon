# AGENTS.md

Instructions for AI coding agents working on tdeemon. Humans follow the
same rules; the full versions are in
[docs/conventions.md](docs/conventions.md), and the design is in
[docs/DESIGN.md](docs/DESIGN.md).

## The project

tdeemon is a self-hosted nutrition tracker with an adaptive TDEE: one Go
binary with SQLite and a React web app inside, an MCP server and a REST
API. It is early: the skeleton, the database layer, users, sign-in
(password, sessions, OpenID Connect), API tokens with scopes, foods,
meals, metrics and the plan (profile, phases, targets, limits) in the
domain and the database with an event log and undo, the API for the
profile and the limits, calibration (the measured TDEE and the verdict
on a phase), the tool registry with `get_profile`, `get_day`,
`get_summary`, `find_foods`, `get_food`, `upsert_food`, `log_meal`,
`update_meal`, `delete_meal`, `log_metric`, `delete_metric`, `checkin`
and `undo_last`, the MCP server that serves them at `/mcp`, the API
contract and an empty web app exist.

| Path | What |
| --- | --- |
| `cmd/tdeemon` | the binary: the server (configuration, wiring, start and stop) and the `user create`, `user set-password`, `user set-telegram`, `user set-locale` and `import-history` commands |
| `internal/config` | settings from `TDEEMON_*` environment variables |
| `internal/domain` | the domain types and their rules, one package each: `user`, `day` (the logical day and its status), `food` (foods and nutrients), `meal` (items, totals, warnings), `metric` (a day's measurements and their merge rules), `profile` (sex, birth date and height, and the age, BMI, BMR and protein's reference mass that follow), `plan` (phases and their rates, targets by type of day, the type of a day), `limits` (the three levels of limits of a plan, and `Validate`, whose contract is docs/limits.md), `calibration` (the trend weight, the measured TDEE with its interval and the verdict on a phase, DESIGN.md §7), `recovery` (the last week's sleep, resting heart rate and HRV against the weeks before), `event` (who wrote what, for the audit trail and undo), `clientid` (the ids a client makes for a meal or an undo, so a retry is known), `text` (the rules for free text, shared by the others) |
| `internal/auth` | credentials: password rules, argon2id hashes, session and API tokens, scopes, sealed values and who a request acts for |
| `internal/ingest` | what devices and apps send (Health Auto Export, a simple JSON form) turned into a day's metrics: which value a day keeps, units, a check of each value; the days of workouts in Strong's CSV export; the archive of the bodies |
| `internal/history` | the one-off import of a user's history from the predecessor: days' metrics and meals from its database, with the rest of a day's total, foods from its food table, and a plan file written from its goals note (DESIGN.md §10) |
| `internal/telegram` | sends a message through the instance's Telegram bot, and only sends |
| `internal/notify` | sends a user a notification in their Telegram chat, following a group to its supergroup |
| `internal/watchdog` | the alarms when a user's data stop coming: no metric for 26 h, an ingest token unused for 3 days |
| `internal/scheduler` | the jobs of each user's day (the morning summary, the watchdog), each occurrence once in the usual case and at least once in any, recorded in the store; and the planner's statistics refreshed daily |
| `internal/store` | SQLite access and the migrations in `migrations/` |
| `internal/store/storetest` | a fresh, migrated database file for tests |
| `internal/api` | the HTTP API (huma), whose spec is `api/openapi.json`: sign-in, sessions, API tokens, the profile and the limits of a plan (web session only), the imports (API token only), and the tools (`tools.go`, the registry; `mcp.go` serves it at `/mcp`) |
| `internal/server` | the HTTP server and graceful shutdown |
| `internal/webui` | serves the built web app (build tag `webui`) |
| `web/` | the React app (Vite, TypeScript 7, TanStack, Tailwind) |
| `web/codegen` | generates `web/src/api/schema.d.ts` from the spec |
| `docs/` | the design, the conventions, and the limits of a plan with their sources (`limits.md`) |
| `.github/` | the CI workflows, the ruleset for `main`, issue and PR templates |
| `scripts/` | `next-version.sh` for releases and `image-tags.sh` for their image, with their tests; `apply-github-settings.sh`, for the maintainer only |

## Commands

```sh
mise install       # every tool at the pinned version (mise.toml, mise.lock)
lefthook install   # git hooks, once per clone
make check         # what CI checks, except a few that only CI runs
make fmt           # Go and the web app; a new migration: sqlfluff fix <file>
make help          # the other targets
```

Without mise activated in the shell, run the tools as
`mise exec -- <tool>`. Run the server with `TDEEMON_BASE_URL`,
`TDEEMON_SECRET_KEY` and `go run ./cmd/tdeemon`; the README has the
details.

## Rules

- **Never commit to `main`.** Work on a branch and open a pull request.
  It is squash-merged, so its title, a Conventional Commit
  (`feat: …`, `fix(store): …`, `!` for breaking changes), becomes the
  commit on `main`.
- **No AI attribution** in commits or pull requests: no `Co-Authored-By`
  trailers for AI tools and no "Generated with" lines.
- **No personal data in the repo, ever.** The repo is public. Real
  weights, meals, health exports and chat sessions stay out of git; test
  fixtures are made up or anonymised. No secrets either.
- **A vulnerability stays out of public view.** One you find stays out
  of anything pushed to GitHub (issues, pull requests, branches and
  commits) until the maintainer lands the fix, as docs/conventions.md
  describes; tell the maintainer in the session.
- **Never run `scripts/apply-github-settings.sh`**, except with
  `--dry-run`. It changes the live repository's settings and the ruleset
  for `main` with admin rights; the maintainer runs it.
- **Tests come with every change in behaviour**, and they fail without
  it. Use a real SQLite file (`storetest.Open`), not mocks of the
  standard library. Coverage floors are in `.testcoverage.yml` and
  `web/vite.config.ts`.
- **Generated files stay fresh:**
  - after changing the API, regenerate `api/openapi.json`
    (`go test ./internal/api -update`) and the TypeScript types
    (`(cd web/codegen && npm ci && npm run gen:api)`);
  - after a migration, refresh the schema snapshot
    (`go test ./internal/store -run TestSchema -update`).
- **API tokens are kept out by default.** An operation refuses API
  tokens unless its `Metadata` opens it to a scope
  (`openToTokens(auth.ScopeIngest)`, in `internal/api/tokens.go`);
  settings and anything about tokens or sessions stay closed to them.
- **One user's data stays theirs.** A store method for a user's rows
  takes the user's ID and filters by it; a handler takes the user from
  the request's principal. A new operation in the OpenAPI document adds
  its case to `isolationCases` in `internal/api/isolation_test.go`, or
  the test fails, and a new kind of data adds rows of it for both users
  to `newIsolationEnv`; a new tool adds its case to
  `mcpIsolationCases` in `internal/api/mcp_isolation_test.go`, which MCP
  holds to (`docs/conventions.md`).
- **Answers match the document.** Tests check every answer against
  `api/openapi.json`; a new operation adds its case to `answerCases` in
  `internal/api/schema_test.go`, or the test fails. In an answer's type
  a pointer has `omitempty` and a slice `nullable:"false"`, and a
  handler returns `internalError(msg, err)` instead of logging it
  (`docs/conventions.md`).
- **Migrations only go forward.** Never edit a migration once it is
  merged; add a new one. The rules are in
  `internal/store/migrations/README.md`.
- **Follow the linters, do not silence them.** A disable is for a linter
  that is wrong in that place, and it says which rule and why:
  - Go: `//nolint:<linter> // why` (nolintlint enforces both);
  - web: `// oxlint-disable-next-line <rule> -- why`, and the same for
    `eslint-disable` (oxlint and `scripts/check-disable-comments.sh`
    enforce both).
- **Code, comments, docs and commit messages are in English.**
- **Keep the docs true:** update the README, docs/conventions.md,
  docs/DESIGN.md or docs/limits.md when a change makes them wrong; a
  limit in the code and its row in docs/limits.md change together.

## Definition of Done

A change is done when its behaviour is tested, `make check` and CI
pass, generated files and docs are up to date, nothing personal entered
the repo, and the pull request says what changed, why and how it was
checked.
