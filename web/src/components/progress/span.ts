import { addDays, daysBetween } from '@/i18n/format'

// Span is the period the progress shows: so many days up to today, or the
// phase today is in.
export type Span = '30' | '90' | '180' | '365' | 'phase'

export const spans: readonly Span[] = ['30', '90', '180', '365', 'phase']

// A history is of 366 days at most (the API's), and its table of weeks
// of a year's.
const mostDays = 366
const mostWeeks = 52

// firstDay is a span's first day, up to today: the phase's first, but no
// more than the days a history has.
export function firstDay(span: Span, today: string, phaseStart?: string): string {
  if (span !== 'phase') {
    return addDays(today, 1 - Number(span))
  }
  const earliest = addDays(today, 1 - mostDays)
  return phaseStart === undefined || phaseStart < earliest ? earliest : phaseStart
}

// byMonth reports whether a span's axis names months rather than days:
// one of more than half a year.
export function byMonth(from: string, today: string): boolean {
  return daysBetween(from, today) + 1 > 180
}

// weeksOf is the table of weeks of a span: each day of it up to
// yesterday (today is open) in a week, at most a year's. from is the
// history to ask for, a week before the first, whose trend weight the
// first's change is measured from; count the weeks shown, 0 when the span
// has none over yet.
export function weeksOf(first: string, today: string): { from: string; to: string; count: number } {
  const to = addDays(today, -1)
  const days = daysBetween(first, to) + 1
  const count = Math.max(0, Math.min(mostWeeks, Math.ceil(days / 7)))
  return { from: addDays(to, -7 * count), to, count }
}
