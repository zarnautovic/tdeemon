import type { components } from '@/api/schema'
import { useChangeWords } from '@/components/plan/change-words'
import { useDate } from '@/components/plan/proposal-text'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

export type Review = components['schemas']['ReviewBody']

// ReviewWords are what a weekly review says, in the user's language.
export interface ReviewWords {
  // What it decided, a sentence: "Lowered your target by 100 kcal a day".
  headline: string
  // The day's line of it, after "Autopilot: ".
  line: string
  // Why, in the review's numbers: the first at a glance, the rest a tap
  // away.
  why: string[]
}

// useReviewWords says a weekly review from its numbers (the rationale's
// code `autopilot`): the server stores them, never its words, so a user
// who changes the language reads it in theirs.
export function useReviewWords(): (r: Review) => ReviewWords {
  const t = useMessages()
  const f = useFormat()
  const date = useDate()
  const words = useChangeWords()
  const s = t.autopilot.say
  const kcal = (n: number) => f.number(n)
  const pct = (n: number) => f.percent(n, 2)

  // pace says the measured rate against the phase's: as a loss or a gain
  // when both go the same way, else signed.
  const pace = (r: Review) => {
    const plan = r.target_pct_week
    const [lo, hi] = [plan - r.tolerance_pct_week, plan + r.tolerance_pct_week]
    const m = r.measured_pct_week
    if (plan < 0 && hi <= 0 && m <= 0) {
      return s.paceLost(pct(-m), pct(-plan), pct(-hi), pct(-lo))
    }
    if (plan > 0 && lo >= 0 && m >= 0) {
      return s.paceGained(pct(m), pct(plan), pct(lo), pct(hi))
    }
    return s.paceMoved(pct(m), pct(plan), pct(lo), pct(hi))
  }
  const energy = (r: Review): string[] => {
    if (r.tdee_kcal > 0) {
      return [
        s.energy(
          kcal(r.tdee_kcal),
          kcal(r.tdee_half_width),
          kcal(r.intake_kcal),
          kcal(r.target_kcal),
        ),
      ]
    }
    return r.intake_kcal > 0 ? [s.intakeOnly(kcal(r.intake_kcal), kcal(r.target_kcal))] : []
  }
  const rule = (name: string | undefined) =>
    Object.entries(t.plan.rules).find(([k]) => k === name)?.[1] ?? name ?? ''

  // brief is why no step came, in a few words; a reason of a step's that
  // could not be taken, its limit's.
  const brief = (r: Review): string => {
    if (r.reason === 'few_data') {
      return r.logged_days < r.needs_logged_days
        ? s.fewLogged(r.logged_days, r.needs_logged_days)
        : s.fewWeighIns(r.weigh_ins, r.needs_weigh_ins)
    }
    return Object.entries(s.skips).find(([k]) => k === r.reason)?.[1] ?? s.skips.limit
  }

  // paced is why a step was asked for: the pace, and the energy.
  const paced = (r: Review) => [pace(r), ...energy(r)]
  // reasons are why, in the numbers, by the review's reason.
  const reasons: Record<Review['reason'], (r: Review) => string[]> = {
    cut_slow: paced,
    cut_fast: paced,
    bulk_slow: paced,
    bulk_fast: paced,
    maintain_loss: paced,
    maintain_gain: paced,
    on_target: paced,
    intake: (r) => [pace(r), s.intake, ...energy(r)],
    steps_guard: (r) => {
      const by = pct(Math.abs(r.steps_change_pct))
      return [pace(r), r.steps_change_pct > 0 ? s.stepsRose(by) : s.stepsFell(by)]
    },
    rejected: (r) => [s.rejected, pace(r)],
    bulk_fat: (r) => [s.bulkFat, ...energy(r)],
    lean_loss: (r) => [s.leanLoss(pct(Math.abs(r.measured_pct_week))), ...energy(r)],
    underweight: (r) => [s.underweight(f.number(r.bmi, 1))],
    few_data: (r) => [
      s.fewData(r.needs_logged_days, r.needs_weigh_ins, r.logged_days, r.weigh_ins),
    ],
    not_trusted: (r) => [
      [
        ...(r.tdee_kcal > 0 ? [s.tdeeAbout(kcal(r.tdee_kcal), kcal(r.tdee_half_width))] : []),
        r.expected === undefined ? s.trustedLater : s.trustedAbout(date(r.expected)),
      ].join(' '),
    ],
    first_wait: (r) => [
      r.from === undefined ? s.firstWait : `${s.firstWait} ${s.firstFrom(date(r.from))}`,
    ],
    interval: (r) => [
      r.from === undefined ? s.interval : `${s.interval} ${s.nextFrom(date(r.from))}`,
    ],
    pending: () => [s.pending],
    few_carbs: (r) => [
      `${s.fewCarbs(
        words.upper(t.plan.change.days[r.day_type ?? 'every']),
        f.number(r.carbs_g),
        f.number(r.carbs_floor_g),
      )} ${s.askCoach}`,
      pace(r),
    ],
    no_plan: () => [s.noPlan],
    limit: (r) => [s.limit(rule(r.rule))],
  }

  return (r) => {
    const down = r.step_kcal < 0
    const step = kcal(Math.abs(r.step_kcal))
    let headline: string
    let line: string
    switch (r.outcome) {
      case 'applied':
        headline = down ? s.lowered(step) : s.raised(step)
        line = s.stepLine(f.signed(r.step_kcal))
        break
      case 'proposed':
        if (r.reason === 'underweight') {
          headline = s.maintenance
        } else {
          headline = down ? s.suggestsLower(step) : s.suggestsRaise(step)
        }
        line = words.lower(headline)
        break
      case 'hold': {
        const reason =
          r.reason === 'intake' || r.reason === 'steps_guard' || r.reason === 'rejected'
            ? r.reason
            : 'on_target'
        headline = s.holds[reason]
        line = s.holdLines[reason]
        break
      }
      case 'skipped':
        headline = s.noStep(brief(r))
        line = s.noStepLine(brief(r))
        break
    }
    const why = reasons[r.reason](r)
    // A step, or one proposed, says what it moves: carbs alone.
    if (r.outcome === 'applied' || (r.outcome === 'proposed' && r.reason !== 'underweight')) {
      why.push(
        s.stepCarbs(
          f.signed(r.step_kcal),
          f.signed(r.step_kcal / 4),
          kcal(r.target_kcal),
          kcal(r.after_kcal),
        ),
      )
    }
    return { headline, line, why }
  }
}
