# tdeemon

Self-hosted, AI-first nutrition tracking with an adaptive TDEE.

Log meals by chat or photo from any AI client (via MCP) or the built-in
chat; tdeemon does the math, learns your real energy expenditure from
intake and weight trend, and lets an AI coach adjust your goals — cut,
maintenance, bulk or recomp — within limits enforced in code: you and
the admin may tighten them, and loosen some within a floor nobody can
lower.

> **Status: early development.** Only the server skeleton, the database
> layer, the API contract and the web app's frame with the day, the
> foods, the charts and the settings exist so far.
> The full design lives in [`docs/DESIGN.md`](docs/DESIGN.md).

## Planned highlights

- One container, one SQLite file, multi-user (household or friends).
- MCP server: use Claude, ChatGPT, a local model or any MCP client.
- Built-in chat with your own API key, photo logging, PWA.
- Apple Health import (Health Auto Export), later native HealthKit.
- Adaptive calibration with confidence intervals; AI changes plans only
  through proposals you accept, and limits are enforced in code
  ([docs/limits.md](docs/limits.md), with their sources; not medical
  advice).

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

The end-to-end tests run the app in Chromium against the binary built
with it: `make e2e-browser` once, then `make e2e`.

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
| `TDEEMON_SECRET_KEY` | — (required) | at least 32 bytes; seals the single sign-on state, and will encrypt stored secrets (sessions do not depend on it) |
| `TDEEMON_LISTEN` | `:8080` | address the HTTP server binds |
| `TDEEMON_DATA_DIR` | `./data` | database (`tdeemon.db`), backups and archives; keep it on a local disk, never NFS |
| `TDEEMON_BACKUP_KEEP` | `7` | how many daily snapshots of the database to keep in `TDEEMON_DATA_DIR/backups`, up to 366; `0` takes none ([Data](#data)) |
| `TDEEMON_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `TDEEMON_LOG_FORMAT` | `json` | `json` or `text` |
| `TDEEMON_OIDC_ISSUER` | — | issuer URL of an OpenID Connect provider for single sign-on (https; http only for localhost) |
| `TDEEMON_OIDC_CLIENT_ID` | — | tdeemon's client ID there; required with the issuer |
| `TDEEMON_OIDC_CLIENT_SECRET` | — | its client secret, for a confidential client |
| `TDEEMON_OIDC_NAME` | `Single sign-on` | what the sign-in page calls the provider |
| `TDEEMON_TRUSTED_PROXIES` | — | reverse proxies whose `X-Forwarded-For` names the client, as addresses or CIDR ranges (`10.42.0.0/16, fd00::/8`); set it behind a proxy, or the limits on failed sign-ins count the proxy's address, that is everyone together |
| `TDEEMON_LIMIT_<NAME>` | the column "Hard limit" of [docs/limits.md](docs/limits.md) | the instance's limits of a plan, such as `TDEEMON_LIMIT_KCAL_FLOOR_FEMALE=1000` for a clinician's patient; the names are in docs/limits.md, "The names of the limits". Never below the floor in the code, and some only tighter |
| `TDEEMON_TELEGRAM_BOT_TOKEN` | — | the token of a Telegram bot (from @BotFather) that sends users their notifications and answers their commands ([Telegram](#telegram)) |
| `TDEEMON_TELEGRAM_API_URL` | `https://api.telegram.org` | the Bot API, Telegram's or a [Bot API server](https://github.com/tdlib/telegram-bot-api) of your own; `http` only for one in your network, as the token is in every request |
| `TZ` | system | default time zone of the instance |

Every invalid or missing variable is reported at startup in one go.

## Docker

The image holds one static binary with the web app inside, on distroless,
and runs as an unprivileged user (uid 65532). Everything it writes goes to
one volume at `/data`, so the root filesystem can be read-only. Each
release publishes it as `ghcr.io/zarnautovic/tdeemon:X.Y.Z` (also `:X.Y`),
for linux/amd64 and linux/arm64; or build it yourself:

```sh
docker build -t tdeemon .

# Once: the settings, readable by you only. Keep the secret key: it seals
# the single sign-on state and will encrypt stored secrets. set -C refuses
# to replace an existing .env, so running this again keeps the key.
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
  (default `dev`). Without arguments the binary runs the server, and
  `user create` adds a user ([Users](#users)); anything else prints the
  usage and exits without touching the database.
- The build runs `go vet` and the tests against the web app it embeds.
  Podman builds the same file.

## Users

tdeemon has no sign-up page: an admin adds users with the binary itself.
It works on the database of a server that has started at least once,
which creates and migrates it, and the server may keep running:

```sh
docker exec <container> /tdeemon user create --admin \
  --email you@example.com --timezone Europe/Zagreb
# or, in Kubernetes:
kubectl exec deploy/tdeemon -- /tdeemon user create --admin --email you@example.com
```

- It prints a generated password once; keep it in a password manager.
  To choose the password instead, add `--password-stdin` and pass it on
  the first line of standard input (`docker exec -i`).
- A password has at least 15 characters, as NIST SP 800-63B-4 advises
  when it is the only factor, and at most 256. tdeemon stores only its
  argon2id hash.
- `--name` sets the name the app shows (by default, the part of the
  address before the @), `--timezone` the user's time zone (by default
  `TZ`, or UTC), and `--locale` their language, `en` or `hr` (by default
  `en`); `tdeemon user set-locale --email … --locale hr` changes it later.
  `tdeemon help` lists the flags.
- `tdeemon user set-password --email …` gives a user a new password,
  generated or with `--password-stdin`, and ends their sessions: the way
  back when a password is lost. Their API tokens keep working; if they
  may have leaked too, revoke them (see API tokens below).
- The commands read the same `TDEEMON_*` variables as the server, and
  they refuse a database the server has not migrated to their version.

Signing in goes through the web app's sign-in page (`/login`), with a
password or the OpenID Connect provider, or straight through the API
(`POST /api/v1/auth/login`); either sets an HttpOnly session cookie for
30 days. Cross-origin browser requests that change something are
refused (Go's `http.CrossOriginProtection`), and over HTTPS the cookie is
a `__Host-` cookie.

Against guessing and floods:

- **Failed sign-ins:** after 10 in 15 minutes from one client address
  (an IPv6 address counts by its /64), or for one account (from
  anywhere, and the same for an address no account has), sign-ins from
  there or for it answer 429 with `Retry-After` until the 15 minutes
  have passed, the right password included. Sign-ins still being checked
  count too, so a burst sent at once gets no further. A successful
  sign-in starts the account's count over. Single sign-on is not
  counted.
- **Bad API tokens:** after 50 in 15 minutes from one address, every
  request with a Bearer token from there answers 429, a good token
  included, until the 15 minutes have passed.
- **Behind a reverse proxy**, set `TDEEMON_TRUSTED_PROXIES` to its
  addresses, so the limits count the client it forwards
  (`X-Forwarded-For`, the right-most address that is not a trusted
  proxy) and not the proxy; tdeemon logs a warning when forwarded
  requests come in without it. The counts live in memory, at most 100,000
  addresses or accounts per limit (past that, new ones go uncounted
  until older ones expire, and up to 14 seconds more), and start over
  when the server restarts.
- **Password hashing** runs at most one hash per CPU at a time; more
  sign-ins at once wait their turn instead of adding 19 MiB each, for up
  to 10 seconds, then answer 503 with `Retry-After`; such a sign-in does
  not count as a failure. A client that goes away while it waits gives up
  its place; one that leaves while its password is being checked counts
  as a failed sign-in.
- **Security headers** on every response: a content security policy
  that loads nothing from other sites and forbids framing
  (`default-src 'self'; frame-ancestors 'none'`), `nosniff`,
  `Referrer-Policy: no-referrer`, and HSTS over HTTPS.

### Telegram

tdeemon sends a user's notifications to a Telegram chat through a bot of
the instance, in the user's language (English, or Croatian for `hr`), and
answers the commands sent to it there. It reads the bot's messages by
asking Telegram for them (long polling), so no link from Telegram
reaches it; no other program may poll the same bot.

1. Create a bot with [@BotFather](https://t.me/BotFather) (`/newbot`) and
   set its token as `TDEEMON_TELEGRAM_BOT_TOKEN`.
2. The user opens the bot in Telegram and presses **Start**: a bot may
   only write to someone who has written to it first.
3. Find the chat's id, the user's id in Telegram for a private chat: a
   running tdeemon logs it for a message from a chat that is no user's
   (`a message to the Telegram bot from a chat that is no user's`,
   `chat_id`), such as the **Start** above, or in a group a command
   (`/start@<bot>`). Without one running, the bot's updates have it
   (`https://api.telegram.org/bot<token>/getUpdates`, `message.chat.id`).
4. Set it for the user:

   ```sh
   kubectl exec deploy/tdeemon -- /tdeemon user set-telegram \
     --email you@example.com --chat-id 123456789
   ```

   The chat gets a message first; a chat that does not take it (a wrong
   id, a bot not started) is not set, and neither is one while Telegram
   does not answer. `--chat-id 0` removes it.

**The morning summary** comes at 10:00 on the user's clock (or later
that morning, if the server was down at 10:00), once a day, without an
LLM. In the user's language (English, or Croatian for `hr`), it has a
line for each of these that came:

- last night's sleep (the night that ended this morning, never the one
  before, whatever hour the user's day starts);
- this morning's weight, the trend weight and body fat;
- yesterday against its target: kcal, protein, carbs and fat, the day's
  type, and whether it is excluded; or that nothing was logged;
- yesterday's steps, active kcal and exercise;
- calibration's verdict in a line: why there is none, or the rate
  against the target and what it proposes;
- recovery's flags, with their averages.

A value that did not come is left out; nothing is guessed.

**Commands**, in the user's chat with the bot, which Telegram's menu
lists:

- `/sazetak` or `/summary`: the morning summary, as it is now;
- `/pomoc`, `/help` or `/start`, or, in a private chat, any other
  message: the commands.

The answer is in the user's language, and in a group set for several
users, one for each; a group's other messages get none. A chat that is
no user's gets no answer, and neither does a command older than 5
minutes (one sent while the server was down). A command changes
nothing: the morning summary still comes at 10:00.

**The watchdog** looks each hour and tells the user, in the same chat,
when their data stop coming:

- no new or changed metric for more than 26 hours: a day of syncs
  missed, as a day of a user who moves changes its steps;
- an ingest token unused for more than 3 days: the device that sends
  with it has stopped. Every ingest token counts as a daily device's, so
  one that sends only now and then (a Strong export twice a week) alarms
  after each pause, until the alarms have settings.

Each stop alarms once (and a chat that blocked the bot, not at all); it
alarms again only after data have come and stopped again. A group chat
that Telegram turns into a supergroup is followed to its new id, at the
change's message or at the next send.

### Single sign-on (OpenID Connect)

With `TDEEMON_OIDC_*` set, users can sign in through a provider such as
Authentik or Keycloak instead of a password (the password keeps working,
for when the provider is down):

1. At the provider, create an OAuth2/OpenID Connect application: a
   confidential client with the redirect URI
   `<TDEEMON_BASE_URL>/api/v1/auth/oidc/callback` and the scopes
   `openid`, `email` and `profile`. In Authentik the issuer is
   `https://<authentik>/application/o/<slug>/`.
2. Make the provider say whether an address is verified
   (`email_verified`): tdeemon links an account by its address only when
   it is. Authentik sends `false` by default since 2025.10. Its guide
   ([Verify email addresses](https://docs.goauthentik.io/add-secure-apps/providers/oauth2/verify-email-address))
   records the address a user confirmed through an email link in the
   `email_verified_address` attribute; then a scope mapping
   (Customization → Property Mappings → Scope Mapping, scope name
   `email`), chosen for the provider instead of the default one, vouches
   for exactly that address:

   ```python
   email = request.user.email
   verified_address = request.user.attributes.get("email_verified_address")
   return {
       "email": email,
       "email_verified": bool(email and verified_address == email),
   }
   ```

   An address that changes afterwards (by the user, an admin or a source
   sync) is no longer vouched for until it is confirmed again, unless it
   returns to the address confirmed before; to require a new
   confirmation then too, clear the attribute on every change, as the
   guide says. If only the admin sets addresses, `"email_verified": True`
   does instead.
   Authentik's own warning applies: do not assert verified addresses
   that users can change freely, or one of them could take over another's
   tdeemon account at the first sign-in.
3. Create the user in tdeemon (`tdeemon user create --email …`) with the
   address they have at the provider. Nobody gets an account by signing
   in.
4. The first sign-in links the provider's account to that user, if the
   provider says the address is verified, the address matches exactly
   (ASCII letter case aside) and the user has no other account at that
   provider; after that the provider's subject identifies them, whatever
   the address becomes. A refused sign-in says why, and the log records
   the reason. There is no command yet to undo a link.

The browser starts at `/api/v1/auth/oidc/login` (add `?return_to=/path`
to land somewhere else on the instance); `GET /api/v1/auth/methods` says
whether the instance has a provider. The flow uses PKCE, a nonce and a
state bound to the browser in a short-lived sealed cookie. tdeemon finds
the provider on the first sign-in, so it starts while the provider is
down; after a failure it waits 30 seconds before asking again. The
provider's endpoints must use https, like its issuer.

### API tokens

Devices and clients act for a user with an API token instead of a
session: the phone (Health Auto Export) sends health data, an AI client
logs meals through MCP. A token has scopes: `ingest` (health data from
the phone), `tools` (the tools: read, log and change, through MCP) and
`read` (the tools that only read).

```sh
# Signed in (the session cookie in cookies.txt), create one; the token is
# shown only in this answer, tdeemon keeps only its hash.
curl -b cookies.txt -H 'Content-Type: application/json' \
  -d '{"name":"iPhone","scopes":["ingest"]}' https://tdeemon.example.com/api/v1/tokens

# The device sends it with every request:
curl -H 'Authorization: Bearer tdm_…' https://tdeemon.example.com/api/v1/auth/whoami
```

`GET /api/v1/tokens` lists them (name, scopes, when last used),
`DELETE /api/v1/tokens/{id}` revokes one at once, and
`DELETE /api/v1/tokens` all of them, as after a password reset. A token
reaches only
the operations open to its scopes: `GET /api/v1/auth/whoami` and
`GET /api/v1/info` with any scope, the imports below with `ingest`, and
the tools: those that read with `read` or `tools` (`GET /api/v1/me`,
the profile and the plan today; `GET /api/v1/days/{date}`, a day,
`today` or `YYYY-MM-DD`; `GET /api/v1/foods?query=…` and
`GET /api/v1/foods/{id}`; `GET /api/v1/summary`, calibration's summary
of the days up to yesterday; `GET /api/v1/history`, the days or weeks of
a span), and those that
write with `tools` alone (`POST /api/v1/foods`, a food saved or
corrected; `POST /api/v1/meals`, and `PATCH` and `DELETE` on
`/api/v1/meals/{id}`, a meal logged, changed or deleted;
`PUT` and `DELETE` on `/api/v1/days/{date}/metrics/{kind}`, a
measurement logged or deleted; `PATCH /api/v1/days/{date}`, a
check-in; `PUT /api/v1/me/notes`, the notes for a coach;
`POST /api/v1/undo`, the latest change taken back). Every
other
operation is closed to tokens (403), so a token can never create, list
or revoke tokens, read the session or change a setting (the name,
language, time zone, day's start, profile or limits), nor change the
plan as the app does (`POST /api/v1/plan/changes`), nor apply a
proposal that changes the limits. Only
`Authorization: Bearer tdm_…` carries a token (an empty Bearer is a bad
one): another header, such as a reverse proxy's Basic credentials or the
ID token an authenticating proxy passes on as a Bearer, leaves the
request to its session cookie.
The web app's settings list and revoke them too, and connect another
(Devices and AI coaches → Connect, where a token is an access key): an
AI coach, with `tools`; an iPhone with Health Auto Export, with
`ingest`; or another, of the scopes picked. The token is shown once,
when it is made, with the steps to give it to the client below, and the
sheet says when the client first calls with it.

### MCP

An AI client reaches the tools at `<TDEEMON_BASE_URL>/mcp` (MCP over
Streamable HTTP), with an API token as a Bearer token: one with `tools`
for every tool, or `read` for those that only read. Each tool is the
operation of the API above, with its validation and its answers, and a
meal or a change made through MCP is the client `mcp`'s. The server
tells every client how to log (its MCP instructions), so a client's own
skill, if any, shrinks to a few lines. Claude Code, for one:

```sh
claude mcp add --transport http tdeemon https://tdeemon.example.com/mcp \
  --header "Authorization: Bearer tdm_…"
```

The app's Connect gives this command with the token in it, the address
being the instance's (`TDEEMON_BASE_URL`, which `GET /api/v1/info` says
as `base_url`), and Claude Desktop's configuration, through
[mcp-remote](https://www.npmjs.com/package/mcp-remote), which passes the
header on. ChatGPT's connectors sign in with OAuth rather than a
header, which tdeemon does not offer yet.

### Import from Apple Health

[Health Auto Export](https://www.healthexportapp.com) (iOS, its REST
API automation is a paid feature) sends the phone's and the watch's
metrics to tdeemon:

- **URL:** `https://tdeemon.example.com/api/v1/ingest/hae`, method POST,
  format JSON;
- **header:** `Authorization: Bearer tdm_…`, a token with the `ingest`
  scope;
- **metrics:** step count, active energy, basal energy burned, Apple
  exercise time, sleep analysis, weight, body fat percentage, lean body
  mass, resting heart rate, heart rate variability and VO2 max. Others
  are skipped, and the answer names them; workouts are not imported;
- **Summarize Data:** on, with the time grouping by day, and a date
  range that starts at midnight (such as today, or since the last sync
  from a whole day): a day's total is then whole, and only grows from
  sync to sync. Sleep comes as nights; a sleep stage of an export that
  is not summarized is left out, and the answer says to turn it on.

The app's Connect, for an iPhone, makes the token and gives this as a
checklist, with the URL and the header's value to copy.

A value belongs to the calendar date of its point, the parts of a day's
total add up, the longest sleep is the night, and the day keeps its
earliest weighing, with body fat and lean mass from the same weighing.
A metric in a unit tdeemon does not take for it (stones, seconds of
exercise) is left out rather than read wrong.

The values merge as an import: a value you entered stays, the day's
first weighing stays, and a running total only grows, so a sync of part
of the day undoes nothing. A value out of its bounds is left out, and
the answer says why. A body must be under 10 MB; split a large first
export by date range in the app.

`POST /api/v1/ingest/metrics` takes the same from a script, one day at a
time:

```sh
curl -H 'Authorization: Bearer tdm_…' -H 'Content-Type: application/json' \
  -d '{"date": "2026-10-21", "metrics": {"weight": 80.2, "steps": 9000}}' \
  https://tdeemon.example.com/api/v1/ingest/metrics
```

Both routes take an API token, not a web session: what they write is
recorded as an import. The bodies are kept, compressed, in
`TDEEMON_DATA_DIR/ingest/<user>/<day>/`, outside the database and its
backups: Health Auto Export's as they came, even one that does not
read, and the simple form's as read. A user's days older than 30 go
when their next body arrives.

### Import from Strong

The Strong app's CSV export (Settings, Export Strong Data) holds the
whole history of workouts. tdeemon keeps the days they were on, each
workout on the day it starts in, by your day start hour, as a meal is:
a day with a workout counts as a training day, for the targets that
differ by the type of day, unless you set the day's type in the
check-in yourself. The sets themselves wait for the training module,
which reads them from the latest export tdeemon keeps.

Send the export as it comes, with a token with the `ingest` scope, from
an iOS Shortcut for instance (Get Contents of URL, method POST, request
body File, or Form with the file as `file`):

```sh
curl -H 'Authorization: Bearer tdm_…' -H 'Content-Type: text/csv' \
  --data-binary @strong_workouts.csv \
  https://tdeemon.example.com/api/v1/ingest/strong
```

Each export replaces the days of the one before: a workout you deleted
in Strong is no longer a day of a workout. An export without a workout
is refused, so a wrong file cannot clear them. The export is archived
with the other bodies, and the latest that read is kept past their 30
days, as `TDEEMON_DATA_DIR/ingest/<user>/latest-strong.csv.gz`.

### Import the history of the predecessor

tdeemon replaces the maintainer's earlier service, health-api, and takes
its history once: the days' metrics and meals from its database (its
`health.db`, an index of the vault's daily notes). Each meal goes on its
note's day, at its time on your clock (one before 05:00, the
predecessor's rollover, past midnight), split by its text against your
foods: a part with an amount and a food of yours becomes an item of
that food, the rest of the meal's total an estimate, "Rest of the meal
(history split)", so the meal keeps its kcal; a meal with no such part,
or whose items would pass its total, is one estimate, and so is a meal
a run before wrote so. A dry run splits against the foods of `--foods`
too, which it has not written. A day
whose total in the note has more kcal than its meals gets the rest as
one estimate, "Rest of the day (history import)", so the day's intake
is the note's. A day
with a remark (a restaurant, an illness, a trip) is excluded from
calibration for it, and so is a day over without intake ("No intake
logged in the predecessor", or "Intake of the predecessor not
importable"), but not one you logged meals of in tdeemon, as the
predecessor's calibration left both out; mood, hunger and status are
left out.

Copy `health.db` while health-api is stopped, or with `sqlite3 health.db
".backup copy.db"`: a copy without its `-wal` file misses the last
writes. Put it in a directory tdeemon can write to, as SQLite reads a
database in WAL mode only there.

```sh
kubectl exec deploy/tdeemon -- /tdeemon import-history \
  --email you@example.com --health-db /data/health.db --dry-run
```

With `--foods`, it takes the vault's food table, `Baza-hrane.md`, too
(or alone): a food given for 100 g or 100 ml stays so; one given for a
weight or a volume ("220 g", "~400 g", "0,5 kg", "330 ml", "2 dl") is
stored per 100 g or ml, with that amount as its serving "portion",
unless its values per 100 would be no food, when it is a food per
portion and the report lists it; any other portion ("1 medium", "400 g
cooked") is a food per portion, the portion in its note. A food whose
note names a declaration is a label's, the rest estimates. A name the
table has twice takes each row's portion, and a food you have already,
by its name, stays yours. A row of several foods eaten together, a plus
in its name or portion ("Jogurt + Whey + Müsli"), is no food and is left
out: a meal of them is a meal.

With `--plan`, it takes a plan file you write from the goals note, which
is prose: the profile, the training weekdays, the phases and the targets
with their history, and notes for a coach, in the shape of
[internal/history/testdata/plan.json](internal/history/testdata/plan.json).
It runs after the days, as the limits hold the current phase and target,
and the ones after them, to your weigh-ins of the last week (a dry run
takes the ones the days would bring); the current target is held as a
new one, as the history before it vouches for nothing. A violation
refuses the plan whole, and nothing of it is written
([docs/limits.md](docs/limits.md)).
A plan of your own in tdeemon, your training weekdays, the fields of
your profile you have set and your notes stay yours; a plan a run
before wrote in part is finished. A phase or a target the file has not
counts as yours, so a plan file changed after its import changes
nothing of the plan: run it with `--dry-run` until it is right.

With `--split-report PATH` it imports nothing, and needs no `--email`:
it writes the split of the meals of `--health-db` and the combinations
of `--foods` (a food of the table with a plus in its name or portion,
"Jogurt + Whey + Müsli") to PATH, a JSON file readable by you only, to
look at before the import. Each meal and combination has its parts, as
its text gives them: the part's words, its amount in g, ml or pieces,
and the food of the table it reads as; and a meal its place in its day
and its class, `known` (an amount and a food for each part), `amounts`
(an amount for each), `partial` or `untold`. It reads the words of the
notes, so it errs at times; the import applies it so that a meal keeps
its total whatever it got wrong.

```sh
kubectl exec deploy/tdeemon -- /tdeemon import-history \
  --health-db /data/health.db --foods /data/Baza-hrane.md \
  --split-report /data/split.json
```

It says which time zone and day start it imports by: create the user
with your `--timezone` first. Without `--dry-run` it writes, and says
what it left out: a value out of its bounds, a day whose meals pass its
total, a meal renamed to fit. It may run again, and changes nothing it
imported before: a meal that differs from the last run's (the note, the
time zone or the day's start changed) stays as it was, and so does a
day you have excluded or included since; both are reported.

## Data

tdeemon keeps everything in one SQLite file, `TDEEMON_DATA_DIR/tdeemon.db`,
which it creates readable by its owner only, and applies database
migrations at startup. The bodies of imports are kept apart, about 30
days, in `TDEEMON_DATA_DIR/ingest/` (see
[Import from Apple Health](#import-from-apple-health)). Before it
migrates a database that already has data, it saves a snapshot to
`TDEEMON_DATA_DIR/backups/pre-migrate-<version>-<time>.db`. It keeps the
newest snapshot of each schema version, for the three newest versions.

Once a day it saves a snapshot to
`TDEEMON_DATA_DIR/backups/daily-<time>.db` (the time in UTC): at startup
when there is none of the last 23½ hours, and then at a check every hour,
so at the same hour each day. It keeps the newest `TDEEMON_BACKUP_KEEP`
of them, 7 unless set, and removes the older ones after each snapshot: a
lower number takes effect with the next one, and `0` takes no more but
leaves the ones there. A snapshot is a whole copy of the database,
written with `VACUUM INTO` while tdeemon runs; writes wait for it. Leave
room on the volume for one copy more than it keeps, as the oldest goes
only once the new one is written, besides the three snapshots before a
migration: with the default, 11 copies of the database. The snapshots
sit on the same disk as the database: copy the backups directory off
it, for example with a volume snapshot or a job of your backup tool.

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
