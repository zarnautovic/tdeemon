# tdeemon — design (draft v0.2)

> Name: TDEE + daemon. Successor to the author's personal setup
> (an Obsidian vault + a small Go service + an AI agent on Telegram),
> rebuilt as a self-hostable, multi-user, open-source app (AGPL-3.0-only).
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
| `users` | id, name, email, role (admin/user), locale, timezone, day_start_hour, sex (for the energy equations)?, birth_date?, height (cm)? | invites; a password (argon2id), and OIDC identities in `user_identities`; the profile (sex, birth date, height) is optional for logging, needed for a plan, and changes only from a web session (§8) |
| `sessions` | id (SHA-256 of the cookie's token), user_id, created_at, last_seen_at, expires_at | web sessions; 30 days from sign-in |
| `user_identities` | issuer, subject, user_id | OpenID Connect identities (Authentik…); linked once by a verified email, then by subject |
| `api_tokens` | id, user_id, name, hash, scopes, last_used_at | scopes: `ingest` (phone), `tools` (MCP/clients), `read` |
| `foods` | id, user_id, name, brand, basis (100g/100ml/portion), kcal, protein, carbs, fat, optional fiber/saturated_fat/sugars/salt/polyols/alcohol, portion_g?, source (label/estimate/user/dish, later recipe/off), note, archived | the user's food DB; values per 100 g or 100 ml, or per portion for a whole dish (a canteen meal); "same name + same brand" is the same food, corrected in place |
| `food_servings` | food_id, user_id, name, amount | named amounts of a food by weight or volume: "piece" = 60 g of egg, "slice" = 40 g of bread |
| `food_aliases` | food_id, user_id, alias | "the dessert from last Sunday", "my bread" |
| `recipes` / `recipe_items` | recipe → food_id + grams, yield_g | batch cooking; exposed as a food with computed per-100 g values |
| `meals` | id, user_id, day, eaten_at, description, client (mcp/chat/web/telegram/import), note, fingerprint | the fingerprint of the create, for idempotent retries (§9) |
| `meal_items` | id, meal_id, user_id, food_id?, name, quantity as given (amount with its unit g/ml, or servings × serving, or portions), amount + unit (g/ml/portion), the nutrients, source, note | **snapshot at log time**; editing a food later does not silently rewrite history |
| `metrics` | user_id, day, metric, value, source, updated_at | long format: weight, body_fat, lean_mass, steps, active_kcal, basal_kcal, sleep_h, rhr, hrv, vo2max, exercise_min |
| `days` | user_id, day, excluded_reason?, training_day?, mood?, hunger?, note? | check-in (mood and hunger 1–5); open and complete follow from the clock, only an exclusion is stored, with its reason; `excluded` drops a day from calibration (restaurant, sick, forgot to log); `training_day` is the check-in's toggle of the day's type, NULL when the plan decides (§7) |
| `targets` | user_id, effective_from, for training days and for rest days: kcal, protein, fat; proposal_id? | effective-dated rows; the why lives in the proposal; an unsplit target has the same for both; carbs are what the kcal leave after protein and fat, so a kcal step moves the carbs and keeps the grams of protein and fat |
| `proposals` | id, user_id, actor (user / model + client), status (pending/applied/rejected/expired/reverted), rationale, window, changes (JSON diff), preview, created_at, decided_at | every plan change (§8); applied ones are the revision log |
| `plan_settings` | user_id, training_weekdays, limits?; later autonomy (propose/autopilot) and check-in weekday | the training weekdays give each day its type ahead of time (§7); the user's limits are a JSON object of their names ([limits.md](limits.md)) holding only those off their defaults, so a limit at its default follows a later release's default, and change only from a web session, never through tools (§8) |
| `phases` | user_id, start, type (cut/maintain/bulk/recomp), end_day?, rate_pct_week (signed), goal_weight?, milestone_weight? | the user's goal timeline, one phase per start, none overlapping another; a phase without an end lasts until the next starts; a phase start anchors the water/glycogen window; a diet break or a deload maintenance week is just a short `maintain` phase; goal and milestone may sit above or below the current weight |
| `profile_notes` | user_id, notes | free-text preferences and guidance (e.g. food dislikes, schedule constraints) handed to the AI as context; one text per user |
| `shopping_items` | user_id, text, done | |
| `events` | id, user_id, at, actor (user/import), client, model, operation, entity, before_state, after_state, undone_by, stale | every write, with the state before and after (JSON); the audit trail, the tombstone of a deleted meal, and what `undo_last` takes back; `stale` marks a change undo passed over (§5); the states of the plan's writes have JSON names of their own, so a field added later reads older states |

A table of a user's rows carries `user_id` even when its parent row has
it too (`meal_items`, `food_servings`, `food_aliases`), with a foreign
key on the pair, so a child can never belong to another user's parent and
the isolation test can compare every table by user.

**Foods.** The agent works out what was eaten from a short description
or a photo, finds it in the food DB, and saves what is new for next
time; the code does the arithmetic. A food is one of:

- **by weight or volume** (most of them): values per 100 g or 100 ml, so
  any amount eaten is one multiplication, half a bar as much as a whole
  one. Named servings turn "4 eggs" or "half a roll" into grams;
- **per portion**: a dish judged as a whole (a canteen meal, a
  restaurant plate), eaten whole or in part ("half a portion"). With the
  portion's weight known it can be logged in grams too. When the agent
  logs such a dish it asks whether to keep it for next time; the user
  decides;
- **a cooked dish without a recipe** (`source: dish`): the agent works
  out the values per 100 g of the finished dish from the ingredients and
  the weight of the pot, or roughly, and keeps the ingredients as a note;
  a cake cut into pieces is a food per portion. This stays an option
  after recipes come: a recipe stores its ingredients for a dish cooked
  again, and produces the same kind of food, with `source: recipe`.

The same name and brand (compared without regard to case or runs of
space) are the same food: saving it again corrects it rather than adding
a second. An alias leads to one food. Search runs over names, brands and
aliases through a full-text index that ignores case and diacritics
("cevapi" finds "Ćevapi") and matches word prefixes: foods whose name
holds every word first, then name and aliases, then the rest, shorter
names first, from the user's own foods alone; archived foods stay out
of it but keep their history.

A meal is a meal, not a row in the food DB: its items point at foods.
The optional nutrients are kept when known (a label) and used when
present; nobody has to enter them. Values are stored as they are and
rounded only for display (kcal to a whole number, grams to a tenth, salt
to a hundredth), which the nutrients and targets in the API's answers
already are: a total is the sum of what was eaten, rounded, and what
remains of a target is worked out before rounding. Amounts and metrics
come as they are stored.

**Metric merge rules** carry over from the predecessor
(`internal/domain/metric`): a value the user gave (`manual`, from any
client) beats an imported one, of every kind; the day's first imported
weighing stays (the morning's, not the one after dinner; body fat and
lean mass with it); an import of sleep or of a running total of the day
(steps, active and basal kcal, exercise minutes) is kept only when it is
more, so a sync of part of the day undoes nothing; any other import
replaces the one before. Each kind has bounds a person can have, so a
typo or minutes for hours of sleep is refused rather than stored;
converting units is ingest's, by the units the source names. A merge
reports what it changed, with the value before, for the event log and
undo; deleting a wrong manual value lets imports apply again.

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
| `get_profile` | the profile (sex, age, height), current phase and goal, current targets (day-type aware), profile notes, locale, units |
| `find_foods(query, limit)` | the foods whose name, brand or an alias has words starting with the query's, without case or diacritics, the name's first; with the last use (the amount as given and in the food's unit) |
| `log_meal(id, eaten_at?, description, items[], note?)` | stores the meal under the client's UUIDv7 (a retry returns it, §9); returns breakdown + day status |
| `update_meal(id, eaten_at?, description?, note?, ops[])` | add/replace/remove items, change the time and the words; an add carries its item's id, made by the client, so one sent again adds once; an item no op touches keeps what it brought when logged; changes sent at once are each kept; returns breakdown + day status |
| `delete_meal(id)` / `undo_last()` | `undo_last` takes back the user's latest change not undone yet (an import's writes are left, and so are changes of the plan: the profile and the user's limits, which only a web session makes, and the phases, targets and training weekdays, which go back as a `revert` that `Validate` checks against the limits as they are now), newest first: undoing again goes further back, it does not redo. A change whose entity is no longer as it left it is not taken back (an import wrote it, and undoing would lose the import; or a later change undo passed over), nor one that would clash with what came since (another food took the name or an alias, another meal an item) or that cannot be read or put back: the answer says why, and the next undo goes on to the change before. A failure of the database passes over nothing, so the next undo tries the same change |
| `get_food(id)` | a food by its id, archived too: what correcting the food in the way of a save starts from |
| `upsert_food(...)` | save or correct a food (label values); the same name and brand are the same food, and a name or alias another food has is refused naming that food |
| `get_day(date?)` | meals, totals, targets, remaining, metrics, check-in |
| `log_metric(kind, value, date?)` / `delete_metric(kind, date?)` | weight and other manual measurements, of today or a day before; a value the user gave counts over an import, and deleting a wrong one lets imports apply again; each answers the day |
| `checkin(date?, mood?, hunger?, note?, training_day?, excluded_reason?)` | what the user says of a day, each field given and only those: mood and hunger 1–5 (0 clears), a note, the day's type (`training`, `rest`, or `plan`: the training weekdays decide), and an exclusion from calibration with its reason (empty brings the day back); a day to come takes its type and exclusion, not a mood, hunger or note; one event, so one undo takes it back; answers the day |
| `get_summary(window?)` | weekly averages, weight trend, measured TDEE, calibration verdict, recovery flags |
| `get_history(window, granularity)` | coaching: compact day/week table across all metrics (§8) |
| `get_plan()` | phases, target history **with outcomes**, the limits, pending proposals |
| `propose_changes(changes, rationale, window)` | validated diff of phases/targets/settings/profile/notes + preview; applies nothing |
| `apply_proposal(id)` / `reject_proposal(id)` / `revert(id)` | **apply only after explicit user consent**; recorded with actor and rationale |
| `update_profile_notes(...)` | preferences and life context the coach should remember |
| `shopping_list(op, items?)` | |

`log_meal` items accept these shapes (field names are illustrative; the
tool schema is the reference). An amount always names its unit, and it
must be the food's: 250 ml of a food by weight is refused rather than
taken for 250 g. Grams of a portion food work when its portion weight
is known.

```jsonc
{ "food_id": "0199…", "amount": 150, "unit": "g" }           // known food, in its unit (g or ml)
{ "food_id": "0199…", "servings": 4, "serving": "piece" }    // a named serving: 4 eggs
{ "food_id": "0199…", "portions": 0.5 }                      // half a canteen portion
{ "food": { "name": "Greek yoghurt 2 %", "basis": "100g", "source": "label",
            "kcal": 73, "protein": 9.5, "carbs": 3.6, "fat": 2.0 },
  "amount": 200, "unit": "g" }                               // a new food, saved with the meal
{ "name": "Restaurant pasta", "kcal": 850, "protein": 32,
  "carbs": 105, "fat": 30 }                                  // a one-off estimate of a whole plate
```

The response holds the stored meal (items with computed macros), the day
totals against the day-type target, what remains, and `warnings`. Warning
examples: the stated kcal are more than 15 % (and 10 kcal) off what the
grams give by the Atwater factors (4P + 4C + 9F + 7 alcohol, with fiber
and polyols by any of the conventions labels use), saturated fat is
above total fat, a duplicate of a meal logged minutes ago. A new food is
saved in the meal's transaction, so a meal refused saves none, and a
meal sent again saves none: it is told by what was sent (§9) before
anything of it is read or saved.

REST under `/api/v1` mirrors the same operations for the web UI and
scripts. A tool is one operation of the API, in the registry
(`internal/api/tools.go`): its MCP name, its REST route, its handler.
MCP runs a tool through that operation, so the validation, the token
check and the answers are REST's. A tool that reads takes a token with
the `read` or the `tools` scope; one that writes, only `tools`. So far:
`get_profile` is `GET /api/v1/me`; `get_day` is
`GET /api/v1/days/{date}`, a date or `today`, the user's day now (MCP
takes its `date` as optional, `today` when left out); `find_foods` is
`GET /api/v1/foods?query=…`; `get_food` is `GET /api/v1/foods/{id}`,
archived too; and `upsert_food` is `POST /api/v1/foods`, which corrects
a food by an id it has (404 for another) or by its name and brand, and
names the food in the way of a name or an alias (409), archived or not,
which a search would not find but `get_food` gives. `log_meal` is
`POST /api/v1/meals` (201, or 200 for a retry), `update_meal` is
`PATCH /api/v1/meals/{id}` (ops: add, replace, remove an item; an item
no op touches keeps what it brought when logged), and `delete_meal` is
`DELETE /api/v1/meals/{id}`; each answers where the meal's day stands.
A meal logged with a token is the client `api`'s until MCP names
itself. `log_metric` is `PUT /api/v1/days/{date}/metrics/{kind}`,
`delete_metric` is `DELETE` on the same route (404 for a kind the day
has none of), and `checkin` is `PATCH /api/v1/days/{date}`; each takes a
date or `today`, and answers the day as `get_day` does.

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

- **Window:** the 42 days up to yesterday, today not being over
  (`internal/domain/calibration`), and none before the user's first day
  with a meal: weigh-ins imported from before it have no intake to go
  with. Over 28 days the estimate swung with
  water the weight held for weeks, as after a change of training; over
  42 it kept closer to the past manual revisions and still followed the
  TDEE as it fell.
- **Weight trend:** an exponentially smoothed trend weight, which takes a
  tenth a day of each weigh-in's difference from it: after a gap of n
  days, 1 − 0.9ⁿ, so after weeks without weighing it follows the weight
  again at once. The summary shows it, and it is the trend weight the
  limits read, the one on the last day asked for. The slope is a linear
  regression of the weigh-ins in the window, not of the trend, which
  lags behind (about 9 days, at a tenth). A day without a weigh-in is
  simply missing.
- **Intake:** the average over the `complete`, non-`excluded` days with
  at least one meal, from the window's first weigh-in to its last: the
  days whose energy the slope measures. A day without a meal was not
  logged: its intake is unknown, not 0.
- **An excluded day** counts for nothing, its weigh-in neither, nor in
  the trend: a day the user took out (sick, a trip, a restaurant) often
  has a weight that is off too, and one at the window's edge would move
  the slope.
- **Days not logged** are taken to be like the logged ones, give or take
  the spread of a day. A day left out for being unlike them, as a big
  day one does not log or excludes, biases the TDEE low by what it would
  have added; no interval can tell, so logging an estimate of such a
  day is better than leaving it out.
- **TDEE** = intake − slope × 7700 / 7, with an 80 % interval from two
  errors:
  - The slope's, widened by the autocorrelation of the residuals.
    Weigh-ins on following days carry water and salt over, so fewer of
    them are independent than it seems. The lag-1 autocorrelation of a
    few weeks falls short of the true one, and is raised by
    (2 + 4ρ) / n. In a simulation, the interval then covers the TDEE in
    about 80 % of the windows, for water that carries over from not at
    all to strongly. One that takes the weigh-ins as independent covers
    39 to 77 %, the less the more water carries over.
  - The residuals' variance has a floor, a scale step's square (0.1 kg)
    over 12, so weigh-ins on a straight line do not make the slope
    certain.
  - The intake average's, for the days not logged: they are taken to
    differ from the logged ones together, as the weekends of someone who
    logs only on weekdays would, so the error is a day's spread, at
    least a tenth of the average intake, times the share of days not
    logged, and none when every day is. Weekends
    never logged and 400 kcal higher bias the TDEE 114 kcal low; the
    interval still covers it in about 61 % of the simulated windows,
    where one that took the days not logged for a random sample covers
    41 %.
  - The bound is Student's t, with the degrees of freedom of both
    errors combined (Welch and Satterthwaite).
- **Trust:** an estimate is trusted when half its interval is at most
  200 kcal, and the window has the data an autopilot step needs, 4
  logged days and 3 weigh-ins a week ([limits.md](limits.md)), over its
  whole weeks counted back from its last day, and in its last 7 days: a
  slope that ends weeks before its last day says little of the weight
  now. Whole weeks, so someone who weighs in thrice a week passes in a
  window of any length, and each with a weigh-in: the slope cannot see
  into a week without one. The 200 kcal are a first value, to be checked
  against real data. With fewer than 6 weigh-ins at least a week apart,
  7 logged days, or 14 days, there is no estimate. Either way, the
  doubts say what is missing: `few_days`, `few_weigh_ins`,
  `few_logged_days`, `wide_interval`. Too few days is the only doubt of
  a window that has them, as only the days to come settle it.
- **Known limits,** for the real data to settle (#116): trusting only
  narrow intervals favours windows whose autocorrelation came out low,
  so trusted intervals cover less than 80 % when much water carries
  over; and the autocorrelation pairs weigh-ins in order, whatever the
  days between, so weigh-ins in clusters get too narrow an interval.
- **The day it is measured for** is yesterday, but a change of intake
  today counts: on a phase's first day, the estimate is the one from
  before it. The summary says the date of the last weigh-in the trend
  weight took, as a trend weeks old is not the weight now.
- **Verdict** (`calibration.Judge`): a recommendation appears only when
  the estimate is trusted, and of the phase's own days, not from before
  it, and the phase has a target. Otherwise the summary shows the
  estimate with its uncertainty, not just "calibrating", and the
  verdict's reasons (`no_phase`, `no_target`, `no_estimate`,
  `before_phase`, `not_trusted`).
- **Goal-agnostic rules.** Each phase has a target rate in % body weight
  per week, and the verdict a tolerance around it, ±0.1 % a week for
  every phase. The verdict compares the measured rate, the slope as a
  share of the trend weight, with that: `on_target`, `below` (the weight
  falls faster or rises slower than planned) or `above`. It proposes a
  kcal step toward it:
  - Too slow on a cut → lower the target.
  - Gaining too fast on a bulk → shrink the surplus.
  - Drifting during maintenance → nudge back.
  - The step comes twice: the gap in kcal a day the action asks for,
    which a coach works with, as a coach's proposal meets no limit of a
    step; and the step the autopilot's limits allow, the lesser of the
    user's `step_kcal` and `step_pct` of the weekly average
    (`limits.Limits.Step`, the one `Validate` holds a step to), which
    the autopilot (P1) takes. In P0 nothing applies itself.
  - Each phase type's default target rate, and the rates a user may
    set without a confirmation, are in [limits.md](limits.md), from the
    literature; the summary flags a maintenance phase whose trend
    strays 3 % from the one it started with.
- **Guards, by phase type:**
  - Water/glycogen window: the first 14 days of *every* phase (glycogen
    refills on a bulk start too), and of the days after a phase's
    planned end when no phase follows, are left out. A window needs 14
    days of the phase after them, so until the phase's 28th day the
    estimate is the latest one from before the phase, marked as such,
    with no verdict on the phase. It goes back past a phase too short to
    give one, as a diet break, but not past one with days enough and too
    little data in them, nor to a window that ends 42 days or more
    before: an estimate from before those would be too old.
  - The guards below come in order, and the first that trips takes the
    verdict's action.
  - Underweight: a loss under way below a BMI of 18.5, by the trend
    weight now, proposes maintenance, with the gap to a rate of 0 and
    no autopilot step ([limits.md](limits.md)). The loss is the plan's,
    as the limits read it (`limits.LossUnderWay`): the phase's rate, or
    a target that implies a loss with a trusted TDEE. So a cut whose
    weight stalled gets no deeper cut, and a bulk that loses still eats
    more. The summary flags such a loss with a verdict or without, and
    says when the profile has no height to tell.
  - Lean-mass guard (cut/recomp): ≥ 0.5 kg/week lean loss, the lean
    mass's slope over the window, beyond its error (80 %), turns a cut
    into "raise protein/calories", a step up. The lean mass needs
    measuring as the weight does: six times, from the first to the last
    at least a week apart. A day with a body fat and a weigh-in but no
    lean mass has the weight's share that is not fat.
  - Fat-gain guard (bulk): body fat % rising beyond its error (80 %,
    Student's t of its regression), with the lean mass rising less than
    0.1 kg a week → shrink the surplus, a step down.
  - NEAT guard: the last 7 days' steps ±30 % from the up to 21 days
    before them in the window, in either direction, holds the kcal: the
    change is the activity's, not the metabolism's. It needs 4 days with
    steps in the last 7 and 7 before; without them it is off, and the
    summary says so.
  - Strength veto (cut/maintain), with training in P3.
- **What the limits read** (`calibration.StateOf`): the trend weight now
  and when the phase started, the measured TDEE and whether it is
  trusted (a TDEE from before the phase may be: it is a TDEE all the
  same), and of the last 7 days the fat-free mass, the active energy and
  the days logged and weighed. The autopilot's own history is the
  autopilot's, the days since the phase last changed with it: until it
  counts them, they are 0, and its wait for a first step holds.
- **Split targets:** a change moves the weekly average and keeps the
  difference between training and rest days as it is; the split itself
  is the plan's, which the coach changes (§8).
- **The day's type** comes from the plan's training weekdays, so a day's
  target is known in the morning. A workout read from a source
  (until P3, Strong's export) or the check-in's toggle corrects it
  afterwards, the toggle having the last word, and calibration uses the
  corrected type. The weekly average weighs the day types by the number
  of training weekdays.
- **Protein in g/kg of a reference mass** instead of a fixed gram value:
  the trend weight, but at most the weight at a BMI of 30, as the
  sources' values are per kg of body weight. The target is stored in
  grams, and the limit is checked in g/kg; a floor in grams the user
  sets only narrows it.
- **Per-user parameters** for all of the above, within the limits
  ([limits.md](limits.md)). In P0 they are the same for everyone
  (`calibration.DefaultParams`); with the autopilot (P1), they become the
  user's.
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
| **Plan** | coach session: the user + a strong model (MCP, or the built-in `smart` slot) | on request, e.g. monthly | phase timeline, target rate, goal/milestone weight, protein g/kg, day-type split, profile notes, later the training program; a change of the user's limits or profile only as a proposal a web session applies |
| **Autopilot** | server, deterministic | weekly check-in | kcal within the user's limits of step and drift, from the calibration verdict (§7) |
| **Logging** | any client, `fast` model | many times a day | meals, metrics, check-ins — never the plan |

### Proposals, not direct writes

1. **Propose.** `propose_changes` submits a diff (phases, targets,
   settings, the profile, notes) with a rationale and the analysed
   window. The server validates it and returns a preview: daily targets
   per day type, the expected rate, any guardrail violations.
2. **Consent.** The user agrees in plain language ("može", "yes") and the
   AI calls `apply_proposal`. Pending proposals also show in the web UI
   with Accept/Reject, so a proposal made in Claude can be accepted in
   the app.
3. **History.** Applied proposals are the revision history: who (user,
   model name, client), when, why, before → after. `revert` creates the
   inverse proposal.

### Autonomy, per user

- `propose` (default): the autopilot and the coach only propose.
- `autopilot`: weekly adjustments within the user's limits of step and
  drift ([limits.md](limits.md)) apply automatically and are reported;
  past the drift limit, the autopilot proposes instead.
- A plan change never applies without consent.

### Guardrails live in code, not in prompts

The server cannot trust the model. MCP means *any* client and *any*
model, and a model can be talked into things, misread data, or follow
instructions injected through a label photo, a recipe or a web page
it read while researching. So limits are enforced by the server.

- **One function.** A pure, unit-tested `Validate(change, state,
  limits) → violations and warnings` (`internal/domain/limits`), the
  state being the profile and what the data
  say now (among them the trend weight now and when the phase started,
  the measured TDEE and whether calibration trusts it, the autopilot's
  drift and last step, the days logged and the weigh-ins), runs on
  `propose_changes` (the preview shows violations), again on
  `apply_proposal` (the data or the limits may have changed since the
  proposal), on every autopilot step, and on manual target edits in the
  UI.
- **Machine-readable violations.** A violation names the rule, the limit and the
  proposed value, e.g. `{rule: "kcal_floor", limit: 1800, proposed: 1500}`, the
  type of day for a limit of each, and the level that refuses it (the code, the
  instance or the user), so it is clear who could allow it
  ([limits.md](limits.md) lists the rules). The model can then explain the
  refusal and propose something allowed, so the loop corrects itself.
- **Limits**, among them (the full list, the default values and their
  sources are in [limits.md](limits.md)):
  - a kcal floor, for each day type, and a warning below the estimated
    BMR;
  - the maximum rate of loss and of gain (% body weight/week);
  - a protein floor and ceiling (g/kg), and a fat floor;
  - the maximum kcal step of an autopilot step;
  - the minimum time between autopilot changes, so a plateau cannot turn
    into cut-cut-cut, and the data a change needs;
  - the maximum cumulative autopilot drift from the last target the user
    agreed to.
- **Out of the AI's reach by construction, not by instruction.** No
  tool changes the limits or the profile they depend on. A coach may
  propose a change of either (`propose_changes`), but the proposal
  applies only from an interactive web session (cookie + CSRF), never
  through `apply_proposal` from a tool or a Bearer token. The settings
  endpoint takes only a web session too, so not even a leaked MCP token
  can relax them. The API refuses tokens on every operation in its
  OpenAPI document that does not open itself to a scope, so a new
  endpoint starts closed to them. The ingest is in the document, open to
  the `ingest` scope, and MCP runs each tool through its operation, so
  the same check holds there.
- **The user can change them** within the instance's limits, with an
  explicit warning, e.g. for a faster rate of loss. A low-energy diet
  under medical supervision, under 1200 kcal, needs the admin to lower
  the instance's floor; 800 kcal or less, nothing allows. Adults decide;
  models don't.
- **Three levels.**
  - A floor in the code that nobody can lower, the admin included: never
    800 kcal a day or less, never less than 0.8 g/kg of protein, never a
    rate faster than a loss of 1.5 % or a gain of 1 % a week, no plan
    for someone under 18, no change that starts or deepens a loss below
    a BMI of 18.5 (a first kcal target before calibration trusts the
    TDEE is not judged, [limits.md](limits.md)), and no goal or milestone
    weight below it; a loss
    under way that crosses 18.5 makes the autopilot propose
    maintenance.
  - Instance limits (admin, in config) within it are the outer bound for
    everyone on the instance. Someone hosting friends can set a floor
    nobody goes below.
  - Per-user limits within those, which start at the defaults:
    loosening one, up to the instance's limit and no further, takes an
    explicit confirmation in a web session.
- **A plan needs the profile.** The limits depend on sex, age and
  height (a kcal floor, the BMI, the estimated BMR) and on the trend
  weight, so without them `Validate` refuses a phase or a target
  (`profile_missing`, `weight_missing`), and the coach knows what to
  ask. Logging works without them. The profile changes only from a web
  session, as the limits do; the weight and the lean mass are
  measurements that any client logs.
- **Plans only, never logging.** Guardrails apply to targets and phases.
  Logging is never blocked or judged: a 4000 kcal day is simply logged.
- **Not medical advice.** The docs say so plainly. Other people will
  self-host this, so the defaults must be safe for someone who is not
  the author. For the cases for a clinician (limits.md lists them, e.g.
  a very-low-calorie diet, pregnancy, kidney disease), P0 has the
  floor in the code and the docs; onboarding questions come later.

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
  - `POST /api/v1/ingest/hae` is the Health Auto Export payload; `POST
    /api/v1/ingest/metrics` takes `{date, metrics}`; `POST
    /api/v1/ingest/strong` takes Strong's CSV export, the whole history,
    as the body or a form's part `file`, and keeps its days of workouts
    (`workout_days`), each workout on the logical day it starts in, as a
    meal; the latest export that read is kept past the 30 days, for P3 to
    read the sets from (`internal/ingest`).
  - All use a per-user `ingest` token, not a web session, and the
    bodies are archived in the data directory; a user's days older than
    30 go when their next body arrives.
  - A value belongs to the calendar date of its point, as HAE writes it;
    the day keeps its earliest weighing, with body fat and lean mass from
    the same weighing, and each value is checked on its own.
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
    `DATA_DIR`, `LOG_LEVEL`, `LOG_FORMAT`, the optional `OIDC_*`, later
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
    that brings a `feat`, `fix` or `perf` commit or a breaking change
    cuts a semver release automatically: a tag and a GitHub Release
    with the changelog, and, once deploys exist, the image.
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
- **Offline-ready IDs:** clients generate IDs (UUIDv7) for meals, their
  items and later sets, and writes are idempotent on that ID: the same
  write sent again returns what is stored, and other content under the
  ID is a conflict (409). IDs are per user (the key is the user and the
  ID), so another user's row under the same ID is not there for this
  one, and no answer tells it is.
  - "The same" is what the client said (a meal's `Fingerprint`: the time
    to the second, text, foods and quantities in order, an estimate's
    values), not what the server worked out, so a retry after a food was
    corrected is still a retry. A new food sent with an item is told by
    the food as sent (`meal.SentFood`, kept with the item), not by the
    food it is stored as, so a retry is the same however that food is
    renamed or corrected since, or whichever of two requests sent at once
    saved it. A retry is known before anything of it is worked out: it
    reads no food and saves none. A client gives the meal's `id`; when
    `log_meal` leaves out `eaten_at`, the time comes from that UUIDv7, not
    the clock, so a retry has the same one. A new meal's ID must be a
    version 7 ID whose time is near the clock (from a day before to five
    minutes after: a day of an offline client), so a model cannot reuse
    yesterday's meal ID or make one up; an ID already stored goes to the
    fingerprint instead, so an offline queue's late retry still finds its
    meal.
  - The fingerprint is the create's: an edit leaves it, so a late retry
    of the create (an offline queue that lost the answer) returns the
    meal as it is now.
  - An item keeps its ID across edits; a new item may carry the
    client's, and an "add" (which must carry one) of an item the meal
    already has under that ID is a retry: the same content changes
    nothing, other content is a conflict (409). A "replace" whose item
    is as sent already is a retry too. A server-made ID fills in for an
    item logged without one.
  - A deleted meal's ID stays a tombstone in the event log: a late retry
    of its create (or of a create that was undone) is refused as
    deleted (409) rather than bringing the meal back.
  - The future in-gym workout logger can queue writes offline and sync
    without duplicates.

## 10. Migration from the predecessor

1. **A one-off import of the history** (`tdeemon import-history`):
   - the predecessor's database, an index of the vault's daily notes →
     `days`, `metrics` and `meals` (historic meals become one estimate
     item each, and a day whose total is more than its meals gets the
     rest as one estimate more); a day with a remark, or without intake
     logged, is excluded, as the predecessor's calibration left it out,
     and the daily notes' mood, hunger and status are left out;
   - the vault's food table note → `foods`;
   - the vault's goals note, which is prose, through a plan file (JSON)
     the user writes from it and reviews → the profile, the training
     weekdays, `phases`, the `targets` history (from its revision log)
     and `profile_notes`; the current phase and target, and the ones
     after them, are held to the limits, the history before them is
     written as it was.

   Nothing is read from the predecessor or the vault after it: there is
   no shadow mode and no nightly re-import.
2. **New inputs for what the predecessor took in:** Health Auto Export
   directly (§9), the agent through `/mcp`, and Strong's CSV export
   (§9), whose days of workouts make the training days until P3 reads
   its sets from the latest export kept.
3. **Cut-over:**
   - the agent switches to `/mcp`, and its health skill shrinks to a few
     lines;
   - the HAE automation points at the new app; the Strong shortcut sends
     the export to both until P3, as the old service keeps the training
     side (5);
   - the agent's nightly rollover and morning-summary jobs are removed
     (tdeemon has no rollover and sends the summary itself).
4. **Obsidian stays** for other uses. The health folder is archived and
   nothing health-related depends on vault sync anymore.
5. **The old service keeps the training side until P3** (program
   prescription, training dashboard), but tdeemon reads nothing from it;
   the strength veto waits for P3. Its nutrition side is retired at the
   cut-over.
6. **The repo is public from the first commit** (AGPL-3.0-only, and
   contributions come under the same licence, with no CLA). GitHub Free
   only enforces rulesets on public repos, and a protected `main` is a
   requirement. **Personal data never enters the repo:** real-data
   fixtures and eval sets live outside git, and tests in the repo use
   synthetic or anonymised data.

## 11. Phases

| phase | scope |
| --- | --- |
| **P0 — replace the predecessor** | schema + migrations, foods/meals/days/metrics/phases/targets, tool registry, MCP (token auth), REST, HAE ingest, calibration v2, **proposals + coaching tools + guardrails**, a one-off import of the history, minimal web UI (day view, food DB, pending proposals, login), sign-in with a password or OIDC (Authentik), **deterministic morning summary + watchdog via Telegram**; training days from Strong's export; the agent on MCP; cut-over |
| **P1 — v0.1 open source** | multi-user + invites, built-in chat with BYO key (`fast`/`smart` slots) and photos, **autopilot weekly check-in**, MCP coaching prompts, PWA, morning summary + watchdog, recipes, i18n (en/hr), docker compose, docs, CI |
| **P2** | Telegram adapter, OAuth for Claude/ChatGPT connectors, shopping list, generic CSV history import, barcode → Open Food Facts lookup |
| **P3 — training module** | generic workout data (exercise, sets, reps, weight, RPE, duration, distance) + import adapters (Strong, others); **programs as declarative specs** over a library of progression rules, created by the coach through proposals (§8); the RTS program is the first spec; `training_day` derived from any workout source |
| **P4** | in-app workout logging (replaces Strong); native app with HealthKit (replaces HAE) |

Until P3, a training day for split targets comes from the plan's
training weekdays, corrected by a workout in Strong's export or the
manual toggle (`checkin.training_day`).

## 12. Open questions

- ~~Name~~ → **tdeemon**.
- ~~Observability~~ → deferred to a later phase: logs, `/healthz` and the
  watchdog until then; `/metrics` and dashboards later.
- ~~License~~ → **decided: AGPL-3.0-only, without a CLA.**
  Contributions come under the AGPL too (inbound = outbound). A CLA
  would mainly serve dual licensing, and a commercial or hosted SaaS is
  a v1 non-goal (§2). The cost: once outside contributions are in, any
  change of licence, a later AGPL version included, needs every
  contributor's consent. A CLA added later covers only the
  contributions from that point on; earlier ones need their authors'
  consent or a rewrite. The dependency licence policy stays
  (`docs/conventions.md`), so the dependencies' licences do not add to
  that cost.
- ~~Frontend~~ → **decided: React SPA embedded in the Go binary**
  (see §9).
- **Food DB scope.** Per user only, or optionally shared within a
  household.
- **Units.** Metric only in v1, stored in SI.
