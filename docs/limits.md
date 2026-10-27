# Limits and their defaults

**This is not medical advice.** tdeemon is a tool for adults who plan
their own eating. The limits below keep a plan inside what the
literature and common practice treat as safe for a healthy adult; they
do not make a plan right for anyone, and they are no substitute for a
clinician. Someone with a condition listed under
[Cases for a clinician](#cases-for-a-clinician) should plan with one.

Limits apply to plans only: phases, targets and the autopilot's steps
([DESIGN.md](DESIGN.md) §8). Logging is never limited or judged; a
4000 kcal day is simply logged. When a limit in the code changes, this
page changes with it.

## Three levels

1. **A floor in the code** that nobody can lower, the admin included:
   - never 800 kcal a day or less;
   - never less than 0.8 g of protein per kg of the reference mass;
   - never a rate faster than a loss of 1.5 % or a gain of 1 % of the
     trend weight a week: a phase's, or the one a kcal target implies (a
     convention, so no configuration allows a plan past the literature's
     ranges by far);
   - no plan for someone under 18;
   - no change that starts or deepens a loss of weight below a BMI of
     18.5, by the trend weight now (a cut, a recomp below 0, a kcal
     target that implies a loss, or one that lowers the weekly average
     before calibration trusts the TDEE; a first target is not judged,
     see "Reading the table"), and no goal or milestone weight below
     it. A loss under way when the trend crosses 18.5 is not changed
     without the user: the autopilot applies no more steps and proposes
     maintenance, and the summary flags it.
2. **The instance's limits**, set by the admin in the configuration
   within that floor: the outer bound for everyone on the instance. The
   column "Hard limit" below holds their defaults.
3. **The user's limits**, within the instance's, which start at the
   column "Default". The user may tighten them freely; loosening one, up
   to the instance's limit and no further, takes an explicit
   confirmation in a web session.

One function, `Validate`, checks every change of a plan against all
three, and says which rule a change breaks, with the limit and the
value proposed. An AI client can propose a change of the user's limits,
but only the user applies it, from a web session, with the confirmation
a loosening takes; the rest of the proposal is held to the limits as it
leaves them, so a faster cut comes with the limit that allows it
(DESIGN.md §8). The profile only the user sets.

## A plan needs the profile

Several limits depend on the person: the kcal floor on sex, the BMI on
height and weight, the estimated BMR on sex, age, height and weight,
and the floor in the code on age. So `Validate` refuses a phase or a
target without sex (for the energy equations), birth date and height
(`profile_missing`), or without a trend weight (`weight_missing`), and
the coach knows what to ask. Logging works without them.

Since the profile moves the limits, it changes only from a web session,
as they do, and only the user changes it: a coach proposes no change of
it. The weight is a measurement,
which any client may log; it moves the BMI and the reference mass too,
a gap the limits do not close. The lean mass, also a measurement, moves
only the warning on energy availability.

## Reading the table

- **Rates** are in % of the trend weight per week; a minus is a loss. A
  phase's target rate lies between 0 and its limit (a cut's is a loss,
  a bulk's a gain); a recomp has its own range, and maintenance is 0.
- **The rate a kcal target implies** is the weekly average less the
  measured TDEE, times 7, over 7700 kcal per kg, as a share of the trend
  weight. It counts once calibration trusts the measured TDEE (DESIGN.md
  §7), one from before the current phase included; until then a
  target's kcal meet the kcal
  floor, and below a BMI of 18.5 a target may not lower the weekly
  average, which the floor in the code takes for a deeper loss. A first
  target, with none before it, cannot be judged so: no rate can be told
  without a TDEE, and only the kcal floor holds for it.
  Whatever the phase, it is held to the limits of loss and gain: no
  faster loss than a cut's limit for the BMI, no faster gain than a
  bulk's.
- **"Default"** is the user's limit at the start, and the value a phase
  starts with. For a rate, the range in brackets is the user's limit: a
  faster loss or gain needs the user to loosen it first, and a slower
  rate never does. Where a default is a floor or a ceiling, it is the
  user's floor or ceiling.
- **"Hard limit"** is the instance's default limit. An empty cell means
  the default is the instance's limit too, which the admin may tighten
  and nobody loosens.
- **"Basis"** says "evidence" where guidelines or the studies they rest
  on set the value, and "convention" where it is a choice, or one
  review's recommendation among others.
- **Limits are inclusive** ("at most", "at least"), but for the code's
  floor of 800 kcal, which 800 itself breaks.
- **The BMI** that picks a cut's row, also for the loss a kcal target
  implies, is the trend weight's when the current phase started,
  whatever its type, and does not change during the phase. The code's
  floor of 18.5 is the trend weight's now, at every check, the
  autopilot's steps included.
- **The trend weight** is the one now, for the reference mass and to
  turn a rate in % into kg.
- **The one-off import of the predecessor's plan** (`import-history
  --plan`, DESIGN.md §10) holds every phase and target from the current
  ones on to these limits, the future ones too: the current target as a
  new one, as the history before it is the file's and vouches for
  nothing, and each target after it against the one before it, by the
  user's training weekdays if they have them, else the file's. The ones
  before are history, written as they were. A run again that writes
  nothing of the plan holds nothing; a weigh-in the days left out holds
  nothing either.
  Until calibration has a trend weight of its own, the import takes the
  mean of the weigh-ins of the last week up to today that has any,
  within 30 days. Its current phase is held as a new one: its cut's row
  is picked by the trend weight now, not by the one when it started.
- **A default never passes a limit:** it is cut at the instance's or
  the user's, where that is stricter (above about 150 kg, −1.0 % a week
  is more than 1.5 kg; a user's protein ceiling of 2.0 g/kg cuts a
  cut's 2.2).
- **The floors of kcal and fat** hold for each day type, on every
  level, not only for the weekly average; fat is a share of that day's
  kcal.
- **`Validate` refuses a change for the values it sets:** those it
  changes, and those derived from them (a kcal step sets the fat share
  and the implied rate); a value carried over unchanged is not one it
  sets. A value that new data put outside a limit (the measured TDEE
  moved, or the weight rose so that the protein in grams falls short of
  the floor) does not stop other changes, and a change that brings it
  toward the limit passes; the summary flags it.

## The defaults

| Item | Default | Hard limit | Basis | Sources |
| --- | --- | --- | --- | --- |
| Cut, BMI below 30 | −0.5 (0 to −0.7) | −1.0 | evidence | Helms, Aragon and Fitschen 2014; Garthe et al. 2011; Roberts et al. 2020 |
| Cut, BMI 30 or more | −0.75 (0 to −1.0) | −1.25, and at most 1.5 kg a week | convention; the 1.5 kg evidence | NHLBI 1998; Jensen et al. 2014; Weinsier et al. 1995 |
| Bulk | +0.25 (0 to +0.25) | +0.5 | evidence | Iraki et al. 2019; Helms et al. 2023 |
| Recomp | −0.1 (−0.5 to +0.25) | −0.5 to +0.25 | convention | Murphy and Koehler 2022; Barakat et al. 2020 |
| Maintenance | 0 | | convention | Stevens et al. 2006; Turicchi et al. 2020 |
| Protein, floor | 1.6 g/kg | 0.8 g/kg (the RDA; the code's floor) | evidence | Morton et al. 2018; IOM 2005 |
| Protein, ceiling | 3.0 g/kg | | convention | Jäger et al. 2017; Antonio et al. 2016; Roberts et al. 2020 |
| Kcal, floor | 1500 for men, 1200 for women | 1200; never 800 or less (the code's floor) | convention; the 800 and 1200 evidence | Jensen et al. 2014; NICE 2025; NHLBI 1998 |
| Fat, floor | 20 % of the day's kcal | 15 % | evidence; the 15 % convention | IOM 2005; Helms, Aragon and Fitschen 2014; Roberts et al. 2020 |
| Autopilot, step | min(100 kcal, 5 %) of the weekly average | min(200 kcal, 10 %) | convention | MacroFactor |
| Autopilot, interval | 7 days; the first change 14 days or more after a phase starts or changes | | convention | MacroFactor; Nuckols 2024 |
| Autopilot, data | at least 4 logged days and at least 3 weigh-ins in the last 7 days | | convention | MacroFactor |
| Autopilot, drift | 15 % of the TDEE | 25 % | convention | Frankenfield et al. 2005; Müller et al. 2016 |

The protein a phase starts with, between the floor and the ceiling:

| Phase | Default target | Sources |
| --- | --- | --- |
| Cut | 2.2 g/kg | Roberts et al. 2020; Helms, Zinn et al. 2014 |
| Maintenance | 1.6 g/kg | Jäger et al. 2017 |
| Bulk | 1.8 g/kg | Iraki et al. 2019; Morton et al. 2018 |
| Recomp | 2.0 g/kg | convention |

### Notes on the rows

- **Cut, BMI below 30:** Helms, Aragon and Fitschen recommend 0.5 to
  1 % a week, and Roberts et al. 0.5 % or less. Garthe et al. planned
  0.7 % and 1.4 % a week; the slower group, at 0.7 %, gained lean mass,
  and the faster one, at the 1.0 % it reached, did not.
- **Cut, BMI 30 or more:** NHLBI recommends 0.5 to 1 kg a week and a
  loss of 10 % in 6 months, and Jensen et al. 5 to 10 % in 6 months; the
  rates in % are conversions of these. The 1.5 kg a week comes from
  Weinsier et al.: above it, the risk of gallstones rises.
- **Bulk:** both sources recommend 0.25 to 0.5 % a week, and less the
  longer someone has trained (Iraki et al.: advanced lifters more
  conservatively). tdeemon does not ask for experience, so a lifter
  with years of training may set the rate lower.
- **Recomp:** no study gives a rate. Murphy and Koehler found that a
  deficit of about 500 kcal a day stops gains of lean mass, so the
  default is close to maintenance.
- **Maintenance:** the target rate is 0; a phase with another rate is a
  cut, a bulk or a recomp. How far the trend may stray is calibration's
  (DESIGN.md §7): its verdict tolerates ±0.1 % a week around the target
  (a convention), as it does for every phase, and a trend more than 3 %
  from the one the phase started with shows in the summary as the phase
  drifting (Stevens et al. define maintenance as a change of less than
  3 %). The raw weight
  swings by about 0.35 % within a week (Turicchi et al.); the trend
  smooths that out.
- **Protein:** every g/kg value here is per kg of body weight, as in its
  sources. The reference mass is the trend weight, at most the weight at
  a BMI of 30: above it, the actual weight overstates the need (Weijs
  2025: at least 1.2 g/kg "with a maximum weight of BMI 30"). The
  fat-free mass is not used for protein, since its values per kg are
  others (2.3 to 3.1 g/kg of fat-free mass on a cut, Helms, Zinn et al.;
  other factors in Dekker et al. 2022). The target is stored in grams
  and the limit checked in g/kg; a floor in grams the user sets only
  narrows it.
- **Protein, floor:** Morton et al. found gains of muscle level off at
  about 1.6 g/kg, and suggest up to 2.2 to be sure. 0.8 g/kg is the RDA
  for nearly all healthy adults (IOM 2005).
- **Protein, ceiling:** no source sets an upper limit. Jäger et al. see
  possible benefits above 3.0 g/kg; Antonio et al. found no harm at
  about 3.3 g/kg in a one-year crossover with about six months on it,
  in 14 men; Roberts et al. allow up to 3.5 g/kg on a cut, against
  hunger. The 3.0 is a convention.
- **Protein by phase:** the cut's 2.2 g/kg is in Roberts et al.'s range
  of 1.8 to 2.7, and Helms, Zinn et al.'s per kg of fat-free mass comes
  to about the same; maintenance follows Jäger et al. (1.4 to 2.0), the
  bulk Iraki et al. and Morton et al. (1.6 to 2.2); the recomp's 2.0 is
  a convention between the two.
- **Kcal, floor:** Jensen et al. prescribe 1200 to 1500 kcal for women
  and 1500 to 1800 for men as one way to diet; their lower ends are the
  defaults. NICE calls a diet of 800 to 1200 kcal low-energy and one
  under 800 very-low-energy, both only with specialist services (NICE
  2025, 1.16.8 to 1.16.11; Jensen et al. and NHLBI likewise for 800).
  The instance's 1200, the top of NICE's low-energy range and the bottom
  of Jensen et al.'s for women, allows 1200 itself; a diet under it
  needs an admin who lowers the instance's floor, for a clinician's
  patient. The code refuses 800 or less, one kcal stricter than "under
  800".
- **Fat, floor:** 20 % is the lower end of the IOM's range (20 to 35 %),
  a reference intake. The hard limit is the lower end of one review's
  range, Helms, Aragon and Fitschen's (15 to 30 %), so a convention;
  Roberts et al. go down to 10 % in contest preparation, which tdeemon
  does not allow.
- **Autopilot, step:** only the autopilot's steps are limited; a
  coach's proposal meets every limit but the autopilot's step,
  interval, data and drift. A step moves the weekly average, and the
  difference between training and rest days stays as it is (the coach
  can change that split, DESIGN.md §8). MacroFactor publishes no caps;
  it smooths a step over weeks.
- **Autopilot, interval and data:** MacroFactor needs at least 4 logged
  days and 1 weigh-in in 7, 3 weigh-ins being its ideal, which tdeemon
  requires. Its estimate of the TDEE settles after about 14 to 30 days
  (Nuckols 2024).
- **Autopilot, drift:** the autopilot's net change of the weekly
  average (a step up and a step down cancel), from the last kcal target
  the user agreed to (the phase's start, or an applied proposal that
  set the kcal), as a share of the TDEE then: the measured one, else the
  equation's estimate. Past the user's limit the autopilot applies
  nothing more and proposes instead; the user may raise the limit, in a
  web session, up to 25 %. The autopilot's own proposals meet its step,
  interval and data, not its drift, and applying one sets a new base.
  The bounds start from the error of the estimate: the best equation
  for the resting rate is within 10 % of the measured rate for most
  people (Frankenfield et al.), and the TDEE adapts to a diet (Müller et
  al.). The TDEE's expected fall, about 22 to 24 kcal a day for each kg
  lost once the weight has settled (Hall et al. 2011), is modelled
  apart and is not drift.

## The names of the limits

The code (`internal/domain/limits`), the API and the configuration call
the limits by these names; the instance's are set with
`TDEEMON_LIMIT_<NAME>`, such as `TDEEMON_LIMIT_KCAL_FLOOR_FEMALE=1000`.
Rates are in % a week, shares in %.

| Name | Row | Looser when |
| --- | --- | --- |
| `cut_rate` | Cut, BMI below 30 | lower |
| `cut_rate_obese` | Cut, BMI 30 or more | lower |
| `loss_kg_week` | the 1.5 kg a week, which holds for every loss: a cut, a recomp below 0 and the loss a kcal target implies; a user's starts at 1.5 too | higher |
| `bulk_rate` | Bulk | higher |
| `recomp_min`, `recomp_max` | Recomp | lower, higher |
| `protein_floor`, `protein_ceiling` | Protein, floor and ceiling, g/kg | lower, higher |
| `protein_floor_grams` | a floor in grams, the user's only; 0 for none | lower |
| `kcal_floor_male`, `kcal_floor_female` | Kcal, floor | lower |
| `fat_floor_pct` | Fat, floor | lower |
| `step_kcal`, `step_pct` | Autopilot, step | higher |
| `interval_days`, `first_step_days` | Autopilot, interval | lower |
| `logged_days`, `weigh_ins` | Autopilot, data | lower |
| `drift_pct` | Autopilot, drift | higher |

The instance may only tighten the limits with an empty cell under
"Hard limit" (`protein_ceiling` and the autopilot's interval and data),
and no level passes the floor in the code. A user's limit looser than
the instance's holds only as far as the instance's.

`Validate` names the rule a change breaks, with the limit that holds
for the user (the strictest of the levels), the value proposed, the
type of day for the limits of each, and the outermost level the value
breaks: `code` (nothing allows it), `instance` (only the admin could)
or `user` (the user may loosen it, up to the instance's). Values on a
limit pass, within the error of float arithmetic.
The rules: `profile_missing`, `weight_missing`, `min_age`,
`underweight_loss`, `underweight_goal`, `underweight_milestone`,
`cut_rate`, `bulk_rate`, `recomp_rate`, `loss_kg_week`,
`implied_loss_rate`, `implied_gain_rate`, `implied_loss_kg_week`,
`kcal_floor`, `fat_floor`, `protein_floor`, `protein_ceiling`,
`protein_floor_grams`, and for the autopilot's steps `autopilot_step`,
`autopilot_interval`, `autopilot_first_step`, `autopilot_logged_days`,
`autopilot_weigh_ins`, `autopilot_drift`, `tdee_missing` (a step with no
TDEE to measure its drift against) and `autopilot_shape` (a step is a
shift of the weekly average of the target before, and nothing else). A
change that breaks a rule of its shape, or a state or limits that break
theirs (a stored value written past its rules, a number that is no
number), is `invalid_change`. A proposal's change of a user's limit
past the instance's is named by the limit (`cut_rate`,
`kcal_floor_male`, …), of the level `instance`, and so is a minimum it
sets past the instance's maximum, or a maximum under its minimum
(`recomp_min` and `recomp_max`, `protein_floor` and `protein_ceiling`),
which would leave the limits that hold allowing no plan. What starts
the autopilot's waits again, and what counts as a logged day, are the
autopilot's to decide; `Validate` holds the numbers it is given to the
limits.

## Warnings, not limits

These show in a proposal's preview without refusing it:

- **Below the estimated BMR** (`below_bmr`): a kcal target under the resting
  rate by the Mifflin–St Jeor equation (Frankenfield et al.), with the trend
  weight. It is no floor: the BMR grows with the whole weight, so at a BMI of 30
  or more it often lies above the guidelines' own 1200 to 1500 kcal for women
  and 1500 to 1800 for men, which a floor would refuse.
- **Low energy availability** (`low_energy_availability`): below 30 kcal per kg
  of fat-free mass a day, intake less the energy of exercise, when the fat-free
  mass is known. The threshold was found in women (Loucks and Thuma 2003), and
  the IOC no longer holds to a fixed one, lower for men (Mountjoy et al. 2023),
  so it is a warning only, and a convention. The active energy a device reports
  counts all movement, not only exercise, so it stands in for the energy of
  exercise by overstating it: the warning comes sooner, not later.
- **Low carbohydrate** (`low_carbs`): below 130 g a day, the RDA, set by the
  brain's use of glucose (IOM 2005). A low-carbohydrate diet is a choice, so
  there is no floor.

## Cases for a clinician

tdeemon does not plan for these. The code refuses what it can tell from
the profile, the plan and the trend weight (a plan for someone under
18; a change that starts or deepens a loss under a BMI of 18.5, or a
goal or milestone weight under it; 800 kcal a day or less), and a loss
under way that crosses 18.5 makes the autopilot propose maintenance.
For the rest, P0 has this page, and questions at sign-up come later:

- a very-low-calorie diet, under 800 kcal a day (NICE 2025; Jensen et
  al. 2014);
- an eating disorder, or a BMI below 18.5;
- pregnancy (IOM 2009) and breastfeeding;
- people under 18;
- kidney disease, where protein is limited rather than raised (KDIGO
  2024; Ikizler et al. 2020; Ko et al. 2020);
- diabetes treated with insulin or a sulfonylurea, where a deficit can
  bring on hypoglycaemia.

## Sources

- Antonio J, Ellerbroek A, Silver T, et al. (2016). A high protein diet
  has no harmful effects: a one-year crossover study in
  resistance-trained males. J Nutr Metab 2016:9104792.
  <https://doi.org/10.1155/2016/9104792>
- Barakat C, Pearson J, Escalante G, Campbell B, De Souza EO (2020).
  Body recomposition: can trained individuals build muscle and lose fat
  at the same time? Strength Cond J 42(5):7–21.
  <https://doi.org/10.1519/SSC.0000000000000584>
- Dekker IM, van Rijssen NM, Verreijen A, et al. (2022). Calculation of
  protein requirements; a comparison of calculations based on bodyweight
  and fat free mass. Clin Nutr ESPEN 48:378–385.
  <https://doi.org/10.1016/j.clnesp.2022.01.014>
- Frankenfield D, Roth-Yousey L, Compher C (2005). Comparison of
  predictive equations for resting metabolic rate in healthy nonobese
  and obese adults: a systematic review. J Am Diet Assoc
  105(5):775–789. <https://doi.org/10.1016/j.jada.2005.02.005>
- Garthe I, Raastad T, Refsnes PE, Koivisto A, Sundgot-Borgen J (2011).
  Effect of two different weight-loss rates on body composition and
  strength and power-related performance in elite athletes. Int J Sport
  Nutr Exerc Metab 21(2):97–104. <https://doi.org/10.1123/ijsnem.21.2.97>
- Hall KD, Sacks G, Chandramohan D, et al. (2011). Quantification of the
  effect of energy imbalance on bodyweight. Lancet 378(9793):826–837.
  <https://doi.org/10.1016/S0140-6736(11)60812-X>
- Helms ER, Aragon AA, Fitschen PJ (2014). Evidence-based
  recommendations for natural bodybuilding contest preparation:
  nutrition and supplementation. J Int Soc Sports Nutr 11:20.
  <https://doi.org/10.1186/1550-2783-11-20>
- Helms ER, Zinn C, Rowlands DS, Brown SR (2014). A systematic review of
  dietary protein during caloric restriction in resistance trained lean
  athletes: a case for higher intakes. Int J Sport Nutr Exerc Metab
  24(2):127–138. <https://doi.org/10.1123/ijsnem.2013-0054>
- Helms ER, Spence AJ, Sousa C, et al. (2023). Effect of small and large
  energy surpluses on strength, muscle, and skinfold thickness in
  resistance-trained individuals: a parallel groups design. Sports Med
  Open 9(1):102. <https://doi.org/10.1186/s40798-023-00651-y>
- Ikizler TA, Burrowes JD, Byham-Gray LD, et al. (2020). KDOQI clinical
  practice guideline for nutrition in CKD: 2020 update. Am J Kidney Dis
  76(3 Suppl 1):S1–S107. <https://doi.org/10.1053/j.ajkd.2020.05.006>
- Institute of Medicine (IOM) (2005). Dietary Reference Intakes for
  Energy, Carbohydrate, Fiber, Fat, Fatty Acids, Cholesterol, Protein,
  and Amino Acids. National Academies Press.
  <https://doi.org/10.17226/10490>
- Institute of Medicine (IOM) and National Research Council (2009).
  Weight Gain During Pregnancy: Reexamining the Guidelines. National
  Academies Press. <https://doi.org/10.17226/12584>
- Iraki J, Fitschen P, Espinar S, Helms E (2019). Nutrition
  recommendations for bodybuilders in the off-season: a narrative
  review. Sports 7(7):154. <https://doi.org/10.3390/sports7070154>
- Jäger R, Kerksick CM, Campbell BI, et al. (2017). International
  Society of Sports Nutrition position stand: protein and exercise. J
  Int Soc Sports Nutr 14:20. <https://doi.org/10.1186/s12970-017-0177-8>
- Jensen MD, Ryan DH, Apovian CM, et al. (2014). 2013 AHA/ACC/TOS
  guideline for the management of overweight and obesity in adults.
  Circulation 129(25 Suppl 2):S102–S138.
  <https://doi.org/10.1161/01.cir.0000437739.71477.ee>
- KDIGO CKD Work Group (2024). KDIGO 2024 clinical practice guideline
  for the evaluation and management of chronic kidney disease. Kidney
  Int 105(4S):S117–S314. <https://doi.org/10.1016/j.kint.2023.10.018>
- Ko GJ, Rhee CM, Kalantar-Zadeh K, Joshi S (2020). The effects of
  high-protein diets on kidney health and longevity. J Am Soc Nephrol
  31(8):1667–1679. <https://doi.org/10.1681/ASN.2020010028>
- Loucks AB, Thuma JR (2003). Luteinizing hormone pulsatility is
  disrupted at a threshold of energy availability in regularly
  menstruating women. J Clin Endocrinol Metab 88(1):297–311.
  <https://doi.org/10.1210/jc.2002-020369>
- MacroFactor. How does MacroFactor make adjustments for a weight gain
  or weight loss goal? MacroFactor Help Center, read 2026-10-21.
  <https://help.macrofactorapp.com/en/articles/222-how-does-macrofactor-make-adjustments-for-a-weight-gain-or-weight-loss-goal>
- Morton RW, Murphy KT, McKellar SR, et al. (2018). A systematic review,
  meta-analysis and meta-regression of the effect of protein
  supplementation on resistance training-induced gains in muscle mass
  and strength in healthy adults. Br J Sports Med 52(6):376–384.
  <https://doi.org/10.1136/bjsports-2017-097608>
- Mountjoy M, Ackerman KE, Bailey DM, et al. (2023). 2023 International
  Olympic Committee's (IOC) consensus statement on Relative Energy
  Deficiency in Sport (REDs). Br J Sports Med 57(17):1073–1097.
  <https://doi.org/10.1136/bjsports-2023-106994>
- Müller MJ, Enderle J, Bosy-Westphal A (2016). Changes in energy
  expenditure with weight gain and weight loss in humans. Curr Obes Rep
  5(4):413–423. <https://doi.org/10.1007/s13679-016-0237-4>
- Murphy C, Koehler K (2022). Energy deficiency impairs resistance
  training gains in lean mass but not strength: a meta-analysis and
  meta-regression. Scand J Med Sci Sports 32(1):125–137.
  <https://doi.org/10.1111/sms.14075>
- NHLBI Obesity Education Initiative Expert Panel (1998). Clinical
  Guidelines on the Identification, Evaluation, and Treatment of
  Overweight and Obesity in Adults: The Evidence Report. NIH Publication
  No. 98-4083. <https://www.ncbi.nlm.nih.gov/books/NBK2003/>
- NICE (2025). Overweight and obesity management (NG246).
  <https://www.nice.org.uk/guidance/ng246>
- Nuckols G (2024). MacroFactor's algorithms and core philosophy.
  MacroFactor, updated 2024-10-10.
  <https://macrofactor.com/macrofactors-algorithms-and-core-philosophy/>
- Roberts BM, Helms ER, Trexler ET, Fitschen PJ (2020). Nutritional
  recommendations for physique athletes. J Hum Kinet 71:79–108.
  <https://doi.org/10.2478/hukin-2019-0096>
- Stevens J, Truesdale KP, McClain JE, Cai J (2006). The definition of
  weight maintenance. Int J Obes 30(3):391–399.
  <https://doi.org/10.1038/sj.ijo.0803175>
- Turicchi J, O'Driscoll R, Horgan G, et al. (2020). Weekly, seasonal
  and holiday body weight fluctuation patterns among individuals engaged
  in a European multi-centre behavioural weight loss maintenance
  intervention. PLoS One 15(4):e0232152.
  <https://doi.org/10.1371/journal.pone.0232152>
- Weijs PJM (2025). Protein requirement in obesity. Curr Opin Clin Nutr
  Metab Care 28(1):27–32. <https://doi.org/10.1097/MCO.0000000000001087>
- Weinsier RL, Wilson LJ, Lee J (1995). Medically safe rate of weight
  loss for the treatment of obesity: a guideline based on risk of
  gallstone formation. Am J Med 98(2):115–117.
  <https://doi.org/10.1016/S0002-9343(99)80394-5>
