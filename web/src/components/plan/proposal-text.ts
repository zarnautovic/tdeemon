import { useQuery } from '@tanstack/react-query'

import type { components } from '@/api/schema'
import { sessionQuery } from '@/api/session'
import { limitFields, shown } from '@/components/plan/limit-fields'
import { useFormat } from '@/i18n/format'
import { useLocale } from '@/i18n/locale'
import { useMessages } from '@/i18n/messages'

export type Violation = components['schemas']['ViolationValue']
type Phase = components['schemas']['PhaseBody']
type DayTarget = components['schemas']['DayTargetBody']

// TargetValues are a target's values on each type of day.
interface TargetValues {
  training: DayTarget
  rest: DayTarget
}

const weekdays = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']

type Messages = ReturnType<typeof useMessages>
type Rule = keyof Messages['plan']['rules']
type Level = keyof Messages['plan']['levels']
type Limit = keyof Messages['plan']['limit']

// violationsOf are the violations of the limits a refusal (422) names, as
// its error details' values.
export function violationsOf(error: unknown): Violation[] {
  if (
    typeof error !== 'object' ||
    error === null ||
    !('errors' in error) ||
    !Array.isArray(error.errors)
  ) {
    return []
  }
  return error.errors.flatMap((e: unknown): Violation[] => {
    if (typeof e !== 'object' || e === null || !('value' in e)) {
      return []
    }
    const v: unknown = e.value
    if (
      typeof v !== 'object' ||
      v === null ||
      !('rule' in v) ||
      typeof v.rule !== 'string' ||
      !('limit' in v) ||
      typeof v.limit !== 'number' ||
      !('proposed' in v) ||
      typeof v.proposed !== 'number'
    ) {
      return []
    }
    return [
      {
        rule: v.rule,
        limit: v.limit,
        proposed: v.proposed,
        ...('day_type' in v && typeof v.day_type === 'string' ? { day_type: v.day_type } : {}),
        ...('level' in v && typeof v.level === 'string' ? { level: v.level } : {}),
      },
    ]
  })
}

// useLimitWords says a limit of the user's in words, as the limits'
// form shows it: its name, and a value of it with its unit (a loss as how
// fast). A name the app does not know is said as it is.
export function useLimitWords() {
  const t = useMessages()
  const f = useFormat()
  const isLimit = (name: string): name is Limit => Object.hasOwn(t.plan.limit, name)
  return {
    label: (name: string) => (isLimit(name) ? t.plan.limit[name] : name),
    value: (name: string, v: number) => {
      const field = limitFields.find((lf) => lf.name === name)
      if (field === undefined) {
        return f.field(v)
      }
      const n = shown(field, v)
      const unit = t.plan.unit[field.unit]
      return `${f.field(n)} ${typeof unit === 'string' ? unit : unit(n)}`
    },
  }
}

// useViolationText says a violation of the limits in words: what it is
// of, the value proposed and the limit, and who could allow it. A
// limit's own, past the instance's, is named as the limits' form names
// it.
export function useViolationText(): (v: Violation) => string {
  const t = useMessages()
  const f = useFormat()
  const limit = useLimitWords()
  const isRule = (r: string): r is Rule => Object.hasOwn(t.plan.rules, r)
  const isLevel = (l: string): l is Level => Object.hasOwn(t.plan.levels, l)
  return (v) => {
    const rule = isRule(v.rule) ? t.plan.rules[v.rule] : limit.label(v.rule)
    const on =
      v.day_type === 'training'
        ? ` (${t.plan.change.training})`
        : v.day_type === 'rest'
          ? ` (${t.plan.change.rest})`
          : ''
    if (v.level === undefined || !isLevel(v.level)) {
      return rule + on
    }
    return `${rule}${on}: ${t.plan.proposedLimit(f.number(v.proposed, 2), f.number(v.limit, 2), t.plan.levels[v.level])}`
  }
}

// useDate says a day of a proposal (YYYY-MM-DD), short, with its year when
// it is of another year than now's on the user's clock: a change of a year
// must not read as none.
export function useDate(): (date: string) => string {
  const f = useFormat()
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  return (date) => f.calendarDay(date, timeZone)
}

// usePlanWords says the parts of a plan in words, as a proposal's changes
// and the current plan show them: a phase (its type, rate, end, goal and
// milestone), a target (once for an unsplit one, else each type of
// day's) and the training weekdays.
export function usePlanWords() {
  const t = useMessages()
  const f = useFormat()
  const locale = useLocale()
  const date = useDate()
  const c = t.plan.change
  const weekday = new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' })
  // 2026-10-25 was a Sunday.
  const weekdayName = (name: string) =>
    weekday.format(Date.UTC(2026, 9, 25 + weekdays.indexOf(name)))
  const day = (d: DayTarget) =>
    c.day(f.number(d.kcal), f.number(d.protein, 1), f.number(d.carbs, 1), f.number(d.fat, 1))
  return {
    phase: (p: Phase) =>
      [
        `${c.types[p.type]}, ${c.rate(f.percent(p.rate_pct_week, 2))}`,
        ...(p.end === undefined ? [] : [c.until(date(p.end))]),
        ...(p.goal_weight === undefined ? [] : [c.goal(f.number(p.goal_weight, 1))]),
        ...(p.milestone_weight === undefined ? [] : [c.milestone(f.number(p.milestone_weight, 1))]),
      ].join(', '),
    target: (v: TargetValues) =>
      day(v.training) === day(v.rest)
        ? day(v.training)
        : `${c.training}: ${day(v.training)}; ${c.rest}: ${day(v.rest)}`,
    days: (names: string[]) => (names.length === 0 ? c.noDays : names.map(weekdayName).join(', ')),
  }
}
