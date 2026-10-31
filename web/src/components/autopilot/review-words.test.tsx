import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { useReviewWords } from '@/components/autopilot/review-words'
import { setLocale } from '@/i18n/locale'
import { fakeServer, signedIn } from '@/test/app'
import { review } from '@/test/plan'

type Review = components['schemas']['ReviewBody']

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
})

// say is the words of each review, in the language set.
function say(reviews: Review[]) {
  fakeServer(signedIn())
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  const { result } = renderHook(() => useReviewWords(), { wrapper })
  return reviews.map((r) => result.current(r))
}

test('a review says each reason in its words, from its numbers', () => {
  const cases: [Partial<Review>, string, string][] = [
    [
      {
        outcome: 'applied',
        reason: 'bulk_slow',
        step_kcal: 100,
        measured_pct_week: 0.08,
        target_pct_week: 0.25,
      },
      'Raised your target by 100 kcal a day',
      'You gained 0.08% a week; the plan is 0.25% (0.15% to 0.35%).',
    ],
    [
      { outcome: 'hold', reason: 'on_target', step_kcal: 0, measured_pct_week: -0.47 },
      'Holding: your pace is on plan',
      'You lost 0.47% a week; the plan is 0.5% (0.4% to 0.6%).',
    ],
    [
      { outcome: 'hold', reason: 'steps_guard', step_kcal: 0, steps_change_pct: -34 },
      'Holding: your steps explain the pace',
      'You lost 0.31% a week; the plan is 0.5% (0.4% to 0.6%).',
    ],
    [
      { outcome: 'hold', reason: 'intake', step_kcal: 0 },
      'Holding: the target fits, the intake missed it',
      'You lost 0.31% a week; the plan is 0.5% (0.4% to 0.6%).',
    ],
    [
      { outcome: 'hold', reason: 'rejected', step_kcal: 0 },
      'Holding: the target you chose stays',
      'You rejected its last step. The target stays as you chose until the gap grows by a step.',
    ],
    [
      { outcome: 'skipped', reason: 'few_data', step_kcal: 0, logged_days: 3 },
      'No step: 3 of 4 logged days',
      'A step needs 4 logged days and 3 weigh-ins in the last 7 days. This week had 3 and 5.',
    ],
    [
      { outcome: 'skipped', reason: 'few_data', step_kcal: 0, weigh_ins: 2 },
      'No step: 2 of 3 weigh-ins',
      'A step needs 4 logged days and 3 weigh-ins in the last 7 days. This week had 6 and 2.',
    ],
    [
      {
        outcome: 'skipped',
        reason: 'not_trusted',
        step_kcal: 0,
        tdee_half_width: 310,
        expected: '2026-11-18',
      },
      'No step: TDEE not trusted yet',
      'Your TDEE is about 2,600 ± 310 kcal. A step waits until calibration trusts it: about Nov 18, if you keep logging as now.',
    ],
    [
      { outcome: 'skipped', reason: 'not_trusted', step_kcal: 0, tdee_kcal: 0 },
      'No step: TDEE not trusted yet',
      'A step waits until calibration trusts it. Keep logging and weighing in.',
    ],
    [
      { outcome: 'skipped', reason: 'first_wait', step_kcal: 0, from: '2026-11-06' },
      'No step: the phase or target is new',
      'A new phase or target moves water as much as fat at first. The first step can come from Nov 6.',
    ],
    [
      { outcome: 'skipped', reason: 'interval', step_kcal: 0 },
      'No step: a step a week at most',
      'The autopilot takes one step a week at most.',
    ],
    [
      { outcome: 'skipped', reason: 'pending', step_kcal: 0 },
      'No step: a proposal waits',
      "A proposal waits for your answer. The review waits too, so two changes don't meet in your plan.",
    ],
    [
      { outcome: 'skipped', reason: 'few_carbs', step_kcal: 0, carbs_g: 105, day_type: 'rest' },
      'No step: too few carbs',
      'Rest days: 105 g carbs after a step down, under 130 g. The autopilot never changes protein or fat. Ask your AI coach to change the split.',
    ],
    [
      { outcome: 'skipped', reason: 'no_plan', step_kcal: 0 },
      'No step: no phase or target',
      'The autopilot needs a phase and a kcal target to judge your pace against.',
    ],
    [
      { outcome: 'skipped', reason: 'limit', step_kcal: 0, rule: 'kcal_floor' },
      'No step: a limit',
      'The step would break a limit: the kcal of a day. Your AI coach can look at the plan.',
    ],
    [
      { outcome: 'proposed', reason: 'underweight', step_kcal: 0, bmi: 18.4 },
      'Suggests maintenance',
      'Your BMI is 18.4, under 18.5. The autopilot never takes a loss further there, so it proposes maintenance.',
    ],
    [
      { outcome: 'applied', reason: 'lean_loss', step_kcal: 100, measured_pct_week: -0.62 },
      'Raised your target by 100 kcal a day',
      'You lost 0.62% a week, and lean mass with it beyond its error. A step up keeps more of it.',
    ],
    [
      { outcome: 'applied', reason: 'bulk_fat', step_kcal: -100 },
      'Lowered your target by 100 kcal a day',
      'Your body fat rose beyond its error while lean mass stayed flat. A smaller surplus, never below your TDEE.',
    ],
    [
      {
        outcome: 'applied',
        reason: 'maintain_gain',
        step_kcal: -100,
        measured_pct_week: 0.2,
        target_pct_week: 0,
      },
      'Lowered your target by 100 kcal a day',
      'Your weight moved 0.2% a week; the plan is 0% (-0.1% to 0.1%).',
    ],
  ]
  const said = say(cases.map(([over]) => review(over)))
  expect(said.map((w) => [w.headline, w.why[0]])).toEqual(
    cases.map(([, headline, why]) => [headline, why]),
  )
})

test("a review's line for the day says what it did in a few words", () => {
  const [step, hold, skip, proposed] = say([
    review(),
    review({ outcome: 'hold', reason: 'steps_guard', step_kcal: 0 }),
    review({ outcome: 'skipped', reason: 'few_data', step_kcal: 0, logged_days: 3 }),
    review({ outcome: 'proposed' }),
  ])
  expect(step?.line).toBe('target -100 kcal a day, from today')
  expect(hold?.line).toBe('holding, your steps explain it')
  expect(skip?.line).toBe('no step, 3 of 4 logged days')
  expect(proposed?.line).toBe('suggests lowering your target by 100 kcal a day')
  // A step, or one proposed, says what it moves: carbs alone.
  expect(step?.why.at(-1)).toBe(
    'Each type of day moves -100 kcal, all carbs (-25 g): 2,179 → 2,079 kcal a day on average.',
  )
  expect(hold?.why).toEqual([
    'You lost 0.31% a week; the plan is 0.5% (0.4% to 0.6%).',
    'Your steps fell 3% from the weeks before. That explains the pace, not your metabolism, so the target holds.',
  ])
})

test('in Croatian a review says it in its words, the user without a gender', () => {
  setLocale('hr')
  const [step, skip] = say([
    review(),
    review({ outcome: 'skipped', reason: 'few_data', step_kcal: 0, logged_days: 3 }),
  ])
  expect(step?.headline).toBe('Spustio je cilj za 100 kcal dnevno')
  expect(step?.line).toBe('cilj −100 kcal dnevno, od danas')
  expect(step?.why[1]).toBe(
    'TDEE je 2.600 ± 120 kcal; unos je bio 2.140 kcal dnevno uz cilj od 2.179.',
  )
  expect(skip?.headline).toBe('Bez koraka: 3 od 4 upisana dana')
})
