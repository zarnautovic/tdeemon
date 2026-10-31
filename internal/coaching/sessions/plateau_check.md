# Plateau check

The user feels the weight has stalled. First check that the stall is
real, then weigh the options. Take the steps in order.

1. Call `get_plan`, `get_summary`, and `get_history` with `granularity`
   `week` from 8 weeks before today. Read the methodology below before
   you explain a number.
2. **Is it real?** A stall is the verdict of the phase's own days: on a
   cut, a loss slower than planned is the rate `above`; on a bulk, a
   gain slower than planned is `below`. It is no stall yet when the
   verdict is `before_phase` (the phase's first 28 days) or
   `not_trusted`, when the intake changed in the last 14 days (water),
   or when the trend covers less than two weeks. Say so, and when the
   data will tell.
3. **Adherence:** the intake against the target over the window and
   week by week, the days not logged or excluded, and the weekends. A
   stall whose intake missed a target the verdict holds (`hold`) is the
   intake's: say so, and change no target. An action that changes the
   target asks for that too.
4. **Activity:** the steps of the last weeks against those before (the
   guard `activity`, the flag `no_steps`). Fewer steps explain a slower
   loss.
5. **The autopilot:** its mode, its steps in this phase, its drift
   against its limit, its next review, and a proposal of its that
   waits. A stall it already steps against needs nothing more from a
   coach; a proposal of its that waits may be the answer.
6. **Options**, only for a real stall the autopilot does not take: let
   the autopilot go on; a step of the target toward `gap_kcal`; after a
   long cut, a diet break at the TDEE for one or two weeks; or the end
   of the phase. Weigh each against the limits (the kcal floor, the
   rate, the fat floor, 130 g of carbs) and the recovery (sleep, resting
   heart rate, HRV).
7. The option the user chooses is `propose_changes` with the rationale
   and the window analysed: show its preview, and ask.
   `apply_proposal` only after the user's yes to it.
8. Answer in the user's language, the numbers as the tools give them,
   each with its uncertainty.
