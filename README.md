# tdeemon

Self-hosted, AI-first nutrition tracking with an adaptive TDEE.

Log meals by chat or photo from any AI client (via MCP) or the built-in
chat; tdeemon does the math, learns your real energy expenditure from
intake and weight trend, and lets an AI coach adjust your goals — cut,
maintenance, bulk or recomp — within guardrails you control.

> **Status: early development.** Only the server skeleton and the
> database layer exist so far.
> The full design lives in [`docs/DESIGN.md`](docs/DESIGN.md).

## Planned highlights

- One container, one SQLite file, multi-user (household or friends).
- MCP server: use Claude, ChatGPT, a local model or any MCP client.
- Built-in chat with your own API key, photo logging, PWA.
- Apple Health import (Health Auto Export), later native HealthKit.
- Adaptive calibration with confidence intervals; AI changes plans only
  through proposals you accept, and limits are enforced in code.

## Development

The toolchain is pinned in `mise.toml`; install it with
[mise](https://mise.jdx.dev):

```sh
mise install
go test -race ./...

TDEEMON_BASE_URL=http://localhost:8080 \
TDEEMON_SECRET_KEY="$(openssl rand -base64 32)" \
TDEEMON_LOG_FORMAT=text \
go run ./cmd/tdeemon
```

## Configuration

All settings come from environment variables:

| Variable | Default | Meaning |
|---|---|---|
| `TDEEMON_BASE_URL` | — (required) | public URL of the instance, e.g. `https://tdeemon.example.com` |
| `TDEEMON_SECRET_KEY` | — (required) | at least 32 bytes; signs sessions and encrypts stored secrets |
| `TDEEMON_LISTEN` | `:8080` | address the HTTP server binds |
| `TDEEMON_DATA_DIR` | `./data` | database (`tdeemon.db`), backups and archives; keep it on a local disk, never NFS |
| `TDEEMON_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `TDEEMON_LOG_FORMAT` | `json` | `json` or `text` |
| `TZ` | system | default time zone of the instance |

Every invalid or missing variable is reported at startup in one go.

## Data

tdeemon keeps everything in one SQLite file, `TDEEMON_DATA_DIR/tdeemon.db`,
and applies database migrations at startup. Before it migrates a database
that already has data, it saves a snapshot to `TDEEMON_DATA_DIR/backups/`
and keeps the three newest.

## Disclaimer

tdeemon is not medical advice. Talk to a professional before making
significant changes to your diet or training.

## License

[AGPL-3.0](LICENSE). Contributions require signing a CLA (set up before
the first external contribution).
