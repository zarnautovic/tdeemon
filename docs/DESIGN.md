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
| `foods` | id, user_id, name, brand, basis (100g/100ml/portion), kcal, protein, carbs, fat, optional fiber/saturated_fat/sugars/salt/polyols/alcohol, portion_g?, source (label/estimate/user/dish/off/recipe), note, archived | the user's food DB; values per 100 g or 100 ml, or per portion for a food by the piece; one food, never a dish ("Foods" below); "same name + same brand" is the same food, which `upsert_food` corrects in place and a new food sent with a meal takes as it is |
| `food_servings` | food_id, user_id, name, amount | named amounts of a food by weight or volume: "piece" = 60 g of egg, "slice" = 40 g of bread |
| `food_aliases` | food_id, user_id, alias | "the dessert from last Sunday", "my bread" |
| `food_barcodes` | food_id, user_id, code | the product's barcodes, one for each size of its package, a scan of one finding the food; a barcode leads to one food of the user's; an EAN-13, EAN-8 or GTIN-14 by its digits, checked by its check digit, a UPC-A kept as its EAN-13 (a 0 before it) and a GTIN-14 of a 0 first as that EAN-13, so any form a scan reads finds it |
| `recipes` / `recipe_items` | food_id (the recipe's food), yield_g?, portions?, pot_g?; its ingredients as a meal's items (food_id, quantity as given, amount + unit, the nutrients) | batch cooking: a recipe is a food of `source: recipe` whose values per 100 g of the cooked dish are worked out of its ingredients and the dish's weight ("Recipes" below) |
| `meals` | id, user_id, day, eaten_at, description, client (mcp/chat/web/telegram/import), note, fingerprint | the fingerprint of the create, for idempotent retries (§9) |
| `meal_items` | id, meal_id, user_id, food_id?, name, brand (the food's, apart from its name), quantity as given (amount with its unit g/ml or without it for the food's own, or servings × serving, the serving left out for the food's one, or portions), amount + unit (g/ml/portion), the nutrients, source, note | **snapshot at log time**; editing a food later does not silently rewrite history |
| `metrics` | user_id, day, metric, value, source, updated_at | long format: weight, body_fat, lean_mass, steps, active_kcal, basal_kcal, sleep_h, rhr, hrv, vo2max, exercise_min |
| `days` | user_id, day, excluded_reason?, training_day?, mood?, hunger?, note? | check-in (mood and hunger 1–5); open and complete follow from the clock, only an exclusion is stored, with its reason; `excluded` drops a day from calibration (restaurant, sick, forgot to log); `training_day` is the check-in's toggle of the day's type, NULL when the plan decides (§7) |
| `targets` | user_id, effective_from, for training days and for rest days: kcal, protein, fat; proposal_id? | effective-dated rows; the why lives in the proposal; an unsplit target has the same for both; carbs are what the kcal leave after protein and fat, so a kcal step moves the carbs and keeps the grams of protein and fat |
| `proposals` | id, user_id, client + model, status (pending/applied/rejected/superseded/reverted), rationale (a coach's words, or the app's code), window?, changes (JSON: each change with the state before and after), reverts?, created_at, expires_at, decided_at + who decided, reverted_by? | every plan change (§8); applied ones are the revision log; a pending one past expires_at is expired, which follows from the time and is not stored |
| `plan_settings` | user_id, training_weekdays, limits?; later autonomy (propose/autopilot) and check-in weekday | the training weekdays give each day its type ahead of time (§7); the user's limits are a JSON object of their names ([limits.md](limits.md)) holding only those off their defaults, so a limit at its default follows a later release's default, and change only from a web session, never through tools (§8) |
| `phases` | user_id, start, type (cut/maintain/bulk/recomp), end_day?, rate_pct_week (signed), goal_weight?, milestone_weight? | the user's goal timeline, one phase per start, none overlapping another; a phase without an end lasts until the next starts; a phase start anchors the water/glycogen window; a diet break or a deload maintenance week is just a short `maintain` phase; goal and milestone may sit above or below the current weight |
| `profile_notes` | user_id, notes | free-text preferences and guidance (e.g. food dislikes, schedule constraints) handed to the AI as context; one text per user |
| `shopping_items` | user_id, text, done | |
| `events` | id, user_id, at, actor (user/import), client, model, operation, entity, before_state, after_state, undone_by, stale, part_of? | every write, with the state before and after (JSON); the audit trail, the tombstone of a deleted meal, and what `undo_last` takes back; `stale` marks a change undo passed over (§5); `part_of` names the change a write is part of (the new foods a meal brings, and an archived food it brings back, are its create's or edit's), which undo takes back with it and never alone; the states of the plan's writes have JSON names of their own, so a field added later reads older states |

A table of a user's rows carries `user_id` even when its parent row has
it too (`meal_items`, `food_servings`, `food_aliases`, `food_barcodes`),
with a foreign key on the pair, so a child can never belong to another
user's parent and the isolation test can compare every table by user.

**Foods.** The agent works out what was eaten from a short description
or a photo, finds it in the food DB, and saves what is new for next
time; the code does the arithmetic. A food is one food, a packaged
product or a plain one, never several eaten together (decided October
2026: a meal of several is a meal, its parts its items):

- **by weight or volume** (most of them): values per 100 g or 100 ml, so
  any amount eaten is one multiplication, half a bar as much as a whole
  one. Named servings turn "4 eggs" or "half a roll" into grams;
- **per portion**: a food judged by the piece (a muffin, a croissant),
  eaten whole or in part ("half a portion"). With the portion's weight
  known it can be logged in grams too.

A meal of separate things is an item each, in its amount: the agent
asks a part's amount the user gave no hint of ("some muesli"; "a
banana" is one), and logs its estimate of a part the user did not weigh
as an item of its own, beside the others, with no new food. A dish,
cooked at home or not, and a restaurant's or a canteen's plate are no
food: the agent logs each part the user names as an item, its estimate
of the part when nobody weighed it, with the portion it took in the
item's name ("Steak ~250 g"), and its estimate of the plate as one item
when the user names no part; it asks no amount nobody weighed (decided
on 26 October 2026: a part's estimate is no worse than the plate's, and
says what it took, so a correction changes that part alone); a dish the
user already has as a food is used as it is. Recipes are for a dish cooked
again: a recipe stores its ingredients and produces a food
of its own, with `source: recipe` (below). A food of `source: dish` or a dish
per portion, which the code still takes, is what an earlier client or
an import may have saved.

**Recipes.** A recipe is a food of the user's (`source: recipe`, by
weight), so search finds it and a meal logs it as any food; its values
per 100 g of the cooked dish are worked out of its ingredients and the
dish's weight (`internal/domain/recipe`), and change only with the
recipe: a save of its food as a food is refused, and so is a food of
`source: recipe` without one. Its ingredients are a meal's items, of
foods of the user's (no estimates), each with the values its food
brought when the recipe was saved: correcting a food later does not
change the recipe until it is saved again, and a meal logged keeps what
it had (decided October 2026). The dish's weight is optional, as a pot
may be too big for the scale: without it the raw ingredients' weight
(ml as g) stands for it, which makes a share of the dish exact and its
grams an approximation; one below 30 % or above three times the raw
weight is warned of, not refused. Its food has the serving `recipe`, the
whole dish, and `portion`, the dish divided into the portions the recipe
says, so "a tenth of the curry" is 0.1 of `recipe`. The empty pot's
weight is kept for the next weighing. A recipe may be an ingredient of
another, never of itself at any depth. Saving it again by its name
corrects it, as a food's does; the save is one event (`recipe.save`)
with the recipe whole before and after, and the new foods it brings are
part of it, so undo takes it back whole, its food kept, archived, while
a meal or another recipe has it. Tracking what is left of the pot is
for later. The web app has the recipes as a list of the foods' own; a
recipe's page says a portion's energy, and whether the dish was weighed
(≈ on its grams, and on what is per gram, when not, never on a portion
or a share); its editor is a page, its ingredients found as Log a meal
finds foods; "Cooked again" changes only the amounts, the weight (from
the pot, the empty pot remembered) and the portions, the last weight
never taken over. Log a meal finds a recipe as any food, marked, with a
portion's energy; its amount is a portion (when divided), a share of
the whole dish or grams, first as it was eaten last, else a portion, or
a quarter of an undivided one. A share is typed as a fraction ("1/10",
"2/3") or a number, or chosen in a chip (½, ⅓, ¼, and "1/…", which
waits for the denominator), and is sent whole: a third is 1/3, not
0.33. Grams of a dish not weighed are marked ≈, a portion and a share
never. Log on a recipe's page opens Log a meal on its amount, so the
share is chosen, not taken over. A share logged reads as the nearest
fraction of a denominator up to 12 ("⅒ of the recipe"), else a
number, and portions in the language's plural; a meal does not say
whether its dish was weighed, so neither shows grams there.

The same name and brand (compared without regard to case or runs of
space) are the same food: saving it again (`upsert_food`) corrects it
rather than adding a second, while a new food sent with a meal under
them is that food as the user has it, unchanged, so a model's guess
never overwrites a label's values. An alias leads to one food. Search
runs over names, brands and aliases through a full-text index that
ignores case and diacritics ("cevapi" finds "Ćevapi", and a đ is a d
or a dj: "grozde" and "grozdje" find "grožđe") and matches word
prefixes: foods whose name holds every word first, then name and
aliases, then the rest, shorter names first, from the user's own foods
alone; archived foods stay out of it but keep their history, and are
found alone in a search of the archived ones (`archived`).

**Barcodes.** A food keeps its product's barcodes, and a barcode leads
to one food (`food_barcodes`). A barcode looked up
(`lookup_barcode`) is the user's food of it, archived too;
without one, the product of [Open Food Facts](https://openfoodfacts.org)
as a new food, not saved, with its page there: the user sees it, and a
save of it (`source: off`, the barcode with it) makes it theirs, so the
next scan finds it without asking. A food of theirs of the product's
name and brand comes with it (`named_food`), whose barcodes lack the
barcode yet: the product as a meal's new food is that food, unchanged,
and adds the barcode to it, where a save of the product would correct
it. The web app's Log a meal scans one with the camera, or takes its
digits typed: the user's food opens for its amount (the last eaten),
the product as a new food to check against the label and add, the
food of its name gets the barcode, and one nobody has, or one a lookup
that failed may not tell, opens a new food of it, typed off the label;
a food's form adds one the same way, and takes one away. Digits typed
are checked as the server checks them (their length and check digit)
before they are sent. Chrome on Android reads it with its own
`BarcodeDetector`; Safari has none, and zxing-wasm reads it there
(`barcode-detector`, MIT), loaded with the camera alone, its WASM the
app's own, which the content security policy lets compile
(`'wasm-unsafe-eval'`). Only by barcode: Open Food Facts' search by
name is weak and allows 10 requests a minute; a user looks up 30
barcodes a minute there, so an
agent caught in a loop does not use up the instance's 100. Its names
are anyone's to write: clipped to a food's, and data to an agent, never
instructions (the MCP instructions say so). Its data are open under
the ODbL, so a client says where they come from. A product without a
name or its four values per 100 g or 100 ml is no food, and one the
user enters from the label keeps the
barcode as any food does; an optional value or a serving no food has is
left out. Open Food Facts keeps per 100 g and per 100 ml under one name,
so a product's values are per 100 ml only for a drink sold by volume
(an ice cream sold so is labelled per 100 g).
`TDEEMON_OPEN_FOOD_FACTS_URL=off` sends no request there.

A meal is a meal, not a row in the food DB: its items point at foods.
The optional nutrients are kept when known (a label) and used when
present; nobody has to enter them. Values are stored as they are and
rounded only for display (kcal to a whole number, grams to a tenth, salt
to a hundredth), which the nutrients and targets in the API's answers
already are: a total is the sum of what was eaten, rounded, and what
remains of a target is the target less the total as both are given, so
the three add up as the app shows them (2400 less 1079.5 kcal, given as
1080, leave 1320, not 1321). Amounts and metrics come as they are
stored.

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
answer, so a food log is **1–2 calls**: `find_foods`, then `log_meal`;
3 when a word `find_foods` did not find is searched once more in
another form (singular, without its ending, in English), which costs a
round (~8k input tokens, nearly all read from the cache) where a
duplicate food would stay in the list.

| tool | purpose |
| --- | --- |
| `get_profile` | the profile (sex, age, height), current phase and goal, current targets (day-type aware), profile notes, locale, units, and the time now on the user's clock (a model has none) |
| `find_foods(query[], limit, archived?)` | the foods whose name, brand or an alias has words starting with a query's, without case or diacritics, the name's first, among those in use (or the archived ones alone, with `archived`); a meal's foods at once, one query each, each food told by the queries that found it, and the queries that found none (`unfound`), so a model does not send them again (another form of the word, once); with the last use (the amount as given and in the food's unit) |
| `log_meal(id?, eaten_at?, description, items[], note?)` | stores the meal under the client's UUIDv7 (a retry returns it, §9), or one the server makes for a client that cannot (a model without a clock: that meal sent again is another); a meal is logged once eaten, so a time still to come (past the few minutes a clock may be ahead) is refused; returns breakdown + day status |
| `update_meal(id, eaten_at?, description?, note?, ops[])` | add/replace/remove items, give an item a note (a note op, which changes nothing else of it), change the time (not to one still to come) and the words; an add carries its item's id, made by the client, so one sent again adds once; an item no op touches keeps what it brought when logged; changes sent at once are each kept; returns breakdown + day status |
| `delete_meal(id)` / `undo_last(id?)` | `undo_last` (under an id the client makes, a UUIDv7 as a meal's, so the same undo sent again takes back no other change, §9; or one the server makes for a client that cannot, whose undo sent again takes back another) takes back the user's latest change not undone yet (an import's writes are left, and so are changes of the plan: the profile and the user's limits, which only a web session makes, and the phases, targets and training weekdays, which go back as a `revert` that `Validate` checks against the limits as they are now), newest first: undoing again goes further back, it does not redo. A meal logged or changed with new foods (an estimate saved as a food, too) is one change: its undo takes the foods back with it, whole or not at all. A change whose entity is no longer as it left it is not taken back (an import wrote it, and undoing would lose the import; or a later change undo passed over), nor one that would clash with what came since (another food took the name, an alias or a barcode, another meal an item) or that cannot be read or put back: the answer says why, and the next undo goes on to the change before. A failure of the database passes over nothing, so the next undo tries the same change |
| `get_food(id)` | a food by its id, archived too: what correcting the food in the way of a save starts from; a recipe's with its recipe whole (the dish's weight, raw and as weighed, its portions, the pot, the totals of the dish and of a portion, and its ingredients, each marked `changed` whose food gives another amount now), which `find_foods` and the list give without the ingredients |
| `lookup_barcode(code)` | what a barcode is ("Barcodes" in §4): the user's food of it (`food`), or Open Food Facts' product as a new food, not saved (`open_food_facts`), with the user's food of its name and brand (`named_food`); the instructions say how to log each, and that a code read off a photo by its digits can be misread past its check digit |
| `upsert_food(...)` | save or correct a food (label values); the same name and brand are the same food, and a name, alias or barcode another food has is refused naming that food; a barcode the user says is a food's goes in its barcodes |
| `upsert_recipe(id?, name, aliases?, items[], yield_g?, portions?, pot_g?, note?, archived?)` | save or correct a recipe ("Recipes" in §4): by its id (404 for none of the user's), or else its name, or a new one; its ingredients are `log_meal`'s items but estimates, a new food saved with it; a save replaces it whole, each ingredient taking its food's values now; a food of the name that is no recipe, or an alias another food has, is refused (409) naming it, and so is an ingredient that is the recipe at any depth (422); answers its food with the recipe, and warnings (a weight far off the raw ingredients', a new food's values that do not add up). `upsert_food` refuses a recipe's food but for archiving it |
| `get_day(date?)` | meals, totals, targets, remaining, metrics, check-in |
| `log_metric(kind, value, date?)` / `delete_metric(kind, date?)` | weight and other manual measurements, of today or a day before; a value the user gave counts over an import, and deleting a wrong one lets imports apply again; each answers the day |
| `checkin(date?, mood?, hunger?, note?, training_day?, excluded_reason?)` | what the user says of a day, each field given and only those: mood and hunger 1–5 (0 clears), a note, the day's type (`training`, `rest`, or `plan`: the training weekdays decide), and an exclusion from calibration with its reason (empty brings the day back); a day to come takes its type and exclusion, not a mood, hunger or note; one event, so one undo takes it back; answers the day |
| `get_summary(weeks?)` | the days up to yesterday: weekly averages (1 to 12 weeks, 4 by default), weight trend, measured TDEE with its interval and doubts, and on the way to one what it has of what it needs and about when it comes (§7), calibration verdict with its guards and flags, recovery flags (sleep under 6.5 h, resting heart rate 5 % over and HRV 15 % under the 21 days before the last 7, an average of at least 4 of the last 7 days and 5 of the 21) and the usual range of each, its base that much either way, and the days possibly incomplete (§4). A day reads as calibration reads it: meals by their day, an excluded day's weigh-in and steps, and a value of 0, as none. It reads in full the days calibration reads in full (`calibration.FullDays`: its window, the one before a phase it may fall back to, and the intake of the 14 days before a boundary that may begin one, 133 days) or the 12 weeks it may give, whichever are more, and of the days before them what calibration reads of the whole history: the weigh-ins and the first meal's day. Recovery reads today too, as a night's sleep is dated by the morning it ends |
| `get_history(from?, to?, granularity?)` | coaching: a day or week table of intake against target, the measurements, the trend weight and the TDEE measured up to each day, at most 366 days up to today (§8) |
| `get_plan()` | phases, target history **with outcomes**, the limits that hold, the pending proposal and the latest revisions, of the last 365 days and those to come (§8) |
| `propose_changes(rationale, model?, window?, phases?, targets?, training_weekdays?, limits?)` | a change of the plan, the user's limits among it (by name, applied only in the app), held to its rules (no change of the past) and to the limits in the transaction that stores it: one past them is refused (422), each violation an error detail with the rule, the day type, the limit, the value proposed and the level as its value; else it waits, replacing any other, and answers with a preview (each check, the target's weekly kcal and the rate it implies with a trusted TDEE, warnings); applies nothing |
| `apply_proposal(id)` / `reject_proposal(id)` / `revert_proposal(id, rationale)` | **apply only after explicit user consent**: held to the limits again (422), a proposal that waits no more is a 409, one that changes the limits applies only in a web session (403 through MCP or a token), with `confirm` for a limit it loosens (409 without; the web app's alone, which MCP neither shows nor takes), and applied again it answers as it is; recorded with actor and rationale. `revert_proposal` proposes the inverse of an applied one, as `propose_changes` does, while the plan is as it left it and its changes are still to come |
| `update_profile_notes(notes)` | preferences and life context the coach should remember, replaced whole (empty deletes them); undo takes it back |
| `shopping_list(op, items?)` | |

`log_meal` items accept these shapes (field names are illustrative; the
tool schema is the reference). An amount's unit must be the food's: 250
ml of a food by weight is refused rather than taken for 250 g. Left out
for a food of the user's (`food_id`), it is the food's, g or ml; a new
food's amount names it, as the food may turn out to be the user's by
another basis, and so does a food per portion, whose grams work only
when its portion weight is known. The unit is the item's, beside an
amount: a food has none, and servings or portions take none, but a new
food's servings with its own unit said again are its servings, as a
serving's amount is in that unit (a model says so often).

```jsonc
{ "food_id": "0199…", "amount": 150, "unit": "g" }           // known food, in its unit (g or ml)
{ "food_id": "0199…", "servings": 4, "serving": "piece" }    // a named serving: 4 eggs
{ "food_id": "0199…", "portions": 0.5 }                      // half a muffin
{ "food": { "name": "Greek yoghurt 2 %", "basis": "100g", "source": "label",
            "kcal": 73, "protein": 9.5, "carbs": 3.6, "fat": 2.0 },
  "amount": 200, "unit": "g" }                               // a new food, saved with the meal
{ "name": "Restaurant pasta", "kcal": 850, "protein": 32,
  "carbs": 105, "fat": 30 }                                  // a one-off estimate of a whole plate
```

The response holds the stored meal (items with computed macros), the day
totals against the day-type target, what remains, and `warnings`.
Warning examples: the stated kcal are more than 15 % (and 10 kcal) off
what the grams give by the Atwater factors (4P + 4C + 9F + 7 alcohol,
with fiber and polyols by any of the conventions labels use), saturated
fat is above total fat, a duplicate of a meal logged minutes ago. A new
food is saved in the meal's transaction (one of the name and brand of a
food the user has is that food, unchanged, and the item comes from its
values, or is a 422 when they do not take its quantity), so a meal
refused saves none, and a meal sent again saves none: it is told by what
was sent (§9) before anything of it is read or saved.

REST under `/api/v1` mirrors the same operations for the web UI and
scripts. A tool is one operation of the API, in the registry
(`internal/api/tools.go`): its MCP name, its REST route, its handler.
MCP runs a tool through that operation, so the validation, the token
check and the answers are REST's. A tool that reads takes a token with
the `read` or the `tools` scope; one that writes, only `tools`. So far:
`get_profile` is `GET /api/v1/me`; `get_day` is
`GET /api/v1/days/{date}`, a date or `today`, the user's day now (MCP
takes its `date` as optional, `today` when left out); `get_summary` is
`GET /api/v1/summary?weeks=`, of the days up to yesterday; `find_foods` is
`GET /api/v1/foods?query=…&query=…`, a query each repeated (MCP sends a
list of them as the parameter repeated); `get_food` is
`GET /api/v1/foods/{id}`,
archived too; `lookup_barcode` is `GET /api/v1/barcodes/{code}`;
`upsert_recipe` is `POST /api/v1/recipes`; and
`upsert_food` is `POST /api/v1/foods`, which corrects
a food by an id it has (404 for another) or by its name and brand, and
names the food in the way of a name, an alias or a barcode (409),
archived or not, which a search would not find but `get_food` gives.
Beside the tools, the web app lists the foods with
`GET /api/v1/foods/list`, a web
session's only: a page (`limit`, `offset`) of the archived foods or the
others (`archived`), of a source alone when asked (`source=estimate`,
the estimates to check, or `source=recipe`, the recipes), by name, by
their last use or by how many of
the user's latest 100 meals had them (`order=name|use|frequent`), each
with its last use, and how many there are (`total`); says on a food's
page how many of the user's meals had it and the last of them, the
amount it offers to log, with `GET /api/v1/foods/{id}/use`, a web
session's only too; makes a new food with
`POST /api/v1/foods/new`, also a web session's only, which refuses a
name and brand the user has (409, naming the food, archived too) where
`upsert_food` would correct that food, and a new recipe with
`POST /api/v1/recipes/new`, the same of a recipe's name; and offers
meals to log again
with `GET /api/v1/meals/recent`, a web session's only too: the latest
meals of the latest 200, each meal eaten again once (the latest, of the
same items in amounts within 1 %, as the duplicate warning tells them),
with when it and each time before were eaten (`eaten`, the instant and
the day), of which the app shows the meals of the same foods once, the
latest that a search matches, in whatever amounts. It offers them, and
the foods, by the meal of the day being logged (breakfast, lunch, a
snack, dinner, on the user's clock): first those eaten at it in the
last 14 days, the most often first, then the others, and last those the
day had already, as the meal logged a minute ago is not the next; with
nothing searched for, the first meals of it are cards above the search.
The slot and the order are the app's: the server says only when.
The charts read
`GET /api/v1/history?from=&to=&granularity=day|week`, the tool
`get_history`: every day of a span of at most 366 days up to today (the
90 up to it by default), or its calendar weeks, Monday to Sunday, the
last up to the span's last day, with what was eaten against the
day's target, the measurements, the trend weight and the TDEE
calibration measures up to each day, as the summary computes them, of
the days that are over: today's are the summary's, up to yesterday,
and its weigh-in joins the trend once the day is over (§7). It is the
table a coach looks back with (§8). `GET /api/v1/plan`, the tool
`get_plan`, is the plan a coach reads (§8): the phases and the targets
in force within the last 365 days and those to come, the limits that
hold, the proposal that waits for consent and the latest 10 revisions.
Each target's days up to yesterday come with their outcome: the intake
and the target of the logged days, and what calibration measures of
them (`calibration.Measure`, without the first 14 days after a change
of intake, which the 28 days before a target's first day tell too)
against the phase's planned rate. The pending proposal comes with its preview
(`pending_preview`), by the data and the limits now: what the limits
would refuse, if they changed since it was made, shows before the user
tries to apply it. The settings are a web session's
only: `PATCH /api/v1/me` changes the name, the language, the time zone
and the day's start hour, those given (what was logged keeps its day).
`PUT /api/v1/me/notes`, the tool `update_profile_notes`, replaces the
notes for a coach whole, an event undo takes back. `log_meal` is
`POST /api/v1/meals` (201, or 200 for a retry), `update_meal` is
`PATCH /api/v1/meals/{id}` (ops: add, replace, remove an item; an item
no op touches keeps what it brought when logged), and `delete_meal` is
`DELETE /api/v1/meals/{id}`; each answers where the meal's day stands.
A meal logged with a token is the client `api`'s until MCP names
itself. `log_metric` is `PUT /api/v1/days/{date}/metrics/{kind}`,
`delete_metric` is `DELETE` on the same route (404 for a kind the day
has none of), and `checkin` is `PATCH /api/v1/days/{date}`; each takes a
date or `today`, and answers the day as `get_day` does. `undo_last` is
`POST /api/v1/undo`: it answers the change it took back (marked
`repeated` for an undo sent again under its id: each undo takes a new
one) with the days it was of (a meal's before and after it, a day's or
a measurement's; none for a food or the notes), so a client showing
another day can say where it went; a 409 that says why for one it
passed over, or a 404 with nothing left. The web app's undo of a change
it just made (a toast's) names that change by its action and entity
(`last`, which MCP neither shows nor takes): it takes it back only
while it is the latest, and after another, made since through any
client, it takes back nothing (412). The app lists the latest changes
with `GET /api/v1/changes?limit=`, a web session's only: the user's
latest (10 by default, 50 at most), made through any client, each with
what it did and to what (its action and entity, as undo names them),
when, through what client and model, the days it was of, the names of
what it was of as the user's data has them (a meal's description and
its items', a food's name and brand, a measurement's kind, a proposal's
status after it), whether an undo took it back, and the one undo takes
back next (`next`), which the app's Undo there names (`last`). The
undos themselves are not among them, nor an import's writes, which undo
leaves and a device sends often enough to bury the user's own, nor the
new foods a meal brought, which are part of the meal's change. The
server gives codes and names, and the app the words.

## 6. AI layer

1. **MCP server** at `/mcp`, Streamable HTTP.
   - **v1:** auth with a per-user Bearer token. That is enough for
     Hermes, Claude Code and most self-hosted clients. The server is
     stateless and answers in JSON; it lists the tools the token's
     scopes open, each with the input schema of its operation (its
     parameters and body, standing alone) and hints of whether it only
     reads, replaces or deletes what is there, and may be sent again; a
     call runs the operation in process, with the token's principal, so
     a meal logged through MCP is the client `mcp`'s; a `date` left out
     is today. Its `instructions`, the same for every client
     (`internal/api/mcp_instructions.md`), are the policy of logging:
     log at once what is clear, ask one question only when the food or
     the amount is not, "estimate" logs nothing, show the breakdown the
     tool answers, and never work out a logged total or what remains,
     which the tools give.
   - **Later:** a minimal built-in OAuth 2.1 authorisation server (PKCE,
     dynamic client registration) so Claude and ChatGPT custom
     connectors work.
   - **Photos:** the image stays in the client; the model sends
     structured items.
   - **Cost:** every request of a log, a round of tool calls or the
     answer, carries the client's prompt, the tools and the
     instructions, so a log of two rounds sends them three times. The
     target is in two parts:
     - **A log pays under ~10k input tokens in full**, the cache warm:
       what a client that caches the prompt does not read from the
       cache. The first log after the cache has expired (five minutes
       with Anthropic's) pays for the tools and instructions once more.
     - **The tools and the instructions are a budget of their own**,
       which every request carries, cached or not: ~9.1k tokens as the
       eval measured them in October 2026, as a client that does not
       cache pays it in full with each request (~10.5k until October
       2026, ~6k then until the rules of a meal's parts, ~6.5k until the
       coach's history and notes, ~7k until the coach's plan). So a tool's
       schema carries what a model needs to call it, not the format of a
       number, examples or bounds, which the operation checks; a schema
       two tools take is said in full in its own tool's (a new food in
       `upsert_food`'s, an item in `log_meal`'s) and named in a word in
       the other's, beside the field's own words; a tool's description
       in MCP is its brief, what it does and answers, the policy of
       logging being the instructions', while its operation's
       description in the OpenAPI document says it all.
       `TestMCPFitsItsBudget` holds them to a budget in bytes. A schema
       named in a word is an object without properties, which a client
       that takes only the OpenAPI subset of JSON Schema (Gemini's
       function declarations, which do not take `additionalProperties`
       either) would refuse.

     The eval of MCP (`internal/eval`) measures both, and a log's whole
     input.
2. **Built-in chat** at `/api/chat`, used by the web UI/PWA.
   - Server-side agent loop over the tool registry, with vision for
     photos.
   - Provider is any OpenAI-compatible endpoint (OpenAI, OpenRouter,
     Ollama, …), with native Anthropic as an option.
   - **Two model slots:** `fast` for logging and `smart` for coaching
     sessions (§8).
   - **Keys are per user** (BYO), encrypted at rest with the instance
     secret. An admin may set an instance default.
   - **Cost target:** under ~10k input tokens per food log, as for MCP.
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
  TDEE as it fell. Where the weigh-ins' residuals of the 42 days carry
  over by 0.7 or more (water that holds for days, as a heavier body's
  does), the window is 63 days, as far as the last change of intake and
  the first data let it: such water leaves 42 days few independent moves
  of the weight, so a wide interval, and the longer window more. It is
  taken by the autocorrelation, not by which interval is narrower, so
  the interval still covers about 80 % (a simulation of a long history
  checks it).
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
- **On the way to an estimate,** the summary says how far the window is
  from one, so the first weeks of logging show something being built:
  what it has of each thing an estimate needs against the least it needs
  (`needs`: the days, the logged days between the first weigh-in and the
  last, the weigh-ins and the days between the first and the last), what
  a trusted one needs (`trust`: the widest ± and the user's weigh-ins
  and logged days a week), and about when a first estimate comes
  (`expected`, `calibration.Expected`): the first day whose window would
  have them, if the days to come are logged and weighed in as often as
  the last 14 were, counted by those shares, and as a phase that starts
  meanwhile moves the window; none when they had no meal or no
  weigh-in, or past 90 days. The app says these numbers, never its own:
  the counts ticked once met, the day, and of a TDEE not trusted yet, a
  sentence for each doubt.
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
  it, with a TDEE above 0, and the phase has a target. Otherwise the
  summary shows the estimate with its uncertainty, not just
  "calibrating", and the verdict's reasons (`no_phase`, `no_target`,
  `no_estimate`, `before_phase`, `not_trusted`, `no_tdee`).
- **Goal-agnostic rules.** Each phase has a target rate in % body weight
  per week, and the verdict a tolerance around it, ±0.1 % a week for
  every phase. The verdict compares the measured rate, the slope as a
  share of the trend weight, with that: `on_target`, `below` (the weight
  falls faster or rises slower than planned) or `above`. It proposes a
  kcal step of the target toward the weekly average that gives the
  target rate with the measured TDEE, so the proposal is about the
  target, not about what was eaten: a user who eats beside a target that
  holds is told to hold, and the verdict gives what was eaten and the
  target, so the summary can say the intake missed it; and a change of
  the target applied is not proposed again.
  - Too slow on a cut → lower the target.
  - Gaining too fast on a bulk → shrink the surplus.
  - Drifting during maintenance → nudge back, past the tolerance; a
    slower drift within it shows as a flag once the trend strays 3 %.
  - The step comes twice: the gap in kcal a day the action asks for,
    which a coach works with, as a coach's proposal meets no limit of a
    step, and which holds within the tolerance's kcal; and the step the
    autopilot's limits allow, the lesser of the user's `step_kcal` and
    `step_pct` of the weekly average (`limits.Limits.Step`, the one
    `Validate` holds a step to), which the autopilot (P1) takes. In P0
    nothing applies itself.
  - Each phase type's default target rate, and the rates a user may
    set without a confirmation, are in [limits.md](limits.md), from the
    literature; the summary flags a maintenance phase whose trend
    strays 3 % from the one it started with.
- **Guards, by phase type:**
  - Water/glycogen window: the first 14 days of a phase (glycogen
    refills on a bulk start too), and of the days after a phase's
    planned end when no phase follows, are left out when the intake
    changes there: when the logged intake after it, of 3 days or more,
    differs from the 14 days before it by more than a tenth, as a cut of
    1000 kcal or a reverse diet does. A step of 100 or 200 kcal moves no
    water to wait out, so the window runs on across it, and a first plan
    that names how the user has been eating keeps their history. Each
    side stops at the neighbouring phase's boundary, so a diet break of
    a week is held to its own days, not averaged with the cut after it.
    With fewer than 3 logged days after it, or none before, it is a
    change. A window needs 14 days of the phase after them, so until
    the phase's 28th day the estimate is the latest one from before the
    phase, marked as such, with no verdict on the phase, and the summary
    says that last day (`before_phase_until`). It goes back
    past a phase too short to give one, as a diet break, but not past
    one with days enough and too little data in them, nor to a window
    that ends 42 days or more before: an estimate from before those
    would be too old.
  - The guards below come in order, and the first that trips takes the
    verdict's action.
  - Underweight: a loss under way below a BMI of 18.5, by the trend
    weight now, proposes maintenance, with the gap to the TDEE, never a
    cut, and no autopilot step ([limits.md](limits.md)); a phase that
    does not lose, whose target does, stays and raises its kcal to the
    TDEE at least, more if its rate asks. The loss is the plan's,
    as the limits read it (`limits.LossUnderWay`): the phase's rate, or
    a target that implies a loss with a trusted TDEE, past the error of
    float arithmetic. So a cut whose weight stalled gets no deeper cut.
    The summary flags such a loss with a verdict or without, and says
    when the profile has no height to tell.
  - Lean-mass guard (cut/recomp): the lean mass's slope over the window
    at a loss of 0.5 kg a week or more, and a loss beyond its error (its
    80 % interval under 0), turns a cut into "raise protein/calories", a
    step up at least. The lean mass needs measuring as the weight does:
    six times, from the first to the last at least a week apart, and the
    last in the window's last 14 days. A window without a lean mass
    measured takes the weight's share that is not fat, on the days with
    a body fat and a weigh-in; one with both keeps to the measured, as a
    slope of the switch between them would be no loss.
  - Fat-gain guard (bulk): body fat % rising beyond its error (80 %,
    Student's t of its regression), with the lean mass rising less than
    0.1 kg a week → shrink the surplus over the TDEE, by a step at most
    and never below the TDEE. A guard keeps a larger gap of the rate's
    that points its way. A guard's action, from the window's body data,
    comes again while they stay as they are; the autopilot's interval
    spaces its steps.
  - NEAT guard: the last 7 days' steps ±30 % from the up to 21 days
    before them in the window hold the kcal when the change explains the
    miss the action would correct: fewer steps a slower loss or a faster
    gain, more steps the other way. The change is then the activity's,
    not the metabolism's; a target that catches up with the intake is
    no such miss. It needs 4 days with steps in the last 7 and 5 before;
    without them it is off, and the summary says so (`no_steps`), as it
    does when the body is not measured enough for a guard the phase has
    (`no_lean_mass`, `no_body_fat`).
  - Strength veto (cut/maintain), with training in P3.
- **What the limits read** (`calibration.StateOf`): the trend weight now
  and when the phase started (the day before it, from a weigh-in at most
  14 days older, else the phase's first weigh-in), the measured TDEE and
  whether it is
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
- **Test fixtures:** an anonymised history of 13 weeks lives in the repo
  (`internal/domain/calibration/testdata/history.json`). Real data stays
  in a private fixture outside git and is checked against past manual
  target revisions. From one measured day to the next measured one, the
  estimate moves at most 150 kcal while its window's data follow the
  same change of intake (a phase's start, or the day after a planned end
  with no phase after it); the switch from the estimate before a phase
  to the phase's own is another window. Its
  interval holds the TDEE each revision was made on. The first verdict
  within a week of each takes the revision's action, or the action the
  fixture says it takes instead, and why; or there is none, for the
  reason each verdict of the week gives (docs/conventions.md).

## 8. Coaching: the AI changes the plan

Users need not tune numbers by hand. They say what they want ("start a
lean bulk in January", "I've plateaued, what now?", "make weekends
easier") and the AI changes phases, goals and targets, like a private
coach or nutritionist. For a one-off session like that, the right tool
is a strong model through MCP, paid for by the user's own
Claude/ChatGPT subscription. The user may still change the plan in the
app (decided on 29 October 2026): edit the phase, plan the next one, and
set the targets by type of day, each change a proposal of the user's
own, applied at once (below).

| layer | who | how often | may change |
| --- | --- | --- | --- |
| **Plan** | coach session: the user + a strong model (MCP, or the built-in `smart` slot); or the user alone, in the app | on request, e.g. monthly | phase timeline, target rate, goal/milestone weight, protein g/kg, day-type split, profile notes, later the training program; a change of the user's limits only as a proposal a web session applies (the profile the user sets) |
| **Autopilot** | server, deterministic | weekly check-in | kcal within the user's limits of step and drift, from the calibration verdict (§7) |
| **Logging** | any client, `fast` model | many times a day | meals, metrics, check-ins — never the plan |

### Proposals, not direct writes

1. **Propose.** `propose_changes` submits a diff (phases, targets, the
   training weekdays, and the user's limits, which only a web session
   applies) with a rationale and the analysed window. The
   server validates it and returns a preview: daily targets per day
   type, the expected rate, any guardrail violations. Each change
   carries the plan as it is before it, and keeps the past as it was: a
   target takes effect from today on, and a phase that has started
   keeps its type and the days it has had. The notes for a coach are no
   part of the plan: `update_profile_notes` writes them, and undo takes
   them back.
2. **Consent.** The user agrees in plain language ("može", "yes") and the
   AI calls `apply_proposal`. Mostly a chat ends so; a proposal it left
   undecided waits, shows in the web app's plan (its section) with
   only what it changes, each value from what it was to what it is to
   be, who made it and when, how long it waits, what the data say
   beside it (the pace calibration measures against the phase's range,
   how far its TDEE is trusted, and whether they ask for the change),
   its preview by the data now, Accept and Reject, so a proposal made
   in Claude can be accepted in the app; while it waits, the plan's
   section has a dot and the day a line of its gist, and the morning
   summary says it waits. One that changes the user's limits is
   accepted there alone, after the user's yes in the chat too, and one
   that loosens a limit asks first, as the limits' form does; taking
   back a limit a revision tightened says that it loosens it.
   **The user's own change** in the app (`POST /api/v1/plan/changes`,
   a web session's alone) is a proposal of the user's, made and applied
   in one transaction: the same rules and limits hold it, a violation
   said at its field and one of the user's own limits loosened in their
   form, and it is a revision as any other, which the history shows and
   a revert takes back. Its rationale is a code, `changed in the app`,
   which the app says in the user's language, as it takes a revision
   back with `taken back in the app`: no one types why.
3. **History.** Applied proposals are the revision history: who (user,
   model name, client), when, why, before → after. `revert` creates the
   inverse proposal, each change put back as it was before; applied, it
   marks the one it takes back reverted. It needs the plan as the
   proposal left it, so no later change is lost, and changes that keep
   the past as it was: a change that took effect moves on by a new
   proposal. The web app shows the plan as it holds today (the phase and
   the target in force, by their numbers, the training days, and what is
   to come), the history, and takes a revision back
   in one step: the user, there, consents to the inverse, which applies
   at once (a revert the limits refuse waits, as any proposal).
4. **Lifetime.** A proposal waits a week for consent, then expires. It
   is superseded when the plan changes under it (any write of a phase,
   a target or the training weekdays, an applied proposal included, and
   for one that changes the limits, a write of them), or when a newer
   proposal replaces it, so one proposal waits at a time
   and what it showed is what applies. Undo leaves proposals, as it
   leaves the plan.

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
  `propose_changes` (a violation refuses it), again on
  `apply_proposal` (the data or the limits may have changed since the
  proposal), on every autopilot step, and on manual target edits in the
  UI. A proposal of several changes is held as `proposal.Changes.Check`
  splits it: a change of the user's limits against the instance's (one
  past them, or a minimum past the instance's maximum, a violation of
  the instance's level, named by the limit),
  and the rest against the limits as it leaves them, so a faster cut
  comes with the limit that allows it: each phase it sets as a change of
  the current one, and the
  target in force from each day it changes one from, against the one in
  force then before, in the phase in force then as it leaves it (a new
  phase starts at the trend weight now, which takes today's weigh-in
  too), with the training weekdays it leaves (new ones move
  a split target's weekly average, so they check today's target and each
  to come); with the data and the limits of now for a change ahead too.
  The check reads them in the transaction that stores or applies the
  proposal, which holds the one write connection, so a change of the
  limits meanwhile cannot slip past it. Through MCP, a refusal's text
  carries each violation as its JSON.
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
  propose a change of the limits (`propose_changes`), never of the
  profile, but the proposal applies only from an interactive web session
  (cookie + CSRF), with the confirmation a loosening takes, as the
  limits' form asks it, never through `apply_proposal` from a tool or a
  Bearer token; MCP does not even take the confirmation. The settings
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
  target period: the intake against the target, and the TDEE and the
  rate calibration measures of the period's days, leaving out the first
  14 after a change of intake as `get_summary` does; the limits that hold; the pending
  proposal and the latest revisions, each with its changes before and
  after.
- `get_summary`: the deterministic calibration. The server computes, the
  model interprets.

### MCP extras

- **Prompts as ready-made sessions** ("Monthly review", "Plateau check",
  "Plan a new phase"). They show up as slash commands in clients and
  load the right context.
- **The methodology as an MCP resource**, i.e. how TDEE and verdicts are
  computed, so the coach understands the numbers it reads.
- **Tool annotations:** `readOnlyHint` on reads and `destructiveHint` on
  `apply_proposal`, so clients ask before writes; `revert_proposal` only
  proposes.

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
- **Scheduler:** a minute's tick runs the jobs due in each user's day,
  on their wall clock (a job at 10:00 is due from 10:00 for a while, so a
  server down then catches up, within the day); each occurrence (such as
  the day) once in the usual case, recorded in `job_runs`, so a restart
  does not run it again, and at least once in any: a crash between a
  run and its record runs it again; a job that fails is tried again
  after 10 minutes. It also refreshes the query
  planner's statistics once a day (`PRAGMA optimize`).
- **Telegram:** the instance's bot (`internal/telegram`) sends, and
  reads its messages by long polling (`getUpdates`, no webhook, as the
  instance may be private); a user's chat is in `telegram_chats`, which
  an admin sets (`tdeemon user set-telegram`, after the user starts the
  bot) until the web app's settings link it with a code. The commands
  (`internal/api/commands.go`): `/sazetak` (`/summary`) sends the morning
  summary as it is now, `/pomoc` (`/help`) lists them; only a chat in
  `telegram_chats` is answered, and a command writes nothing. The
  adapter with the agent loop (§6) will take the bot's other messages.
- **Morning summary:** a deterministic template (sleep, yesterday vs
  target, weight trend, calibration line, a proposal of the plan that
  waits for an answer, recovery flags), the
  scheduler's job at 10:00 on the user's clock, sent whole to their
  Telegram chat and in a line to each device by Web Push
  (`internal/api/morning.go`, from the same reads as `get_day` and
  `get_summary`); a device's tap opens the whole on the day
  (`/?summary=date`, `GET /morning`, read when opened, so not stored).
  The sleep is only the night that ended today; what did not come is
  left out. In the user's language: English, or Croatian for `hr`. No
  LLM; an optional AI comment is off by default.
- **Web Push** (`internal/push`): a device's subscription is in
  `push_subscriptions`, one per endpoint, of the user signed in when it
  subscribed. The message is encrypted for it (RFC 8291) and signed by
  the instance's VAPID key (RFC 8292), derived from the secret key; it
  goes only to the push services' hosts, following no redirect, and a
  subscription they no longer have (404, 410) is deleted.
  The web app's service worker (`web/src/sw.ts`) shows a push and
  opens its page at a tap, and subscribes again when the browser renews
  a subscription; the app turns them on at a tap (the browser's
  permission asked then), and tells the server of the subscription each
  time it opens, as iOS can drop one. A device turned off is remembered
  in the browser, as the permission stays given; signing out turns the
  device off first, at the server and in the browser.
  `internal/notify` sends a notification to every channel of a user's,
  Telegram's chat and each device, and fails (so the scheduler tries
  again) only when none took it. An inbox of the notifications comes
  when there are more kinds of them.
- **Watchdog** (`internal/watchdog`), the scheduler's job each hour: no
  metric for more than 26 h, an ingest token unused for more than 3
  days. It goes out through the same channels (`internal/notify`, which
  follows a group to its supergroup), a tap on a device opening the
  devices in the settings, once per stop: the stop is
  recorded by when the data last came. Later, configurable per user, and
  nothing logged by a set hour.
- **Deploy:**
  - Docker image with one volume, `/data`.
  - Env, all prefixed `TDEEMON_`: `BASE_URL`, `SECRET_KEY`, `LISTEN`,
    `DATA_DIR`, `BACKUP_KEEP`, `LOG_LEVEL`, `LOG_FORMAT`, the optional
    `OIDC_*`, `LIMIT_*`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_API_URL` and
    `OPEN_FOOD_FACTS_URL`; plus the standard `TZ`. The README has the current table.
  - Ships a `docker-compose.yml`; the homelab gets its own manifests.
- **Backup:** a built-in daily `VACUUM INTO` snapshot with retention
  (`TDEEMON_BACKUP_KEEP`, 7 by default; plus one before every migration),
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
    with the changelog, and the image on ghcr.io.
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
  - Tailwind + shadcn/ui on Base UI (components owned in-repo). The
    look is design tokens in `web/src/index.css`: a light and a dark
    theme, the system's or one chosen in the browser
    (`web/src/lib/theme.ts`), the macros' own colours, which no colour
    that means something (the brand's, a warning's, a destructive's, a
    day's over its target) is near in either theme
    (`web/src/tokens.test.ts`), and Geist,
    self-hosted (Fontsource). The sections (today, the foods, the
    progress, the plan) are a bar at the bottom on a phone, with Log a
    meal in its middle, and a sidebar from 1024 px, with the settings at
    its foot. Log a meal (and its key, N) opens the meal to log on today
    over the screen shown, which stays as it was, and the toast of the meal
    logged leads to the day; on today's page it is the page's own, so the
    meal's row takes the focus. On a phone the settings are a list under the
    plan, beside what a plan rests on of the user's (the profile, the
    limits, the notes for a coach), each row with its value in short and a
    page of its own. The keyboard has shortcuts (`?` lists them): N logs a
    meal, / finds a food, ⌘Z or Ctrl+Z outside a field takes back the
    latest change (made anywhere, which its toast says), G and a letter
    goes to a section (D, F, P, L, S, which the sidebar shows as the
    pointer or the focus is on one), and ← → to the day before or after;
    the first stop of Tab skips the sections to the page. On a desktop, the
    day's meals are one stop of Tab (a roving tabindex): ↑ ↓ Home End move
    between the rows, and on a row Enter opens its editor (a click or Space
    opens it to its items, as a tap does), R repeats it today and Delete
    deletes it. A meal just logged or repeated takes the focus to its row,
    which a screen reader says. A change says it is done in a toast (Base
    UI), one at a time, with what the server warned of it and Undo, which
    takes back that change only (`last`, §5); a delete asks nothing first,
    and its toast stays 10 s, as one that warns, 6 s the others; none goes
    while the pointer or the focus is on the toasts, or a finger touched
    them last. After the toast, the recent changes (a sheet from the day's
    header and from the settings) list the latest changes, made by the
    user, their AI coach or a device, each said as its toast said it, with
    Undo on the one undo takes back next. The toasts are over a sheet too,
    so a change made in one is taken back there, and clear of its buttons,
    which a tap under them would miss: at the top of a phone, beside the
    panel on a desktop; F6 takes the focus to them from a sheet, which
    keeps Tab within it. A sheet is Base UI's Drawer: from the bottom on a
    phone, with a handle, dragged down to close (from anywhere its list is
    at its top), and at the right on a desktop, dragged away by its title;
    its × stays, for the keyboard and a screen reader. What a finger taps
    is 44 px or more either way, but a link in a sentence. A sheet has one
    primary button at a time: the quick add's new food (in short, its other
    fields under More and on its page) and its estimate are a step of their
    own, in the search's place, with Add to meal alone. The quick add's
    search takes the focus as it opens, but on a touch screen (`pointer:
    coarse`), whose keyboard would cover what it lists, only when tapped.
  - The day judges nothing it shows (§8: logging is never judged; the
    plan works on the week's average). It has what was eaten against
    its target in a ring and the macros' bars, beside the TDEE and what
    is left of it after what was eaten, live through the day (a day
    before: how far under or over it); until there is a TDEE, in its
    place, calibration's counts against what it needs and about when a
    first comes (§7). Within 5 % of the kcal target
    either way, and 100 kcal at least, a day is on it, in the brand's
    colour; past that, the ring, and the week's mini rings and bars
    above the target's line, run on in a warm neutral (`--over`),
    thinner, so it shows without the colour too, with the words "+180
    over target": red is for errors and deletes. Protein's target is a
    floor, said reached; carbs and fat past theirs say only how far. A
    day before over its target says it in a sentence beside its TDEE
    ("680 over target, still 500 under TDEE"). The plan says the deficit
    or surplus its targets plan in words, never beside a negative
    number. The TDEE and its ±, how far it is trusted, the trend weight,
    the deficit and the verdict are each an ⓘ (`InfoTip`) away from what
    they are, in two or three plain sentences drawn from §7.
  - The day's week is the calendar week, Monday to Sunday, as the
    progress's weeks are: its days, each what it ate against its
    target, and a line of their average over the days logged, as
    calibration counts them (a day without a meal, one left out, and
    today while it is open are not), against their target ("This week:
    average 1,905 of 2,079 · 174 under a day · 5 days logged"). The
    line opens the days in a sheet, and of this week what the days
    left, today's among them, can eat on average and keep the week on
    its target, by the plan's targets of the days to come; below the
    plan's kcal floor, the day's target is said instead. A phone has
    the week above the day. A desktop has the day in 8 of 12 columns,
    the hero's numbers side by side and the meals a table of their kcal
    and macros with the day's total, and beside it the week as bars with
    the target and the average, the body, the check-in and the phase.
  - A new user's first steps take the place of the day's hero until
    there is a plan, or a proposal of one: a card of four, each ticked
    once done (the profile; a first weigh-in, in the card; an AI coach
    or a phone that called with its access key; a first meal), and,
    once the first three are, what to ask the AI coach for a plan, to
    copy. Put off for later, in the browser, the plan's page brings it
    back. Until the user has measured anything, the check-in is a line
    that asks how the day was, and the body has no empty tiles, only the
    way to a first weigh-in. A day before the plan's first target says
    when the plan starts, not to set one up.
  - A weigh-in is a tap away: each of the body's tiles opens its
    measurement (the weight in a field in its place, hinting with the
    day before's; the others in the measurements' sheet, at their own),
    the quick add has a Weigh-in chip, and in the morning, while today
    has none, the hero asks for it in a line, unless the day before's
    came from a device (a smart scale's user is not asked).
  - Connecting an AI coach or a phone is a guided sheet: what it is,
    its access key (an API token; the web never says "token") with the
    scope its kind takes, and the steps to give the key to it, shown
    this once: the instance's MCP address (its base URL, which
    `GET /api/v1/info` says), Claude Code's command, Claude Desktop's
    configuration, or Health Auto Export's checklist. Then it says when
    the client first calls, by the key's last use, which it asks the
    server for every two seconds until it comes.
  - ECharts: `echarts/core` with only the charts it draws and the SVG
    renderer, loaded with the progress's page (a chunk of its own), not
    with the app. A chart's colours are the theme's tokens, read from the
    page (`getComputedStyle`) and again when the theme changes; the dark
    theme's series are health-api's validated palette (`palette.ts`), its
    warning and destructive moved off the macros' amber and pink. A chart has
    one axis of values and a legend of its card's; to a screen reader it
    is an image with a summary, and the weeks' table says the charts in
    text.
  - The progress leads with the verdict on the phase, or, until the TDEE
    is reliable, with calibration on its way to it (§7); without a phase
    the TDEE still leads, the way to a plan after it. The weight's axis
    holds the weights: a goal far off them is said, not drawn. The TDEE
    and intake chart shades the space between the TDEE and the last 7
    days' intake, and says what it comes to ("You eat about 500 kcal under
    what you burn: about −0.5 kg a week"), at 7700 kcal a kg, the
    server's (`limits.json`). The weeks are calendar weeks, Monday to
    Sunday, as everywhere in the app. Sleep, the resting heart rate and
    the HRV in small are the last 7 days' average against the usual (the
    summary's recovery), in words and an arrow, a line of the 7 days'
    average in a neutral colour over the usual range; each tile is one
    button to its whole chart, and what recovery flags is a sentence of
    the hero's. Sleep is hours and minutes everywhere ("6 h 52").
  - Loading and a bad network: a page that loads is a skeleton of its
    shape under its header, never a bare "Loading…". The day waits for
    what it shows above its meals as well (the user, the summary, the
    plan, the week's history and today's chips; `useDayReady`), its parts
    there but hidden, so each is asked once, and then shows all at once:
    nothing comes in above the meals after the first paint to push them
    down under a finger. Once it has shown, another day keeps the one
    before until it comes. What did not load says so in its place, within
    the page's frame (its header and the navigation stay): what did not
    ("Couldn't load today."), why, and Try again, not in an error's red.
    What came stays when asking for it again fails (TanStack Query's
    error with data), under a thin bar over the page that says since when
    ("Offline · data from 21:14", or "Not updated" when the server
    failed); a refusal (4xx) is not the network's, and its part says it.
    A section opened while the session cannot be asked again opens on the
    one the app has. A reload without a network opens the app's shell
    from its service worker (the PWA, below), which says the server is
    out of reach; the last day from a persisted cache, and meals logged
    offline queued until the network is back, are still to come.
  - i18n without a library: each language's text is a typed object
    (`web/src/i18n`), so a key missing in one language fails the type
    check. Paraglide JS was the plan, but its compiler loads plugins
    from a CDN at build time and pulls ~500 packages, one without a
    licence on npm; two languages do not need that.
  - **The app installs** (a PWA, `vite-plugin-pwa`): a manifest with
    the brand's icons (`web/icons/generate.sh` draws them, a maskable one
    and iOS's apple-touch icon among them), and a service worker that
    precaches the shell (the page, scripts, styles, fonts and icons) and
    answers a navigation with it, so the app opens without a network.
    It keeps no data: the API, `/mcp` and `/healthz` go past it, and a
    sign-in's redirects with them. The app registers the worker itself
    (`web/src/lib/service-worker.ts`), once the page has loaded, and asks
    for a new one each time it comes into view and every hour, as an app
    on a phone brought back has no page load to make the browser ask. A
    new version does not reload the page under the user: a toast says it
    is ready, and its Reload takes it and loads that page alone
    (`UpdatePrompt`); another tab keeps its page, and its own toast. The
    server serves the manifest as `application/manifest+json` and the
    worker revalidated, so a check finds a new one at once. Web push
    comes with it later.
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
    reads no food and saves none. A client gives the meal's `id` (one
    that cannot make one, a model without a clock or a tool to run code,
    leaves it out, and the server makes one: that meal sent again is
    another, which the warning of a duplicate tells, or a 409 when its
    items carry ids of their own; MCP says such a tool is not safe to
    send again); when
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
  - An undo carries an ID the client makes too (or, from a client that
    cannot, one the server makes), under the same rules: the
    change it settled (taken back, or passed over) keeps it, and an undo
    that found nothing is recorded with it, so the same undo sent again
    answers as before rather than taking back another change.
  - A deleted meal's ID stays a tombstone in the event log: a late retry
    of its create (or of a create that was undone) is refused as
    deleted (409) rather than bringing the meal back.
  - The future in-gym workout logger can queue writes offline and sync
    without duplicates.

## 10. Migration from the predecessor

Done at the cut-over, on 2026-10-29.

1. **A one-off import of the history**, at the cut-over:
   - the predecessor's database, an index of the vault's daily notes →
     `days`, `metrics` and `meals`: each historic meal split into the
     user's foods where its text told them (a part with an amount and a
     food of the user's became an item of that food, the rest of the
     meal an estimate of its own, so the meal kept its total), else one
     estimate; a day whose total was more than its meals got the rest
     as one estimate more; a day with a remark, or without intake
     logged, excluded, as the predecessor's calibration left it out;
   - the vault's food table note → `foods`, a row of several foods
     eaten together as a plate, per portion;
   - a plan file written from the vault's goals note → the profile, the
     training weekdays, `phases`, the `targets` history and
     `profile_notes`.

   The import (`tdeemon import-history`, `internal/history`) served the
   maintainer's history alone and was removed after the cut-over; git
   history has it. The app still says the name it gave the rest of a
   meal ("Rest of the meal (history split)") in the user's language.
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
