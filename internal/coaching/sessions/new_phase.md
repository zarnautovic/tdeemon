# Plan a new phase

The user wants a new phase: a cut, maintenance, a bulk or a recomp, now
or after the current one. Take the steps in order.

1. Call `get_profile`, `get_plan`, `get_summary`, and `get_history`
   with `granularity` `week` from 12 weeks before today. Read the
   methodology below before you explain a number.
2. A plan needs the whole profile (sex, birth date, height): without
   it, ask the user to fill it in the app, and stop.
3. **The goal:** ask what the user has not said, one question at a
   time: the type of phase, when it starts, a goal or milestone weight,
   how long, and the training weekdays. Read the notes for a coach.
4. **The pace:** the phase type's default rate and the range the
   limits allow (`get_plan`'s `limits`). Say what it means in kg a week
   at the trend weight, and about when a goal weight would come.
5. **The targets:** start from the measured TDEE (not trusted: its
   estimate and interval, a first target calibration corrects) and the
   energy of the rate (7700 kcal a kg). Protein in g/kg of the reference
   mass within the limits (by default 2.2 on a cut, 1.6 at
   maintenance, 1.8 on a bulk, 2.0 on a recomp), fat at least the fat
   floor, the rest carbs; training and rest days split as the user
   wants. The preview says what the targets come to: check the rate
   there, and adjust.
6. **Its place:** a phase after the current starts the day after the
   current one's last day (a phase's end is its last day). To replace
   the current one, end it yesterday and start the new one today: a
   phase that has started keeps its type. A long cut may want a diet
   break before the next.
7. `propose_changes` with the phase, the targets, the training weekdays
   when they change, the rationale and the window analysed: show its
   preview, and ask. A limit in the way changes in the same proposal,
   which only the app applies, after the user's yes. `apply_proposal`
   only after the user's yes to it.
8. Answer in the user's language, the numbers as the tools give them,
   each with its uncertainty.
