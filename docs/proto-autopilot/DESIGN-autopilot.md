# Autopilot weekly review: UX design (prototype)

Status: a prototype for review. It lives on the branch `proto/autopilot`
and is never merged. It covers Vikunja story 212. The rules of the
autopilot are already decided (DESIGN.md §7 and §8, limits.md "Autopilot"
rows). This document designs how they are shown and controlled.

Every number in the screenshots is made up. The prototype fakes the
autopilot's API in the web app (`web/src/proto/`) on top of a real
server, which holds a made-up user.

## How to run it

On the branch `proto/autopilot-design` the prototype's code is not in
the tree but in `docs/proto-autopilot/prototype.patch`, so the branch
keeps the app as on `main`: `git apply docs/proto-autopilot/prototype.patch`
on a scratch branch brings it back (the paths below are then there).

```sh
scripts/proto-autopilot.sh   # builds the server, seeds a made-up user, runs Vite on :5199
# open http://127.0.0.1:5199/plan?ap=auto-step
# sign in as ana@example.com / only-for-the-prototype
```

A panel at the bottom right picks the state (`?ap=<id>`), switches the
account between English and Croatian, and opens **Messages**
(`/proto/messages`), which shows every push, Telegram message and morning
line in both languages. These actions work against the mock:

- the mode switch, with its confirmation;
- the review day;
- Accept and Reject on the autopilot's proposals;
- Take back on its steps.

To retake the screenshots, run `(cd web && node e2e/proto-shots.ts)`
while the script is running.

## 0. Decisions (2026-10-31), binding for the implementation

These settle sections 5 and 6; where they differ from the prototype, they
win.

Rules (section 5):

1. **"Weekly review"** is what the user sees, in both languages ("Tjedni
   pregled"); code, docs and the story may say check-in.
2. **Drift base:** no step comes before calibration trusts the TDEE, so
   the base is the trusted TDEE when the user agreed to the target; if
   it was not trusted then, the TDEE at the autopilot's first step,
   frozen until the next agreed target. Until there is a base the app
   shows the limit as "from the first step", never a moving number.
3. **Repeated reasons:** a review whose outcome and reason are the same
   as the last review's sends no message (push or Telegram); the plan's
   card and the morning summary still say it. A hold (pace on plan) and
   "too little data" are not repeats when their numbers changed.
4. **Too few carbs:** a step down is not taken when it would leave
   either day type's carbs below `limits.CarbsRDA` (130 g, the existing
   warning's threshold); the review says so (reason `few_carbs`, with
   the carbs it would leave) and points to the coach. limits.md gets
   the row.
5. **First review date:** the API gives it (`first_step`: the first
   review weekday on or after the day calibration expects a trusted
   TDEE and the waits allow a step); the app never computes it.
6. **Undo:** the recent-changes sheet lists a step as by Autopilot,
   without Undo; the step's push says "take it back in the plan".
7. **Rejected proposal:** after the user rejects an autopilot proposal,
   later reviews propose again only when the gap the verdict asks for
   grew by at least one step since; otherwise they hold and say the
   target stays as the user chose.
8. **Revert of a step:** a revert is a proposal the user applied that
   sets the kcal, so it is an agreed target: drift resets to 0 and the
   14-day wait starts again, as for any agreed target.

UI (section 6):

1. Settings stay on the plan, under "You and limits" (`/plan/autopilot`).
2. The day's line shows on the review day only; a proposal keeps the
   existing pending-proposal line while it waits.
3. Turning the autopilot off asks nothing; turning it on asks, as in
   the prototype.
4. Pushes of a hold or a no step are sent silent (no sound or
   vibration) under the tag `autopilot`, so each replaces the last;
   steps and proposals notify normally. Telegram gets every review.
5. Drift is shown signed ("−200 of 390 kcal").
6. "100 kcal = 25 g carbs" only under "The numbers" and in Telegram; the
   history's change lines already show the carbs.
7. Croatian weekday chips in lower case ("pon", "uto") are right.

## 1. Research: what changes the design

| Source | Finding | What it means here |
| --- | --- | --- |
| MacroFactor, [program styles](https://help.macrofactorapp.com/en/articles/91-program-styles) | Three named styles: **Coached** (the app adjusts), **Collaborative** (the app adjusts, the user shapes the days), **Manual**. You can switch at any time without losing data. | Name the autonomy as a plain choice and keep switching cheap. The current decision has two modes, which is the right size. |
| MacroFactor, [check-ins](https://help.macrofactorapp.com/en/articles/247-introduction-to-check-ins-and-coaching-modules) | A weekly check-in on a day the user picks. Each coaching module says why it appeared, and a "fast check-in" gives just the numbers. | Lead with the decision in one line and keep the full numbers a tap away. Each review gets a fixed day. |
| MacroFactor, [adjustments](https://help.macrofactorapp.com/en/articles/222-how-does-macrofactor-make-adjustments-for-a-weight-gain-or-weight-loss-goal), [logging needs](https://help.macrofactorapp.com/en/articles/110-how-frequently-do-i-need-to-log-my-nutrition-for-the-expenditure-algorithm-and-weekly-coaching-updates) | Smoothed, conservative changes with no catch-up. With too little data it **pauses** rather than guesses. | "No step" is a real outcome and should be said plainly. Show the step, not the gap the data imply. |
| Carbon, [weekly check-in](https://help.joincarbon.com/en/articles/6004812-weekly-check-in), [why it won't adjust](https://help.joincarbon.com/en/articles/6059935-why-won-t-the-calorie-planner-adjust) | It notifies on the check-in day. When targets do not move it says why: overdue, or at the floor, in which case it suggests a reverse diet. | Every outcome has a reason line, including "too few carbs, ask your coach". |
| RP Diet, [weekly review](https://docs.rpstrength.com/docs/diet-coach-app/1.22-beta/features/weekly-review-and-weekly-planning/), [weight](https://docs.rpstrength.com/docs/diet-coach-app/1.22-beta/concepts/weight/) | Too few weigh-ins means last week's target is repeated. A target the user overrides stops the automatic changes. | This matches "Accepting or setting a target resets drift", which the UI says next to the drift. |
| Microsoft [HAX guidelines](https://www.microsoft.com/en-us/research/uploads/prod/2019/01/Guidelines-for-Human-AI-Interaction-camera-ready.pdf): G1, G2, G9, G11, G17, G18 | Say what the system can do and how well. Make correction cheap. Explain why. Offer global controls. Notify about changes. | The settings page states what it may change, what it never changes and when it waits. Every step can be taken back. Messages are sent even when nothing changes. |
| Google [PAIR, Explainability + Trust](https://pair.withgoogle.com/chapter/explainability-trust/) | Say what it can't do early. Never show "what" without "why" when the stakes are high. Don't look more certain than you are. Increase automation under the user's guidance. | Show the TDEE as "± …". The confirmation when turning it on states the bounds. Propose mode is the default. |
| Betterment, [auto-adjust notices](https://betterment.com/help/auto-adjust-notifications) | Every automatic change is logged in the activity feed and announced. | An autopilot step is a revision in the plan's history, by "Autopilot", with **Take back**. |

I did not verify the following: whether MacroFactor has a dedicated
"why didn't my target change" screen, and whether RP has an explicit
approve step. No source covered Nest.

## 2. Design decisions

1. **Name the event "Weekly review" ("Tjedni pregled"), not "check-in".**
   The app already uses "Check-in" for the daily "How was today?" card
   (`t.day.checkIn`). Two different check-ins would be confused. "Weekly
   review" says what happens.
2. **Name the autopilot "Autopilot" and give it the `Gauge` icon, not
   Sparkles or Bot.** Sparkles marks the AI coach. The autopilot is a
   deterministic rule on the server, and the icon should not suggest a
   model. The limits group is already called "Autopilot".
3. **Put the settings on the plan, as a row in "You and limits"**, linking to
   `/plan/autopilot`, not in Settings. Autonomy is about the plan. It sits
   next to the limits that bound it (its step and drift are limits), and like
   the limits it is a web-session-only setting. Settings holds device and
   account matters.
4. **The mode is a two-way switch: "Ask me first" and "Autopilot". "Ask me
   first" is the default.** Turning on the autopilot asks first, with the
   bounds in numbers: "by at most 100 kcal a day a week and 390 kcal in
   all, and tell you each time". Turning it off happens at once. This
   follows the app's own pattern, where only loosening a limit asks first,
   and HAX G1 and PAIR (say the bounds up front).
5. **The review day is a row of seven weekday chips** (the existing
   `ChipGroup`), with "In the morning, before your morning summary. Next
   review: Sat, Nov 7." One tap, and the next date is visible.
6. **The settings page always shows three lists: "What it may change",
   "What it never changes" and "When it waits", each in numbers from the
   user's limits.** This is the explainer the research asks for, and it
   stays on the page rather than in a one-time tour. One line notes that
   ⌘Z does not take a step back, and says where to take it back instead.
7. **Show the drift as "−200 of 390 kcal" with a bar, in autopilot mode
   only.** The hint names the base ("since you agreed to 2,279 kcal a day
   on Sep 26") and the reset rule. Once the next step would pass the
   limit, the bar turns to the warning colour and says "Limit reached: the
   next step is proposed". Drift means nothing in propose mode, so it is
   hidden there.
8. **The plan gets an Autopilot card between the phase or pending
   proposal and the targets.** It shows the mode badge, the last review
   (one headline, one pace line, and "The numbers" behind a disclosure),
   the next review, the drift, and "Settings ›". It sits above the targets
   because its decision is why the targets changed.
9. **An applied step is an ordinary revision** in Plan history, "Oct 31,
   by Autopilot". It shows the headline, the pace, the exact change (the
   existing `ProposalChanges`: kcal and carbs on each type of day) and
   **Take back**. The rationale is the code `autopilot`, rendered from the
   review's numbers in the user's language. In history the rationale is
   brief, because the changes beneath it carry the rest.
10. **The autopilot's proposal reuses the existing proposal card**:
    "Proposed by Autopilot", the expiry, the headline and the full
    numbers. The card's "What the data say" box is hidden for the
    autopilot's own proposals, because the review *is* the data. When it
    proposes because of drift, a warning box says: "On autopilot this step
    would apply by itself, but it would take the drift to −400 kcal, past
    your limit of 390 (15 % of your TDEE). Accepting it makes it a target
    you agreed to: the drift starts again at 0." While its proposal waits,
    the Autopilot card says only "It waits for your answer, above", so
    nothing is said twice.
11. **On the day, one line under the hero, on the review day only.** A
    step uses the brand's colours ("Autopilot: target −100 kcal a day,
    from today · Why"), because the ring above it changed. A hold or no
    step is a neutral line. A proposal reuses the proposal nudge with the
    autopilot's wording and icon ("The autopilot suggests: training days
    2,350 → 2,250 kcal"). Every line links to the plan. After that day,
    the morning summary and the plan carry it.
12. **A message for every outcome, a hold and no step included** (HAX
    G18; Carbon), and the morning summary gets an "Autopilot:" line right
    after "Calibration:". Push titles state the outcome, and the body
    gives the numbers and what to do. Telegram gets the whole review
    (decision, why, drift, next review, link). There is no blame: "3 of 4
    logged days", never "you didn't log".
13. **A step up looks like a step down, with the sign and verb swapped**
    ("Raised your target by 100 kcal a day"). The lean-mass and fat-gain
    guards each get a sentence of their own reason.
14. **Underweight is a plan-change proposal ("Suggests maintenance"), never
    a step.** The card shows the new phase and the targets at the TDEE.
    The copy names the BMI and the rule.
15. **Use the brand colour for the dot on a step, the warning colour on a
    proposal (it needs the user), the success colour on a hold, and a
    muted colour for no step.** The words always say which; colour is
    never the only signal.

## 3. Screens

All screenshots are in this folder: `<screen>-<phone|desktop>-<light|dark>-<en|hr>.png`.

### Settings, `/plan/autopilot`

- Autopilot on: [phone light en](settings-autopilot-phone-light-en.png),
  [phone dark hr](settings-autopilot-phone-dark-hr.png),
  [phone light hr](settings-autopilot-phone-light-hr.png),
  [desktop light](settings-autopilot-desktop-light-en.png),
  [desktop dark](settings-autopilot-desktop-dark-en.png)
- Ask first (no drift shown): [settings-propose-phone-light-en](settings-propose-phone-light-en.png)
- The confirmation when turning it on: [settings-turn-on-phone-light-en](settings-turn-on-phone-light-en.png)
- Drift limit reached: [settings-drift-full-phone-light-en](settings-drift-full-phone-light-en.png)
- The row in "You and limits": [plan-list-row-phone-light-en](plan-list-row-phone-light-en.png)

### Plan

- Step applied: [phone light en](plan-step-phone-light-en.png),
  [phone dark hr](plan-step-phone-dark-hr.png),
  [phone light hr](plan-step-phone-light-hr.png),
  [desktop light](plan-step-desktop-light-en.png),
  [desktop dark](plan-step-desktop-dark-en.png); the step in history:
  [history-step-phone-light-en](history-step-phone-light-en.png)
- Ask first, a step proposed: [plan-propose-phone-light-en](plan-propose-phone-light-en.png)
- Past the drift limit, proposes: [phone light en](plan-drift-phone-light-en.png),
  [phone dark hr](plan-drift-phone-dark-hr.png),
  [phone light hr](plan-drift-phone-light-hr.png),
  [desktop light](plan-drift-desktop-light-en.png),
  [desktop dark](plan-drift-desktop-dark-en.png)
- Hold, pace on plan: [plan-hold](plan-hold-phone-light-en.png)
- Hold, steps explain it: [plan-steps-guard](plan-steps-guard-phone-light-en.png)
- No step, 3 of 4 logged days: [plan-few-data](plan-few-data-phone-light-en.png)
- Before calibration trusts the TDEE: [plan-not-trusted](plan-not-trusted-phone-light-en.png)
- A new phase, first step later: [plan-first-wait](plan-first-wait-phone-light-en.png)
- No phase or target: [plan-no-plan](plan-no-plan-phone-light-en.png)
- Waits for a coach's proposal: [plan-pending-coach](plan-pending-coach-phone-light-en.png)
- Underweight, maintenance proposed: [plan-underweight](plan-underweight-phone-light-en.png)
- Bulk too slow, step up: [plan-bulk-up](plan-bulk-up-phone-light-en.png)
- Lean mass lost on a cut, step up: [plan-lean-loss](plan-lean-loss-phone-light-en.png)
- Too few carbs, ask your coach: [plan-few-carbs](plan-few-carbs-phone-light-en.png)

### Day

- Step applied: [phone light en](day-step-phone-light-en.png),
  [phone dark hr](day-step-phone-dark-hr.png),
  [phone light hr](day-step-phone-light-hr.png),
  [desktop light](day-step-desktop-light-en.png),
  [desktop dark](day-step-desktop-dark-en.png)
- Hold: [day-hold](day-hold-phone-light-en.png). No step: [day-few-data](day-few-data-phone-light-en.png).
  Proposal (the nudge): [day-proposal](day-proposal-phone-light-en.png)
- Morning summary with the autopilot's line: [phone light en](day-morning-phone-light-en.png),
  [phone light hr](day-morning-phone-light-hr.png),
  [phone dark hr](day-morning-phone-dark-hr.png),
  [desktop light](day-morning-desktop-light-en.png),
  [desktop dark](day-morning-desktop-dark-en.png)

### Messages

[messages-desktop-light-en](messages-desktop-light-en.png) shows every state's
push, Telegram message, morning line and day line, in English and Croatian.

## 4. Copy

The source is `web/src/proto/copy.ts`. When this is implemented, the app's
strings move to `web/src/i18n/{en,hr}.ts`, and the message strings move to
the server's phrases in `internal/api/morning.go` and the new notifier.
The Croatian is written gender-neutral wherever it addresses the user ("unos
je bio", "dogovoreni cilj"), because the app does not know the user's
gender. The autopilot itself is masculine ("Spustio je cilj").

The tables below come from the prototype, for a review on Mon, Nov 2.
Numbers are formatted by the browser's Intl; the server will write them
its own way (`2079`, `−0,31 %/tj`).

### App strings (English / Croatian)

| Key | English | Croatian |
| --- | --- | --- |
| name | Autopilot | Autopilot |
| event | Weekly review | Tjedni pregled |
| modes | Ask me first · Autopilot | Pitaj me prvo · Autopilot |
| badge | On / Asks first | Uključen / Pita prvo |
| hint, propose | Each week the autopilot reviews your pace and suggests a step. Nothing changes until you accept it; a suggestion waits 7 days. | Svaki tjedan autopilot pregleda tvoj tempo i predloži korak. Ništa se ne mijenja dok ga ne prihvatiš; prijedlog čeka 7 dana. |
| hint, autopilot | Each week the autopilot reviews your pace and takes a small step by itself, within your limits, and tells you. Past your drift limit it asks you instead. | Svaki tjedan autopilot pregleda tvoj tempo i sam napravi mali korak, unutar tvojih granica, i javi ti. Preko granice odstupanja umjesto toga te pita. |
| turn on | Turn on the autopilot? It will change your kcal target by itself, by at most 100 kcal a day a week and 390 kcal in all, and tell you each time. You can take any step back in the plan's history. | Uključiti autopilot? Sam će mijenjati tvoj cilj kcal, najviše 100 kcal dnevno u tjednu i ukupno 390 kcal, i javiti ti svaki put. Svaki korak možeš vratiti u povijesti plana. |
| may | Your kcal target, by at most 100 kcal a day in a week (5% of it at most) · Training and rest days by the same kcal, so only carbs move: 100 kcal is 25 g · In all, at most 390 kcal from the target you last agreed to (15% of your TDEE) | Tvoj cilj kcal, najviše 100 kcal dnevno u tjednu (najviše 5 % cilja) · Dane treninga i odmora za isto kcal, pa se mijenjaju samo ugljikohidrati: 100 kcal je 25 g · Ukupno najviše 390 kcal od zadnjeg dogovorenog cilja (15 % tvog TDEE-a) |
| never | Your phase, its pace or its goal · Protein, fat, or the split between training and rest days · Your training days or your limits · A loss deeper while your BMI is under 18.5: it proposes maintenance instead | Tvoju fazu, njen tempo ni cilj · Proteine, masti ni razliku između dana treninga i odmora · Tvoje dane treninga ni tvoje granice · Dublji gubitak dok ti je BMI ispod 18,5: tada predlaže održavanje |
| waits | 4 logged days and 3 weigh-ins in the last 7 days · A TDEE that calibration trusts · 14 days into a phase, or from a target you set; 7 days since its last step · No other proposal waiting for your answer | 4 upisana dana i 3 vaganja u zadnjih 7 dana · TDEE kojem kalibracija vjeruje · 14 dana od početka faze ili novog cilja; 7 dana od zadnjeg koraka · Da nijedan drugi prijedlog ne čeka tvoj odgovor |
| undo | Undo (⌘Z) doesn't take back a step: take it back in the plan's history, as any change of the plan. | Poništi (⌘Z) ne vraća korak: vrati ga u povijesti plana, kao svaku promjenu plana. |
| drift | −200 of 390 kcal · 190 kcal left · Limit reached: the next step is proposed | −200 od 390 kcal · ostalo 190 kcal · Granica dosegnuta: sljedeći korak bit će prijedlog |
| past drift | On autopilot this step would apply by itself, but it would take the drift to −400 kcal, past your limit of 390 (15 % of your TDEE). Accepting it makes it a target you agreed to: the drift starts again at 0. | Na autopilotu bi se ovaj korak primijenio sam, ali odstupanje bi bilo −400 kcal, preko tvoje granice od 390 (15 % TDEE-a). Prihvaćen postaje dogovoreni cilj: odstupanje kreće od 0. |

### What a review says, by state

#### English

| State | Push title | Push body | Morning summary | Day line |
| --- | --- | --- | --- | --- |
| Autopilot · step down applied | Target -100 kcal a day | Weekly review: losing 0.31%/wk, plan 0.50%. New average 2,079 kcal a day. Tap to see why, or take it back. | Autopilot: target -100 kcal/day, to 2,079 (losing 0.31%/wk, plan 0.50%) | Autopilot: target -100 kcal a day, from today |
| Ask first · a step proposed | Suggests -100 kcal a day | Weekly review: losing 0.31%/wk, plan 0.50%. Accept or reject in the plan by Mon, Nov 9. | Autopilot: suggests -100 kcal/day (losing 0.31%/wk, plan 0.50%), waits for your answer | (the proposal line) |
| Autopilot · hold, pace on plan | Holding: pace on plan | Weekly review: losing 0.47%/wk, plan 0.50%. The target stays 2,179 kcal a day. | Autopilot: holding, pace on plan (losing 0.47%/wk, plan 0.50%) | Autopilot: holding, pace on plan |
| Autopilot · hold, steps explain it | Holding: steps explain it | Weekly review: losing 0.31%/wk, plan 0.50%, but your steps fell 34 %. The target stays 2,179 kcal. | Autopilot: holding, steps -34 % explain the pace | Autopilot: holding, your steps explain it |
| Autopilot · no step, 3/4 logged days | No step: 3/4 logged days | A step needs 4 logged days and 3 weigh-ins in the last 7. Next review Mon, Nov 9. | Autopilot: no step, 3/4 logged days | Autopilot: no step, 3/4 logged days |
| Autopilot · TDEE not trusted yet | No step: TDEE not trusted yet | Your TDEE is about 2,540 ± 310 kcal. A first step about Wed, Nov 18, if you keep logging as now. | Autopilot: no step, TDEE not trusted yet | Autopilot: no step, TDEE not trusted yet |
| Autopilot · new phase, first step later | No step: the phase is new | A phase’s first 14 days move water as much as fat. The first step can come from Wed, Nov 11. | Autopilot: no step, the phase is new | Autopilot: no step, the phase is new |
| Autopilot · no phase or target | No step: no phase or target | Set a phase and a kcal target in your plan, and the review judges your pace against them. | Autopilot: no step, no phase or target | Autopilot: no step, no phase or target |
| Autopilot · waits for a coach’s proposal | No step: a proposal waits | Answer the proposal in your plan first; the review waits for it. Next review Mon, Nov 9. | Autopilot: no step, a proposal waits | Autopilot: no step, a proposal waits |
| Autopilot · past the drift limit, proposes | Suggests -100 kcal a day | Weekly review: losing 0.28%/wk, plan 0.50%. It’s past your drift limit (-400 of 390 kcal), so it asks first. Answer by Mon, Nov 9. | Autopilot: suggests -100 kcal/day (losing 0.28%/wk, plan 0.50%), waits for your answer | (the proposal line) |
| Autopilot · underweight, maintenance proposed | Maintenance suggested | Your BMI is 18.4, under 18.5, so the autopilot won’t take the loss further. Accept or reject in the plan. | Autopilot: suggests maintenance, your BMI is under 18.5 | (the proposal line) |
| Autopilot · bulk too slow, step up | Target +100 kcal a day | Weekly review: gaining 0.08%/wk, plan 0.25%. New average 2,886 kcal a day. Tap to see why, or take it back. | Autopilot: target +100 kcal/day, to 2,886 (gaining 0.08%/wk, plan 0.25%) | Autopilot: target +100 kcal a day, from today |
| Autopilot · too few carbs, ask a coach | No step: too few carbs | Rest days have 63 g carbs, and it never changes protein or fat. Ask your coach to change the split. | Autopilot: no step, too few carbs | Autopilot: no step, too few carbs |
| Autopilot · lean mass lost, step up | Target +100 kcal a day | Weekly review: losing 0.62%/wk, plan 0.50%. New average 2,279 kcal a day. Tap to see why, or take it back. | Autopilot: target +100 kcal/day, to 2,279 (losing 0.62%/wk, plan 0.50%) | Autopilot: target +100 kcal a day, from today |

#### Croatian

| State | Push title | Push body | Morning summary | Day line |
| --- | --- | --- | --- | --- |
| Autopilot · step down applied | Cilj −100 kcal dnevno | Tjedni pregled: tempo −0,31 %/tj, plan −0,50 %/tj. Novi prosjek 2.079 kcal dnevno. Dodirni za razlog ili vraćanje. | Autopilot: cilj −100 kcal/dan, na 2.079 (tempo −0,31 %/tj, plan −0,50 %/tj) | Autopilot: cilj −100 kcal dnevno, od danas |
| Ask first · a step proposed | Predlaže −100 kcal dnevno | Tjedni pregled: tempo −0,31 %/tj, plan −0,50 %/tj. Prihvati ili odbij u planu do pon, 9. stu. | Autopilot: predlaže −100 kcal/dan (tempo −0,31 %/tj, plan −0,50 %/tj), čeka tvoj odgovor | (the proposal line) |
| Autopilot · hold, pace on plan | Bez promjene: tempo u planu | Tjedni pregled: tempo −0,47 %/tj, plan −0,50 %/tj. Cilj ostaje 2.179 kcal dnevno. | Autopilot: bez promjene, tempo u planu (tempo −0,47 %/tj, plan −0,50 %/tj) | Autopilot: bez promjene, tempo u planu |
| Autopilot · hold, steps explain it | Bez promjene: koraci | Tjedni pregled: tempo −0,31 %/tj, plan −0,50 %/tj, ali koraka je 34 % manje. Cilj ostaje 2.179 kcal. | Autopilot: bez promjene, koraci −34 % objašnjavaju tempo | Autopilot: bez promjene, objašnjavaju to koraci |
| Autopilot · no step, 3/4 logged days | Bez koraka: 3/4 upisana dana | Korak treba 4 upisana dana i 3 vaganja u zadnjih 7. Sljedeći pregled pon, 9. stu. | Autopilot: bez koraka, 3/4 upisana dana | Autopilot: bez koraka, 3/4 upisana dana |
| Autopilot · TDEE not trusted yet | Bez koraka: TDEE još nije pouzdan | TDEE je oko 2.540 ± 310 kcal. Prvi korak oko sri, 18. stu, ako nastaviš upisivati kao sada. | Autopilot: bez koraka, TDEE još nije pouzdan | Autopilot: bez koraka, TDEE još nije pouzdan |
| Autopilot · new phase, first step later | Bez koraka: faza je nova | Prvih 14 dana faze miče vodu koliko i mast. Prvi korak može doći od sri, 11. stu. | Autopilot: bez koraka, faza je nova | Autopilot: bez koraka, faza je nova |
| Autopilot · no phase or target | Bez koraka: nema faze ni cilja | Postavi fazu i cilj kcal u planu, pa će pregled ocjenjivati tempo prema njima. | Autopilot: bez koraka, nema faze ni cilja | Autopilot: bez koraka, nema faze ni cilja |
| Autopilot · waits for a coach’s proposal | Bez koraka: prijedlog čeka | Prvo odgovori na prijedlog u planu; pregled ga čeka. Sljedeći pregled pon, 9. stu. | Autopilot: bez koraka, prijedlog čeka | Autopilot: bez koraka, prijedlog čeka |
| Autopilot · past the drift limit, proposes | Predlaže −100 kcal dnevno | Tjedni pregled: tempo −0,28 %/tj, plan −0,50 %/tj. To je preko granice odstupanja (−400 od 390 kcal), pa pita prvo. Odgovori do pon, 9. stu. | Autopilot: predlaže −100 kcal/dan (tempo −0,28 %/tj, plan −0,50 %/tj), čeka tvoj odgovor | (the proposal line) |
| Autopilot · underweight, maintenance proposed | Predlaže održavanje | BMI ti je 18,4, ispod 18,5, pa autopilot ne nastavlja gubitak. Prihvati ili odbij u planu. | Autopilot: predlaže održavanje, BMI je ispod 18,5 | (the proposal line) |
| Autopilot · bulk too slow, step up | Cilj +100 kcal dnevno | Tjedni pregled: tempo +0,08 %/tj, plan +0,25 %/tj. Novi prosjek 2.886 kcal dnevno. Dodirni za razlog ili vraćanje. | Autopilot: cilj +100 kcal/dan, na 2.886 (tempo +0,08 %/tj, plan +0,25 %/tj) | Autopilot: cilj +100 kcal dnevno, od danas |
| Autopilot · too few carbs, ask a coach | Bez koraka: premalo UH | Dani odmora imaju 63 g UH, a proteine i masti nikad ne mijenja. Zamoli trenera da promijeni raspodjelu. | Autopilot: bez koraka, premalo UH | Autopilot: bez koraka, premalo UH |
| Autopilot · lean mass lost, step up | Cilj +100 kcal dnevno | Tjedni pregled: tempo −0,62 %/tj, plan −0,50 %/tj. Novi prosjek 2.279 kcal dnevno. Dodirni za razlog ili vraćanje. | Autopilot: cilj +100 kcal/dan, na 2.279 (tempo −0,62 %/tj, plan −0,50 %/tj) | Autopilot: cilj +100 kcal dnevno, od danas |

#### Telegram, two examples

```text
Autopilot · weekly review, Monday, November 2
Lowered your target by 100 kcal a day.
You lost 0.31% a week; the plan is 0.50% (0.40% to 0.60%).
Your TDEE is 2,600 ± 120 kcal; you ate 2,140 kcal a day against a target of 2,179.
Both types of day lose 100 kcal, all of it carbs (−25 g): 2,179 → 2,079 kcal a day on average.
Drift: -200 of 390 kcal.
Next review Mon, Nov 9. Take it back in the plan: /plan
```

```text
Autopilot · tjedni pregled, ponedjeljak, 2. studenoga
Spustio je cilj za 100 kcal dnevno.
Gubiš 0,31 % tjedno; plan je 0,50 % (0,40 % do 0,60 %).
TDEE je 2.600 ± 120 kcal; unos je bio 2.140 kcal dnevno uz cilj od 2.179.
Oba tipa dana gube 100 kcal, sve iz ugljikohidrata (−25 g): prosjek 2.179 → 2.079 kcal dnevno.
Odstupanje: −200 od 390 kcal.
Sljedeći pregled pon, 9. stu. Vrati u planu: /plan
```

```text
Autopilot · weekly review, Monday, November 2
No step: 3 of 4 logged days.
A step needs 4 logged days and 3 weigh-ins in the last 7 days. This week had 3 logged days and 4 weigh-ins.
Drift: -100 of 390 kcal.
Next review Mon, Nov 9.
```

```text
Autopilot · tjedni pregled, ponedjeljak, 2. studenoga
Bez koraka: 3 od 4 upisana dana.
Korak treba 4 upisana dana i 3 vaganja u zadnjih 7 dana. Ovaj tjedan: 3 upisana dana i 4 vaganja.
Odstupanje: −100 od 390 kcal.
Sljedeći pregled pon, 9. stu.
```

## 5. Where the agreed rules caused UX problems

1. **"Check-in" is already taken.** The daily card is "Check-in" ("How
   was today?"). The UI says "Weekly review" instead. The code, the
   Vikunja story and DESIGN.md can keep "check-in" if the maintainer
   prefers, but the user should never see two different check-ins.
2. **Drift needs a TDEE before calibration trusts one.** limits.md says
   to use "the measured one, else the equation's estimate". The prototype
   therefore shows "0 of 381 kcal" from an untrusted ≈2,540. That number
   moves every week until the TDEE is trusted, which means the limit
   moves under the user. Suggestion: freeze the drift limit at the first
   step, or show "—" until there is one, since no step can come before a
   trusted TDEE anyway.
3. **A review that cannot step for structural reasons repeats its message
   every week.** With no phase or target, or with too few carbs, the
   "every review sends a message" rule produces the same "No step: no
   phase or target" every Monday. Suggestion: send it once, then stay
   silent (the plan's card still says it) until something changes.
4. **"Too few carbs to lower" has no threshold.** The rules say "a target
   with too few carbs for a step down". The UI needs the number, the
   carbs left after a 25 g step, against a floor that is not defined yet
   (the prototype shows 64 g on rest days). This needs a limit in
   limits.md before it can be explained.
5. **The first step after calibration is trusted is not a review day.**
   `calibration.Expected` gives a date (Mon, Nov 16), but steps only come
   on the review weekday. The copy should name the first review on or
   after it (Sat, Nov 21). The API should give that date, not the app
   (see `first_step` below).
6. **Undo versus revert.** The rule that ⌘Z does not take a step back is
   right, but users will try it, because the toast system teaches them
   that Undo takes back "the latest change". The settings page and the
   step's push ("Tap to see why, or take it back") point to the history.
   The recent-changes sheet should list the step as "by Autopilot" with
   no Undo button.
7. **A rejected autopilot proposal.** The rules do not say whether the
   next review proposes the same step again a week later. If the data
   still ask for it, it will, and the user rejects the same step every
   week. Suggestion: after a reject, the next review proposes only if the
   gap grew, and the card says "You rejected it. The target stays; the
   next review looks again."

## 6. Open questions for the maintainer

1. Is it right to put the settings on the plan, under "You and limits"
   (`/plan/autopilot`), rather than in Settings?
2. Should the day's line show only on the review day, or until the user
   opens the plan?
3. Should turning the autopilot **off** also ask for confirmation? The
   prototype does not ask: it is the safe direction.
4. Should the push for a hold or a no step be sent with lower priority,
   or only to Telegram? The rule says every review notifies. A weekly
   "holding" push may teach users to ignore the autopilot's pushes.
5. Should drift be signed ("−200") or plain ("200 kcal moved down")? The
   prototype shows it signed, because ups and downs cancel.
6. Should a step taken back in history count toward drift? The
   prototype's mock leaves drift as it was. The rules imply that a revert
   is a target the user set, so drift would reset to 0. Please confirm.
7. Should the step's ratio of kcal to carbs (100 kcal = 25 g) be shown
   on every step, or only in "The numbers"? The prototype shows it in
   "The numbers" and in Telegram.
8. Weekday names in the chips: Croatian uses lower case ("pon", "uto"),
   as the week strip does. Is that OK?

## 7. What the API needs to give the web app

The new types are in `web/src/proto/types.ts`. Everything is per user,
and the plan's operations already isolate it.

### `GET /api/v1/autopilot` (web session; tokens may read, never write)

| Field | Type | Used by |
| --- | --- | --- |
| `mode` | `propose` \| `autopilot` | plan card badge, settings switch, row summary |
| `weekday` | weekday | settings chips, row summary, card caption |
| `next` | date | the next review (card, settings hint, messages) |
| `first_step` | date, optional | the first review that may step (after the 14-day wait or calibration's expected date), so the app never computes it |
| `step_kcal` | number | the step its limits allow now (card in propose mode, turn-on text, drift-full check) |
| `drift.used_kcal` | signed number | the drift meter |
| `drift.limit_kcal`, `drift.limit_pct` | number | the drift meter and text |
| `drift.base_kcal`, `drift.base_from` | number, date | "since you agreed to 2,279 kcal a day on Sep 26" |
| `drift.tdee_kcal` | number | the turn-on text's "390 kcal in all" |
| `last` | `CheckIn`, optional | the card, the day's line, the morning line |

### `PUT /api/v1/autopilot` (web session only; never MCP or a token)

Takes `{ mode?, weekday? }` and answers like the GET. It is an event in
the audit trail, but Undo does not apply to it.

### `CheckIn`, a review (also `ProposalBody.check_in` on a proposal or revision the autopilot made)

| Field | Notes |
| --- | --- |
| `date` | the review's date |
| `outcome` | `applied` \| `proposed` \| `hold` \| `skipped` |
| `reason` | `cut_slow`, `cut_fast`, `bulk_slow`, `bulk_fast`, `bulk_fat`, `lean_loss`, `on_target`, `steps_guard`, `not_trusted`, `no_plan`, `few_data`, `first_wait`, `interval`, `pending`, `few_carbs`, `underweight` |
| `step_kcal` | signed: the step applied or proposed |
| `past_drift` | true when it proposes only because of the drift limit |
| `proposal_id` | the proposal it made, for "it waits for your answer, above" |
| `measured_pct_week`, `target_pct_week`, `tolerance_pct_week` | the verdict's pace |
| `tdee_kcal`, `tdee_half_width` | the TDEE and its ± |
| `intake_kcal`, `target_kcal`, `after_kcal` | the week's intake, and the weekly average of the target before and after |
| `logged_days`, `weigh_ins`, `needs_logged_days`, `needs_weigh_ins` | for `few_data` |
| `steps_change_pct` | for `steps_guard` |
| `from`, `expected` | for `first_wait` and `not_trusted` |
| `carbs_g` (and the floor once it exists) | for `few_carbs` |
| `bmi` | for `underweight` |
| `answer` | `accepted` \| `rejected`, once the user has answered its proposal |

### Changes to existing answers

- `ActorBody.client` takes the value `autopilot`, with no model.
  `useWho` says "Autopilot".
- `ProposalBody.rationale` takes the code `autopilot`, and
  `ProposalBody.check_in` holds its numbers. The app renders it in the
  user's language, as it does `changed in the app`.
- `GET /api/v1/changes` (recent changes) lists a step as by `autopilot`,
  with no undo.
- `GET /api/v1/morning`: the server adds the "Autopilot:" line (§4) after
  "Calibration:". The short form for a push adds the outcome's title.
- Notifications: a new push per review (title and body as in §4, URL
  `/plan`, tag `autopilot`) and a Telegram message (§4).

## 8. What the prototype changed in the app (for whoever implements it)

These are the prototype's own files and edits. They mark where the real
code goes.

- New: `web/src/components/autopilot/` (`autopilot-card.tsx`,
  `autopilot-line.tsx`, `drift-meter.tsx`, `rationale.tsx`, `words.ts`)
  and `web/src/pages/autopilot.tsx`.
- Edited: `pages/plan.tsx` (the card), `pages/day.tsx` (the line),
  `components/plan/you-list.tsx` (the row), `revisions.tsx` and
  `pending-proposal.tsx` (the rationale, and no "What the data say" on
  the autopilot's own proposal), `proposal-nudge.tsx` (its wording and
  icon), `change-words.ts` (`useWho`), `router.tsx`, `list-row.tsx`.
- Prototype only, to drop: `web/src/proto/*` (the mock, its states and
  copy, the panel and the messages page), the middleware line in
  `api/client.ts`, the panel in `root-layout.tsx`,
  `web/e2e/proto-shots.ts`, `scripts/proto-autopilot.sh`.
