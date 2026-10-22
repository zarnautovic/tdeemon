# Conventions

The rules every change follows. Tools check what they can; the rest is
for the author and the reviewer.

## Running the checks

- **`make check`** runs every check CI runs, except four that only CI
  runs: the Docker image build, CodeQL, the pull request title check, and
  zizmor's online audits, which need `GH_TOKEN` set:
  - the linters and format checks for every language;
  - the Go and web tests, with coverage, the Go tests behind the
    `webui` tag against a fresh build, and the tests of the scripts;
  - the check that the generated TypeScript types match the API spec;
  - `govulncheck`, `npm audit`, the licence check and the gitleaks scan
    of the history.
  `make fmt` formats Go and the web app in place, and `make help` lists
  the targets. The tools run through mise, at the versions in
  `mise.toml`.
- **CI** (`.github/workflows/ci.yml`) runs the same targets on every pull
  request and on `main`, in five parallel jobs: Go, the web app, the
  other files, the dependencies and secrets, and the Docker image, which
  is built with its tests but not pushed. Only pushes to `main` write the
  caches; pull requests read them. Each job installs only the tools it
  runs (`install_args`), and mise does not install the others on demand,
  so a tool that a job's make target starts using goes into that job's
  list too. Without it mise warns that the tool "is not installed and
  auto-install is disabled" and runs the runner's own copy if there is
  one (Node, Python, yamllint and shellcheck are), at whatever version;
  otherwise the job stops. In CI, CodeQL and the Docker build, a Go
  module or tool the proxy fails to serve comes from its source instead.
- **`mise.lock`** pins each tool's version and, for the tools mise
  downloads as binaries, the URL and checksum for Linux and macOS on x64
  and arm64; `.mise/locks/` pins the dependencies of the Python and npm
  tools. Go tools are built from source and checked against the Go
  checksum database. CI installs with `--locked`. After changing a
  version in `mise.toml`, run
  `mise lock --platform linux-x64,linux-arm64,macos-arm64,macos-x64`.
  Renovate does this itself, in mise's safe mode.
- **Git hooks** (lefthook, `lefthook install` once per clone):
  - pre-commit formats the staged files and stages what the formatters
    fixed, then lints them; Go is linted as packages, so the whole module;
  - commit-msg checks the message is a Conventional Commit;
  - pre-push runs the Go and web tests (lefthook skips them when the
    checked-out branch adds or changes no files against its last push);
  - they are a convenience and CI is the gate: `git commit --no-verify`
    skips them.
- The Makefile needs GNU make: macOS's `/usr/bin/make` is GNU make 3.81,
  and on the BSDs it is `gmake`. GitHub's runners have it; the hooks do
  not need it.

## Tests and the Definition of Done

### What gets tested

- **Domain logic:** unit tests, table-driven.
- **The store:** integration tests against a real SQLite file
  (`storetest.Open`: each test gets its own copy of a database migrated
  once per test binary, as migrating takes seconds under `-race`), and
  every migration from an empty database.
- **HTTP and MCP handlers:** integration tests through the handler, with a
  real store behind it.
  Every answer a test gets through the instance's `do` or `serve` is
  checked against the OpenAPI document (`checkAnswer`, with
  `huma.Validate` in its mode for answers): its status must be one the
  operation lists, and its body valid all the way down, through every
  `$ref`, array and object, with no property the document leaves out.
  `TestAnswersMatchTheDocument` has a case for every operation, which
  must answer with success; `TestAnswersCoverEveryOperation` fails for an
  operation without one, so a new operation adds its case to
  `answerCases`. The problems answered before a handler runs, huma's
  (an unknown query parameter, a body that is not JSON, too large or
  too slow) and the server's (a bad API token, an address that sent too
  many, a token the operation does not let in), are listed for every
  operation (`documentRequestErrors`).
  What an answer's type may hold, so the document and the TypeScript
  types say what comes:
  - its body is a struct, never a bare slice, whose top level no tag
    could keep from being null;
  - a struct it embeds is exported: huma describes no field of an
    unexported embedded struct;
  - a pointer has `omitempty`: nil is then left out, never `null`;
  - a slice says `nullable:"false"`, as huma describes every other slice
    as null or an array, and is never nil without `omitempty` (`[]`,
    not `null`);
  - a slice or a map holds no pointers: huma describes their elements
    as not null;
  - a map is never nil without `omitempty`.

  `TestAnswersHoldNoNull` holds the types of every operation's answers
  to the rules for bodies, pointers and slices, whatever the data;
  `checkAnswer` catches a nil slice or map in the answers the tests get.
- **Imports:** payloads of the shapes a source sends, with made-up
  values, in `internal/ingest/testdata/` (Health Auto Export's in
  `hae/`, Strong's in `strong/`). Real ones stay outside git:
  `TestRealPayloads` reads the directory `TDEEMON_HAE_PAYLOADS` names,
  such as an instance's archive
  (`TDEEMON_DATA_DIR/ingest/<user>`), `.json` and `.json.gz`. It fails
  for a shape that does not read (a unit, a point, a date, a sleep
  stage) and for a metric whose points give no value; run with `-v`
  (`go test ./internal/ingest -run TestRealPayloads -v`), it says what
  each day kept and what it left out, without the values. Without the
  variable it is skipped.
- **Calibration:** a history of days, phases, targets and the manual
  revisions of the target, as a fixture
  (`internal/domain/calibration/history_test.go`). The anonymised one is
  `testdata/history.json`. The real one stays outside git:
  `TestCalibrateRealHistory` reads `calibration.json` in the directory
  `TDEEMON_PRIVATE_FIXTURES` names, and is skipped without it. Both are
  held to the same checks; run with `-v`, the test says what calibration
  made of each revision. The anonymised one is written from the real
  one, by the anonymisation only the real one holds, with
  `go test ./internal/domain/calibration -run TestWriteAnonymisedHistory -anonymise`,
  and read before it is committed.
- **Tools** (`tools()` in `internal/api/tools.go`) are operations like
  any other, with their cases in `isolationCases` and `answerCases`;
  `TestToolsAreNamed` lists the tools that exist, by their MCP name
  (DESIGN.md §5), and `TestToolsTakeReadAndToolsTokens` holds a tool that
  reads to the `read` and `tools` scopes, and one that writes to `tools`
  alone. A tool names its scopes (`newTool`), or cannot be registered.
  MCP serves every tool at `/mcp` through its operation
  (`internal/api/mcp.go`): its input schema is the operation's
  parameters and body, from the OpenAPI document, and a call is a
  request of the operation with the caller's principal, so MCP and REST
  cannot drift.
- **The web app:** components and hooks with Vitest and Testing Library.
- End-to-end tests of the main flows (Playwright) come later.

### Coverage

Coverage is a floor, not the goal: the rule is a test that fails without
the change. The floors fail `make test`, and so CI.

- **Go** (go-test-coverage, `.testcoverage.yml`):
  - 75 % in total and in every package;
  - 90 % for the domain and the boundaries where input enters, with
    their subpackages: `internal/domain`, `internal/tools`,
    `internal/api`, `internal/mcp`, `internal/config`, `internal/auth`
    (credentials), `internal/ingest` (what devices send) and
    `internal/history` (the predecessor's history). A new boundary, such
    as the chat, joins the list;
  - the `storetest` helper is excluded, as the tests that use it exercise
    it.
- **The web app** (Vitest, `web/vite.config.ts`):
  - 75 % of lines and functions. Branches and statements have no
    threshold: the React Compiler adds a cache branch, and the statements
    around it, to every memoised value, and a test takes it only one way;
  - the entry point `main.tsx` and the shadcn registry components are
    excluded.

### Definition of Done

A change is done when:

- its behaviour is covered by tests that fail without it;
- `make check` passes, and so does CI;
- generated files are fresh:
  - `api/openapi.json` and `web/src/api/schema.d.ts` after an API
    change;
  - the schema snapshot after a migration
    (`go test ./internal/store -run TestSchema -update`);
- the docs say what changed: README, these conventions, `DESIGN.md` for
  design decisions, `limits.md` for a limit of a plan;
- no personal data entered the repo (real weights, meals, sessions);
- the pull request title is a Conventional Commit, and the description
  says what changed, why, and how it was checked.

## Commits and pull requests

- Every change reaches `main` through a pull request and is squash-merged,
  so the pull request's title becomes the commit on `main`.
- Commit messages and pull requests are in English, like the code and
  the docs, and carry no AI attribution: no `Co-Authored-By` trailers for
  AI tools and no "Generated with" lines.
- **The ruleset for `main`** (`.github/rulesets/main.json`, applied with
  `scripts/apply-github-settings.sh`), with no bypass:
  - no direct pushes, force pushes or deletion;
  - a pull request, squash-merged, with linear history;
  - every required check green on a branch that is up to date with
    `main`: Go, Web app, Other files, Dependencies and secrets, Docker
    image and Conventional Commit title, each from GitHub Actions;
  - no new CodeQL error or high-severity security alert;
  - review threads resolved;
  - no approvals, since the project has one maintainer.

  The squash commit's title is the pull request's title unless it is
  edited in the merge dialog; leave it as it is. A renamed CI job must be
  renamed in `main.json` too, or every pull request waits for the old
  name. In an emergency the ruleset can be switched off in the
  repository settings (or with `"enforcement": "disabled"` and the
  script), and back on with the script.
- **Titles and commit messages are Conventional Commits:**
  `type(scope)!: summary`.
  - The types are feat, fix, perf, refactor, test, docs, ci, build, chore,
    style and revert.
  - The scope is optional.
  - `!` after the type marks a breaking change, and so does a line that
    starts with `BREAKING CHANGE:` or `BREAKING-CHANGE:`, even indented by
    spaces, in the pull request's description, which becomes the commit's
    body. cog counts such a line anywhere in the body, even in a code
    block: write it once, in the footer at the end, and nowhere else.
    Footers in the branch's commits are lost in the squash.
  - The summary is short and ends without a full stop.
  - `feat`, `fix` and `perf` decide the next release, and from 1.0.0 on a
    breaking change raises the major version whatever its type (see
    Releases).
  - GitHub's Revert button titles a pull request `Revert "feat: x"`;
    rename it to `revert: feat: x`.
- **The checks:** the "PR title" workflow runs `cog verify` (cocogitto,
  `cog.toml`) on every pull request title, and the commit-msg hook runs it
  on every commit message. Merge and fixup commits pass the hook, since
  the squash merge replaces them; a title starting with "Merge" does not
  pass.

## Releases

- **Every merge to `main` that brings a `feat`, `fix` or `perf` commit,
  or a breaking change, cuts a release** (the "Release" workflow):
  - the next version comes from the Conventional Commits since the last
    tag (`scripts/next-version.sh`, `cog.toml`): `feat` raises the minor
    version, `fix` and `perf` the patch, and a breaking change of any
    type the major;
  - before 1.0.0, a breaking change of any type raises the minor
    version instead;
  - the release is a `vX.Y.Z` tag and a GitHub Release whose notes list
    the commits by type, with a link to each, and for a breaking change
    a BREAKING mark and its footer (`.github/release-notes.tera`);
  - and the image `ghcr.io/zarnautovic/tdeemon`, for linux/amd64 and
    linux/arm64, tagged `X.Y.Z`, `X.Y` and `sha-<commit>`
    (`scripts/image-tags.sh`), with its SBOM and provenance; it builds
    without a cache, in a job of the same workflow, as a release the
    workflow's token makes starts no other. If that job fails, use
    *Re-run failed jobs* within 30 days, and only while no newer release
    has run, or its `X.Y` tag would go back: *Re-run all jobs* finds the
    commit released and releases nothing, image included;
  - the release job's token created the package `tdeemon` on ghcr.io,
    so it took this repository's visibility, public, and the image's
    `org.opencontainers.image.source` label links it here: anyone pulls
    it without credentials. A package first pushed any other way would
    be private;
  - other types (docs, chore, ci, refactor…) make no release;
  - a commit that is already in a release makes no other, so re-running
    the workflow is safe.
- `make next-version` shows the version the next release would get, and
  `make test-scripts` checks those rules and the image's tags.
- **The changelog is the list of GitHub Releases**, not a `CHANGELOG.md`:
  committing one would mean pushing to `main`, which the ruleset allows
  no one.

## Dependencies and security

- **Renovate** (`.github/renovate.json5`, once its GitHub App is
  installed) keeps every pinned version current: Go modules and the Go
  version, npm packages, the tools in `mise.toml` and `mise.lock`, the
  Docker base images by digest, the GitHub Actions by commit SHA, and
  mise, BuildKit and the SBOM scanner in the workflows.
  - One batch a week; a new npm release waits a few days first (only
    npm: the other sources are taken the day they appear).
  - Minor, patch and digest updates merge themselves once CI is green;
    majors, and minors of a package before 1.0, wait for a review.
  - The Go version moves in one pull request, in `go.mod`, `mise.toml`
    and the build image; so does Node's, in `mise.toml` and the web build
    image.
  - Titles are `fix(deps)`, which makes a release, for the Go modules,
    the npm `dependencies`, the Go version and the runtime image.
    Everything else is `chore(deps)`, also the npm devDependencies and
    lock file maintenance, although the build tools among them shape the
    bundle: their changes ship with the next release.
  - Pins that must not move have a rule with the reason: TypeScript 5 in
    `web/codegen`, Python below 3.15 for the tools.
  - mise itself, in the workflows, moves with its checksum.
- **Known vulnerabilities:** `govulncheck` for Go (only code that is
  reached counts) and `npm audit` at high severity (`make audit`).
- **Licences** (`make licenses`, osv-scanner): every Go and npm
  dependency must be under a permissive licence or MPL-2.0. Copyleft
  licences such as the GPL would limit any later change of the project's
  licence to GPL-compatible ones; the list is `LICENSES` in the
  `Makefile`.
  osv-scanner checks only licences here (`osv-scanner-licenses.toml`);
  known vulnerabilities are left to `make audit`.
- **Secrets:** gitleaks scans the staged changes in the pre-commit hook
  and the whole history in CI (`make secrets`), and prints each finding
  with the secret redacted. Rotate a secret that was real. A finding in
  a pull request's own commits comes out of the branch (amend or rebase,
  then force-push it): the squash merge gives `main` new commits, so
  ignoring it by fingerprint would not last. A finding already on
  `main` stays in its history, so its fingerprint, which names the
  commit, goes in `.gitleaksignore`; otherwise it fails every pull
  request.
- **CodeQL** scans Go, the TypeScript and the workflows on every pull
  request and weekly; the ruleset blocks merging a new error or a
  high-severity alert.
- **Vulnerability reports** come in privately, as GitHub's private
  security advisories (`SECURITY.md`); the settings script turns private
  vulnerability reporting on. The fix can be prepared and reviewed in
  the advisory's private fork, but it lands as an ordinary `fix:` pull
  request: the advisory's merge button skips CI and the ruleset. The
  advisory is published once the release that ships the fix is out.

## Go

### Versions and tools

- **Go:** the latest stable release, in the `go` directive of `go.mod` and
  in `mise.toml`.
- **golangci-lint v2**, pinned in `mise.toml` and configured in
  `.golangci.yml`. It runs staticcheck and the other linters below, and
  formats with gofumpt and goimports.

```sh
mise install           # Go, Node and golangci-lint at the pinned versions
golangci-lint run      # lint, formatting included
golangci-lint fmt      # format in place
go test -race ./...
```

Without mise activated in the shell, prefix the commands with
`mise exec --`. Files behind the `webui` build tag are linted once the web
app is built: `golangci-lint run --build-tags webui` after `npm run build`
in `web/`.

### Lint rules

- **The standard set** (errcheck, govet, ineffassign, staticcheck, unused),
  with every govet analyzer except two:
  - `fieldalignment`: field order is for readers, not for saving a few
    bytes;
  - `shadow`: `if err := …; err != nil` inside a function that already has
    an `err` is idiomatic Go. Shadowing anything else is still a smell;
    rename instead.
- **Bugs:** bodyclose, contextcheck, errorlint, exhaustive (a `default`
  case counts), fatcontext, forcetypeassert, gosec, nilerr, noctx,
  rowserrcheck, sqlclosecheck.
- **Idioms and consistency:**
  - modernize, for newer language and library features;
  - revive;
  - gocritic with its diagnostic, style and performance checks, except
    `hugeParam` (small values such as `config.Config` are passed by value)
    and `unnamedResult`;
  - cyclop: complexity of at most 15 per function;
  - errname, sloglint (see Logging below), usestdlibvars, usetesting,
    thelper, tparallel, unparam, misspell, canonicalheader, exptostd, iface
    and recvcheck.
- **Formatting:** gofumpt with grouped parameters (`func(a, b string)`)
  and no naked returns; goimports with this module's imports in their
  own group.
- **`//nolint`** only for a linter that is wrong in that place. It names
  the linter and says why (nolintlint enforces both):

  ```go
  f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // G304: the operator chooses the database path
  ```

### Code

- **Standard library first:** `net/http` routing, `log/slog`, `errors`,
  `database/sql`. A new dependency needs a reason the standard library
  does not cover.
- **Errors:**
  - wrap them with what was being done, using `%w`:
    `fmt.Errorf("list views: %w", err)`;
  - compare with `errors.Is` and `errors.As`, never by text;
  - messages are lower case, without final punctuation;
  - an error is either handled (logged) or returned, not both;
  - an API handler returns an error it cannot handle as
    `internalError("reading the profile failed", err)`, and does not log
    it. The answer keeps no word of it, as a file, a query or a user
    could be in it: `hideInternalErrors` drops the details of every 5xx
    and logs them once, with the operation, its route (not the
    request's path: nothing the client sent) and the user, at the
    level of the error (`logLevel`: debug for a client that went
    away). A plain error is logged the same way, at error level; a body
    that did not come whole, which huma answers with a 500 before any
    handler, at debug level (`recordBodyErrors`). A 5xx without details
    (the server is busy) is not logged.
    What answers before an operation runs (the session and token
    lookups) or outside one (OpenID Connect's browser flow, and the
    session a sign-in starts, which the two share) logs its failures
    itself and answers with a bare problem; so does a failure an
    operation handles and goes on from (an ingest's archive), with the
    user.
- **`context.Context`** is the first parameter of anything that queries
  the database, talks to the network or waits on other goroutines, and is
  never stored in a struct. File system calls take none, as `os` has no
  use for one.
- **No global mutable state.** Dependencies are passed in: `cmd/tdeemon`'s
  `run` takes the arguments, the environment lookup and the writers. The
  one exception is `slog.SetDefault` in `run`: whatever a dependency logs
  through `slog`'s default logger or the standard `log` package then
  comes out in the configured format. Our own code takes its logger as a
  parameter.
  Package-level variables only hold read-only values Go cannot declare
  as constants (sentinel errors, compiled regular expressions, embedded
  files, fixed lookup tables) and the version set at build time. The one
  exception is a limit of the whole process: `internal/auth`'s hashing
  slots, which every password check in the process shares; a test that
  takes them all does not run in parallel.
- **Doc comments** on everything exported: what it does, and why when that
  is not obvious.
- **Logging** with `log/slog`: a constant message, key-value pairs with
  snake_case keys (`"data_dir", dir`), no `fmt.Sprintf` in the message.
- **Database:** the schema and migration rules are in
  [`internal/store/migrations/README.md`](../internal/store/migrations/README.md).
- **Every write of a user's data is an event:** a store method that
  writes takes the `event.Actor` (the user or an import, the client, the
  model) and records an event in the same transaction, with the state
  before and after, which undo puts back, but for the changes of the
  plan: those undo leaves are listed in `undoLeaves`, and have no way
  back. A write that changes nothing (a
  retry, a merge of nothing new) records nothing. A new kind of write
  adds its `event.Action`, the operation to the `events` table's CHECK,
  and its way back to `takeBack` and the reading of its state to
  `stateNow` (what undo compares with the state the change left), or,
  for a change of the plan, its place in `undoLeaves` and in their empty
  `case`. The state an
  event records must be what `stateNow` reads back, so the two compare
  alike: read it back in the transaction, or normalise it as the table
  does (-0 to 0, a time to the second in UTC). A way back that clashes
  with what came since returns a `clash` with its `UndoReason` (a new
  kind of clash is a new reason), and a way back and the reading in
  `stateNow` read the change's own data through `decode`, `entityID` and
  `entityDay`. Those failures, a clash, and a constraint the state
  before breaks now pass the change over; any other error, a bug
  included, is returned and passes over nothing.
- **One user's data stays theirs:**
  - every store method that reads or writes a user's rows takes the
    user's ID as a parameter and filters by it (`WHERE user_id = ?`).
    The compiler makes sure the ID is passed rather than hidden in a
    context; `TestIsolation` makes sure the queries the operations reach
    use it. Writes always filter by the user, also those keyed by a
    secret's hash (touching or ending a session). The only reads without
    it are how a request learns its user (by the hash of a session's or
    an API token's secret; by the provider's subject for OpenID Connect;
    by address for the sign-in and the admin's commands), and the only
    other exception is housekeeping that shows or changes no one's data
    (removing expired sessions);
  - a handler takes the user from the request's principal
    (`auth.PrincipalFrom`), never from the path, the query or the body
    (the sign-in, which finds the user by the address it is given,
    aside);
  - another user's row answers exactly like one that does not exist
    (the same 404, or a list without it);
  - `TestIsolation` in `internal/api` runs every operation in the
    OpenAPI document as the other of two users, both ways (an admin and
    a user acting). Each case must get a 2xx from its own operation;
    then the owner's rows in the database (`users` and every table with
    a `user_id`) and what the owner sees through the API must be as they
    were, and no answer during the case (headers and body) may contain
    one of the owner's text values from the database of 8 bytes or more
    that the other user's rows do not also have. Numbers, and shorter
    text, a case checks itself. `TestIsolationCoversEveryOperation`
    fails for an operation without a case in `isolationCases`, so a new
    operation adds its case;
  - a new kind of data (foods, meals) also goes into `newIsolationEnv`:
    rows of it for both users, in every state the queries tell apart
    (used and unused, archived or not), with text unique to each user
    (as the token names are). The test fails when a table of user data
    has no rows for one of them, and for a table without a `user_id`
    that is not listed as no one's data: every table of a user's rows has
    a `user_id` column (`internal/store/migrations/README.md`);
  - routes outside the OpenAPI document are not covered: the OpenID
    Connect browser flow has its own tests (`internal/api/oidc_test.go`),
    and MCP has its own list, `mcpIsolationCases`: each tool called as
    the other party, a user and an admin, through MCP.
    `TestMCPIsolationCoversEveryTool` fails for a tool MCP serves without
    a case.

### Tests

- A change in behaviour comes with a test that fails without it.
- Table-driven tests run each case with `t.Run` and a name, and call
  `t.Parallel()` wherever they share no state.
- A test that can block has a deadline (`context.WithTimeout`, or a
  `select` with `time.After`), so a regression fails instead of hanging.
- Helpers call `t.Helper()`; one that takes `testing.TB` names it `tb`.
- Real dependencies where they are cheap: a real SQLite file
  (`storetest.Open`), `httptest` servers. No mocks of the standard
  library.
- `t.Context()`, `t.TempDir()` and `t.Setenv` instead of hand-made
  equivalents.
- `go test -race ./...` passes.

## Web app (TypeScript, React)

### Versions and tools

- **Node:** the active LTS line, pinned in `mise.toml`.
- **TypeScript 7**, the compiler rewritten in Go, for the app. `web/codegen`
  stays on TypeScript 5.9 until openapi-typescript supports 7.
- **React 19** with the React Compiler, Vite, TanStack Router and Query,
  Tailwind 4, shadcn components on Base UI.
- **oxlint** with type-aware rules (oxlint-tsgolint) and **Prettier**,
  both pinned in `web/package.json` and configured in `web/.oxlintrc.json`
  and `web/.prettierrc.json`.

```sh
cd web
npm run lint           # oxlint with type information
npm run format         # Prettier in place; format:check only checks
npm run typecheck      # tsc -b
npm test               # Vitest
```

### Why oxlint, not ESLint

- ESLint's type-aware rules come from typescript-eslint, which supports
  TypeScript below 6.1 only, and eslint-plugin-jsx-a11y does not support
  ESLint 10. oxlint's type-aware rules run on TypeScript 7, and the React,
  hooks, React Compiler, accessibility, import and Vitest rules are built
  in.
- Vite's own template lints with it, and it lints the app in about a
  second.
- The cost: its type-aware rules are still being completed, and ESLint
  plugins (its JS plugins are in alpha) are not used, so there is no
  TanStack Query plugin.

### Lint rules

- **Categories** correctness, suspicious and perf, as errors, across the
  eslint, typescript, unicorn, oxc, react, jsx-a11y, import, vitest and
  promise plugins.
- **TypeScript:** on top of those, typescript-eslint's strict-type-checked
  and stylistic-type-checked sets, with the options of strict-type-checked:
  `+` does not mix types, template literals take only strings (and `Error`
  or `URL`), and a `@ts-expect-error` needs a reason of ten characters.
  Among them:
  - no `any`, and no unsafe use of an `any` value;
  - no floating or misused promises;
  - no non-null assertions;
  - type-only imports;
  - exhaustive `switch`.
- **React:**
  - the rules of hooks and the React Compiler's rules (purity, refs,
    immutability, `setState` during render or in effects);
  - `only-export-components`, so fast refresh keeps state. It is off for
    the shadcn components in `src/components/ui`, which export their
    variants next to the component by design.
  - `react-in-jsx-scope` is off: the JSX transform made it obsolete.
- **Accessibility:** the jsx-a11y rules.
- **Imports:** no cycles, no duplicates, and side-effect imports only for
  CSS.
- **Strictness of the run:** a warning fails the run, and so does a
  disable comment that disables nothing. A disable names the rule
  (oxlint's `unicorn/no-abusive-eslint-disable`) and says why after
  `--`, on the same line (`scripts/check-disable-comments.sh`, in
  `make lint-web` and the pre-commit hook):

  ```ts
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- the API guarantees the shape
  ```

### Formatting

Prettier, with no semicolons, single quotes and 100 columns. Imports are
sorted in groups, with a blank line between groups: Node built-ins and
packages, then `@/`, then relative imports. Tailwind classes are kept in
Tailwind's order, in `className` and in the strings given to `cva` and
`cn`. The generated `src/api/schema.d.ts` is left as the
generator writes it.

### Code

- **`tsconfig`** is strict, with `noUncheckedIndexedAccess`,
  `exactOptionalPropertyTypes`, `noImplicitOverride`, `noUnused*`,
  `erasableSyntaxOnly` and `verbatimModuleSyntax`.
- **Function components and hooks only**, and current React APIs: refs are
  props, so no `forwardRef`, and no `defaultProps`.
- **Server data only through TanStack Query**, using `$api` from
  `src/api/client.ts`, which is typed from the OpenAPI spec. Components do
  not call `fetch`, and do not copy server data into `useState`.
- **Types:** `unknown` and narrowing instead of `any`.
- **One exported component per file**, kept small.
- **Imports** from another directory of `src` go through `@/`, never
  `../` (`import/no-relative-parent-imports`). `./` is for a neighbour in
  the same directory, such as `./schema` in `src/api`.
- **Text:** user-facing text goes through i18n once it is set up.

### Tests

- Vitest and Testing Library. Tests check behaviour through the DOM:
  roles and text, or test ids where nothing better identifies an element.
- The network is faked at `fetch` with `vi.stubGlobal`, not by mocking our
  own modules, and the stubs are undone after each test.
- A change in behaviour comes with a test that fails without it.
- Asynchronous UI is awaited with `findBy…` queries, never with sleeps.

## Other files

Every tool here is pinned in `mise.toml`; `mise install` brings them all.

| Files | Tool | Configuration |
| --- | --- | --- |
| everything | editorconfig-checker | `.editorconfig`, `.editorconfig-checker.json` |
| SQL migrations | sqlfluff (SQLite dialect) | `.sqlfluff`, `.sqlfluffignore` |
| `Dockerfile` | hadolint | `.hadolint.yaml` |
| GitHub Actions workflows | actionlint, zizmor | `.github/zizmor.yml` |
| YAML | yamllint | `.yamllint.yaml` |
| Markdown | markdownlint-cli2 | `.markdownlint-cli2.jsonc` |
| shell scripts (`scripts/`) | shellcheck | none |

```sh
editorconfig-checker
sqlfluff lint            # sqlfluff fix <new migration> lays one out
hadolint Dockerfile
actionlint && zizmor .  # once there are workflows
yamllint -s .
markdownlint-cli2
```

- **EditorConfig:** UTF-8, LF line endings, a final newline, no trailing
  whitespace, and two spaces of indentation. Go and Makefiles use tabs,
  and SQL uses four spaces.
  - In Markdown, list markers set the indentation.
  - The checker skips `LICENSE` (verbatim text), the generated schema
    snapshot and Go files, which gofumpt formats.
- **SQL migrations:**
  - keywords and types are upper case; identifiers and functions lower
    case. sqlfluff reads some keywords as functions or identifiers
    (`CURRENT_TIMESTAMP`, `CURRENT_DATE`, `CURRENT_TIME`, `RAISE`, `CAST`
    and the collations `NOCASE`, `BINARY` and `RTRIM`); they may stay
    upper case. A trigger's `new.` and `old.` are names like any other,
    in lower case;
  - in a trigger body, sqlfluff takes `new.x` in a `SELECT` without
    `FROM` for an unknown table (RF01). Put the condition in the
    trigger's `WHEN`, or mark that line `-- noqa: RF01`;
  - four-space indentation, at most 100 columns;
  - `sqlfluff fix` lays out a **new** migration. A merged migration is
    never rewritten, not even to its layout: tdeemon would warn that its
    checksum no longer matches. If a newer sqlfluff flags an old
    migration, list that file in `.sqlfluffignore` with the reason;
  - sqlfluff's SQLite dialect cannot parse `SAVEPOINT`, `RELEASE` or
    `ROLLBACK TO`, which the runner allows, nor the arguments of an FTS5
    table. A migration that needs them goes into `.sqlfluffignore` too,
    kept to the statements sqlfluff cannot read, so the others stay
    linted;
  - the file name and the schema rules are in
    [`internal/store/migrations/README.md`](../internal/store/migrations/README.md);
  - every migration runs from an empty database in the tests
    (`TestSchema`);
  - the generated `internal/store/testdata/schema.sql` is not linted.
- **Dockerfile:** hadolint at its strictest threshold (style), with Docker
  Hub and Google's registry as the only trusted registries. Base images
  are also fully qualified and pinned by digest; hadolint does not check
  that, so the reviewer does.
- **GitHub Actions:**
  - actionlint checks the workflows;
  - zizmor audits them for security, for example credentials left in
    the checkout or permissions broader than the job needs;
  - every action, GitHub's own included, is pinned to a full commit SHA
    (zizmor's `unpinned-uses` with the `hash-pin` policy), with the
    version in a comment, the form Renovate reads and writes:

    ```yaml
    - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
    ```

- **YAML:** yamllint's default rules, without the `---` document start,
  with lines up to 120 columns, `on:` allowed as a key, and one space
  before a comment, as in the action pins.
- **Markdown:** markdownlint's default rules.
  - Prose wraps at 80 columns; tables, code and headings may run longer.
  - Headings are real headings, not bold lines.
  - Fenced code blocks name their language (`text` for plain text).
- **Shell scripts:** bash with `set -euo pipefail`, checked by shellcheck.
