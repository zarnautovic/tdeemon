# Monthly review

A review of the plan's last weeks: how they went, and whether the plan
should change. Take the steps in order.

1. Call `get_plan`, `get_summary`, and `get_history` with `granularity`
   `week` from 5 weeks before today. Read the methodology below before
   you explain a number.
2. **Where the user stands:** the phase and its week, the trend weight
   now and its change over the weeks, the measured TDEE with its
   interval, and whether it is trusted.
3. **Adherence**, week by week: the intake against the target, kcal and
   protein; the days logged and the weigh-ins; the excluded days and
   why.
4. **The verdict:** the measured rate against the target rate, its
   action, and the guards and flags. Without a verdict, say its reasons:
   what is missing and, from `expected` or `before_phase_until`, when it
   comes. Change nothing on a TDEE not trusted.
5. **The autopilot:** its mode, the steps and proposals among the
   revisions since the last review, its drift against its limit, and its
   next review. What it already does, a coach does not do again.
6. **Recovery**, if measured: sleep, resting heart rate and HRV against
   their usual range.
7. **Conclusion:** what goes well and what to change. An intake that
   missed the target asks for eating to the target, not for another
   target. Propose a change only when the data ask for it and the
   autopilot does not already take it: `propose_changes` with the
   rationale and the window analysed, show its preview, and ask.
   `apply_proposal` only after the user's yes to it.
8. Answer in the user's language, the numbers as the tools give them,
   each with its uncertainty.
