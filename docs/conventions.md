# Conventions

The rules every change follows. Tools check what they can; the rest is
for the author and the reviewer.

## Running the checks

- **`make check`** runs every check CI runs:
  - the linters and format checks for every language;
  - the Go and web tests, with coverage, and the Go tests behind the
    `webui` tag against a fresh build;
  - the check that the generated TypeScript types match the API spec;
  - `govulncheck` and `npm audit`.
  `make fmt` formats Go and the web app in place, and `make help` lists
  the targets. The tools run through mise, at the versions in
  `mise.toml`.
- **CI** (`.github/workflows/ci.yml`) runs the same targets on every pull
  request and on `main`, in four parallel jobs: Go, the web app, the
  other files, and the Docker image, which is built with its tests but not
  pushed. Only pushes to `main` write the caches; pull requests read
  them.
- **`mise.lock`** pins each tool's version and, for the tools mise
  downloads as binaries, the URL and checksum for Linux and macOS on x64
  and arm64; `.mise/locks/` pins the dependencies of the Python and npm
  tools. Go tools are built from source and checked against the Go
  checksum database. CI installs with `--locked`. After changing a
  version in `mise.toml`, run
  `mise lock --platform linux-x64,linux-arm64,macos-arm64,macos-x64`.
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

## Commits and pull requests

- Every change reaches `main` through a pull request and is squash-merged,
  so the pull request's title becomes the commit on `main`.
- **Titles and commit messages are Conventional Commits:**
  `type(scope)!: summary`.
  - The types are feat, fix, perf, refactor, test, docs, ci, build, chore,
    style and revert.
  - The scope is optional.
  - `!` after the type marks a breaking change. A `BREAKING CHANGE:`
    footer counts only in the last paragraph of the pull request's
    description, which becomes the commit's body; footers in the branch's
    commits are lost in the squash.
  - The summary is short and ends without a full stop.
  - `feat` and `fix` decide the next release.
  - GitHub's Revert button titles a pull request `Revert "feat: x"`;
    rename it to `revert: feat: x`.
- **The checks:** the "PR title" workflow runs `cog verify` (cocogitto,
  `cog.toml`) on every pull request title, and the commit-msg hook runs it
  on every commit message. Merge and fixup commits pass the hook, since
  the squash merge replaces them; a title starting with "Merge" does not
  pass.

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
  - an error is either handled (logged) or returned, not both.
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
  files, fixed lookup tables) and the version set at build time.
- **Doc comments** on everything exported: what it does, and why when that
  is not obvious.
- **Logging** with `log/slog`: a constant message, key-value pairs with
  snake_case keys (`"data_dir", dir`), no `fmt.Sprintf` in the message.
- **Database:** the schema and migration rules are in
  [`internal/store/migrations/README.md`](../internal/store/migrations/README.md).

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
  disable comment that disables nothing. A disable names the rule and
  says why after `--`:

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
| shell scripts | shellcheck | none |

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
  - `sqlfluff fix` lays out a **new** migration. An applied migration is
    never rewritten, not even to its layout: the runner would report it
    as changed. If a newer sqlfluff flags an old migration, list that file
    in `.sqlfluffignore` with the reason;
  - sqlfluff's SQLite dialect cannot parse `SAVEPOINT`, `RELEASE` or
    `ROLLBACK TO`, which the runner allows. A migration that needs them
    goes into `.sqlfluffignore` too;
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
- **Shell scripts:** shellcheck, when the repo gets any.
