import type { components } from '@/api/schema'
import { addDays } from '@/i18n/format'

type Phase = components['schemas']['PhaseBody']
type Target = components['schemas']['TargetPeriodBody']

// A phase's place in time, against today: one that ended, today's, or one
// to come.
export type PhaseState = 'past' | 'now' | 'planned'

// A Span is a phase on the timeline: its days, the last one its end or
// the day before the next phase starts (undefined: open), and its place.
export interface Span {
  phase: Phase
  last: string | undefined
  state: PhaseState
}

// spansOf are the phases on the timeline, in order, each with its last
// day and its place against today (YYYY-MM-DD, the user's). A phase with
// no end lasts until the next one starts.
export function spansOf(phases: readonly Phase[], today: string): Span[] {
  return phases.map((phase, i) => {
    const next = phases[i + 1]
    const last = phase.end ?? (next === undefined ? undefined : addDays(next.start, -1))
    let state: PhaseState = 'now'
    if (phase.start > today) {
      state = 'planned'
    } else if (last !== undefined && last < today) {
      state = 'past'
    }
    return { phase, last, state }
  })
}

// targetNow is the target in force today, the last to take effect by it.
export function targetNow(targets: readonly Target[], today: string): Target | undefined {
  return targets.findLast((tg) => tg.effective_from <= today)
}

// weekdays are the days of the week as the API names them, Monday first.
export const weekdays = [
  'monday',
  'tuesday',
  'wednesday',
  'thursday',
  'friday',
  'saturday',
  'sunday',
] as const

// weeklyAverage is a day's target averaged over a week of training days
// training days and the rest rest days, as the server averages it.
export function weeklyAverage(training: number, rest: number, trainingDays: number): number {
  const n = Math.min(Math.max(trainingDays, 0), 7)
  return (n * training + (7 - n) * rest) / 7
}
