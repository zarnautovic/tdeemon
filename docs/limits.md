# Limits and their defaults

**This is not medical advice.** tdeemon is a tool for adults who plan
their own eating. The limits below keep a plan inside what the
literature and common practice treat as safe for a healthy adult; they
do not make a plan right for anyone, and they are no substitute for a
clinician. Someone with a condition listed under
[Cases for a clinician](#cases-for-a-clinician) should plan with one.

Limits apply to plans only: phases, targets and the autopilot's steps
([DESIGN.md](DESIGN.md) §8). Logging is never limited or judged; a
4000 kcal day is simply logged.

## Three levels

1. **A floor in the code** that nobody can lower, the admin included:
   never 800 kcal a day or less, and never less than 0.8 g of protein
   per kg of the reference mass.
2. **The instance's limits**, set by the admin in the configuration
   within that floor: the outer bound for everyone on the instance. The
   column "Hard limit" below holds their defaults.
3. **The user's limits**, within the instance's. The column "Default"
   holds a user's starting values. The user may tighten them freely;
   going past a default needs an explicit confirmation, in a web
   session, and nothing goes past the instance's limit.

One function, `Validate`, checks every change of a plan against all
three, and says which rule a change breaks, with the limit and the
value proposed. An AI client can propose a change of the user's limits,
but only the user applies it, from a web session (DESIGN.md §8).

## A plan needs the profile

Several limits depend on the person: the kcal floor on sex, the BMI on
height, the estimated BMR on sex, age, height and weight, and the cases
for a clinician on age. Without sex (for the energy equations), birth
date and height, `Validate` refuses a phase or a target
(`profile_missing`), so the coach knows what to ask. Logging works
without them.

## The defaults

Rates are in % of body weight per week, by the trend weight; a minus is
a loss. The column "Basis" says whether the evidence supports a value
("evidence") or it is a reasonable choice that no study settles
("convention"); the notes after the table say which part is which.

| Item | Default | Hard limit | Basis | Sources |
| --- | --- | --- | --- | --- |
| Cut, BMI below 30 | −0.5 (−0.25 to −0.7) | −1.0 | evidence | Helms, Aragon and Fitschen 2014; Garthe et al. 2011; Roberts et al. 2020 |
| Cut, BMI 30 or more | −0.75 (−0.5 to −1.0) | −1.25, and at most 1.5 kg a week | convention; the 1.5 kg evidence | NHLBI 1998; Jensen et al. 2014; Weinsier et al. 1995 |
| Bulk | +0.25 | +0.5 | evidence | Iraki et al. 2019; Helms et al. 2023 |
| Recomp | −0.1 | −0.5 to +0.25 | convention | Murphy and Koehler 2022; Barakat et al. 2020 |
| Maintenance | 0 ± 0.1 | a change of more than 3 % leaves the phase | convention; the 3 % evidence | Stevens et al. 2006; Turicchi et al. 2020 |
| Protein, floor | 1.6 g/kg | 0.8 g/kg (the RDA; the code's floor) | evidence | Morton et al. 2018; IOM 2005 |
| Protein, cut | 2.2 g/kg (1.8 to 2.7) | at most 3.0 g/kg | evidence; the 3.0 convention | Roberts et al. 2020; Helms, Zinn et al. 2014; Jäger et al. 2017; Antonio et al. 2016 |
| Kcal, floor | max(1500 for men, 1200 for women; the estimated BMR) | 1200; never 800 or less (the code's floor) | convention; the 800 and 1200 evidence | Jensen et al. 2014; NICE 2025; NHLBI 1998 |
| Fat, floor | 20 % of energy | max(15 %, 0.5 g/kg) | evidence; the hard limit convention | IOM 2005; Helms, Aragon and Fitschen 2014; Iraki et al. 2019 |
| Autopilot, step | min(100 kcal, 5 %) of the weekly average | min(200 kcal, 10 %) | convention | MacroFactor |
| Autopilot, interval | 7 days; the first change at least 14 days after a phase starts or changes | | convention | MacroFactor; Nuckols 2024 |
| Autopilot, data | at least 4 logged days and 3 weigh-ins in the 7 days | | convention | MacroFactor |
| Autopilot, drift | ±15 % of the TDEE the phase started with, then the user confirms | ±25 % | convention | Frankenfield et al. 2005; Müller et al. 2016 |

Notes on the rows:

- **Cut, BMI below 30:** Helms, Aragon and Fitschen recommend 0.5 to
  1 % a week, Roberts et al. prefer 0.5 % or less, and Garthe et al.
  found 0.7 % kept more lean mass than 1.4 %. The lower end of −0.25 is
  a convention.
- **Cut, BMI 30 or more:** NHLBI recommends 0.5 to 1 kg a week and a
  loss of 10 % in 6 months, and Jensen et al. 5 to 10 % in 6 months; the
  rates in % are conversions of these. The 1.5 kg a week comes from
  Weinsier et al.: above it, the risk of gallstones rises.
- **Bulk:** both sources recommend 0.25 to 0.5 % a week, the less the
  more experienced the lifter; tdeemon does not ask for experience, so
  an experienced lifter lowers the default.
- **Recomp:** no study gives a rate. Murphy and Koehler found that a
  deficit of about 500 kcal a day stops gains of lean mass, so the
  default is close to maintenance.
- **Maintenance:** Stevens et al. define maintenance as a change of less
  than 3 % of body weight; the band of ±0.1 % a week is a convention,
  wider than the trend's noise but well under the weekly swings
  Turicchi et al. measured in the raw weight.
- **Protein, floor:** Morton et al. found gains of muscle level off at
  about 1.6 g/kg (and suggest up to 2.2 to be sure); 0.8 g/kg is the
  RDA, for people who do not train.
- **Protein, cut:** the range is Roberts et al.'s. Helms, Zinn et al.
  give 2.3 to 3.1 g per kg of fat-free mass, about the same. No source
  sets an upper limit: Jäger et al. see possible benefits above 3.0, and
  Antonio et al. found no harm at about 3.3 g/kg over a year, in 14 men.
  The 3.0 is a convention.
- **Protein by phase:** maintenance 1.6 g/kg (1.4 to 2.0) after Jäger et
  al.; bulk 1.8 (1.6 to 2.2) after Iraki et al. and Morton et al.;
  recomp 2.0, a convention between the two.
- **The reference mass for protein** is the fat-free mass when it is
  known, else the trend weight, but at most the weight at a BMI of 30:
  above it, the actual weight overstates the need (Weijs 2025; Dekker et
  al. 2022 prefer the fat-free mass). The target is stored in grams and
  the limit checked in g/kg; a floor in grams the user sets only
  narrows it.
- **Kcal, floor:** Jensen et al. prescribe 1200 to 1500 kcal for women
  and 1500 to 1800 for men as one way to diet; their lower ends are the
  defaults. Below 1200 kcal NICE speaks of a low-energy diet, and below
  800 of a very-low-energy diet, both only with specialist support
  (NICE 2025, 1.16.8 to 1.16.11; Jensen et al. and NHLBI likewise for
  800). The estimated BMR as a floor is a convention. The floor holds
  for each day type, not only for the weekly average.
- **Fat, floor:** 20 % is the lower end of the IOM's range (20 to
  35 %). The hard limit follows the lowest values sports nutrition
  recommends: 15 % (Helms, Aragon and Fitschen) and 0.5 g/kg (Iraki et
  al.).
- **The autopilot's limits are all conventions.** MacroFactor publishes
  no caps; it smooths a step over weeks, and needs at least 4 logged
  days and 1 weigh-in in 7 (3 weigh-ins being its ideal, which tdeemon
  requires). Its estimate of the TDEE settles after about 14 to 30 days
  (Nuckols 2024). The drift bounds start from the error of the
  equations that estimate the TDEE: within 10 % for most people
  (Frankenfield et al.), and the TDEE adapts to a diet (Müller et al.).
- **The autopilot steps the weekly average**, and the difference
  between training and rest days stays as it is. The TDEE is expected to
  fall by about 22 kcal a day for each kg lost, once the weight has
  settled (Hall et al. 2011); that is modelled apart from the drift.

## Warnings, not limits

These show in a proposal's preview without refusing it:

- **Low energy availability:** below 30 kcal per kg of fat-free mass a
  day, intake less the energy of exercise, when the fat-free mass is
  known. The threshold was found in women (Loucks and Thuma 2003), and
  the IOC no longer holds to a fixed one, lower for men (Mountjoy et al.
  2023), so it is a warning only; the active energy a device reports
  stands in for the energy of exercise, which is more and so warns
  sooner. A convention.
- **Low carbohydrate:** below 130 g a day, the RDA, set by the brain's
  use of glucose (IOM 2005). A low-carbohydrate diet is a choice, so
  there is no floor.

## Cases for a clinician

tdeemon does not plan for these; in P0 they get the hard limits and this
page, and questions at sign-up come later:

- a very-low-calorie diet, 800 kcal a day or less (NICE 2025; Jensen et
  al. 2014);
- an eating disorder, or a BMI below 18.5 (no cut phase);
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
- Institute of Medicine (2005). Dietary Reference Intakes for Energy,
  Carbohydrate, Fiber, Fat, Fatty Acids, Cholesterol, Protein, and Amino
  Acids. National Academies Press. <https://doi.org/10.17226/10490>
- Institute of Medicine and National Research Council (2009). Weight
  Gain During Pregnancy: Reexamining the Guidelines. National Academies
  Press. <https://doi.org/10.17226/12584>
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
