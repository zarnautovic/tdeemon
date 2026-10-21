# tdeemon

Self-hosted, AI-first nutrition tracking with an adaptive TDEE.

Log meals by chat or photo from any AI client (via MCP) or the built-in
chat; tdeemon does the math, learns your real energy expenditure from
intake and weight trend, and lets an AI coach adjust your goals — cut,
maintenance, bulk or recomp — within guardrails you control.

> **Status: early development.** Only the server skeleton, the database
> layer, the API contract and an empty web app exist so far.
> The full design lives in [`docs/DESIGN.md`](docs/DESIGN.md).

## Planned highlights

- One container, one SQLite file, multi-user (household or friends).
- MCP server: use Claude, ChatGPT, a local model or any MCP client.
- Built-in chat with your own API key, photo logging, PWA.
- Apple Health import (Health Auto Export), later native HealthKit.
- Adaptive calibration with confidence intervals; AI changes plans only
  through proposals you accept, and limits are enforced in code.

## Development

The toolchain and every linter are pinned in `mise.toml`; install them
with [mise](https://mise.jdx.dev). `make check` runs what CI checks,
except a few checks that only CI runs, and the git hooks run the fast part
on each commit. The rules for code and tests, and the CI-only checks, are
in [docs/conventions.md](docs/conventions.md), and
[CONTRIBUTING.md](CONTRIBUTING.md) says how to contribute. Report a
security vulnerability privately, as [SECURITY.md](SECURITY.md) describes.

```sh
mise install
lefthook install    # git hooks, once per clone
make check          # make fmt formats, make help lists the targets

TDEEMON_BASE_URL=http://localhost:8080 \
TDEEMON_SECRET_KEY="$(openssl rand -base64 32)" \
TDEEMON_LOG_FORMAT=text \
go run ./cmd/tdeemon
```

### Web app

The web app in `web/` (React, TypeScript, Vite) is compiled into the
binary only with the `webui` build tag, so Go builds and tests never need
Node; a binary built without the tag serves a short notice at `/`. To work
on the app, run the server as above and the Vite dev server next to it,
which forwards `/api` to the server (`TDEEMON_DEV_SERVER`, default
`http://localhost:8080`):

```sh
cd web && npm ci
npm run dev     # http://localhost:5173
npm test        # Vitest
npm run lint    # oxlint; npm run format for Prettier
```

A binary with the app inside:

```sh
(cd web && npm ci && npm run build)   # writes internal/webui/dist
go build -tags webui ./cmd/tdeemon
```

### API contract

The JSON API under `/api/v1` is described by an OpenAPI 3.1 document that
is generated from the Go handlers in `internal/api` (with
[huma](https://huma.rocks)). It is served at `/api/v1/openapi.json` and
committed as `api/openapi.json`, and the TypeScript types in
`web/src/api/schema.d.ts` are generated from it. `go test` fails when
`api/openapi.json` is stale, and `npm run check:api` in `web/codegen` when
the types are stale or no longer type-check. After changing the API:

```sh
go test ./internal/api -run TestOpenAPISpec -update
cd web/codegen && npm ci && npm run gen:api
```

## Configuration

All settings come from environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `TDEEMON_BASE_URL` | — (required) | public URL of the instance, without a path (tdeemon is served at the root of its host), e.g. `https://tdeemon.example.com` |
| `TDEEMON_SECRET_KEY` | — (required) | at least 32 bytes; signs sessions and encrypts stored secrets |
| `TDEEMON_LISTEN` | `:8080` | address the HTTP server binds |
| `TDEEMON_DATA_DIR` | `./data` | database (`tdeemon.db`), backups and archives; keep it on a local disk, never NFS |
| `TDEEMON_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `TDEEMON_LOG_FORMAT` | `json` | `json` or `text` |
| `TZ` | system | default time zone of the instance |

Every invalid or missing variable is reported at startup in one go.

## Docker

The image holds one static binary with the web app inside, on distroless,
and runs as an unprivileged user (uid 65532). Everything it writes goes to
one volume at `/data`, so the root filesystem can be read-only.

```sh
docker build -t tdeemon .

# Once: the settings, readable by you only. Keep the secret key: it signs
# sessions and encrypts stored secrets. set -C refuses to replace an
# existing .env, so running this again keeps the key.
(umask 077 && set -C && printf 'TDEEMON_BASE_URL=%s\nTDEEMON_SECRET_KEY=%s\n' \
  http://localhost:8080 "$(openssl rand -base64 32)" > .env)

docker run --read-only --stop-timeout 20 -v tdeemon-data:/data \
  -p 8080:8080 --env-file .env tdeemon
```

- `--env-file` keeps the key out of the command line, where other users
  could see it in the process list; `.env` is ignored by git and by the
  image build.
- Mount a volume at `/data`. The image declares none on purpose: without
  one, a read-only container stops at startup instead of keeping the
  database where it gets lost with the container. A named volume takes the
  right ownership; a host directory must be writable by uid 65532 (with
  rootless Podman, add `:U` to the mount, e.g. `-v ./data:/data:U`).
- Nothing else needs to be writable, so `--read-only` (or
  `readOnlyRootFilesystem` in Kubernetes) works without a tmpfs.
- On stop the server gives in-flight requests up to 15 seconds;
  `--stop-timeout 20` waits for that, where the default of 10 would kill
  the process first.
- `--build-arg VERSION=v1.2.3` sets the version the server reports
  (default `dev`). The binary takes no arguments; with any it prints its
  usage and exits, without touching the database.
- The build runs `go vet` and the tests against the web app it embeds.
  Podman builds the same file.

## Data

tdeemon keeps everything in one SQLite file, `TDEEMON_DATA_DIR/tdeemon.db`,
which it creates readable by its owner only, and applies database
migrations at startup. Before it migrates a database that already has
data, it saves a snapshot to
`TDEEMON_DATA_DIR/backups/pre-migrate-<version>-<time>.db`. It keeps the
newest snapshot of each schema version, for the three newest versions.

To restore a snapshot:

1. Stop tdeemon.
2. Move `tdeemon.db`, `tdeemon.db-wal` and `tdeemon.db-shm` aside
   together, for example into a `before-restore/` folder. After an
   unclean stop the `-wal` file holds the latest committed changes, so
   the three files are only a complete copy of the current database
   together. Left next to the restored file, SQLite would replay the old
   `-wal` onto it.
3. Copy the snapshot to `tdeemon.db`.
4. Start tdeemon. A release newer than the snapshot migrates it again; to
   stay on the snapshot's schema, run the release that shipped it.

## Disclaimer

tdeemon is not medical advice. Talk to a professional before making
significant changes to your diet or training.

## License

[AGPL-3.0-only](LICENSE). Contributions come under the same licence, with no
CLA to sign ([CONTRIBUTING.md](CONTRIBUTING.md)).
