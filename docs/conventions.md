# Conventions

The rules every change follows. Tools check what they can; the rest is
for the author and the reviewer. The web app (TypeScript, React) and the
other files (SQL, Docker, workflows) get their sections with their own
tooling.

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
