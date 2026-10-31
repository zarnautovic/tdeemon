# How tdeemon reads the numbers

This page is for a coach, an AI model or a person, who reads what
tdeemon answers: how the trend weight, the measured TDEE, the verdict on
a phase and the autopilot come about, and what they do not say. The
server computes every number here; a coach explains them and never
computes them again. The tools give this page with each coaching session
(`get_coaching_guide`), and MCP serves it as a resource. The design
behind it, with the reasons, is in [DESIGN.md](DESIGN.md) §7 and §8, and
the limits with their sources in [limits.md](limits.md).

## The trend weight

- A day's weigh-in moves by a kilogram or more with water, salt,
  glycogen and the gut. The trend weight smooths that: each day it takes
  a tenth of the weigh-in's difference from it (after a gap of n days,
  1 − 0.9ⁿ of it). It lags the weight by about 9 days.
- The trend weight is the weight the summary, the plan and the limits
  read (`trend_kg`). A single weigh-in, or a week's average, says little
  on its own: never read a change of the plan into it.
- An excluded day (a day the user took out: sick, a trip, a restaurant
  meal not weighed) counts for nothing, its weigh-in neither.

## The measured TDEE

- **Window:** the 42 days up to yesterday (today is not over), none
  before the user's first logged meal; 63 days when the weigh-ins carry
  water over for days, as a heavier body's do.
- **Intake:** the average of the window's complete, not excluded days
  with at least one meal, between its first weigh-in and its last. A day
  without a meal was not logged: its intake is unknown, not 0.
- **Slope:** a linear regression of the weigh-ins in the window, in kg a
  week (`kg_per_week`, a minus a loss).
- **TDEE** = intake − slope × 7700 / 7, in kcal a day: what was eaten,
  corrected by the energy the weight's change stored or released.
- **Interval** (`half_width`): an 80 % interval, from the slope's error
  (widened as weigh-ins on following days are not independent) and the
  intake's (the days not logged). A TDEE of 2600 ± 150 is somewhere
  between 2450 and 2750 four times in five. Two estimates whose
  intervals overlap do not differ.
- **Days not logged** are taken to be like the logged ones. When they
  are not (a weekend eaten but never logged), the TDEE comes out too
  low; no interval can tell. Logging an estimate of a day beats leaving
  it out.
- **No estimate** with fewer than 14 days, 7 logged days or 6
  weigh-ins at least a week apart.
- **Trusted** (`trusted`): half the interval is at most 200 kcal, and
  each whole week of the window, and its last 7 days, has the logged
  days and weigh-ins an autopilot step needs (4 and 3 by default, the
  user's limits `logged_days` and `weigh_ins`).
- **Doubts** say what is missing: `few_days`, `few_weigh_ins`,
  `few_logged_days`, `wide_interval`. On the way to a first estimate,
  `needs` counts what the window has against what it needs, `trust`
  what a trusted one needs, and `expected` about when a first comes.

## Water and glycogen after a change

- When the intake changes by more than a tenth (a cut starts, a diet
  break, a reverse diet), water and
  glycogen move the weight for about two weeks. The first 14 days after
  such a change are left out of the window. A step of 100 or 200 kcal
  moves no water, and the window runs on across it.
- A window needs 14 days of the phase after them: until the phase's
  28th day, the estimate is the latest from before the phase
  (`before_phase`, until `before_phase_until`), and there is no verdict
  on the phase yet. This is no stall: the phase has not had the days to
  be judged.
- Each target in `get_plan` says what its days came to (`outcome`), its
  `measured_from` past those 14 days.

## The verdict on a phase

- A phase has a target rate in % of the trend weight a week
  (`target_pct_week`, a minus a loss). The verdict compares the measured
  rate (`measured_pct_week`) with it, ±0.1 % a week
  (`tolerance_pct_week`): `on_target`, `below` (a faster loss or a
  slower gain than planned) or `above`.
- It comes only from a trusted estimate of the phase's own days, with a
  target. Otherwise `reasons` say why not: `no_phase`, `no_target`,
  `no_estimate`, `before_phase`, `not_trusted`, `no_tdee`.
- **Action:** `hold`, `lower_kcal`, `raise_kcal`, `shrink_surplus`,
  `raise_protein_or_kcal` or `maintain`. Nothing applies it by itself
  but the autopilot, within the user's limits.
- **The gap and the step:** `gap_kcal` is the change of the target's
  weekly average that would give the target rate with the measured TDEE
  (more than 0 is to eat more): a coach's to propose, which no step limit
  holds. `step_kcal` is the step toward it the autopilot's limits allow.
- **The intake, not the target:** the verdict is about the target.
  `intake_kcal` is what was eaten, `target_kcal` the target's weekly
  average. A rate that misses while the intake missed the target is the
  intake's: the advice is to eat to the target, not to change it.
- **Guards** turn or hold the action, the first that trips taking it:
  - `underweight`: a loss under way below a BMI of 18.5 proposes
    maintenance, never a deeper cut.
  - `lean_mass`: on a cut or a recomp, a loss of lean mass beyond its
    error at a loss of 0.5 kg a week or more raises protein or kcal.
  - `fat_gain`: on a bulk, body fat rising beyond its error with the
    lean mass rising less than 0.1 kg a week shrinks the surplus.
  - `activity`: the last 7 days' steps 30 % off the weeks before, which
    explains the miss, hold the kcal: the change is the activity's, not
    the metabolism's.
- **Flags** say what to show beside it: `underweight`, `no_height`,
  `maintenance_drift` (a maintenance phase's trend 3 % from where it
  started), `no_steps`, `no_lean_mass`, `no_body_fat` (a guard off for
  want of data).

## The autopilot

- The weekly review, on the user's weekday, takes the verdict's step
  within the user's limits. Its mode (`get_plan`'s `autopilot.mode`):
  `propose`, its steps are proposals the user accepts; `autopilot`, a
  step within the limits applies by itself and is reported.
- A step shifts both types of day by the same kcal, in the carbs.
- It waits: 14 days from a phase's start or a target the user set, 7
  days from its last step, and while a proposal of the user's or a
  coach's waits. It needs a trusted TDEE and the week's logged days and
  weigh-ins, and never leaves a type of day under 130 g of carbs.
- **Drift** (`drift_kcal`, against `drift_limit_kcal`): the net sum of
  its steps since the last target the user agreed to, 15 % of the TDEE
  by default. Past it, the autopilot proposes instead of stepping.
- Its steps and proposals are revisions in `get_plan`, by the client
  `autopilot`, with the review that made them (`check_in`). What it has
  already done is the first thing to read before proposing more.

## Limits

- Three levels: a floor in the code nobody lowers (never 800 kcal a day
  or less, 0.8 g/kg of protein, a loss faster than 1.5 % or a gain
  faster than 1 % a week, no change that starts or deepens a loss below
  a BMI of 18.5); the instance's;
  and the user's, which start at the defaults. `get_plan`'s `limits`
  are those that hold.
- The server checks every change against them. A refusal names each
  violation with its rule, its limit and the value proposed: explain it
  and propose what the limits allow.
- A change of the user's own limits may be part of a proposal, but only
  the app applies it, after the user's yes.
- Protein is in g/kg of a reference mass: the trend weight, but at most
  the weight at a BMI of 30.

## Reading the numbers well

- Weeks, not days: a judgement needs the trend over two weeks or more.
- Adherence first: compare `intake_kcal` with `target_kcal` before
  blaming the TDEE or the target.
- A wide interval is no finding: wait for the data, or say what would
  narrow it (more weigh-ins, more days logged).
- The TDEE falls as the weight falls and with a long deficit; a step of
  the target follows it. A real stall is a verdict of the phase's own
  days, trusted, after the 14 days of water.
- Say the numbers as the tools answer them, with their uncertainty.
