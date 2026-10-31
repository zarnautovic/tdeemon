import type { components } from '@/api/schema'
import { targetNow, weeklyAverage } from '@/components/plan/phases'
import { useDate, useLimitWords, usePlanWords } from '@/components/plan/proposal-text'
import { addDays, useFormat } from '@/i18n/format'
import { useLocale } from '@/i18n/locale'
import { rationaleOf, useMessages } from '@/i18n/messages'

type Proposal = components['schemas']['ProposalBody']
type Actor = components['schemas']['ActorBody']
type Phase = components['schemas']['PhaseBody']
type DayTarget = components['schemas']['DayTargetBody']
type Target = components['schemas']['TargetPeriodBody']
type TargetChange = components['schemas']['TargetChangeBody']
type LimitWarning = components['schemas']['LimitWarningBody']

// TargetValues are a target's values on each type of day.
export interface TargetValues {
  training: DayTarget
  rest: DayTarget
}

// A DayLine is what a change of a target changes on some days: which
// (the training days, the rest days or every day) and each value, from
// what it was to what it is to be.
export interface DayLine {
  days: 'training' | 'rest' | 'every'
  parts: string[]
}

// makers are the makers of the AI models, as a person knows them, by how
// the model's name starts: claude-opus-5-5 is Claude.
const makers: readonly (readonly [RegExp, string])[] = [
  [/^claude/i, 'Claude'],
  [/^(gpt|chatgpt|o\d)/i, 'ChatGPT'],
  [/^gemini/i, 'Gemini'],
]

// priorOf is the target a change of the target from its day moves the
// plan from: the one it changes, or for a new one the target in force the
// day before, by the plan's targets; undefined before the first.
export function priorOf(
  ch: TargetChange,
  targets: readonly Target[] | undefined,
): TargetValues | undefined {
  return ch.before ?? (targets && targetNow(targets, addDays(ch.from, -1)))
}

// kcalShift is how far a proposal moves the weekly average of the kcal,
// over trainingDays training days, by its first change of a target that
// has one before it; 0 for none.
export function kcalShift(
  proposal: Proposal,
  targets: readonly Target[],
  trainingDays: number,
): number {
  for (const ch of proposal.changes.targets) {
    const prior = priorOf(ch, targets)
    if (ch.after !== undefined && prior !== undefined) {
      const avg = (v: TargetValues) => weeklyAverage(v.training.kcal, v.rest.kcal, trainingDays)
      return Math.round(avg(ch.after) - avg(prior))
    }
  }
  return 0
}

// useWho says who proposed or made a change of the plan: the maker of
// the model that acted, with the model; the AI coach, for a model not
// named; the autopilot, the server's own; or the user, in the app.
export function useWho(): (by: Actor) => string {
  const t = useMessages()
  return (by) => {
    if (by.client === 'autopilot') {
      return t.autopilot.name
    }
    const model = by.model ?? ''
    if (model !== '') {
      const maker = makers.find(([name]) => name.test(model))?.[1] ?? t.plan.pending.coach
      return t.plan.pending.who(maker, model)
    }
    return by.client === 'web' ? t.plan.byUser : t.plan.pending.coach
  }
}

// useRationale says why a change of the plan was proposed: a coach's
// words as they were written, and the app's code in the language.
export function useRationale(): (rationale: string) => string {
  const t = useMessages()
  return (rationale) => {
    const code = rationaleOf(rationale)
    return code === undefined ? rationale : t.plan.rationales[code]
  }
}

// useChangeWords says what a change of the plan changes, and only that:
// each value from what it was to what it is to be, as shown (a gram that
// rounds to the same is no change), "1,950 → 1,850 kcal". upper starts a
// line with a capital, as the language writes one.
export function useChangeWords() {
  const t = useMessages()
  const f = useFormat()
  const locale = useLocale()
  const date = useDate()
  const c = t.plan.change
  const s = t.day.short
  const pct = (n: number) => f.percent(n, 2)
  const weight = (kg: number | undefined) => (kg === undefined ? '–' : f.number(kg, 1))
  const end = (d: string | undefined) => (d === undefined ? c.open : c.until(date(d)))
  const dayParts = (b: DayTarget, a: DayTarget): string[] => [
    ...(f.number(b.kcal) === f.number(a.kcal) ? [] : [c.kcal(f.number(b.kcal), f.number(a.kcal))]),
    ...(['protein', 'carbs', 'fat'] as const).flatMap((m) =>
      f.grams(b[m]) === f.grams(a[m]) ? [] : [c.grams(s[m], f.grams(b[m]), f.grams(a[m]))],
    ),
  ]
  // unsplit tells a target the same on both types of day, as shown.
  const unsplit = (v: TargetValues) => dayParts(v.training, v.rest).length === 0
  return {
    upper: (text: string) => text.charAt(0).toLocaleUpperCase(locale) + text.slice(1),
    lower: (text: string) => text.charAt(0).toLocaleLowerCase(locale) + text.slice(1),
    // target is what a target changes on each type of day; once, of
    // every day, for one the same on both before and after.
    target: (before: TargetValues, after: TargetValues): DayLine[] => {
      if (unsplit(before) && unsplit(after)) {
        const parts = dayParts(before.training, after.training)
        return parts.length === 0 ? [] : [{ days: 'every', parts }]
      }
      return (['training', 'rest'] as const).flatMap((days) => {
        const parts = dayParts(before[days], after[days])
        return parts.length === 0 ? [] : [{ days, parts }]
      })
    },
    // phase is what an edit of a phase changes: its type, its rate, its
    // end, its goal and its milestone.
    phase: (b: Phase, a: Phase): string[] => [
      ...(b.type === a.type ? [] : [c.to(c.types[b.type], c.types[a.type])]),
      ...(pct(b.rate_pct_week) === pct(a.rate_pct_week)
        ? []
        : [c.rate(c.to(pct(b.rate_pct_week), pct(a.rate_pct_week)))]),
      ...(b.end === a.end
        ? []
        : [
            b.end !== undefined && a.end !== undefined
              ? c.until(c.to(date(b.end), date(a.end)))
              : c.to(end(b.end), end(a.end)),
          ]),
      ...(weight(b.goal_weight) === weight(a.goal_weight)
        ? []
        : [c.goal(c.to(weight(b.goal_weight), weight(a.goal_weight)))]),
      ...(weight(b.milestone_weight) === weight(a.milestone_weight)
        ? []
        : [c.milestone(c.to(weight(b.milestone_weight), weight(a.milestone_weight)))]),
    ],
  }
}

// useWarningText says a warning of the limits in plain words, with why it
// matters, of the type of day it is of.
export function useWarningText(): (w: LimitWarning) => string {
  const t = useMessages()
  const f = useFormat()
  const words = useChangeWords()
  return (w) => {
    const said = t.plan.warnings[w.rule](f.number(w.value), f.number(w.threshold))
    return w.day_type === undefined
      ? said
      : `${words.upper(t.plan.change.days[w.day_type])}: ${words.lower(said)}`
  }
}

// useGist says the gist of a proposal in a line: its first change, short
// (a target by its first value changed on each type of day, "rest days
// 1,950 → 1,850 kcal"), and how many more it has.
export function useGist(): (proposal: Proposal, targets: readonly Target[]) => string {
  const t = useMessages()
  const date = useDate()
  const words = useChangeWords()
  const plan = usePlanWords()
  const limit = useLimitWords()
  const c = t.plan.change
  return (proposal, targets) => {
    const { phases, targets: changes, training_weekdays: days, limits } = proposal.changes
    const gists = [
      ...phases.map((ch) => {
        if (ch.after === undefined) {
          return c.deletedPhase(date(ch.start))
        }
        if (ch.before === undefined) {
          return `${c.newPhase(date(ch.start))}: ${plan.phase(ch.after)}`
        }
        return `${c.phase(date(ch.start))}: ${words.phase(ch.before, ch.after).join(', ')}`
      }),
      ...changes.map((ch) => {
        const prior = priorOf(ch, targets)
        if (ch.after === undefined) {
          return c.deletedTarget(date(ch.from))
        }
        if (prior === undefined) {
          return `${c.newTarget(date(ch.from))}: ${plan.target(ch.after)}`
        }
        return words
          .target(prior, ch.after)
          .map((line) => `${c.days[line.days]} ${line.parts[0] ?? ''}`)
          .join(', ')
      }),
      ...(days === undefined ? [] : [`${c.weekdays}: ${plan.days(days.after)}`]),
      ...limits.map(
        (ch) =>
          `${c.limit(limit.label(ch.name))}: ${c.to(limit.value(ch.name, ch.before), limit.value(ch.name, ch.after))}`,
      ),
    ]
    const first = words.lower(gists[0] ?? '')
    return gists.length > 1 ? `${first} (+${String(gists.length - 1)})` : first
  }
}
