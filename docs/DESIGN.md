# tdeemon — design (draft v0.2)

> Name: TDEE + daemon. Successor to the author's personal setup
> (an Obsidian vault + a small Go service + an AI agent on Telegram),
> rebuilt as a self-hostable, multi-user, open-source app (AGPL-3.0).
> Status: design phase, 2026-10. Nothing here is built yet.

## 1. Why

The predecessor works and is genuinely useful, but it can't be shared:

- **Seven moving parts** for one person's food log: Obsidian, CouchDB,
  livesync-bridge, NFS mirror, vault-webhook, health-api, Hermes (plus an
  Authentik path split).
- **Markdown is the source of truth and an LLM edits it.** That is where
  the format drift, broken food-table rows, a scanner that has to
  "tolerate drift", index lag and the stale-desktop-vault risk come from.
- **The LLM does arithmetic and bookkeeping** (totals, "don't duplicate
  the row", table format). The agent's skill file grew to ~100 KB of
  rules, and a single food log costs ~10 tool calls and hundreds of
  thousands of input tokens.

The fix is one inversion: **the database is the source of truth, and the
LLM only decides *what* was eaten and *how much*. Code does the rest.**

## 2. Goals and non-goals

### Goals (v1)

- AI-first food logging, by chat or by photo, with deterministic math.
- Adaptive targets for **any goal: cut, maintenance, bulk or recomp**.
  Measured TDEE from intake and weight trend, transparent rules, and an
  explicit-consent step for any target change. Nothing in the model or
  the wording assumes weight loss.
- Self-host as **one container + one SQLite file**.
- **Multi-user from day one.** A household or a few friends on one
  instance; the admin invites them. No public sign-up by default.
- **Not tied to any AI vendor or app.** The MCP server is the foundation:
  any MCP client, any model. A built-in chat uses your own API key.
- Apple Health metrics ingest (Health Auto Export) plus a generic JSON
  endpoint.
- English everywhere in code, API and tool descriptions. The UI and bot
  replies are localised (en, hr).

### Non-goals (v1)

- Training and strength. This becomes an optional module in v2 (§11).
- Logging workouts in the app (replacing Strong). Far future.
- A native mobile app. A PWA comes first; native comes later, mainly for
  HealthKit.
- A commercial or hosted SaaS, Postgres, or Obsidian sync.

## 3. Architecture

```text
            ┌──────────── single Go binary, SQLite (/data) ────────────┐
 HAE ──────▶│ ingest            domain: users, foods, recipes, meals,  │
 JSON ─────▶│                   metrics, days, targets, calibration    │
            │                                                          │
            │ tool registry (one definition, three transports)         │
            │   find_foods · log_meal · update_meal · get_day · ...    │
            │                                                          │
            │ scheduler: summaries, watchdog, backups                  │
            └──┬──────────────┬───────────────┬────────────┬──────────┘
          /mcp (MCP)    /api/chat        Telegram      web UI / PWA
          any client    built-in loop    adapter       dashboard, food DB,
          (Hermes,      (BYO key,        (BYO key,     settings, chat
           Claude, ...)  vision)          same loop)
```

- **One tool registry.** MCP, the built-in chat and Telegram all call the
  same Go functions with the same JSON schemas, so behaviour cannot drift
  between clients.
- **Logging policy ships from the server.** It goes out as MCP server
  `instructions` and the system prompt of the built-in chat. Rules such
  as "log immediately when clear", "ask once if the portion is unknown"
  and "show a breakdown" live in one place. This replaces per-agent
  skill files.
- **No more rollover job.** The "active note" was a markdown artefact. A
  meal has a timestamp, and its logical `day` comes from the user's
  `day_start_hour` (default 04:00), so a 01:30 snack belongs to the
  previous day automatically.

## 4. Data model (SQLite)

Every user-owned table carries `user_id`. Isolation is enforced in the
store layer and covered by tests that run every query as the wrong user.

| table | key columns | notes |
| --- | --- | --- |
| `users` | id, name, email, role (admin/user), locale, timezone, day_start_hour | invites; password (argon2) and/or OIDC subject |
| `api_tokens` | id, user_id, name, hash, scopes, last_used_at | scopes: `ingest` (phone), `tools` (MCP/clients), `read` |
| `foods` | id, user_id, name, brand, kcal/protein/carbs/fat per 100 g, serving_g, serving_name, source (label/estimate/user/off), note, archived | the user's food DB; "same name + same portion" upsert rule enforced in code |
| `food_aliases` | food_id, alias | "the dessert from last Sunday", "my bread" |
| `recipes` / `recipe_items` | recipe → food_id + grams, yield_g | batch cooking; exposed as a food with computed per-100 g values |
| `meals` | id, user_id, day, eaten_at, description, client (mcp/chat/web/telegram/import) | |
| `meal_items` | id, meal_id, food_id?, name, grams?, kcal, protein, carbs, fat, basis (food/label/estimate), note | **macro snapshot at log time**; editing a food later does not silently rewrite history |
| `metrics` | user_id, day, metric, value, source, updated_at | long format: weight, body_fat, lean_mass, steps, active_kcal, basal_kcal, sleep_h, rhr, hrv, vo2max, exercise_min |
| `days` | user_id, day, status (open/complete/excluded), training_day?, mood, hunger, note | check-in; `excluded` drops a day from calibration (restaurant, sick, forgot to log) |
| `targets` | user_id, effective_from, kcal, kcal_training?, kcal_rest?, protein, carbs, fat, proposal_id? | effective-dated rows; the why lives in the proposal |
| `proposals` | id, user_id, actor (user / model + client), status (pending/applied/rejected/expired/reverted), rationale, window, changes (JSON diff), preview, created_at, decided_at | every plan change (§8); applied ones are the revision log |
| `plan_settings` | user_id, autonomy (propose/autopilot), autopilot bounds, check-in weekday, guardrail overrides | guardrails are editable only in the UI, never through tools |
| `phases` | user_id, type (cut/maintain/bulk/recomp), started_at, ends_at?, target_rate_pct_week (signed), goal_weight?, milestone_weight? | the user's goal timeline; a phase start anchors the water/glycogen window; a diet break or a deload maintenance week is just a short `maintain` phase; goal and milestone may sit above or below the current weight |
| `profile_notes` | user_id, text | free-text preferences and guidance (e.g. food dislikes, schedule constraints) handed to the AI as context |
| `shopping_items` | user_id, text, done | |
| `events` | id, user_id, at, actor, client, action, payload | audit trail; powers `undo_last` |

**Metric merge rules** carry over from the predecessor: a manual weight beats an
imported one, sleep is a max-merge per date, cumulative metrics never
shrink on a partial sync.

**Day status:**

- A day becomes `complete` automatically once its boundary passes.
- The summary flags suspiciously low days, e.g. under 50 % of target, as
  "possibly incomplete", and the user can mark them `excluded`.
- Nobody has to close a day anymore.

## 5. Tools (MCP + built-in chat)

**Principle:** few, coarse tools. Each one returns everything needed to
answer, so a food log is **1–2 calls**: `find_foods`, then `log_meal`.

| tool | purpose |
| --- | --- |
| `get_profile` | current phase and goal, current targets (day-type aware), profile notes, locale, units |
| `find_foods(query, limit)` | fuzzy match over foods, aliases, recipes; includes last-used portion |
| `log_meal(eaten_at?, description, items[], note?)` | stores the meal; returns breakdown + day status |
| `update_meal(meal_id, ops[])` | add/replace/remove items, change time; returns breakdown + day status |
| `delete_meal(meal_id)` / `undo_last()` | |
| `upsert_food(...)` | save or correct a food (label values); same-name-same-portion rule |
| `get_day(date?)` | meals, totals, targets, remaining, metrics, check-in |
| `log_metric(metric, value, date?)` | weight and other manual measurements |
| `checkin(date?, mood?, hunger?, note?, training_day?)` | |
| `get_summary(window?)` | weekly averages, weight trend, measured TDEE, calibration verdict, recovery flags |
| `get_history(window, granularity)` | coaching: compact day/week table across all metrics (§8) |
| `get_plan()` | phases, target history **with outcomes**, autopilot bounds, guardrails, pending proposals |
| `propose_changes(changes, rationale, window)` | validated diff of phases/targets/settings/notes + preview; applies nothing |
| `apply_proposal(id)` / `reject_proposal(id)` / `revert(id)` | **apply only after explicit user consent**; recorded with actor and rationale |
| `update_profile_notes(...)` | preferences and life context the coach should remember |
| `shopping_list(op, items?)` | |

`log_meal` items accept three shapes:

```jsonc
{ "food_id": 17, "grams": 150 }                              // known food
{ "name": "Greek yoghurt 2 %", "grams": 200,
  "per_100g": { "kcal": 73, "protein": 9.5, "carbs": 3.6, "fat": 2.0 },
  "save_food": true, "source": "label" }                     // from a label
{ "name": "Restaurant pasta", "kcal": 850, "protein": 32,
  "carbs": 105, "fat": 30, "estimate": true }                // one-off estimate
```

The response holds the stored meal (items with computed macros), the day
totals against the day-type target, what remains, and `warnings`. Warning
examples: the Atwater check (4P + 4C + 9F) is more than 15 % off the
stated kcal, saturated fat is above total fat, a duplicate of a meal
logged minutes ago.

REST under `/api/v1` mirrors the same operations for the web UI and
scripts.

## 6. AI layer

1. **MCP server** at `/mcp`, Streamable HTTP.
   - **v1:** auth with a per-user Bearer token. That is enough for
     Hermes, Claude Code and most self-hosted clients.
   - **Later:** a minimal built-in OAuth 2.1 authorisation server (PKCE,
     dynamic client registration) so Claude and ChatGPT custom
     connectors work.
   - **Photos:** the image stays in the client; the model sends
     structured items.
2. **Built-in chat** at `/api/chat`, used by the web UI/PWA.
   - Server-side agent loop over the tool registry, with vision for
     photos.
   - Provider is any OpenAI-compatible endpoint (OpenAI, OpenRouter,
     Ollama, …), with native Anthropic as an option.
   - **Two model slots:** `fast` for logging and `smart` for coaching
     sessions (§8).
   - **Keys are per user** (BYO), encrypted at rest with the instance
     secret. An admin may set an instance default.
   - **Cost target:** under ~10k input tokens per food log.
3. **Telegram adapter.** One bot per instance; users link their account
   with `/start <code>`, which maps chat_id → user_id. It feeds the same
   loop as the built-in chat, using the user's key.

## 7. Calibration v2

The predecessor's calibration never reached a verdict. It estimated TDEE
from the difference between two consecutive weekly averages, which
swings by thousands of kcal from week to week, and it needed two such
estimates to agree within ±200 kcal.

The replacement:

- **Weight trend:** an exponentially smoothed trend weight plus a linear
  regression over a rolling 21–28-day window. The slope comes with a
  standard error.
- **Intake:** the average over `complete`, non-`excluded` days in the
  same window.
- **TDEE** = intake − slope × 7700 / 7, with a confidence interval
  derived from the slope error and logging completeness.
- **Verdict:** a recommendation appears only when the interval is narrow
  enough. Otherwise the summary shows the estimate with its uncertainty,
  not just "calibrating".
- **Goal-agnostic rules.** Each phase has a target band in % body weight
  per week. The verdict compares the measured rate with that band and
  proposes a kcal step toward it:
  - Too slow on a cut → lower the target.
  - Gaining too fast on a bulk → shrink the surplus.
  - Drifting during maintenance → nudge back.
  - Band defaults per phase type are TBD from the literature and are
    user-adjustable.
- **Guards, by phase type:**
  - Water/glycogen window: the first 14 days of *every* phase (glycogen
    refills on a bulk start too).
  - NEAT guard: steps ±30 % from baseline, in either direction.
  - Lean-mass guard (cut/recomp): ≥ 0.5 kg/week lean loss turns a cut
    into "raise protein/calories".
  - Fat-gain guard (bulk): body fat % rising with lean mass flat →
    shrink the surplus.
  - Strength veto (cut/maintain).
- **Split targets:** the rule decides which day types absorb a change.
  The predecessor's version lowers rest days only and raises both.
- **Protein floor in g/kg** (of body weight or of goal weight) instead of
  a fixed gram value.
- **Per-user parameters** for all of the above. The predecessor's rules
  are the starting cut-phase defaults.
- **Test fixtures:** an anonymised multi-week dataset lives in the repo.
  Real data stays in a private fixture outside git and is checked
  against past manual target revisions.

## 8. Coaching: the AI changes the plan

Users never tune numbers by hand. They say what they want ("start a
lean bulk in January", "I've plateaued, what now?", "make weekends
easier") and the AI changes phases, goals and targets, like a private
coach or nutritionist. For a one-off session like that, the right tool
is a strong model through MCP, paid for by the user's own
Claude/ChatGPT subscription.

| layer | who | how often | may change |
| --- | --- | --- | --- |
| **Plan** | coach session: the user + a strong model (MCP, or the built-in `smart` slot) | on request, e.g. monthly | phase timeline, target rate band, goal/milestone weight, protein g/kg, day-type split, autopilot bounds, profile notes, later the training program |
| **Autopilot** | server, deterministic | weekly check-in | kcal within the plan's bounds, from the calibration verdict (§7) |
| **Logging** | any client, `fast` model | many times a day | meals, metrics, check-ins — never the plan |

### Proposals, not direct writes

1. **Propose.** `propose_changes` submits a diff (phases, targets,
   settings, notes) with a rationale and the analysed window. The
   server validates it and returns a preview: daily targets per day
   type, the expected rate, any guardrail violations.
2. **Consent.** The user agrees in plain language ("može", "yes") and the
   AI calls `apply_proposal`. Pending proposals also show in the web UI
   with Accept/Reject, so a proposal made in Claude can be accepted in
   the app.
3. **History.** Applied proposals are the revision history: who (user,
   model name, client), when, why, before → after. `revert` creates the
   inverse proposal.

### Autonomy, per user

- `propose` (default): the autopilot and the coach only propose.
- `autopilot`: weekly adjustments within the bounds apply automatically
  and are reported.
- A plan change never applies without consent.

### Guardrails live in code, not in prompts

The server cannot trust the model. MCP means *any* client and *any*
model, and a model can be talked into things, misread data, or follow
instructions injected through a label photo, a recipe or a web page
it read while researching. So limits are enforced by the server.

- **One function.** A pure, unit-tested `Validate(change, profile,
  limits) → []Violation` runs on `propose_changes` (the preview shows
  violations), again on `apply_proposal` (the data or the limits may
  have changed since the proposal), on every autopilot step, and on
  manual target edits in the UI.
- **Machine-readable violations.** A violation names the rule, the limit
  and the proposed value, e.g. `{rule: "kcal_floor", limit: 1800,
  proposed: 1500}`. The model can then explain the refusal and propose
  something allowed, so the loop corrects itself.
- **Limits:**
  - kcal floor, absolute and relative to the estimated BMR;
  - maximum rate of loss and of gain (% body weight/week);
  - protein floor (g/kg);
  - maximum kcal step per change;
  - minimum time between autopilot changes, so a plateau cannot turn
    into cut-cut-cut;
  - maximum cumulative autopilot drift from the plan.

  Default values are TBD from the literature; the predecessor's rules
  are the starting point.
- **Out of the AI's reach by construction, not by instruction.** No
  tool reads-for-write or changes limits. The settings endpoint accepts
  only an interactive web session (cookie + CSRF), never a Bearer token,
  so not even a leaked MCP token can relax them.
- **The user can change them,** with an explicit warning, e.g. for a
  medically supervised very-low-calorie diet. Adults decide; models
  don't.
- **Two levels.** Instance limits (admin, in config) are the outer
  bound for everyone on the instance; per-user limits sit within them.
  Someone hosting friends can set a floor nobody goes below.
- **Plans only, never logging.** Guardrails apply to targets and phases.
  Logging is never blocked or judged: a 4000 kcal day is simply logged.
- **Not medical advice.** The docs say so plainly. Other people will
  self-host this, so the defaults must be safe for someone who is not
  the author.

### What the coach reads

The coach gets enough to reason with, without raw dumps:

- `get_history`: a compact day/week table with intake and adherence,
  trend weight, body composition, steps, sleep, RHR/HRV, training days,
  and excluded days with reasons.
- `get_plan`: the phase timeline and the target history *with
  outcomes*, i.e. planned vs measured rate and adherence for every
  target period.
- `get_summary`: the deterministic calibration. The server computes, the
  model interprets.

### MCP extras

- **Prompts as ready-made sessions** ("Monthly review", "Plateau check",
  "Plan a new phase"). They show up as slash commands in clients and
  load the right context.
- **The methodology as an MCP resource**, i.e. how TDEE and verdicts are
  computed, so the coach understands the numbers it reads.
- **Tool annotations:** `readOnlyHint` on reads and `destructiveHint` on
  apply/revert, so clients ask before writes.

### Training uses the same pattern (P3+)

The coach researches a program with the user in their own AI client
(web search, back-and-forth), then submits it as a proposal. On
acceptance it becomes the active program and the app prescribes every
session from it.

This requires **programs to be data, not code**. A program is a
declarative spec: days, slots, exercises, set/rep/RPE schemes, deloads
and tests. It is composed from a library of progression rules
implemented in code: linear, double progression, RPE top set +
back-offs, percentage-based, and so on. The AI composes rules; it does
not invent arithmetic. The predecessor's RPE-based (RTS) program becomes
the first spec.

## 9. Ingest, notifications, ops

- **Ingest:**
  - `POST /ingest/hae` is the Health Auto Export payload; `POST
    /ingest/metrics` takes `{date, metrics}`.
  - Both use a per-user `ingest` token, and the raw payloads are
    archived.
  - Caveat: HAE is iOS-only and its automated export is a paid tier.
    Native HealthKit is the long-term answer (§11).
- **Morning summary:** a deterministic template (sleep, yesterday vs
  target, weight trend, calibration line, recovery flags). Delivered by
  Telegram send (no LLM), web push or ntfy. An optional AI comment is
  off by default.
- **Watchdog**, configurable per user: no metrics for > 26 h, nothing
  logged by a set hour, an ingest token unused for N days. It goes out
  through the same channels.
- **Deploy:**
  - Docker image with one volume, `/data`.
  - Env, all prefixed `TDEEMON_`: `BASE_URL`, `SECRET_KEY`, `LISTEN`,
    `DATA_DIR`, `LOG_LEVEL`, `LOG_FORMAT`, later optional `OIDC_*` and
    `TELEGRAM_BOT_TOKEN`; plus the standard `TZ`. The README has the
    current table.
  - Ships a `docker-compose.yml`; the homelab gets its own manifests.
- **Backup:** a built-in nightly `VACUUM INTO` snapshot with retention
  (plus one before every migration),
  and documented Litestream for continuous replication to S3/B2. Never
  put the DB on NFS.
- **Dev process:**
  - **Principle:** latest stable versions of every language, runtime
    and tool, and current best practice for each language in the repo.
    Rules are written down and machine-checked, and the lint configs
    are reviewed when new tool versions ship.
  - `main` is protected by a ruleset: PRs only, squash merge, merge
    blocked until every required check is green.
  - PR titles follow Conventional Commits, and every merge to `main`
    cuts a semver release automatically (tag, GitHub Release,
    changelog, image).
  - On the author's instance, Flux image automation deploys new
    releases, so a deploy depends only on the merge.
  - CI runs tests with coverage gates plus strict lint and format for
    every language: `golangci-lint` (with gofumpt), oxlint with
    type-aware rules/Prettier/`tsc`, `hadolint`, `actionlint`,
    markdownlint. The same checks run locally via `make check` and
    lefthook.
  - Renovate, CodeQL, gitleaks and a license check guard dependencies
    and secrets.
  - `AGENTS.md` carries the conventions for AI agents, and `CLAUDE.md`
    imports it.
- **Backend stack:** Go and `modernc.org/sqlite` (pure Go, WAL,
  busy_timeout) carry over from the predecessor. Migrations are versioned and
  embedded (no more "rebuild the index"). SQL stays portable, so Postgres
  would be an adapter, not a rewrite.
- **API contract:** the OpenAPI 3.1 spec is generated from the Go
  handlers (huma v2), served at `/api/v1/openapi.json` and committed as
  `api/openapi.json`; the TypeScript types are generated from that file
  (openapi-typescript, in `web/codegen`). Checks fail when either is
  stale, so the frontend, REST and MCP tool schemas cannot drift. MCP
  tools take their JSON Schemas and input validation from the same huma
  registry.
- **Frontend stack:**
  - React (with the React Compiler) + TypeScript + Vite, built as a
    static SPA and embedded into the binary with `go:embed` under the
    `webui` build tag, so Go builds never need Node. No Node at runtime;
    self-host stays one container (multi-stage Docker build).
  - TanStack Query (server state, typed by the OpenAPI document through
    openapi-fetch) and TanStack Router (routes declared in code).
  - Tailwind + shadcn/ui on Base UI (components owned in-repo).
  - ECharts, keeping the validated palette.
  - `vite-plugin-pwa` for install and web push.
  - Later, Capacitor wraps the same app for iOS with a HealthKit plugin.
- **Offline-ready IDs:** clients generate IDs (UUIDv7) for meals, items
  and later sets, and writes are idempotent on that ID. The future
  in-gym workout logger can queue writes offline and sync without
  duplicates.

## 10. Migration from the predecessor

1. **Importer** (`tdeemon import-vault`), reusing the predecessor's
   markdown parser:
   - daily notes → `days`, `metrics`, `meals` (historic meals become one
     estimate item each);
   - the food table note → `foods`;
   - the goals note → `phases`, the `targets` history (from its
     revision log) and `profile_notes`;
   - the Strong archive and the program/exercise notes wait for the
     training module.
2. **Parallel run = shadow mode, about two weeks.** The agent keeps
   logging to the vault, tdeemon re-imports the vault nightly (the
   importer is idempotent) and receives HAE directly. Daily totals and
   metrics are compared; after a final import, logging switches to
   tdeemon.
3. **Cut-over:**
   - the agent switches to `/mcp`, and its health skill shrinks to a few
     lines;
   - the HAE automation points at the new app;
   - the agent's nightly rollover and morning-summary jobs are removed
     (tdeemon has no rollover and sends the summary itself).
4. **Obsidian stays** for other uses. The health folder is archived and
   nothing health-related depends on vault sync anymore.
5. **The old service keeps the training side until P3** (Strong import,
   program prescription, training dashboard). tdeemon reads training
   days from its workouts API for the split targets, and the strength
   veto waits for P3. Its nutrition side is retired after the parallel
   run.
6. **The repo is public from the first commit** (AGPL-3.0, CLA from day
   one). GitHub Free only enforces rulesets on public repos, and a
   protected `main` is a requirement. **Personal data never enters the
   repo:** real-data fixtures and eval sets live outside git, and tests
   in the repo use synthetic or anonymised data.

## 11. Phases

| phase | scope |
| --- | --- |
| **P0 — replace the predecessor** | schema + migrations, foods/meals/days/metrics/phases/targets, tool registry, MCP (token auth), REST, HAE ingest, calibration v2, **proposals + coaching tools + guardrails**, importer, minimal web UI (day view, food DB, pending proposals, login), **deterministic morning summary + watchdog via Telegram**; training days read from the old service; the agent on MCP; cut-over |
| **P1 — v0.1 open source** | multi-user + invites, built-in chat with BYO key (`fast`/`smart` slots) and photos, **autopilot weekly check-in**, MCP coaching prompts, PWA, morning summary + watchdog, recipes, i18n (en/hr), docker compose, docs, CI |
| **P2** | Telegram adapter, OAuth for Claude/ChatGPT connectors, shopping list, generic CSV history import, barcode → Open Food Facts lookup |
| **P3 — training module** | generic workout data (exercise, sets, reps, weight, RPE, duration, distance) + import adapters (Strong, others); **programs as declarative specs** over a library of progression rules, created by the coach through proposals (§8); the RTS program is the first spec; `training_day` derived from any workout source |
| **P4** | in-app workout logging (replaces Strong); native app with HealthKit (replaces HAE) |

Until P3, a training day for split targets comes from a manual toggle
(`checkin.training_day`) or an imported workout metric.

## 12. Open questions

- ~~Name~~ → **tdeemon**.
- ~~Observability~~ → deferred to a later phase: logs, `/healthz` and the
  watchdog until then; `/metrics` and dashboards later.
- ~~License~~ → **decided: AGPL-3.0 + a CLA** from the first external
  contribution. The CLA keeps the author's right to dual-license
  without asking each contributor.
- ~~Frontend~~ → **decided: React SPA embedded in the Go binary**
  (see §9).
- **Food DB scope.** Per user only, or optionally shared within a
  household.
- **Units.** Metric only in v1, stored in SI.
