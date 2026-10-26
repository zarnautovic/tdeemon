import { useQuery } from '@tanstack/react-query'

import type { components } from '@/api/schema'
import { sessionQuery } from '@/api/session'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

export type Violation = components['schemas']['ViolationValue']

type Messages = ReturnType<typeof useMessages>
type Rule = keyof Messages['plan']['rules']
type Level = keyof Messages['plan']['levels']

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

// useViolationText says a violation of the limits in words: what it is
// of, the value proposed and the limit, and who could allow it.
export function useViolationText(): (v: Violation) => string {
  const t = useMessages()
  const f = useFormat()
  const isRule = (r: string): r is Rule => Object.hasOwn(t.plan.rules, r)
  const isLevel = (l: string): l is Level => Object.hasOwn(t.plan.levels, l)
  return (v) => {
    const rule = isRule(v.rule) ? t.plan.rules[v.rule] : v.rule
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
