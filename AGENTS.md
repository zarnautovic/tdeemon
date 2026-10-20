# AGENTS.md

Instructions for AI coding agents working on tdeemon. Humans follow the
same rules; the full versions are in
[docs/conventions.md](docs/conventions.md), and the design is in
[docs/DESIGN.md](docs/DESIGN.md).

## The project

tdeemon is a self-hosted nutrition tracker with an adaptive TDEE: one Go
binary with SQLite and a React web app inside, an MCP server and a REST
API. It is early: the skeleton, the database layer, the API contract and
an empty web app exist.

| Path | What |
| --- | --- |
| `cmd/tdeemon` | the binary: configuration, wiring, start and stop |
| `internal/config` | settings from `TDEEMON_*` environment variables |
| `internal/store` | SQLite access and the migrations in `migrations/` |
| `internal/api` | the HTTP API (huma), which writes `api/openapi.json` |
| `internal/server` | the HTTP server and graceful shutdown |
| `internal/webui` | serves the built web app (build tag `webui`) |
| `web/` | the React app (Vite, TypeScript 7, TanStack, Tailwind) |
| `web/codegen` | generates `web/src/api/schema.d.ts` from the spec |

## Commands

```sh
mise install       # every tool at the pinned version (mise.toml, mise.lock)
lefthook install   # git hooks, once per clone
make check         # everything CI checks; run it before you finish
make fmt           # format Go, the web app and SQL
make help          # the other targets
```

Run the server with `TDEEMON_BASE_URL`, `TDEEMON_SECRET_KEY` and
`go run ./cmd/tdeemon`; the README has the details.

## Rules

- **Never commit to `main`.** Work on a branch and open a pull request.
  It is squash-merged, so its title, a Conventional Commit
  (`feat: …`, `fix(store): …`, `!` for breaking changes), becomes the
  commit on `main`.
- **No AI attribution** in commits or pull requests: no `Co-Authored-By`
  trailers and no "Generated with" lines.
- **No personal data in the repo, ever.** The repo is public. Real
  weights, meals, health exports and chat sessions stay out of git; test
  fixtures are made up or anonymised. No secrets either.
- **Tests come with every change in behaviour**, and they fail without
  it. Use a real SQLite file (`storetest.Open`), not mocks of the
  standard library. Coverage floors are in `.testcoverage.yml` and
  `web/vite.config.ts`.
- **Generated files stay fresh:**
  - after changing the API, regenerate `api/openapi.json`
    (`go test ./internal/api -update`) and the TypeScript types
    (`cd web/codegen && npm run gen:api`);
  - after a migration, refresh the schema snapshot
    (`go test ./internal/store -run TestSchema -update`).
- **Migrations only go forward.** Never edit an applied migration; add a
  new one. The rules are in `internal/store/migrations/README.md`.
- **Follow the linters, do not silence them.** A `//nolint` (Go) or
  `oxlint-disable` (web) names the rule and gives the reason.
- **Code, comments, docs and commit messages are in English.**
- **Keep the docs true:** update the README, docs/conventions.md or
  docs/DESIGN.md when a change makes them wrong.

## Definition of Done

A change is done when its behaviour is tested, `make check` passes,
generated files and docs are up to date, nothing personal entered the
repo, and the pull request says what changed, why and how it was
checked.
